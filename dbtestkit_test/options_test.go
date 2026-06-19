// Package dbtestkit. options_test.go - Unit tests for the functional-options
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
func noopSchemaInit(*dbtestkit.Environment) error { return nil }

// noopDataInit is a minimal DataInitializer.
func noopDataInit(*dbtestkit.Environment) error { return nil }

// noopEngineInit returns a NoopEngine.
func noopEngineInit(*dbtestkit.Environment) (dbtestkit.Engine, error) {
	return dbtestkit.NoopEngine{}, nil
}

// validBaseOpts returns the minimal set of options required to pass
// validation. Tests can append additional options on top.
func validBaseOpts() []dbtestkit.Option {
	return []dbtestkit.Option{
		dbtestkit.WithDriver(newMockDriver("fake-dsn")),
		dbtestkit.WithSchemaInitializer(noopSchemaInit),
		dbtestkit.WithDataInitializer(noopDataInit),
		dbtestkit.WithEngineInitializer(noopEngineInit),
		dbtestkit.WithProjectRoot("/tmp/dbtestkit-fake-project"),
		dbtestkit.WithTestdataDir("/tmp/dbtestkit-fake-testdata"),
	}
}

// ----------------------------------------------- Tests -------------------------------------------- //

// TestDriver_IsValid verifies the Driver.IsValid predicate.
func TestDriver_IsValid(t *testing.T) {
	require.True(t, dbtestkit.DriverMySQL.IsValid())
	require.True(t, dbtestkit.DriverPostgres.IsValid())
	require.True(t, dbtestkit.DriverSQLite.IsValid())
	require.False(t, dbtestkit.Driver("oracle").IsValid())
	require.False(t, dbtestkit.Driver("").IsValid())
}

// TestSupportedDrivers_List verifies the list contains all built-in drivers.
func TestSupportedDrivers_List(t *testing.T) {
	drivers := dbtestkit.SupportedDrivers()
	require.Len(t, drivers, 3)
	require.Contains(t, drivers, dbtestkit.DriverMySQL)
	require.Contains(t, drivers, dbtestkit.DriverPostgres)
	require.Contains(t, drivers, dbtestkit.DriverSQLite)
}

// TestOptions_RequiredDriver verifies that omitting WithDriver returns a
// descriptive error.
func TestOptions_RequiredDriver(t *testing.T) {
	_, err := dbtestkit.ApplyOptionsForTesting(
		dbtestkit.WithSchemaInitializer(noopSchemaInit),
		dbtestkit.WithDataInitializer(noopDataInit),
		dbtestkit.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithDriver is required")
}

// TestOptions_RequiredSchemaInit verifies that omitting WithSchemaInitializer
// returns a descriptive error.
func TestOptions_RequiredSchemaInit(t *testing.T) {
	_, err := dbtestkit.ApplyOptionsForTesting(
		dbtestkit.WithDriver(newMockDriver("fake-dsn")),
		dbtestkit.WithDataInitializer(noopDataInit),
		dbtestkit.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithSchemaInitializer is required")
}

// TestOptions_RequiredDataInit verifies that omitting WithDataInitializer
// returns a descriptive error.
func TestOptions_RequiredDataInit(t *testing.T) {
	_, err := dbtestkit.ApplyOptionsForTesting(
		dbtestkit.WithDriver(newMockDriver("fake-dsn")),
		dbtestkit.WithSchemaInitializer(noopSchemaInit),
		dbtestkit.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithDataInitializer is required")
}

// TestOptions_RequiredEngineInit verifies that omitting WithEngineInitializer
// returns a descriptive error.
func TestOptions_RequiredEngineInit(t *testing.T) {
	_, err := dbtestkit.ApplyOptionsForTesting(
		dbtestkit.WithDriver(newMockDriver("fake-dsn")),
		dbtestkit.WithSchemaInitializer(noopSchemaInit),
		dbtestkit.WithDataInitializer(noopDataInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithEngineInitializer is required")
}

// TestOptions_NilDriver verifies that passing a nil driver is rejected.
func TestOptions_NilDriver(t *testing.T) {
	_, err := dbtestkit.ApplyOptionsForTesting(
		dbtestkit.WithDriver(nil),
		dbtestkit.WithSchemaInitializer(noopSchemaInit),
		dbtestkit.WithDataInitializer(noopDataInit),
		dbtestkit.WithEngineInitializer(noopEngineInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-nil driver")
}

// TestOptions_NilCallbacks verifies that nil callbacks are rejected for every
// callback-typed option.
func TestOptions_NilCallbacks(t *testing.T) {
	t.Run("nil schema init", func(t *testing.T) {
		_, err := dbtestkit.ApplyOptionsForTesting(
			dbtestkit.WithDriver(newMockDriver("fake-dsn")),
			dbtestkit.WithSchemaInitializer(nil),
			dbtestkit.WithDataInitializer(noopDataInit),
			dbtestkit.WithEngineInitializer(noopEngineInit),
		)
		require.Error(t, err)
	})
	t.Run("nil data init", func(t *testing.T) {
		_, err := dbtestkit.ApplyOptionsForTesting(
			dbtestkit.WithDriver(newMockDriver("fake-dsn")),
			dbtestkit.WithSchemaInitializer(noopSchemaInit),
			dbtestkit.WithDataInitializer(nil),
			dbtestkit.WithEngineInitializer(noopEngineInit),
		)
		require.Error(t, err)
	})
	t.Run("nil engine init", func(t *testing.T) {
		_, err := dbtestkit.ApplyOptionsForTesting(
			dbtestkit.WithDriver(newMockDriver("fake-dsn")),
			dbtestkit.WithSchemaInitializer(noopSchemaInit),
			dbtestkit.WithDataInitializer(noopDataInit),
			dbtestkit.WithEngineInitializer(nil),
		)
		require.Error(t, err)
	})
	t.Run("nil cache invalidator", func(t *testing.T) {
		_, err := dbtestkit.ApplyOptionsForTesting(
			append(validBaseOpts(),
				dbtestkit.WithCacheInvalidator(nil),
			)...,
		)
		require.Error(t, err)
	})
	t.Run("nil seeder in list", func(t *testing.T) {
		_, err := dbtestkit.ApplyOptionsForTesting(
			append(validBaseOpts(),
				dbtestkit.WithSeeders(nil),
			)...,
		)
		require.Error(t, err)
	})
}

// TestOptions_DefaultsApplied verifies that defaults fill in for unset
// fields when the user provides only required options.
func TestOptions_DefaultsApplied(t *testing.T) {
	cfg, err := dbtestkit.ApplyOptionsForTesting(validBaseOpts()...)
	require.NoError(t, err)

	require.Equal(t, "/tmp/dbtestkit-fake-project", cfg.ProjectRoot)
	require.Equal(t, "/tmp/dbtestkit-fake-testdata", cfg.TestdataDir)
	// Pristine dump path is derived from testdata dir + driver name.
	require.Contains(t, cfg.PristineDumpPath, "sqlite-pristine.sql")
}

// TestOptions_WithSeeders_Appends verifies that multiple WithSeeders calls
// accumulate rather than overwrite.
func TestOptions_WithSeeders_Appends(t *testing.T) {
	cfg, err := dbtestkit.ApplyOptionsForTesting(
		append(validBaseOpts(),
			dbtestkit.WithSeeders(func(*dbtestkit.Environment) error { return nil }),
			dbtestkit.WithSeeders(func(*dbtestkit.Environment) error { return nil }),
			dbtestkit.WithSeeders(func(*dbtestkit.Environment) error { return nil }),
		)...,
	)
	require.NoError(t, err)
	// 3 seeders should be present.
	require.Len(t, cfg.Seeders, 3)
}

// TestOptions_WithPristineDumpPath overrides the default.
func TestOptions_WithPristineDumpPath(t *testing.T) {
	cfg, err := dbtestkit.ApplyOptionsForTesting(
		append(validBaseOpts(),
			dbtestkit.WithPristineDumpPath("/custom/dump.sql"),
		)...,
	)
	require.NoError(t, err)
	require.Equal(t, "/custom/dump.sql", cfg.PristineDumpPath)
}

// TestOptions_WithGeneratePristine toggles the slow-path force flag.
func TestOptions_WithGeneratePristine(t *testing.T) {
	cfgOff, err := dbtestkit.ApplyOptionsForTesting(validBaseOpts()...)
	require.NoError(t, err)
	require.False(t, cfgOff.GeneratePristine)

	cfgOn, err := dbtestkit.ApplyOptionsForTesting(
		append(validBaseOpts(),
			dbtestkit.WithGeneratePristine(true),
		)...,
	)
	require.NoError(t, err)
	require.True(t, cfgOn.GeneratePristine)
}

// TestOptions_NilOptionRejected verifies that a nil option in the slice
// produces a descriptive error rather than a panic.
func TestOptions_NilOptionRejected(t *testing.T) {
	_, err := dbtestkit.ApplyOptionsForTesting(
		dbtestkit.WithDriver(newMockDriver("fake-dsn")),
		nil, // <- this should error
		dbtestkit.WithSchemaInitializer(noopSchemaInit),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

// TestOptions_WithDatabase_RejectsEmpty verifies that WithDatabase requires
// a non-empty Database name.
func TestOptions_WithDatabase_RejectsEmpty(t *testing.T) {
	_, err := dbtestkit.ApplyOptionsForTesting(
		append(validBaseOpts(),
			dbtestkit.WithDatabase(dbtestkit.DatabaseConfig{
				// Database intentionally empty
				Username: "root",
				Password: "pw",
				Image:    "mysql:lts",
			}),
		)...,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-empty Database")
}

// TestOptions_ApplicativeOrder verifies that later options win for scalar
// fields.
func TestOptions_ApplicativeOrder(t *testing.T) {
	cfg, err := dbtestkit.ApplyOptionsForTesting(
		append(validBaseOpts(),
			dbtestkit.WithTestdataDir("/first"),
			dbtestkit.WithTestdataDir("/second"),
		)...,
	)
	require.NoError(t, err)
	require.Equal(t, "/second", cfg.TestdataDir)
}

// TestOptions_NilLoggerAllowed verifies that WithLogger(nil) cleanly disables
// output rather than panicking on a nil interface call.
func TestOptions_NilLoggerAllowed(t *testing.T) {
	cfg, err := dbtestkit.ApplyOptionsForTesting(
		append(validBaseOpts(),
			dbtestkit.WithLogger(nil),
		)...,
	)
	require.NoError(t, err)
	require.Nil(t, cfg.Logger)
}
