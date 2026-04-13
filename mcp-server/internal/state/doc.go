// Package state defines the pipeline state model and all centralized
// constants for the forge-state MCP server.
//
// Key types:
//   - [State]: the JSON-serializable pipeline state (state.json). Tracks
//     current phase, status, effort, branch, tasks, skipped phases,
//     revision counts, and phase-log entries.
//   - [StateManager]: thread-safe read/write access to state.json on disk.
//     All 26 state-management MCP commands (init, get, phase-start,
//     phase-complete, set-effort, task-update, etc.) delegate to this type.
//
// All phase identifiers, status values, task fields, artifact filenames,
// and other shared constants are defined here to prevent typo-induced bugs
// and make rename operations safe (change once, compile-check everywhere).
//
// Import direction: state is the leaf of the internal dependency graph.
// Every other internal package may import state; state imports nothing
// from internal/.
package state
