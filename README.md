# snapdb

**Boot, snapshot, reset.** Fast integration test databases for Go with Docker-backed MySQL/Postgres and native SQLite.

You know the drill: you want real integration tests against a real database, but spinning one up and reseeding it between every test is slow enough that people start skipping it, or mocking everything, or just... not testing the database layer properly. `snapdb` exists so you don't have to make that trade-off.

It boots a database once per test package, takes a snapshot of it in a known-good state, and then restores that snapshot in milliseconds before every test. No more "let me just truncate everything and hope the seed data comes back right." One `Reset()` call, clean database, every time.

```go
func TestUserSignup(t *testing.T) {
    if err := snapdb.Reset(t, "TestUserSignup"); err != nil {
        t.Fatalf("reset: %v", err)
    }
    // your test, with a guaranteed-clean DB
}
```

That's basically the whole pitch. Everything else in this README is detail.

---

## Why this exists

This started as test infrastructure buried inside [Casdoor](https://github.com/casdoor/casdoor) — good code, but tangled up with Casdoor's own globals, config files, and Xorm engine (I had cloned casdoor and was working on something and eventually had developed a really fine integration testing framework. Didn't asked for PR though...). We pulled the *pattern* out (boot once, snapshot, restore fast) and rebuilt it as a standalone library with zero opinions about your ORM, your schema, or your domain objects.

So `snapdb` doesn't know what xorm is. It doesn't know what gorm is. It has no idea what your `User` struct looks like. All it knows is: here's a database, here's a snapshot of it, restore the snapshot when asked. You bring the rest via small callback functions.

---

## What you get

- 🐳 **MySQL and Postgres, via Docker** — boots a real container, tmpfs-backed so it's fast, torn down automatically when your test binary exits.
- 📁 **SQLite, no Docker at all** — just a file on disk. Great for local dev or CI runners without Docker.
- ⚡ **Millisecond resets** — the slow part (creating your schema, seeding base data) happens once. After that, every test gets a fresh database by restoring a pre-baked snapshot, not by re-running your seed logic.
- 🧩 **Bring your own ORM** — a tiny `Engine` interface (5 methods) is all `snapdb` needs. An xorm adapter ships out of the box; gorm, sqlx, or plain `database/sql` are ~30 lines away.
- 🔒 **Safe with `t.Parallel()`** — resets are serialized internally, so parallel tests sharing one container don't trip over each other.
- 🎛️ **No env vars, no magic** — configuration is explicit functional options, plus four required callbacks the compiler won't let you forget.

---

## Install

```bash
go get github.com/seyallius/snapdb
```

---

## A minimal example

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
        mysql.New(), // pick a backend

        // creates your schema (runs once, slow path only)
        func(env *snapdb.Environment) error {
            eng, _ := xorm.NewEngine("mysql", env.DSN())
            return eng.Sync2(&User{}, &Org{})
        },

        // seeds your base data (runs once, slow path only)
        func(env *snapdb.Environment) error {
            return seedDefaults(env)
        },

        // builds the Engine snapdb will use internally
        func(env *snapdb.Environment) (snapdb.Engine, error) {
            eng, err := xorm.NewEngine("mysql", env.DSN())
            if err != nil {
                return nil, err
            }
            return xormadapter.New(eng), nil
        },
    )
}

func TestUserSignup(t *testing.T) {
    if err := snapdb.Reset(t, "TestUserSignup"); err != nil {
        t.Fatalf("reset: %v", err)
    }
    // write your test against a guaranteed-clean DB
}
```

Those first four arguments to `Run` — the driver, schema setup, data seeding, and engine construction — are the only things `snapdb` truly can't guess for you, so they're plain function arguments rather than options. Forget one and your code won't compile, instead of failing on the first `go test` run.

Everything else (credentials, custom seeders, logging, where files get written) is optional and layered on as `With...` options after those four. The [quickstart guide](docs/quickstart.md) walks through a fuller example, including SQLite (no Docker needed).

---

## Documentation

This README is the "what and why." For the "how," head into [`/docs`](docs/):

- **[Quickstart](docs/quickstart.md)** — get a test suite running in a few minutes, MySQL or SQLite.
- **[Configuration reference](docs/configuration.md)** — every option, every default, in one table.
- **[Drivers](docs/drivers.md)** — what each backend actually does under the hood, and how to write your own.
- **[The Engine & Environment types](docs/engine-and-environment.md)** — the two interfaces you'll actually touch.
- **[Caching & reset correctness](docs/caching.md)** — the genuinely tricky part of test isolation, and how `snapdb` handles it.
- **[Design notes & FAQ](docs/design-notes.md)** — the "why did you do it this way" answers.

---

## A quick note on performance

Numbers will vary by machine, but here's roughly what to expect:

| Operation        | MySQL  | Postgres | SQLite |
|------------------|--------|----------|--------|
| First-run setup  | ~8.7 s | ~6 s     | ~3 s   |
| Subsequent setup | ~1.2 s | ~1.5 s   | ~50 ms |
| Per-test reset   | ~30 ms | ~40 ms   | ~5 ms  |

The first run is slow because it's actually building your schema and seeding data. Every run after that just restores a snapshot — that's where the speed comes from.
