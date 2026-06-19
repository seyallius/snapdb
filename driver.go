// Package dbtestkit. driver.go - Defines the set of supported database drivers and
// provides validation helpers.
package snapdb

import "context"

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

type DatabaseDriver interface {
	Driver() Driver
	Start(ctx context.Context, env *Environment) (string, error)
	RestoreDump(ctx context.Context, env *Environment, dumpPath string) error
	GenerateDump(ctx context.Context, env *Environment, dumpPath string) error
	Truncate(ctx context.Context, env *Environment) error
	Stop(ctx context.Context, env *Environment) error
}

// Driver identifies a supported database backend.
//
// Use one of the named constants below when configuring the environment via
// WithDriver. Custom drivers can be registered separately via the
// drivers subpackage.
type Driver string

const (
	// DriverMySQL selects a MySQL 8.x test container.
	DriverMySQL Driver = "mysql"

	// DriverPostgres selects a PostgreSQL test container.
	DriverPostgres Driver = "postgres"

	// DriverSQLite selects an in-process, file-backed SQLite database (no Docker).
	DriverSQLite Driver = "sqlite"
)

// -------------------------------------------- Public API ------------------------------------------ //

// SupportedDrivers returns the list of drivers built into the library.
//
// Useful for documentation, CLI flag validation, or generating help text.
func SupportedDrivers() []Driver {
	return []Driver{DriverMySQL, DriverPostgres, DriverSQLite}
}

// IsValid reports whether the given driver is supported by the library.
func (d Driver) IsValid() bool {
	switch d {
	case DriverMySQL, DriverPostgres, DriverSQLite:
		return true
	default:
		return false
	}
}

// String implements fmt.Stringer.
func (d Driver) String() string { return string(d) }
