package analytics

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// Estimator loads completed pipeline state.json files from specsDir and
// computes P50/P90 percentile predictions for a given (taskType, effort) pair.
type Estimator struct {
	specsDir string
}

// NewEstimator constructs an Estimator that reads historical pipeline data
// from specsDir.
func NewEstimator(specsDir string) *Estimator {
	return &Estimator{specsDir: specsDir}
}

// EstimateResult holds the P50/P90 predictions for a given (taskType, effort) pair.
type EstimateResult struct {
	SampleSize  int         `json:"sample_size"`
	Tokens      Percentiles `json:"tokens"`
	DurationMin Percentiles `json:"duration_min"`
	CostUSD     Percentiles `json:"cost_usd"`
	Confidence  string      `json:"confidence"`
	Note        string      `json:"note"`
}

// Percentiles holds P50 and P90 percentile values.
type Percentiles struct {
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
}

type sample struct {
	tokens     int
	durationMs int
}

// Estimate scans specsDir for completed pipelines matching effort
// and returns P50/P90 predictions for tokens, duration, and cost.
func (e *Estimator) Estimate(effort string) (*EstimateResult, error) {
	entries, err := os.ReadDir(e.specsDir)
	if err != nil {
		return nil, fmt.Errorf("estimator ReadDir %s: %w", e.specsDir, err)
	}

	var samples []sample

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		workspace := filepath.Join(e.specsDir, entry.Name())

		s, err := state.ReadState(workspace)
		if err != nil {
			// Skip directories without a valid state.json
			continue
		}

		if s.CurrentPhase != "completed" {
			continue
		}

		if s.Effort == nil || *s.Effort != effort {
			continue
		}

		totalTokens := 0
		totalDurationMs := 0

		for _, entry := range s.PhaseLog {
			totalTokens += entry.Tokens
			totalDurationMs += entry.DurationMs
		}

		samples = append(samples, sample{
			tokens:     totalTokens,
			durationMs: totalDurationMs,
		})
	}

	n := len(samples)
	confidence := confidenceLevel(n)

	if n == 0 {
		return &EstimateResult{
			SampleSize:  0,
			Tokens:      Percentiles{},
			DurationMin: Percentiles{},
			CostUSD:     Percentiles{},
			Confidence:  confidence,
			Note:        fmt.Sprintf("No completed pipelines found for effort=%q. Predictions will improve as pipelines are completed.", effort),
		}, nil
	}

	tokenVals := make([]float64, n)
	durationVals := make([]float64, n)
	costVals := make([]float64, n)

	for i, s := range samples {
		tokenVals[i] = float64(s.tokens)
		durationVals[i] = float64(s.durationMs) / 60000.0
		costVals[i] = float64(s.tokens) * costPerToken
	}

	slices.Sort(tokenVals)
	slices.Sort(durationVals)
	slices.Sort(costVals)

	return &EstimateResult{
		SampleSize: n,
		Tokens: Percentiles{
			P50: percentile(tokenVals, 50),
			P90: percentile(tokenVals, 90),
		},
		DurationMin: Percentiles{
			P50: percentile(durationVals, 50),
			P90: percentile(durationVals, 90),
		},
		CostUSD: Percentiles{
			P50: percentile(costVals, 50),
			P90: percentile(costVals, 90),
		},
		Confidence: confidence,
		Note:       fmt.Sprintf("Based on %d completed pipeline(s) for effort=%q.", n, effort),
	}, nil
}

// percentile returns the value at the given percentile (0–100) from a sorted slice.
// For n==1: returns the single element for any percentile.
func percentile(sorted []float64, p int) float64 {
	n := len(sorted)

	if n == 0 {
		return 0
	}

	if n == 1 {
		return sorted[0]
	}

	if p == 50 {
		idx := (n - 1) / 2
		return sorted[idx]
	}

	// P90: ceil(n * 0.9) - 1, clamped to [0, n-1]
	idx := max(int(math.Ceil(float64(n)*float64(p)/100.0))-1, 0)
	if idx >= n {
		idx = n - 1
	}

	return sorted[idx]
}

// confidenceLevel returns the confidence label based on sample size.
func confidenceLevel(n int) string {
	switch {
	case n >= 10:
		return "high"
	case n >= 3:
		return "medium"
	default:
		return "low"
	}
}
