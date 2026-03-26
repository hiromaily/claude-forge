// Package tools implements MCP tool handlers that delegate to StateManager
// methods and enforce guard preconditions.
//
// Blocking guards return an MCP error response (IsError = true).
// Non-blocking warnings are included as a "warning" key in the JSON content.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/events"
	"github.com/hiromaily/claude-forge/mcp-server/state"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// defaultScriptPath resolves the path to build-specs-index.sh.
// FORGE_SCRIPTS_PATH env var takes precedence; otherwise the path is derived
// from this file's location at compile time; fallback is a relative path.
func defaultScriptPath() string {
	if p := os.Getenv("FORGE_SCRIPTS_PATH"); p != "" {
		return filepath.Join(p, "build-specs-index.sh")
	}
	_, file, _, ok := runtime.Caller(0)
	if ok {
		// file is .../mcp-server/tools/handlers.go
		repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
		candidate := filepath.Join(repoRoot, "scripts", "build-specs-index.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "scripts/build-specs-index.sh"
}

// ---------- response helpers ----------

// okText returns a successful result containing text.
func okText(text string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(text), nil
}

// okJSON serialises v to JSON and returns a successful result.
func okJSON(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return errorf("marshal result: %v", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// okWithWarning returns a success result that includes the warning message
// under the "warning" key in JSON content.
func okWithWarning(msg, warning string) (*mcp.CallToolResult, error) {
	payload := map[string]string{"result": msg, "warning": warning}
	data, _ := json.Marshal(payload)
	return mcp.NewToolResultText(string(data)), nil
}

// errorf returns an MCP error result (IsError=true) with a formatted message.
func errorf(format string, args ...any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...)), nil
}

// blockGuard returns an error result for a blocking guard violation.
func blockGuard(err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(err.Error()), nil
}

// ---------- init ----------

// InitHandler handles the "init" MCP tool.
// Accepts: workspace (string), spec_name (string), validated (bool).
// Guard: GuardInitValidated must be true.
func InitHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace := req.GetString("workspace", "")
		if workspace == "" {
			return errorf("workspace parameter is required")
		}
		specName := req.GetString("spec_name", "")
		if specName == "" {
			return errorf("spec_name parameter is required")
		}
		validated := req.GetBool("validated", false)
		if err := GuardInitValidated(validated); err != nil {
			return blockGuard(err)
		}
		if err := sm.Init(workspace, specName); err != nil {
			return errorf("init: %v", err)
		}
		return okText("ok")
	}
}

// ---------- get ----------

// GetHandler handles the "get" MCP tool.
// Accepts: workspace (string), field (string).
func GetHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		field, err := req.RequireString("field")
		if err != nil {
			return errorf("%v", err)
		}
		val, err := sm.Get(workspace, field)
		if err != nil {
			return errorf("get: %v", err)
		}
		return okText(val)
	}
}

// ---------- phase_start ----------

// PhaseStartHandler handles the "phase_start" MCP tool.
// Accepts: workspace (string), phase (string).
// Guard 3c: phase-5 requires non-empty tasks.
func PhaseStartHandler(sm *state.StateManager, bus *events.EventBus) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		phase, err := req.RequireString("phase")
		if err != nil {
			return errorf("%v", err)
		}
		// Guard 3c: blocking
		s, serr := loadState(workspace)
		if serr != nil {
			return errorf("load state: %v", serr)
		}
		if gerr := Guard3cTasksNonEmpty(phase, s); gerr != nil {
			return blockGuard(gerr)
		}
		if err := sm.PhaseStart(workspace, phase); err != nil {
			return errorf("phase_start: %v", err)
		}
		e := events.Event{
			Event:     "phase-start",
			Phase:     phase,
			SpecName:  s.SpecName,
			Workspace: workspace,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Outcome:   "in_progress",
		}
		bus.Publish(e)
		return okText("ok")
	}
}

// ---------- phase_complete ----------

// PhaseCompleteHandler handles the "phase_complete" MCP tool.
// Accepts: workspace (string), phase (string).
// Guards: 3a (artifact), 3e (awaiting_human), 3j (revision pending).
// Warnings: 3f (phase-log missing), 3i (not in_progress).
func PhaseCompleteHandler(sm *state.StateManager, bus *events.EventBus, slack *events.SlackNotifier) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		phase, err := req.RequireString("phase")
		if err != nil {
			return errorf("%v", err)
		}
		s, serr := loadState(workspace)
		if serr != nil {
			return errorf("load state: %v", serr)
		}
		// Blocking guards.
		if gerr := Guard3aArtifactExists(workspace, phase, s); gerr != nil {
			return blockGuard(gerr)
		}
		if gerr := Guard3eCheckpointAwaitingHuman(phase, s); gerr != nil {
			return blockGuard(gerr)
		}
		if gerr := Guard3jCheckpointRevisionPending(phase, s); gerr != nil {
			return blockGuard(gerr)
		}
		// Non-blocking warnings.
		var warnings []string
		if w := Warn3fPhaseLogMissing(phase, s); w != "" {
			warnings = append(warnings, w)
		}
		if w := Warn3iPhaseNotInProgress(s); w != "" {
			warnings = append(warnings, w)
		}
		if err := sm.PhaseComplete(workspace, phase); err != nil {
			return errorf("phase_complete: %v", err)
		}
		e := events.Event{
			Event:     "phase-complete",
			Phase:     phase,
			SpecName:  s.SpecName,
			Workspace: workspace,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Outcome:   "completed",
		}
		bus.Publish(e)
		slack.Notify(e)
		if len(warnings) > 0 {
			return okWithWarning("ok", strings.Join(warnings, "; "))
		}
		return okText("ok")
	}
}

// ---------- phase_fail ----------

// PhaseFailHandler handles the "phase_fail" MCP tool.
// Accepts: workspace (string), phase (string), message (string).
func PhaseFailHandler(sm *state.StateManager, bus *events.EventBus, slack *events.SlackNotifier) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		phase, err := req.RequireString("phase")
		if err != nil {
			return errorf("%v", err)
		}
		message := req.GetString("message", "")
		// Load state for event specName before mutation.
		s, serr := loadState(workspace)
		specName := ""
		if serr == nil {
			specName = s.SpecName
		}
		if err := sm.PhaseFail(workspace, phase, message); err != nil {
			return errorf("phase_fail: %v", err)
		}
		e := events.Event{
			Event:     "phase-fail",
			Phase:     phase,
			SpecName:  specName,
			Workspace: workspace,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Outcome:   "failed",
		}
		bus.Publish(e)
		slack.Notify(e)
		return okText("ok")
	}
}

// ---------- checkpoint ----------

// CheckpointHandler handles the "checkpoint" MCP tool.
// Accepts: workspace (string), phase (string).
func CheckpointHandler(sm *state.StateManager, bus *events.EventBus) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		phase, err := req.RequireString("phase")
		if err != nil {
			return errorf("%v", err)
		}
		// Load state for event specName before mutation.
		s, serr := loadState(workspace)
		specName := ""
		if serr == nil {
			specName = s.SpecName
		}
		if err := sm.Checkpoint(workspace, phase); err != nil {
			return errorf("checkpoint: %v", err)
		}
		e := events.Event{
			Event:     "checkpoint",
			Phase:     phase,
			SpecName:  specName,
			Workspace: workspace,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Outcome:   "awaiting_human",
		}
		bus.Publish(e)
		return okText("ok")
	}
}

// ---------- task_init ----------

// TaskInitHandler handles the "task_init" MCP tool.
// Accepts: workspace (string), tasks (object).
// Guard 3g: checkpoint-b must be completed or skipped.
func TaskInitHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		s, serr := loadState(workspace)
		if serr != nil {
			return errorf("load state: %v", serr)
		}
		// Guard 3g: blocking.
		if gerr := Guard3gCheckpointBDoneOrSkipped(s); gerr != nil {
			return blockGuard(gerr)
		}
		// Parse tasks from arguments.
		args := req.GetArguments()
		tasksRaw, ok := args["tasks"]
		if !ok {
			return errorf("tasks parameter is required")
		}
		tasksData, err := json.Marshal(tasksRaw)
		if err != nil {
			return errorf("marshal tasks: %v", err)
		}
		var tasks map[string]state.Task
		if err := json.Unmarshal(tasksData, &tasks); err != nil {
			return errorf("unmarshal tasks: %v", err)
		}
		if err := sm.TaskInit(workspace, tasks); err != nil {
			return errorf("task_init: %v", err)
		}
		return okText("ok")
	}
}

// ---------- task_update ----------

// TaskUpdateHandler handles the "task_update" MCP tool.
// Accepts: workspace (string), task_num (string), field (string), value (string).
// Guard 3b: review-{N}.md must exist when setting reviewStatus to completed_pass.
// Warning 3h: task not found in state.Tasks.
func TaskUpdateHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		taskNum, err := req.RequireString("task_num")
		if err != nil {
			return errorf("%v", err)
		}
		field, err := req.RequireString("field")
		if err != nil {
			return errorf("%v", err)
		}
		value, err := req.RequireString("value")
		if err != nil {
			return errorf("%v", err)
		}
		s, serr := loadState(workspace)
		if serr != nil {
			return errorf("load state: %v", serr)
		}
		// Guard 3b: blocking.
		if gerr := Guard3bReviewFileExists(workspace, taskNum, value, s); gerr != nil {
			if field == "reviewStatus" {
				return blockGuard(gerr)
			}
		}
		// Warning 3h: non-blocking.
		var warning string
		if w := Warn3hTaskNotFound(taskNum, s); w != "" {
			warning = w
		}
		if err := sm.TaskUpdate(workspace, taskNum, field, value); err != nil {
			return errorf("task_update: %v", err)
		}
		if warning != "" {
			return okWithWarning("ok", warning)
		}
		return okText("ok")
	}
}

// ---------- revision_bump ----------

// RevisionBumpHandler handles the "revision_bump" MCP tool.
// Accepts: workspace (string), rev_type (string).
func RevisionBumpHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		revType, err := req.RequireString("rev_type")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.RevisionBump(workspace, revType); err != nil {
			return errorf("revision_bump: %v", err)
		}
		return okText("ok")
	}
}

// ---------- inline_revision_bump ----------

// InlineRevisionBumpHandler handles the "inline_revision_bump" MCP tool.
// Accepts: workspace (string), rev_type (string).
func InlineRevisionBumpHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		revType, err := req.RequireString("rev_type")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.InlineRevisionBump(workspace, revType); err != nil {
			return errorf("inline_revision_bump: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_branch ----------

// SetBranchHandler handles the "set_branch" MCP tool.
// Accepts: workspace (string), branch (string).
func SetBranchHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		branch, err := req.RequireString("branch")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetBranch(workspace, branch); err != nil {
			return errorf("set_branch: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_task_type ----------

// SetTaskTypeHandler handles the "set_task_type" MCP tool.
// Accepts: workspace (string), task_type (string).
func SetTaskTypeHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		taskType, err := req.RequireString("task_type")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetTaskType(workspace, taskType); err != nil {
			return errorf("set_task_type: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_effort ----------

// SetEffortHandler handles the "set_effort" MCP tool.
// Accepts: workspace (string), effort (string).
func SetEffortHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		effort, err := req.RequireString("effort")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetEffort(workspace, effort); err != nil {
			return errorf("set_effort: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_flow_template ----------

// SetFlowTemplateHandler handles the "set_flow_template" MCP tool.
// Accepts: workspace (string), flow_template (string).
func SetFlowTemplateHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		tmpl, err := req.RequireString("flow_template")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetFlowTemplate(workspace, tmpl); err != nil {
			return errorf("set_flow_template: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_auto_approve ----------

// SetAutoApproveHandler handles the "set_auto_approve" MCP tool.
// Accepts: workspace (string).
func SetAutoApproveHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetAutoApprove(workspace); err != nil {
			return errorf("set_auto_approve: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_skip_pr ----------

// SetSkipPrHandler handles the "set_skip_pr" MCP tool.
// Accepts: workspace (string).
func SetSkipPrHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetSkipPr(workspace); err != nil {
			return errorf("set_skip_pr: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_debug ----------

// SetDebugHandler handles the "set_debug" MCP tool.
// Accepts: workspace (string).
func SetDebugHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetDebug(workspace); err != nil {
			return errorf("set_debug: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_use_current_branch ----------

// SetUseCurrentBranchHandler handles the "set_use_current_branch" MCP tool.
// Accepts: workspace (string), branch (string).
func SetUseCurrentBranchHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		branch, err := req.RequireString("branch")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetUseCurrentBranch(workspace, branch); err != nil {
			return errorf("set_use_current_branch: %v", err)
		}
		return okText("ok")
	}
}

// ---------- set_revision_pending ----------

// SetRevisionPendingHandler handles the "set_revision_pending" MCP tool.
// Accepts: workspace (string), checkpoint (string).
func SetRevisionPendingHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		checkpoint, err := req.RequireString("checkpoint")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SetRevisionPending(workspace, checkpoint); err != nil {
			return errorf("set_revision_pending: %v", err)
		}
		return okText("ok")
	}
}

// ---------- clear_revision_pending ----------

// ClearRevisionPendingHandler handles the "clear_revision_pending" MCP tool.
// Accepts: workspace (string), checkpoint (string).
func ClearRevisionPendingHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		checkpoint, err := req.RequireString("checkpoint")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.ClearRevisionPending(workspace, checkpoint); err != nil {
			return errorf("clear_revision_pending: %v", err)
		}
		return okText("ok")
	}
}

// ---------- skip_phase ----------

// SkipPhaseHandler handles the "skip_phase" MCP tool.
// Accepts: workspace (string), phase (string).
func SkipPhaseHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		phase, err := req.RequireString("phase")
		if err != nil {
			return errorf("%v", err)
		}
		if err := sm.SkipPhase(workspace, phase); err != nil {
			return errorf("skip_phase: %v", err)
		}
		return okText("ok")
	}
}

// ---------- phase_log ----------

// PhaseLogHandler handles the "phase_log" MCP tool.
// Accepts: workspace (string), phase (string), tokens (number),
//
//	duration_ms (number), model (string).
//
// Warning 3d: duplicate phase-log entry.
func PhaseLogHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		phase, err := req.RequireString("phase")
		if err != nil {
			return errorf("%v", err)
		}
		tokens := req.GetInt("tokens", 0)
		durationMs := req.GetInt("duration_ms", 0)
		model := req.GetString("model", "")

		s, serr := loadState(workspace)
		if serr != nil {
			return errorf("load state: %v", serr)
		}
		// Warning 3d: non-blocking.
		warning := Warn3dPhaseLogDuplicate(phase, s)

		if err := sm.PhaseLog(workspace, phase, tokens, durationMs, model); err != nil {
			return errorf("phase_log: %v", err)
		}
		if warning != "" {
			return okWithWarning("ok", warning)
		}
		return okText("ok")
	}
}

// ---------- phase_stats ----------

// PhaseStatsHandler handles the "phase_stats" MCP tool.
// Accepts: workspace (string).
func PhaseStatsHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		result, err := sm.PhaseStats(workspace)
		if err != nil {
			return errorf("phase_stats: %v", err)
		}
		return okJSON(result)
	}
}

// ---------- abandon ----------

// AbandonHandler handles the "abandon" MCP tool.
// Accepts: workspace (string).
func AbandonHandler(sm *state.StateManager, bus *events.EventBus, slack *events.SlackNotifier) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		// Load state for event specName and phase before mutation.
		s, serr := loadState(workspace)
		specName := ""
		phase := ""
		if serr == nil {
			specName = s.SpecName
			phase = s.CurrentPhase
		}
		if err := sm.Abandon(workspace); err != nil {
			return errorf("abandon: %v", err)
		}
		e := events.Event{
			Event:     "abandon",
			Phase:     phase,
			SpecName:  specName,
			Workspace: workspace,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Outcome:   "abandoned",
		}
		bus.Publish(e)
		slack.Notify(e)
		return okText("ok")
	}
}

// ---------- resume_info ----------

// ResumeInfoHandler handles the "resume_info" MCP tool.
// Accepts: workspace (string).
func ResumeInfoHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		result, err := sm.ResumeInfo(workspace)
		if err != nil {
			return errorf("resume_info: %v", err)
		}
		return okJSON(result)
	}
}

// ---------- refresh_index ----------

// RefreshIndexHandler handles the "refresh_index" MCP tool using the default
// script path resolved at init time.
func RefreshIndexHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return RefreshIndexHandlerWithScript(sm, defaultScriptPath())
}

// RefreshIndexHandlerWithScript is the testable variant that accepts an explicit
// script path.  It executes the script via os/exec and returns an error response
// if the script exits non-zero.  It never re-implements the script logic in Go.
func RefreshIndexHandlerWithScript(sm *state.StateManager, scriptPath string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		cmd := exec.CommandContext(ctx, "bash", scriptPath, workspace)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errorf("refresh_index: script failed: %v\n%s", err, string(out))
		}
		return okText("ok")
	}
}

// ---------- internal helpers ----------

// loadState reads state.json from workspace without locking (handler-level read
// for guard checks).  The StateManager methods do their own locking for mutations.
func loadState(workspace string) (*state.State, error) {
	return state.ReadState(workspace)
}
