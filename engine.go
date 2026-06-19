// Package dbtestkit. engine.go - Defines the minimal Engine abstraction the library
// needs to talk to the user's database for truncation, pings, and cache invalidation.
//
// The library intentionally does NOT depend on any specific ORM (xorm, gorm, sqlx, …).
// Users adapt their ORM to this interface; an xorm adapter ships in the
// dbtestkit/xormadapter subpackage.
package snapdb

import "database/sql"

// --------------------------------------------- Types ---------------------------------------------- //

// Engine is the minimal database surface area the library requires.
//
// It exists so the library can TRUNCATE tables, ping the DB, clear ORM-level caches,
// and close the connection — without importing any ORM. Implementations only need to
// forward these calls to the underlying *sql.DB, *xorm.Engine, *gorm.DB, etc.
type Engine interface {
	// Exec executes a statement that does not return rows.
	Exec(query string, args ...any) (sql.Result, error)

	// QueryString runs a query and returns each row as a map[string]string
	// (column name → stringified value). Used for SHOW TABLES / sqlite_master.
	QueryString(query string, args ...any) ([]map[string]string, error)

	// Ping verifies the database connection is alive.
	Ping() error

	// Close releases the underlying connection pool.
	Close() error

	// ClearCache flushes any ORM-level in-memory cache.
	//
	// Return nil if the underlying ORM has no cache (e.g. plain *sql.DB).
	ClearCache() error
}

// NoopEngine is a no-op Engine implementation intended for tests that want to
// exercise library plumbing without a real database. Production callers should
// provide a real engine via WithEngineInitializer.
type NoopEngine struct{}

// -------------------------------------------- Public API ------------------------------------------ //

// Exec satisfies the Engine interface and returns a no-op result.
func (NoopEngine) Exec(string, ...any) (sql.Result, error) { return sql.Result(nil), nil }

// QueryString satisfies the Engine interface and returns an empty slice.
func (NoopEngine) QueryString(string, ...any) ([]map[string]string, error) {
	return nil, nil
}

// Ping satisfies the Engine interface.
func (NoopEngine) Ping() error { return nil }

// Close satisfies the Engine interface.
func (NoopEngine) Close() error { return nil }

// ClearCache satisfies the Engine interface.
func (NoopEngine) ClearCache() error { return nil }
