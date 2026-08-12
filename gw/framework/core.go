package framework

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/x64c/gwf/gw/jobsched"
	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/sqldbs"
	"github.com/x64c/gwf/gw/storages"
	"github.com/x64c/gwf/gw/svc"
	"github.com/x64c/gwf/gw/throttle"
	"github.com/x64c/gwf/gw/uds"
	"github.com/x64c/gwf/gw/web"
	"github.com/x64c/gwf/gw/web/fwupstream"
	"github.com/x64c/gwf/gw/web/requests"
	"github.com/x64c/gwf/gw/web/session"
)

type Core struct {
	AppName              string                                   `json:"app_name"`
	Listen               string                                   `json:"listen"`                 // HTTP Application Listen IP:PORT Address
	Host                 string                                   `json:"host"`                   // HTTP Host. Can be used to generate public url endpoints
	DebugOpts            DebugOpts                                `json:"debug_opts"`             // Debug Options
	TerminateTimeoutSecs int                                      `json:"terminate_timeout_secs"` // REQUIRED (> 0). PER-SERVICE Terminate budget (each service gets a fresh deadline, sequentially). Worst-case total = N services × this. That total must fit under the process supervisor's kill window (e.g. systemd < 90s, launchd < 20s, docker stop < 10s).
	AppRoot              string                                   `json:"-"`                      // Filled from compiled paths
	RootCtx              context.Context                          `json:"-"`                      // Global Context with RootCancel
	RootCancel           context.CancelFunc                       `json:"-"`                      // CancelFunc for RootCtx
	UDSService           *uds.Service                             `json:"-"`                      // PrepareUDSService
	JobSchedulerService  *jobsched.Service                        `json:"-"`                      // PrepareJobSchedulerService
	WebService           *web.Service                             `json:"-"`                      // PrepareWebService
	ClientIPResolver     requests.ClientIPResolver                `json:"-"`                      // PrepareWebService — derives the caller's address per the deployment's trusted proxies
	SessionService       *session.Service                         `json:"-"`                      // PrepareSessionService, PrepareCookieSessions, PrepareBearerSessions
	ThrottleService      *throttle.Service                        `json:"-"`                      // PrepareThrottleService
	VolatileKV           *sync.Map                                `json:"-"`                      // map[string]string
	ActionLocks          *sync.Map                                `json:"-"`                      // map[string]struct{}
	JwksServiceConf      security.JwksServiceConf                 `json:"-"`                      // LoadJwksServiceConf
	BaseHttpClient       *http.Client                             `json:"-"`                      // for requests to external apis
	RawSQLFSMap          map[string]fs.FS                         `json:"-"`                      // Set before PrepareSQLDBClients
	SQLDBClients         map[string]sqldbs.Client                 `json:"-"`                      // PrepareSQLDBClients
	HTMLTemplateStore    map[string]map[string]*template.Template `json:"-"`                      // PrepareHTMLTemplateStore
	FWUpstream           *fwupstream.Hub                          `json:"-"`                      // PrepareFWUpstream (.fwupstream-web.json): FW clients + at-rest token store
	KVDBClients          map[string]kvdbs.Client                  `json:"-"`                      // PrepareKVDBClients
	MainKVDB             kvdbs.DB                                 `json:"-"`                      // From KVDBClients or set directly
	LocalStorages        map[string]*storages.LocalStorage        `json:"-"`                      // PrepareStorages
	StorageClients       map[string]storages.Client               `json:"-"`                      // PrepareStorageClients

	// internal
	serviceGraph serviceGraph  // services and the dependencies between them; drives start order, teardown order, and admission
	shutdownDone chan struct{} // closed once the shutdown walk has finished; what WaitServicesTerminated blocks on
	shutdownErr  error         // first error the walk recorded, including an abandoned service
	shutdownOnce sync.Once     // the walk publishes its result exactly once
}

// RegisterService registers a service with Core and returns its node — the
// identity other services name when they declare a dependency on it, and the
// record that carries whether this service may be used right now.
//
// This is the ONE registration path. The framework's own Prepare*Service
// methods call it exactly as an app's own service does, so the app-author route
// cannot quietly become second-class: if this API is awkward, the framework's
// five services feel it first.
//
// Dependencies are merged from two places, because neither alone is complete:
// what the SERVICE reports about what it was constructed with (ServiceDeps),
// and what the CALLER knows that the service itself cannot — most often that
// something inside its request path, a middleware it never sees, uses another
// service.
//
// A failed registration returns a nil node. Do not carry it onward: naming it
// as a dependency would report a second, misleading error against the wrong
// service and bury the real one.
func (c *Core) RegisterService(s svc.Service, extra ...ServiceDep) (*ServiceNode, error) {
	var deps []ServiceDep
	if sd, ok := s.(ServiceDeps); ok {
		deps = append(deps, sd.ServiceDeps()...) // copied — never append onto the service's own slice
	}
	deps = append(deps, extra...)

	node, err := c.serviceGraph.add(s, deps...)
	if err != nil {
		return nil, err
	}
	if len(node.deps) == 0 {
		log.Printf("[INFO] registered service: %s", node.name)
	} else {
		log.Printf("[INFO] registered service: %s (depends on %s)", node.name, node.dependencyList())
	}
	return node, nil
}

// StartServices seals the graph, then starts every service DEPENDENCIES FIRST —
// the levels walked from the deepest to level 0, so nothing starts before what
// it depends on is already RUNNING.
//
// A service is admitted the moment it starts. Admission is the app-level
// verdict on whether a service may be used, kept separate from the service's
// own phase because the two legitimately disagree during a degraded shutdown.
//
// Within a level the services are independent and could start concurrently.
// They do not yet: parallel start is only safe once Start is contractually
// honest (returns only when genuinely started) and cancellable, and once a
// failure can roll back what already started. Neither exists yet, and order —
// not parallelism — is what this walk had to fix.
func (c *Core) StartServices() error {
	// Registration closes here, and what could not be settled during it is
	// settled now — an optional dependency that was unresolved at the time is
	// either genuinely absent or was registered too late, and only now can the
	// two be told apart. Validate before starting anything: at boot, refusing
	// to run costs nothing, and a graph validated after services are already
	// running would be reporting on a process it can no longer prevent.
	if err := c.serviceGraph.seal(); err != nil {
		return err
	}
	levels := c.serviceGraph.levels()
	log.Printf("[INFO] starting all services (%d, in %d dependency levels)", len(c.serviceGraph.nodes), len(levels))
	c.shutdownDone = make(chan struct{})
	started := make([]*ServiceNode, 0, len(c.serviceGraph.nodes))
	for i := len(levels) - 1; i >= 0; i-- {
		for _, n := range levels[i] {
			if err := n.svc.Start(c.RootCtx); err != nil {
				c.rollbackStarted(started, n, err)
				return err
			}
			n.admitted.Store(true)
			// One collector per node, so a barrier can wait for THIS service
			// rather than merely counting how many have reported.
			go func(n *ServiceNode) { n.termSig <- <-n.svc.Terminated() }(n)
			started = append(started, n)
		}
	}
	return nil
}

// rollbackStarted tears down what a failed boot had already brought up, so a
// process that will not run also leaves nothing running.
//
// Unwound in reverse start order, which is dependents-before-dependencies for
// free: services were started deepest-level-first, so walking that list
// backwards is the same order the shutdown walk uses.
//
// The service that FAILED is not touched. It never reported started, so there
// is nothing to wait for — and by the Start contract a failed start leaves
// nothing behind. If an implementation breaks that contract, the leak is its
// own; the framework cannot tell the difference between "cleaned up after
// itself" and "half-built and lying".
//
// This is only this simple because starts are serial: every other service has
// either fully started or not begun, so there is no in-flight start to cancel.
// In-level start concurrency needs a cancellable Start before it is safe.
func (c *Core) rollbackStarted(started []*ServiceNode, failed *ServiceNode, cause error) {
	log.Printf("[ERROR] service %q failed to start: %v", failed.name, cause)
	if len(started) == 0 {
		log.Printf("[INFO] boot rollback: nothing had started yet")
		return
	}
	log.Printf("[INFO] boot rollback: tearing down %d already-started service(s)", len(started))
	for i := len(started) - 1; i >= 0; i-- {
		if err := c.terminateNode(started[i]); err != nil {
			log.Printf("[ERROR] boot rollback: %v", err)
		}
	}
	log.Printf("[INFO] boot rollback complete — no service left running")
}

// WaitServicesTerminated blocks until the shutdown walk has finished, and
// returns the first error it recorded — including a service that was abandoned,
// so an app can exit non-zero on a shutdown that discarded work.
func (c *Core) WaitServicesTerminated() error {
	<-c.shutdownDone
	return c.shutdownErr
}

// TerminateServices tears every service down DEPENDENTS FIRST — the levels
// walked from 0 upwards, with a completion barrier at each level.
//
// The barrier is the guarantee, not the order. Terminate is documented as
// asynchronous and can return while a service is still winding down (a web
// service's drain runs on its own budget), so initiating teardown in the right
// sequence proves nothing on its own. Each level waits for every one of its
// services to actually report Terminated before the next level — the services
// they depend on — is touched at all.
//
// Within a level the services are independent, so they tear down concurrently.
// That is a correctness property rather than a speed one: a serial walk costs
// the SUM of every teardown, and the whole walk has to fit inside the
// supervisor's kill window, so serializing services that never had reason to
// wait for each other is what turns a well-behaved shutdown into a SIGKILL.
func (c *Core) TerminateServices() {
	levels := c.serviceGraph.levels()
	log.Printf("[INFO] terminating all services (%d, in %d dependency levels)", len(c.serviceGraph.nodes), len(levels))
	var firstErr error
	for i, level := range levels {
		var wg sync.WaitGroup
		errs := make([]error, len(level))
		for j, n := range level {
			wg.Add(1)
			go func(j int, n *ServiceNode) {
				defer wg.Done()
				errs[j] = c.terminateNode(n)
			}(j, n)
		}
		wg.Wait() // ←── THE BARRIER: level i is fully terminated before level i+1 is touched
		for _, err := range errs {
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
		log.Printf("[INFO] terminate level %d/%d complete (%d services)", i, len(levels)-1, len(level))
	}
	c.shutdownOnce.Do(func() {
		c.shutdownErr = firstErr
		close(c.shutdownDone)
	})
}

// terminateNode revokes a service's admission, terminates it, and waits for it
// to report — or gives up and says so.
//
// Admission is revoked FIRST so that no new use can begin while the service is
// being torn down. It cannot help a caller already inside; that is the
// service's own obligation (svc.Service: methods stay safe after Terminate).
//
// A PER-SERVICE budget, as before: one slow teardown must not starve the
// services behind it (a shared budget once handed healthy services an
// already-expired ctx — reproduced 2026-07-27).
//
// If the report never comes, the node is ABANDONED: recorded, logged, and the
// walk continues. Holding the invariant absolutely would mean the supervisor
// eventually SIGKILLs the process, discarding the cleanup of every service that
// would have terminated perfectly. At shutdown the alternative to breaking the
// guarantee is not staying correct, it is losing everyone's cleanup — so we
// break it in one defined direction, loudly, and report a degraded shutdown.
func (c *Core) terminateNode(n *ServiceNode) error {
	n.admitted.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.TerminateTimeoutSecs)*time.Second)
	defer cancel()
	_ = n.svc.Terminate(ctx) // the error travels on the terminated channel, not here
	select {
	case err := <-n.termSig:
		return err
	case <-ctx.Done():
		n.abandoned = true
		log.Printf("[ERROR] service %q ABANDONED: no Terminated signal within %ds — whatever it depends on is now being torn down without the guarantee that it is finished with it", n.name, c.TerminateTimeoutSecs)
		return fmt.Errorf("service %q abandoned: terminate deadline exceeded", n.name)
	}
}
