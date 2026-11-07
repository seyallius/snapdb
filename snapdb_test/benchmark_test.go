// Package snapdb_test. benchmark_test.go - Benchmarks option application using
// the mock driver, so they run without Docker or any real database. Exists
// mainly so tooling that documents performance history (e.g. benchbook) has a
// real, honest measurement to track; this package has no other benchmarkable
// hot path since the real cost of snapdb lives in Docker/container I/O.
package snapdb_test

import (
	"testing"

	"github.com/seyallius/snapdb"
)

// ----------------------------------------------- Benchmarks --------------------------------------- //

// BenchmarkApplyOptions measures the cost of resolving a config.Option
// pipeline (validation + derived-default resolution), the one pure-CPU path
// every Run and Reset goes through.
func BenchmarkApplyOptions(b *testing.B) {
	tmp := b.TempDir()
	mock := newMockDriver("fake-dsn")
	opts := []snapdb.Option{
		snapdb.WithDriver(mock),
		snapdb.WithSchemaInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithDataInitializer(func(*snapdb.Environment) error { return nil }),
		snapdb.WithEngineInitializer(func(*snapdb.Environment) (snapdb.Engine, error) {
			return snapdb.NoopEngine{}, nil
		}),
		snapdb.WithProjectRoot(tmp),
		snapdb.WithTestdataDir(tmp),
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := snapdb.ApplyOptionsForTesting(opts...); err != nil {
			b.Fatal(err)
		}
	}
}
