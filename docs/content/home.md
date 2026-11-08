# snapdb

**Boot, snapshot, reset.** Fast integration test databases for Go with Docker-backed MySQL/Postgres and native SQLite.

You know the drill: you want real integration tests against a real database, but spinning one up and reseeding it between every test is slow enough that people start skipping it, or mocking everything, or just... not testing the database layer properly. `snapdb` exists so you don't have to make that trade-off.

It boots a database once per test package, takes a snapshot of it in a known-good state, and then restores that snapshot in milliseconds before every test.

```go
func TestUserSignup(t *testing.T) {
    if err := snapdb.Reset(t, "TestUserSignup"); err != nil {
        t.Fatalf("reset: %v", err)
    }
    // your test, with a guaranteed-clean DB
}
```

## Why this exists

This started as test infrastructure buried inside [Casdoor](https://github.com/casdoor/casdoor) — good code, but tangled up with Casdoor's own globals, config files, and Xorm engine. We pulled the *pattern* out (boot once, snapshot, restore fast) and rebuilt it as a standalone library with zero opinions about your ORM, your schema, or your domain objects.

So `snapdb` doesn't know what xorm is. It doesn't know what gorm is. It has no idea what your `User` struct looks like. All it knows is: here's a database, here's a snapshot of it, restore the snapshot when asked. You bring the rest via small callback functions.

## What you get

- 🐳 **MySQL and Postgres, via Docker** — boots a real container, tmpfs-backed so it's fast, torn down automatically when your test binary exits.
- 📁 **SQLite, no Docker at all** — just a file on disk. Great for local dev or CI runners without Docker.
- ⚡ **Millisecond resets** — the slow part (creating your schema, seeding base data) happens once. After that, every test gets a fresh database by restoring a pre-baked snapshot, not by re-running your seed logic.
- 🧩 **Bring your own ORM** — a tiny `Engine` interface (5 methods) is all `snapdb` needs. An xorm adapter ships out of the box; gorm, sqlx, or plain `database/sql` are ~30 lines away.
- 🔒 **Safe with `t.Parallel()`** — resets are serialized internally, so parallel tests sharing one container don't trip over each other.
- 🎛️ **No env vars, no magic** — configuration is explicit functional options, plus four required callbacks the compiler won't let you forget.

## Install

```console
go get github.com/seyallius/snapdb
```

## Where to go next

- **[Quickstart](quickstart.md)** — get a test suite running in a few minutes, MySQL or SQLite.
- **[Configuration reference](configuration.md)** — every option, every default, in one table.
- **[Drivers](drivers.md)** — what each backend actually does under the hood, and how to write your own.
- **[The Engine & Environment types](engine-and-environment.md)** — the two interfaces you'll actually touch.
- **[Caching & reset correctness](caching.md)** — the genuinely tricky part of test isolation, and how `snapdb` handles it.
- **[Design notes & FAQ](design-notes.md)** — the "why did you do it this way" answers.

Source and the full README live at [github.com/seyallius/snapdb](https://github.com/seyallius/snapdb). The **Performance** section in the sidebar tracks this library's own microbenchmarks via [benchbook](https://github.com/seyallius/benchbook) — it's incidental to this site, not the point of it.
