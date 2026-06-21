// Package sqlite. sqlite.go - SQLite backend for dbtestkit. Uses an in-process,
// file-backed SQLite database instead of a Docker container — ideal for fast,
// hermetic tests that do not need a real network database.
//
// Reset strategy: since SQLite has no equivalent of mysqldump streaming back
// into the same process, the fast path simply copies a pristine snapshot of
// the database file over the working file. The slow path runs the user's
// schema + data initializers, then snapshots the file as the pristine copy.
package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/seyallius/snapdb"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// SQLiteDriver implements dbtestkit.DatabaseDriver for SQLite.
type SQLiteDriver struct {
	dbPath       string
	pristinePath string
}

// ------------------------------------------- Constructor(s) --------------------------------------- //

// New returns a fresh SQLite driver instance.
//
// Pass the result to dbtestkit.WithDriver:
//
//	dbtestkit.Run(m,
//	    dbtestkit.WithDriver(sqlite.New()),
//	    ...
//	)
func New() dbtestkit.DatabaseDriver {
	return &SQLiteDriver{}
}

// -------------------------------------------- Public API ------------------------------------------ //

// Driver returns the dbtestkit.Driver constant this implementation serves.
func (d *SQLiteDriver) Driver() dbtestkit.Driver { return dbtestkit.DriverSQLite }

// Start ensures the SQLite database file does not exist (fresh start) and
// returns its file:// DSN.
//
// The actual schema + data initialization is performed by the dbtestkit
// core via the user's WithSchemaInitializer and WithDataInitializer
// callbacks — Start only prepares the filesystem.
func (d *SQLiteDriver) Start(_ context.Context, env *dbtestkit.Environment) (string, error) {
	d.dbPath = env.SQLitePath()
	d.pristinePath = env.TestdataDir() + "/sqlite-pristine.snapshot"

	if err := os.MkdirAll(env.TestdataDir(), 0o755); err != nil {
		return "", fmt.Errorf("sqlite: failed to create testdata dir: %w", err)
	}

	// Always start from a clean file — the pristine snapshot will be
	// restored later by either setup (slow path) or reset (fast path).
	if err := os.Remove(d.dbPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("sqlite: failed to remove existing db file: %w", err)
	}

	return fmt.Sprintf("file:%s?cache=shared", d.dbPath), nil
}

// RestoreDump copies the pristine snapshot over the working database file.
// This is the fast-path reset (~μs).
//
// For SQLite, the "dump path" passed in is treated as a binary snapshot
// file path rather than a SQL text dump. This is dramatically faster than
// re-running .dump output through the CLI.
func (d *SQLiteDriver) RestoreDump(_ context.Context, _ *dbtestkit.Environment, dumpPath string) error {
	if _, err := os.Stat(dumpPath); err != nil {
		return fmt.Errorf("sqlite: pristine snapshot missing at %s: %w", dumpPath, err)
	}

	// Close any open connection by removing the working file first; the
	// engine will re-open it on next query. (Callers using a connection
	// pool with cache=shared should ensure the engine has been closed
	// before reset — dbtestkit's core does this via the cache invalidator
	// hook or by closing the engine itself.)
	if err := os.Remove(d.dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("sqlite: failed to remove working db file: %w", err)
	}
	if err := copyFile(dumpPath, d.dbPath); err != nil {
		return fmt.Errorf("sqlite: failed to restore snapshot: %w", err)
	}
	return nil
}

// GenerateDump snapshots the current working database file as the pristine
// copy. Called once during the slow-path setup.
func (d *SQLiteDriver) GenerateDump(_ context.Context, _ *dbtestkit.Environment, dumpPath string) error {
	if _, err := os.Stat(d.dbPath); err != nil {
		return fmt.Errorf("sqlite: working db file missing at %s: %w", d.dbPath, err)
	}
	if err := copyFile(d.dbPath, dumpPath); err != nil {
		return fmt.Errorf("sqlite: failed to write snapshot: %w", err)
	}
	return nil
}

// Truncate empties every user table via DELETE statements. Used by the
// dbtestkit core as a fallback when no pristine snapshot is available.
//
// SQLite does not support TRUNCATE TABLE — we use DELETE, which is slower
// than the snapshot restore but works without a pre-baked snapshot.
func (d *SQLiteDriver) Truncate(_ context.Context, env *dbtestkit.Environment) error {
	engine := env.Engine()
	if engine == nil {
		return fmt.Errorf("sqlite: Truncate called before engine init")
	}

	// Disable foreign key checks during truncation to avoid ordering issues.
	if _, err := engine.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("sqlite: failed to disable foreign keys: %w", err)
	}

	rows, err := engine.QueryString(
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'",
	)
	if err != nil {
		return fmt.Errorf("sqlite: failed to list tables: %w", err)
	}

	// SQLite does not support multi-statement Exec via the standard driver;
	// execute them one by one.
	for _, row := range rows {
		for _, name := range row {
			stmt := fmt.Sprintf("DELETE FROM `%s`", name)
			if _, err = engine.Exec(stmt); err != nil {
				return fmt.Errorf("sqlite: failed to truncate with [%s]: %w", stmt, err)
			}
		}
	}

	if _, err = engine.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("sqlite: failed to re-enable foreign keys: %w", err)
	}
	return nil
}

// Stop removes the working database file.
func (d *SQLiteDriver) Stop(_ context.Context, _ *dbtestkit.Environment) error {
	if d.dbPath == "" {
		return nil
	}
	if err := os.Remove(d.dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("sqlite: failed to remove db file: %w", err)
	}
	return nil
}

// ResetStrategy returns TruncateAndSeed to avoid Windows file-locking issues
// when trying to replace the SQLite file while Xorm holds open connections.
func (d *SQLiteDriver) ResetStrategy() dbtestkit.ResetStrategy {
	return dbtestkit.ResetStrategyTruncateAndSeed
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// copyFile duplicates a file's contents byte-for-byte.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
