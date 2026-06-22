// Package snapdb_test. mock_driver_test.go - A fake DatabaseDriver
// implementation used by the unit tests to exercise the lifecycle without
// spinning up Docker.
package snapdb_test

import (
	"context"
	"sync"

	"github.com/seyallius/snapdb"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// mockDriver is a recording DatabaseDriver that performs no real I/O.
// Every method call appends an entry to Calls so tests can assert ordering.
type mockDriver struct {
	mu    sync.Mutex
	calls []string
	dsn   string
}

// ------------------------------------------- Constructor(s) --------------------------------------- //

// newMockDriver returns a fresh mockDriver.
func newMockDriver(dsn string) *mockDriver {
	return &mockDriver{dsn: dsn}
}

// -------------------------------------------- Public API ------------------------------------------ //

// Driver implements snapdb.DatabaseDriver.
func (m *mockDriver) Driver() snapdb.Driver { return snapdb.DriverSQLite }

// Start implements snapdb.DatabaseDriver.
func (m *mockDriver) Start(_ context.Context, _ *snapdb.Environment) (string, error) {
	m.record("Start")
	return m.dsn, nil
}

// RestoreDump implements snapdb.DatabaseDriver.
func (m *mockDriver) RestoreDump(_ context.Context, _ *snapdb.Environment, _ string) error {
	m.record("RestoreDump")
	return nil
}

// GenerateDump implements snapdb.DatabaseDriver.
func (m *mockDriver) GenerateDump(_ context.Context, _ *snapdb.Environment, _ string) error {
	m.record("GenerateDump")
	return nil
}

// Truncate implements snapdb.DatabaseDriver.
func (m *mockDriver) Truncate(_ context.Context, _ *snapdb.Environment) error {
	m.record("Truncate")
	return nil
}

// Stop implements snapdb.DatabaseDriver.
func (m *mockDriver) Stop(_ context.Context, _ *snapdb.Environment) error {
	m.record("Stop")
	return nil
}

// ResetStrategy implements snapdb.DatabaseDriver.
func (m *mockDriver) ResetStrategy() snapdb.ResetStrategy {
	return snapdb.ResetStrategyRestoreDump
}

// ------------------------------------------- Internal Helpers ------------------------------------- //

// record appends a method name to the call log.
func (m *mockDriver) record(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
}

// Calls returns a snapshot of the recorded call sequence.
func (m *mockDriver) Calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	copy(out, m.calls)
	return out
}
