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
// Returns the first error encountered. A FAILED BOOT NOW RELEASES EVERYTHING:
// StartServices tears down whatever it had already started, and ResourceCleanUp
// then closes the infrastructure clients — services first, clients second, for
// the same reason the shutdown walk is ordered at all (releasing a DB while a
// service that uses it is still tearing down would break the teardown).
//
// That reverses the earlier behavior, where a failed boot deliberately left
// partially-prepared infrastructure to the OS. Process exit does reclaim file
// descriptors, but nothing was released *gracefully*: DB sessions lingered
// server-side until timeout, nothing flushed, and anything a service acquired
// stayed acquired — and in a host that owns its own exit, nothing was reclaimed
// at all. A process that will not run should leave nothing running and nothing
// open.
//
// A shutdown that abandoned work (a service missed its terminate deadline)
// returns that error too, and apps should exit non-zero on it (log.Fatalf):
// the supervisor then records `failed`, not `inactive` — DELIBERATE (decided
// 2026-07-27): a stop that discarded work must not look identical to a clean
// one.
func (c *Core) Run() error {
	log.Printf("[INFO][%s] app.Run()", c.appName)
	// A service panic re-raised by StartServices or WaitServicesTerminated
	// passes through here. This frame releases what IT owns — the
	// infrastructure clients — then lets the panic continue to main, where a
	// defer/recover is the application's choice.
	defer func() {
		if rcv := recover(); rcv != nil {
			c.ResourceCleanUp()
			panic(rcv)
		}
	}()
	if err := c.StartServices(); err != nil {
		c.ResourceCleanUp() // services were already unwound by StartServices
		return err
	}
	err := c.WaitServicesTerminated()
	c.ResourceCleanUp()
	return err
}
