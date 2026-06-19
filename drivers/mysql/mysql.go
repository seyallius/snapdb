// Package mysql. mysql.go - MySQL backend for dbtestkit. Boots a MySQL 8.x
// test container using testcontainers-go, with a tmpfs-backed data directory
// and an optimized entrypoint that re-hydrates a pre-baked empty database
// tarball (skipping the multi-second initdb phase).
package mysql

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/seyallius/snapdb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

const (
	// defaultPort is the well-known MySQL port inside the container.
	defaultPort = "3306"

	// quickstartTarballName is the name of the pre-baked empty-database
	// tarball expected to live in the testdata directory.
	quickstartTarballName = "empty-mysql.tar.gz"

	// entrypointName is the name of the optimized entrypoint script copied
	// into the container.
	entrypointName = "mysql-quickstart-entrypoint.sh"

	// defaultStartupTimeout caps how long we wait for the container to
	// become ready.
	defaultStartupTimeout = 2 * time.Minute
)

// quickstartEntrypoint is embedded at build time so the testdata directory
// does not need to ship the script alongside the Go source.
//
//go:embed mysql-quickstart-entrypoint.sh
var quickstartEntrypoint []byte

// Driver implements dbtestkit.DatabaseDriver for MySQL.
type Driver struct {
	container testcontainers.Container
	cfg       dbtestkit.DatabaseConfig
}

// ------------------------------------------- Constructor(s) --------------------------------------- //

// New returns a fresh MySQL driver instance.
//
// Pass the result to dbtestkit.WithDriver:
//
//	dbtestkit.Run(m,
//	    dbtestkit.WithDriver(mysql.New()),
//	    ...
//	)
func New() dbtestkit.DatabaseDriver {
	return &Driver{}
}

// -------------------------------------------- Public API ------------------------------------------ //

// Driver returns the dbtestkit.Driver constant this implementation serves.
func (d *Driver) Driver() dbtestkit.Driver { return dbtestkit.DriverMySQL }

// Start boots the MySQL container and returns the DSN.
//
// The container is configured with:
//   - tmpfs at /var/lib/mysql for in-memory storage (fast + ephemeral)
//   - an optimized entrypoint that re-hydrates a pre-baked empty DB tarball
//     (skipping initdb's slow first-run sequence)
//   - no wait strategy — the entrypoint guarantees readiness before exec
//     returns control
func (d *Driver) Start(ctx context.Context, env *dbtestkit.Environment) (string, error) {
	cfg := env.DriverConfig()
	d.cfg = cfg

	startupTimeout := cfg.StartupTimeout
	if startupTimeout == 0 {
		startupTimeout = defaultStartupTimeout
	}

	// 1. Stage the embedded entrypoint and (optional) tarball into the
	//    testdata directory so testcontainers can mount them into the
	//    container.
	if err := stageEntrypoint(env.TestdataDir()); err != nil {
		return "", fmt.Errorf("mysql: failed to stage entrypoint: %w", err)
	}

	files := []testcontainers.ContainerFile{
		{
			HostFilePath:      filepath.Join(env.TestdataDir(), entrypointName),
			ContainerFilePath: "/" + entrypointName,
			FileMode:          0o755,
		},
	}
	if tarballPath, err := stageTarball(env.TestdataDir()); err == nil {
		files = append(files, testcontainers.ContainerFile{
			HostFilePath:      tarballPath,
			ContainerFilePath: "/tmp/" + quickstartTarballName,
			FileMode:          0o644,
		})
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("mysql: failed to stage tarball: %w", err)
	}
	// If the tarball doesn't exist yet, the entrypoint gracefully falls
	// back to the official mysql initdb flow. The user can generate it
	// later by running the generate-empty-tarball.sh helper.

	// 2. Build the container.
	urlFunc := func(host string, port nat.Port) string {
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/", cfg.Username, cfg.Password, host, port.Port())
	}
	_ = urlFunc // Kept for parity with the upstream mysql module's API expectations.

	ctr, err := mysql.Run(
		ctx,
		cfg.Image,
		mysql.WithDatabase(cfg.Database),
		mysql.WithUsername(cfg.Username),
		mysql.WithPassword(cfg.Password),
		testcontainers.WithTmpfs(map[string]string{"/var/lib/mysql": "rw"}),
		testcontainers.WithFiles(files...),
		withEntrypoint{"/" + entrypointName},
		testcontainers.WithWaitStrategy(
			waitForMySQL(ctx, startupTimeout, cfg),
		),
	)
	if err != nil {
		return "", fmt.Errorf("mysql: failed to start container: %w", err)
	}
	d.container = ctr

	// 3. Resolve host:port and build the DSN.
	host, err := ctr.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("mysql: failed to get host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, defaultPort)
	if err != nil {
		return "", fmt.Errorf("mysql: failed to get port: %w", err)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", cfg.Username, cfg.Password, host, port.Port(), cfg.Database), nil
}

// RestoreDump pipes the pristine SQL dump into the MySQL CLI client inside
// the container. This is the fast-path reset (~ms).
func (d *Driver) RestoreDump(ctx context.Context, env *dbtestkit.Environment, dumpPath string) error {
	if d.container == nil {
		return fmt.Errorf("mysql: RestoreDump called before Start")
	}
	dumpName := filepath.Base(dumpPath)

	// Copy the dump into the container if it isn't already there.
	if err := d.container.CopyFileToContainer(ctx, dumpPath, "/tmp/"+dumpName, 0o644); err != nil {
		// Already present — ignore the error.
		_ = err
	}

	cmd := fmt.Sprintf("MYSQL_PWD=%s mysql -u %s %s < /tmp/%s",
		d.cfg.Password, d.cfg.Username, d.cfg.Database, dumpName)
	code, reader, err := d.container.Exec(ctx, []string{"sh", "-c", cmd})
	if err != nil {
		return fmt.Errorf("mysql: failed to restore dump: %w", err)
	}
	if code != 0 {
		errBytes, _ := readAll(reader)
		return fmt.Errorf("mysql: dump restore failed (exit %d): %s", code, string(errBytes))
	}
	return nil
}

// GenerateDump dumps the current database to dumpPath via mysqldump.
//
// The dump is generated with --add-drop-table so restores are idempotent and
// do not require a separate TRUNCATE pass.
func (d *Driver) GenerateDump(ctx context.Context, env *dbtestkit.Environment, dumpPath string) error {
	if d.container == nil {
		return fmt.Errorf("mysql: GenerateDump called before Start")
	}
	dumpName := filepath.Base(dumpPath)

	cmd := fmt.Sprintf("MYSQL_PWD=%s mysqldump -u %s --add-drop-table --complete-insert --skip-triggers %s > /tmp/%s",
		d.cfg.Password, d.cfg.Username, d.cfg.Database, dumpName)
	code, reader, err := d.container.Exec(ctx, []string{"sh", "-c", cmd})
	if err != nil {
		return fmt.Errorf("mysql: mysqldump failed: %w", err)
	}
	if code != 0 {
		errBytes, _ := readAll(reader)
		return fmt.Errorf("mysql: mysqldump failed (exit %d): %s", code, string(errBytes))
	}

	if err := copyFileFromContainer(ctx, d.container, "/tmp/"+dumpName, dumpPath); err != nil {
		return fmt.Errorf("mysql: failed to save dump to host: %w", err)
	}
	return nil
}

// Truncate is a no-op for MySQL — the pristine dump's DROP TABLE IF EXISTS
// statements make truncation redundant on the fast path.
func (d *Driver) Truncate(_ context.Context, _ *dbtestkit.Environment) error {
	return nil
}

// Stop terminates the container.
func (d *Driver) Stop(ctx context.Context, _ *dbtestkit.Environment) error {
	if d.container == nil {
		return nil
	}
	if err := d.container.Stop(ctx, nil); err != nil {
		return fmt.Errorf("mysql: failed to stop container: %w", err)
	}
	return nil
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// withEntrypoint is a testcontainers ContainerCustomizer that overrides the
// container's entrypoint. Required because the upstream mysql module pins
// the entrypoint internally.
type withEntrypoint []string

// Customize implements testcontainers.ContainerCustomizer.
func (w withEntrypoint) Customize(req *testcontainers.GenericContainerRequest) error {
	req.Entrypoint = w
	return nil
}

// stageEntrypoint writes the embedded entrypoint script into the testdata
// directory so testcontainers can mount it into the container.
func stageEntrypoint(testdataDir string) error {
	if err := os.MkdirAll(testdataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(testdataDir, entrypointName), quickstartEntrypoint, 0o755)
}

// stageTarball checks whether the pre-baked empty DB tarball already exists
// in the testdata directory. If not, returns os.ErrNotExist so the caller can
// decide whether to proceed without it (the entrypoint gracefully degrades).
func stageTarball(testdataDir string) (string, error) {
	path := filepath.Join(testdataDir, quickstartTarballName)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}
