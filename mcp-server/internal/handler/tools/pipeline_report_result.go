// pipeline_report_result MCP handler.
// Records phase-log entry, validates artifacts, parses verdict, and advances state.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/handler/validation"
)

// reportResultOutcome is the typed result of the report-result core logic.
// It is returned by reportResultCore and consumed by both PipelineReportResultHandler
// (via handleReportResult) and PipelineNextActionHandler (P5 embedding).
// DisplayMessage is a pre-formatted completion line the orchestrator should output verbatim
// after a phase finishes (e.g. "  ✓ Complete  ·  1,847 tokens · 0:23").
// It is only set when NextActionHint is "proceed" and setup_only is false.
type reportResultOutcome struct {
	StateUpdated    bool                   `json:"state_updated"`
	ArtifactWritten string                 `json:"artifact_written"`
	VerdictParsed   string                 `json:"verdict_parsed"`
	Findings        []orchestrator.Finding `json:"findings"`
	NextActionHint  string                 `json:"next_action_hint"`
	Warning         string                 `json:"warning,omitempty"`
	DisplayMessage  string                 `json:"display_message,omitempty"`
}

// reportResultInput collects parsed parameters from the MCP request.
type reportResultInput struct {
	workspace  string
	phase      string
	tokensUsed int
	durationMs int
	model      string
	setupOnly  bool // when true, record phase-log but skip PhaseComplete
}

// PipelineReportResultHandler handles the "pipeline_report_result" MCP tool.
// It records a phase-log entry, validates the artifact, parses any verdict,
// and advances pipeline state accordingly.
// After a successful phase completion (NextActionHint == "proceed"), emits a
// "phase-complete" event to the dashboard.
func PipelineReportResultHandler(sm *state.StateManager, bus *events.EventBus, kb *history.KnowledgeBase) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Step 1: Parse required parameters.
		workspace, phase, result, err := requireWorkspaceAndPhase(req)
		if result != nil {
			return result, err
		}

		// Per-call StateManager: load fresh from disk to avoid stale-cache conflicts
		// with task state written by PipelineNextActionHandler's own per-call sm2
		// (e.g. executeTaskInit writes tasks via sm2, but the global sm cache predates
		// that write and would overwrite the tasks on the first sm.Update call).
		// This mirrors the pattern in PipelineNextActionHandler.
		sm2 := state.NewStateManager(sm.Version())
		if loadErr := sm2.LoadFromFile(workspace); loadErr != nil {
			return errorf("load state: %v", loadErr)
		}

		in := reportResultInput{
			workspace:  workspace,
			phase:      phase,
			tokensUsed: req.GetInt("tokens_used", 0),
			durationMs: req.GetInt("duration_ms", 0),
			model:      req.GetString("model", ""),
			setupOnly:  req.GetBool("setup_only", false),
		}

		out, coreErr := reportResultCore(sm2, kb, in)
		if coreErr != nil {
			return errorf("%v", coreErr)
		}

		// Emit phase-complete event after successful phase completion.
		if out.NextActionHint == "proceed" && !in.setupOnly {
			if st, stErr := loadState(workspace); stErr == nil {
				publishEvent(bus, nil, "phase-complete", phase, st.SpecName, workspace, "completed")
			}
		}

		return okJSON(out)
	}
}

// reportResultCore performs the core report-result logic.
// Returns a typed outcome for callers that need to inspect NextActionHint
// without deserializing a JSON wire response.
func reportResultCore(sm *state.StateManager, kb *history.KnowledgeBase, in reportResultInput) (reportResultOutcome, error) {
	var warnings []string

	// Step 2: Load state for duplicate-log check (before PhaseLog).
	s, err := loadState(in.workspace)
	if err != nil {
		return reportResultOutcome{}, fmt.Errorf("read state: %w", err)
	}
	if w := Warn3dPhaseLogDuplicate(in.phase, s); w != "" {
		warnings = append(warnings, w)
	}

	// Step 3: Record phase-log entry.
	if err := sm.PhaseLog(in.workspace, in.phase, in.tokensUsed, in.durationMs, in.model); err != nil {
		return reportResultOutcome{}, fmt.Errorf("phase_log: %w", err)
	}

	// Step 4: Validate artifacts for this phase.
	results := validation.ValidateArtifacts(in.workspace, in.phase)

	// Step 5: Process validation results.
	var artifactWritten string
	for i, result := range results {
		if strings.HasPrefix(result.Error, "unknown phase:") {
			warnings = append(warnings, "artifact validation skipped: "+result.Error)
			continue
		}
		// For phase-6, ArtifactResult.Valid=false may indicate a FAIL verdict (not a missing file).
		// Block only when there is an error string (file missing, no verdict token found).
		// ParseVerdict is the authoritative mechanism for PASS/FAIL decisions in phase-6.
		if !result.Valid && result.Error != "" {
			return reportResultOutcome{}, fmt.Errorf("artifact invalid for %s: %s", in.phase, result.Error)
		}
		// Step 6: Set artifactWritten from the first result with a File field.
		if i == 0 && result.File != "" {
			artifactWritten = result.File
		}
	}

	// Steps 7–9: Determine state transition based on phase.
	out, err := determineTransition(sm, kb, in, results, artifactWritten, &warnings)
	if err != nil {
		return reportResultOutcome{}, err
	}

	// Merge any warning from the transition handler (e.g. completion gate)
	// into the accumulated warnings before building the final response.
	if out.Warning != "" {
		warnings = append(warnings, out.Warning)
	}
	out.Warning = strings.Join(warnings, "; ")

	// Attach a display message when the phase completed successfully.
	if out.NextActionHint == "proceed" && !in.setupOnly {
		out.DisplayMessage = buildCompleteMessage(in.tokensUsed, in.durationMs)
	}

	return out, nil
}

// handleReportResult serializes reportResultCore output to the MCP wire format.
// Retained for backward compatibility with existing tests.
func handleReportResult(sm *state.StateManager, kb *history.KnowledgeBase, in reportResultInput) (*mcp.CallToolResult, error) {
	out, err := reportResultCore(sm, kb, in)
	if err != nil {
		return errorf("%v", err)
	}
	return okJSON(out)
}
