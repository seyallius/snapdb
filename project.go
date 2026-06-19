// Package dbtestkit. project.go - Provides the helper that locates the project root
// directory by walking up the filesystem until a go.mod file is found.
package snapdb

import (
	"fmt"
	"os"
	"path/filepath"
)

// ------------------------------------------- Internal Helpers ------------------------------------- //

// findProjectRoot walks up from the current working directory until it finds a
// directory containing a go.mod file. Returns an error if none is found before
// reaching the filesystem root.
func findProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	current := wd
	for {
		if _, err = os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("could not find project root (go.mod not found)")
		}
		current = parent
	}
}
