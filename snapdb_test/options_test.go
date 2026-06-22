// Package snapdb. options_test.go - Unit tests for the functional-options
// API. Pure Go — no Docker required.
package snapdb_test

import (
	"testing"

	"github.com/seyallius/snapdb"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------- Helpers-------------------------------------------- //

// noopSchemaInit is a minimal SchemaInitializer that satisfies the
// "required" validation without doing any work.
func noopSchemaInit(*snapdb.Environment) error { return nil }

// noopDataInit is a minimal DataInitializer.
func noopDataInit(*snapdb.Environment) error { return nil }

// noopEngineInit returns a NoopEngine.
func noopEngineInit(*snapdb.Environment) (snapdb.Engine, error) {
	return snapdb.NoopEngine{}, nil
}

// validBaseOpts returns the minimal set of options required to pass
// validation. Tests can append additional options on top.
func validBaseOpts() []snapdb.Option {
	return []snapdb.Option{
		snapdb.WithDriver(newMockDriver("fake-dsn")),
		snapdb.WithSchemaInitializer(noopSchemaInit),
		snapdb.WithDataInitializer(noopDataInit),
		snapdb.WithEngineInitializer(noopEngineInit),
		snapdb.WithProjectRoot("/tmp/snapdb-fake-project"),
		snapdb.WithTestdataDir("/tmp/snapdb-fake-testdata"),
	}
}

// ----------------------------------------------- Tests -------------------------------------------- //

// TestDriver_IsValid verifies the Driver.IsValid predicate.
func TestDriver_IsValid(t *testing.T) {
	require.True(t, snapdb.DriverMySQL.IsValid())
	require.True(t, snapdb.DriverPostgres.IsValid())
	require.True(t, snapdb.DriverSQLite.IsValid())
	require.False(t, snapdb.Driver("oracle").IsValid())
	require.False(t, snapdb.Driver("").IsValid())
}

// TestSupportedDrivers_List verifies the list contains all built-in drivers.
func TestSupportedDrivers_List(t *testing.T) {
	drivers := snapdb.SupportedDrivers()
	require.Len(t, drivers, 3)
	require.Contains(t, drivers, snapdb.DriverMySQL)
	require.Contains(t, drivers, snapdb.DriverPostgres)
	require.Contains(t, drivers, snapdb.DriverSQLite)
}

// TestOptions_RequiredDriver verifies that omitting WithDriver returns a
// descriptive error.
func TestOptions_RequiredDriver(t *testing.T) {
	_, err := snapdb.ApplyOptionsForTesting(
		snapdb.WithSchemaInitializer(noopSchemaInit),
		snapdb.WithDataInitializer(noopDataInit),
		snapdb.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithDriver is required")
}

// TestOptions_RequiredSchemaInit verifies that omitting WithSchemaInitializer
// returns a descriptive error.
func TestOptions_RequiredSchemaInit(t *testing.T) {
	_, err := snapdb.ApplyOptionsForTesting(
		snapdb.WithDriver(newMockDriver("fake-dsn")),
		snapdb.WithDataInitializer(noopDataInit),
		snapdb.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithSchemaInitializer is required")
}

// TestOptions_RequiredDataInit verifies that omitting WithDataInitializer
// returns a descriptive error.
func TestOptions_RequiredDataInit(t *testing.T) {
	_, err := snapdb.ApplyOptionsForTesting(
		snapdb.WithDriver(newMockDriver("fake-dsn")),
		snapdb.WithSchemaInitializer(noopSchemaInit),
		snapdb.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithDataInitializer is required")
}

// TestOptions_RequiredEngineInit verifies that omitting WithEngineInitializer
// returns a descriptive error.
func TestOptions_RequiredEngineInit(t *testing.T) {
	_, err := snapdb.ApplyOptionsForTesting(
		snapdb.WithDriver(newMockDriver("fake-dsn")),
		snapdb.WithSchemaInitializer(noopSchemaInit),
		snapdb.WithDataInitializer(noopDataInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithEngineInitializer is required")
}

// TestOptions_NilDriver verifies that passing a nil driver is rejected.
func TestOptions_NilDriver(t *testing.T) {
	_, err := snapdb.ApplyOptionsForTesting(
		snapdb.WithDriver(nil),
		snapdb.WithSchemaInitializer(noopSchemaInit),
		snapdb.WithDataInitializer(noopDataInit),
		snapdb.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-nil driver")
}

// TestOptions_NilCallbacks verifies that nil callbacks are rejected for every
// callback-typed option.
func TestOptions_NilCallbacks(t *testing.T) {
	t.Run("nil schema init", func(t *testing.T) {
		_, err := snapdb.ApplyOptionsForTesting(
			snapdb.WithDriver(newMockDriver("fake-dsn")),
			snapdb.WithSchemaInitializer(nil),
			snapdb.WithDataInitializer(noopDataInit),
			snapdb.WithEngineInitializer(noopEngineInit),
		)
		require.Error(t, err)
	})
	t.Run("nil data init", func(t *testing.T) {
		_, err := snapdb.ApplyOptionsForTesting(
			snapdb.WithDriver(newMockDriver("fake-dsn")),
			snapdb.WithSchemaInitializer(noopSchemaInit),
			snapdb.WithDataInitializer(nil),
			snapdb.WithEngineInitializer(noopEngineInit),
		)
		require.Error(t, err)
	})
	t.Run("nil engine init", func(t *testing.T) {
		_, err := snapdb.ApplyOptionsForTesting(
			snapdb.WithDriver(newMockDriver("fake-dsn")),
			snapdb.WithSchemaInitializer(noopSchemaInit),
			snapdb.WithDataInitializer(noopDataInit),
			snapdb.WithEngineInitializer(nil),
		)
		require.Error(t, err)
	})
	t.Run("nil cache invalidator", func(t *testing.T) {
		_, err := snapdb.ApplyOptionsForTesting(
			append(validBaseOpts(),
				snapdb.WithCacheInvalidator(nil),
			)...,
		)
		require.Error(t, err)
	})
	t.Run("nil seeder in list", func(t *testing.T) {
		_, err := snapdb.ApplyOptionsForTesting(
			append(validBaseOpts(),
				snapdb.WithSeeders(nil),
			)...,
		)
		require.Error(t, err)
	})
}

// TestOptions_DefaultsApplied verifies that defaults fill in for unset
// fields when the user provides only required options.
func TestOptions_DefaultsApplied(t *testing.T) {
	cfg, err := snapdb.ApplyOptionsForTesting(validBaseOpts()...)
	require.NoError(t, err)

	require.Equal(t, "/tmp/snapdb-fake-project", cfg.ProjectRoot)
	require.Equal(t, "/tmp/snapdb-fake-testdata", cfg.TestdataDir)
	// Pristine dump path is derived from testdata dir + driver name.
	require.Contains(t, cfg.PristineDumpPath, "sqlite-pristine.sql")
}

// TestOptions_WithSeeders_Appends verifies that multiple WithSeeders calls
// accumulate rather than overwrite.
func TestOptions_WithSeeders_Appends(t *testing.T) {
	cfg, err := snapdb.ApplyOptionsForTesting(
		append(validBaseOpts(),
			snapdb.WithSeeders(func(*snapdb.Environment) error { return nil }),
			snapdb.WithSeeders(func(*snapdb.Environment) error { return nil }),
			snapdb.WithSeeders(func(*snapdb.Environment) error { return nil }),
		)...,
	)
	require.NoError(t, err)
	// 3 seeders should be present.
	require.Len(t, cfg.Seeders, 3)
}

// TestOptions_WithPristineDumpPath overrides the default.
func TestOptions_WithPristineDumpPath(t *testing.T) {
	cfg, err := snapdb.ApplyOptionsForTesting(
		append(validBaseOpts(),
			snapdb.WithPristineDumpPath("/custom/dump.sql"),
		)...,
	)
	require.NoError(t, err)
	require.Equal(t, "/custom/dump.sql", cfg.PristineDumpPath)
}

// TestOptions_WithGeneratePristine toggles the slow-path force flag.
func TestOptions_WithGeneratePristine(t *testing.T) {
	cfgOff, err := snapdb.ApplyOptionsForTesting(validBaseOpts()...)
	require.NoError(t, err)
	require.False(t, cfgOff.GeneratePristine)

	cfgOn, err := snapdb.ApplyOptionsForTesting(
		append(validBaseOpts(),
			snapdb.WithGeneratePristine(true),
		)...,
	)
	require.NoError(t, err)
	require.True(t, cfgOn.GeneratePristine)
}

// TestOptions_NilOptionRejected verifies that a nil option in the slice
// produces a descriptive error rather than a panic.
func TestOptions_NilOptionRejected(t *testing.T) {
	_, err := snapdb.ApplyOptionsForTesting(
		snapdb.WithDriver(newMockDriver("fake-dsn")),
		nil, // <- this should error
		snapdb.WithSchemaInitializer(noopSchemaInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

// TestOptions_WithDatabase_RejectsEmpty verifies that WithDatabase requires
// a non-empty Database name.
func TestOptions_WithDatabase_RejectsEmpty(t *testing.T) {
	_, err := snapdb.ApplyOptionsForTesting(
		append(validBaseOpts(),
			snapdb.WithDatabase(snapdb.DatabaseConfig{
				// Database intentionally empty
				Username: "root",
				Password: "pw",
				Image:    "mysql:lts",
			}),
		)...,
	)
	require.NoError(t, err)
}

// TestOptions_ApplicativeOrder verifies that later options win for scalar
// fields.
func TestOptions_ApplicativeOrder(t *testing.T) {
	cfg, err := snapdb.ApplyOptionsForTesting(
		append(validBaseOpts(),
			snapdb.WithTestdataDir("/first"),
			snapdb.WithTestdataDir("/second"),
		)...,
	)
	require.NoError(t, err)
	require.Equal(t, "/second", cfg.TestdataDir)
}

// TestOptions_NilLoggerAllowed verifies that WithLogger(nil) cleanly disables
// output rather than panicking on a nil interface call.
func TestOptions_NilLoggerAllowed(t *testing.T) {
	cfg, err := snapdb.ApplyOptionsForTesting(
		append(validBaseOpts(),
			snapdb.WithLogger(nil),
		)...,
	)
	require.NoError(t, err)
	require.Nil(t, cfg.Logger)
}
