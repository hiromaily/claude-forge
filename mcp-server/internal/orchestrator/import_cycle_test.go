package orchestrator

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// TestNoCycleOrchestratorState machine-verifies that the state package does not
// transitively import orchestrator, keeping the dependency one-way:
// orchestrator → state (never state → orchestrator).
func TestNoCycleOrchestratorState(t *testing.T) {
	t.Parallel()

	cmd := exec.CommandContext(t.Context(), "go", "list", "-json", "-deps",
		"github.com/hiromaily/claude-forge/mcp-server/internal/state")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -json -deps state: %v", err)
	}

	type pkg struct {
		ImportPath string
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if strings.Contains(p.ImportPath, "mcp-server/orchestrator") {
			t.Errorf("state package transitively imports orchestrator: %s", p.ImportPath)
		}
	}
}
