# locking

Named locks: mutual exclusion over names. A caller asks a manager for a
name, runs a function while the name is held, and the manager gives the
name back when the function returns. A name held anywhere is refused at
once. Whether to ask again, and when, is the caller's decision, made where
the caller's situation is known.

## Terms

- **LOCK NAME** — what exclusion is about: "foo". A lock is its name; there
  is no lock apart from it.
- **MANAGER** — what callers speak to. It takes asks, runs the caller's
  function while the name is held, releases at return, and answers what its
  instance holds. Callers never touch a ledger. Every manager has a name of
  its own, given when it is built — what it serves, such as "action" or
  "session" — so an app running several tells them apart, and a manager over
  a shared store can be given a region of its own from it.
- **INTERNAL LEDGER** — the record built into every manager: the names this
  instance holds right now. The manager consults it on every act, and an
  ask for a name found there is refused without going further.
- **EXTERNAL LEDGER** — the record several instances share so that their
  internal ledgers never contradict each other: the source of truth of
  grants across a fleet. It takes asks one at a time, and a grant that
  would duplicate a standing one is refused in the same act that reads it.
  Only a manager built for one has one.
- **GRANT** — a ledger's yes to one ask for "foo". It stands from the ask
  until the manager releases it, and while it stands "foo" is held.
- **PROOF** — the value a manager writes into the external ledger with a
  grant and presents again to release it, so that a release can never
  remove another instance's grant. Only the external ledger needs one; the
  internal ledger has no other writer.

The two ledgers are not two kinds of one thing. The internal ledger is part
of every manager. The external ledger is a separate, shared record that a
manager may additionally keep in step with.

## The two managers

Both satisfy `Manager`. Which one an app builds is the app's choice, made
once.

- **`ManagerLocalLedgerOnly`** — for an app that runs as one instance. Its
  internal ledger is the whole truth: nothing outside the process can hold
  a name, so no proof and no external step exist.
- **`ManagerKVDBLedger`** — for an app that runs as several instances. It
  keeps its internal ledger as its own slice of an external ledger held in
  a KVDB: one string row per standing grant at `prefix + name`, holding the
  proof. Every instance built over the same KVDB and prefix keeps the same
  external ledger.

```
        instance A                          instance B
 ┌────────────────────────────┐    ┌────────────────────────────┐
 │ ManagerKVDBLedger          │    │ ManagerKVDBLedger          │
 │ ┌────────────────────────┐ │    │ ┌────────────────────────┐ │
 │ │ INTERNAL LEDGER        │ │    │ │ INTERNAL LEDGER        │ │
 │ │ names A holds now      │ │    │ │ names B holds now      │ │
 │ └────────────────────────┘ │    │ └────────────────────────┘ │
 └─────────────┬──────────────┘    └─────────────┬──────────────┘
      take · retire · admin              take · retire · admin
               └─────────────────┬───────────────┘
                ┌────────────────▼────────────────┐
                │ EXTERNAL LEDGER: rows in a KVDB │
                │   "foo" → PROOF of the GRANT    │
                └─────────────────────────────────┘

 ┌────────────────────────────┐
 │ ManagerLocalLedgerOnly     │
 │ ┌────────────────────────┐ │
 │ │ INTERNAL LEDGER        │ │   the whole truth; nothing below it
 │ │ names held now         │ │
 │ └────────────────────────┘ │
 └────────────────────────────┘
```

## `Manager`

```go
type Manager interface {
	Name() string
	AcquireDoRelease(ctx context.Context, name string, fn func(ctx context.Context) error) error
	AcquireDoReleaseAll(ctx context.Context, names []string, fn func(ctx context.Context) error) error
	Names() []string
}
```

**`Name`** is the manager's own name, given at construction.

**`AcquireDoRelease`** asks for `name`, runs `fn` while the name is held,
and releases the name when `fn` returns, by any path, a panic included. A
name held anywhere, by this instance or another, refuses the ask at once
with `errs.ActionLocked`, and `fn` does not run. `fn`'s own error is
returned as is. The ctx given to `fn` derives from the caller's and ends
when `fn` returns. There is no clock and no line in the manager: a caller
that wants the name later asks again, on its own terms, with its own
interval and its own bound.

**`AcquireDoReleaseAll`** asks for every name or none, in the order given,
and releases them in reverse. One refusal releases what was already taken
and returns `errs.ActionLocked` naming the refused name.

**`Names`** reports, as a snapshot, the names this instance holds: the
internal ledger.

```go
func NewManagerLocalLedgerOnly(name string) (*ManagerLocalLedgerOnly, error)
func NewManagerKVDBLedger(name string, kvdb kvdbs.DB, prefix string) (*ManagerKVDBLedger, error)
```

Nothing may be empty. `prefix` is the external ledger's region of the
keyspace; the caller derives it, so a manager never learns more than it
needs, and two managers of one app never share a row.

## The external ledger's own surface

Only `ManagerKVDBLedger` has an external ledger, so only it exposes one:

```go
func (m *ManagerKVDBLedger) ExternalLedger() *KVDBLedger

func (l *KVDBLedger) Standing(ctx context.Context, name string) (proof string, ok bool, err error)
func (l *KVDBLedger) CleanupNames(ctx context.Context, names []string) (int, error)
func (l *KVDBLedger) Cleanup(ctx context.Context) (int, error)
```

This surface is for inspection and administration, never for the regular
line: taking and releasing stay the manager's, and `KVDBLedger` exports no
way to do either. Code that needs it holds a `Manager` and asserts the
concrete manager type once, at boot, on the admin path.

**`Standing`** reports the proof standing on `name`, if any, as the
external ledger has it now.

**`CleanupNames`** removes the given names from the external ledger
regardless of who holds them, and reports how many stood. It is for an
instance that died holding names and left them standing, never the regular
release. **`Cleanup`** removes every row in the external ledger's region
the same way, for a fresh start. Both act on the external ledger only,
never on any internal ledger: an instance still holding one of those names
keeps it in its internal ledger, fails its own release at the external
ledger, and logs that. Run them when nothing is meant to hold.

## One ask, start to end

`ManagerLocalLedgerOnly`:

1. The caller asks for "foo". The internal ledger has it: refused. It does
   not: "foo" is recorded, and `fn` runs.
2. `fn` returns. "foo" is removed from the internal ledger.

`ManagerKVDBLedger`:

1. The caller asks for "foo". The internal ledger has it: refused, and the
   external ledger is not asked. It does not: "foo" is reserved in the
   internal ledger, so a second local ask in the meantime is refused
   without a second trip.
2. The manager asks the external ledger with a fresh proof: TAKE. No: the
   reservation is dropped and the ask is refused. Yes: the grant stands
   under this instance's proof, and `fn` runs.
3. `fn` returns. The manager releases at the external ledger first,
   presenting its proof: RETIRE. Then it removes "foo" from the internal
   ledger. The external ledger goes first so that a local ask arriving in
   between is refused by the internal ledger, not by this instance's own
   dying grant.
4. The external ledger fails to answer at release. `fn` is done, so the
   internal ledger is cleared regardless, the row is left standing, and
   the manager logs it. `CleanupNames` on the external ledger is the
   remedy.

## What stands after a crash

An instance that dies while holding names takes its internal ledger with
it. Under `ManagerLocalLedgerOnly` nothing remains. Under
`ManagerKVDBLedger` its rows stand in the external ledger, with no
lifetime, until `CleanupNames` or `Cleanup` on the external ledger removes
them. Until then every instance is refused those names.
