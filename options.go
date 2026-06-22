// Package dbtestkit. options.go - Defines the functional-options API used to
// configure a test environment.
//
// All configuration is supplied via WithXxx options — there are no environment
// variables. This keeps tests hermetic, makes the configuration discoverable
// via code completion, and avoids global state.
package snapdb

import (
	"fmt"
	"time"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// DatabaseConfig holds the credentials and image selection for a containerized
// database. Fields not relevant to a given driver (e.g. Image for SQLite) are
// ignored.
type DatabaseConfig struct {
	// Database is the name of the database to create inside the container.
	Database string

	// Username is the database user the engine will connect as.
	Username string

	// Password is the database user's password.
	Password string

	// Image is the Docker image reference, e.g. "mysql:lts" or "postgres:16-alpine".
	Image string

	// StartupTimeout is the maximum time to wait for the container to become
	// ready. Zero means use the driver default.
	StartupTimeout time.Duration
}

// Option configures the test environment.
type Option func(*config) error

// SchemaInitializer is invoked once on the slow path to create tables.
// It receives the live Environment (engine may be nil if the user chose to
// construct the engine after schema sync — see WithEngineInitializer).
type SchemaInitializer func(env *Environment) error

// DataInitializer is invoked once on the slow path to seed base data.
type DataInitializer func(env *Environment) error

// EngineInitializer constructs the user's Engine from the DSN exposed on env.
// Called once per package setup; the returned Engine is reused for all tests.
type EngineInitializer func(env *Environment) (Engine, error)

// CacheInvalidator is invoked before every reset to flush ORM-level caches.
// Return nil if your ORM has no cache.
type CacheInvalidator func(env *Environment) error

// Seeder is invoked after the pristine state has been restored, on every reset.
// Use it to insert test-specific fixtures.
type Seeder func(env *Environment) error

// config is the internal, mutable representation assembled from options.
// It is never exposed to callers.
type config struct {
	driverImpl       DatabaseDriver
	driverName       Driver
	database         DatabaseConfig
	schemaInit       SchemaInitializer
	dataInit         DataInitializer
	engineInit       EngineInitializer
	cacheInvalidator CacheInvalidator
	seeders          []Seeder
	pristineDumpPath string
	testdataDir      string
	sqlitePath       string
	projectRoot      string
	generatePristine bool
	logger           Logger
}

// defaultConfig returns a config pre-populated with sane defaults. Required
// fields (driver, schemaInit, dataInit, engineInit) are still zero and will
// be validated by validate().
func defaultConfig() *config {
	return &config{
		logger:   NewDefaultLogger(nil),
		database: DatabaseConfig{},
	}
}

// -------------------------------------------- Public API ------------------------------------------ //

// WithDriver supplies the database driver implementation.
//
// Deprecated: pass the driver as the second positional argument to Run
// instead — that form is checked by the compiler, so a missing driver is a
// build error instead of a runtime "WithDriver is required" failure. This
// option is kept only so existing call sites continue to compile; the last
// value set (whether by this option or by Run's positional argument) wins.
//
// Pass the constructor result from the desired driver subpackage:
//
//	import "github.com/seyallius/snapdb/drivers/mysql"
//
//	dbtestkit.Run(m, mysql.New(), schemaInit, dataInit, engineInit,
//	    dbtestkit.WithDatabase(dbCfg),
//	)
func WithDriver(d DatabaseDriver) Option {
	return func(c *config) error {
		if d == nil {
			return fmt.Errorf("dbtestkit: WithDriver requires a non-nil driver")
		}
		c.driverImpl = d
		c.driverName = d.Driver()
		if !c.driverName.IsValid() {
			return fmt.Errorf("dbtestkit: driver %q reported an unsupported identifier (valid: %v)",
				c.driverName, SupportedDrivers())
		}
		return nil
	}
}

// WithDatabase configures credentials and image for the containerized database.
// Required for MySQL and Postgres; ignored for SQLite.
func WithDatabase(db DatabaseConfig) Option {
	return func(c *config) error {
		if db.Database == "" {
			l := defaultConfig().logger
			l.Warn("dbtestkit: WithDatabase provided an empty Database name")
		}
		c.database = db
		return nil
	}
}

// WithSchemaInitializer supplies the callback that creates tables.
//
// Deprecated: pass it as Run's third positional argument instead; see
// WithDriver's doc comment for why. Kept for backward compatibility.
//
// For xorm users this is typically engine.Sync2(&MyModel{}).
func WithSchemaInitializer(fn SchemaInitializer) Option {
	return func(c *config) error {
		if fn == nil {
			return fmt.Errorf("dbtestkit: WithSchemaInitializer requires a non-nil callback")
		}
		c.schemaInit = fn
		return nil
	}
}

// WithDataInitializer supplies the callback that seeds base data on the slow
// path.
//
// Deprecated: pass it as Run's fourth positional argument instead. Kept for
// backward compatibility.
func WithDataInitializer(fn DataInitializer) Option {
	return func(c *config) error {
		if fn == nil {
			return fmt.Errorf("dbtestkit: WithDataInitializer requires a non-nil callback")
		}
		c.dataInit = fn
		return nil
	}
}

// WithEngineInitializer supplies the callback that builds the Engine from the
// DSN exposed on the Environment.
//
// Deprecated: pass it as Run's fifth positional argument instead. Kept for
// backward compatibility.
func WithEngineInitializer(fn EngineInitializer) Option {
	return func(c *config) error {
		if fn == nil {
			return fmt.Errorf("dbtestkit: WithEngineInitializer requires a non-nil callback")
		}
		c.engineInit = fn
		return nil
	}
}

// WithCacheInvalidator supplies an optional callback invoked before every reset.
//
// Use this to flush ORM-level caches (e.g. xorm's Ristretto store) so that
// post-reset reads do not return stale pointers from the previous test.
func WithCacheInvalidator(fn CacheInvalidator) Option {
	return func(c *config) error {
		if fn == nil {
			return fmt.Errorf("dbtestkit: WithCacheInvalidator requires a non-nil callback")
		}
		c.cacheInvalidator = fn
		return nil
	}
}

// WithSeeders appends custom seeder functions executed after the pristine
// state is restored on every reset. Repeatable; later calls append.
func WithSeeders(seeders ...Seeder) Option {
	return func(c *config) error {
		for _, s := range seeders {
			if s == nil {
				return fmt.Errorf("dbtestkit: WithSeeders contains a nil seeder")
			}
		}
		c.seeders = append(c.seeders, seeders...)
		return nil
	}
}

// WithPristineDumpPath overrides the location of the pristine SQL dump file.
//
// Defaults to <testdataDir>/<driver>-pristine.sql.
func WithPristineDumpPath(path string) Option {
	return func(c *config) error {
		if path == "" {
			return fmt.Errorf("dbtestkit: WithPristineDumpPath requires a non-empty path")
		}
		c.pristineDumpPath = path
		return nil
	}
}

// WithTestdataDir overrides the testdata directory used to store auxiliary
// files (pristine dumps, pre-baked container tarballs, etc.).
//
// Defaults to <projectRoot>/testdata.
func WithTestdataDir(dir string) Option {
	return func(c *config) error {
		if dir == "" {
			return fmt.Errorf("dbtestkit: WithTestdataDir requires a non-empty path")
		}
		c.testdataDir = dir
		return nil
	}
}

// WithSQLitePath overrides the on-disk location of the SQLite database file.
// SQLite driver only. Defaults to <testdataDir>/dbtestkit.sqlite.
func WithSQLitePath(path string) Option {
	return func(c *config) error {
		if path == "" {
			return fmt.Errorf("dbtestkit: WithSQLitePath requires a non-empty path")
		}
		c.sqlitePath = path
		return nil
	}
}

// WithProjectRoot explicitly sets the project root directory, bypassing the
// automatic go.mod lookup. Useful in environments where the working directory
// is not inside the module tree.
func WithProjectRoot(path string) Option {
	return func(c *config) error {
		if path == "" {
			return fmt.Errorf("dbtestkit: WithProjectRoot requires a non-empty path")
		}
		c.projectRoot = path
		return nil
	}
}

// WithGeneratePristine forces the slow path on the next setup, regenerating
// the pristine dump file even if one already exists. Useful as a one-shot
// command (e.g. invoked by a `make pristine` target).
func WithGeneratePristine(force bool) Option {
	return func(c *config) error {
		c.generatePristine = force
		return nil
	}
}

// WithLogger replaces the default logger. Pass nil to disable output entirely.
func WithLogger(l Logger) Option {
	return func(c *config) error {
		// nil is allowed — it disables logging.
		c.logger = l
		return nil
	}
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// applyOptions builds a validated config from a list of options.
//
// Options are applied in order; later options win for scalar fields and
// append for slice fields (WithSeeders). Validation runs at the end so that
// "required" errors surface even if the user forgot the corresponding option
// entirely.
func applyOptions(opts []Option) (*config, error) {
	c := defaultConfig()
	for i, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("dbtestkit: option at index %d is nil", i)
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// validate enforces required fields and fills in derived defaults.
func (c *config) validate() error {
	if c.driverImpl == nil {
		return fmt.Errorf("dbtestkit: WithDriver is required")
	}
	if !c.driverName.IsValid() {
		return fmt.Errorf("dbtestkit: driver reported invalid identifier %q (valid: %v)",
			c.driverName, SupportedDrivers())
	}
	if c.schemaInit == nil {
		return fmt.Errorf("dbtestkit: WithSchemaInitializer is required")
	}
	if c.dataInit == nil {
		return fmt.Errorf("dbtestkit: WithDataInitializer is required")
	}
	if c.engineInit == nil {
		return fmt.Errorf("dbtestkit: WithEngineInitializer is required")
	}

	// Apply driver-specific defaults.
	switch c.driverName {
	case DriverMySQL:
		if c.database == (DatabaseConfig{}) {
			c.database = DatabaseConfig{
				Database: "testdb",
				Username: "root",
				Password: "testpass",
				Image:    "mysql:lts",
			}
		}
		if c.database.Image == "" {
			c.database.Image = "mysql:lts"
		}
	case DriverPostgres:
		if c.database == (DatabaseConfig{}) {
			c.database = DatabaseConfig{
				Database: "testdb",
				Username: "postgres",
				Password: "testpass",
				Image:    "postgres:16-alpine",
			}
		}
		if c.database.Image == "" {
			c.database.Image = "postgres:16-alpine"
		}
	case DriverSQLite:
		// SQLite needs no credentials.
	}

	// Resolve project root if not explicitly set.
	if c.projectRoot == "" {
		root, err := findProjectRoot()
		if err != nil {
			return err
		}
		c.projectRoot = root
	}

	// Resolve testdata dir if not explicitly set.
	if c.testdataDir == "" {
		c.testdataDir = c.projectRoot + "/testdata"
	}

	// Resolve pristine dump path if not explicitly set.
	if c.pristineDumpPath == "" {
		c.pristineDumpPath = c.testdataDir + "/" + string(c.driverName) + "-pristine.sql"
	}

	// Resolve SQLite path if not explicitly set.
	if c.driverName == DriverSQLite && c.sqlitePath == "" {
		c.sqlitePath = c.testdataDir + "/dbtestkit.sqlite"
	}

	return nil
}
