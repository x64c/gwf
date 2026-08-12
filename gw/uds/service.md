# UDS Service

Operator command channel over a Unix domain socket. Apps register
command handlers at boot (`CommandStore.AddCommand("name", handler)`);
when running, an operator connects to the socket (e.g. `nc -U
/tmp/<app>.sock`), types a command name + args, and the handler runs.

This service consists of 2 things:

1. **Command registry** — the embedded `*CommandStore`, holding all
   command names → handler functions registered at boot. Always available
   from the moment the Service is constructed; lifecycle-independent.
2. **Background UDS service** — the socket listener + accept loop + the
   per-client connection-handling goroutines. This is what the lifecycle
   controls.

Unlike `throttle` or `session`, the registry on its own does no work —
without an active listener, nothing can invoke the registered commands.
So while #1 *exists* lifecycle-independently, it's only *useful* when #2
is running.

The framework lifecycle (`Start`/`Stop`/`Terminate`) controls **only #2**.
#1 persists across `Stop`/`Start` cycles — the same registered commands
are immediately invokable after a restart.

## Prepare

`Core.PrepareUDSService(commands)`:
- Reads `.uds.json` for the socket path config.
- Constructs the `CommandStore`, registers the supplied command set.
- Constructs the Service (`NewService`) embedding the store.
- Registers the Service with `Core.RegisterService`, which places it in the
  composition graph the start and terminate walks follow.

After `Prepare`, the `CommandStore` is fully populated and queryable
(`GetHandler(name)`, `PrintHelp(w)`), but nothing serves it — `Start`
provides the access channel.

## Start()

Bootstrap (returns errors directly if either step fails):
- Removes any leftover socket file at `Conf.SocketPath`.
- `net.Listen("unix", path)` to create the socket.
- `os.Chmod(path, 0660)` to restrict to owner + group.

Then spawns the run goroutine:
- A cleanup sub-goroutine waits on `<-s.Ctx.Done()` and closes the
  listener + removes the socket file when the service is told to stop.
- The main loop calls `s.listener.Accept()` in a loop. Each accepted
  connection is dispatched to `handleConn` in its own goroutine — that
  goroutine reads a line, looks up the command, runs the handler,
  writes back, repeats until the client disconnects or `q`/`quit`.

Each `handleConn` also spawns a tiny watcher goroutine that closes the
client connection on `<-s.Ctx.Done()` — so in-flight connections get
torn down when the service is asked to stop.

## Stop()

Cancels the per-cycle ctx. Effects:
- The listener-cleanup sub-goroutine fires: `s.listener.Close()` and
  `os.Remove(s.Conf.SocketPath)`.
- The main accept loop sees `net.ErrClosed` on its next `Accept()` and
  exits.
- Each in-flight `handleConn` goroutine's watcher fires: `c.Close()` on
  the per-client `net.Conn`, forcing any in-progress command read or
  write to error out.
- `s.wg`-equivalent waiting? No — handleConn goroutines aren't
  WaitGroup-tracked here; they're force-closed by ctx-driven conn close.

After Stop:
- The socket file is **gone from disk**. No operator can connect.
- The `CommandStore` is still populated — handlers persist in memory.
- `Start()` again rebuilds the listener from `Conf.SocketPath`,
  recreates the socket file, and resumes accepting. The registered
  commands are immediately available.

This is a meaningful "offline" stop, unlike throttle/session: while
stopped, the operator-facing surface (the socket) is genuinely
unreachable.

## Terminate()

Same flow as `Stop()` (cancel + waitStopped), plus:
- Terminal: state stays `TERMINATING`. The service cannot be `Start`ed
  again in this process lifetime.
- Fires the framework's `Terminated` channel so
  `Core.WaitServicesTerminated` can count this service as done.

The `CommandStore` and the (now-removed) socket file are reclaimed at
process exit — no UDS-specific cleanup beyond what `Stop` already does.

## Operator note

"Stop UDS" = **the service goes offline**. The socket file is removed
from `/tmp/`, in-flight client connections are force-closed, and no new
connections can be accepted until `Start()` rebuilds the listener.

Registered commands survive the stop — when `Start()` resumes, the
operator can reconnect and the same command set is immediately
available, with no need to re-register handlers.
