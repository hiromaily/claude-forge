// Package state implements the StateManager that provides all state mutation
// and query operations for the forge-state MCP server. All 26 state-management
// commands are implemented here, using a sync.RWMutex for concurrent access.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StateManager owns the mutex and provides all methods that correspond to the
// 26 MCP state-management commands.  All mutating methods acquire mu.Lock() for the
// full read-modify-write cycle. Read-only methods (Get, PhaseStats, ResumeInfo)
// delegate to GetState(), which also acquires mu.Lock() to handle the lazy-load
// write path; mu.RLock() is not used in this implementation.
type StateManager struct {
	mu        sync.RWMutex
	state     *State // in-memory cache; nil until LoadFromFile or Init
	workspace string // bound workspace path; empty until first bind
	version   string // MCP server binary version; written to state.json on Init
}

// NewStateManager constructs a StateManager ready for use.
// version is the MCP server binary version (e.g. "v2.1.0") and is written to
// the forge-state-mcp-version field of state.json when Init is called.
func NewStateManager(version string) *StateManager {
	return &StateManager{version: version}
}

// Version returns the MCP server binary version that was passed to NewStateManager.
func (m *StateManager) Version() string {
	return m.version
}

// ---------- helpers ----------

func statePath(workspace string) string {
	return filepath.Join(workspace, StateFileName)
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// readState reads and unmarshals state.json from workspace.
// The caller must hold an appropriate lock.
func readState(workspace string) (*State, error) {
	return ReadState(workspace)
}

// ReadState reads and unmarshals state.json from workspace.
// Exported so that tools package can reuse it without duplicating read logic.
// TODO: replace ReadState calls with sm.GetState() once tools reduction is complete.
func ReadState(workspace string) (*State, error) {
	path := statePath(workspace)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("readState: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("readState unmarshal: %w", err)
	}
	return &s, nil
}

// writeState marshals s and writes it atomically (temp-file + rename) to
// workspace/state.json.  The caller must hold mu.Lock().
func writeState(workspace string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("writeState marshal: %w", err)
	}
	tmp := statePath(workspace) + TempSuffix
	if err := os.WriteFile(tmp, data, FilePermRW); err != nil {
		return fmt.Errorf("writeState write tmp: %w", err)
	}
	if err := os.Rename(tmp, statePath(workspace)); err != nil {
		return fmt.Errorf("writeState rename: %w", err)
	}
	return nil
}

// containsPhase returns true if phase is in ValidPhases.
func containsPhase(phase string) bool {
	return slices.Contains(ValidPhases, phase)
}

// nextPhase returns the phase that follows current in ValidPhases.
// If current is the last entry or not found, "completed" is returned.
func nextPhase(current string) string {
	found := false
	for _, p := range ValidPhases {
		if found {
			return p
		}
		if p == current {
			found = true
		}
	}
	return PhaseCompleted
}

// ---------- allowed Get fields ----------

// allowedGetFields is the set of top-level and dot-notation sub-fields that
// Get supports.  This mirrors what cmd_get does via `jq -r ".${field}"`.
// These are intentionally kept as string literals matching the JSON struct tags
// in State, so that the mapping stays visually aligned with the JSON schema.
var allowedGetFields = map[string]bool{
	"version":                 true,
	"forge-state-mcp-version": true,
	"specName":                true,
	"workspace":               true,
	"branch":                  true,
	"effort":                  true,
	"flowTemplate":            true,
	"autoApprove":             true,
	"skipPr":                  true,
	"useCurrentBranch":        true,
	"debug":                   true,
	"skippedPhases":           true,
	"currentPhase":            true,
	"currentPhaseStatus":      true,
	"completedPhases":         true,
	// dot-notation sub-fields
	"revisions":                       true,
	"revisions.designRevisions":       true,
	"revisions.taskRevisions":         true,
	"revisions.designInlineRevisions": true,
	"revisions.taskInlineRevisions":   true,
	"checkpointRevisionPending":       true,
	"tasks":                           true,
	"phaseLog":                        true,
	"timestamps":                      true,
	"timestamps.created":              true,
	"timestamps.lastUpdated":          true,
	"timestamps.phaseStarted":         true,
	"error":                           true,
}

// ---------- StateManager methods ----------

// Init creates a new state.json in workspace following the canonical schema
// defined by the State struct in state.go.
// Calling Init a second time is intentional (fresh-start operation); it replaces
// any prior binding of sm.workspace and sm.state.
func (m *StateManager) Init(workspace, specName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ts := nowISO()
	s := &State{
		Version:            2,
		MCPVersion:         m.version,
		SpecName:           specName,
		Workspace:          workspace,
		Branch:             nil,
		Effort:             nil,
		FlowTemplate:       nil,
		AutoApprove:        false,
		SkipPr:             false,
		UseCurrentBranch:   false,
		Debug:              false,
		SkippedPhases:      []string{},
		CurrentPhase:       PhaseOne,
		CurrentPhaseStatus: StatusPending,
		CompletedPhases:    []string{PhaseSetup},
		Revisions: Revisions{
			DesignRevisions:       0,
			TaskRevisions:         0,
			DesignInlineRevisions: 0,
			TaskInlineRevisions:   0,
		},
		CheckpointRevisionPending: map[string]bool{
			PhaseCheckpointA: false,
			PhaseCheckpointB: false,
		},
		Tasks:    map[string]Task{},
		PhaseLog: []PhaseLogEntry{},
		Timestamps: Timestamps{
			Created:      ts,
			LastUpdated:  ts,
			PhaseStarted: nil,
		},
		Error: nil,
	}
	if err := writeState(workspace, s); err != nil {
		return err
	}
	// Store in-memory cache and bind workspace — both under the already-held lock.
	m.workspace = workspace
	m.state = s
	return nil
}

// LoadFromFile reads workspace/state.json, runs Migrate, and stores the result
// as the in-memory cache. Sets sm.workspace.
// If already bound to a different workspace, returns a workspace-mismatch error.
// If bound to the same workspace, re-reads from disk (refresh semantics).
func (m *StateManager) LoadFromFile(workspace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.workspace != "" && m.workspace != workspace {
		return fmt.Errorf("workspace mismatch: got %q, bound to %q", workspace, m.workspace)
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}
	Migrate(s)
	m.workspace = workspace
	m.state = s
	return nil
}

// Update applies fn to the in-memory state under mu.Lock().
// If sm.workspace is empty, returns "state not loaded" error.
// If sm.state is nil but sm.workspace is set, lazy-loads from disk before calling fn.
// The lazy-load disk read executes under the already-held mu.Lock(); no nested
// lock acquisition occurs inside the lazy-load path.
// Mutations are applied to a deep copy; sm.state is only replaced after a
// successful write to disk, ensuring in-memory and on-disk state stay consistent.
// fn must not call any StateManager method (would deadlock).
func (m *StateManager) Update(fn func(*State) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.workspace == "" {
		return errors.New("state not loaded: call Init or LoadFromFile first")
	}

	// Lazy-load if cache is nil but workspace is bound.
	if m.state == nil {
		s, err := readState(m.workspace)
		if err != nil {
			return err
		}
		Migrate(s)
		m.state = s
	}

	// Work on a deep copy so that a persist failure leaves sm.state untouched.
	stateCopy := deepCopyState(m.state)

	if err := fn(stateCopy); err != nil {
		return err
	}

	stateCopy.Timestamps.LastUpdated = nowISO()

	// Only promote the copy to the live cache after a successful disk write.
	if err := writeState(m.workspace, stateCopy); err != nil {
		return err
	}
	m.state = stateCopy
	return nil
}

// GetState returns a deep copy of the current in-memory state.
// It acquires mu.Lock() (not RLock) because the lazy-load path writes sm.state.
// If sm.workspace is empty, returns "state not loaded" error.
// If sm.state is nil but sm.workspace is set, lazy-loads from disk.
// Takes no workspace argument — routing is by sm.workspace.
func (m *StateManager) GetState() (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.workspace == "" {
		return nil, errors.New("state not loaded: call Init or LoadFromFile first")
	}

	// Lazy-load if cache is nil but workspace is bound.
	if m.state == nil {
		s, err := readState(m.workspace)
		if err != nil {
			return nil, err
		}
		Migrate(s)
		m.state = s
	}

	return deepCopyState(m.state), nil
}

// deepCopyState returns a deep copy of s such that mutating the returned struct
// does not affect s. Pointer fields, slices, and maps are all duplicated.
func deepCopyState(s *State) *State {
	if s == nil {
		return nil
	}
	// Marshal and unmarshal is the simplest correct deep-copy for this struct.
	data, err := json.Marshal(s)
	if err != nil {
		// Should never happen with our own State struct; panic to surface the bug immediately.
		panic(fmt.Sprintf("deepCopyState: failed to marshal state: %v", err))
	}
	var cp State
	if err := json.Unmarshal(data, &cp); err != nil {
		// Should never happen with data we just marshaled from State.
		panic(fmt.Sprintf("deepCopyState: failed to unmarshal state: %v", err))
	}
	return &cp
}

// bindWorkspace performs the workspace entry-point guard under mu.Lock so that
// auto-bind (m.workspace == "") and mismatch checks are race-free.
// It must be called before any lock-free read of m.workspace, and callers
// must NOT hold mu when calling it.
func (m *StateManager) bindWorkspace(workspace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.workspace == "" {
		m.workspace = workspace
		return nil
	}
	if m.workspace != workspace {
		return fmt.Errorf("workspace mismatch: got %q, bound to %q", workspace, m.workspace)
	}
	return nil
}

// Get returns the string representation of field from state.json.
// field may use dot notation for sub-fields (e.g., "timestamps.created").
// Boolean and numeric values are rendered as their JSON string equivalents.
// Null pointer fields are rendered as "null".
func (m *StateManager) Get(workspace, field string) (string, error) {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return "", err
	}

	if !allowedGetFields[field] {
		return "", fmt.Errorf("Get: unknown field %q", field)
	}

	s, err := m.GetState()
	if err != nil {
		return "", err
	}

	return getField(s, field)
}

// getField extracts a field from a State by name using a switch over all
// allowed field names, including dot-notation sub-fields.
//
//nolint:gocyclo // complexity is inherent in the dispatch table (one case per field)
func getField(s *State, field string) (string, error) {
	switch field {
	case "version":
		return strconv.Itoa(s.Version), nil
	case "forge-state-mcp-version":
		return s.MCPVersion, nil
	case "specName":
		return s.SpecName, nil
	case "workspace":
		return s.Workspace, nil
	case "branch":
		if s.Branch == nil {
			return "null", nil
		}
		return *s.Branch, nil
	case "effort":
		if s.Effort == nil {
			return "null", nil
		}
		return *s.Effort, nil
	case "flowTemplate":
		if s.FlowTemplate == nil {
			return "null", nil
		}
		return *s.FlowTemplate, nil
	case "autoApprove":
		return strconv.FormatBool(s.AutoApprove), nil
	case "skipPr":
		return strconv.FormatBool(s.SkipPr), nil
	case "useCurrentBranch":
		return strconv.FormatBool(s.UseCurrentBranch), nil
	case "debug":
		return strconv.FormatBool(s.Debug), nil
	case "skippedPhases":
		return marshalJSON(s.SkippedPhases)
	case "currentPhase":
		return s.CurrentPhase, nil
	case "currentPhaseStatus":
		return s.CurrentPhaseStatus, nil
	case "completedPhases":
		return marshalJSON(s.CompletedPhases)
	case "revisions":
		return marshalJSON(s.Revisions)
	case "revisions.designRevisions":
		return strconv.Itoa(s.Revisions.DesignRevisions), nil
	case "revisions.taskRevisions":
		return strconv.Itoa(s.Revisions.TaskRevisions), nil
	case "revisions.designInlineRevisions":
		return strconv.Itoa(s.Revisions.DesignInlineRevisions), nil
	case "revisions.taskInlineRevisions":
		return strconv.Itoa(s.Revisions.TaskInlineRevisions), nil
	case "checkpointRevisionPending":
		return marshalJSON(s.CheckpointRevisionPending)
	case "tasks":
		return marshalJSON(s.Tasks)
	case "phaseLog":
		return marshalJSON(s.PhaseLog)
	case "timestamps":
		return marshalJSON(s.Timestamps)
	case "timestamps.created":
		return s.Timestamps.Created, nil
	case "timestamps.lastUpdated":
		return s.Timestamps.LastUpdated, nil
	case "timestamps.phaseStarted":
		if s.Timestamps.PhaseStarted == nil {
			return "null", nil
		}
		return *s.Timestamps.PhaseStarted, nil
	case "error":
		if s.Error == nil {
			return "null", nil
		}
		return marshalJSON(s.Error)
	default:
		return "", fmt.Errorf("getField: unknown field %q", field)
	}
}

// marshalJSON encodes v to a compact JSON string.
func marshalJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshalJSON: %w", err)
	}
	return string(data), nil
}

// PhaseStart marks phase as in_progress, equivalent to cmd_phase_start.
func (m *StateManager) PhaseStart(workspace, phase string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsPhase(phase) {
		return fmt.Errorf("PhaseStart: invalid phase %q", phase)
	}

	return m.Update(func(s *State) error {
		ts := nowISO()
		s.CurrentPhase = phase
		s.CurrentPhaseStatus = StatusInProgress
		s.Timestamps.PhaseStarted = &ts
		s.Error = nil
		return nil
	})
}

// PhaseComplete marks phase as completed and advances currentPhase to the
// next phase in ValidPhases, equivalent to cmd_phase_complete.
func (m *StateManager) PhaseComplete(workspace, phase string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsPhase(phase) {
		return fmt.Errorf("PhaseComplete: invalid phase %q", phase)
	}

	return m.Update(func(s *State) error {
		next := nextPhase(phase)

		// Add to completedPhases (deduplicated).
		s.CompletedPhases = appendUnique(s.CompletedPhases, phase)
		s.CurrentPhase = next
		if next == PhaseCompleted {
			s.CurrentPhaseStatus = StatusCompleted
		} else {
			s.CurrentPhaseStatus = StatusPending
		}
		s.Timestamps.PhaseStarted = nil
		return nil
	})
}

// PhaseFail records a phase failure with message, equivalent to cmd_phase_fail.
func (m *StateManager) PhaseFail(workspace, phase, message string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsPhase(phase) {
		return fmt.Errorf("PhaseFail: invalid phase %q", phase)
	}

	return m.Update(func(s *State) error {
		ts := nowISO()
		s.CurrentPhaseStatus = StatusFailed
		s.Error = &PhaseError{
			Phase:     phase,
			Message:   message,
			Timestamp: ts,
		}
		return nil
	})
}

// Checkpoint marks phase as awaiting_human, equivalent to _do_checkpoint.
// Accepts checkpoint-a, checkpoint-b, and any phase that matches the current
// phase in state (e.g., post-to-source when the engine returns a checkpoint
// action for Jira posting).
func (m *StateManager) Checkpoint(workspace, phase string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if phase != PhaseCheckpointA && phase != PhaseCheckpointB {
		s, err := readState(workspace)
		if err != nil {
			return fmt.Errorf("Checkpoint: read state: %w", err)
		}
		if s.CurrentPhase != phase {
			return fmt.Errorf("Checkpoint: invalid phase %q (expected checkpoint-a, checkpoint-b, or current phase %q)", phase, s.CurrentPhase)
		}
	}

	return m.Update(func(s *State) error {
		s.CurrentPhase = phase
		s.CurrentPhaseStatus = StatusAwaitingHuman
		return nil
	})
}

// Abandon sets currentPhaseStatus to "abandoned", equivalent to _do_abandon.
func (m *StateManager) Abandon(workspace string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		s.CurrentPhaseStatus = StatusAbandoned
		return nil
	})
}

// SkipPhase adds phase to skippedPhases and advances currentPhase,
// equivalent to _do_skip_phase.
func (m *StateManager) SkipPhase(workspace, phase string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsPhase(phase) {
		return fmt.Errorf("SkipPhase: invalid phase %q", phase)
	}

	return m.Update(func(s *State) error {
		next := nextPhase(phase)
		s.SkippedPhases = appendUnique(s.SkippedPhases, phase)
		s.CurrentPhase = next
		s.CurrentPhaseStatus = StatusPending
		return nil
	})
}

// RevisionBump increments the design or task revision counter,
// equivalent to _do_revision_bump.
func (m *StateManager) RevisionBump(workspace, revType string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsRevType(revType) {
		return fmt.Errorf("RevisionBump: unknown revision type %q (expected: %s)",
			revType, strings.Join(ValidRevTypes, ", "))
	}

	return m.Update(func(s *State) error {
		switch revType {
		case RevTypeDesign:
			s.Revisions.DesignRevisions++
		case RevTypeTasks:
			s.Revisions.TaskRevisions++
		}
		return nil
	})
}

// InlineRevisionBump increments the design or task inline revision counter,
// equivalent to _do_inline_revision_bump.
func (m *StateManager) InlineRevisionBump(workspace, revType string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsRevType(revType) {
		return fmt.Errorf("InlineRevisionBump: unknown revision type %q (expected: %s)",
			revType, strings.Join(ValidRevTypes, ", "))
	}

	return m.Update(func(s *State) error {
		switch revType {
		case RevTypeDesign:
			s.Revisions.DesignInlineRevisions++
		case RevTypeTasks:
			s.Revisions.TaskInlineRevisions++
		}
		return nil
	})
}

// PipelineConfig holds the initial pipeline configuration values applied after Init.
// Using Configure instead of individual setters reduces disk I/O to a single write.
type PipelineConfig struct {
	Effort           string
	FlowTemplate     string
	AutoApprove      bool
	SkipPR           bool
	Debug            bool
	UseCurrentBranch bool
	Branch           string // only used when UseCurrentBranch is true
	SkippedPhases    []string
}

// Configure applies the initial pipeline configuration in a single write to state.json,
// replacing separate SetEffort/SetFlowTemplate/SkipPhase calls that would
// each trigger their own read-modify-write disk cycle.
func (m *StateManager) Configure(workspace string, cfg PipelineConfig) error {
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	// Validate before writing.
	if !containsEffort(cfg.Effort) {
		return fmt.Errorf("Configure: invalid effort %q (expected: %s)",
			cfg.Effort, strings.Join(ValidEfforts, ", "))
	}
	if !containsTemplate(cfg.FlowTemplate) {
		return fmt.Errorf("Configure: invalid flowTemplate %q (expected: %s)",
			cfg.FlowTemplate, strings.Join(ValidTemplates, ", "))
	}
	for _, phase := range cfg.SkippedPhases {
		if !containsPhase(phase) {
			return fmt.Errorf("Configure: invalid phase %q", phase)
		}
	}

	return m.Update(func(s *State) error {
		s.Effort = &cfg.Effort
		s.FlowTemplate = &cfg.FlowTemplate
		if cfg.AutoApprove {
			s.AutoApprove = true
		}
		if cfg.SkipPR {
			s.SkipPr = true
		}
		if cfg.Debug {
			s.Debug = true
		}
		if cfg.UseCurrentBranch {
			s.UseCurrentBranch = true
		}
		if cfg.Branch != "" {
			s.Branch = &cfg.Branch
		}
		for _, phase := range cfg.SkippedPhases {
			s.SkippedPhases = appendUnique(s.SkippedPhases, phase)
		}
		// Advance currentPhase only if it lands on a skipped phase.
		for s.CurrentPhase != PhaseCompleted && slices.Contains(s.SkippedPhases, s.CurrentPhase) {
			s.CurrentPhase = nextPhase(s.CurrentPhase)
			if s.CurrentPhase == PhaseCompleted {
				s.CurrentPhaseStatus = StatusCompleted
			} else {
				s.CurrentPhaseStatus = StatusPending
			}
		}
		return nil
	})
}

// SetBranch sets the branch field, equivalent to _do_set_branch.
func (m *StateManager) SetBranch(workspace, branch string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		s.Branch = &branch
		return nil
	})
}

// SetEffort validates and sets the effort field, equivalent to _do_set_effort.
// Returns error for values outside ValidEfforts.
func (m *StateManager) SetEffort(workspace, effort string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsEffort(effort) {
		return fmt.Errorf("SetEffort: invalid effort %q (expected: %s)",
			effort, strings.Join(ValidEfforts, ", "))
	}

	return m.Update(func(s *State) error {
		s.Effort = &effort
		return nil
	})
}

// SetFlowTemplate validates and sets the flowTemplate field,
// equivalent to _do_set_flow_template.
func (m *StateManager) SetFlowTemplate(workspace, flowTemplate string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	if !containsTemplate(flowTemplate) {
		return fmt.Errorf("SetFlowTemplate: invalid flowTemplate %q (expected: %s)",
			flowTemplate, strings.Join(ValidTemplates, ", "))
	}

	return m.Update(func(s *State) error {
		s.FlowTemplate = &flowTemplate
		return nil
	})
}

// SetAutoApprove sets autoApprove = true, equivalent to _do_set_auto_approve.
func (m *StateManager) SetAutoApprove(workspace string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		s.AutoApprove = true
		return nil
	})
}

// SetSkipPr sets skipPr = true, equivalent to _do_set_skip_pr.
func (m *StateManager) SetSkipPr(workspace string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		s.SkipPr = true
		return nil
	})
}

// SetDebug sets debug = true, equivalent to _do_set_debug.
func (m *StateManager) SetDebug(workspace string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		s.Debug = true
		return nil
	})
}

// SetUseCurrentBranch sets useCurrentBranch = true and branch = branch,
// equivalent to _do_set_use_current_branch.
func (m *StateManager) SetUseCurrentBranch(workspace, branch string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		s.UseCurrentBranch = true
		s.Branch = &branch
		return nil
	})
}

// SetRevisionPending sets checkpointRevisionPending[checkpoint] = true.
// Only "checkpoint-a" and "checkpoint-b" are valid checkpoint values.
func (m *StateManager) SetRevisionPending(workspace, checkpoint string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		return applyRevisionPending(s, checkpoint, true)
	})
}

// ClearRevisionPending sets checkpointRevisionPending[checkpoint] = false.
// Only "checkpoint-a" and "checkpoint-b" are valid checkpoint values.
func (m *StateManager) ClearRevisionPending(workspace, checkpoint string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		return applyRevisionPending(s, checkpoint, false)
	})
}

// applyRevisionPending is the shared body of SetRevisionPending and
// ClearRevisionPending. It mutates s directly, relying on the caller's
// Update closure to handle persistence. The body contains no calls to
// readState or writeState.
func applyRevisionPending(s *State, checkpoint string, value bool) error {
	if checkpoint != PhaseCheckpointA && checkpoint != PhaseCheckpointB {
		return fmt.Errorf("invalid checkpoint %q (expected: %s, %s)", checkpoint, PhaseCheckpointA, PhaseCheckpointB)
	}

	if s.CheckpointRevisionPending == nil {
		s.CheckpointRevisionPending = map[string]bool{}
	}
	s.CheckpointRevisionPending[checkpoint] = value
	return nil
}

// TaskInit stores all supplied tasks and writes state.json,
// equivalent to _do_task_init.
func (m *StateManager) TaskInit(workspace string, tasks map[string]Task) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		s.Tasks = tasks
		return nil
	})
}

// TaskUpdate modifies a single field within the named task entry,
// equivalent to _do_task_update.
func (m *StateManager) TaskUpdate(workspace, taskNum, field, value string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		task, ok := s.Tasks[taskNum]
		if !ok {
			return fmt.Errorf("TaskUpdate: task %q not found", taskNum)
		}

		switch field {
		case TaskFieldImplStatus:
			task.ImplStatus = value
		case TaskFieldReviewStatus:
			task.ReviewStatus = value
		case TaskFieldExecutionMode:
			task.ExecutionMode = value
		case TaskFieldTitle:
			task.Title = value
		case TaskFieldImplRetries:
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("TaskUpdate: %s value %q is not an integer: %w", TaskFieldImplRetries, value, err)
			}
			task.ImplRetries = n
		case TaskFieldReviewRetries:
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("TaskUpdate: %s value %q is not an integer: %w", TaskFieldReviewRetries, value, err)
			}
			task.ReviewRetries = n
		default:
			return fmt.Errorf("TaskUpdate: unknown field %q", field)
		}

		s.Tasks[taskNum] = task
		return nil
	})
}

// PhaseLog appends a metrics entry to the phaseLog array,
// equivalent to _do_phase_log.
func (m *StateManager) PhaseLog(workspace, phase string, tokens, durationMs int, model string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	return m.Update(func(s *State) error {
		ts := nowISO()
		entry := PhaseLogEntry{
			Phase:      phase,
			Tokens:     tokens,
			DurationMs: durationMs,
			Model:      model,
			Timestamp:  ts,
		}
		s.PhaseLog = append(s.PhaseLog, entry)
		return nil
	})
}

// PhaseStatsResult is the structured return value for PhaseStats.
type PhaseStatsResult struct {
	TotalTokens     int             `json:"totalTokens"`
	TotalDurationMs int             `json:"totalDurationMs"`
	Entries         []PhaseLogEntry `json:"entries"`
}

// PhaseStats aggregates phaseLog metrics, equivalent to cmd_phase_stats.
// It is read-only.
func (m *StateManager) PhaseStats(workspace string) (*PhaseStatsResult, error) {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return nil, err
	}

	s, err := m.GetState()
	if err != nil {
		return nil, err
	}

	result := &PhaseStatsResult{
		Entries: s.PhaseLog,
	}
	for _, entry := range s.PhaseLog {
		result.TotalTokens += entry.Tokens
		result.TotalDurationMs += entry.DurationMs
	}
	return result, nil
}

// ResumeInfoResult mirrors the JSON object produced by cmd_resume_info.
type ResumeInfoResult struct {
	CurrentPhase              string          `json:"currentPhase"`
	CurrentPhaseStatus        string          `json:"currentPhaseStatus"`
	CompletedPhases           []string        `json:"completedPhases"`
	SkippedPhases             []string        `json:"skippedPhases"`
	Effort                    *string         `json:"effort"`
	FlowTemplate              *string         `json:"flowTemplate"`
	AutoApprove               bool            `json:"autoApprove"`
	SkipPr                    bool            `json:"skipPr"`
	UseCurrentBranch          bool            `json:"useCurrentBranch"`
	Branch                    *string         `json:"branch"`
	SpecName                  string          `json:"specName"`
	Revisions                 Revisions       `json:"revisions"`
	Error                     *PhaseError     `json:"error"`
	PendingTasks              []string        `json:"pendingTasks"`
	CompletedTasks            []string        `json:"completedTasks"`
	TotalTasks                int             `json:"totalTasks"`
	PhaseLogEntries           int             `json:"phaseLogEntries"`
	TotalTokens               int             `json:"totalTokens"`
	TotalDurationMs           int             `json:"totalDuration_ms"`
	Debug                     bool            `json:"debug"`
	TasksWithRetries          []TaskRetryInfo `json:"tasksWithRetries"`
	CheckpointRevisionPending map[string]bool `json:"checkpointRevisionPending"`
}

// TaskRetryInfo is used within ResumeInfoResult.
type TaskRetryInfo struct {
	Task          string `json:"task"`
	ImplRetries   int    `json:"implRetries"`
	ReviewRetries int    `json:"reviewRetries"`
}

// ResumeInfo returns a summary of state for the orchestrator,
// equivalent to cmd_resume_info.  It is read-only.
func (m *StateManager) ResumeInfo(workspace string) (*ResumeInfoResult, error) {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return nil, err
	}

	s, err := m.GetState()
	if err != nil {
		return nil, err
	}

	skipped := s.SkippedPhases
	if skipped == nil {
		skipped = []string{}
	}

	// Pending tasks: implStatus != "completed" OR reviewStatus == "completed_fail"
	pendingTasks := []string{}
	completedTasks := []string{}
	tasksWithRetries := []TaskRetryInfo{}

	for k, t := range s.Tasks {
		if t.ImplStatus != TaskStatusCompleted || t.ReviewStatus == TaskStatusCompletedFail {
			pendingTasks = append(pendingTasks, k)
		}
		if t.ReviewStatus == TaskStatusCompletedPass || t.ReviewStatus == TaskStatusCompletedPassNote {
			completedTasks = append(completedTasks, k)
		}
		if t.ImplRetries > 0 || t.ReviewRetries > 0 {
			tasksWithRetries = append(tasksWithRetries, TaskRetryInfo{
				Task:          k,
				ImplRetries:   t.ImplRetries,
				ReviewRetries: t.ReviewRetries,
			})
		}
	}

	totalTokens := 0
	totalDuration := 0
	for _, entry := range s.PhaseLog {
		totalTokens += entry.Tokens
		totalDuration += entry.DurationMs
	}

	crp := s.CheckpointRevisionPending
	if crp == nil {
		crp = map[string]bool{
			PhaseCheckpointA: false,
			PhaseCheckpointB: false,
		}
	}

	return &ResumeInfoResult{
		CurrentPhase:              s.CurrentPhase,
		CurrentPhaseStatus:        s.CurrentPhaseStatus,
		CompletedPhases:           s.CompletedPhases,
		SkippedPhases:             skipped,
		Effort:                    s.Effort,
		FlowTemplate:              s.FlowTemplate,
		AutoApprove:               s.AutoApprove,
		SkipPr:                    s.SkipPr,
		UseCurrentBranch:          s.UseCurrentBranch,
		Branch:                    s.Branch,
		SpecName:                  s.SpecName,
		Revisions:                 s.Revisions,
		Error:                     s.Error,
		PendingTasks:              pendingTasks,
		CompletedTasks:            completedTasks,
		TotalTasks:                len(s.Tasks),
		PhaseLogEntries:           len(s.PhaseLog),
		TotalTokens:               totalTokens,
		TotalDurationMs:           totalDuration,
		Debug:                     s.Debug,
		TasksWithRetries:          tasksWithRetries,
		CheckpointRevisionPending: crp,
	}, nil
}

// RefreshIndex rebuilds the specs index for the workspace,
// equivalent to cmd_refresh_index.  Implementation deferred to tools package,
// which calls indexer.BuildSpecsIndex to produce .specs/index.json.
func (m *StateManager) RefreshIndex(workspace string) error {
	// Workspace entry-point guard.
	if err := m.bindWorkspace(workspace); err != nil {
		return err
	}

	// Delegated to tools.RefreshIndexHandler, which calls indexer.BuildSpecsIndex.
	// Not implemented here to keep the state package dependency-free of the indexer package.
	return errors.New("RefreshIndex: not implemented in state package; use tools.RefreshIndexHandler")
}

// ---------- enum helpers ----------

func containsRevType(rt string) bool {
	return slices.Contains(ValidRevTypes, rt)
}

func containsEffort(e string) bool {
	return slices.Contains(ValidEfforts, e)
}

func containsTemplate(t string) bool {
	return slices.Contains(ValidTemplates, t)
}

// appendUnique appends s to slice only if it is not already present.
func appendUnique(slice []string, s string) []string {
	if slices.Contains(slice, s) {
		return slice
	}
	return append(slice, s)
}
