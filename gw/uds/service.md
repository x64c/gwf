# UDS Service

Operator command channel over a Unix domain socket. Apps register
command handlers at boot (`CommandStore.AddCommand("name", handler)`);
when running, an operator connects to the socket (e.g. `nc -U
/run/<app>/uds.sock`), types a command name + args, and the handler runs.

**Privilege posture: the command surface is single-tier, and the tier is
ADMIN.** There is no per-command capability distinction — anyone who can
connect to the socket can run every registered command, including any
that mutate services or reveal secrets. The socket's file permissions
(`socket_mode` + the directory) are the entire access control: grant
them as you would grant full administrative access to the application.

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
- Reads `.uds.json` — socket path, socket mode (octal string), per-line read
  cap. Every field is required-explicit; a missing or incoherent value is a
  named boot refusal (`Conf.Validate`).
- Constructs the `CommandStore`, registers the supplied command set.
- Constructs the Service (`NewService`) embedding the store.
- Registers the Service with `Core.RegisterService`, which places it in the
  composition graph the start and terminate walks follow.

After `Prepare`, the `CommandStore` is fully populated and queryable
(`GetHandler(name)`, `PrintHelp(w)`), but nothing serves it — `Start`
provides the access channel.

## Start()

Bootstrap (returns errors directly if any step fails):
- **A file already at `Conf.SocketPath` is a START FAILURE, diagnosed by
  name and never removed** — cleanup is proper on every stop path, so a
  file here means something is wrong, and deleting it would destroy the
  evidence or a running instance's socket. Three refusals: a live process
  answers the path (another instance, or an impostor — investigate); a
  dead socket file (unclean shutdown — remove it and start again); a
  non-socket file (foreign — investigate how it got there).
- `net.Listen("unix", path)` creates the socket **born at the conf-stated
  `socket_mode`** (umask around the bind): the file never exists at any
  other mode, under any process umask. A chmod then guarantees the exact
  final mode.

Then spawns the run goroutine:
- A cleanup sub-goroutine waits on `<-s.Ctx.Done()` and closes the
  listener + removes the socket file when the service is told to stop.
- The main loop calls `s.listener.Accept()` in a loop. Each accepted
  connection is dispatched to `handleConn` in its own goroutine — that
  goroutine reads a line, looks up the command, runs the handler,
  writes back, repeats until the client disconnects or `q`/`quit`.
- **Every connection and every command line is logged with the peer's
  kernel-reported identity** (`SO_PEERCRED`: uid, resolved to a username
  when the system knows one, gid, pid) — attribution the client cannot
  fake. It is audit, not authorization: an unreadable credential logs as
  `peer=?` and the connection proceeds on the socket's file permissions
  as ever.

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

## Socket placement

The framework sets no default path — `socket_path` is the deployment's
choice — but the choice carries the socket's whole security story, so the
directory must be chosen deliberately:

- **Access to the socket is two checks**: search (`x`) permission on every
  directory in the path, then write permission on the socket file itself
  (`socket_mode`). A private directory is therefore a gate in front of the
  mode bits.
- **Deletion rights live on the DIRECTORY, not the file.** In a directory
  others can write, others can unlink the live socket (unless the sticky
  bit restricts it) — the service keeps its now-unreachable listener and
  never notices.
- **Creation rights live on the directory too.** In a world-writable
  directory, anyone can pre-create a file — or bind their own socket — at
  the well-known path while the service is down: an impostor prompt for
  operators, and a boot obstruction for the service.

So: use a directory writable only by the app's own user, ideally also
traversable only by the operator group (e.g. mode 0750). A per-app
runtime directory such as `/run/<app>/` is the conventional home; under
systemd, `RuntimeDirectory=` creates it fresh (correctly owned) at every
service start and removes it at stop. A world-writable directory like
`/tmp` satisfies none of this and is not an appropriate home for the
operator surface.

## Operator note

"Stop UDS" = **the service goes offline**. The socket file is removed
from disk, in-flight client connections are force-closed, and no new
connections can be accepted until `Start()` rebuilds the listener.

Registered commands survive the stop — when `Start()` resumes, the
operator can reconnect and the same command set is immediately
available, with no need to re-register handlers.
