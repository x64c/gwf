package framework

import (
	"context"
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
	"github.com/x64c/gwf/gw/tg"
	"github.com/x64c/gwf/gw/throttle"
	"github.com/x64c/gwf/gw/uds"
	"github.com/x64c/gwf/gw/web"
	"github.com/x64c/gwf/gw/web/fwupstream"
	"github.com/x64c/gwf/gw/web/session"
)

type Core struct {
	AppName              string                                   `json:"app_name"`
	Listen               string                                   `json:"listen"`                 // HTTP Application Listen IP:PORT Address
	Host                 string                                   `json:"host"`                   // HTTP Host. Can be used to generate public url endpoints
	DebugOpts            DebugOpts                                `json:"debug_opts"`             // Debug Options
	TerminateTimeoutSecs int                                      `json:"terminate_timeout_secs"` // REQUIRED (> 0). TerminateServices budget: one shared deadline for the whole sequential shutdown, not per service. Must fit under the process supervisor's kill window (e.g. systemd < 90s, launchd < 20s, docker stop < 10s).
	AppRoot              string                                   `json:"-"`                      // Filled from compiled paths
	RootCtx              context.Context                          `json:"-"`                      // Global Context with RootCancel
	RootCancel           context.CancelFunc                       `json:"-"`                      // CancelFunc for RootCtx
	UDSService           *uds.Service                             `json:"-"`                      // PrepareUDSService
	JobSchedulerService  *jobsched.Service                        `json:"-"`                      // PrepareJobSchedulerService
	WebService           *web.Service                             `json:"-"`                      // PrepareWebService
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
	TypedGroupRegistry   map[string]tg.RegGrp                     `json:"-"`                      // Group Registry for typed groups
	KVDBClients          map[string]kvdbs.Client                  `json:"-"`                      // PrepareKVDBClients
	MainKVDB             kvdbs.DB                                 `json:"-"`                      // From KVDBClients or set directly
	LocalStorages        map[string]*storages.LocalStorage        `json:"-"`                      // PrepareStorages
	StorageClients       map[string]storages.Client               `json:"-"`                      // PrepareStorageClients

	// internal
	services         []svc.Service // registered services (via AddService), iterated by StartServices / TerminateServices in order
	terminationSigCh chan error    // buffered (cap = len(services)); per-service collector goroutines forward each `<-s.Terminated()` here; WaitServicesTerminated reads N values to know all services have fully terminated
}

func (c *Core) AddService(s svc.Service) {
	log.Printf("[INFO] adding service: %s", s.Name())
	c.services = append(c.services, s)
	log.Printf("[INFO] total services: %d", len(c.services))
}

func (c *Core) StartServices() error {
	log.Printf("[INFO] starting all services (%d)", len(c.services))
	c.terminationSigCh = make(chan error, len(c.services))
	for _, s := range c.services {
		err := s.Start(c.RootCtx)
		if err != nil {
			return err
		}
		go func(s svc.Service) {
			err := <-s.Terminated()
			c.terminationSigCh <- err
		}(s) // pass the loop var to the param. otherwise, they are captured inside goroutine lazily
	}
	return nil
}

func (c *Core) WaitServicesTerminated() error {
	for i := 0; i < len(c.services); i++ {
		if err := <-c.terminationSigCh; err != nil {
			return err
		}
	}
	return nil
}

func (c *Core) TerminateServices() {
	log.Printf("[INFO] terminating all services (%d)", len(c.services))
	// Operation-deadline ctx — separate from RootCtx, which is already cancelled
	// by the signal handler. Terminate's waitStopped needs a ctx that isn't
	// already done, otherwise its select fires the ctx.Done() branch before
	// the stopped channel closes, and `terminated` never gets sent.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.TerminateTimeoutSecs)*time.Second)
	defer cancel()
	for _, s := range c.services {
		_ = s.Terminate(ctx)
	}
}
