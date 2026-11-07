# Drivers

A "driver" in `snapdb` is anything that implements `DatabaseDriver`:

```go
type DatabaseDriver interface {
    Driver() Driver
    Start(ctx context.Context, env *Environment) (string, error)
    RestoreDump(ctx context.Context, env *Environment, dumpPath string) error
    GenerateDump(ctx context.Context, env *Environment, dumpPath string) error
    Truncate(ctx context.Context, env *Environment) error
    Stop(ctx context.Context, env *Environment) error
    ResetStrategy() ResetStrategy
}
```

Three ship with the library. Here's what each one is actually doing under the hood.

## MySQL (`drivers/mysql`)

- Boots `mysql:lts` by default — override the image via `WithDatabase`.
- Runs with `/var/lib/mysql` on **tmpfs**, so all the database's own disk I/O happens in memory.
- Ships an embedded entrypoint script that, if a pre-baked empty-database tarball is present, restores it directly instead of running MySQL's normal first-boot initialization sequence — which is the slowest part of starting a fresh container.
- Snapshot format: a `mysqldump` with `--add-drop-table --complete-insert --skip-triggers`.
- Reset: pipes the dump back in with `mysql < dump.sql`. Because the dump includes `DROP TABLE IF EXISTS`, this is a full drop-and-recreate in one pass — no separate truncation step needed.

**Generating the empty-database tarball.** This is a one-time, manual step for whoever first sets up the repo (or whenever the base MySQL image gets bumped):

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

If that tarball is missing, the entrypoint just falls back to MySQL's normal `initdb` flow — things still work, just a bit slower on that first boot.

## Postgres (`drivers/postgres`)

- Boots `postgres:16-alpine` by default.
- Also runs with tmpfs, at `/var/lib/postgresql/data`.
- Snapshot format: `pg_dump --clean --if-exists --no-owner --no-privileges`.
- Reset: `psql -f dump.sql`, which similarly drops and recreates in one pass.

Postgres doesn't get a quickstart tarball the way MySQL does — `initdb` on Postgres is already fast enough on tmpfs (about a second) that the extra machinery wasn't worth it.

## SQLite (`drivers/sqlite`)

- No Docker, no container, no network. It's a file.
- Snapshot format: a **binary file copy**, not a SQL dump. SQLite's `.dump` output replays as individual statements, each its own transaction — copying the raw file is roughly two orders of magnitude faster and produces byte-identical state.
- Reset: copy the snapshot file back over the working database file. Microseconds, basically.
- Fallback: if no snapshot exists yet, `Truncate` walks `sqlite_master`, finds every user table, and issues a `DELETE FROM` for each one, then re-runs your `DataInitializer`.

SQLite is also the one driver that declares `ResetStrategyTruncateAndSeed` instead of `ResetStrategyRestoreDump`. That's not a performance choice — it's a correctness one. If your SQLite connection runs in WAL mode (common — many ORMs turn it on by default), recently committed data can sit only in a `-wal` sidecar file until it's checkpointed back into the main database file. A naive copy-the-file snapshot can capture a database that's silently missing that data, and restoring it next to a stale `-wal` file from a different run produces SQLite's least friendly error message: `database disk image is malformed`. Declaring `ResetStrategyTruncateAndSeed` tells the core lifecycle "don't use snapshot semantics for me" — resets go through `Truncate` + re-seed instead, which is always safe regardless of journal mode.

**A practical note on SQLite drivers:** `snapdb`'s own test suite uses [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite), a pure-Go SQLite implementation with no CGO dependency. That's a choice about the *test suite*, not a constraint the library imposes on you — whatever you build inside your `EngineInitializer` is what actually gets used, so `mattn/go-sqlite3` (CGO-based) works just as well if that's your preference.

## Writing your own driver

Implement the interface above and expose a constructor:

```go
package redis

import "github.com/seyallius/snapdb"

func New() snapdb.DatabaseDriver { return &Driver{} }

type Driver struct{ /* ... */ }

func (d *Driver) Driver() snapdb.Driver       { return snapdb.Driver("redis") }
func (d *Driver) ResetStrategy() snapdb.ResetStrategy { return snapdb.ResetStrategyRestoreDump }
// ... Start, RestoreDump, GenerateDump, Truncate, Stop ...
```

A couple of things worth knowing before you do:

- **`snapdb.Driver` is just a string**, so a custom identifier like `"redis"` works fine at runtime. It just won't show up in `SupportedDrivers()`, which only lists the built-in three. That's cosmetic — it doesn't block anything.
- **Think honestly about `ResetStrategy`.** It exists because not every backend can safely support a binary snapshot restore (see the SQLite/WAL story above). If your backend has similar constraints — file locks, write-ahead logs, anything that makes "just copy the file" unsafe — declare `ResetStrategyTruncateAndSeed` and implement `Truncate` properly rather than forcing a snapshot approach that might silently corrupt state on a busy CI box.
- **The interface lives in the core `snapdb` package**, not a `drivers` subpackage, specifically so driver implementations can import `snapdb` (for the `Environment` type) without creating an import cycle. This is the same shape `database/sql` uses with `database/sql/driver`.