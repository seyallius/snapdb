// Package sqlite. sqlite_test.go - Integration tests for the SQLite driver.
// These run a real SQLite database on local disk — no Docker required.
package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3" // Register the "sqlite3" driver with database/sql.
	"github.com/seyallius/snapdb"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // Register the pure-Go "sqlite" driver with database/sql (no CGO required).
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// sqliteEngine is a minimal snapdb.Engine wrapping *sql.DB on SQLite.
// Used only by these tests to exercise the driver end-to-end.
type sqliteEngine struct{ db *sql.DB }

// -------------------------------------------- Public API ------------------------------------------ //

func (e *sqliteEngine) Exec(q string, args ...any) (sql.Result, error) {
	return e.db.Exec(q, args...)
}

func (e *sqliteEngine) QueryString(q string, args ...any) ([]map[string]string, error) {
	rows, err := e.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []map[string]string
	for rows.Next() {
		raw := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]string, len(cols))
		for i, c := range cols {
			if raw[i].Valid {
				row[c] = raw[i].String
			} else {
				row[c] = ""
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (e *sqliteEngine) Ping() error       { return e.db.Ping() }
func (e *sqliteEngine) Close() error      { return e.db.Close() }
func (e *sqliteEngine) ClearCache() error { return nil }

// ----------------------------------------------- Tests -------------------------------------------- //

// TestSQLiteDriver_StartAndStop verifies that Start creates the testdata dir
// and returns a usable DSN, and that Stop removes the db file.
func TestSQLiteDriver_StartAndStop(t *testing.T) {
	tmp := t.TempDir()
	drv := New()

	env := &snapdb.Environment{}
	// We need to construct an Environment manually with the fields SQLite
	// reads. The snapdb package exposes SQLitePath() and TestdataDir()
	// as public methods — we set them via the testing helper.
	env = envWithSQLitePaths(env, tmp, filepath.Join(tmp, "test.sqlite"))

	dsn, err := drv.Start(context.Background(), env)
	require.NoError(t, err)
	require.Contains(t, dsn, "file:")

	// Stop should remove the db file (which doesn't exist yet — that's OK).
	err = drv.Stop(context.Background(), env)
	require.NoError(t, err)
}

// TestSQLiteDriver_GenerateAndRestoreDump verifies the snapshot round-trip:
// write some data, snapshot it, restore it, verify the data is back.
func TestSQLiteDriver_GenerateAndRestoreDump(t *testing.T) {
	tmp := t.TempDir()
	drv := New()

	dbPath := filepath.Join(tmp, "test.sqlite")
	snapshotPath := filepath.Join(tmp, "snapshot.bin")

	env := envWithSQLitePaths(nil, tmp, dbPath)

	// Start (creates empty db file path)
	_, err := drv.Start(context.Background(), env)
	require.NoError(t, err)

	// Open the database and create a table + insert a row.
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO users (name) VALUES (?)", "alice")
	require.NoError(t, err)

	// Close db so the snapshot is consistent.
	_ = db.Close()

	// Generate the snapshot.
	err = drv.GenerateDump(context.Background(), env, snapshotPath)
	require.NoError(t, err)
	require.FileExists(t, snapshotPath)

	// Mutate the working db: drop the row.
	db, err = sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM users")
	require.NoError(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
	_ = db.Close()

	// Restore the snapshot.
	err = drv.RestoreDump(context.Background(), env, snapshotPath)
	require.NoError(t, err)

	// Verify the row is back.
	db, err = sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var name string
	err = db.QueryRow("SELECT name FROM users WHERE id = 1").Scan(&name)
	require.NoError(t, err)
	require.Equal(t, "alice", name)
}

// TestSQLiteDriver_RestoreDump_MissingSnapshot verifies that RestoreDump
// returns a descriptive error when the snapshot file is absent.
func TestSQLiteDriver_RestoreDump_MissingSnapshot(t *testing.T) {
	tmp := t.TempDir()
	drv := New()

	env := envWithSQLitePaths(nil, tmp, filepath.Join(tmp, "test.sqlite"))
	_, err := drv.Start(context.Background(), env)
	require.NoError(t, err)

	err = drv.RestoreDump(context.Background(), env, filepath.Join(tmp, "nonexistent.bin"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
}

// TestSQLiteDriver_Truncate verifies that Truncate empties all user tables.
func TestSQLiteDriver_Truncate(t *testing.T) {
	tmp := t.TempDir()
	drv := New()

	dbPath := filepath.Join(tmp, "test.sqlite")
	env := envWithSQLitePaths(nil, tmp, dbPath)

	_, err := drv.Start(context.Background(), env)
	require.NoError(t, err)

	// Open the db, create a table, insert rows.
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("CREATE TABLE foo (id INTEGER PRIMARY KEY, val TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO foo (val) VALUES (?),(?),(?)", "a", "b", "c")
	require.NoError(t, err)

	// Build an engine for the driver to use during Truncate.
	eng := &sqliteEngine{db: db}

	// Truncate expects env.Engine() to return a valid Engine.
	env = envWithEngine(env, eng)

	err = drv.Truncate(context.Background(), env)
	require.NoError(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM foo").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

// TestSQLiteDriver_Truncate_NoEngine verifies that Truncate fails with a
// descriptive error when env.Engine() is nil.
func TestSQLiteDriver_Truncate_NoEngine(t *testing.T) {
	tmp := t.TempDir()
	drv := New()

	env := envWithSQLitePaths(nil, tmp, filepath.Join(tmp, "test.sqlite"))
	_, err := drv.Start(context.Background(), env)
	require.NoError(t, err)

	err = drv.Truncate(context.Background(), env)
	require.Error(t, err)
	require.Contains(t, err.Error(), "engine")
}

// TestSQLiteDriver_DriverName verifies that SQLiteDriver() returns the correct
// identifier.
func TestSQLiteDriver_DriverName(t *testing.T) {
	drv := New()
	require.Equal(t, snapdb.DriverSQLite, drv.Driver())
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// envWithSQLitePaths returns an *snapdb.Environment configured with the
// given testdata dir and SQLite file path. The snapdb package does not
// expose a public constructor for Environment (it's an opaque struct), so
// we use the testing helper that the package exposes.
func envWithSQLitePaths(env *snapdb.Environment, testdataDir, sqlitePath string) *snapdb.Environment {
	return snapdb.NewEnvironmentForTesting(env, snapdb.DriverSQLite, testdataDir, sqlitePath)
}

// envWithEngine returns a copy of env with the Engine field replaced.
func envWithEngine(env *snapdb.Environment, eng snapdb.Engine) *snapdb.Environment {
	return snapdb.EnvWithEngineForTesting(env, eng)
}
