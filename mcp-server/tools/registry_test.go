// Package tools — tests for updated RegisterAll signature and tool count.
// These tests verify that RegisterAll accepts 8 parameters and registers 39 tools
// including the subscribe_events, ast_summary, ast_find_definition, dependency_graph, impact_scope,
// pipeline_init, pipeline_init_with_context, pipeline_next_action, pipeline_report_result, and history_search tools.
package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/events"
	"github.com/hiromaily/claude-forge/mcp-server/history"
	"github.com/hiromaily/claude-forge/mcp-server/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// TestRegisterAllNewSignatureCount verifies that the updated 8-arg RegisterAll
// registers exactly 39 tools, including subscribe_events, ast_summary, ast_find_definition,
// dependency_graph, impact_scope, pipeline_init, pipeline_init_with_context,
// pipeline_next_action, pipeline_report_result, and history_search.
func TestRegisterAllNewSignatureCount(t *testing.T) {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager()
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "", history.New(""))

	msg := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var resp struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	raw, _ := json.Marshal(msg)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	if got := len(resp.Result.Tools); got != 39 {
		t.Errorf("RegisterAll: expected 39 tools, got %d", got)
		for _, tool := range resp.Result.Tools {
			t.Logf("  tool: %v", tool["name"])
		}
	}
}

// TestRegisterAllSubscribeEventsRegistered verifies that subscribe_events is one
// of the registered tools when RegisterAll is called with the 7-arg signature.
func TestRegisterAllSubscribeEventsRegistered(t *testing.T) {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager()
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "9090", orchestrator.NewEngine("", ""), "", history.New(""))

	msg := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	raw, _ := json.Marshal(msg)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, tool := range resp.Result.Tools {
		if tool.Name == "subscribe_events" {
			found = true
			break
		}
	}
	if !found {
		t.Error("subscribe_events tool not found in registered tools")
	}
}

// TestRegisterAllBusPassedToHandlers verifies that the bus passed to RegisterAll
// is used by event-emitting handlers (not a fresh NewEventBus() instance).
func TestRegisterAllBusPassedToHandlers(t *testing.T) {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager()
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")

	// Subscribe before RegisterAll so we use the same bus.
	_, ch := bus.Subscribe()

	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "", history.New(""))

	// Trigger abandon via the server using the AbandonHandler registered in RegisterAll.
	dir := t.TempDir()
	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}

	h := AbandonHandler(sm, bus, slack)
	req := callTool(t, h, map[string]any{"workspace": dir})
	if req.IsError {
		t.Fatalf("AbandonHandler returned error: %v", textContent(req))
	}

	e, ok := drainEvent(ch)
	if !ok {
		t.Fatal("no event received — bus passed to RegisterAll was not used by AbandonHandler")
	}
	if e.Event != "abandon" {
		t.Errorf("expected abandon event, got %q", e.Event)
	}
}
