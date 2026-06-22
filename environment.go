// Package snapdb. environment.go - Defines the Environment struct that is passed
// to every user-supplied callback. It exposes the runtime context the caller needs
// (DSN, driver, project root, logger, engine handle) without leaking internal state.
package snapdb

import (
	"context"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// Environment is the runtime handle passed to every user callback
// (schema init, data init, engine init, cache invalidation, seeders).
//
// It is intentionally read-only from the caller's perspective. The library
// populates it during the setup phase; callers consume its fields.
type Environment struct {
	driver      Driver
	database    DatabaseConfig
	dsn         string
	projectRoot string
	testdataDir string
	sqlitePath  string
	engine      Engine
	logger      Logger
	ctx         context.Context
}

// -------------------------------------------- Public API ------------------------------------------ //

// Driver returns the active database driver.
func (e *Environment) Driver() Driver { return e.driver }

// Database returns the database credentials and image configuration supplied
// via WithDatabase (or the driver's defaults if WithDatabase was not called).
//
// Driver implementations use this to build container requests. User callbacks
// typically do not need it — they should use DSN() instead.
func (e *Environment) Database() DatabaseConfig { return e.database }

// DriverConfig is an alias for Database used by driver implementations.
// Kept as a separate method so the public API reads naturally for both
// audiences.
func (e *Environment) DriverConfig() DatabaseConfig { return e.database }

// DSN returns the connection string the engine should use to connect.
//
// For MySQL/Postgres this is a network DSN; for SQLite this is a file:// URL.
// The value is only populated after the underlying container (or file) has
// been created.
func (e *Environment) DSN() string { return e.dsn }

// ProjectRoot returns the absolute path to the project root (where go.mod lives).
func (e *Environment) ProjectRoot() string { return e.projectRoot }

// TestdataDir returns the absolute path to the testdata directory.
//
// Defaults to <projectRoot>/testdata. Override with WithTestdataDir.
func (e *Environment) TestdataDir() string { return e.testdataDir }

// SQLitePath returns the on-disk location of the SQLite database file.
//
// Only meaningful when Driver() == DriverSQLite. Returns the empty string
// for other drivers.
func (e *Environment) SQLitePath() string { return e.sqlitePath }

// Engine returns the user-supplied Engine, or nil if it has not been
// constructed yet (e.g. during the schema-initialization phase).
func (e *Environment) Engine() Engine { return e.engine }

// Logger returns the active Logger.
func (e *Environment) Logger() Logger { return e.logger }

// Context returns the lifecycle context bound to the current setup or reset
// operation. Callers should prefer this context over context.Background() so
// their work is cancelled when the test process exits.
func (e *Environment) Context() context.Context {
	if e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// withEngine returns a shallow copy of the Environment with the engine field
// replaced. Used internally so callbacks invoked after engine construction
// (seeders, cache invalidator) observe the live engine.
func (e *Environment) withEngine(eng Engine) *Environment {
	cp := *e
	cp.engine = eng
	return &cp
}
