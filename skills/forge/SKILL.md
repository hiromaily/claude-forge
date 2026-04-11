---
name: forge
description: Orchestrate a full development pipeline using MCP-driven subagents. Accepts plain text, GitHub Issue URLs, or Jira Issue URLs as input.
---

# claude-forge Orchestrator

## Step 1: Initialize or Resume

**To resume a suspended pipeline**, supply the spec directory name from `.specs/`:

Example: `/forge 20260401-effort-only-flow`

1. Call `mcp__forge-state__pipeline_init(arguments=$ARGUMENTS)`.
2. If `result.errors` is non-empty: surface the errors to the user and stop.
3. If `result.resume_mode` is `"auto"`: the input matched an existing spec directory.
   **Do not ask for confirmation** — proceed directly to Step 2.
4. For new pipelines (`resume_mode` is absent):
   a. If `result.fetch_needed` is non-null: fetch the external data described by `result.fetch_needed`
      (GitHub issue fields or Jira issue fields), then call
      `mcp__forge-state__pipeline_init_with_context(workspace=result.workspace, source_id=result.source_id, source_url=result.source_url, flags=result.flags, external_context=<fetched data>)`.
      (`task_text` is not applicable for GitHub/Jira sources — do not pass it.)
   b. If `result.fetch_needed` is null (plain text input): call
      `mcp__forge-state__pipeline_init_with_context(workspace=result.workspace, flags=result.flags, task_text=result.core_text)`.
      (`result.core_text` is a top-level field in the `pipeline_init` response — the task text with bare flags stripped.)

   **After the first `pipeline_init_with_context` call**, check which field is present in the response:

   **If `result.needs_discussion` is non-null** (triggered only when `--discuss` flag is present,
   source is plain text, and `--auto` is absent):
   1. Present each question in `result.needs_discussion.questions` to the user via AskUserQuestion
      (single prompt listing all questions).
   2. Collect the user's answers as a single freeform string
      (e.g. `"Q1: <answer>\nQ2: <answer>\nQ3: <answer>"`).
   3. Call `mcp__forge-state__pipeline_init_with_context` again (the discussion call) with:
      `workspace=<same>, flags=<same>, task_text=<same>, discussion_answers=<collected answers string>`.
      The response will contain `needs_user_confirmation` where
      `needs_user_confirmation.enriched_request_body` is non-empty.
   4. Proceed with the effort/branch confirmation step below using this `needs_user_confirmation`.
      When calling `pipeline_init_with_context` with `user_confirmation`, pass back:
      `user_confirmation={effort: <confirmed>, workspace_slug: <slug>, use_current_branch: <bool>, enriched_request_body: needs_user_confirmation.enriched_request_body}`.
      This carries the enriched `request.md` body forward to workspace initialisation.

   **If `result.needs_user_confirmation` is present directly** (`--discuss` absent, `--auto` set,
   or GitHub/Jira source): proceed with the effort/branch confirmation step below
   (no `enriched_request_body` to pass back).

   **Effort/branch confirmation step** (applies after either path above):
   Present **all** of the following to the user in a **single prompt** (use AskUserQuestion
   with multiple questions):
   1. **Effort level**: the detected `detected_effort` and all three effort options from
      `effort_options` (S, M, L — each with their skipped phases, using the `label` field).
   2. **Branch decision**: based on `current_branch` and `is_main_branch` from the response:
      - If `is_main_branch` is true: inform the user a new branch will be created (no question needed).
      - If `is_main_branch` is false: ask whether to use the current branch or create a new one.
   While waiting, generate a concise English slug (3–6 words, lowercase, hyphen-separated,
   ASCII only) that summarises the task — e.g. `"add-user-auth-endpoint"` or
   `"fix-report-export-timeout"`. If the input is in a non-English language, translate
   the core intent into English for the slug.
   Then call `mcp__forge-state__pipeline_init_with_context` again with the same parameters plus
   `user_confirmation={effort: <confirmed>, workspace_slug: <slug>, use_current_branch: <bool>}`.
   If `needs_user_confirmation.enriched_request_body` is non-empty (from the discussion path),
   also include `enriched_request_body: <value>` inside `user_confirmation`.
   The response will contain `branch` (the branch name) and `create_branch` (boolean).
   If `create_branch` is true: run `git checkout -b <branch>` via Bash immediately.
   Use the `workspace` from the confirmed response for all subsequent calls.
   **Important**: Always pass the workspace path from `pipeline_init` unchanged to the first
   `pipeline_init_with_context` call. Never construct workspace paths manually.

## Step 2: Main Loop

Repeat until done:

1. Call `mcp__forge-state__pipeline_next_action(workspace=<workspace>)`.
2. Execute the action based on `action.type`:
   - `spawn_agent`: If `action.display_message` is non-empty, output it verbatim.
     Then call Agent tool with `action.prompt`. Use `action.agent` as description.
     - If `action.parallel_task_ids` is non-empty: spawn one Agent call per task ID in
       parallel; wait for all to complete before calling report_result.
   - `checkpoint`: **Before presenting anything to the user**, call
     `mcp__forge-state__checkpoint(workspace, phase=action.name)`.
     This is mandatory — it registers the pause so the pipeline can exit safely if
     the user closes the conversation before responding. Never skip or defer this call.
     Then present `action.present_to_user` to the user. Wait for response.
     - **Special: `post-to-source` checkpoint** — when `action.name`
       is `"post-to-source"` (the message will indicate GitHub or Jira):
       1. Ask the user whether to post the work report (use AskUserQuestion
          with options "post" / "skip").
       2. If the user chooses **"post"**:
          a. Extract the source URL from `action.present_to_user` (the line starting with `URL:`).
          b. Determine the source type from the URL (GitHub if `github.com`, Jira if `atlassian.net`):
             - **GitHub**: run
               `gh issue comment <url> --body-file {workspace}/summary.md`
             - **Jira**: Extract the domain and issue key from the URL
               (e.g. `example.atlassian.net` and `PROJ-123` from `https://example.atlassian.net/browse/PROJ-123`). Try in order:
               1. Atlassian MCP tools (if available)
               2. Convert `{workspace}/summary.md` to Atlassian Document Format (ADF) and run:
                  `curl -s -X POST -H "Content-Type: application/json" -u "$JIRA_USER:$JIRA_TOKEN" "https://<domain>/rest/api/3/issue/<key>/comment" -d '<ADF JSON>'`
          c. Report success or failure to the user.
       3. If the user chooses **"skip"**: do nothing.
     Call `mcp__forge-state__phase_complete(workspace, phase=action.name)`.
     Do NOT call pipeline_report_result for checkpoints.
   - `exec`: Run `action.commands` via Bash.
     Then call `pipeline_report_result` with `phase=action.phase`.
     If `action.setup_only` is true, pass `setup_only=true` to `pipeline_report_result`.
     (action.phase is always populated for exec actions.)
   - `write_file`: Write `action.content` to `action.path`. Then call
     `pipeline_report_result` with `phase=action.phase`. (action.phase always populated.)
   - `human_gate`: A task requires human action (e.g. merge an external PR, update dependencies).
     Present `action.present_to_user` to the user using AskUserQuestion with `action.options`.
     - If the user chooses **"done"** or **"skip"**: call `pipeline_next_action` again.
       The handler automatically marks the task as completed.
     - If the user chooses **"abandon"**: call `mcp__forge-state__abandon(workspace)`.
     Do NOT call `checkpoint` or `phase_complete` for human_gate actions.
     Do NOT call `pipeline_report_result` for human_gate actions.
   - `done`: Pipeline complete. Stop.
3. For `spawn_agent`, `exec`, and `write_file`: call
   `mcp__forge-state__pipeline_report_result(workspace, phase=action.phase,
   tokens_used=<tokens>, duration_ms=<ms>, model=<model>,
   setup_only=action.setup_only)`.  (Omit `setup_only` when false/absent.)
   If `result.display_message` is non-empty, output it verbatim.
   Check `result.next_action_hint`:
   - `"revision_required"`: present findings to user.
   - `"setup_continue"`: immediately call `pipeline_next_action` again
     (the engine will return the next setup step or the real dispatch).

## Supported Flags

- `--auto`: Skips all human confirmation prompts; progresses the pipeline automatically.
  Takes precedence over `--discuss`: when both flags are present, discussion mode is suppressed
  and `needs_user_confirmation` is returned directly.
- `--discuss`: Triggers a pre-pipeline clarification dialogue for plain text input pipelines.
  The orchestrator presents targeted questions to the user, collects answers, and writes the
  enriched task description into `request.md` before invoking any agents. Has no effect for
  GitHub/Jira source pipelines (external context already provides structured content) or when
  `--auto` is also set.
- `--nopr`: Skips the PR creation phase at the end of the pipeline.
- `--debug`: Enables debug mode in the pipeline state.

## Rules

- Never make orchestration decisions independently — follow action.type exactly.
- Never skip pipeline_report_result for spawn_agent, exec, or write_file actions.
- Never pass `isolation: "worktree"` to any Agent call.
- On MCP error: surface the error to the user and stop.
