---
name: forge
description: Orchestrate a full development pipeline using MCP-driven subagents. Accepts plain text or issue tracker URLs (GitHub, Jira, Linear, etc.) as input.
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
   a. If `result.fetch_needed` is non-null:
      1. Fetch the external data using the method specified in `fetch_needed`:
         - If `fetch_needed.mcp_tool` is set: call the MCP tool with `fetch_needed.mcp_params`.
         - Else if `fetch_needed.command` is set: execute the command via Bash and parse the JSON output.
         - Else: follow `fetch_needed.instruction` as a fallback guide.
      2. Map the response fields to `external_context` using `fetch_needed.response_mapping`:
         for each entry `(response_key → context_key)`, set `external_context[context_key] = response[response_key]`.
      3. Call `mcp__forge-state__pipeline_init_with_context(workspace=result.workspace, source_id=result.source_id, source_url=result.source_url, flags=result.flags, external_context=<mapped data>)`.
         (`task_text` is not applicable for external issue sources — do not pass it.)
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
   1. **Effort level**: present all three effort options from `effort_options`
      (S, M, L — each with `skipped_phases` using the `label` field).
      Each option has a `recommended` boolean — mark the one where
      `recommended` is `true` as "(Recommended)".
   2. **Branch decision**: based on `current_branch` and `is_main_branch` from the response:
      - If `is_main_branch` is true: inform the user a new branch will be created (no question needed).
      - If `is_main_branch` is false: ask whether to use the current branch or create a new one.
   While waiting, generate a concise English slug (3–6 words, lowercase, hyphen-separated,
   ASCII only) that summarises the task — e.g. `"add-user-auth-endpoint"` or
   `"fix-report-export-timeout"`. If the input is in a non-English language, translate
   the core intent into English for the slug.
   **Do not include the issue number** (GitHub `#N` or Jira `PROJ-123`) in the slug —
   the server prepends `source_id` automatically when present.
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

1. Call `mcp__forge-state__pipeline_next_action(workspace=<workspace>,
   previous_action_complete=true,
   previous_tokens=<tokens from last action>,
   previous_duration_ms=<ms from last action>,
   previous_model=<model from last action>,
   previous_setup_only=<setup_only from last action>)`.
   On the very first call, omit the `previous_*` parameters.

2. If `result.report_result` is non-null:
   - If `result.report_result.next_action_hint == "revision_required"`:
     present `result.report_result.findings` to the user, then continue the loop.
     On the next `pipeline_next_action` call, omit `previous_action_complete` (or pass false),
     and pass `previous_tokens=0, previous_duration_ms=0` with no `previous_model` or
     `previous_setup_only` — no new agent ran during this step, so P5 must not fire again.
   (`setup_continue` is handled server-side — the handler re-enters NextAction automatically.)

3. Execute the action based on `action.type`:
   - `spawn_agent`: If `action.display_message` is non-empty, output it verbatim.
     Then call Agent tool with `action.prompt`. Use `action.agent` as description.
     Record the tokens, duration, and model for the next `pipeline_next_action` call.
     - If `action.parallel_task_ids` is non-empty: spawn one Agent call per task ID in
       parallel; wait for all to complete before calling `pipeline_next_action` again.
     - **Artifact write fallback**: After the agent returns, if `action.output_file` is
       non-empty, check whether `{workspace}/{action.output_file}` exists on disk. If
       the file does **NOT** exist, the agent returned its output as text instead of
       writing the file (subagents may lack write permission in some permission modes).
       In that case, use the Write tool to write the agent's final response text to
       `{workspace}/{action.output_file}` before calling `pipeline_next_action`.
       This ensures `pipeline_report_result` artifact validation always succeeds.
   - `checkpoint`: **Before presenting anything to the user**, call
     `mcp__forge-state__checkpoint(workspace, phase=action.name)`.
     This is mandatory — it registers the pause so the pipeline can exit safely if
     the user closes the conversation before responding. Never skip or defer this call.
     Then present `action.present_to_user` to the user. Mention that the Dashboard
     can be used to approve without terminal input.
     **Immediately** call `mcp__forge-state__pipeline_next_action(workspace)` (no
     `user_response`, no `previous_*`). The server blocks up to 15 s waiting for a
     Dashboard approval event.
     - If the response has `still_waiting: true`: call `pipeline_next_action(workspace)`
       again immediately (no `user_response`). Repeat until a non-checkpoint action is
       returned or the user provides a terminal response.
     - If the user types a response in the terminal (proceed / revise / abandon) while
       still_waiting is looping: on the next `pipeline_next_action` call, pass
       `user_response=<response>` instead of looping.
     - If a non-checkpoint action is returned: Dashboard approved; proceed normally.
     - **Special: `post-to-source` checkpoint** — when `action.name`
       is `"post-to-source"`:
       1. Ask the user whether to post the work report (use AskUserQuestion
          with options "post" / "skip").
       2. If the user chooses **"post"** and `action.post_method` is present:
          a. Read the body content from `action.post_method.body_source`.
          b. Post the comment using the method specified in `post_method`:
             - If `post_method.mcp_tool` is set: call it with `post_method.mcp_params`
               and the body content (pass body as the `body` parameter).
             - Else if `post_method.command` is set: execute the command via Bash.
             - Else if `post_method.instruction` is set: follow the instruction as a fallback guide.
          c. Report success or failure to the user.
       3. If the user chooses **"skip"**: do nothing.
       Pass the user's response to `pipeline_next_action(workspace, user_response=<response>)`.
     The engine handles all checkpoint state transitions deterministically
     (proceed → advance, revise → rewind, abandon → mark abandoned).
     Do NOT call `phase_complete` for checkpoints — the engine owns the lifecycle.
     On every `pipeline_next_action` call for a checkpoint (still_waiting loops and
     terminal-response call alike), omit `previous_action_complete` (or pass false),
     and pass `previous_tokens=0, previous_duration_ms=0` with no `previous_model`
     or `previous_setup_only`
     (checkpoints have no agent cost; omitting `previous_action_complete` causes the P5 block to be skipped).
   - `exec`: Run `action.commands` via Bash. Record the duration and `action.setup_only`
     for the next `pipeline_next_action` call. Pass `previous_setup_only=true` if
     `action.setup_only` is true. There is no model to record for exec actions; omit
     `previous_model` or pass it as an empty string.
     Also pass `previous_action_complete=true` (see Rules below).
   - `write_file`: Write `action.content` to `action.path`. Record the duration for the
     next `pipeline_next_action` call. Omit `previous_model` or pass it as an empty string.
     Also pass `previous_action_complete=true` (see Rules below).
   - `human_gate`: A task requires human action (e.g. merge an external PR, update dependencies).
     Present `action.present_to_user` to the user using AskUserQuestion with `action.options`.
     - If the user chooses **"done"** or **"skip"**: call `pipeline_next_action` again
       with no `previous_*` parameters (no agent ran).
       The handler automatically marks the task as completed.
     - If the user chooses **"abandon"**: call `mcp__forge-state__abandon(workspace)`.
     Do NOT call `checkpoint` or `phase_complete` for human_gate actions.
   - `done`: Pipeline complete. Stop.

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

Flags can also be set as persistent defaults via `/forge-setup`. Explicit flags on `/forge` always override preferences.

## Rules

- Never make orchestration decisions independently — follow action.type exactly.
- Always pass `previous_action_complete=true`, `previous_tokens`, `previous_duration_ms`, `previous_model`, and `previous_setup_only` on every `pipeline_next_action` call after the first (after any `spawn_agent`, `exec`, or `write_file` action completes). Do NOT pass `previous_action_complete=true` after a checkpoint — it must remain false (or omitted) to skip the P5 report block.
- When `still_waiting: true` is returned: call `pipeline_next_action(workspace)` again immediately with no `previous_*` or `user_response`. This is the Dashboard long-poll loop — keep calling until a non-still_waiting response arrives or the user types a terminal response.
- Never pass `isolation: "worktree"` to any Agent call.
- On MCP error: surface the error to the user and stop.
