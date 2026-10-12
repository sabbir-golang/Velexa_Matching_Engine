package me

import (
	"os"
	"path/filepath"
	"testing"
)

// repoRoot returns the repository root (the directory holding go.mod). Tests run
// with the package directory as their working directory, but the engine resolves
// spec/, data/ and docs/ relative to the repo root — where it is normally started.
func repoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}
