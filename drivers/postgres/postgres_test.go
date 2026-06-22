package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq" // Register Postgres driver
	"github.com/seyallius/snapdb"
	"github.com/seyallius/snapdb/drivers/postgres"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------- Fixtures ------------------------------------- //

// sqlEngine wraps *sql.DB to satisfy dbtestkit.Engine for testing.
// (Same implementation as MySQL test)
type sqlEngine struct{ db *sql.DB }

func (e *sqlEngine) Exec(q string, args ...any) (sql.Result, error) { return e.db.Exec(q, args...) }
func (e *sqlEngine) QueryString(q string, args ...any) ([]map[string]string, error) {
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
func (e *sqlEngine) Ping() error       { return e.db.Ping() }
func (e *sqlEngine) Close() error      { return e.db.Close() }
func (e *sqlEngine) ClearCache() error { return nil }

// ------------------------------------------- Tests -------------------------------------- //

func TestPostgresDriver_Integration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test. Set TEST_INTEGRATION=1 to run (requires Docker).")
	}

	tmp := t.TempDir()
	dumpPath := filepath.Join(tmp, "postgres-pristine.sql")

	env, eng, drv, err := dbtestkit.SetupForTesting(
		dbtestkit.WithDriver(postgres.New()),
		dbtestkit.WithDatabase(dbtestkit.DatabaseConfig{
			Database:       "testdb",
			Username:       "postgres",
			Password:       "testpass",
			Image:          "postgres:16-alpine",
			StartupTimeout: 3 * time.Minute,
		}),
		dbtestkit.WithSchemaInitializer(func(*dbtestkit.Environment) error { return nil }),
		dbtestkit.WithDataInitializer(func(*dbtestkit.Environment) error { return nil }),
		dbtestkit.WithEngineInitializer(func(env *dbtestkit.Environment) (dbtestkit.Engine, error) {
			// lib/pq accepts the exact DSN format postgres.go returns
			db, err := sql.Open("postgres", env.DSN())
			if err != nil {
				return nil, err
			}
			return &sqlEngine{db: db}, nil
		}),
		dbtestkit.WithProjectRoot(tmp),
		dbtestkit.WithTestdataDir(tmp),
		dbtestkit.WithPristineDumpPath(dumpPath),
		dbtestkit.WithLogger(dbtestkit.NewDefaultLogger(nil)),
	)
	require.NoError(t, err, "SetupForTesting failed to boot Postgres container")

	ctx := context.Background()
	defer func() {
		_ = eng.Close()
		_ = drv.Stop(ctx, env)
	}()

	// 1. Create table and insert data
	_, err = eng.Exec("CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(50))")
	require.NoError(t, err)
	_, err = eng.Exec("INSERT INTO users (id, name) VALUES (1, 'alice')")
	require.NoError(t, err)

	// 2. Generate Dump (Tests pg_dump execution)
	err = drv.GenerateDump(ctx, env, dumpPath)
	require.NoError(t, err)
	require.FileExists(t, dumpPath)

	// 3. Mutate data
	_, err = eng.Exec("DELETE FROM users")
	require.NoError(t, err)

	// 4. Restore Dump (Tests psql pipe execution)
	err = drv.RestoreDump(ctx, env, dumpPath)
	require.NoError(t, err)

	// 5. Verify data is back
	var count int
	err = eng.(*sqlEngine).db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "Data should be restored from dump")
}
