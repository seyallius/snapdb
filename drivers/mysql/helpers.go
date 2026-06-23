// Package mysql. helpers.go - Internal helpers for the MySQL driver: io helpers,
// container file copy, and the wait-strategy builder. Kept separate so the main
// mysql.go file stays focused on the DatabaseDriver contract.
package mysql

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/seyallius/snapdb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

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

// bakeEmptyTarball spins up a temporary MySQL container, lets it complete its
// normal initdb sequence, tars up the resulting /var/lib/mysql directory, and
// saves it to the testdata directory for future fast-path startups.
func bakeEmptyTarball(ctx context.Context, env *snapdb.Environment, cfg snapdb.DatabaseConfig) error {
	startupTimeout := cfg.StartupTimeout
	if startupTimeout == 0 {
		startupTimeout = defaultStartupTimeout
	}

	baker, err := mysql.Run(
		ctx,
		cfg.Image,
		mysql.WithDatabase(cfg.Database),
		mysql.WithUsername(cfg.Username),
		mysql.WithPassword(cfg.Password),
		testcontainers.WithWaitStrategy(waitForMySQL(ctx, startupTimeout, cfg)),
	)
	if err != nil {
		return fmt.Errorf("failed to start baker container: %w", err)
	}
	defer baker.Terminate(ctx)

	// Create the tarball inside the container
	cmd := []string{"sh", "-c", "tar cf - -C /var/lib/mysql . | gzip --fast > /tmp/empty-mysql.tar.gz"}
	code, reader, err := baker.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to exec tar in baker: %w", err)
	}
	if code != 0 {
		errBytes, _ := readAll(reader)
		return fmt.Errorf("tar failed (exit %d): %s", code, string(errBytes))
	}

	// Copy the tarball out to the host
	tarballPath := filepath.Join(env.TestdataDir(), quickstartTarballName)
	if err = os.MkdirAll(env.TestdataDir(), 0o755); err != nil {
		return fmt.Errorf("failed to create testdata dir: %w", err)
	}

	if err = copyFileFromContainer(ctx, baker, "/tmp/empty-mysql.tar.gz", tarballPath); err != nil {
		return fmt.Errorf("failed to save tarball to host: %w", err)
	}
	return nil
}

// waitForMySQL builds the wait strategy for the container. We avoid the
// upstream module's ForSQL helper because it spams "unexpected EOF" packets
// to the console while polling.
//
// Instead we wait for the entrypoint's "ready for connections" log line,
// which fires exactly once after the optimized first-run sequence completes.
func waitForMySQL(_ context.Context, timeout time.Duration, cfg snapdb.DatabaseConfig) wait.Strategy {
	urlFunc := func(host string, port nat.Port) string {
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/", cfg.Username, cfg.Password, host, port.Port())
	}
	_ = urlFunc

	// Log-based wait is faster and quieter than ForSQL polling.
	return wait.ForAll(
		wait.
			ForLog("ready for connections").
			WithStartupTimeout(timeout).
			WithOccurrence(1), // Single occurrence: quickstart skips the temporary initdb mysqld
		wait.
			ForListeningPort(defaultPort).
			WithStartupTimeout(timeout), // Guards against slow-path fallback where temporary mysqld logs ready but has no TCP socket
	)
}
