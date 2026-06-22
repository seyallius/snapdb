// Package snapdb. testing.go - Exposes a minimal set of internal hooks for
// use by tests in this package and downstream consumers that need to assert
// option application without spinning up a real container.
//
// This file ships with the library (it is not a _test.go file) so the
// functions it exposes are part of the public API. They are clearly named
// "ForTesting" to discourage production use.
package snapdb

import "context"

// --------------------------------------------- Types ---------------------------------------------- //

// TestingConfig is a read-only view of the internal config struct, exposed
// for tests that need to assert what options were applied. Production code
// should never use this type.
type TestingConfig struct {
	DriverName       Driver
	Database         DatabaseConfig
	SchemaInit       SchemaInitializer
	DataInit         DataInitializer
	EngineInit       EngineInitializer
	CacheInvalidator CacheInvalidator
	Seeders          []Seeder
	PristineDumpPath string
	TestdataDir      string
	SQLitePath       string
	ProjectRoot      string
	GeneratePristine bool
	Logger           Logger
}

// ------------------------------------------- Constructor(s) --------------------------------------- //

// NewEnvironmentForTesting constructs an Environment with the minimal fields
// a driver needs to operate (driver name, testdata dir, sqlite path).
//
// If base is non-nil, its other fields (DSN, project root, logger) are
// inherited; otherwise sensible defaults are used. This is intended for
// driver-level tests that want to exercise Start/Stop/RestoreDump/Truncate
// in isolation without invoking the full lifecycle.
func NewEnvironmentForTesting(base *Environment, driverName Driver, testdataDir, sqlitePath string) *Environment {
	env := &Environment{
		driver:      driverName,
		testdataDir: testdataDir,
		sqlitePath:  sqlitePath,
		logger:      NewDefaultLogger(nil),
	}
	if base != nil {
		env.database = base.database
		env.dsn = base.dsn
		env.projectRoot = base.projectRoot
		env.ctx = base.ctx
		env.engine = base.engine
	}
	return env
}

// -------------------------------------------- Public API ------------------------------------------ //

// ApplyOptionsForTesting builds a TestingConfig from a list of options.
//
// This is the only sanctioned way for tests to inspect option application
// without reimplementing the validation logic. Production callers should
// use Run, which performs the same validation internally.
//
// Returns an error if the options fail validation (missing required fields,
// nil callbacks, etc.).
func ApplyOptionsForTesting(opts ...Option) (*TestingConfig, error) {
	cfg, err := applyOptions(opts)
	if err != nil {
		return nil, err
	}
	return &TestingConfig{
		DriverName:       cfg.driverName,
		Database:         cfg.database,
		SchemaInit:       cfg.schemaInit,
		DataInit:         cfg.dataInit,
		EngineInit:       cfg.engineInit,
		CacheInvalidator: cfg.cacheInvalidator,
		Seeders:          cfg.seeders,
		PristineDumpPath: cfg.pristineDumpPath,
		TestdataDir:      cfg.testdataDir,
		SQLitePath:       cfg.sqlitePath,
		ProjectRoot:      cfg.projectRoot,
		GeneratePristine: cfg.generatePristine,
		Logger:           cfg.logger,
	}, nil
}

// SetupForTesting runs the package-internal setup function with a config
// derived from the given options. It returns the constructed Environment,
// Engine, and the resolved DatabaseDriver, exactly as the production Run
// function would see them.
//
// Tests use this to exercise the fast/slow-path logic without invoking
// os.Exit (which Run does and which makes direct testing impossible).
func SetupForTesting(opts ...Option) (*Environment, Engine, DatabaseDriver, error) {
	cfg, err := applyOptions(opts)
	if err != nil {
		return nil, nil, nil, err
	}
	ctx := context.Background()
	return setup(ctx, cfg)
}

// ResetForTesting runs the package-internal resetDatabase function against
// a runtime state assembled from the given options + a previously-returned
// Environment/Engine/Driver triple. It does NOT call os.Exit and is safe
// to use from test code.
//
// seeders and cacheInvalidator override whatever was supplied via opts.
func ResetForTesting(
	t MinimalTandB,
	opts ...Option,
) error {
	cfg, err := applyOptions(opts)
	if err != nil {
		return err
	}
	ctx := context.Background()
	env, eng, drv, err := setup(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = teardown(ctx, env, eng, drv) }()

	rt := &runtimeState{cfg: cfg, env: env, engine: eng, drv: drv}
	return resetDatabase(t, rt)
}

// EnvWithEngineForTesting returns a shallow copy of env with the Engine
// field replaced. Used by driver tests to inject a test-specific Engine
// before calling Truncate.
func EnvWithEngineForTesting(env *Environment, eng Engine) *Environment {
	if env == nil {
		return nil
	}
	return env.withEngine(eng)
}
