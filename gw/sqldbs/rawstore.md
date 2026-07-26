# Raw SQL Store

`RawSQLStore` holds keyed SQL templates loaded from disk at boot. Handlers
look up a template by key and execute it through any DB.

```go
sql, _ := client.RawSQLStore("models").Get("user.select")
rows, _ := client.DB("main").Query(ctx, sql, args...)
```

## Layers

```
Client (= one DBMS connection profile)
  ├── stores map[name]*RawSQLStore     ← N sql sets, keyed by name
  │     ├── "models"
  │     ├── "migrations"
  │     └── ...
  └── dbs map[name]*DB                 ← M databases reachable via this profile
        ├── "main"
        ├── "warehouse"
        └── ...
```

Stores and DBs are independent dimensions on the Client — both keyed access,
no required pairing. A handler picks whichever store fits whichever DB at
call time.

## Loading — compile-time embed

SQL files are baked into the binary at build time via `//go:embed`:

```go
import "embed"

//go:embed **/*
var SQLFS embed.FS
```

The resulting `embed.FS` satisfies `fs.FS` and is what gets passed to the
framework helper below. No runtime file copying needed; no on-disk SQL
directory shipped alongside the binary.

## The framework helper

`Core.RawSQLFSMap map[string]fs.FS` lists every SQL set the app provides.
`PrepareSQLDBClients` then loads each FS into every registered Client under
its name:

```go
app.RawSQLFSMap = map[string]fs.FS{
    "models":     modelsFS,
    "migrations": migrationsFS,
}
app.PrepareSQLDBClients(mysql.PrepareClients, pgsql.PrepareClients)

// Result:
//   <mysql client>: stores["models"], stores["migrations"]
//   <pgsql client>: stores["models"], stores["migrations"]
```

Every Client ends up with the same set of store *names*. The *contents*
differ by dialect — see "File extensions" below.

## Why broadcast — the parity guarantee

The broadcast is load-bearing, not just convenient. It enforces the
contract that every Client carries the same store names with the same keys,
which is what lets handlers stay dialect-blind:

```go
// works on either client because the store-name parity is guaranteed
srcStore := srcClient.RawSQLStore("models")
dstStore := dstClient.RawSQLStore("models")

readSQL, _  := srcStore.Get("user.select")  // dialect-flavored for srcClient
writeSQL, _ := dstStore.Get("user.insert")  // dialect-flavored for dstClient

srcClient.DB("a").Query(ctx, readSQL, ...)
dstClient.DB("b").Exec(ctx, writeSQL, ...)
```

The handler doesn't branch on which driver is which. Cross-DBMS read/write
just works because the interfaces hide dialect details and the broadcast
hides the store-name divergence risk.

If a future setup truly needs per-Client divergent stores, bypass the helper
and call `client.LoadRawSQL(name, fs)` directly per Client after
`PrepareSQLDBClients` — but expect to lose parity (and the portability that
parity gives you) at any handler that touches both Clients.

## File extensions — dialect filtering at load

Each driver's `LoadRawSQL` walks the supplied `fs.FS` and picks only the
files it understands. The set of recognised extensions is **defined by
each driver impl**, not by the framework. The framework only standardises
the fallback (`.sql`) and the override rule.

| Ext           | Recognised by                | Role                              |
|---------------|------------------------------|-----------------------------------|
| `.sql`        | every driver                 | shared/portable SQL, fallback     |
| `.<dialect>`  | the driver that owns it      | dialect-specific, overrides `.sql`|

Examples from the current impls:

| Ext      | Recognised by  | Use for                                           |
|----------|----------------|---------------------------------------------------|
| `.mysql` | MySQL driver   | MySQL-specific (backticks, `ON DUPLICATE KEY`, …) |
| `.pgsql` | PostgreSQL driver | PostgreSQL-specific (`$N`, `RETURNING`, …)     |

A future driver introduces its own extension (e.g. `.mssql`, `.oracle`) —
the framework doesn't need to know.

**Override rule:** the dialect-specific file always loads. `.sql` only
loads if no dialect-specific file already exists for that key. So the same
key (e.g. `"user.select"`) resolves to dialect-appropriate text on each
Client without collision.

## Path → key

`fs.FS` paths (always forward-slash) become dot-joined keys, with the
file's extension stripped:

```
user/profile/select.sql        → "user.profile.select"
user/profile/select.<dialect>  → "user.profile.select"  (specific driver only)
```

## Author guideline

- Write `.sql` for queries that work on every dialect (most CRUD).
- Add a dialect-specific override (`.<dialect>` defined by the relevant
  driver) only when dialect-specific syntax is required for that key.
- Don't pre-filter what each Client loads — let the helper broadcast.
  Handlers pick by name at call time; the broadcast preserves the parity
  that cross-DBMS portability depends on.
