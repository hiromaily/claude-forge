package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// moduleRoot returns the absolute path to the mcp-server/ module root
// by walking up from the current file until go.mod is found.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from " + file)
		}
		dir = parent
	}
}
