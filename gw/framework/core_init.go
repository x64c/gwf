package framework

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// BaseInit - 1st step for initialization
// 1. set AppRoot
// 2. load config/.core.json file
// 3. prepare base fields
// 4. Start ShutdownSignalListener
func (c *Core) BaseInit(appRoot string, rootCtx context.Context, rootCancel context.CancelFunc) error {
	c.AppRoot = appRoot
	// Load .env.json
	envFilePath := filepath.Join(appRoot, "config", ".core.json")
	//file, readErr := os.Open(envFilePath) // (*os.File, error)
	envBytes, err := os.ReadFile(envFilePath) // ([]byte, error)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(envBytes, c); err != nil {
		return err
	}
	if c.TerminateTimeoutSecs <= 0 {
		return fmt.Errorf("terminate_timeout_secs must be set (seconds > 0) in .core.json: got %d", c.TerminateTimeoutSecs)
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
	c.ActionLocks = &sync.Map{}
}
