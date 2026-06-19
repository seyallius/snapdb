// Package dbtestkit. lifecycle.go - Orchestrates the test lifecycle. Exposes
// two public entry points: Run (called from TestMain) and Reset (called from
// each test). Everything else is internal plumbing.
//
// The library uses a package-scoped global to share state between Run and
// Reset because Go's TestMain model is inherently package-scoped — the test
// binary boots one process per package, so a package-level global is the
// natural lifecycle boundary.
package snapdb

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// runtime is the package-scoped state shared between Run and Reset. It is
// populated by Run and consumed by Reset. Reset panics if Run was not called.
var (
	runtime *runtimeState
	resetMu sync.Mutex
)

// runtimeState holds everything Run builds so Reset can reuse it.
type runtimeState struct {
	cfg    *config
	env    *Environment
	engine Engine
	drv    DatabaseDriver
}

// MinimalTandB is the subset of *testing.T / *testing.B the library needs.
// Defined here so callers can pass either without the library depending on
// the testing package at compile time (other than in tests).
type MinimalTandB interface {
	Helper()
	Logf(format string, args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// -------------------------------------------- Public API ------------------------------------------ //

// Run wires up the test environment and executes the test binary.
//
// It MUST be called from a TestMain function — the os.Exit at the end is what
// TestMain expects. The options list must include at minimum:
//   - WithDriver
//   - WithSchemaInitializer
//   - WithDataInitializer
//   - WithEngineInitializer
//
// Run performs the one-time container setup (fast path if a pristine dump
// exists, slow path otherwise), then defers to m.Run() to actually execute
// tests. After tests finish, Run tears down the container and exits.
func Run(m RunM, opts ...Option) {
	cfg, err := applyOptions(opts)
	if err != nil {
		fmt.Printf("❌ dbtestkit: invalid configuration: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env, eng, drv, err := setup(ctx, cfg)
	if err != nil {
		fmt.Printf("❌ dbtestkit: setup failed: %v\n", err)
		os.Exit(1)
	}

	// Stash the runtime so Reset can find it.
	runtime = &runtimeState{
		cfg:    cfg,
		env:    env,
		engine: eng,
		drv:    drv,
	}

	// Run the tests.
	code := m.Run()

	// Tear down.
	if err := teardown(ctx, env, eng, drv); err != nil {
		fmt.Printf("❌ dbtestkit: teardown failed: %v\n", err)
	}

	os.Exit(code)
}

// RunM is the subset of *testing.M the library needs. Defined here so the
// core package does not need to import "testing" at compile time (other than
// in test files).
type RunM interface {
	Run() int
}

// Reset restores the database to its pristine state and runs any custom
// seeders. Call this at the top of every test that depends on a clean
// database.
//
// testNames is an optional list of human-readable labels (e.g. sub-test
// names) that will be echoed in the reset banner for log readability.
func Reset(t MinimalTandB, testNames ...string) error {
	if t != nil {
		t.Helper()
	}

	if runtime == nil {
		panic("dbtestkit: Reset called before Run (did you forget to wire TestMain?)")
	}

	// Serialize resets to prevent concurrent truncation/insertion races on
	// the shared database. The library is process-scoped: parallel tests in
	// the same package share one container.
	resetMu.Lock()
	defer resetMu.Unlock()

	return resetDatabase(t, runtime, testNames...)
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// setup performs the one-time package initialization: resolves the project
// root, boots the driver, runs the slow or fast path depending on whether a
// pristine dump exists, and constructs the user's engine.
func setup(ctx context.Context, cfg *config) (*Environment, Engine, DatabaseDriver, error) {
	if cfg.logger != nil {
		cfg.logger.Info("🚀 Test Container Setup")
	}
	totalStart := time.Now()

	// 1. Build the initial Environment. Engine is nil here — it gets
	//    populated after Start() returns the DSN and the user's
	//    EngineInitializer runs.
	env := &Environment{
		driver:      cfg.driverName,
		database:    cfg.database,
		projectRoot: cfg.projectRoot,
		testdataDir: cfg.testdataDir,
		sqlitePath:  cfg.sqlitePath,
		logger:      cfg.logger,
		ctx:         ctx,
	}

	// 2. Boot the container (or local file for SQLite).
	stepStart := time.Now()
	dsn, err := cfg.driverImpl.Start(ctx, env)
	if err != nil {
		return nil, nil, nil, err
	}
	env.dsn = dsn
	logStep(cfg, "Start Container", stepStart)

	// 3. Decide fast vs slow path.
	useFastPath := !cfg.generatePristine && fileExists(cfg.pristineDumpPath)

	if useFastPath {
		// FAST PATH: restore the existing pristine dump, then construct engine.
		stepStart = time.Now()
		if err := cfg.driverImpl.RestoreDump(ctx, env, cfg.pristineDumpPath); err != nil {
			return nil, nil, nil, fmt.Errorf("fast path: restore dump: %w", err)
		}
		logStep(cfg, "Restore Pristine Dump (Fast Path)", stepStart)
	} else {
		// SLOW PATH: run schema + data initializers, then generate the dump.
		stepStart = time.Now()
		if err := cfg.schemaInit(env); err != nil {
			return nil, nil, nil, fmt.Errorf("slow path: schema init: %w", err)
		}
		logStep(cfg, "Initialize Schema (Slow Path)", stepStart)

		stepStart = time.Now()
		if err := cfg.dataInit(env); err != nil {
			return nil, nil, nil, fmt.Errorf("slow path: data init: %w", err)
		}
		logStep(cfg, "Initialize Data (Slow Path)", stepStart)

		stepStart = time.Now()
		if err := cfg.driverImpl.GenerateDump(ctx, env, cfg.pristineDumpPath); err != nil {
			return nil, nil, nil, fmt.Errorf("slow path: generate dump: %w", err)
		}
		logStep(cfg, "Generate & Save Pristine Dump", stepStart)
	}

	// 4. Construct the user's engine.
	stepStart = time.Now()
	eng, err := cfg.engineInit(env)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("engine init: %w", err)
	}
	env.engine = eng
	logStep(cfg, "Initialize Engine", stepStart)

	// 5. Verify connectivity.
	stepStart = time.Now()
	if err := eng.Ping(); err != nil {
		return nil, nil, nil, fmt.Errorf("ping database: %w", err)
	}
	logStep(cfg, "Verify Database Connection", stepStart)

	logEnd(cfg, "Test Container Ready!", totalStart)
	return env, eng, cfg.driverImpl, nil
}

// teardown stops the container and closes the engine.
func teardown(ctx context.Context, env *Environment, eng Engine, drv DatabaseDriver) error {
	if eng != nil {
		if err := eng.Close(); err != nil && env.logger != nil {
			env.logger.Warn(fmt.Sprintf("failed to close engine: %v", err))
		}
	}
	if drv != nil {
		if err := drv.Stop(ctx, env); err != nil {
			return fmt.Errorf("stop driver: %w", err)
		}
	}
	return nil
}

// resetDatabase is the per-test reset. It invalidates caches, restores the
// pristine dump, runs the user's seeders, and verifies connectivity.
func resetDatabase(_ MinimalTandB, rt *runtimeState, testNames ...string) error {
	cfg := rt.cfg
	env := rt.env
	eng := rt.engine
	drv := rt.drv

	// Build a per-reset environment that exposes the live engine to user
	// callbacks (cache invalidator, seeders).
	runtimeEnv := env.withEngine(eng)

	var label string
	if len(testNames) > 0 {
		label = joinStrings(testNames, "|>")
	}
	if cfg.logger != nil {
		if label != "" {
			cfg.logger.Info(fmt.Sprintf("🔄 Reset Test Environment [%s]", label))
		} else {
			cfg.logger.Info("🔄 Reset Test Environment")
		}
	}
	totalStart := time.Now()

	// 1. Invalidate user caches BEFORE the restore so post-reset reads do
	//    not return stale pointers from the previous test.
	if cfg.cacheInvalidator != nil {
		stepStart := time.Now()
		if err := cfg.cacheInvalidator(runtimeEnv); err != nil {
			return fmt.Errorf("cache invalidator: %w", err)
		}
		logStep(cfg, "Invalidate Caches", stepStart)
	}

	// 2. Clear ORM-level cache if the engine exposes one.
	stepStart := time.Now()
	if err := eng.ClearCache(); err != nil {
		return fmt.Errorf("clear engine cache: %w", err)
	}
	logStep(cfg, "Clear Engine Cache", stepStart)

	// 3. Restore the pristine state. For MySQL/Postgres this pipes the dump
	//    back into the container; for SQLite this copies the snapshot file.
	stepStart = time.Now()
	if err := drv.RestoreDump(runtimeEnv.Context(), runtimeEnv, cfg.pristineDumpPath); err != nil {
		return fmt.Errorf("restore dump: %w", err)
	}
	logStep(cfg, "Restore Pristine State", stepStart)

	// 4. Run custom seeders.
	if len(cfg.seeders) > 0 {
		stepStart = time.Now()
		for i, seeder := range cfg.seeders {
			if err := seeder(runtimeEnv); err != nil {
				return fmt.Errorf("seeder %d: %w", i, err)
			}
		}
		logStep(cfg, "Run Custom Seeders", stepStart)
	}

	logEnd(cfg, "Reset Complete!", totalStart)
	return nil
}

// fileExists reports whether the given path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// logStep / logEnd forward to the configured logger, guarding against nil
// loggers (so WithLogger(nil) cleanly disables output).
func logStep(cfg *config, name string, start time.Time) {
	if cfg.logger == nil {
		return
	}
	cfg.logger.Step(name, time.Since(start))
}

func logEnd(cfg *config, name string, start time.Time) {
	if cfg.logger == nil {
		return
	}
	cfg.logger.End(name, time.Since(start))
}

// joinStrings joins a slice of strings with sep. A tiny helper to avoid
// pulling in strings.Join for one call site.
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
}
