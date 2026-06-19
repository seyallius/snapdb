// Package postgres. postgres.go - PostgreSQL backend for dbtestkit. Boots a
// Postgres test container via testcontainers-go, using tmpfs for the data
// directory and a tuned configuration for fast test execution.
package postgres

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/seyallius/snapdb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

const (
	// defaultPort is the well-known PostgreSQL port inside the container.
	defaultPort = "5432"

	// defaultStartupTimeout caps how long we wait for the container to
	// become ready.
	defaultStartupTimeout = 2 * time.Minute
)

// PostgresDriver implements dbtestkit.DatabaseDriver for PostgreSQL.
type PostgresDriver struct {
	container testcontainers.Container
	cfg       dbtestkit.DatabaseConfig
}

// ------------------------------------------- Constructor(s) --------------------------------------- //

// New returns a fresh Postgres driver instance.
//
// Pass the result to dbtestkit.WithDriver:
//
//	dbtestkit.Run(m,
//	    dbtestkit.WithDriver(postgres.New()),
//	    ...
//	)
func New() dbtestkit.DatabaseDriver { return &PostgresDriver{} }

// -------------------------------------------- Public API ------------------------------------------ //

// Driver returns the dbtestkit.Driver constant this implementation serves.
func (d *PostgresDriver) Driver() dbtestkit.Driver { return dbtestkit.DriverPostgres }

// Start boots the Postgres container and returns the DSN.
//
// The container is configured with:
//   - tmpfs at /var/lib/postgresql/data for in-memory storage
//   - a wait strategy that polls the postgres CLI until the server responds
//   - the standard postgres initdb flow (database + user created from
//     POSTGRES_DB / POSTGRES_USER / POSTGRES_PASSWORD env vars)
func (d *PostgresDriver) Start(ctx context.Context, env *dbtestkit.Environment) (string, error) {
	cfg := env.DriverConfig()
	d.cfg = cfg

	startupTimeout := cfg.StartupTimeout
	if startupTimeout == 0 {
		startupTimeout = defaultStartupTimeout
	}

	// Postgres doesn't ship a quickstart tarball equivalent of MySQL's
	// empty-mysql.tar.gz — initdb runs in ~1s on tmpfs, which is already
	// fast enough. If you need even faster startup, pre-bake a tarball of
	// /var/lib/postgresql/data and mount it the same way mysql does.

	ctr, err := postgres.Run(
		ctx,
		cfg.Image,
		postgres.WithDatabase(cfg.Database),
		postgres.WithUsername(cfg.Username),
		postgres.WithPassword(cfg.Password),
		testcontainers.WithTmpfs(map[string]string{"/var/lib/postgresql/data": "rw"}),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(startupTimeout),
				wait.ForListeningPort(defaultPort).
					WithStartupTimeout(startupTimeout),
			),
		),
	)
	if err != nil {
		return "", fmt.Errorf("postgres: failed to start container: %w", err)
	}
	d.container = ctr

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("postgres: failed to get host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, defaultPort)
	if err != nil {
		return "", fmt.Errorf("postgres: failed to get port: %w", err)
	}

	// libpq-style DSN — works with pgx, lib/pq, and xorm's postgres driver.
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port.Port(), cfg.Username, cfg.Password, cfg.Database), nil
}

// RestoreDump pipes the pristine SQL dump into psql inside the container.
// This is the fast-path reset (~ms).
func (d *PostgresDriver) RestoreDump(ctx context.Context, env *dbtestkit.Environment, dumpPath string) error {
	if d.container == nil {
		return fmt.Errorf("postgres: RestoreDump called before Start")
	}
	dumpName := filepath.Base(dumpPath)

	// Copy the dump into the container if it isn't already there.
	if err := d.container.CopyFileToContainer(ctx, dumpPath, "/tmp/"+dumpName, 0o644); err != nil {
		_ = err // already present
	}

	cmd := fmt.Sprintf("PGPASSWORD=%s psql -U %s -d %s -v ON_ERROR_STOP=1 -f /tmp/%s",
		d.cfg.Password, d.cfg.Username, d.cfg.Database, dumpName)
	code, reader, err := d.container.Exec(ctx, []string{"sh", "-c", cmd})
	if err != nil {
		return fmt.Errorf("postgres: failed to restore dump: %w", err)
	}
	if code != 0 {
		errBytes, _ := readAll(reader)
		return fmt.Errorf("postgres: dump restore failed (exit %d): %s", code, string(errBytes))
	}
	return nil
}

// GenerateDump writes a fresh pristine dump to dumpPath via pg_dump.
//
// Uses --clean --if-exists so the dump contains DROP TABLE IF EXISTS
// equivalents (DROP ... IF EXISTS), making restores idempotent without
// requiring a separate TRUNCATE pass.
func (d *PostgresDriver) GenerateDump(ctx context.Context, env *dbtestkit.Environment, dumpPath string) error {
	if d.container == nil {
		return fmt.Errorf("postgres: GenerateDump called before Start")
	}
	dumpName := filepath.Base(dumpPath)

	cmd := fmt.Sprintf("PGPASSWORD=%s pg_dump -U %s -d %s --clean --if-exists --no-owner --no-privileges > /tmp/%s",
		d.cfg.Password, d.cfg.Username, d.cfg.Database, dumpName)
	code, reader, err := d.container.Exec(ctx, []string{"sh", "-c", cmd})
	if err != nil {
		return fmt.Errorf("postgres: pg_dump failed: %w", err)
	}
	if code != 0 {
		errBytes, _ := readAll(reader)
		return fmt.Errorf("postgres: pg_dump failed (exit %d): %s", code, string(errBytes))
	}

	if err := copyFileFromContainer(ctx, d.container, "/tmp/"+dumpName, dumpPath); err != nil {
		return fmt.Errorf("postgres: failed to save dump to host: %w", err)
	}
	return nil
}

// Truncate is a no-op for Postgres — the pristine dump's DROP ... IF EXISTS
// statements make truncation redundant on the fast path.
func (d *PostgresDriver) Truncate(_ context.Context, _ *dbtestkit.Environment) error {
	return nil
}

// Stop terminates the container.
func (d *PostgresDriver) Stop(ctx context.Context, _ *dbtestkit.Environment) error {
	if d.container == nil {
		return nil
	}
	if err := d.container.Stop(ctx, nil); err != nil {
		return fmt.Errorf("postgres: failed to stop container: %w", err)
	}
	return nil
}

// ResetStrategy returns RestoreDump to utilize the fast-path CLI pipe.
func (d *PostgresDriver) ResetStrategy() dbtestkit.ResetStrategy {
	return dbtestkit.ResetStrategyRestoreDump
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// readAll is a thin wrapper around io.ReadAll that swallows the error — we
// only use it for diagnostic output when a container Exec has already failed.
func readAll(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return io.ReadAll(r)
}

// copyFileFromContainer extracts a file from the container to the host.
func copyFileFromContainer(ctx context.Context, ctr testcontainers.Container, containerPath, hostPath string) error {
	reader, err := ctr.CopyFileFromContainer(ctx, containerPath)
	if err != nil {
		return fmt.Errorf("failed to copy file from container: %w", err)
	}
	defer reader.Close()

	file, err := os.Create(hostPath)
	if err != nil {
		return fmt.Errorf("failed to create host file: %w", err)
	}
	defer file.Close()

	if _, err = io.Copy(file, reader); err != nil {
		return fmt.Errorf("failed to write to host file: %w", err)
	}
	return nil
}
