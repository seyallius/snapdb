package mysql_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql" // Register MySQL driver
	"github.com/seyallius/snapdb"
	"github.com/seyallius/snapdb/drivers/mysql"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------- Fixtures ------------------------------------- //

// sqlEngine wraps *sql.DB to satisfy dbtestkit.Engine for testing.
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

func TestMySQLDriver_Integration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test. Set TEST_INTEGRATION=1 to run (requires Docker).")
	}

	tmp := t.TempDir()
	dumpPath := filepath.Join(tmp, "mysql-pristine.sql")

	// SetupForTesting boots the actual Docker container and returns the live resources
	env, eng, drv, err := dbtestkit.SetupForTesting(
		dbtestkit.WithDriver(mysql.New()),
		dbtestkit.WithDatabase(dbtestkit.DatabaseConfig{
			Database:       "testdb",
			Username:       "root",
			Password:       "testpass",
			Image:          "mysql:lts",
			StartupTimeout: 3 * time.Minute, // Allow time for image pull on cold CI
		}),
		dbtestkit.WithSchemaInitializer(func(*dbtestkit.Environment) error { return nil }),
		dbtestkit.WithDataInitializer(func(*dbtestkit.Environment) error { return nil }),
		dbtestkit.WithEngineInitializer(func(env *dbtestkit.Environment) (dbtestkit.Engine, error) {
			db, err := sql.Open("mysql", env.DSN())
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
	require.NoError(t, err, "SetupForTesting failed to boot MySQL container")

	ctx := context.Background()

	// Ensure we tear down the container even if the test fails midway
	defer func() {
		_ = eng.Close()
		_ = drv.Stop(ctx, env)
	}()

	// 1. Create table and insert data
	_, err = eng.Exec("CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(50))")
	require.NoError(t, err)
	_, err = eng.Exec("INSERT INTO users (id, name) VALUES (1, 'alice')")
	require.NoError(t, err)

	// 2. Generate Dump (Tests mysqldump execution inside container)
	err = drv.GenerateDump(ctx, env, dumpPath)
	require.NoError(t, err)
	require.FileExists(t, dumpPath)

	// 3. Mutate data
	_, err = eng.Exec("DELETE FROM users")
	require.NoError(t, err)

	var count int
	err = eng.(*sqlEngine).db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "Data should be deleted before restore")

	// 4. Restore Dump (Tests mysql CLI pipe execution inside container)
	err = drv.RestoreDump(ctx, env, dumpPath)
	require.NoError(t, err)

	// 5. Verify data is back
	err = eng.(*sqlEngine).db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "Data should be restored from dump")
}
