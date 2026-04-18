// response construction helpers and low-level utilities used by
// MCP tool handlers.
//
// Extracted from handlers.go to keep that file focused on the 26 *Handler
// functions only.

package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
)

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
	data, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"result":%q,"warning":%q}`, msg, warning)), nil
	}
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

// ---------- internal helpers ----------

// loadState reads state.json from workspace without locking (handler-level read
// for guard checks).  The StateManager methods do their own locking for mutations.
func loadState(workspace string) (*state.State, error) {
	return state.ReadState(workspace)
}

// validateTaskDependencies checks task dependency integrity at task_init time.
// Returns a list of human-readable warning strings. An empty list means no issues.
//
// Checks performed:
//  1. depends_on references exist as valid task keys
//  2. No circular dependencies
//  3. Parallel tasks do not write to the same files
//
//nolint:gocyclo // complexity is inherent in the dependency validation logic
func validateTaskDependencies(tasks map[string]state.Task) []string {
	var warnings []string

	// Build a set of valid task keys.
	validKeys := make(map[int]bool, len(tasks))
	for k := range tasks {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		validKeys[n] = true
	}

	// Check 1: depends_on references exist.
	for k, task := range tasks {
		for _, dep := range task.DependsOn {
			if !validKeys[dep] {
				warnings = append(warnings, fmt.Sprintf(
					"task %s depends on task %d which does not exist", k, dep))
			}
		}
	}

	// Check 2: Circular dependency detection via DFS.
	// Build adjacency list from depends_on.
	adj := make(map[int][]int)
	for k, task := range tasks {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		adj[n] = task.DependsOn
	}

	// DFS cycle detection: 0=unvisited, 1=in-stack, 2=done.
	color := make(map[int]int)
	var hasCycle bool
	var dfs func(int)
	dfs = func(node int) {
		if hasCycle {
			return
		}
		color[node] = 1
		for _, dep := range adj[node] {
			switch color[dep] {
			case 1:
				hasCycle = true
				return
			case 0:
				dfs(dep)
			}
		}
		color[node] = 2
	}
	for k := range adj {
		if color[k] == 0 {
			dfs(k)
		}
	}
	if hasCycle {
		warnings = append(warnings, "circular dependency detected among tasks")
	}

	// Check 3: Parallel tasks must not write to the same files.
	fileOwners := make(map[string]string) // file path → first parallel task key
	for k, task := range tasks {
		if task.ExecutionMode != "parallel" {
			continue
		}
		for _, f := range task.Files {
			if owner, exists := fileOwners[f]; exists {
				warnings = append(warnings, fmt.Sprintf(
					"parallel tasks %s and %s both write to %s", owner, k, f))
			} else {
				fileOwners[f] = k
			}
		}
	}

	return warnings
}

// publishEvent constructs an Event and publishes it to bus. If slack is non-nil,
// it also calls slack.Notify so callers can pass nil when Slack is not needed.
func publishEvent(bus *events.EventBus, slack *events.SlackNotifier, eventType, phase, specName, workspace, outcome string) {
	publishEventWithDetail(bus, slack, eventType, phase, specName, workspace, outcome, "")
}

// publishEventWithDetail is like publishEvent but includes an optional Detail field.
func publishEventWithDetail(bus *events.EventBus, slack *events.SlackNotifier, eventType, phase, specName, workspace, outcome, detail string) {
	e := events.Event{
		Event:     eventType,
		Phase:     phase,
		SpecName:  specName,
		Workspace: workspace,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Outcome:   outcome,
		Detail:    detail,
	}
	bus.Publish(e)
	if slack != nil {
		slack.Notify(e)
	}
}
