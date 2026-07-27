package framework

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/x64c/gwf/gw/web"
)

// PrepareWebService loads config/.web-server.json (web.ServerConf), validates
// it against the already-settled core conf, and registers the web service.
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
	if conf.DrainTimeoutSecs <= 0 {
		return fmt.Errorf("drain_timeout_secs must be set (seconds > 0) in .web-server.json: got %d", conf.DrainTimeoutSecs)
	}
	if conf.DrainTimeoutSecs >= c.TerminateTimeoutSecs {
		return fmt.Errorf("drain_timeout_secs (%d) must be less than terminate_timeout_secs (%d): a full drain could never finish inside the shutdown budget", conf.DrainTimeoutSecs, c.TerminateTimeoutSecs)
	}
	c.WebService = web.NewService(addr, httpHandler, time.Duration(conf.DrainTimeoutSecs)*time.Second)
	c.AddService(c.WebService)
	return nil
}
