package jobsched

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/x64c/gwf/gw/svc"
)

type Service struct {
	name        string             // registered instance identity; see NewServiceAs
	ctx         context.Context    // per-cycle runtime context (set in Start)
	cancel      context.CancelFunc // per-cycle cancel (set in Start)
	state       svc.AtomicState    // internal service state (State() may be read concurrently with lifecycle writes)
	terminated  chan error         // one-shot; fires when Terminate completes
	stopped     chan struct{}      // per-cycle; closed when run goroutine has stopped
	oneTimeJobs map[int64][]*OneTimeJob
	cronJobs    map[string]*CronJob
	mu          sync.Mutex
	wg          sync.WaitGroup

	// callbacks is boot wiring behind SetCallbacks: the fields are plain funcs
	// read from job goroutines, so a write after Start would be a data race —
	// which is why they are not exported fields.
	callbacks Callbacks
}

// Callbacks are the service-level job-event callbacks, set as one unit at boot
// via SetCallbacks. nil fields are simply not called.
type Callbacks struct {
	OnOneTimeJobAdded    func(job *OneTimeJob)
	OnCronJobAdded       func(job *CronJob)
	OnOneTimeJobFinished func(job *OneTimeJob, err error)
	OnCronJobFinished    func(job *CronJob, err error)
	OnOneTimeJobDeleted  func(job *OneTimeJob)
	OnCronJobDeleted     func(job *CronJob)
}

// SetCallbacks installs the service-level callbacks. Callbacks are BOOT
// WIRING: they are read from job goroutines without a lock, race-free only
// under the contract that the write happens before Start. A call in any state
// but READY is refused with an error — refusal, not a process kill, same as
// throttle.SetBucketGroup.
func (s *Service) SetCallbacks(cb Callbacks) error {
	if state := s.state.Load(); state != svc.StateREADY {
		return fmt.Errorf("jobsched %q: can't set callbacks: state is %v — callbacks are boot wiring, set before Start", s.Name(), state)
	}
	s.callbacks = cb
	return nil
}

func (s *Service) Name() string {
	return s.name
}

func (s *Service) State() svc.State {
	return s.state.Load()
}

func NewService() *Service {
	return NewServiceAs("JobSchedulerService")
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
func NewServiceAs(name string) *Service {
	s := &Service{
		name:        name,
		terminated:  make(chan error, 1),
		oneTimeJobs: make(map[int64][]*OneTimeJob),
		cronJobs:    make(map[string]*CronJob),
	}
	s.state.Store(svc.StateREADY)
	return s
}

// UseDefaultLoggers installs default logging callbacks — SetCallbacks with a
// stock Callbacks value, same boot-wiring contract.
func (s *Service) UseDefaultLoggers() error {
	return s.SetCallbacks(Callbacks{
		OnOneTimeJobAdded: func(job *OneTimeJob) {
			log.Printf("[INFO] One-time job added: %s for %v", job.ID, job.ExecTime)
		},
		OnCronJobAdded: func(job *CronJob) {
			log.Printf("[INFO] cron job added: %s", job.ID)
		},
		OnCronJobFinished: func(job *CronJob, err error) {
			if err == nil {
				log.Printf("[INFO] cron job finished: %s", job.ID)
			} else {
				log.Printf("[INFO] cron job finished: %s with error: %v", job.ID, err)
			}
		},
		OnOneTimeJobFinished: func(job *OneTimeJob, err error) {
			if err == nil {
				log.Printf("[INFO] one-time job finished: %s", job.ID)
			} else {
				log.Printf("[INFO] one-time job finished: %s with error: %v", job.ID, err)
			}
		},
	})
}

// Start : READY → RUNNING. parentCtx is the runtime cancellation lineage.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state.Load())
	}
	log.Printf("[INFO][%s] Starting.", s.Name())
	s.ctx, s.cancel = context.WithCancel(parentCtx)
	s.stopped = make(chan struct{}) // fresh per cycle
	s.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running.", s.Name())
	go s.run()
	return nil
}

// Stop : RUNNING → STOPPING → READY. Synchronous on the run goroutine's exit
// (which waits for all worker goroutines to finish first).
func (s *Service) Stop(ctx context.Context) error {
	if s.state.Load() == svc.StateREADY {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", s.state.Load())
	}
	s.state.Store(svc.StateSTOPPING)
	return s.stop(ctx)
}

// Terminate : any → TERMINATING (irreversible). If RUNNING, full stop;
// if STOPPING, just wait for run goroutine to exit. Fires Terminated.
func (s *Service) Terminate(ctx context.Context) (err error) {
	if s.state.Load() == svc.StateTERMINATING {
		return nil // idempotent — returns before the defer arms
	}
	prevState := s.state.Load()
	s.state.Store(svc.StateTERMINATING)
	log.Printf("[INFO][%s] Terminating.", s.Name())
	defer func() {
		s.terminated <- err // THE ONLY send site; unconditional, exactly once
		if err == nil {
			log.Printf("[INFO][%s] Terminated.", s.Name())
		} else {
			log.Printf("[ERROR][%s] Terminated with stop error: %v", s.Name(), err)
		}
	}()
	switch prevState {
	case svc.StateRUNNING:
		err = s.stop(ctx)
	case svc.StateSTOPPING:
		err = s.waitStopped(ctx)
	}
	return err
}

// stop runs the full stop activity: log "Stopping.", cancel, waitStopped.
func (s *Service) stop(ctx context.Context) error {
	log.Printf("[INFO][%s] Stopping.", s.Name())
	s.cancel()
	return s.waitStopped(ctx)
}

// waitStopped waits for the run goroutine to exit; logs "Stopped." on success.
func (s *Service) waitStopped(ctx context.Context) error {
	select {
	case <-s.stopped:
		log.Printf("[INFO][%s] Stopped.", s.Name())
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop deadline exceeded: %w", ctx.Err())
	}
}

func (s *Service) Terminated() <-chan error {
	return s.terminated
}

func (s *Service) run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	defer close(s.stopped)
	defer s.transitionAfterRun() // LIFO: runs first
	for {
		select {
		case <-s.ctx.Done():
			s.wg.Wait() // wait for all worker goroutines
			return
		case now := <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[PANIC][%s] recovered: %v\n%s", s.Name(), r, debug.Stack())
					}
				}()
				s.runOneTimeJobs(now)
				s.runCronJobs(now)
			}()
		}
	}
}

func (s *Service) transitionAfterRun() {
	if s.state.Load() == svc.StateSTOPPING {
		s.state.Store(svc.StateREADY)
	}
}

// GetOneTimeJobs returns a snapshot of all pending one-time jobs, keyed by
// their scheduled minute-level timestamp. The jobs are struct copies: mutating
// one changes nothing the scheduler runs (svc.Service: no escape hatches).
func (s *Service) GetOneTimeJobs() map[int64][]*OneTimeJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[int64][]*OneTimeJob, len(s.oneTimeJobs))
	for key, jobs := range s.oneTimeJobs {
		copies := make([]*OneTimeJob, len(jobs))
		for i, job := range jobs {
			j := *job
			copies[i] = &j
		}
		result[key] = copies
	}
	return result
}

// GetCronJobs returns a snapshot of all registered cron jobs, keyed by their
// ID. The jobs are struct copies: mutating one changes nothing the scheduler
// runs (svc.Service: no escape hatches).
func (s *Service) GetCronJobs() map[string]*CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]*CronJob, len(s.cronJobs))
	for id, job := range s.cronJobs {
		j := *job
		result[id] = &j
	}
	return result
}

// runTask runs one job's Task on the job's own goroutine, converting a panic
// into the error handed to the job's callbacks: the job fails, the scheduler
// lives. The stack is logged here, where it exists. Without this, a panicking
// job killed the whole process — the job goroutine has no other recover.
func (s *Service) runTask(jobKind, jobID string, task func() error) (err error) {
	defer func() {
		if rcv := recover(); rcv != nil {
			log.Printf("[PANIC][%s] %s %q Task panicked: %v\n%s", s.Name(), jobKind, jobID, rcv, debug.Stack())
			err = fmt.Errorf("%s %q panicked: %v", jobKind, jobID, rcv)
		}
	}()
	return task()
}

// runCallback runs one job callback (OnFinished, or a service-level
// On*JobFinished) on the job's goroutine under the same guard as runTask: a
// panicking callback fails the callback, not the process.
func (s *Service) runCallback(jobKind, jobID, callback string, f func()) {
	defer func() {
		if rcv := recover(); rcv != nil {
			log.Printf("[PANIC][%s] %s %q %s panicked: %v\n%s", s.Name(), jobKind, jobID, callback, rcv, debug.Stack())
		}
	}()
	f()
}

func (s *Service) AddOneTimeJob(job *OneTimeJob) error {
	now := time.Now()
	margin := 30 * time.Second
	if job.ExecTime.Before(now.Add(margin)) {
		return fmt.Errorf(
			"cannot schedule job %s too close or in the past (ExecTime: %s, now: %s)",
			job.ID, job.ExecTime, now,
		)
	}
	// Round up to the next minute if ExecTime has seconds/nanoseconds
	regTime := job.ExecTime
	if regTime.Second() > 0 || regTime.Nanosecond() > 0 {
		regTime = regTime.Truncate(time.Minute).Add(time.Minute)
	}
	key := regTime.Unix() / 60
	s.mu.Lock()
	if s.oneTimeJobs == nil {
		s.oneTimeJobs = make(map[int64][]*OneTimeJob) // safety net
	}
	s.oneTimeJobs[key] = append(s.oneTimeJobs[key], job) // to make this safer?
	s.mu.Unlock()
	if job.OnAdded != nil { // Job-specific callback
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Println("[PANIC] Recovered in job.OnAdded:", r)
				}
			}()
			job.OnAdded()
		}()
	}
	if s.callbacks.OnOneTimeJobAdded != nil { // Service-level default callback
		s.callbacks.OnOneTimeJobAdded(job)
	}
	return nil
}

func (s *Service) AddCronJob(job *CronJob) error {
	s.mu.Lock()
	if s.cronJobs == nil {
		s.cronJobs = make(map[string]*CronJob)
	}
	if _, exists := s.cronJobs[job.ID]; exists {
		return fmt.Errorf("cron job with ID %q already exists", job.ID)
	}
	s.cronJobs[job.ID] = job
	s.mu.Unlock()
	// Job-specific callback
	if job.OnAdded != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Println("[PANIC] Recovered in job.OnAdded:", r)
				}
			}()
			job.OnAdded()
		}()
	}
	// Service-level default callback
	if s.callbacks.OnCronJobAdded != nil {
		s.callbacks.OnCronJobAdded(job)
	}
	return nil
}

// DeleteOneTimeJob - Delete a job
func (s *Service) DeleteOneTimeJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, jobs := range s.oneTimeJobs {
		filtered := jobs[:0]
		for _, job := range jobs {
			if job.ID == jobID {
				if s.callbacks.OnOneTimeJobDeleted != nil {
					s.callbacks.OnOneTimeJobDeleted(job)
				}
			} else {
				filtered = append(filtered, job)
			}
		}
		if len(filtered) == 0 {
			delete(s.oneTimeJobs, key)
		} else {
			s.oneTimeJobs[key] = filtered
		}
	}
}

// DeleteCronJob removes a cron job by its ID
func (s *Service) DeleteCronJob(jobID string) {
	s.mu.Lock()
	job, exists := s.cronJobs[jobID]
	if !exists {
		s.mu.Unlock()
		return
	}
	delete(s.cronJobs, jobID)
	s.mu.Unlock()
	// trigger global delete callback outside lock
	if s.callbacks.OnCronJobDeleted != nil {
		s.callbacks.OnCronJobDeleted(job)
	}
}
