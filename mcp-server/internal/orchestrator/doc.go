// Package orchestrator contains the deterministic pipeline engine that drives
// all phase transitions in claude-forge.
//
// The central type is [Engine], whose [Engine.NextAction] method reads the
// current [state.State] and returns a typed [Action] — spawn_agent,
// checkpoint, exec, write_file, human_gate, or done. The LLM orchestrator
// executes the returned action without making control-flow decisions.
//
// Key components:
//   - [Engine]: the state machine. All dispatch decisions are deterministic
//     functions of state.json. Handles effort-aware phase skipping, retry
//     limits, artifact validation, and review verdict routing.
//   - [Action]: the typed action struct returned by NextAction. Fields are
//     populated based on the action type (see actions.go for type constants).
//   - [Registry]: maps phase IDs to metadata (labels, skip rules per
//     template). Initialised at package init from the declarative
//     phaseRegistry table.
//   - [DetectEffort]: heuristic effort detection from task text, Jira story
//     points, and GitHub labels.
//   - [DeriveBranchName]: generates deterministic branch names from spec names.
//   - [ClassifyBranchType]: keyword-based branch type classification
//     (feature/fix/refactor/docs/chore) from design content.
//
// Import direction: orchestrator → state (one-way). Never import tools or
// history — that would create an import cycle. See go-package-layering.md.
package orchestrator
