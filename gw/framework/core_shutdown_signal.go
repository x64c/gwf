package framework

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var once sync.Once

// startShutdownSignalListener wires SIGINT/SIGTERM to a graceful in-process
// shutdown.
//
// Shutdown is a two-step cooperative handoff:
//
//  1. c.RootCancel() — the in-process equivalent of SIGTERM. Closes RootCtx
//     and cascades to every context derived from it (each service's s.Ctx,
//     and whatever the app derived from RootCtx). This is a *broadcast*, not
//     enforcement: goroutines watching <-ctx.Done() react and clean up their
//     own resources (close listeners, stop tickers, return from run loops).
//     Go has no SIGKILL equivalent for goroutines — cooperative cancellation
//     via context is the only way to wind them down.
//
//     In-flight HTTP request contexts are deliberately NOT in that cascade:
//     they carry RootCtx's values but not its cancellation, because this step
//     is what OPENS the graceful drain — killing the requests it exists to
//     protect would defeat it. They are cancelled when the drain window
//     closes, which is the moment grace has actually run out (web.Service.run,
//     drain_timeout_secs).
//
//  2. c.TerminateServices() — sequential per-service Terminate calls. Each
//     service's `Terminated` channel only fires from an explicit Terminate;
//     without this step, c.WaitServicesTerminated() would hang forever waiting
//     for signals that never come. By the time we get here, most run
//     goroutines have already exited from step 1's cascade — Terminate's
//     waitStopped sees `stopped` already closed and returns instantly.
//
// As a result of step 1 happening in parallel with step 2, subsystem-level
// logs from `run` goroutines can appear *before* the lifecycle log sequence
// (Terminating/Stopping/...) for that same service. That interleaving is
// by design: subsystem logs come from the `run` goroutine reacting to
// ctx.Done(), lifecycle logs come from the Terminate caller.
func (c *Core) startShutdownSignalListener() {
	once.Do(func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigs
			log.Printf("[INFO] got signal [%s]. shutting down app [%s] ...", sig, c.AppName)
			c.RootCancel()        // step 1: broadcast cancel — see doc above
			c.TerminateServices() // step 2: explicit per-service Terminate — see doc above
		}()
	})
	log.Printf("[INFO][CORE] shutdown signal listener started")
}
