// Package tools implements all 44 MCP tool handlers registered by the
// forge-state server.
//
// The handlers are organised into categories:
//
// Pipeline lifecycle (4 tools):
//   - [PipelineInitHandler]: input parsing and resume detection.
//   - [PipelineInitWithContextHandler]: three-call confirmation flow
//     (effort detection → optional discussion → workspace initialisation).
//   - [PipelineNextActionHandler]: reads state, calls [orchestrator.Engine],
//     enriches spawn_agent prompts with 4-layer assembly, and returns
//     typed actions.
//   - [PipelineReportResultHandler]: records phase metrics, validates
//     artifacts, parses review verdicts, and advances pipeline state.
//
// State management (26 tools):
//   - init, get, phase-start, phase-complete, phase-fail, checkpoint,
//     abandon, skip-phase, task-init, task-update, revision-bump,
//     inline-revision-bump, set-revision-pending, clear-revision-pending,
//     set-branch, set-effort, set-flow-template, set-auto-approve,
//     set-skip-pr, set-debug, set-use-current-branch, phase-log,
//     phase-stats, resume-info, refresh-index.
//
// Code analysis (4 tools): ast_summary, ast_find_definition,
// dependency_graph, impact_scope.
//
// Validation (2 tools): validate_input, validate_artifact.
//
// History & analytics (8 tools): search_patterns, subscribe_events,
// history_search, history_get_patterns, history_get_friction_map,
// analytics_pipeline_summary, analytics_repo_dashboard, analytics_estimate.
//
// All handlers are registered in [RegisterAll] (registry.go).
//
// Import direction: tools → orchestrator → state (one-way DAG).
// tools also imports history, profile, prompt, validation, and events.
package tools
