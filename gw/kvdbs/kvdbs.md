# kvdbs

The key-value store interface: `Client` (one server), `DB` (one database on it),
and the contract every implementation owes.

Written so that a consumer can predict an implementation's behavior without
reading it, and check that prediction. Where the interface is silent, every
driver fills the gap differently and no consumer can be right — so the parts
below that look like pedantry are the parts that had bugs.

## Naming

Methods carry the type they operate on, because the bare verbs do not say
enough: `Len` of what, `Remove` of what?

| group | prefix | why |
|---|---|---|
| key ops | none | `Exists`, `TTL`, `Delete`, `Expire`, `Persist`, `Type`, `ScanKeys`, `Clock` apply to a key whatever it holds |
| value | `…Value` | `SetValue`, `SetValuePersistent`, `GetValue` — verb first, the type as suffix |
| list | `List…` | `ListPush`, `ListPop`, `ListLen`, `ListRange`, `ListRemove`, `ListTrim` |
| hash | `Hash…` | `HashSetField(s)`, `HashGetField(s)`, `HashGetAll`, `HashRemoveFields`, … |

`WithKeyTTL` says whose lifetime is being set. A hash is **one key**, and the
lifetime belongs to the key — all fields share it and die together. There is no
per-field lifetime here, so `HashSetFieldsWithKeyTTL` writes fields **and sets
the whole hash's lifetime**, while `HashSetFields` leaves that lifetime alone.
That difference — *does this write change the key's lifetime?* — is the only
thing separating the two families.

## Time

Questions are asked in the caller's terms; an implementation answers on its own
clock, which advances in discrete ticks. `Clock()` publishes it (see
`g/clock`). Given a precision P, a caller may rely on:

- **the stored lifetime covers the one asked for** — it can run over by less
  than P, and does not fall short;
- **a duration read back is accurate to within P**.

A lifetime is placed by reading *both its ends* onto that clock, and the key
then holds its final mark until the clock leaves it. That is what makes
coverage come out right; handing a backend a pre-computed duration instead lets
it anchor at its own reading and land a tick short.

**Zero means one thing everywhere, whatever the verb: a lifetime already over,
so the key goes.** Nothing is stored for no time, and nothing outlives a
lifetime of none. Storing without a lifetime is a separate operation with its
own name (`SetValuePersistent`), as is dropping one (`Persist`) — neither is
smuggled in as a duration that means its opposite. Lifetimes are never
negative; an implementation rejects one rather than inventing a meaning.

## Values

What a store holds is a string, which is why the readers return one. Writes take
`any` as a **convenience**: an implementation accepts whichever types it can
encode, and how it encodes them is its own business — this interface neither
specifies that nor can see it.

**One guarantee, the identity one: a string written is the same string read
back, byte for byte.** Nothing is promised about the stored form of any other
type, nor about whether a given implementation accepts it at all.

So when the representation matters — a layout something else will parse, a value
another implementation may later serve — produce it yourself and write a string.
A time is the clearest case: RFC 3339, epoch seconds and epoch nanoseconds all
encode an instant, and a stored one is meaningless unless every later reader
agrees which was used.

## Conditional writes

The hash writes come in two families:

- `Hash…` — **upsert**: writes the fields, creating the key when absent.
- `Hash…IfExists` — **update-only**: writes when the key already exists, never
  creates.

Pick by whether creation is correct. Where a key's existence is itself
meaningful — absence means gone, or its lifetime is owned elsewhere — the upsert
family **recreates** it, and only the `IfExists` family states what such a
caller means. That distinction is not decoration: an unconditional write onto an
expired session row is how a dead session comes back to life carrying no
identity, and how encrypted credentials end up in a key with no lifetime at all.

For an implementation, the existence test and the write **must be one
indivisible operation**. Performed separately they offer nothing over the caller
running `Exists` then writing — which is the race they exist to remove. A
backend that cannot do it indivisibly returns `ErrNotSupported` rather than
emulating it.

## Reads report absence loosely

`HashGetFields` and `HashGetAll` return an **empty map for both** a missing key
and a key holding none of the requested fields. Use `Exists` to tell those
apart. Emptiness is not absence, and treating it as absence is how a caller ends
up believing a row is gone when it is merely unfamiliar.

## Implementations

`kvdbs/redis` is the reference implementation — 1ms precision, see its
`TimePrecision`.
