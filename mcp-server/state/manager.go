// Package state implements the StateManager that provides all state mutation
// and query operations for the forge-state MCP server. It is the Go equivalent
// of scripts/state-manager.sh, with a sync.RWMutex replacing file-based locking.
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
// state-manager.sh commands.  All mutating methods acquire mu.Lock() for the
// full read-modify-write cycle; read-only methods use mu.RLock().
type StateManager struct {
	mu sync.RWMutex
}

// NewStateManager constructs a StateManager ready for use.
func NewStateManager() *StateManager {
	return &StateManager{}
}

// ---------- helpers ----------

func statePath(workspace string) string {
	return filepath.Join(workspace, "state.json")
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// readState reads and unmarshals state.json from workspace.
// The caller must hold an appropriate lock.
func readState(workspace string) (*State, error) {
	return ReadState(workspace)
}

// ReadState reads and unmarshals state.json from workspace.
// Exported so that tools package can reuse it without duplicating read logic.
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
	tmp := statePath(workspace) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
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
	return "completed"
}

// ---------- allowed Get fields ----------

// allowedGetFields is the set of top-level and dot-notation sub-fields that
// Get supports.  This mirrors what cmd_get does via `jq -r ".${field}"`.
var allowedGetFields = map[string]bool{
	"version":            true,
	"specName":           true,
	"workspace":          true,
	"branch":             true,
	"taskType":           true,
	"effort":             true,
	"flowTemplate":       true,
	"autoApprove":        true,
	"skipPr":             true,
	"useCurrentBranch":   true,
	"debug":              true,
	"skippedPhases":      true,
	"currentPhase":       true,
	"currentPhaseStatus": true,
	"completedPhases":    true,
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

// Init creates a new state.json in workspace following the exact schema
// produced by cmd_init in state-manager.sh.
func (m *StateManager) Init(workspace, specName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ts := nowISO()
	s := &State{
		Version:            1,
		SpecName:           specName,
		Workspace:          workspace,
		Branch:             nil,
		TaskType:           nil,
		Effort:             nil,
		FlowTemplate:       nil,
		AutoApprove:        false,
		SkipPr:             false,
		UseCurrentBranch:   false,
		Debug:              false,
		SkippedPhases:      []string{},
		CurrentPhase:       "phase-1",
		CurrentPhaseStatus: "pending",
		CompletedPhases:    []string{"setup"},
		Revisions: Revisions{
			DesignRevisions:       0,
			TaskRevisions:         0,
			DesignInlineRevisions: 0,
			TaskInlineRevisions:   0,
		},
		CheckpointRevisionPending: map[string]bool{
			"checkpoint-a": false,
			"checkpoint-b": false,
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
	return writeState(workspace, s)
}

// Get returns the string representation of field from state.json.
// field may use dot notation for sub-fields (e.g., "timestamps.created").
// Boolean and numeric values are rendered as their JSON string equivalents.
// Null pointer fields are rendered as "null".
func (m *StateManager) Get(workspace, field string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !allowedGetFields[field] {
		return "", fmt.Errorf("Get: unknown field %q", field)
	}

	s, err := readState(workspace)
	if err != nil {
		return "", err
	}

	return getField(s, field)
}

// getField extracts a field from a State by name using a switch over all
// allowed field names, including dot-notation sub-fields.
func getField(s *State, field string) (string, error) {
	switch field {
	case "version":
		return strconv.Itoa(s.Version), nil
	case "specName":
		return s.SpecName, nil
	case "workspace":
		return s.Workspace, nil
	case "branch":
		if s.Branch == nil {
			return "null", nil
		}
		return *s.Branch, nil
	case "taskType":
		if s.TaskType == nil {
			return "null", nil
		}
		return *s.TaskType, nil
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
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsPhase(phase) {
		return fmt.Errorf("PhaseStart: invalid phase %q", phase)
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.CurrentPhase = phase
	s.CurrentPhaseStatus = "in_progress"
	s.Timestamps.PhaseStarted = &ts
	s.Timestamps.LastUpdated = ts
	s.Error = nil

	return writeState(workspace, s)
}

// PhaseComplete marks phase as completed and advances currentPhase to the
// next phase in ValidPhases, equivalent to cmd_phase_complete.
func (m *StateManager) PhaseComplete(workspace, phase string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsPhase(phase) {
		return fmt.Errorf("PhaseComplete: invalid phase %q", phase)
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	next := nextPhase(phase)

	// Add to completedPhases (deduplicated).
	s.CompletedPhases = appendUnique(s.CompletedPhases, phase)
	s.CurrentPhase = next
	if next == "completed" {
		s.CurrentPhaseStatus = "completed"
	} else {
		s.CurrentPhaseStatus = "pending"
	}
	s.Timestamps.LastUpdated = ts
	s.Timestamps.PhaseStarted = nil

	return writeState(workspace, s)
}

// PhaseFail records a phase failure with message, equivalent to cmd_phase_fail.
func (m *StateManager) PhaseFail(workspace, phase, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsPhase(phase) {
		return fmt.Errorf("PhaseFail: invalid phase %q", phase)
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.CurrentPhaseStatus = "failed"
	s.Error = &PhaseError{
		Phase:     phase,
		Message:   message,
		Timestamp: ts,
	}
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// Checkpoint marks phase as awaiting_human, equivalent to _do_checkpoint.
// Only checkpoint-a and checkpoint-b are valid values.
func (m *StateManager) Checkpoint(workspace, phase string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if phase != "checkpoint-a" && phase != "checkpoint-b" {
		return fmt.Errorf("Checkpoint: invalid phase %q (expected checkpoint-a or checkpoint-b)", phase)
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.CurrentPhase = phase
	s.CurrentPhaseStatus = "awaiting_human"
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// Abandon sets currentPhaseStatus to "abandoned", equivalent to _do_abandon.
func (m *StateManager) Abandon(workspace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.CurrentPhaseStatus = "abandoned"
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SkipPhase adds phase to skippedPhases and advances currentPhase,
// equivalent to _do_skip_phase.
func (m *StateManager) SkipPhase(workspace, phase string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsPhase(phase) {
		return fmt.Errorf("SkipPhase: invalid phase %q", phase)
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	next := nextPhase(phase)

	s.SkippedPhases = appendUnique(s.SkippedPhases, phase)
	s.CurrentPhase = next
	s.CurrentPhaseStatus = "pending"
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// RevisionBump increments the design or task revision counter,
// equivalent to _do_revision_bump.
func (m *StateManager) RevisionBump(workspace, revType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsRevType(revType) {
		return fmt.Errorf("RevisionBump: unknown revision type %q (expected: %s)",
			revType, strings.Join(ValidRevTypes, ", "))
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	switch revType {
	case "design":
		s.Revisions.DesignRevisions++
	case "tasks":
		s.Revisions.TaskRevisions++
	}
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// InlineRevisionBump increments the design or task inline revision counter,
// equivalent to _do_inline_revision_bump.
func (m *StateManager) InlineRevisionBump(workspace, revType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsRevType(revType) {
		return fmt.Errorf("InlineRevisionBump: unknown revision type %q (expected: %s)",
			revType, strings.Join(ValidRevTypes, ", "))
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	switch revType {
	case "design":
		s.Revisions.DesignInlineRevisions++
	case "tasks":
		s.Revisions.TaskInlineRevisions++
	}
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetBranch sets the branch field, equivalent to _do_set_branch.
func (m *StateManager) SetBranch(workspace, branch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.Branch = &branch
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetTaskType sets the taskType field, equivalent to _do_set_task_type.
func (m *StateManager) SetTaskType(workspace, taskType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.TaskType = &taskType
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetEffort validates and sets the effort field, equivalent to _do_set_effort.
// Returns error for values outside ValidEfforts.
func (m *StateManager) SetEffort(workspace, effort string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsEffort(effort) {
		return fmt.Errorf("SetEffort: invalid effort %q (expected: %s)",
			effort, strings.Join(ValidEfforts, ", "))
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.Effort = &effort
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetFlowTemplate validates and sets the flowTemplate field,
// equivalent to _do_set_flow_template.
func (m *StateManager) SetFlowTemplate(workspace, flowTemplate string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !containsTemplate(flowTemplate) {
		return fmt.Errorf("SetFlowTemplate: invalid flowTemplate %q (expected: %s)",
			flowTemplate, strings.Join(ValidTemplates, ", "))
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.FlowTemplate = &flowTemplate
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetAutoApprove sets autoApprove = true, equivalent to _do_set_auto_approve.
func (m *StateManager) SetAutoApprove(workspace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.AutoApprove = true
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetSkipPr sets skipPr = true, equivalent to _do_set_skip_pr.
func (m *StateManager) SetSkipPr(workspace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.SkipPr = true
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetDebug sets debug = true, equivalent to _do_set_debug.
func (m *StateManager) SetDebug(workspace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.Debug = true
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetUseCurrentBranch sets useCurrentBranch = true and branch = branch,
// equivalent to _do_set_use_current_branch.
func (m *StateManager) SetUseCurrentBranch(workspace, branch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.UseCurrentBranch = true
	s.Branch = &branch
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// SetRevisionPending sets checkpointRevisionPending[checkpoint] = true.
// Only "checkpoint-a" and "checkpoint-b" are valid checkpoint values.
func (m *StateManager) SetRevisionPending(workspace, checkpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return updateRevisionPending(workspace, checkpoint, true)
}

// ClearRevisionPending sets checkpointRevisionPending[checkpoint] = false.
// Only "checkpoint-a" and "checkpoint-b" are valid checkpoint values.
func (m *StateManager) ClearRevisionPending(workspace, checkpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return updateRevisionPending(workspace, checkpoint, false)
}

// updateRevisionPending is the shared body of SetRevisionPending and
// ClearRevisionPending.  The caller must hold mu.Lock().
func updateRevisionPending(workspace, checkpoint string, value bool) error {
	if checkpoint != "checkpoint-a" && checkpoint != "checkpoint-b" {
		return fmt.Errorf("invalid checkpoint %q (expected: checkpoint-a, checkpoint-b)", checkpoint)
	}

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	if s.CheckpointRevisionPending == nil {
		s.CheckpointRevisionPending = map[string]bool{}
	}
	s.CheckpointRevisionPending[checkpoint] = value
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// TaskInit stores all supplied tasks and writes state.json,
// equivalent to _do_task_init.
func (m *StateManager) TaskInit(workspace string, tasks map[string]Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	s.Tasks = tasks
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// TaskUpdate modifies a single field within the named task entry,
// equivalent to _do_task_update.
func (m *StateManager) TaskUpdate(workspace, taskNum, field, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	task, ok := s.Tasks[taskNum]
	if !ok {
		return fmt.Errorf("TaskUpdate: task %q not found", taskNum)
	}

	ts := nowISO()
	switch field {
	case "implStatus":
		task.ImplStatus = value
	case "reviewStatus":
		task.ReviewStatus = value
	case "executionMode":
		task.ExecutionMode = value
	case "title":
		task.Title = value
	case "implRetries":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("TaskUpdate: implRetries value %q is not an integer: %w", value, err)
		}
		task.ImplRetries = n
	case "reviewRetries":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("TaskUpdate: reviewRetries value %q is not an integer: %w", value, err)
		}
		task.ReviewRetries = n
	default:
		return fmt.Errorf("TaskUpdate: unknown field %q", field)
	}

	s.Tasks[taskNum] = task
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
}

// PhaseLog appends a metrics entry to the phaseLog array,
// equivalent to _do_phase_log.
func (m *StateManager) PhaseLog(workspace, phase string, tokens, durationMs int, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := readState(workspace)
	if err != nil {
		return err
	}

	ts := nowISO()
	entry := PhaseLogEntry{
		Phase:      phase,
		Tokens:     tokens,
		DurationMs: durationMs,
		Model:      model,
		Timestamp:  ts,
	}
	s.PhaseLog = append(s.PhaseLog, entry)
	s.Timestamps.LastUpdated = ts

	return writeState(workspace, s)
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, err := readState(workspace)
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
	TaskType                  *string         `json:"taskType"`
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, err := readState(workspace)
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
		if t.ImplStatus != "completed" || t.ReviewStatus == "completed_fail" {
			pendingTasks = append(pendingTasks, k)
		}
		if t.ReviewStatus == "completed_pass" || t.ReviewStatus == "completed_pass_with_notes" {
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
			"checkpoint-a": false,
			"checkpoint-b": false,
		}
	}

	return &ResumeInfoResult{
		CurrentPhase:              s.CurrentPhase,
		CurrentPhaseStatus:        s.CurrentPhaseStatus,
		CompletedPhases:           s.CompletedPhases,
		SkippedPhases:             skipped,
		TaskType:                  s.TaskType,
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

// RefreshIndex executes build-specs-index.sh for the workspace,
// equivalent to cmd_refresh_index.  Implementation deferred to tools package.
func (m *StateManager) RefreshIndex(workspace string) error {
	// Delegated to tools.RefreshIndexHandler via os/exec.
	// Not implemented here to keep the state package dependency-free of os/exec.
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
