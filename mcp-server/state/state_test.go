package state_test

import (
	"encoding/json"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// convenience aliases so the test body stays readable
type (
	State         = state.State
	Revisions     = state.Revisions
	Task          = state.Task
	PhaseLogEntry = state.PhaseLogEntry
	Timestamps    = state.Timestamps
)

var (
	ValidPhases    = state.ValidPhases
	ValidEfforts   = state.ValidEfforts
	ValidTemplates = state.ValidTemplates
	ValidRevTypes  = state.ValidRevTypes
)

// TestStateJSONRoundTrip verifies that State serialises and deserialises
// correctly using the exact JSON keys expected by state.json.
func TestStateJSONRoundTrip(t *testing.T) {
	branch := "feature/foo"
	taskType := "feature"
	effort := "L"
	flowTemplate := "full"
	phaseStarted := "2026-03-26T01:47:48Z"

	original := State{
		Version:            1,
		SpecName:           "test-spec",
		Workspace:          ".specs/test",
		Branch:             &branch,
		TaskType:           &taskType,
		Effort:             &effort,
		FlowTemplate:       &flowTemplate,
		AutoApprove:        false,
		SkipPr:             false,
		UseCurrentBranch:   false,
		Debug:              false,
		SkippedPhases:      []string{},
		CurrentPhase:       "phase-1",
		CurrentPhaseStatus: "in_progress",
		CompletedPhases:    []string{"setup"},
		Revisions: Revisions{
			DesignRevisions:       0,
			TaskRevisions:         0,
			DesignInlineRevisions: 1,
			TaskInlineRevisions:   1,
		},
		CheckpointRevisionPending: map[string]bool{
			"checkpoint-a": false,
			"checkpoint-b": false,
		},
		Tasks: map[string]Task{
			"1": {
				Title:         "Some task",
				ExecutionMode: "sequential",
				ImplStatus:    "pending",
				ReviewStatus:  "pending",
				ImplRetries:   0,
				ReviewRetries: 0,
			},
		},
		PhaseLog: []PhaseLogEntry{
			{
				Phase:      "phase-1",
				Tokens:     12345,
				DurationMs: 99000,
				Model:      "sonnet",
				Timestamp:  "2026-03-26T01:50:28Z",
			},
		},
		Timestamps: Timestamps{
			Created:      "2026-03-26T01:47:48Z",
			LastUpdated:  "2026-03-26T01:50:28Z",
			PhaseStarted: &phaseStarted,
		},
		Error: nil,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded State
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Version != original.Version {
		t.Errorf("Version: got %d, want %d", decoded.Version, original.Version)
	}
	if decoded.SpecName != original.SpecName {
		t.Errorf("SpecName: got %q, want %q", decoded.SpecName, original.SpecName)
	}
	if decoded.Branch == nil || *decoded.Branch != branch {
		t.Errorf("Branch: got %v, want %q", decoded.Branch, branch)
	}
}

// TestStateJSONKeys verifies that JSON keys match state-manager.sh output exactly.
func TestStateJSONKeys(t *testing.T) {
	branch := "main"
	s := State{Branch: &branch}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map: %v", err)
	}

	expectedKeys := []string{
		"version", "specName", "workspace", "branch", "taskType",
		"effort", "flowTemplate", "autoApprove", "skipPr",
		"useCurrentBranch", "debug", "skippedPhases", "currentPhase",
		"currentPhaseStatus", "completedPhases", "revisions",
		"checkpointRevisionPending", "tasks", "phaseLog", "timestamps", "error",
	}
	for _, k := range expectedKeys {
		if _, ok := raw[k]; !ok {
			t.Errorf("missing expected JSON key %q in State output", k)
		}
	}
}

// TestTaskJSONKeys verifies Task struct uses camelCase json tags, including
// implRetries and reviewRetries as numbers (int), not strings.
func TestTaskJSONKeys(t *testing.T) {
	task := Task{
		Title:         "My task",
		ExecutionMode: "sequential",
		ImplStatus:    "pending",
		ReviewStatus:  "pending",
		ImplRetries:   3,
		ReviewRetries: 2,
	}
	data, _ := json.Marshal(task)

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, k := range []string{"title", "executionMode", "implStatus", "reviewStatus", "implRetries", "reviewRetries"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("missing expected JSON key %q in Task output", k)
		}
	}

	// implRetries and reviewRetries must be JSON numbers (float64 after Unmarshal), not strings
	if v, ok := raw["implRetries"]; !ok || v == nil {
		t.Error("implRetries missing")
	} else if _, ok := v.(float64); !ok {
		t.Errorf("implRetries is not a number: %T %v", v, v)
	}
	if v, ok := raw["reviewRetries"]; !ok || v == nil {
		t.Error("reviewRetries missing")
	} else if _, ok := v.(float64); !ok {
		t.Errorf("reviewRetries is not a number: %T %v", v, v)
	}
}

// TestPhaseLogEntryDurationMsKey verifies the duration_ms key (snake_case) is used.
func TestPhaseLogEntryDurationMsKey(t *testing.T) {
	entry := PhaseLogEntry{
		Phase:      "phase-1",
		Tokens:     100,
		DurationMs: 5000,
		Model:      "sonnet",
		Timestamp:  "2026-03-26T01:50:28Z",
	}
	data, _ := json.Marshal(entry)

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := raw["duration_ms"]; !ok {
		t.Errorf("expected key 'duration_ms' but not found; keys: %v", raw)
	}
	if _, ok := raw["durationMs"]; ok {
		t.Errorf("unexpected key 'durationMs' found (should be 'duration_ms')")
	}
}

// TestValidConstants verifies the enum slices are non-empty and contain expected values.
func TestValidConstants(t *testing.T) {
	if len(ValidPhases) == 0 {
		t.Error("ValidPhases is empty")
	}
	if len(ValidEfforts) == 0 {
		t.Error("ValidEfforts is empty")
	}
	if len(ValidTemplates) == 0 {
		t.Error("ValidTemplates is empty")
	}
	if len(ValidRevTypes) == 0 {
		t.Error("ValidRevTypes is empty")
	}

	// Spot-check a few values
	hasPhase := false
	for _, p := range ValidPhases {
		if p == "phase-5" {
			hasPhase = true
		}
	}
	if !hasPhase {
		t.Error("ValidPhases should contain 'phase-5'")
	}

	hasEffort := false
	for _, e := range ValidEfforts {
		if e == "XS" {
			hasEffort = true
		}
	}
	if !hasEffort {
		t.Error("ValidEfforts should contain 'XS'")
	}

	hasTemplate := false
	for _, tmpl := range ValidTemplates {
		if tmpl == "lite" {
			hasTemplate = true
		}
	}
	if !hasTemplate {
		t.Error("ValidTemplates should contain 'lite'")
	}

	hasRevType := false
	for _, r := range ValidRevTypes {
		if r == "design" {
			hasRevType = true
		}
	}
	if !hasRevType {
		t.Error("ValidRevTypes should contain 'design'")
	}
}
