# Caching and reset correctness

Here's something that catches people off guard: the hard part of test isolation usually isn't the database. It's everything sitting *in front of* the database that doesn't know a reset just happened.

## The actual problem

Say your ORM caches query results — xorm does this with a Ristretto-backed store, and plenty of others do something similar. You run a test, it reads a `User` row, the ORM caches a pointer to that row in memory. Then `Reset()` runs: the database gets wiped and reseeded. The *database* is now clean. But that cache? It's still holding a pointer to the old, now-gone row. Your next test reads from cache, gets stale (or in some async-eviction cases, genuinely invalid) data, and you spend an afternoon convinced your database reset logic is broken when it's actually working fine — the cache just didn't get the memo.

This was a real, specific bug in the original Casdoor test infrastructure this library grew out of: Ristretto's eviction happens on background goroutines, so there's a window where a "cleared" cache can still hand out a pointer to a row that no longer exists. Anything you build on top of `snapdb` that uses a caching ORM can hit the same thing if you don't flush the cache on reset.

## How `snapdb` handles it

Two deliberately separate hooks, called in this order on every `Reset()`:

1. **`WithCacheInvalidator`** — runs first, before the database is touched at all. This is for *your* application-level caches — things `snapdb` has no way of knowing about. Got a `sync.Map` full of "logged in users"? An in-memory permissions lookup? Flush it here.

2. **`Engine.ClearCache()`** — runs second, right after. This is the ORM-level cache, flushed through whatever `Engine` you built. For the xorm adapter, this forwards straight to `xorm.Engine.ClearCache()`.

```go
snapdb.Run(
    m, mysql.New(), schemaInit, dataInit, engineInit,
    snapdb.WithCacheInvalidator(func(env *snapdb.Environment) error {
        myAppCache.Clear()
        permissionsLookup.Reset()
        return nil
    }),
)
```

If your ORM doesn't cache anything — plain `database/sql`, most lightweight query builders — your `Engine.ClearCache()` implementation can just `return nil`. `snapdb` calls it unconditionally every reset; a no-op is a completely valid answer.

## The order matters

Cache invalidation happens **before** the database restore, not after. The reasoning: you want any stale pointers gone *before* new data lands, not racing against it. If invalidation ran after the restore, there'd be a window where freshly-seeded rows exist in the database but a stale cache entry from the previous test could still shadow them on the next read.

## A mental model that helps

Treat "is my test database actually clean" as two separate questions:

- **Is the data on disk correct?** That's what `RestoreDump`/`Truncate` handles.
- **Does anything in memory still think the old data is true?** That's what `WithCacheInvalidator` and `Engine.ClearCache()` handle.

Most test-isolation bugs that *look* like database problems are actually the second question going unanswered. If you ever see a test fail intermittently with data that looks like it's from a *previous* test rather than missing entirely, that's usually the tell — go check what's caching what.