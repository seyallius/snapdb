// Package snapdb_test. lifecycle_test.go - Unit tests for the lifecycle
// orchestration (setup fast vs slow path, reset sequence). Uses the mock
// driver so no Docker is required.
package snapdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/seyallius/snapdb"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------- Tests -------------------------------------------- //

// TestLifecycle_SlowPath verifies that when no pristine dump exists, setup
// calls schemaInit + dataInit + GenerateDump, in that order.
func TestLifecycle_SlowPath(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	var schemaCalled, dataCalled bool

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error {
			schemaCalled = true
			return nil
		}),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error {
			dataCalled = true
			return nil
		}),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(filepath.Join(tmp, "nonexistent.sql")),
	}

	_, _, _, err := snapdb.SetupForTesting(opts...)
	require.NoError(t, err)

	require.True(t, schemaCalled, "schemaInit should be called on slow path")
	require.True(t, dataCalled, "dataInit should be called on slow path")

	calls := mock.Calls()
	require.Contains(t, calls, "Start")
	require.Contains(t, calls, "GenerateDump")
	// On slow path, RestoreDump is NOT called during setup.
	require.NotContains(t, calls, "RestoreDump")
	// Stop is NOT called by SetupForTesting — it returns the live resources
	// to the caller (so they can drive additional Reset calls). Production
	// Run() calls teardown after m.Run().
	require.NotContains(t, calls, "Stop")
}

// TestLifecycle_FastPath verifies that when a pristine dump exists, setup
// calls RestoreDump instead of schemaInit + dataInit + GenerateDump.
func TestLifecycle_FastPath(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	// Pre-create the pristine dump file so the fast path triggers.
	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	var schemaCalled, dataCalled bool

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error {
			schemaCalled = true
			return nil
		}),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error {
			dataCalled = true
			return nil
		}),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
	}

	_, _, _, err := snapdb.SetupForTesting(opts...)
	require.NoError(t, err)

	require.False(t, schemaCalled, "schemaInit must NOT be called on fast path")
	require.False(t, dataCalled, "dataInit must NOT be called on fast path")

	calls := mock.Calls()
	require.Contains(t, calls, "Start")
	require.Contains(t, calls, "RestoreDump")
	// On fast path, GenerateDump is NOT called during setup.
	require.NotContains(t, calls, "GenerateDump")
}

// TestLifecycle_WithGeneratePristine_ForcesSlowPath verifies that
// WithGeneratePristine(true) takes the slow path even if a dump exists.
func TestLifecycle_WithGeneratePristine_ForcesSlowPath(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	// Pre-create the dump.
	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	var schemaCalled, dataCalled bool

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error {
			schemaCalled = true
			return nil
		}),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error {
			dataCalled = true
			return nil
		}),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
		snapdb.WithGeneratePristine(true),
	}

	_, _, _, err := snapdb.SetupForTesting(opts...)
	require.NoError(t, err)

	require.True(t, schemaCalled, "schemaInit should be called when generatePristine=true")
	require.True(t, dataCalled, "dataInit should be called when generatePristine=true")

	calls := mock.Calls()
	require.Contains(t, calls, "GenerateDump")
	require.NotContains(t, calls, "RestoreDump")
}

// TestLifecycle_ResetSequence verifies that Reset invokes cache invalidator,
// ClearCache, RestoreDump, and seeders in the correct order.
func TestLifecycle_ResetSequence(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	// Pre-create the dump so setup is fast.
	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	var invalidatorCalled, seederCalled bool

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithCacheInvalidator(func(*snapdb.Environment) error {
			invalidatorCalled = true
			return nil
		}),
		snapdb.WithSeeders(func(*snapdb.Environment) error {
			seederCalled = true
			return nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
		snapdb.WithLogger(nil), // quiet
	}

	err := snapdb.ResetForTesting(noopT{}, opts...)
	require.NoError(t, err)

	require.True(t, invalidatorCalled, "cache invalidator must be called on reset")
	require.True(t, seederCalled, "seeder must be called on reset")
}

// TestLifecycle_ResetWithoutSeeders verifies that a reset with no custom
// seeders still completes successfully.
func TestLifecycle_ResetWithoutSeeders(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
		snapdb.WithLogger(nil),
	}

	err := snapdb.ResetForTesting(noopT{}, opts...)
	require.NoError(t, err)
}

// TestLifecycle_ResetWithoutCacheInvalidator verifies that omitting the
// cache invalidator does not break reset (it's optional).
func TestLifecycle_ResetWithoutCacheInvalidator(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
		snapdb.WithLogger(nil),
	}

	err := snapdb.ResetForTesting(noopT{}, opts...)
	require.NoError(t, err)
}

// TestLifecycle_DSNExposedToEngineInit verifies that the DSN returned by
// driver.Start is visible to the user's EngineInitializer via env.DSN().
func TestLifecycle_DSNExposedToEngineInit(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("custom-dsn-12345")

	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	var seenDSN string
	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			seenDSN = env.DSN()
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
		snapdb.WithLogger(nil),
	}

	_, _, _, err := snapdb.SetupForTesting(opts...)
	require.NoError(t, err)
	require.Equal(t, "custom-dsn-12345", seenDSN)
}

// TestLifecycle_EngineExposedToResetCallbacks verifies that the engine
// constructed by EngineInitializer is visible to seeders and cache
// invalidators via env.Engine().
func TestLifecycle_EngineExposedToResetCallbacks(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	var invalidatorSawEngine, seederSawEngine bool

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithCacheInvalidator(func(env *snapdb.Environment) error {
			invalidatorSawEngine = env.Engine() != nil
			return nil
		}),
		snapdb.WithSeeders(func(env *snapdb.Environment) error {
			seederSawEngine = env.Engine() != nil
			return nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
		snapdb.WithLogger(nil),
	}

	err := snapdb.ResetForTesting(noopT{}, opts...)
	require.NoError(t, err)
	require.True(t, invalidatorSawEngine, "cache invalidator must see the engine")
	require.True(t, seederSawEngine, "seeder must see the engine")
}

// TestLifecycle_SchemaInitSeesNilEngine verifies that during the slow path,
// schemaInit is invoked BEFORE the engine is constructed — so env.Engine()
// returns nil at that point.
func TestLifecycle_SchemaInitSeesNilEngine(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	var schemaSawEngine bool

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(env *snapdb.Environment) error {
			schemaSawEngine = env.Engine() != nil
			return nil
		}),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(filepath.Join(tmp, "nonexistent.sql")),
		snapdb.WithLogger(nil),
	}

	_, _, _, err := snapdb.SetupForTesting(opts...)
	require.NoError(t, err)
	require.False(t, schemaSawEngine,
		"schemaInit runs before engine init; env.Engine() should be nil")
}

// TestLifecycle_ResetForTesting_TearsDown verifies that ResetForTesting
// drives the full lifecycle including teardown — i.e. Stop is called on the
// driver after the reset completes. This complements TestLifecycle_SlowPath,
// which uses SetupForTesting (no teardown).
func TestLifecycle_ResetForTesting_TearsDown(t *testing.T) {
	tmp := t.TempDir()
	mock := newMockDriver("fake-dsn")

	dumpPath := filepath.Join(tmp, "existing.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- fake"), 0o644))

	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(env *snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
		snapdb.WithPristineDumpPath(dumpPath),
		snapdb.WithLogger(nil),
	}

	err := snapdb.ResetForTesting(noopT{}, opts...)
	require.NoError(t, err)

	calls := mock.Calls()
	require.Contains(t, calls, "Start", "ResetForTesting must boot the driver")
	require.Contains(t, calls, "RestoreDump", "ResetForTesting must restore the dump")
	require.Contains(t, calls, "Stop", "ResetForTesting must tear down the driver after reset")
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// noopT is a minimal implementation of snapdb.MinimalTandB for use in
// tests where we don't care about log output.
type noopT struct{}

func (noopT) Helper()                           {}
func (noopT) Logf(format string, args ...any)   {}
func (noopT) Errorf(format string, args ...any) {}
func (noopT) Fatalf(format string, args ...any) {}
