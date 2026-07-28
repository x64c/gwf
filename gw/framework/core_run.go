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
//
// A shutdown that abandoned work (a service missed its terminate deadline)
// returns that error too, and apps should exit non-zero on it (log.Fatalf):
// the supervisor then records `failed`, not `inactive` — DELIBERATE (decided
// 2026-07-27): a stop that discarded work must not look identical to a clean
// one.
func (c *Core) Run() error {
	log.Printf("[INFO][%s] app.Run()", c.AppName)
	if err := c.StartServices(); err != nil {
		return err
	}
	err := c.WaitServicesTerminated()
	c.ResourceCleanUp()
	return err
}
