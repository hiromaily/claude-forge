package main

import (
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/dashboard"
	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// TestDashboardStart_NoopWhenPortEmpty verifies that main's wiring of
// dashboard.Start is correct: passing an empty FORGE_EVENTS_PORT must
// produce no HTTP listener.
//
// This guards the contract main.go relies on — that calling Start
// unconditionally is safe and that the empty-port short-circuit lives
// inside the dashboard package, not in main.
func TestDashboardStart_NoopWhenPortEmpty(t *testing.T) {
	t.Parallel()

	httpSrv := dashboard.Start("", events.NewEventBus(), state.NewStateManager("test"))
	if httpSrv != nil {
		t.Fatal(`expected dashboard.Start("", ...) to return nil`)
	}
}
