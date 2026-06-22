# Design notes & FAQ

A collection of "why did you do it this way" answers. None of this is required reading to use `snapdb` — it's here for when you're curious, or when you're deciding whether to extend it.

## Why required arguments instead of all functional options?

Earlier versions of this library took everything — driver, schema setup, data seeding, engine construction — as `With...` options, validated at runtime. That meant forgetting one didn't fail until you ran `go test`, and the error was a string (`"WithEngineInitializer is required"`) rather than something your editor could catch.

The fix: split configuration into what's *genuinely optional* (credentials, logging, custom seeders — anything with a sensible default or no default at all) versus what's *never optional* (you cannot meaningfully boot a test database without knowing which database, how to build its schema, how to seed it, and how to talk to it). The second group became plain positional arguments to `Run`. Now a missing one is a compile error, and the four things every setup truly needs are visible right in the function signature instead of buried in a list of options.

The old `WithDriver` / `WithSchemaInitializer` / `WithDataInitializer` / `WithEngineInitializer` options still exist, marked deprecated, so nothing that already depends on the old shape breaks.

## Why functional options for everything else, instead of env vars?

The very first version of this (back in its Casdoor days) used environment variables like `TEST_DB_DRIVER` and `TEST_GENERATE_PRISTINE`. Env vars are easy to reach for, but they're global, invisible to anyone reading the code, and they don't survive a refactor — nothing stops `TEST_DB_DRIVER` from getting renamed in CI config and silently breaking somewhere else entirely.

Functional options fix that:

- **Discoverable** — your editor's autocomplete shows you every `With...` function that exists.
- **Composable** — `WithSeeders` called twice just accumulates both sets; most other options are simple last-write-wins.
- **Testable** — there's an internal `ApplyOptionsForTesting` helper specifically so the library's own test suite can assert "yes, this option does what it says" without spinning up a container.
- **Type-safe** — passing the wrong type to an option is a compile error. An env var is just a string until something tries to parse it, possibly at 2am during a deploy.

## Why is the `DatabaseDriver` interface defined in the core package, not a `drivers` subpackage?

Because driver implementations (`drivers/mysql`, `drivers/sqlite`, and anything you write yourself) need to import the core `snapdb` package for the `Environment` type. If the interface itself lived in a `drivers` package, and `snapdb` needed to reference that interface for its own `Run`/`Reset` machinery, you'd get an import cycle: core imports drivers, drivers import core.

Defining the interface in core and letting individual driver packages import core (one-directional) sidesteps that entirely. It's the same trick `database/sql` plays with `database/sql/driver` — except here, since there's no separate "registry" concept needed, the interface just lives directly in the main package.

## Why is `Reset` a package-level function instead of a method on something?

Go's `TestMain` is inherently scoped to one process per test package — there's no natural place to stash "the current test database" except somewhere process-global, because that's genuinely what it is: one database, shared by every test in that package, for the lifetime of that test binary. A package-level function matches that reality instead of pretending there's an instance to call a method on.

The one risk with anything global is concurrent access, which is why `Reset` takes an internal lock before doing anything — if you've got `t.Parallel()` tests sharing the one container, their resets queue up safely instead of stepping on each other.

## Why does `schemaInit` see a `nil` engine?

Different ORMs want schema creation done in different orders relative to "the engine exists." Rather than picking one order and forcing every ORM to fit it, `snapdb` lets your `schemaInit` callback build whatever connection it personally needs straight from `env.DSN()`. The engine your `engineInit` callback returns — the one `snapdb` actually holds onto and uses internally — only gets built *after* schema sync finishes. So `env.Engine()` is `nil` specifically during `schemaInit`, and populated for everything that runs afterward (`dataInit`, cache invalidation, seeders).

This is covered in more detail, with the full method table, in [The Engine & Environment types](engine-and-environment.md).

## Why a binary file copy for SQLite instead of a SQL dump?

SQLite's `.dump` output is a sequence of `INSERT` statements, each effectively its own transaction when replayed — correct, but slow, especially as your seed data grows. Copying the raw database file produces byte-identical state roughly two orders of magnitude faster. The trade-off is that a binary snapshot isn't human-readable or diffable the way a `.sql` dump is — but for a throwaway test fixture, that's a trade worth making.

This does come with one sharp edge — see [the SQLite section of the drivers doc](drivers.md#sqlite-driverssqlite) for why SQLite uses `ResetStrategyTruncateAndSeed` instead of the snapshot-restore path the other drivers use.