// Package analytics provides pipeline analytics: Collector, Estimator, and Reporter.
package analytics

import (
	"os"
	"path/filepath"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// Reporter builds a RepoDashboard by scanning all pipeline specs in specsDir.
type Reporter struct {
	specsDir string
	kb       *history.KnowledgeBase
}

// NewReporter creates a Reporter for the given specsDir.
// kb may be nil; in that case MostCommonFindings will be an empty slice.
func NewReporter(specsDir string, kb *history.KnowledgeBase) *Reporter {
	return &Reporter{
		specsDir: specsDir,
		kb:       kb,
	}
}

// RepoDashboard holds aggregate statistics across all pipeline runs.
type RepoDashboard struct {
	TotalPipelines        int                      `json:"total_pipelines"`
	Completed             int                      `json:"completed"`
	Abandoned             int                      `json:"abandoned"`
	ByTaskType            map[string]TaskTypeStats `json:"by_task_type"`
	ByFlowTemplate        map[string]FlowStats     `json:"by_flow_template"`
	TotalTokens           int                      `json:"total_tokens"`
	EstimatedTotalCostUSD float64                  `json:"estimated_total_cost_usd"`
	ReviewPassRate        float64                  `json:"review_pass_rate"`
	AvgRetriesPerPipeline float64                  `json:"avg_retries_per_pipeline"`
	MostCommonFindings    []PatternEntry           `json:"most_common_findings"`
}

// TaskTypeStats holds per-task-type aggregate metrics.
type TaskTypeStats struct {
	Count     int     `json:"count"`
	AvgTokens float64 `json:"avg_tokens"`
	AvgDurMin float64 `json:"avg_duration_min"`
}

// FlowStats holds per-flow-template aggregate metrics.
type FlowStats struct {
	Count int `json:"count"`
}

// PatternEntry is a common review finding pattern from the knowledge base.
type PatternEntry struct {
	Description string `json:"description"`
	Count       int    `json:"count"`
	Severity    string `json:"severity"`
}

// taskTypeAccumulator accumulates raw sums for computing averages.
type taskTypeAccumulator struct {
	count       int
	totalTokens int
	totalDurMs  int
}

// Dashboard scans specsDir and builds aggregate pipeline statistics.
//
//nolint:cyclop,gocyclo // complexity is inherent in multi-dimensional aggregation
func (r *Reporter) Dashboard() (*RepoDashboard, error) {
	entries, err := os.ReadDir(r.specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return r.emptyDashboard(), nil
		}
		return nil, err
	}

	dash := &RepoDashboard{
		ByTaskType:     make(map[string]TaskTypeStats),
		ByFlowTemplate: make(map[string]FlowStats),
	}

	// accumulators for computing averages after the scan
	taskTypeAcc := make(map[string]*taskTypeAccumulator)

	var (
		totalRetries    int
		completedCount  int
		reviewPassCount int
	)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		s, err := state.ReadState(filepath.Join(r.specsDir, entry.Name()))
		if err != nil {
			// Skip pipelines with unreadable state.
			continue
		}

		dash.TotalPipelines++

		isCompleted := s.CurrentPhase == "completed"
		isAbandoned := s.CurrentPhase == "abandoned"

		switch {
		case isCompleted:
			dash.Completed++
			completedCount++
		case isAbandoned:
			dash.Abandoned++
		}

		// Sum tokens from PhaseLog for all pipelines (not just completed).
		for _, pl := range s.PhaseLog {
			dash.TotalTokens += pl.Tokens
		}

		if !isCompleted {
			continue
		}

		// All metrics below are only for completed pipelines.

		// Task type stats accumulation.
		taskType := ""
		if s.TaskType != nil {
			taskType = *s.TaskType
		}

		flowTemplate := ""
		if s.FlowTemplate != nil {
			flowTemplate = *s.FlowTemplate
		}

		if taskType != "" {
			acc, exists := taskTypeAcc[taskType]
			if !exists {
				acc = &taskTypeAccumulator{}
				taskTypeAcc[taskType] = acc
			}

			acc.count++

			for _, pl := range s.PhaseLog {
				acc.totalTokens += pl.Tokens
				acc.totalDurMs += pl.DurationMs
			}
		}

		// Flow template stats.
		if flowTemplate != "" {
			fs := dash.ByFlowTemplate[flowTemplate]
			fs.Count++
			dash.ByFlowTemplate[flowTemplate] = fs
		}

		// Review pass rate: pipeline passes if both DesignRevisions and TaskRevisions are 0.
		if s.Revisions.DesignRevisions == 0 && s.Revisions.TaskRevisions == 0 {
			reviewPassCount++
		}

		// Retries per pipeline.
		for _, task := range s.Tasks {
			totalRetries += task.ImplRetries + task.ReviewRetries
		}
	}

	// Compute EstimatedTotalCostUSD from total tokens.
	dash.EstimatedTotalCostUSD = float64(dash.TotalTokens) * costPerToken

	// Compute ReviewPassRate.
	if completedCount > 0 {
		dash.ReviewPassRate = float64(reviewPassCount) / float64(completedCount)
	}

	// Compute AvgRetriesPerPipeline.
	if completedCount > 0 {
		dash.AvgRetriesPerPipeline = float64(totalRetries) / float64(completedCount)
	}

	// Populate ByTaskType from accumulators.
	for tt, acc := range taskTypeAcc {
		var avgTokens, avgDurMin float64
		if acc.count > 0 {
			avgTokens = float64(acc.totalTokens) / float64(acc.count)
			avgDurMin = float64(acc.totalDurMs) / float64(acc.count) / 60000.0
		}

		dash.ByTaskType[tt] = TaskTypeStats{
			Count:     acc.count,
			AvgTokens: avgTokens,
			AvgDurMin: avgDurMin,
		}
	}

	// Populate MostCommonFindings from knowledge base.
	dash.MostCommonFindings = r.mostCommonFindings()

	return dash, nil
}

// emptyDashboard returns a zero-value RepoDashboard with non-nil slices/maps.
func (*Reporter) emptyDashboard() *RepoDashboard {
	return &RepoDashboard{
		ByTaskType:         make(map[string]TaskTypeStats),
		ByFlowTemplate:     make(map[string]FlowStats),
		MostCommonFindings: []PatternEntry{},
	}
}

// mostCommonFindings returns the top 10 patterns from the knowledge base,
// or an empty slice if kb is nil.
func (r *Reporter) mostCommonFindings() []PatternEntry {
	if r.kb == nil {
		return []PatternEntry{}
	}

	patterns := r.kb.Patterns.Query("", "", 10)
	if len(patterns) == 0 {
		return []PatternEntry{}
	}

	result := make([]PatternEntry, 0, len(patterns))
	for _, p := range patterns {
		result = append(result, PatternEntry{
			Description: p.Pattern,
			Count:       p.Frequency,
			Severity:    p.Severity,
		})
	}

	return result
}
