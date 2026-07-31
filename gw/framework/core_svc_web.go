package framework

import (
	"encoding/json/v2"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/web"
)

// PrepareWebService loads config/.web-server.json (web.ServerConf), validates
// it — its own invariants, then the one relation it has to the already-settled
// core conf — and registers the web service.
// Call this when all the required services are prepared.
func (c *Core) PrepareWebService(addr string, httpHandler http.Handler) error {
	confFilePath := filepath.Join(c.AppRoot, "config", ".web-server.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return err
	}
	var conf web.ServerConf
	if err = json.Unmarshal(confBytes, &conf); err != nil {
		return err
	}
	if err = conf.Validate(); err != nil {
		return err
	}
	if conf.DrainTimeoutSecs >= c.TerminateTimeoutSecs {
		return fmt.Errorf("drain_timeout_secs (%d) must be less than terminate_timeout_secs (%d): a full drain could never finish inside the shutdown budget", conf.DrainTimeoutSecs, c.TerminateTimeoutSecs)
	}
	// Client-address resolution belongs to the web server — logging, audit and
	// rate limiting all ask for it — so it is prepared here. Trusting nothing is
	// a valid answer, and also the one that makes every request behind a proxy
	// look like the proxy, so it is said out loud.
	c.ClientIPResolver = conf.ClientIPResolver()
	if c.ClientIPResolver.TrustedCount() == 0 {
		log.Printf("[WARN][WebService] no trusted_proxy_cidrs: client address = connection peer, forwarding headers ignored.")
	}

	c.WebService = web.NewService(addr, httpHandler, conf)
	// The web subsystem ships middleware that may reach for these, and it
	// cannot know which of it the app actually mounted — the route tree arrives
	// as an opaque handler. So it declares the POSSIBILITY. Over-declaring
	// costs one ordering constraint; under-declaring is the defect this graph
	// exists to prevent, so the conservative direction is the correct one.
	//
	// Named from the registered instances rather than from string literals: a
	// service registered under a name of the app's choosing still matches, and
	// one the app never prepared contributes no dependency at all.
	deps := make([]ServiceDep, 0, 2)
	if c.ThrottleService != nil {
		deps = append(deps, MayUse(c.ThrottleService.Name()))
	}
	if c.SessionService != nil {
		deps = append(deps, MayUse(c.SessionService.Name()))
	}
	if _, err = c.RegisterService(c.WebService, deps...); err != nil {
		return err
	}
	return nil
}
