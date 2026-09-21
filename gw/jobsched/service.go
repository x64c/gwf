package jobsched

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/x64c/gwf/gw/coord"
	"github.com/x64c/gwf/gw/svc"
)

type Service struct {
	name                                 string             // registered instance identity; see NewServiceAs
	coordMode                            coord.Mode         // the app's own coordination mode; a job's scope is read against it
	ctx                                  context.Context    // per-cycle runtime context (set in Start)
	cancel                               context.CancelFunc // per-cycle cancel (set in Start)
	state                                svc.AtomicState    // internal service state (State() may be read concurrently with lifecycle writes)
	terminated                           chan error         // one-shot; fires when Terminate completes
	stopped                              chan struct{}      // per-cycle; closed when run goroutine has stopped
	oneTimeJobsPerInstance               map[int64][]*OneTimeJob
	cronJobsPerInstance                  map[string]*CronJob
	oneTimeJobsPerApp                    map[int64][]*OneTimeJob
	cronJobsPerApp                       map[string]*CronJob
	ledgerForJobsPerAppCrossProcProvider LedgerForJobsPerAppCrossProcProvider // asked once, on the first OncePerApp job
	ledger                               LedgerForJobsPerAppCrossProc         // what it yielded; nil until then, and forever if no such job is registered
	ledgerMu                             sync.Mutex                           // guards the resolution above; never held with mu
	mu                                   sync.Mutex
	wg                                   sync.WaitGroup

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

// NewService builds the scheduler for an app coordinating as coordMode.
// perAppJobLedgerProvider is where the ledger for its OncePerApp jobs comes
// from, and it is asked only if such a job is ever registered.
func NewService(coordMode coord.Mode, perAppJobLedgerProvider LedgerForJobsPerAppCrossProcProvider) (*Service, error) {
	return NewServiceAs("JobSchedulerService", coordMode, perAppJobLedgerProvider)
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
//
// coordMode is the app's own coordination mode, and a scheduler that does not
// know it cannot say what a job's Scope means. An undeclared mode is refused
// here, once, rather than at every job.
func NewServiceAs(name string, coordMode coord.Mode, perAppJobLedgerProvider LedgerForJobsPerAppCrossProcProvider) (*Service, error) {
	if coordMode != coord.InProc && coordMode != coord.CrossProc {
		return nil, fmt.Errorf("jobsched %q: coordination mode %v — a mode is a choice, never a default", name, coordMode)
	}
	s := &Service{
		name:                                 name,
		coordMode:                            coordMode,
		terminated:                           make(chan error, 1),
		oneTimeJobsPerInstance:               make(map[int64][]*OneTimeJob),
		cronJobsPerInstance:                  make(map[string]*CronJob),
		oneTimeJobsPerApp:                    make(map[int64][]*OneTimeJob),
		cronJobsPerApp:                       make(map[string]*CronJob),
		ledgerForJobsPerAppCrossProcProvider: perAppJobLedgerProvider,
	}
	s.state.Store(svc.StateREADY)
	return s, nil
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
				s.runOneTimeJobsPerInstance(now)
				s.runCronJobsPerInstance(now)
				s.runOneTimeJobsPerApp(now)
				s.runCronJobsPerApp(now)
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

	result := make(map[int64][]*OneTimeJob, len(s.oneTimeJobsPerInstance)+len(s.oneTimeJobsPerApp))
	for _, box := range []map[int64][]*OneTimeJob{s.oneTimeJobsPerInstance, s.oneTimeJobsPerApp} {
		for key, jobs := range box {
			for _, job := range jobs {
				j := *job
				result[key] = append(result[key], &j)
			}
		}
	}
	return result
}

// GetCronJobs returns a snapshot of all registered cron jobs, keyed by their
// ID. The jobs are struct copies: mutating one changes nothing the scheduler
// runs (svc.Service: no escape hatches).
func (s *Service) GetCronJobs() map[string]*CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]*CronJob, len(s.cronJobsPerInstance)+len(s.cronJobsPerApp))
	for _, box := range []map[string]*CronJob{s.cronJobsPerInstance, s.cronJobsPerApp} {
		for id, job := range box {
			j := *job
			result[id] = &j
		}
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

// resolveLedgerForJobsPerAppCrossProc resolves the ledger on first need and
// keeps it. The provider is asked once per successful answer, never for a
// per-instance job, and a failure is returned rather than remembered.
func (s *Service) resolveLedgerForJobsPerAppCrossProc() (LedgerForJobsPerAppCrossProc, error) {
	s.ledgerMu.Lock()
	defer s.ledgerMu.Unlock()
	if s.ledger != nil {
		return s.ledger, nil
	}
	if s.ledgerForJobsPerAppCrossProcProvider == nil {
		return nil, fmt.Errorf("jobsched %q: no ledger provider", s.Name())
	}
	ledger, err := s.ledgerForJobsPerAppCrossProcProvider()
	if err != nil {
		return nil, err
	}
	if ledger == nil {
		return nil, fmt.Errorf("jobsched %q: the ledger provider yielded none", s.Name())
	}
	s.ledger = ledger
	return ledger, nil
}

// addOneTimeJobPerAppInCrossProcMode takes the one-time jobs AddOneTimeJob
// cannot: one instance must be chosen for the job's planned minute, which is
// what the ledger decides.
func (s *Service) addOneTimeJobPerAppInCrossProcMode(job *OneTimeJob) error {
	if _, err := s.resolveLedgerForJobsPerAppCrossProc(); err != nil {
		return fmt.Errorf("jobsched %q: one-time job %q: %w", s.Name(), job.ID, err)
	}
	if err := s.storeOneTimeJob(job, s.oneTimeJobsPerApp); err != nil {
		return err
	}
	s.notifyOneTimeJobAdded(job)
	return nil
}

func (s *Service) AddOneTimeJob(job *OneTimeJob) error {
	if s.coordMode == coord.CrossProc && job.Scope == OncePerApp {
		return s.addOneTimeJobPerAppInCrossProcMode(job)
	}
	// s.coordMode == coord.InProc || job.Scope == OncePerInstance:
	// under InProc, OncePerApp collapses into PerInstance — one process is the
	// whole app, so its one firing is this instance's.
	return s.addOneTimeJobPerInstance(job)
}

func (s *Service) addOneTimeJobPerInstance(job *OneTimeJob) error {
	if err := s.storeOneTimeJob(job, s.oneTimeJobsPerInstance); err != nil {
		return err
	}
	s.notifyOneTimeJobAdded(job)
	return nil
}

// storeOneTimeJob checks the job's timing and files it in into, under the
// minute it will fire in — the same arithmetic the tick uses to look it up.
func (s *Service) storeOneTimeJob(job *OneTimeJob, into map[int64][]*OneTimeJob) error {
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
	defer s.mu.Unlock()
	if into == nil {
		return fmt.Errorf("jobsched %q: one-time job %q: scheduler not built by NewService", s.Name(), job.ID)
	}
	into[key] = append(into[key], job)
	return nil
}

// notifyOneTimeJobAdded fires the added-callbacks outside the lock, job-specific
// one first. A panicking job callback fails the callback, not the process.
func (s *Service) notifyOneTimeJobAdded(job *OneTimeJob) {
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
}

// addCronJobPerAppInCrossProcMode takes the cron jobs AddCronJob cannot: one
// instance must be chosen for each planned minute, which is what the ledger
// decides.
func (s *Service) addCronJobPerAppInCrossProcMode(job *CronJob) error {
	if _, err := s.resolveLedgerForJobsPerAppCrossProc(); err != nil {
		return fmt.Errorf("jobsched %q: cron job %q: %w", s.Name(), job.ID, err)
	}
	if err := s.storeCronJob(job, s.cronJobsPerApp); err != nil {
		return err
	}
	s.notifyCronJobAdded(job)
	return nil
}

func (s *Service) AddCronJob(job *CronJob) error {
	if s.coordMode == coord.CrossProc && job.Scope == OncePerApp {
		return s.addCronJobPerAppInCrossProcMode(job)
	}
	// s.coordMode == coord.InProc || job.Scope == OncePerInstance:
	// under InProc, OncePerApp collapses into PerInstance — one process is the
	// whole app, so its one firing is this instance's.
	return s.addCronJobPerInstance(job)
}

func (s *Service) addCronJobPerInstance(job *CronJob) error {
	if err := s.storeCronJob(job, s.cronJobsPerInstance); err != nil {
		return err
	}
	s.notifyCronJobAdded(job)
	return nil
}

// storeCronJob files the job in into. An id must be unique across BOTH boxes:
// DeleteCronJob and GetCronJobs span them, so two jobs sharing an id would make
// either of those ambiguous.
func (s *Service) storeCronJob(job *CronJob, into map[string]*CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if into == nil {
		return fmt.Errorf("jobsched %q: cron job %q: scheduler not built by NewService", s.Name(), job.ID)
	}
	if _, exists := s.cronJobsPerInstance[job.ID]; exists {
		return fmt.Errorf("cron job with ID %q already exists", job.ID)
	}
	if _, exists := s.cronJobsPerApp[job.ID]; exists {
		return fmt.Errorf("cron job with ID %q already exists", job.ID)
	}
	into[job.ID] = job
	return nil
}

// notifyCronJobAdded fires the added-callbacks outside the lock, job-specific
// one first. A panicking job callback fails the callback, not the process.
func (s *Service) notifyCronJobAdded(job *CronJob) {
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
}

// DeleteOneTimeJob - Delete a job
func (s *Service) DeleteOneTimeJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, box := range []map[int64][]*OneTimeJob{s.oneTimeJobsPerInstance, s.oneTimeJobsPerApp} {
		for key, jobs := range box {
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
				delete(box, key)
			} else {
				box[key] = filtered
			}
		}
	}
}

// DeleteCronJob removes a cron job by its ID
func (s *Service) DeleteCronJob(jobID string) {
	s.mu.Lock()
	job, exists := s.cronJobsPerInstance[jobID]
	if exists {
		delete(s.cronJobsPerInstance, jobID)
	} else if job, exists = s.cronJobsPerApp[jobID]; exists {
		delete(s.cronJobsPerApp, jobID)
	}
	s.mu.Unlock()
	if !exists {
		return
	}
	// trigger global delete callback outside lock
	if s.callbacks.OnCronJobDeleted != nil {
		s.callbacks.OnCronJobDeleted(job)
	}
}

func (s *Service) runOneTimeJobsPerInstance(now time.Time) {
	key := now.Unix() / 60
	s.mu.Lock()
	jobs := s.oneTimeJobsPerInstance[key]
	delete(s.oneTimeJobsPerInstance, key)
	s.mu.Unlock()
	for _, job := range jobs {
		s.runOneTimeJob(job)
	}
}

// resolvedLedgerForJobsPerAppCrossProc returns the ledger if it has already
// been resolved, without asking the provider: the tick must not do boot wiring.
// A per-app job is only ever stored after a successful resolution, so a nil
// here with jobs present is a bug, not a configuration.
func (s *Service) resolvedLedgerForJobsPerAppCrossProc() LedgerForJobsPerAppCrossProc {
	s.ledgerMu.Lock()
	defer s.ledgerMu.Unlock()
	return s.ledger
}

// runOneTimeJobsPerApp takes this minute's per-app one-time jobs and runs the
// ones this instance claims. The local take happens first, exactly as in the
// per-instance walk, so a job is never attempted twice here; the other
// instances hold their own copies and contend for the same mark.
//
// A claim error is not a verdict, so the job is not run — and since every
// instance drops its copy on its own take, the occurrence may pass unfired.
// That is the fail-closed direction: a duplicate run cannot be undone.
func (s *Service) runOneTimeJobsPerApp(now time.Time) {
	plannedMinute := now.Unix() / 60
	s.mu.Lock()
	jobs := s.oneTimeJobsPerApp[plannedMinute]
	delete(s.oneTimeJobsPerApp, plannedMinute)
	s.mu.Unlock()
	if len(jobs) == 0 {
		return
	}
	ledger := s.resolvedLedgerForJobsPerAppCrossProc()
	if ledger == nil {
		log.Printf("[ERROR][%s] %d per-app one-time job(s) due with no ledger — not run", s.Name(), len(jobs))
		return
	}
	for _, job := range jobs {
		won, err := ledger.Claim(s.ctx, job.ID, plannedMinute)
		if err != nil {
			log.Printf("[ERROR][%s] one-time job %q: claim failed, not run: %v", s.Name(), job.ID, err)
			continue
		}
		if !won {
			continue // another instance has this minute
		}
		s.runOneTimeJob(job)
	}
}

// runCronJobsPerApp runs the per-app cron jobs whose schedule matches this
// minute, each only if this instance claims that minute. Losing is a normal
// outcome and is silent; the job stays registered and contends again at its
// next occurrence.
func (s *Service) runCronJobsPerApp(now time.Time) {
	s.mu.Lock()
	jobs := make([]*CronJob, 0, len(s.cronJobsPerApp))
	for _, job := range s.cronJobsPerApp {
		jobs = append(jobs, job)
	}
	s.mu.Unlock()
	if len(jobs) == 0 {
		return
	}
	ledger := s.resolvedLedgerForJobsPerAppCrossProc()
	if ledger == nil {
		log.Printf("[ERROR][%s] %d per-app cron job(s) with no ledger — none run", s.Name(), len(jobs))
		return
	}
	plannedMinute := now.Unix() / 60
	for _, job := range jobs {
		if !job.Matches(now) {
			continue
		}
		won, err := ledger.Claim(s.ctx, job.ID, plannedMinute)
		if err != nil {
			log.Printf("[ERROR][%s] cron job %q: claim failed, occurrence skipped: %v", s.Name(), job.ID, err)
			continue
		}
		if !won {
			continue // another instance has this minute
		}
		s.runCronJob(job)
	}
}

func (s *Service) runOneTimeJob(job *OneTimeJob) {
	s.wg.Go(func() {
		err := s.runTask("one-time job", job.ID, job.Task)
		if job.OnFinished != nil {
			s.runCallback("one-time job", job.ID, "OnFinished", func() { job.OnFinished(err) })
		}
		if s.callbacks.OnOneTimeJobFinished != nil {
			s.runCallback("one-time job", job.ID, "OnOneTimeJobFinished", func() { s.callbacks.OnOneTimeJobFinished(job, err) })
		}
	})
}

func (s *Service) runCronJobsPerInstance(now time.Time) {
	s.mu.Lock()
	// Copy values to a slice so we can unlock early
	jobs := make([]*CronJob, 0, len(s.cronJobsPerInstance))
	for _, job := range s.cronJobsPerInstance {
		jobs = append(jobs, job)
	}
	s.mu.Unlock()
	for _, job := range jobs {
		if job.Matches(now) {
			s.runCronJob(job)
		}
	}
}

func (s *Service) runCronJob(job *CronJob) {
	s.wg.Go(func() {
		err := s.runTask("cron job", job.ID, job.Task)
		if job.OnFinished != nil {
			s.runCallback("cron job", job.ID, "OnFinished", func() { job.OnFinished(err) })
		}
		if s.callbacks.OnCronJobFinished != nil {
			s.runCallback("cron job", job.ID, "OnCronJobFinished", func() { s.callbacks.OnCronJobFinished(job, err) })
		}
	})
}
