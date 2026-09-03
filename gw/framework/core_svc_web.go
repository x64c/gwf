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

// LoadWebServerConf reads and validates config/.web-server.json into
// c.WebServerConf. Separate from PrepareWebService because the conf is needed
// while the route tree is being built — handler wrappers read the app's
// canonical Host from it — and that happens before the service is constructed.
func (c *Core) LoadWebServerConf() error {
	confFilePath := filepath.Join(c.AppRoot, "config", ".web-server.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(confBytes, &c.WebServerConf); err != nil {
		return err
	}
	return c.WebServerConf.Validate()
}

// PrepareWebService validates the given conf — its own invariants, then the one
// relation it has to the already-settled core conf — and registers the web
// service. The conf is taken as an argument rather than read from the Core so
// the value the server runs on is the value validated here.
// Call this when all the required services are prepared.
func (c *Core) PrepareWebService(conf web.ServerConf, httpHandler http.Handler) (*web.Service, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	if conf.DrainTimeoutSecs >= c.SvcTerminateTimeoutSecs {
		return nil, fmt.Errorf("drain_timeout_secs (%d) must be less than svc_terminate_timeout_secs (%d): a full drain could never finish inside the shutdown budget", conf.DrainTimeoutSecs, c.SvcTerminateTimeoutSecs)
	}
	// Client-address resolution belongs to the web server — logging, audit and
	// rate limiting all ask for it — so it is prepared here. Trusting nothing is
	// a valid answer, and also the one that makes every request behind a proxy
	// look like the proxy, so it is said out loud.
	c.ClientIPResolver = conf.ClientIPResolver()
	if c.ClientIPResolver.TrustedCount() == 0 {
		log.Printf("[WARN][WebService] no trusted_proxy_cidrs: client address = connection peer, forwarding headers ignored.")
	}

	c.WebServerConf = conf
	c.webService = web.NewService(httpHandler, conf)
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
	if c.throttleNode != nil {
		deps = append(deps, MayUse(c.throttleNode.Name()))
	}
	if c.sessionService != nil {
		deps = append(deps, MayUse(c.sessionService.Name()))
	}
	node, err := c.RegisterService(c.webService, deps...)
	if err != nil {
		return nil, err
	}
	c.webNode = node
	return c.webService, nil
}
