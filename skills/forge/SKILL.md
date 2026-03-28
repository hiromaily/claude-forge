---
name: forge
description: Orchestrate a full development pipeline using MCP-driven subagents. Accepts plain text, GitHub Issue URLs, or Jira Issue URLs as input.
---

# claude-forge Orchestrator

## Step 1: Initialize or Resume

1. Call `mcp__forge-state__pipeline_init(raw_arguments=$ARGUMENTS)`.
2. If `result.fetch_needed == true`: fetch external data from `result.fetch_url`, then
   call `mcp__forge-state__pipeline_init_with_context(...)`.
   Confirm workspace path and task summary with the user before continuing.
3. If `result.status == "resume"`: confirm resume from `result.workspace`.

## Step 2: Main Loop

Repeat until done:

1. Call `mcp__forge-state__pipeline_next_action(workspace=<workspace>)`.
2. Execute the action based on `action.type`:
   - `spawn_agent`: Call Agent tool with `action.prompt`. Use `action.agent` as description.
     - If `action.parallel_task_ids` is non-empty: spawn one Agent call per task ID in
       parallel; wait for all to complete before calling report_result.
   - `checkpoint`: Call `mcp__forge-state__checkpoint(workspace, phase=action.name)`.
     Present `action.present_to_user` to the user. Wait for response.
     Call `mcp__forge-state__phase_complete(workspace, phase=action.name)`.
     Do NOT call pipeline_report_result for checkpoints.
   - `exec`: Run `action.commands` via Bash. Then call `pipeline_report_result`
     with `phase=action.phase`. (action.phase is always populated for exec actions.)
   - `write_file`: Write `action.content` to `action.path`. Then call
     `pipeline_report_result` with `phase=action.phase`. (action.phase always populated.)
   - `done`:
     - If `action.summary` starts with `"skip:"`: parse the phase name from `action.summary`
       (format: `"skip:<phase-id>"`). Call
       `mcp__forge-state__phase_complete(workspace, phase=<parsed-phase-id>)` then loop.
       (Do NOT use currentPhase from state — state has not yet advanced at this point.)
     - Otherwise: pipeline complete. Stop.
3. For `spawn_agent`, `exec`, and `write_file`: call
   `mcp__forge-state__pipeline_report_result(workspace, phase=action.phase,
   tokens_used=<tokens>, duration_ms=<ms>, model=<model>)`.
   Check `result.next_action_hint`: if `"revision_required"`, present findings to user.

## Rules

- Never make orchestration decisions independently — follow action.type exactly.
- Never skip pipeline_report_result for spawn_agent, exec, or write_file actions.
- Never pass `isolation: "worktree"` to any Agent call.
- On MCP error: surface the error to the user and stop.
