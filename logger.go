// Package snapdb. logger.go - Provides the Logger interface and a default
// tree-style printer that mirrors the original casdoor test output format.
package snapdb

import (
	"fmt"
	"io"
	"os"
	"time"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// Logger is the logging surface used by the library. Implementations are free to
// forward to *testing.T, log/slog, zap, or any other sink.
type Logger interface {
	// Step reports the elapsed time of an inner setup/reset step.
	Step(name string, elapsed time.Duration)

	// End reports the elapsed time of an outer setup/reset phase.
	End(name string, elapsed time.Duration)

	// Info emits an informational line (e.g. "==> 🚀 Test Container Setup").
	Info(msg string)

	// Warn emits a non-fatal warning.
	Warn(msg string)
}

// -------------------------------------------- Public API ------------------------------------------ //

// DefaultLogger prints tree-style timing output to stdout, matching the format used
// by the original casdoor test utilities:
//
//	===> 🚀 Test Container Setup
//	    ├── Create MySQL Container:                420ms
//	    └── ✅ Test Container Ready!               1.2s
type DefaultLogger struct {
	w io.Writer
}

// NewDefaultLogger returns a DefaultLogger writing to w. Pass nil to use os.Stdout.
func NewDefaultLogger(w io.Writer) *DefaultLogger {
	if w == nil {
		w = os.Stdout
	}
	return &DefaultLogger{w: w}
}

// Step implements Logger.
func (l *DefaultLogger) Step(name string, elapsed time.Duration) {
	fmt.Fprintf(l.w, "\t├── %-35s %v\n", name+":", elapsed.Round(time.Millisecond))
}

// End implements Logger.
func (l *DefaultLogger) End(name string, elapsed time.Duration) {
	fmt.Fprintf(l.w, "\t└── ✅ %-32s %v\n\n", name, elapsed.Round(time.Millisecond))
}

// Info implements Logger.
func (l *DefaultLogger) Info(msg string) {
	fmt.Fprintf(l.w, "\n===> %s\n", msg)
}

// Warn implements Logger.
func (l *DefaultLogger) Warn(msg string) {
	fmt.Fprintf(l.w, "⚠️  Warning: %s\n", msg)
}
