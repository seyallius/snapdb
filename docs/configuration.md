# Configuration reference

`snapdb` has exactly one entry point: `Run`. Everything it needs is either one of four required arguments or one of several optional `With...` functions.

```go
func Run(
    m RunM,
    driver DatabaseDriver,
    schemaInit SchemaInitializer,
    dataInit DataInitializer,
    engineInit EngineInitializer,
    opts ...Option,
)
```

## The four required arguments

There's no meaningful default for "which database" or "how do I build your schema" — so these aren't options, they're plain arguments, and the compiler checks them for you.

| Argument     | Type                                     | Purpose                                                                                                                                                            |
|--------------|------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `driver`     | `DatabaseDriver`                         | The backend instance — `mysql.New()`, `postgres.New()`, or `sqlite.New()`.                                                                                         |
| `schemaInit` | `func(env *Environment) error`           | Creates your tables. Called once, slow path only.                                                                                                                  |
| `dataInit`   | `func(env *Environment) error`           | Seeds your base data. Called once on the slow path, and again on every reset for drivers that use the truncate-and-reseed strategy (currently just SQLite).        |
| `engineInit` | `func(env *Environment) (Engine, error)` | Builds the `Engine` that `snapdb` will use for cache clearing and connectivity checks. See [The Engine interface](engine-and-environment.md#the-engine-interface). |

> **Coming from an older version?** `WithDriver`, `WithSchemaInitializer`, `WithDataInitializer`, and `WithEngineInitializer` still exist and still work — they're just deprecated in favor of the positional form above. If you pass both, the last one set wins, same as any other option.

## Optional options

| Option                     | Purpose                                                                                                  |
|----------------------------|----------------------------------------------------------------------------------------------------------|
| `WithDatabase(cfg)`        | Database name, credentials, image, startup timeout.                                                      |
| `WithCacheInvalidator(fn)` | Flush ORM-level caches before every reset.                                                               |
| `WithSeeders(fns...)`      | Append per-test seeders, run after every reset. Repeatable — call it more than once and they accumulate. |
| `WithPristineDumpPath(p)`  | Override where the pristine snapshot/dump file lives.                                                    |
| `WithTestdataDir(p)`       | Override the testdata directory.                                                                         |
| `WithSQLitePath(p)`        | Override the SQLite database file path (SQLite only).                                                    |
| `WithProjectRoot(p)`       | Skip automatic `go.mod` lookup and use this path instead.                                                |
| `WithGeneratePristine(b)`  | Force the slow path on the next setup, regenerating the snapshot even if one already exists.             |
| `WithLogger(l)`            | Swap out the default tree-style logger. Pass `nil` to silence output entirely.                           |

## Defaults

If you don't call `WithDatabase`, each Docker-backed driver picks something reasonable:

| Driver   | Database | Username   | Password   | Image                |
|----------|----------|------------|------------|----------------------|
| MySQL    | `testdb` | `root`     | `testpass` | `mysql:lts`          |
| Postgres | `testdb` | `postgres` | `testpass` | `postgres:16-alpine` |
| SQLite   | —        | —          | —          | —                    |

SQLite ignores `WithDatabase` entirely — there's no server to authenticate against.

A few path defaults, in case you never touch them:

- `WithTestdataDir` → `<projectRoot>/testdata`
- `WithPristineDumpPath` → `<testdataDir>/<driver>-pristine.sql`
- `WithSQLitePath` → `<testdataDir>/snapdb.sqlite`

`projectRoot` itself is found automatically by walking up from the current directory until a `go.mod` shows up. You only need `WithProjectRoot` if your test binary runs somewhere unusual (e.g. a build step that copies test files out of the module tree).

## What happens during setup, step by step

`Run` does this once, the moment your test binary starts:

1. Find the project root (`go.mod` lookup, or `WithProjectRoot`).
2. Boot the backend — a Docker container for MySQL/Postgres, or just opening a file path for SQLite.
3. Decide fast path or slow path:
    - **Fast path** (the normal case): a pristine snapshot already exists on disk → restore it. Takes about a second for MySQL/Postgres, microseconds for SQLite.
    - **Slow path** (first run ever, or `WithGeneratePristine(true)`): run `schemaInit`, then `dataInit`, then save a fresh snapshot so the *next* run can take the fast path.
4. Call `engineInit` with the live connection string.
5. Ping the engine to make sure it's actually reachable before handing control to your tests.

## What happens on every `Reset`

1. Grab an internal lock (parallel tests sharing one container serialize here, so they don't reset on top of each other).
2. Call your `CacheInvalidator`, if you supplied one.
3. Call `Engine.ClearCache()`.
4. Restore the pristine state — either by re-piping the snapshot back into the database (MySQL/Postgres, and SQLite via a file copy), or by truncating tables and re-running `dataInit` (SQLite's fallback strategy, used automatically where binary snapshot restores aren't safe).
5. Run your registered seeders, in the order you registered them.

## Regenerating the pristine dump

When your schema changes — new table, new column, a new default row that should exist from the start — the saved snapshot goes stale. Force a regeneration with `WithGeneratePristine(true)`:

```go
snapdb.Run(
    m, mysql.New(), schemaInit, dataInit, engineInit,
    snapdb.WithGeneratePristine(true), // forces the slow path, ignoring any existing snapshot
)
```

A common pattern is a dedicated make target that runs this once and commits the resulting dump file:

```makefile
.PHONY: pristine
pristine:
	go test -run '^$$' ./... -tags=pristine
```

Commit the generated dump to your repo. Everyone else's `go test` then takes the fast path from the start.