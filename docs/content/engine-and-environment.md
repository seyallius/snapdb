# The `Engine` and `Environment` types

These are the two types you'll actually interact with directly. Everything else in `snapdb` is plumbing you set up once in `TestMain` and then forget about.

## `Environment`

Every callback you hand to `Run` — `schemaInit`, `dataInit`, `engineInit`, `WithCacheInvalidator`, `WithSeeders` — receives a `*snapdb.Environment`. Think of it as "everything you might need to know about the database right now."

| Method          | Returns           | Notes                                                                                                 |
|-----------------|-------------------|-------------------------------------------------------------------------------------------------------|
| `Driver()`      | `Driver`          | Which backend is active (`DriverMySQL`, `DriverPostgres`, `DriverSQLite`).                            |
| `Database()`    | `DatabaseConfig`  | Credentials, image, etc. Mostly useful to driver authors; callbacks usually want `DSN()` instead.     |
| `DSN()`         | `string`          | The connection string. Only populated after the backend has actually started.                         |
| `ProjectRoot()` | `string`          | Absolute path to wherever `go.mod` lives.                                                             |
| `TestdataDir()` | `string`          | Absolute path to the testdata directory.                                                              |
| `SQLitePath()`  | `string`          | The SQLite file path. Empty string for MySQL/Postgres.                                                |
| `Engine()`      | `Engine`          | The engine you built in `engineInit` — **see the nil-engine note below.**                             |
| `Logger()`      | `Logger`          | The active logger, which may be `nil` if you disabled logging.                                        |
| `Context()`     | `context.Context` | Cancelled when the test process exits. Prefer this over `context.Background()` in your own callbacks. |

### Why `Engine()` is `nil` during `schemaInit`

This trips people up exactly once, so it's worth calling out clearly: **`env.Engine()` returns `nil` while your `schemaInit` callback is running.**

The reasoning: schema creation (`xorm.Sync2` and friends) often wants its own engine handle, and there's no single "correct" order for "build the engine" vs. "sync the schema" that works for every ORM. Rather than guessing, `snapdb` lets `schemaInit` build whatever engine handle it needs directly from `env.DSN()`. The "official" engine — the one your `engineInit` callback returns — only gets constructed *after* schema sync finishes. So:

- During `schemaInit`: `env.Engine()` is `nil`. Build your own connection from `env.DSN()` if you need one.
- During `dataInit`, `WithCacheInvalidator`, and any `WithSeeders` callback: `env.Engine()` is populated and ready to use.

## `Engine`

`snapdb` doesn't know what ORM you're using, and it doesn't want to. All it needs is five methods:

```go
type Engine interface {
    Exec(query string, args ...any) (sql.Result, error)
    QueryString(query string, args ...any) ([]map[string]string, error)
    Ping() error
    Close() error
    ClearCache() error
}
```

### If you're using xorm

There's a drop-in adapter:

```go
import "github.com/seyallius/snapdb/xormadapter"

eng, err := xorm.NewEngine("mysql", dsn)
if err != nil {
    return nil, err
}
return xormadapter.New(eng), nil
```

`xormadapter.Adapter` forwards `Exec` and `QueryString` straight through to the underlying `*xorm.Engine`, and `ClearCache` forwards to xorm's own cache-flushing, which matters if you're using xorm's Ristretto-backed query cache (more on that in [Caching & reset correctness](caching.md)).

### If you're using anything else

Implement the five methods yourself. It's genuinely small — for gorm, something like:

```go
package gormadapter

import (
    "database/sql"

    "github.com/seyallius/snapdb"
    "gorm.io/gorm"
)

type Adapter struct{ db *gorm.DB }

func New(db *gorm.DB) *Adapter { return &Adapter{db: db} }

func (a *Adapter) Exec(query string, args ...any) (sql.Result, error) {
    tx := a.db.Exec(query, args...)
    return nil, tx.Error // gorm doesn't expose a sql.Result; adjust to your needs
}

func (a *Adapter) QueryString(query string, args ...any) ([]map[string]string, error) {
    var rows []map[string]any
    if err := a.db.Raw(query, args...).Scan(&rows).Error; err != nil {
        return nil, err
    }
    // convert rows to []map[string]string as needed
    ...
}

func (a *Adapter) Ping() error {
    sqlDB, err := a.db.DB()
    if err != nil {
        return err
    }
    return sqlDB.Ping()
}

func (a *Adapter) Close() error {
    sqlDB, err := a.db.DB()
    if err != nil {
        return err
    }
    return sqlDB.Close()
}

func (a *Adapter) ClearCache() error { return nil } // gorm has no query cache to flush
```

If your ORM has no concept of a query cache (plain `database/sql`, most lightweight wrappers), just return `nil` from `ClearCache()`. `snapdb` calls it unconditionally on every reset, but a no-op is a perfectly fine implementation.

One adapter, one small file, and you never touch it again — it's infrastructure, not something you'll be debugging every week.