package framework

import (
	"encoding/json/v2"
	"fmt"
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
	c.WebService = web.NewService(addr, httpHandler, conf)
	c.AddService(c.WebService)
	return nil
}
