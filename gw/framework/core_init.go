package framework

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// BaseInit is the runtime wiring after NewCore has read config/.core.json:
// it keeps the root context and its cancel, prepares the default features,
// and starts the shutdown signal listener. It reads no conf — identity and
// boot wiring were both decoded by NewCore, and the identity is unreachable
// from here on.
func (c *Core) BaseInit(rootCtx context.Context, rootCancel context.CancelFunc) error {
	if c.appName == "" || c.coordMode == 0 {
		return fmt.Errorf("Core has no identity — construct it with NewCore(appRoot)")
	}
	c.RootCtx = rootCtx
	c.RootCancel = rootCancel
	c.prepareDefaultFeatures()
	c.startShutdownSignalListener()
	return nil
}

func (c *Core) prepareDefaultFeatures() {
	c.VolatileKV = &sync.Map{}
	c.BaseHttpClient = &http.Client{}
}
