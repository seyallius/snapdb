# dbtestkit

A generic, ORM-agnostic test container toolkit for Go integration tests that need a real database.

`dbtestkit` boots a database (in Docker for MySQL/PostgreSQL, or on local disk for SQLite), restores a pre-baked pristine SQL dump for millisecond resets between tests, and exposes a single `Reset` entry-point that returns the database to a known-good state before every test.

Configuration is entirely **functional-options based** — no environment variables. Every project-specific concern (schema creation, data seeding, engine construction, cache invalidation) is supplied by the caller via callbacks, so the library never imports your ORM or your domain objects.

---

## Features

- **Three backends out of the box** — MySQL 8.x, PostgreSQL 16, and in-process SQLite.
- **Fast path / slow path** — the first test run generates a pristine SQL dump (~8 s); subsequent runs restore it in milliseconds. Performance is preserved from the original casdoor test utils.
- **Functional options** — no env vars, no globals, no `init()` magic. Discoverable via code completion.
- **ORM-agnostic** — bring your own `Engine` (xorm, gorm, sqlx, plain `*sql.DB`). An xorm adapter ships in the `xormadapter` subpackage.
- **Test isolation** — `Reset` clears ORM caches, restores the pristine dump, and re-runs your custom seeders before every test.
- **Race-safe** — resets are serialized via a mutex so parallel tests in the same package share one container safely.
- **KISS / DRY / SOLID** — small interfaces, single-responsibility files, open-closed extension via the `DatabaseDriver` interface.

---

## Installation

```bash
go get github.com/seyallius/snapdb
```

---

## Quick start

This example uses xorm + MySQL. The pattern is identical for any other combination — swap the driver import and the engine initializer.

```go
package mypackage_test

import (
	"testing"

	"github.com/seyallius/snapdb"
	"github.com/seyallius/snapdb/drivers/mysql"
	"github.com/seyallius/snapdb/xormadapter"
	"github.com/xorm-io/xorm"
)

// TestMain wires up the test container once for the whole package.
func TestMain(m *testing.M) {
	dbtestkit.Run(m,
		// 1. Pick a backend.
		dbtestkit.WithDriver(mysql.New()),

		// 2. (Optional) Override credentials / image.
		dbtestkit.WithDatabase(dbtestkit.DatabaseConfig{
			Database: "myapp",
			Username: "root",
			Password: "testpass",
			Image:    "mysql:lts",
		}),

		// 3. Tell dbtestkit how to create tables (slow path only).
		dbtestkit.WithSchemaInitializer(func(env *dbtestkit.Environment) error {
			eng, _ := xorm.NewEngine("mysql", env.DSN())
			return eng.Sync2(&User{}, &Org{}, &Token{})
		}),

		// 4. Tell dbtestkit how to seed base data (slow path only).
		dbtestkit.WithDataInitializer(func(env *dbtestkit.Environment) error {
			// Insert your default org, app, admin user, etc.
			return seedDefaults(env)
		}),

		// 5. Tell dbtestkit how to build your engine from the DSN.
		dbtestkit.WithEngineInitializer(func(env *dbtestkit.Environment) (dbtestkit.Engine, error) {
			eng, err := xorm.NewEngine("mysql", env.DSN())
			if err != nil {
				return nil, err
			}
			return xormadapter.New(eng), nil
		}),

		// 6. (Optional) Flush ORM caches before every reset.
		dbtestkit.WithCacheInvalidator(func(env *dbtestkit.Environment) error {
			// e.g. clear your Ristretto store, in-memory lookup tables, etc.
			return nil
		}),

		// 7. (Optional) Per-test seeders.
		dbtestkit.WithSeeders(func(env *dbtestkit.Environment) error {
			// Insert test-specific fixtures.
			return nil
		}),
	)
}

// Each test calls Reset at the top to get a clean database.
func TestUserSignup(t *testing.T) {
	if err := dbtestkit.Reset(t, "TestUserSignup"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	// ... your test logic ...
}
```

---

## How it works

### Setup phase (once per test package)

`Run` performs a one-time setup:

1. Resolves the project root by walking up until `go.mod` is found.
2. Boots the backend (Docker container for MySQL/Postgres, local file for SQLite).
3. **Fast path** (default, if a pristine dump exists): restore the existing dump into the backend (~1 s).
4. **Slow path** (first run, or `WithGeneratePristine(true)`):
    - Call the user's `SchemaInitializer` (e.g. `xorm.Sync2`).
    - Call the user's `DataInitializer` (e.g. insert default rows).
    - Generate a fresh pristine dump (`mysqldump`, `pg_dump`, or SQLite file copy).
5. Call the user's `EngineInitializer` with the live DSN.
6. Ping the engine to verify connectivity.
7. Hand control to `m.Run()`.

### Reset phase (per test)

`Reset` is called at the top of every test that needs a clean DB:

1. Acquire a global mutex (parallel tests serialize on resets).
2. Call the user's `CacheInvalidator` (if provided) to flush ORM-level caches.
3. Call `engine.ClearCache()` to flush xorm's Ristretto store (or no-op for ORMs without a cache).
4. Restore the pristine dump into the backend (MySQL/Postgres: `mysql < dump.sql`; SQLite: file copy).
5. Run each registered `Seeder` in registration order.

### Performance characteristics

Carried over from the original casdoor test utils:

| Operation        | MySQL  | Postgres | SQLite |
|------------------|--------|----------|--------|
| First-run setup  | ~8.7 s | ~6 s     | ~3 s   |
| Subsequent setup | ~1.2 s | ~1.5 s   | ~50 ms |
| Per-test reset   | ~30 ms | ~40 ms   | ~5 ms  |

The first-run cost is dominated by `Sync2` + base data insertion. Subsequent runs restore the pre-baked dump. Per-test resets pipe the dump back into the container via the DB CLI (`mysql`, `psql`) or copy a snapshot file (SQLite) — both are dramatically faster than re-running ORM inserts.

---

## Configuration reference

All configuration is supplied via `WithXxx` options. There are no environment variables.

### Required options

| Option                      | Purpose                                             |
|-----------------------------|-----------------------------------------------------|
| `WithDriver(d)`             | The database driver instance (e.g. `mysql.New()`).  |
| `WithSchemaInitializer(fn)` | Callback that creates tables (slow path only).      |
| `WithDataInitializer(fn)`   | Callback that seeds base data (slow path only).     |
| `WithEngineInitializer(fn)` | Callback that builds the `Engine` from `env.DSN()`. |

### Optional options

| Option                     | Purpose                                                      |
|----------------------------|--------------------------------------------------------------|
| `WithDatabase(cfg)`        | Database name, credentials, image, startup timeout.          |
| `WithCacheInvalidator(fn)` | Flush ORM-level caches before every reset.                   |
| `WithSeeders(fns...)`      | Append per-test seeders. Repeatable.                         |
| `WithPristineDumpPath(p)`  | Override the location of the pristine dump file.             |
| `WithTestdataDir(p)`       | Override the testdata directory.                             |
| `WithSQLitePath(p)`        | Override the SQLite database file path (SQLite only).        |
| `WithProjectRoot(p)`       | Bypass automatic `go.mod` lookup.                            |
| `WithGeneratePristine(b)`  | Force the slow path on the next setup (regenerate the dump). |
| `WithLogger(l)`            | Replace the default logger. Pass `nil` to disable output.    |

### Defaults

If `WithDatabase` is omitted, sensible defaults are applied per driver:

| Driver   | Database | Username   | Password   | Image                |
|----------|----------|------------|------------|----------------------|
| MySQL    | `testdb` | `root`     | `testpass` | `mysql:lts`          |
| Postgres | `testdb` | `postgres` | `testpass` | `postgres:16-alpine` |
| SQLite   | n/a      | n/a        | n/a        | n/a                  |

If `WithTestdataDir` is omitted, it defaults to `<projectRoot>/testdata`.

If `WithPristineDumpPath` is omitted, it defaults to `<testdataDir>/<driver>-pristine.sql`.

If `WithSQLitePath` is omitted, it defaults to `<testdataDir>/dbtestkit.sqlite`.

---

## The `Environment` type

Every user callback receives an `*dbtestkit.Environment`. It exposes:

| Method          | Description                                                                                                                                                                       |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Driver()`      | The active `Driver` constant.                                                                                                                                                     |
| `Database()`    | The `DatabaseConfig` (credentials, image, etc.).                                                                                                                                  |
| `DSN()`         | The connection string. Populated after `Start()`.                                                                                                                                 |
| `ProjectRoot()` | Absolute path to the project root.                                                                                                                                                |
| `TestdataDir()` | Absolute path to the testdata directory.                                                                                                                                          |
| `SQLitePath()`  | SQLite file path (SQLite only).                                                                                                                                                   |
| `Engine()`      | The user-supplied `Engine`. **Nil during `SchemaInitializer`** (the engine is constructed after schema sync). Non-nil during `DataInitializer`, `CacheInvalidator`, and `Seeder`. |
| `Logger()`      | The active `Logger` (may be nil).                                                                                                                                                 |
| `Context()`     | The lifecycle context (cancelled when the test process exits).                                                                                                                    |

---

## The `Engine` interface

`dbtestkit` does **not** import any specific ORM. Instead, it defines a minimal `Engine` interface:

```go
type Engine interface {
    Exec(query string, args ...any) (sql.Result, error)
    QueryString(query string, args ...any) ([]map[string]string, error)
    Ping() error
    Close() error
    ClearCache() error
}
```

If you use xorm, the `xormadapter` subpackage provides a drop-in:

```go
import "github.com/seyallius/snapdb/xormadapter"

eng, _ := xorm.NewEngine("mysql", dsn)
return xormadapter.New(eng), nil
```

For other ORMs (gorm, sqlx, plain `*sql.DB`), implement the five methods yourself — it's about 30 lines of boilerplate.

---

## Drivers

### MySQL (`drivers/mysql`)

- Boots `mysql:lts` (override via `WithDatabase`).
- Uses **tmpfs** at `/var/lib/mysql` for in-memory storage.
- Ships an embedded `mysql-quickstart-entrypoint.sh` that re-hydrates a pre-baked empty DB tarball (skipping the multi-second `initdb` first-run sequence).
- Dump format: `mysqldump --add-drop-table --complete-insert --skip-triggers`.
- Reset: `mysql < dump.sql` (drop + recreate in one pass — no separate TRUNCATE needed).

To generate the empty-DB tarball for the first time, run this once on a machine with Docker:

```bash
docker run --rm -v "$PWD/testdata:/out" alpine sh -c '
  apk add --no-cache docker-cli
  docker run -d --name mysql-tmp \
    -e MYSQL_DATABASE=testdb \
    -e MYSQL_ROOT_PASSWORD=testpass \
    mysql:lts
  sleep 30
  docker exec mysql-tmp sh -c "tar cf - -C /var/lib/mysql . | gzip --fast" > /out/empty-mysql.tar.gz
  docker rm -f mysql-tmp
'
```

If the tarball is missing, the entrypoint gracefully falls back to the official `initdb` flow.

### Postgres (`drivers/postgres`)

- Boots `postgres:16-alpine` (override via `WithDatabase`).
- Uses **tmpfs** at `/var/lib/postgresql/data`.
- Dump format: `pg_dump --clean --if-exists --no-owner --no-privileges`.
- Reset: `psql -f dump.sql` (DROP + CREATE in one pass).

Postgres doesn't ship a quickstart tarball equivalent of MySQL's empty-mysql.tar.gz — `initdb` runs in ~1 s on tmpfs, which is already fast enough.

### SQLite (`drivers/sqlite`)

- No Docker. Uses an in-process, file-backed SQLite database.
- Dump format: **binary file snapshot** (not a SQL text dump) — dramatically faster than re-running `.dump` output.
- Reset: copy the snapshot file over the working DB file (~μs).
- Truncate fallback: walks `sqlite_master` and issues `DELETE FROM <table>` for each user table (used only if no snapshot exists).

---

## Adding a new driver

Implement the `dbtestkit.DatabaseDriver` interface:

```go
type DatabaseDriver interface {
    Driver() Driver
    Start(ctx context.Context, env *Environment) (string, error)
    RestoreDump(ctx context.Context, env *Environment, dumpPath string) error
    GenerateDump(ctx context.Context, env *Environment, dumpPath string) error
    Truncate(ctx context.Context, env *Environment) error
    Stop(ctx context.Context, env *Environment) error
}
```

Then expose a `New()` constructor and pass it to `WithDriver`:

```go
package redis

import "github.com/seyallius/snapdb"

func New() dbtestkit.DatabaseDriver { return &Driver{} }

type Driver struct{ /* ... */ }

func (d *Driver) Driver() dbtestkit.Driver { return dbtestkit.Driver("redis") }
// ... implement the remaining methods ...
```

Note that `dbtestkit.Driver` is just a `string`, so you can use a custom value — but the library's `IsValid()` predicate won't recognize it. That's fine for in-tree drivers; just don't expect the `SupportedDrivers()` list to include it.

---

## Caching and reset correctness

The hardest part of test isolation is **cache coherence**, not database state. The original casdoor code had subtle bugs where Ristretto's async eviction goroutines would hand out stale pointers after a reset. `dbtestkit` addresses this with two layers:

1. **`WithCacheInvalidator` callback** — invoked *before* the dump restore. Use this to flush any in-memory stores that hold pointers to DB rows (Ristretto, sync.Map, custom lookup tables).
2. **`Engine.ClearCache()`** — invoked *after* the cache invalidator. For xorm, this forwards to `xorm.Engine.ClearCache()` which flushes the ORM-level Ristretto store.

If your ORM has no cache (plain `*sql.DB`), return nil from both — the library handles it gracefully.

---

## Regenerating the pristine dump

When your schema changes (new table, new column, new default row), regenerate the pristine dump:

```go
// In a one-off test or a makefile target:
dbtestkit.Run(m,
    dbtestkit.WithDriver(mysql.New()),
    dbtestkit.WithGeneratePristine(true), // <-- forces slow path
    // ... rest of options ...
)
```

Or expose a `make pristine` target:

```makefile
.PHONY: pristine
pristine:
	go test -run '^$$' ./... -tags=pristine
```

The slow path runs once, generates the dump, and saves it to `<testdataDir>/<driver>-pristine.sql`. Commit the dump to your repo — subsequent test runs use the fast path.

---

## Migrating from the casdoor test utils

The original casdoor code used environment variables (`TEST_DB_DRIVER`, `TEST_GENERATE_PRISTINE`). This library replaces them with functional options:

| Old (env var)                 | New (option)                 |
|-------------------------------|------------------------------|
| `TEST_DB_DRIVER=mysql`        | `WithDriver(mysql.New())`    |
| `TEST_DB_DRIVER=sqlite`       | `WithDriver(sqlite.New())`   |
| `TEST_GENERATE_PRISTINE=true` | `WithGeneratePristine(true)` |

Project-specific functions like `object.InitDb()`, `object.CreateTables()`, `object.SetupCacheInvalidation()` move into the corresponding callbacks:

| Old (casdoor)                         | New (dbtestkit callback)     |
|---------------------------------------|------------------------------|
| `object.CreateTables()`               | `WithSchemaInitializer(...)` |
| `object.InitDb()`                     | `WithDataInitializer(...)`   |
| `object.InitAdapter()` + engine setup | `WithEngineInitializer(...)` |
| `object.SetupCacheInvalidation()`     | `WithCacheInvalidator(...)`  |
| `DefaultTestSeeder()`                 | `WithSeeders(...)`           |

The casdoor-specific `MockSession`, `NewMockBContextAndRecorder`, and `LogRequestResponse` helpers are **not** part of this library — they're beego-specific and don't belong in a generic DB test toolkit. Copy them into your project's own test utils package.

---

## Design notes

### Why functional options instead of env vars?

Env vars are global, hidden, and don't survive refactorings. Functional options are:

- **Discoverable** — IDE autocomplete shows every available knob.
- **Composable** — multiple `WithSeeders` calls accumulate; scalar options last-wins.
- **Testable** — `ApplyOptionsForTesting` lets you assert option application without spinning up a container.
- **Type-safe** — passing a non-`Driver` to `WithDriver` is a compile error, not a runtime surprise.

### Why a `DatabaseDriver` interface in the core package?

Placing the interface in the core `dbtestkit` package (rather than a `drivers` subpackage) avoids an import cycle: driver subpackages import `dbtestkit` for the `Environment` type, so `dbtestkit` cannot import them back. This is the same pattern `database/sql` uses with `driver.Driver`.

### Why is `Reset` global?

Go's `TestMain` model is inherently package-scoped — one process per package. A package-level global is the natural lifecycle boundary. The library serializes resets via a mutex, so `t.Parallel()` tests share one container safely.

### Why does `SchemaInitializer` see a nil engine?

In the slow path, schema creation (e.g. `xorm.Sync2`) typically needs its own engine handle. Rather than imposing a specific construction order, the library lets the `SchemaInitializer` build its own engine from `env.DSN()` if needed. The "official" engine — the one returned by `EngineInitializer` — is constructed *after* schema sync, so `env.Engine()` is nil during `SchemaInitializer` and non-nil during `DataInitializer`, `CacheInvalidator`, and `Seeder`.

### Why a binary snapshot for SQLite?

SQLite's `.dump` produces a SQL text file that's expensive to replay (each statement is a separate transaction). Copying the database file is ~100× faster and produces identical state. The trade-off is that the snapshot is binary — you can't inspect or diff it — but for test fixtures that's fine.

---

## Running the tests

The library ships with three categories of tests:

```bash
# Unit tests (no Docker required)
go test ./...

# SQLite integration tests (no Docker required)
go test ./drivers/sqlite/...

# MySQL / Postgres integration tests (require Docker)
go test ./drivers/mysql/...
go test ./drivers/postgres/...
```

### SQLite driver used in tests

The SQLite integration tests use [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — a pure-Go (no CGO) SQLite driver. This means the tests run out-of-the-box on Windows / macOS / Linux without requiring a C toolchain. The library itself is ORM-agnostic: production users of `dbtestkit` can choose `modernc.org/sqlite`, `mattn/go-sqlite3` (CGO), or any other SQLite driver — whatever they pass to their `WithEngineInitializer` is what gets used.

---

## License

MIT — see [LICENSE](../LICENSE).
