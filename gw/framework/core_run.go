package framework

import "log"

// Run is the canonical "start the app and block until shutdown" entry point.
//
// It composes the three steps each app's main would otherwise repeat:
//
//	c.StartServices()           // start all registered services
//	c.WaitServicesTerminated()  // block until all have terminated (via signal handler → Terminate)
//	c.ResourceCleanUp()         // close infrastructure clients (KVDB, SQL DB, ...)
//
// Returns the first error encountered. On StartServices failure, ResourceCleanUp
// is intentionally NOT called — preserving the current behaviour where a failed
// boot leaves partially-prepared infrastructure to the OS. Closing the leak is
// a separate decision (likely paired with a registry-driven cleanup refactor).
func (c *Core) Run() error {
	log.Printf("[INFO][%s] app.Run()", c.AppName)
	if err := c.StartServices(); err != nil {
		return err
	}
	err := c.WaitServicesTerminated()
	c.ResourceCleanUp()
	return err
}
