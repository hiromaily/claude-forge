// Package tools — integration tests for handler registration and compilation.
// Full guard-enforcement tests live in Task 7; here we verify that:
//   - RegisterAll compiles and adds exactly 47 tools
//   - Each handler calls the StateManager without panicking on valid input
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/analytics"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/profile"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
)

// setupWorkspace creates a temp dir with a state.json initialised to the given specName.
func setupWorkspace(t *testing.T, specName string) string {
	t.Helper()
	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, specName); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return dir
}

// callTool invokes handler directly, bypassing MCP transport.
func callTool(t *testing.T, handler server.ToolHandlerFunc, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error (unexpected): %v", err)
	}
	return res
}

// ---------- registry count test ----------

func TestRegisterAllCount(t *testing.T) {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	RegisterAll(srv, sm, events.NewEventBus(), events.NewSlackNotifier(""), "",
		orchestrator.NewEngine("", ""), "", history.New(""), history.NewKnowledgeBase(""),
		profile.New("", ""), (*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

	// Extract tools via ListTools to count them.
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
	if got := len(resp.Result.Tools); got != 47 {
		t.Errorf("RegisterAll: expected 47 tools, got %d", got)
		for _, tool := range resp.Result.Tools {
			t.Logf("  tool: %v", tool["name"])
		}
	}
}

// ---------- tool naming convention test ----------

func TestToolNamesUseUnderscores(t *testing.T) {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	sm := state.NewStateManager("dev")
	RegisterAll(srv, sm, events.NewEventBus(), events.NewSlackNotifier(""), "",
		orchestrator.NewEngine("", ""), "", history.New(""), history.NewKnowledgeBase(""),
		profile.New("", ""), (*analytics.Collector)(nil), (*analytics.Estimator)(nil), (*analytics.Reporter)(nil))

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
	for _, tool := range resp.Result.Tools {
		for _, ch := range tool.Name {
			if ch == '-' {
				t.Errorf("tool name %q contains hyphen; expected underscores only", tool.Name)
			}
		}
	}
}

// ---------- init handler ----------

func TestInitHandlerValidated(t *testing.T) {
	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	// Remove state.json so init creates it fresh.
	_ = os.Remove(filepath.Join(dir, "state.json"))

	h := InitHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"spec_name": "test-spec",
		"validated": true,
	})
	if res.IsError {
		t.Errorf("InitHandler with validated=true returned error: %v", textContent(res))
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Errorf("state.json not created: %v", err)
	}
}

func TestInitHandlerNotValidated(t *testing.T) {
	dir := t.TempDir()
	sm := state.NewStateManager("dev")

	h := InitHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"spec_name": "test-spec",
		"validated": false,
	})
	if !res.IsError {
		t.Errorf("InitHandler with validated=false should return error")
	}
}

// ---------- get handler ----------

func TestGetHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := GetHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"field":     "specName",
	})
	if res.IsError {
		t.Errorf("GetHandler returned error: %v", textContent(res))
	}
	if got := textContent(res); got != "test-spec" {
		t.Errorf("GetHandler specName: got %q, want %q", got, "test-spec")
	}
}

// ---------- phase_start handler ----------

func TestPhaseStartHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := PhaseStartHandler(sm, events.NewEventBus())
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-1",
	})
	if res.IsError {
		t.Errorf("PhaseStartHandler returned error: %v", textContent(res))
	}
}

// ---------- phase_start guard 3c (tasks empty for phase-5) ----------

func TestPhaseStartHandlerGuard3c(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := PhaseStartHandler(sm, events.NewEventBus())
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-5",
	})
	if !res.IsError {
		t.Errorf("PhaseStartHandler phase-5 with empty tasks should return error")
	}
}

// ---------- phase_complete handler with warning ----------

func TestPhaseCompleteHandlerWarning3i(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")
	// Create the required artifact for phase-1.
	if err := os.WriteFile(filepath.Join(dir, "analysis.md"), []byte("analysis"), 0o644); err != nil {
		t.Fatal(err)
	}
	// phase-1 is pending, not in_progress — should produce a warning but NOT block.
	h := PhaseCompleteHandler(sm, events.NewEventBus(), events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-1",
	})
	if res.IsError {
		t.Errorf("PhaseCompleteHandler should not error for pending phase: %v", textContent(res))
	}
}

// ---------- phase_complete guard 3a (artifact missing) ----------

func TestPhaseCompleteHandlerGuard3a(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")
	// Do NOT create analysis.md — artifact is missing.
	h := PhaseCompleteHandler(sm, events.NewEventBus(), events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-1",
	})
	if !res.IsError {
		t.Errorf("PhaseCompleteHandler should block when artifact is missing")
	}
}

// ---------- phase_fail handler ----------

func TestPhaseFailHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")
	if err := sm.PhaseStart(dir, "phase-1"); err != nil {
		t.Fatal(err)
	}

	h := PhaseFailHandler(sm, events.NewEventBus(), events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-1",
		"message":   "something went wrong",
	})
	if res.IsError {
		t.Errorf("PhaseFailHandler returned error: %v", textContent(res))
	}
}

// ---------- abandon handler ----------

func TestAbandonHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := AbandonHandler(sm, events.NewEventBus(), events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
	})
	if res.IsError {
		t.Errorf("AbandonHandler returned error: %v", textContent(res))
	}
}

// ---------- set_branch handler ----------

func TestSetBranchHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := SetBranchHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"branch":    "feature/test",
	})
	if res.IsError {
		t.Errorf("SetBranchHandler returned error: %v", textContent(res))
	}
}

// ---------- set_effort handler ----------

func TestSetEffortHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := SetEffortHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"effort":    "M",
	})
	if res.IsError {
		t.Errorf("SetEffortHandler returned error: %v", textContent(res))
	}
}

func TestSetEffortHandlerInvalid(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := SetEffortHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"effort":    "INVALID",
	})
	if !res.IsError {
		t.Errorf("SetEffortHandler with invalid effort should return error")
	}
}

// ---------- resume_info handler ----------

func TestResumeInfoHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := ResumeInfoHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
	})
	if res.IsError {
		t.Errorf("ResumeInfoHandler returned error: %v", textContent(res))
	}
}

// ---------- refresh_index handler (Go implementation) ----------

func TestRefreshIndexHandlerGoImpl_Success(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	h := refreshIndexWithSpecsDir(specsDir)
	res := callTool(t, h, map[string]any{})
	if res.IsError {
		t.Errorf("refreshIndexWithSpecsDir on empty dir should succeed, got error: %v", textContent(res))
	}
}

func TestRefreshIndexHandlerGoImpl_EmptyDir(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	h := refreshIndexWithSpecsDir(specsDir)
	res := callTool(t, h, map[string]any{})
	if res.IsError {
		t.Errorf("refreshIndexWithSpecsDir on empty specsDir returned error: %v", textContent(res))
	}
	// Verify index.json was written.
	idxPath := filepath.Join(specsDir, "index.json")
	if _, err := os.Stat(idxPath); err != nil {
		t.Errorf("index.json not created: %v", err)
	}
}

// ---------- task_init guard 3g (checkpoint-b not done) ----------

func TestTaskInitHandlerGuard3g(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := TaskInitHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"tasks":     map[string]any{},
	})
	if !res.IsError {
		t.Errorf("TaskInitHandler should block when checkpoint-b not done")
	}
}

// ---------- phase_complete guard 3j (checkpoint revision pending) ----------

func TestPhaseCompleteHandlerGuard3j(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	// Put checkpoint-a into awaiting_human status (required by guard 3e).
	if err := sm.Checkpoint(dir, "checkpoint-a"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	// Set checkpointRevisionPending["checkpoint-a"] = true (triggers guard 3j).
	if err := sm.SetRevisionPending(dir, "checkpoint-a"); err != nil {
		t.Fatalf("SetRevisionPending: %v", err)
	}

	h := PhaseCompleteHandler(sm, events.NewEventBus(), events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "checkpoint-a",
	})
	if !res.IsError {
		t.Errorf("PhaseCompleteHandler should block when checkpointRevisionPending is true")
	}
}

// ---------- task_init guard 3g — checkpoint-b skipped (positive path) ----------

func TestTaskInitHandlerGuard3gSkipped(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	// Mark checkpoint-b as skipped — guard 3g should allow task_init.
	if err := sm.SkipPhase(dir, "checkpoint-b"); err != nil {
		t.Fatalf("SkipPhase: %v", err)
	}

	h := TaskInitHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"tasks": map[string]any{
			"1": map[string]any{"title": "Task 1", "executionMode": "sequential", "implStatus": "pending"},
		},
	})
	if res.IsError {
		t.Errorf("TaskInitHandler should succeed when checkpoint-b is skipped: %v", textContent(res))
	}
}

func TestTaskInitHandlerZeroTasksError(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	if err := sm.SkipPhase(dir, "checkpoint-b"); err != nil {
		t.Fatalf("SkipPhase: %v", err)
	}

	h := TaskInitHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"tasks":     map[string]any{},
	})
	if !res.IsError {
		t.Errorf("TaskInitHandler should fail with empty tasks map, got success")
	}
}

// ---------- validateTaskDependencies ----------

func TestValidateTaskDependencies_ValidTasks(t *testing.T) {
	t.Parallel()
	tasks := map[string]state.Task{
		"1": {Title: "First", ExecutionMode: "parallel", Files: []string{"a.go"}},
		"2": {Title: "Second", ExecutionMode: "parallel", Files: []string{"b.go"}},
		"3": {Title: "Third", ExecutionMode: "sequential", DependsOn: []int{1, 2}, Files: []string{"c.go"}},
	}
	warnings := validateTaskDependencies(tasks)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestValidateTaskDependencies_InvalidDependsOn(t *testing.T) {
	t.Parallel()
	tasks := map[string]state.Task{
		"1": {Title: "First", ExecutionMode: "sequential", DependsOn: []int{99}},
	}
	warnings := validateTaskDependencies(tasks)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "does not exist") {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestValidateTaskDependencies_CircularDependency(t *testing.T) {
	t.Parallel()
	tasks := map[string]state.Task{
		"1": {Title: "A", ExecutionMode: "sequential", DependsOn: []int{2}},
		"2": {Title: "B", ExecutionMode: "sequential", DependsOn: []int{1}},
	}
	warnings := validateTaskDependencies(tasks)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "circular") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected circular dependency warning, got %v", warnings)
	}
}

func TestValidateTaskDependencies_ParallelFileConflict(t *testing.T) {
	t.Parallel()
	tasks := map[string]state.Task{
		"1": {Title: "A", ExecutionMode: "parallel", Files: []string{"shared.go"}},
		"2": {Title: "B", ExecutionMode: "parallel", Files: []string{"shared.go"}},
	}
	warnings := validateTaskDependencies(tasks)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "both write to") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected parallel file conflict warning, got %v", warnings)
	}
}

// ---------- phase_log handler ----------

func TestPhaseLogHandler(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := PhaseLogHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":   dir,
		"phase":       "phase-1",
		"tokens":      1000,
		"duration_ms": 5000,
		"model":       "sonnet",
	})
	if res.IsError {
		t.Errorf("PhaseLogHandler returned error: %v", textContent(res))
	}
}

// ---------- phase_log duplicate warning (Warn3d) ----------

func TestPhaseLogHandlerWarn3d(t *testing.T) {
	dir := setupWorkspace(t, "test-spec")
	sm := state.NewStateManager("dev")

	h := PhaseLogHandler(sm)
	// First call — no warning.
	callTool(t, h, map[string]any{
		"workspace":   dir,
		"phase":       "phase-1",
		"tokens":      100,
		"duration_ms": 1000,
		"model":       "sonnet",
	})
	// Second call — duplicate warning expected.
	res := callTool(t, h, map[string]any{
		"workspace":   dir,
		"phase":       "phase-1",
		"tokens":      200,
		"duration_ms": 2000,
		"model":       "sonnet",
	})
	if res.IsError {
		t.Errorf("PhaseLogHandler duplicate should not block: %v", textContent(res))
	}
	if !hasWarning(res) {
		t.Errorf("PhaseLogHandler duplicate: expected warning key in content")
	}
}

// ---------- event publish tests ----------

// drainEvent reads the first event from ch with a short timeout, or returns zero value.
func drainEvent(ch <-chan events.Event) (events.Event, bool) {
	select {
	case e, ok := <-ch:
		return e, ok
	default:
		return events.Event{}, false
	}
}

func TestPhaseStartHandlerPublishesEvent(t *testing.T) {
	dir := setupWorkspace(t, "my-spec")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	_, ch := bus.Subscribe()

	h := PhaseStartHandler(sm, bus)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-1",
	})
	if res.IsError {
		t.Fatalf("PhaseStartHandler returned error: %v", textContent(res))
	}

	e, ok := drainEvent(ch)
	if !ok {
		t.Fatal("no event published by PhaseStartHandler")
	}
	if e.Event != "phase-start" {
		t.Errorf("Event.Event: got %q, want %q", e.Event, "phase-start")
	}
	if e.Outcome != "in_progress" {
		t.Errorf("Event.Outcome: got %q, want %q", e.Outcome, "in_progress")
	}
	if e.Phase != "phase-1" {
		t.Errorf("Event.Phase: got %q, want %q", e.Phase, "phase-1")
	}
	if e.SpecName != "my-spec" {
		t.Errorf("Event.SpecName: got %q, want %q", e.SpecName, "my-spec")
	}
	if e.Workspace != dir {
		t.Errorf("Event.Workspace: got %q, want %q", e.Workspace, dir)
	}
	if e.Timestamp == "" {
		t.Error("Event.Timestamp is empty")
	}
}

func TestPhaseStartHandlerNoPublishOnError(t *testing.T) {
	dir := setupWorkspace(t, "my-spec")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	_, ch := bus.Subscribe()

	// phase-5 with empty tasks triggers guard error — no publish
	h := PhaseStartHandler(sm, bus)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-5",
	})
	if !res.IsError {
		t.Fatal("expected error from PhaseStartHandler guard")
	}
	if _, ok := drainEvent(ch); ok {
		t.Error("event should not be published when handler returns an error")
	}
}

func TestPhaseCompleteHandlerPublishesEvent(t *testing.T) {
	dir := setupWorkspace(t, "my-spec")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	_, ch := bus.Subscribe()
	// Write required artifact for phase-1.
	if err := os.WriteFile(filepath.Join(dir, "analysis.md"), []byte("analysis"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := PhaseCompleteHandler(sm, bus, events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-1",
	})
	if res.IsError {
		t.Fatalf("PhaseCompleteHandler returned error: %v", textContent(res))
	}

	e, ok := drainEvent(ch)
	if !ok {
		t.Fatal("no event published by PhaseCompleteHandler")
	}
	if e.Event != "phase-complete" {
		t.Errorf("Event.Event: got %q, want %q", e.Event, "phase-complete")
	}
	if e.Outcome != "completed" {
		t.Errorf("Event.Outcome: got %q, want %q", e.Outcome, "completed")
	}
}

func TestPhaseFailHandlerPublishesEvent(t *testing.T) {
	dir := setupWorkspace(t, "my-spec")
	sm := state.NewStateManager("dev")
	if err := sm.PhaseStart(dir, "phase-1"); err != nil {
		t.Fatal(err)
	}
	bus := events.NewEventBus()
	_, ch := bus.Subscribe()

	h := PhaseFailHandler(sm, bus, events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-1",
		"message":   "test failure",
	})
	if res.IsError {
		t.Fatalf("PhaseFailHandler returned error: %v", textContent(res))
	}

	e, ok := drainEvent(ch)
	if !ok {
		t.Fatal("no event published by PhaseFailHandler")
	}
	if e.Event != "phase-fail" {
		t.Errorf("Event.Event: got %q, want %q", e.Event, "phase-fail")
	}
	if e.Outcome != "failed" {
		t.Errorf("Event.Outcome: got %q, want %q", e.Outcome, "failed")
	}
}

func TestCheckpointHandlerPublishesEvent(t *testing.T) {
	dir := setupWorkspace(t, "my-spec")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	_, ch := bus.Subscribe()

	h := CheckpointHandler(sm, bus)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "checkpoint-a",
	})
	if res.IsError {
		t.Fatalf("CheckpointHandler returned error: %v", textContent(res))
	}

	e, ok := drainEvent(ch)
	if !ok {
		t.Fatal("no event published by CheckpointHandler")
	}
	if e.Event != "checkpoint" {
		t.Errorf("Event.Event: got %q, want %q", e.Event, "checkpoint")
	}
	if e.Outcome != "awaiting_human" {
		t.Errorf("Event.Outcome: got %q, want %q", e.Outcome, "awaiting_human")
	}
}

func TestAbandonHandlerPublishesEvent(t *testing.T) {
	dir := setupWorkspace(t, "my-spec")
	sm := state.NewStateManager("dev")
	bus := events.NewEventBus()
	_, ch := bus.Subscribe()

	h := AbandonHandler(sm, bus, events.NewSlackNotifier(""))
	res := callTool(t, h, map[string]any{
		"workspace": dir,
	})
	if res.IsError {
		t.Fatalf("AbandonHandler returned error: %v", textContent(res))
	}

	e, ok := drainEvent(ch)
	if !ok {
		t.Fatal("no event published by AbandonHandler")
	}
	if e.Event != "abandon" {
		t.Errorf("Event.Event: got %q, want %q", e.Event, "abandon")
	}
	if e.Outcome != "abandoned" {
		t.Errorf("Event.Outcome: got %q, want %q", e.Outcome, "abandoned")
	}
}

// ---------- validate_artifact handler smoke test ----------

// TestValidateArtifactHandler_Phase6 is a smoke test that calls ValidateArtifactHandler
// directly using t.Context() (not the callTool helper which uses context.Background()).
// It verifies the handler returns a valid JSON array with valid:true for a PASS impl file.
func TestValidateArtifactHandler_Phase6(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte("## Summary\n\nPASS\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := ValidateArtifactHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"workspace": dir,
		"phase":     "phase-6",
	}
	res, err := h(t.Context(), req)
	if err != nil {
		t.Fatalf("ValidateArtifactHandler returned Go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ValidateArtifactHandler phase-6 returned MCP error: %v", textContent(res))
	}

	var results []struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &results); err != nil {
		t.Fatalf("unmarshal results array: %v (content: %s)", err, textContent(res))
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result, got 0")
	}
	if !results[0].Valid {
		t.Errorf("TestValidateArtifactHandler_Phase6: got valid=false, want valid=true")
	}
}

// ---------- helpers ----------

// textContent extracts the text from the first TextContent item in a result.
func textContent(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// hasWarning returns true if the result content contains a "warning" key.
func hasWarning(res *mcp.CallToolResult) bool {
	if res == nil {
		return false
	}
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			var m map[string]any
			if err := json.Unmarshal([]byte(tc.Text), &m); err == nil {
				if _, ok := m["warning"]; ok {
					return true
				}
			}
		}
	}
	return false
}
