// Package analytics aggregates pipeline history into cost/time predictions
// and per-pipeline summaries.
package analytics

import (
	"path/filepath"

	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// costPerToken is the approximate USD cost per output token (rough Sonnet rate).
const costPerToken = 0.000006

// Collector aggregates PhaseLog entries from a single pipeline workspace into
// a PipelineSummary. It has no exported fields; specsDir is accepted for API
// symmetry with Estimator and Reporter but is not stored.
type Collector struct{}

// NewCollector returns a new Collector. The specsDir argument is accepted for
// API symmetry with Estimator and Reporter but is not stored on the struct.
func NewCollector(_ string) *Collector {
	return &Collector{}
}

// PipelineSummary contains aggregated metrics for a single pipeline run.
type PipelineSummary struct {
	Pipeline         string        `json:"pipeline"`
	TaskType         string        `json:"task_type"`
	Effort           string        `json:"effort"`
	FlowTemplate     string        `json:"flow_template"`
	TotalTokens      int           `json:"total_tokens"`
	TotalDurationMs  int           `json:"total_duration_ms"`
	EstimatedCostUSD float64       `json:"estimated_cost_usd"`
	PhasesExecuted   int           `json:"phases_executed"`
	PhasesSkipped    int           `json:"phases_skipped"`
	Retries          int           `json:"retries"`
	ReviewFindings   FindingCounts `json:"review_findings"`
}

// FindingCounts holds counts of critical and minor review findings.
type FindingCounts struct {
	Critical int `json:"critical"`
	Minor    int `json:"minor"`
}

// Collect reads state.json from workspace, aggregates PhaseLog entries,
// and returns a PipelineSummary. Absent review files (os.IsNotExist) produce
// zero findings and are not treated as errors.
func (c *Collector) Collect(workspace string) (*PipelineSummary, error) {
	s, err := state.ReadState(workspace)
	if err != nil {
		return nil, err
	}

	var totalTokens, totalDurationMs int

	for _, entry := range s.PhaseLog {
		totalTokens += entry.Tokens
		totalDurationMs += entry.DurationMs
	}

	phasesExecuted := len(s.CompletedPhases)
	phasesSkipped := len(s.SkippedPhases)

	var retries int

	for _, task := range s.Tasks {
		retries += task.ImplRetries + task.ReviewRetries
	}

	findings, err := c.parseReviewFindings(workspace)
	if err != nil {
		return nil, err
	}

	summary := &PipelineSummary{
		Pipeline:         s.SpecName,
		TaskType:         derefString(s.TaskType),
		Effort:           derefString(s.Effort),
		FlowTemplate:     derefString(s.FlowTemplate),
		TotalTokens:      totalTokens,
		TotalDurationMs:  totalDurationMs,
		EstimatedCostUSD: float64(totalTokens) * costPerToken,
		PhasesExecuted:   phasesExecuted,
		PhasesSkipped:    phasesSkipped,
		Retries:          retries,
		ReviewFindings:   findings,
	}

	return summary, nil
}

// parseReviewFindings reads all review-*.md files from workspace (including
// review-design.md, review-tasks.md, and per-task review-N.md files) and
// counts CRITICAL and MINOR findings. Files that do not exist are silently
// treated as zero findings.
func (*Collector) parseReviewFindings(workspace string) (FindingCounts, error) {
	var counts FindingCounts

	reviewFiles, err := filepath.Glob(filepath.Join(workspace, "review-*.md"))
	if err != nil {
		return FindingCounts{}, err
	}

	for _, filePath := range reviewFiles {
		_, findings, err := orchestrator.ParseVerdict(filePath)
		if err != nil {
			return FindingCounts{}, err
		}

		for _, f := range findings {
			switch f.Severity {
			case orchestrator.SeverityCritical:
				counts.Critical++
			case orchestrator.SeverityMinor:
				counts.Minor++
			}
		}
	}

	return counts, nil
}

// derefString dereferences a *string, returning an empty string for nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
