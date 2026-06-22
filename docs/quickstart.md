# Quickstart

This walks through wiring `snapdb` into a real test package, twice — once with MySQL (the "I want a real database" path) and once with SQLite (the "I don't want Docker right now" path). The pattern is identical either way; only the driver and the engine wiring change.

## 1. Install

```bash
go get github.com/seyallius/snapdb
```

## 2. Pick a driver and write `TestMain`

Every package that uses `snapdb` needs exactly one `TestMain`, because the database is booted once per test binary, not once per test.

`snapdb.Run` takes four things every setup needs — no exceptions, no defaults that could silently do the wrong thing — as plain arguments, followed by whatever optional configuration you want:

```go
snapdb.Run(
    m,           // *testing.M
    driver,      // which backend: mysql.New(), postgres.New(), or sqlite.New()
    schemaInit,  // func(env *snapdb.Environment) error — create your tables
    dataInit,    // func(env *snapdb.Environment) error — seed your base data
    engineInit,  // func(env *snapdb.Environment) (snapdb.Engine, error) — build your engine
    opts...,     // anything optional: WithDatabase, WithSeeders, WithLogger, etc.
)
```

Because these four are ordinary function arguments, the compiler enforces them. There's no `"WithSchemaInitializer is required"` error waiting to surprise you on the first `go test` run — if you forget one, your code doesn't build.

### MySQL version

```go
package mypackage_test

import (
    "testing"

    "github.com/seyallius/snapdb"
    "github.com/seyallius/snapdb/drivers/mysql"
    "github.com/seyallius/snapdb/xormadapter"
    "github.com/xorm-io/xorm"
)

func TestMain(m *testing.M) {
    snapdb.Run(
        m,
        mysql.New(),

        // SchemaInitializer — runs once, slow path only
        func(env *snapdb.Environment) error {
            eng, err := xorm.NewEngine("mysql", env.DSN())
            if err != nil {
                return err
            }
            return eng.Sync2(&User{}, &Org{}, &Token{})
        },

        // DataInitializer — runs once, slow path only
        func(env *snapdb.Environment) error {
            return seedDefaultOrgAndAdmin(env)
        },

        // EngineInitializer — builds the Engine snapdb hands to your callbacks
        func(env *snapdb.Environment) (snapdb.Engine, error) {
            eng, err := xorm.NewEngine("mysql", env.DSN())
            if err != nil {
                return nil, err
            }
            return xormadapter.New(eng), nil
        },

        // Optional: override credentials / image. Sensible defaults apply if you skip this.
        snapdb.WithDatabase(snapdb.DatabaseConfig{
            Database: "myapp",
            Username: "root",
            Password: "testpass",
            Image:    "mysql:lts",
        }),
    )
}
```

### SQLite version (no Docker)

Same shape, different driver, no `WithDatabase` needed:

```go
package mypackage_test

import (
    "testing"

    "github.com/seyallius/snapdb"
    "github.com/seyallius/snapdb/drivers/sqlite"
    "github.com/seyallius/snapdb/xormadapter"
    "github.com/xorm-io/xorm"
    _ "modernc.org/sqlite" // pure-Go driver, no CGO needed
)

func TestMain(m *testing.M) {
    snapdb.Run(
        m,
        sqlite.New(),
        func(env *snapdb.Environment) error {
            eng, err := xorm.NewEngine("sqlite", env.DSN())
            if err != nil {
                return err
            }
            return eng.Sync2(&User{}, &Org{}, &Token{})
        },
        func(env *snapdb.Environment) error {
            return seedDefaultOrgAndAdmin(env)
        },
        func(env *snapdb.Environment) (snapdb.Engine, error) {
            eng, err := xorm.NewEngine("sqlite", env.DSN())
            if err != nil {
                return nil, err
            }
            return xormadapter.New(eng), nil
        },
    )
}
```

## 3. Reset before every test

```go
func TestUserSignup(t *testing.T) {
    if err := snapdb.Reset(t, "TestUserSignup"); err != nil {
        t.Fatalf("reset: %v", err)
    }

    // the database is now exactly as it was right after your
    // DataInitializer ran — write your test against that state
}
```

The string you pass to `Reset` is just a label for log output — it shows up in the "🔄 Reset Test Environment [...]" banner so you can tell which test triggered which reset when you're staring at CI logs. It's optional; `snapdb.Reset(t)` works fine too.

## 4. Run it

```bash
go test ./...
```

The first run takes a few seconds (it's genuinely building your schema and seeding data). Every run after that is fast, because it's restoring a saved snapshot instead of redoing that work. If you change your schema later, see [Regenerating the pristine dump](configuration.md#regenerating-the-pristine-dump) in the configuration reference.

## Where to go next

- Need custom per-test fixtures on top of the base seed data? See `WithSeeders` in the [configuration reference](configuration.md).
- Using gorm or plain `database/sql` instead of xorm? See [The Engine interface](engine-and-environment.md#the-engine-interface) — it's a short interface to implement yourself.
- Curious what's actually happening during setup and reset? See [Drivers](drivers.md).