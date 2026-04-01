---
name: forge
description: Orchestrate a full development pipeline using MCP-driven subagents. Accepts plain text, GitHub Issue URLs, or Jira Issue URLs as input.
---

# claude-forge Orchestrator

## Step 1: Initialize or Resume

1. Call `mcp__forge-state__pipeline_init(arguments=$ARGUMENTS)`.
2. If `result.errors` is non-empty: surface the errors to the user and stop.
3. If `result.resume == true`: confirm resume from `result.workspace` with the user, then go to Step 2.
4. For all new pipelines (resume is false or absent):
   a. If `result.fetch_needed` is non-null: fetch the external data described by `result.fetch_needed`
      (GitHub issue fields or Jira issue fields), then call
      `mcp__forge-state__pipeline_init_with_context(workspace=result.workspace, flags=result.flags, external_context=<fetched data>)`.
   b. If `result.fetch_needed` is null (plain text input): call
      `mcp__forge-state__pipeline_init_with_context(workspace=result.workspace, flags=result.flags)`.
   In both cases, the response will contain `needs_user_confirmation`. Present the detected
   `detected_task_type`, `detected_effort`, and `flow_template` to the user and wait for confirmation.
   While waiting, generate a concise English slug (3–6 words, lowercase, hyphen-separated,
   ASCII only) that summarises the task — e.g. `"add-user-auth-endpoint"` or
   `"fix-report-export-timeout"`. If the input is in a non-English language, translate
   the core intent into English for the slug.
   Then call `mcp__forge-state__pipeline_init_with_context` again with the same parameters plus
   `user_confirmation={task_type: <confirmed>, effort: <confirmed>, workspace_slug: <slug>}`.
   Use the `workspace` from the confirmed response for all subsequent calls.
   **Important**: Always pass the workspace path from `pipeline_init` unchanged to the first
   `pipeline_init_with_context` call. Never construct workspace paths manually.

## Step 2: Main Loop

Repeat until done:

1. Call `mcp__forge-state__pipeline_next_action(workspace=<workspace>)`.
2. Execute the action based on `action.type`:
   - `spawn_agent`: Call Agent tool with `action.prompt`. Use `action.agent` as description.
     - If `action.parallel_task_ids` is non-empty: spawn one Agent call per task ID in
       parallel; wait for all to complete before calling report_result.
   - `checkpoint`: Call `mcp__forge-state__checkpoint(workspace, phase=action.name)`.
     Present `action.present_to_user` to the user. Wait for response.
     - **Special: `post-to-jira` checkpoint** — when `action.name` is `"post-to-jira"`:
       1. Ask the user whether to post the work report to Jira (use AskUserQuestion
          with options "post" / "skip").
       2. If the user chooses **"post"**:
          a. Read `{workspace}/request.md` front matter to extract `source_url`
             (the Jira issue URL, e.g. `https://company.atlassian.net/browse/PROJ-123`).
          b. Extract the Jira issue key from the URL (e.g. `PROJ-123`).
          c. Read `{workspace}/final-summary.md` for the comment body.
          d. Post the comment using one of these methods (try in order):
             - Atlassian MCP tools (if available in the current environment)
             - `curl` with Jira REST API v3:
               ```
               curl -s -X POST \
                 -H "Content-Type: application/json" \
                 -u "$JIRA_USER:$JIRA_TOKEN" \
                 "https://<domain>/rest/api/3/issue/<issue-key>/comment" \
                 -d '{"body":{"version":1,"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<summary content>"}]}]}}'
               ```
          e. Report success or failure to the user.
       3. If the user chooses **"skip"**: do nothing.
     Call `mcp__forge-state__phase_complete(workspace, phase=action.name)`.
     Do NOT call pipeline_report_result for checkpoints.
   - `exec`: Execute `action.commands` based on the first element:
     - If `action.commands[0]` is `task_init`: call `mcp__forge-state__task_init`
       (parse tasks from `tasks.md` in the workspace and pass as the `tasks` parameter).
     - If `action.commands[0]` is `create_branch`: run `git checkout -b <action.commands[1]>`
       via Bash, then call `mcp__forge-state__set_branch(workspace, branch=action.commands[1])`.
     - Otherwise: run `action.commands` via Bash.
     Then call `pipeline_report_result` with `phase=action.phase`.
     If `action.setup_only` is true, pass `setup_only=true` to `pipeline_report_result`.
     (action.phase is always populated for exec actions.)
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
   tokens_used=<tokens>, duration_ms=<ms>, model=<model>,
   setup_only=action.setup_only)`.  (Omit `setup_only` when false/absent.)
   Check `result.next_action_hint`:
   - `"revision_required"`: present findings to user.
   - `"setup_continue"`: immediately call `pipeline_next_action` again
     (the engine will return the next setup step or the real dispatch).

## Rules

- Never make orchestration decisions independently — follow action.type exactly.
- Never skip pipeline_report_result for spawn_agent, exec, or write_file actions.
- Never pass `isolation: "worktree"` to any Agent call.
- On MCP error: surface the error to the user and stop.
