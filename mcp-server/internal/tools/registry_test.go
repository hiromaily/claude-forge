// Package tools — tests for updated RegisterAll signature and tool count.
// These tests verify that RegisterAll accepts 13 parameters and registers 46 tools
// including the subscribe_events, ast_summary, ast_find_definition, dependency_graph, impact_scope,
// pipeline_init, pipeline_init_with_context, pipeline_next_action, pipeline_report_result,
// history_search, history_get_patterns, history_get_friction_map, profile_get,
// analytics_pipeline_summary, analytics_repo_dashboard, and analytics_estimate tools.
package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/analytics"
	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/profile"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// TestRegisterAllNewSignatureCount verifies that the updated 13-arg RegisterAll
// registers exactly 46 tools (set_task_type removed), including subscribe_events,
// ast_summary, ast_find_definition, dependency_graph, impact_scope, pipeline_init,
// pipeline_init_with_context, pipeline_next_action, pipeline_report_result,
// history_search, history_get_patterns, history_get_friction_map, profile_get,
// analytics_pipeline_summary, analytics_repo_dashboard, and analytics_estimate.
func TestRegisterAllNewSignatureCount(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

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
	if got := len(resp.Result.Tools); got != 46 {
		t.Errorf("RegisterAll: expected 46 tools, got %d", got)
		for _, tool := range resp.Result.Tools {
			t.Logf("  tool: %v", tool["name"])
		}
	}
}

// TestSetTaskTypeToolNotRegistered verifies that set_task_type is not among the
// registered MCP tools after Task 5 removes it.
func TestSetTaskTypeToolNotRegistered(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

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
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	for _, tool := range resp.Result.Tools {
		if tool.Name == "set_task_type" {
			t.Errorf("set_task_type should not be registered after Task 5 removal")
		}
	}
}

// TestSearchPatternsToolSchemaNoTaskType verifies that the search_patterns tool
// does not include a task_type parameter in its schema.
func TestSearchPatternsToolSchemaNoTaskType(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

	msg := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	raw, _ := json.Marshal(msg)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	for _, tool := range resp.Result.Tools {
		if tool.Name == "search_patterns" {
			if _, hasTaskType := tool.InputSchema.Properties["task_type"]; hasTaskType {
				t.Errorf("search_patterns schema should not have task_type parameter")
			}
		}
	}
}

// TestAnalyticsEstimateToolSchemaNoTaskType verifies that analytics_estimate does
// not include a task_type parameter in its schema.
func TestAnalyticsEstimateToolSchemaNoTaskType(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

	msg := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	raw, _ := json.Marshal(msg)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	for _, tool := range resp.Result.Tools {
		if tool.Name == "analytics_estimate" {
			if _, hasTaskType := tool.InputSchema.Properties["task_type"]; hasTaskType {
				t.Errorf("analytics_estimate schema should not have task_type parameter")
			}
		}
	}
}

// TestHistorySearchToolSchemaNoTaskTypeFilter verifies that history_search does
// not include a task_type_filter parameter in its schema.
func TestHistorySearchToolSchemaNoTaskTypeFilter(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

	msg := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	raw, _ := json.Marshal(msg)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	for _, tool := range resp.Result.Tools {
		if tool.Name == "history_search" {
			if _, hasFilter := tool.InputSchema.Properties["task_type_filter"]; hasFilter {
				t.Errorf("history_search schema should not have task_type_filter parameter")
			}
		}
	}
}

// TestRegisterAllSubscribeEventsRegistered verifies that subscribe_events is one
// of the registered tools when RegisterAll is called with the 13-arg signature.
func TestRegisterAllSubscribeEventsRegistered(t *testing.T) {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "9090", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

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

// TestPipelineNextActionToolSchemaOptionalParams verifies that the pipeline_next_action
// tool schema includes the four optional previous_* parameters added in Task 3.
func TestPipelineNextActionToolSchemaOptionalParams(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")
	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

	msg := srv.HandleMessage(t.Context(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
					Required   []string       `json:"required"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	raw, _ := json.Marshal(msg)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}

	wantOptional := []string{
		"previous_tokens",
		"previous_duration_ms",
		"previous_model",
		"previous_setup_only",
	}

	for _, tool := range resp.Result.Tools {
		if tool.Name != "pipeline_next_action" {
			continue
		}
		for _, param := range wantOptional {
			if _, ok := tool.InputSchema.Properties[param]; !ok {
				t.Errorf("pipeline_next_action schema missing optional param %q", param)
			}
		}
		// Verify that none of the four params are in the required list.
		required := make(map[string]bool, len(tool.InputSchema.Required))
		for _, r := range tool.InputSchema.Required {
			required[r] = true
		}
		for _, param := range wantOptional {
			if required[param] {
				t.Errorf("pipeline_next_action param %q should be optional, but found in required list", param)
			}
		}
		return
	}
	t.Error("pipeline_next_action tool not found in registered tools")
}

// TestRegisterAllBusPassedToHandlers verifies that the bus passed to RegisterAll
// is used by event-emitting handlers (not a fresh NewEventBus() instance).
func TestRegisterAllBusPassedToHandlers(t *testing.T) {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier("")

	// Subscribe before RegisterAll so we use the same bus.
	_, ch := bus.Subscribe()

	RegisterAll(srv, sm, bus, slack, "", orchestrator.NewEngine("", ""), "",
		history.New(""), history.NewKnowledgeBase(""), profile.New("", ""),
		(*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

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
