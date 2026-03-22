---
name: forge
description: Orchestrate a full development pipeline (Analysis → Investigation → Design → AI Review → Tasks → AI Review → Implementation → Comprehensive Review → Verification → PR → Summary) using isolated subagents. Accepts plain text, GitHub Issue URLs, or Jira Issue URLs as input.
---

# Development Pipeline

Execute a complete development workflow by delegating each phase to an isolated subagent. Files serve as the communication medium between phases, keeping the main agent as a thin orchestrator and preventing context accumulation.

## When to Use

When implementing a feature, fix, or refactoring task that spans multiple files or subsystems. The pipeline prevents context pollution that would otherwise degrade reasoning quality over a long session.

## Arguments

`$ARGUMENTS` — one of the following:

| Input type | Example | Behavior |
|-----------|---------|----------|
| **GitHub Issue URL** | `https://github.com/org/repo/issues/123` | Fetch issue details as context. Post summary as issue comment when done. |
| **Jira Issue URL** | `https://org.atlassian.net/browse/PROJ-123` | Fetch issue details as context. Post summary as Jira comment when done. |
| **Plain text** | `Add retry logic to the API client` | Use as-is. No external posting. |
| **Workspace path** (resume) | `.specs/20260320-fix-auth-timeout` | Resume pipeline from `state.json`. |

**Source detection** — at Workspace Setup, parse `$ARGUMENTS` and store the detected source type and identifier in `request.md` as YAML front matter:

```yaml
---
source_type: github_issue | jira_issue | text
source_url: <URL if applicable>
source_id: <issue number or key if applicable>
---
```

This metadata is used at the end of the pipeline to determine whether and where to post the summary.

---

## Hard Constraints

> **NEVER pass `isolation: "worktree"` to any Agent tool call in this pipeline.**
> Worktree isolation creates a detached copy of the repo. Implementation subagents on isolated
> worktrees cannot see changes made by predecessor tasks, and review subagents end up checking
> a stale copy rather than the live feature branch. All subagents MUST run directly on the
> shared feature branch with no isolation parameter.

---

## Architecture Principles

- **Files are the API**: Every phase writes its output to a file. Subsequent phases read only those files — never the conversation history.
- **Main agent is an orchestrator**: The main agent only routes work, presents summaries, and asks for human approval. It does not accumulate analysis results in its own context.
- **Human checkpoints**: The pipeline pauses after Design and after Task Decomposition. The user reviews and approves before execution continues.
- **Single feature branch for implementation**: All tasks run directly on one shared feature branch (not isolated worktrees). This ensures dependent tasks see the changes from their predecessors and review agents check the right location.
- **Parallel tasks do not self-commit**: When running parallel task groups, each agent writes its file changes but does NOT run `git commit`. The main agent does one batch commit after each parallel group completes, eliminating git race conditions.
- **Parallel where safe**: Independent implementation tasks within Phase 5 can run in parallel. Phases 1–4 are strictly sequential (each phase depends on the previous phase's output file).
- **State is persisted to disk**: Every phase transition updates `state.json` via the state manager script. This enables reliable resume after interruption and survives context compaction.

---

## Agent Invocation Convention

All named agents are defined in `agents/` as `.md` files. Invoke each agent using the **Agent tool** with:
- `description`: short description of the phase
- `prompt`: phase-specific context (workspace path, task number, etc.)

The agent's `.md` file provides the system prompt (role, instructions, output format). The orchestrator's prompt passes only **runtime parameters** — do NOT duplicate the agent's instructions.

**File-writing responsibility:**
- **Phases 1–4b, 6**: The agent returns its output as a string. The **orchestrator** writes the return value to the appropriate file (e.g., `analysis.md`, `review-{N}.md`).
- **Phase 5 (implementer)**: The agent writes **code files** and `impl-{N}.md` directly, because it must interact with the filesystem as part of implementation.
- **Final Verification**: The verifier agent fixes issues directly and reports verbally. No artifact file is written.

**Writing new artifact files (Write tool constraint):** The Write tool requires a prior Read call on the target path. For artifact files that do not yet exist, use one of two patterns before calling Write:
- Create an empty file first: `cat /dev/null > {path}` via Bash, then Read the empty file, then Write.
- Write directly via Bash heredoc: `cat <<'EOF' > {path}` ... `EOF` — skipping the Write tool entirely for the initial creation.

---

## State Management

Pipeline state is tracked in `{workspace}/state.json` and managed by `scripts/state-manager.sh`. This enables:

1. **Resume after interruption** — re-invoke the skill with the workspace path to pick up where you left off
2. **Hook enforcement** — hooks read `state.json` to know which phase is active and enforce constraints
3. **Compaction resilience** — state survives context compaction because it's on disk, not in memory

### State Manager Commands

The orchestrator calls the state manager via Bash before and after each phase:

```bash
SM="scripts/state-manager.sh"

# Initialize state (during Workspace Setup)
$SM init {workspace} {spec-name}

# Before spawning a phase agent
$SM phase-start {workspace} {phase}

# After a phase agent completes successfully
$SM phase-complete {workspace} {phase}

# On phase failure
$SM phase-fail {workspace} {phase} "error message"

# At human checkpoints
$SM checkpoint {workspace} {phase}

# After Checkpoint B approval — populate tasks from tasks.md
$SM task-init {workspace} '{"1": {"title": "...", "executionMode": "sequential", "implStatus": "pending", "implRetries": 0, "reviewStatus": "pending", "reviewRetries": 0}, ...}'

# During Phase 5-6 — update individual task progress
$SM task-update {workspace} {N} implStatus in_progress
$SM task-update {workspace} {N} implStatus completed
$SM task-update {workspace} {N} reviewStatus completed_pass

# Track revisions
$SM revision-bump {workspace} design
$SM revision-bump {workspace} tasks

# Set branch name when created
$SM set-branch {workspace} feature/{spec-name}

# Get resume information
$SM resume-info {workspace}
```

### Hooks (Automatic Enforcement)

Hooks are defined in `hooks/hooks.json` and run automatically:

| Hook | Trigger | What it enforces |
|------|---------|-----------------|
| **PreToolUse** (Edit\|Write\|Bash) | Every Edit/Write/Bash call | Blocks source file writes during Phase 1-2; blocks `git commit` during parallel Phase 5 |
| **PostToolUse** (Agent) | Every Agent call completes | Warns if agent output is empty or missing expected verdict (APPROVE/APPROVE_WITH_NOTES/REVISE/PASS/FAIL) |
| **Stop** | Claude tries to stop | Blocks stop if pipeline is active and `summary.md` doesn't exist yet |

---

## Architecture Overview

### Agent Roster

| Phase | Agent | Reads | Writes |
| --- | --- | --- | --- |
| Workspace Setup | **Main agent** | — | `request.md`, `state.json` |
| 1 — Situation Analysis | `situation-analyst` | `request.md` | `analysis.md` |
| 2 — Investigation | `investigator` | `request.md`, `analysis.md` | `investigation.md` |
| 3 — Design | `architect` | `request.md`, `analysis.md`, `investigation.md` | `design.md` |
| 3b — Design AI Review | `design-reviewer` | `request.md`, `analysis.md`, `investigation.md`, `design.md` | `review-design.md` |
| Checkpoint A | **Main agent** | `design.md`, `review-design.md` | `state.json` |
| 4 — Task Decomposition | `task-decomposer` | `request.md`, `design.md`, `investigation.md` | `tasks.md` |
| 4b — Tasks AI Review | `task-reviewer` | `request.md`, `design.md`, `investigation.md`, `tasks.md` | `review-tasks.md` |
| Checkpoint B | **Main agent** | `tasks.md`, `review-tasks.md` | `state.json` |
| 5 — Implementation | `implementer` | `request.md`, `design.md`, `tasks.md`, `review-{dep}.md` | code files, `impl-{N}.md` |
| 6 — Review | `impl-reviewer` | `request.md`, `tasks.md`, `design.md`, `impl-{N}.md`, code files | `review-{N}.md` |
| 7 — Comprehensive Review | `comprehensive-reviewer` | `request.md`, `design.md`, `tasks.md`, all `impl-{N}.md`, all `review-{N}.md`, code diff | `comprehensive-review.md` |
| Final Verification | `verifier` | feature branch | — |
| PR Creation | **Main agent** | — | PR on GitHub |
| Final Summary | **Main agent** | artifacts vary by task_type (see Final Summary section); also reads `analysis.md`, `investigation.md` (where present) for Improvement Report | `summary.md`, `state.json` |
| Post to Source | **Main agent** | `summary.md`, `request.md` (source metadata) | GitHub/Jira comment (if applicable) |

**Key constraint:** The main agent never reads code files directly. It only reads the small artifact files
(`analysis.md`, `design.md`, `tasks.md`, `review-{N}.md`) to stay token-efficient.

**Each subagent invocation runs to completion** — subagents are not paused or resumed mid-task.
When a phase needs to be retried (rejection or FAIL), a *new* subagent is spawned from scratch
with the previous output as additional context.

---

## Resume Check

**Before anything else**, determine if this is a fresh start or a resume:

1. Check if `$ARGUMENTS` matches an existing workspace path (contains `.specs/` or `state.json`).
2. If yes, set `{workspace}` to that path. Run:
   ```bash
   scripts/state-manager.sh resume-info {workspace}
   ```
   If the command fails (non-zero exit), `state.json` may be corrupted. Inform the user and offer to either reinitialize the workspace or abort.

3. From the resume info, restore `{spec-name}`, `{branch}`, and determine the resume point:

   | `currentPhaseStatus` | Action |
   |---------------------|--------|
   | `completed` | Pipeline already done. Report to user and stop. |
   | `awaiting_human` | Re-present the checkpoint (A or B) to the user. |
   | `in_progress` | Phase was interrupted mid-execution. Re-run it from scratch. |
   | `failed` | Re-run the failed phase. |
   | `pending` | Start this phase normally. |

4. Restore `{task_type}`, `{skipped_phases}`, `{auto_approve}`, `{effort}`, and `{flow_template}` from resume-info:

   - Set `{task_type}` from `resume_info.taskType`.
   - Set `{skipped_phases}` from `resume_info.skippedPhases`.
   - Set `{auto_approve}` from `resume_info.autoApprove` (defaults to `false` if absent — pre-F3 pipeline).
   - Set `{skip_pr}` from `resume_info.skipPr` (defaults to `false` if absent).
   - Set `{use_current_branch}` from `resume_info.useCurrentBranch` (defaults to `false` if absent).
     If `true`, also set `{existing_branch}` from `resume_info.branch`.
   - Set `{debug_mode}` from `resume_info.debug` (defaults to `false` if absent — pre-debug pipeline).
   - **If `taskType` is `null`** in resume-info (pipeline was started before F4 was deployed):
     default `{task_type}` to `feature` and log a warning:
     > "Warning: taskType not found in state.json (pre-F4 pipeline). Defaulting to 'feature' (full pipeline)."
   - **If `skippedPhases` is empty or absent and `taskType` is non-null**:
     re-derive `{skipped_phases}` from the skip table in Workspace Setup using the restored `{task_type}`.
     Do NOT call `skip-phase` again — the state machine has already advanced past those phases.
     This is purely an in-context variable restoration for the orchestrator's skip-gate checks.
   - **If `effort` is `null`** in resume-info (pre-F13 pipeline): set `{effort}` to `M` in-context and
     log a note: "Note: effort not found in state.json (pre-F13 pipeline). Using M in-context for display only."
     Do NOT call `set-effort` — the `skippedPhases` were already correctly set by the original 1D task-type
     dispatch. The effort value is in-context only for Final Summary display.
   - **If `flowTemplate` is `null`** in resume-info (pre-F13 pipeline): re-derive from `(taskType, M)` using
     the flow template matrix in Workspace Setup and store in `{flow_template}` in-context only.
     Do NOT call `set-flow-template` — the original `skippedPhases` remain authoritative.
     This is in-context only for display and logging.
   - Retain `{effort}` and `{flow_template}` as in-context variables for the duration of the resumed pipeline.

5. For Phase 5-6 resume: check `pendingTasks` and `completedTasks` from resume-info. Skip completed tasks, resume from the first incomplete one.

6. Print a resume summary to the user:
   ```
   Resuming pipeline from {currentPhase} ({currentPhaseStatus}).
   Completed phases: [{completedPhases}]
   Task type: {task_type}
   Effort: {effort}
   Flow template: {flow_template}
   Auto mode: {auto_approve}
   Skip PR: {skip_pr}
   Debug mode: {debug_mode}
   Use current branch: {use_current_branch}
   Skipped phases: [{skipped_phases}]
   Tasks: {completedTasks}/{totalTasks} done.
   Workspace: {workspace}
   ```

7. **Skip to the current phase** — do not re-run completed phases.

If `$ARGUMENTS` does NOT match an existing workspace, proceed to Input Validation below.

---

## Input Validation

**Before Workspace Setup**, validate that `$ARGUMENTS` represents a coherent, actionable development request. This prevents wasting tokens on nonsensical or clearly invalid input.

### Step 1: Deterministic checks (script-enforced)

Run the validation script:
```bash
bash scripts/validate-input.sh "$ARGUMENTS"
```

If the script exits non-zero, **stop the pipeline immediately** and relay the error message from stderr to the user. Do NOT proceed to Workspace Setup.

The script checks: empty input, minimum length after flag stripping, and URL format validation.

### Step 2: Semantic validation (LLM assessment)

After the script passes, evaluate whether `$ARGUMENTS` is a coherent software development request. Reject if ANY of the following apply:
- **Gibberish**: random characters, keyboard mashing, or meaningless word sequences (e.g., `asdf jkl qwer`, `あああいいいうう`, `foo bar baz qux quux`)
- **Not a development task**: the text is clearly unrelated to software development (e.g., a recipe, a poem, casual conversation, general knowledge questions)
- **Ambiguous beyond interpretation**: the text contains real words but forms no actionable instruction even with generous interpretation (e.g., `blue sky thinking`, `maybe something`)

If rejected, stop immediately with a specific reason:
> "Error: The input does not appear to be a valid development task.
> Reason: {specific reason — e.g., 'Input appears to be random characters' or 'Input is not related to software development'}
>
> Please provide a concrete development task such as:
> - `Add retry logic to the API client`
> - `Fix the null pointer in user authentication`
> - `Refactor the payment module to use the strategy pattern`
> - A GitHub Issue URL or Jira Issue URL"

### Borderline cases — DO NOT reject

- Terse but valid instructions (e.g., `fix login bug`, `add tests`) — **accept**
- Misspelled but understandable text (e.g., `ad retrry to api cliant`) — **accept**
- Vague but development-related (e.g., `improve performance`) — **accept** (the pipeline's analysis phase will seek clarification)
- Japanese or other non-English development requests — **accept**

If validation passes, proceed to Step 3.

### Step 3: External dependency checks

If `$ARGUMENTS` contains a Jira URL (`*.atlassian.net/browse/*`), check whether `mcp__atlassian__getJiraIssue` appears in your available tools. If it does **not** appear, stop immediately:

> "Error: Jira integration requires the Atlassian plugin, which is not installed.
>
> Please run the following commands:
> ```
> /plugin install atlassian@claude-plugins-official
> /reload-plugins
> ```
> Then re-run this pipeline command."

Do NOT proceed to Workspace Setup — the pipeline will fail later when attempting to fetch the Jira issue.

If all checks pass, proceed to Workspace Setup.

---

## Workspace Setup

Before running any phase, establish the workspace:

0. **Branch check** — immediately run `git branch --show-current` to detect the current branch.
   - If the current branch is `main` (or `master`): proceed normally. Set `{use_current_branch}` = false.
   - If the current branch is **not** `main`/`master`: prompt the user **before any other work**:
     > "Current branch: **{current_branch}**
     >
     > You are not on the main branch. How would you like to proceed?
     > 1. **Use this branch** — work directly on `{current_branch}` (no new branch will be created)
     > 2. **Create a new branch** — a new feature branch will be created as usual"
     Wait for the user's response.
     - If the user chooses **1 (use this branch)**: set `{use_current_branch}` = true and `{existing_branch}` = `{current_branch}`.
     - If the user chooses **2 (create a new branch)**: set `{use_current_branch}` = false. Proceed normally.

1. Derive a short `{spec-name}` slug from `$ARGUMENTS` — 2–4 lowercase words joined by hyphens
   that capture the essence of the work (e.g. `yaml-workflow-loader`, `fix-auth-timeout`,
   `refactor-dry-run`). Do this now, before reading any code.
2. Run `date +"%Y%m%d"` and store the result as `{date}`.
3. Create directory: `.specs/{date}-{spec-name}/`
4. **Detect source type** — parse `$ARGUMENTS` to determine the input source:
   - If it matches `https://github.com/.*/issues/\d+` → `source_type: github_issue`. Extract the owner, repo, and issue number. Fetch the issue body using `gh issue view <number> --repo <owner>/<repo> --json title,body` and include it as context.
   - If it matches `https://.*\.atlassian\.net/browse/[A-Z]+-\d+` → `source_type: jira_issue`. Extract the issue key. Fetch the issue details using `mcp__atlassian__getJiraIssue` and include the summary and description as context.
   - Otherwise → `source_type: text`.
5. **Detect task type, effort, and execution flags** — determine `{task_type}`, `{effort}`, and `{auto_approve}` using this priority order:

   a. **Explicit flag**: if `$ARGUMENTS` contains `--type=<value>`, use that value.
      Strip the flag from the task description before creating `request.md`.
      Valid values: `feature`, `bugfix`, `investigation`, `docs`, `refactor`.
      If the value is unrecognised, warn the user and default to `feature`.

   a-ii. **Effort flag**: if `$ARGUMENTS` contains `--effort=<value>`, use that value.
      Strip `--effort=<value>` from the task description before creating `request.md`.
      Valid values: `XS`, `S`, `M`, `L`.
      If the value is unrecognised, warn the user and proceed to the effort detection chain.
      Store the value as `{effort_flag}` for use in the effort detection chain below.

   b. **Auto-approve flag**: if `$ARGUMENTS` contains `--auto` (bare, without a value suffix),
      set `{auto_approve}` to `true`.
      Strip `--auto` from the task description before creating `request.md`.
      Print a one-time notice immediately:
      > "Running in autonomous mode — checkpoints will be auto-approved when AI verdict is APPROVE."
      If `--auto` is absent, set `{auto_approve}` to `false`.

      **Edge case**: if `$ARGUMENTS` contains `--auto=<value>` (with any value suffix), treat
      `--auto` as **absent**, log a warning, and set `{auto_approve}` to `false`:
      > "Warning: Unrecognised --auto value; ignored. Use bare --auto for autonomous mode."
      Do NOT strip `--auto=<value>` from the task description in this case.

   b-ii. **Skip-PR flag**: if `$ARGUMENTS` contains `--nopr` (bare, without a value suffix),
       set `{skip_pr}` to `true`.
       Strip `--nopr` from the task description before creating `request.md`.
       Print a one-time notice immediately:
       > "PR creation will be skipped — changes will be committed to the feature branch only."
       If `--nopr` is absent, set `{skip_pr}` to `false`.

   b-iii. **Debug flag**: if `$ARGUMENTS` contains `--debug` (bare, without a value suffix),
       set `{debug_mode}` to `true`.
       Strip `--debug` from the task description before creating `request.md`.
       Print a one-time notice immediately:
       > "Debug mode enabled — an execution flow report will be appended to summary.md."
       If `--debug` is absent, set `{debug_mode}` to `false`.

       **Edge case**: if `$ARGUMENTS` contains `--debug=<value>` (with any value suffix), treat
       `--debug` as **absent** and set `{debug_mode}` to `false`:
       > "Warning: Unrecognised --debug value; ignored. Use bare --debug for debug report."
       Do NOT strip `--debug=<value>` from the task description in this case.

       After detection, if `{debug_mode}` is `true`:
       ```bash
       $SM set-debug {workspace}
       ```

   c. **Jira issue type** (only when `source_type` is `jira_issue`): map `issuetype.name`
      from the already-fetched Jira issue object using this table:

      | Jira issuetype.name                          | task_type                                           |
      |----------------------------------------------|-----------------------------------------------------|
      | Bug                                          | bugfix                                              |
      | Story, Improvement, New Feature, Epic        | feature                                             |
      | Sub-task                                     | inherit from parent (fetch parent, re-apply table;  |
      |                                              | default to feature if parent also ambiguous)        |
      | Task                                         | feature (default; override with --type= if wrong)   |

   d. **GitHub labels** (only when `source_type` is `github_issue`): inspect `labels[].name`
      from the already-fetched issue using keyword matching:
      - Contains `bug`, `type: bug`, `kind/bug`, `fix` → `bugfix`
      - Contains `documentation`, `docs` → `docs`
      - Contains `refactor`, `cleanup`, `chore` → `refactor`
      - Contains `investigation`, `research`, `spike` → `investigation`
      - Contains `enhancement`, `feature`, `new feature` → `feature`
      - Multiple conflicting labels → present ambiguity to user and ask for clarification.
      - No matching labels → default to `feature`.

   e. **Heuristic from plain text** (only when `source_type` is `text`): inspect the
      first sentence of `$ARGUMENTS` for keywords:
      - Contains `fix`, `bug`, `regression`, `crash`, `error` → `bugfix`
      - Contains `investigate`, `research`, `explore`, `spike`, `why` → `investigation`
      - Contains `document`, `docs`, `readme`, `changelog`, `comment` → `docs`
      - Contains `refactor`, `clean up`, `reorganise`, `move`, `rename` → `refactor`
      - Otherwise → `feature`

   When the type is determined by heuristic (cases c, d, e), mark `{task_type_is_heuristic}` = true.
   Do NOT prompt yet — wait until after effort detection to combine both prompts if needed.
   Skip this flag if `--type=` was used explicitly (case a).

   After effort detection (see step 5f below):
   - If `{task_type_is_heuristic}` is true **and** `{effort_is_heuristic}` is true (both heuristic): combine into one prompt:
     > "I've inferred:
     >   Task type: **{task_type}** (because: {reason})
     >   Effort: **{effort}** (because: {reason})
     >
     > Does this look correct? (yes / adjust: type=\<type\>, effort=\<effort\>)"
   - If only `{task_type_is_heuristic}` is true: prompt for task type only:
     > "Detected task type: **{task_type}**. Is this correct?
     > (Alternatives: feature / bugfix / investigation / docs / refactor)"
   - If only `{effort_is_heuristic}` is true: prompt for effort only:
     > "Inferred effort: **{effort}** (because: {reason}). Is this correct?
     > (Options: XS / S / M / L)"
   - If neither is heuristic (both explicit): skip confirmation entirely.

   Wait for confirmation or correction before proceeding (when at least one prompt is needed).

   **Effort detection chain (step 5f):**

   Determine `{effort}` using the following priority order. Stop at the first successful detection.

   f-1. **Explicit flag** (`--effort=` from step a-ii): if `{effort_flag}` is set and valid, use it.
        Set `{effort_is_heuristic}` = false.

   f-2. **Jira story points** (only when `source_type` is `jira_issue`): read `customfield_10016`
        from the already-fetched Jira issue object. If the field is absent, `None`, non-numeric,
        or zero, fall through silently to f-3. Mapping:
        - SP <= 1 → `XS`
        - SP 2–3 → `S`
        - SP 5 → `M`
        - SP 8+ → `L`
        Set `{effort_is_heuristic}` = false.

   f-3. **Heuristic from description complexity**: inspect `$ARGUMENTS` (after flag stripping) for
        complexity signals:
        - Single word, single filename, or explicit "1-liner" / "typo" phrasing → `XS`
        - Short phrase under ~10 words, e.g. "fix X in Y" → `S`
        - Multi-sentence description or mentions multiple files/components → `M`
        - Description spans many components, or mentions "architecture", "redesign", "migration" → `L`
        Set `{effort_is_heuristic}` = true. (Prompt will fire — see combined prompt logic above.)

   f-4. **Default**: `M` (safe fallback — matches current single-template behavior).
        Set `{effort_is_heuristic}` = false.

   After determining `{effort}`, store as in-context variable `{effort}`. The `$SM set-effort` call
   happens in step 7 after workspace initialization.

   **Flow template derivation (step 5g):**

   After effort detection, derive `{flow_template}` from the 2D lookup table:

   ```
   (task_type, effort) → flow_template

                XS       S        M         L
   feature    | lite   | light  | standard | full
   bugfix     | direct | lite   | light    | standard
   refactor   | lite   | light  | standard | full
   docs       | direct | direct | lite     | light
   investig.  | lite   | lite   | light    | standard
   ```

   Store `{flow_template}` as an in-context variable. The `$SM set-flow-template` call happens in step 7
   after workspace initialization.

   **`full` template + `--auto` conflict**: if `{flow_template}` is `full` AND `{auto_approve}` is `true`:
   prompt the user once:
   > "The `full` template requires all checkpoints to be manually approved.
   > Passing --auto conflicts with this requirement.
   > Continue without auto-approve? (yes / abort)"

   On yes: do NOT call `set-auto-approve`. Set `{auto_approve}` = false.
   On abort: stop (workspace not yet initialized — no `$SM abandon` needed).

6. Write `.specs/{date}-{spec-name}/request.md` with YAML front matter and body:
   ```markdown
   ---
   source_type: github_issue | jira_issue | text
   source_url: <URL if applicable, otherwise omit>
   source_id: <issue number/key if applicable, otherwise omit>
   task_type: feature | bugfix | investigation | docs | refactor
   ---

   <$ARGUMENTS and any fetched issue context>
   ```
7. **Initialize state** — run these commands in order (**`set-task-type`, `set-effort`, and `set-flow-template` are MANDATORY immediately after `init`** — see "Mandatory Calls" section):
   ```bash
   scripts/state-manager.sh init {workspace} {spec-name}
   scripts/state-manager.sh set-task-type {workspace} {task_type}
   scripts/state-manager.sh set-effort {workspace} {effort}
   scripts/state-manager.sh set-flow-template {workspace} {flow_template}
   ```
   If `--auto` was present AND the conflict check passed (i.e. `{auto_approve}` is `true`), also record it:
   ```bash
   scripts/state-manager.sh set-auto-approve {workspace}
   ```
   If `--nopr` was present (i.e. `{skip_pr}` is `true`), also record it:
   ```bash
   scripts/state-manager.sh set-skip-pr {workspace}
   ```
   If the user chose to use the current branch (i.e. `{use_current_branch}` is `true`), record it:
   ```bash
   scripts/state-manager.sh set-use-current-branch {workspace} {existing_branch}
   ```
   Then call `skip-phase` for each phase in the canonical skip sequence for `({task_type}, {effort})`,
   in canonical PHASES-array order, one call at a time with no gaps.

   Use the **20-cell canonical skip sequence table** below — this is the authoritative pre-computed
   union of the template base skip set and the task-type supplemental skip set for each cell:

   | (task_type, effort) | Flow template | Workspace Setup skip-phase calls (in order) | Phase 1 block note |
   |---------------------|--------------|---------------------------------------------|---------------------|
   | `(feature, XS)` | `lite` | `phase-4b`, `checkpoint-b`, `phase-6`, `phase-7` | After phase-complete phase-1: call `skip-phase phase-2` |
   | `(feature, S)` | `light` | `phase-4b`, `checkpoint-b`, `phase-7` | (none) |
   | `(feature, M)` | `standard` | (none) | (none) |
   | `(feature, L)` | `full` | (none) | (none); autoApprove forced false if --auto conflict |
   | `(bugfix, XS)` | `direct` | `phase-1`, `phase-2`, `phase-3`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-6`, `phase-7` | (none; direct flow — stub synthesis, then phase-3b + checkpoint-a run on stub) |
   | `(bugfix, S)` | `lite` | `phase-4`, `phase-4b`, `checkpoint-b`, `phase-6`, `phase-7` | After phase-complete phase-1: call `skip-phase phase-2` |
   | `(bugfix, M)` | `light` | `phase-4`, `phase-4b`, `checkpoint-b`, `phase-7` | (none) |
   | `(bugfix, L)` | `standard` | `phase-4`, `phase-4b`, `checkpoint-b`, `phase-7` | (none) |
   | `(refactor, XS)` | `lite` | `phase-4b`, `checkpoint-b`, `phase-6`, `phase-7` | After phase-complete phase-1: call `skip-phase phase-2` |
   | `(refactor, S)` | `light` | `phase-4b`, `checkpoint-b`, `phase-7` | (none) |
   | `(refactor, M)` | `standard` | (none) | (none) |
   | `(refactor, L)` | `full` | (none) | (none); autoApprove forced false if --auto conflict |
   | `(docs, XS)` | `direct` | `phase-1`, `phase-2`, `phase-3`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-6`, `phase-7` | (none; direct flow — stub synthesis, then phase-3b + checkpoint-a run on stub) |
   | `(docs, S)` | `direct` | `phase-1`, `phase-2`, `phase-3`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-6`, `phase-7` | (none; direct flow — stub synthesis, then phase-3b + checkpoint-a run on stub) |
   | `(docs, M)` | `lite` | `phase-2`, `phase-3`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-6`, `phase-7` | phase-2 is in Workspace Setup skips; do NOT call skip-phase phase-2 in Phase 1 block |
   | `(docs, L)` | `light` | `phase-2`, `phase-3`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-7` | (none) |
   | `(investigation, XS)` | `lite` | `phase-3`, `phase-3b`, `checkpoint-a`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-5`, `phase-6`, `phase-7`, `final-verification`, `pr-creation` | After phase-complete phase-1: call `skip-phase phase-2` only if phase-2 not already in {skipped_phases} |
   | `(investigation, S)` | `lite` | `phase-3`, `phase-3b`, `checkpoint-a`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-5`, `phase-6`, `phase-7`, `final-verification`, `pr-creation` | After phase-complete phase-1: call `skip-phase phase-2` only if phase-2 not already in {skipped_phases} |
   | `(investigation, M)` | `light` | `phase-3`, `phase-3b`, `checkpoint-a`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-5`, `phase-6`, `phase-7`, `final-verification`, `pr-creation` | (none) |
   | `(investigation, L)` | `standard` | `phase-3`, `phase-3b`, `checkpoint-a`, `phase-4`, `phase-4b`, `checkpoint-b`, `phase-5`, `phase-6`, `phase-7`, `final-verification`, `pr-creation` | (none) |

   Example for `(bugfix, M)` → `light` template:
   ```bash
   SM="scripts/state-manager.sh"
   for phase in phase-4 phase-4b checkpoint-b phase-7; do
     $SM skip-phase {workspace} $phase
   done
   ```

   **Important:** `skip-phase` advances `currentPhase` to the next phase in the PHASES array.
   Phases must be skipped in canonical PHASES-array order, one call at a time, without gaps.
   Calling `skip-phase` out of order would corrupt the state machine.

   **`direct` template stub synthesis:** If `{flow_template}` is `direct`, after all Workspace Setup
   phase-skips are applied, write three stub files. Phase 3b (AI Design Review) and Checkpoint A
   (Human Design Review) will run on these stubs before Phase 5 begins:

   `{workspace}/analysis.md`:
   ```markdown
   ---
   stub: true
   ---

   # Analysis — Direct Flow

   Direct flow: no situation analysis was performed.
   ```

   `{workspace}/design.md`:
   ```markdown
   ---
   task_type: {task_type}
   stub: true
   ---

   # Design — Direct Flow

   Direct flow: implement per request.md. No architectural analysis was performed.
   ```

   `{workspace}/tasks.md`:
   ```markdown
   ## Task 1: Implement the change [sequential]
   **Design ref:** Direct — see request.md
   **Depends on:** None
   **Files:** (from request.md)
   **Acceptance criteria:**
   - [ ] Change described in request.md is implemented
   - [ ] Existing tests pass
   ```

   Then initialise task tracking:
   ```bash
   $SM task-init {workspace} '{"1": {"title": "Implement", "implStatus": "pending", "reviewStatus": "pending", "executionMode": "sequential", "implRetries": 0, "reviewRetries": 0}}'
   ```

   After stub synthesis and task-init, proceed to Phase 3b (which reviews the stub design.md)
   and then Checkpoint A (where the human confirms the request before implementation begins).
   Phase 4 was already skipped, so after Checkpoint A the pipeline proceeds to Phase 5.

   Retain `{task_type}`, `{effort}`, `{flow_template}`, `{skipped_phases}`, `{auto_approve}`, `{skip_pr}`, and `{debug_mode}` as
   in-context variables for the duration of the pipeline (same pattern as `{workspace}` and `{spec-name}`).
   All subsequent phase blocks refer to these variables without re-reading `state.json`.

8. Store the workspace path as `{workspace}` — all subsequent phases read from and write to this
   directory. Use `{workspace}` as the shorthand in all prompts below.

---

## Mandatory Calls — Never Skip

> **These categories of state-manager calls are MANDATORY. Skipping any of them causes downstream failures (null taskType/effort errors, empty metrics, broken checkpoint safety nets, wrong skip sequences). Treat each as a hard requirement, not a suggestion.**

| When | Command | Consequence if skipped |
|------|---------|----------------------|
| **The Initialize-state step of Workspace Setup** (immediately after `$SM init`) | `$SM set-task-type {workspace} {task_type}` | `taskType: null` → Final Summary dispatch error, wrong phase skipping |
| **The Initialize-state step of Workspace Setup** (after `set-task-type`) | `$SM set-effort {workspace} {effort}` | `effort: null` → pre-tool-hook Rule 3f warning on every phase-1 start; effort missing from resume-info |
| **The Initialize-state step of Workspace Setup** (after `set-effort`) | `$SM set-flow-template {workspace} {flow_template}` | `flowTemplate: null` → flow template missing from resume-info; wrong template restored on resume |
| **After every Agent tool call** | `$SM phase-log {workspace} {phase-id} {tokens} {duration} {model}` | Execution Stats table in Final Summary is empty |
| **At every Checkpoint (A/B)** | `$SM checkpoint {workspace} {phase}` before `$SM phase-complete` | `currentPhaseStatus` never reaches `awaiting_human` → checkpoint hook guard blocks `phase-complete` (exit 2), stop-hook safety net bypassed |

---

## Phase Execution

**State update pattern** — wrap every phase like this. **All three post-agent calls (write artifact → phase-log → phase-complete) are mandatory. Do NOT skip phase-log.**

```bash
SM="scripts/state-manager.sh"

# Before spawning the agent:
$SM phase-start {workspace} {phase-id}

# Spawn agent... (Agent tool call)
# The Agent tool returns metadata including: total_tokens, duration_ms

# After agent returns successfully — ALL THREE STEPS ARE REQUIRED:
# 1. Write artifact file (orchestrator responsibility for Phases 1-4b, 6)
# 2. Log phase metrics (MANDATORY — do NOT skip):
$SM phase-log {workspace} {phase-id} {total_tokens} {duration_ms} {model}
# 3. Advance state:
$SM phase-complete {workspace} {phase-id}

# If agent fails or returns empty:
$SM phase-fail {workspace} {phase-id} "description of failure"
```

**Metrics logging** — after every Agent tool call, extract `total_tokens` and `duration_ms` from the agent's response metadata and call `phase-log`. For Phase 5-6 (per-task agents), use `phase-log {workspace} "task-{N}-impl" {tokens} {duration} {model}` and `phase-log {workspace} "task-{N}-review" {tokens} {duration} {model}` to track each task agent individually. This data is used in the Final Summary to produce a cost/time breakdown table.

---

### Phase 1 — Situation Analysis

**Agent**: `situation-analyst` (standard flow) or `analyst` (lite template)
**Output**: Return value → orchestrator writes to `analysis.md` (and `investigation.md` for lite template)

```bash
$SM phase-start {workspace} phase-1
```

**Conditional branch on `{flow_template}`:**

**If `{flow_template}` == `"lite"`** — spawn the `analyst` agent (merged Phase 1+2):

```
{workspace} = {workspace}
```

Write the return value to **both**:
- `{workspace}/analysis.md` (Sections 1-4: relevant files, key interfaces, existing tests, known constraints)
- `{workspace}/investigation.md` (Sections 5-10: root cause / integration points, edge cases, external dependencies, prior art, ambiguities, deletion/rename impact)

Both files must be written before proceeding.

```bash
$SM phase-log {workspace} phase-1 {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-1
```

Then skip phase-2 — but only if it is NOT already in `{skipped_phases}` (for `(docs, M)`, phase-2 is already in Workspace Setup skips; for `(investigation, XS/S)`, phase-2 is not in Workspace Setup skips so this call proceeds):

```bash
# Only call this if phase-2 is NOT already in {skipped_phases}:
$SM skip-phase {workspace} phase-2
```

> **Important:** This is the ONLY place `skip-phase phase-2` is called for the `lite` template. It is NOT called in Workspace Setup. Do NOT call `phase-start phase-2`.

**Else (standard flow)** — spawn the `situation-analyst` agent:

```
{workspace} = {workspace}
```

Write the return value to `{workspace}/analysis.md`.

```bash
$SM phase-log {workspace} phase-1 {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-1
```

> **`docs/M` and `docs/L` flow — stub synthesis after Phase 1:** For `(docs, M)` (`lite` template) and
> `(docs, L)` (`light` template), Phase 1 runs but Phases 2, 3, Phase 4, Phase 4b, and
> checkpoint-b were already skipped during Workspace Setup. After Phase 1 completes, synthesise
> `design.md` and `tasks.md` stubs, then proceed to Phase 3b (AI review of stub) and Checkpoint A
> (human confirmation):
>
> `{workspace}/design.md`:
>
> ```markdown
> ---
> task_type: docs
> stub: true
> ---
>
> # Design — Documentation Update
>
> This is a documentation-only task. No code architecture changes are planned.
>
> ## Approach
>
> Direct documentation edits as described in request.md.
>
> ## Files to Modify
>
> (Orchestrator fills this in from analysis.md after Phase 1 completes.)
>
> ## Test Strategy
>
> Manual review of rendered output. No automated tests required.
> ```
>
> `{workspace}/tasks.md`:
>
> ```markdown
> ## Task 1: Apply documentation edits [sequential]
> **Design ref:** Direct edits
> **Depends on:** None
> **Files:** (from analysis.md)
> **Acceptance criteria:**
> - [ ] All documentation files listed in request.md are updated
> - [ ] No broken links introduced
> - [ ] Formatting is consistent with surrounding content
> ```
>
> Then initialise task tracking:
>
> ```bash
> $SM task-init {workspace} '{"1": {"title": "Apply documentation edits", "executionMode": "sequential", "implStatus": "pending", "implRetries": 0, "reviewStatus": "pending", "reviewRetries": 0}}'
> ```
>
> After stub synthesis and task-init, proceed to Phase 3b (AI reviews the stub design.md) and
> Checkpoint A (human confirms the documentation plan before implementation begins).
>
> **Note:** For `(docs, XS)` and `(docs, S)` (`direct` template), Phase 1 was skipped during Workspace
> Setup and stub synthesis was done in the Initialize-state step of Workspace Setup. The Phase 1 block is not reached for
> these cells. Phase 3b and Checkpoint A still run on the stubs.

---

### Phase 2 — Investigation

> **Skip gate:** If `phase-2` is in `{skipped_phases}`: skip-phase was already called during Workspace Setup (for `docs` task-type supplemental, or `docs/M` union), OR it will be called inside the Phase 1 block for `lite`-template flows after `phase-complete phase-1`. Check `{skipped_phases}` — if `phase-2` is present, do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `investigator`
**Output**: Return value → orchestrator writes to `investigation.md`

```bash
$SM phase-start {workspace} phase-2
```

Spawn the `investigator` agent with:

```
{workspace} = {workspace}
```

Write the return value to `{workspace}/investigation.md`.

```bash
$SM phase-log {workspace} phase-2 {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-2
```

---

### Phase 3 — Design

> **Skip gate:** If `phase-3` is in `{skipped_phases}` (present for `docs` supplemental, `investigation` supplemental, and their union with any template): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `architect`
**Output**: Return value → orchestrator writes to `design.md`

```bash
$SM phase-start {workspace} phase-3
```

Spawn the `architect` agent with:

```
{workspace} = {workspace}
```

If this is a revision (Phase 3b returned REVISE):
- Append: `This is a revision. Also read {workspace}/review-design.md for AI review findings to address.`
- Run: `$SM revision-bump {workspace} design`

Write the return value to `{workspace}/design.md`.

```bash
$SM phase-log {workspace} phase-3 {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-3
```

> **`bugfix` flow — stub synthesis:** If `{task_type}` is `bugfix`, after Phase 3 completes, synthesise a `tasks.md` stub and initialise task tracking before proceeding to Phase 3b. (Phase 4, Phase 4b, and checkpoint-b were already skipped during Workspace Setup. `task-init` is allowed here because checkpoint-b is in `skippedPhases`.) Write `{workspace}/tasks.md` with the following content:
>
> ```markdown
> ## Task 1: Implement bug fix [sequential]
> **Design ref:** Fix Strategy section of design.md
> **Depends on:** None
> **Files:** (from design.md architectural changes section)
> **Acceptance criteria:**
> - [ ] Root cause identified in investigation.md is addressed
> - [ ] Regression test is added covering the bug scenario
> - [ ] Existing tests continue to pass
> ```
>
> Then initialise task tracking and proceed directly to Phase 5:
>
> ```bash
> $SM task-init {workspace} '{"1": {"title": "Implement bug fix", "executionMode": "sequential", "implStatus": "pending", "implRetries": 0, "reviewStatus": "pending", "reviewRetries": 0}}'
> ```

---

### Phase 3b — Design AI Review

> **Skip gate:** If `phase-3b` is in `{skipped_phases}` (only for `investigation` task type, where phase-3 is also skipped): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block. Phase 3b is **mandatory for all other task types** — it always runs when Phase 3 runs.

**Agent**: `design-reviewer`
**Output**: Return value → orchestrator writes to `review-design.md`

```bash
$SM phase-start {workspace} phase-3b
```

Immediately after Phase 3 completes, spawn the `design-reviewer` agent with:

```
{workspace} = {workspace}
```

Write the return value to `{workspace}/review-design.md`.

```bash
$SM phase-log {workspace} phase-3b {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-3b
```

- If verdict is **REVISE** (contains at least one CRITICAL finding): re-run Phase 3 with revision context, then re-run Phase 3b. Max 2 cycles before escalating to the human. (This loop applies to all task types where Phase 3 runs — everything except `investigation`.)
- If verdict is **APPROVE** (no findings): continue to Checkpoint A.
- If verdict is **APPROVE_WITH_NOTES** (MINOR findings only): continue to Checkpoint A. Include the MINOR findings from review-design.md in the checkpoint presentation.

---

### Checkpoint A — Design Review (Human)

> **Skip gate 1 (task-type/template):** If `checkpoint-a` is in `{skipped_phases}` (only for `investigation` task type, where phase-3 is also skipped): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block. Checkpoint A is **mandatory for all other task types** — the human always reviews the design before implementation proceeds.

> **Skip gate 2 (auto-approve):** If `{auto_approve}` is `true` AND the AI reviewer verdict in `{workspace}/review-design.md` is APPROVE or APPROVE_WITH_NOTES (no CRITICAL findings): skip this checkpoint.
> Print: "Auto-approving Checkpoint A (AI verdict: APPROVE_WITH_NOTES)." (or APPROVE)
> Call:
> ```bash
> $SM checkpoint {workspace} checkpoint-a
> $SM phase-complete {workspace} checkpoint-a
> ```
> Proceed directly to the next phase block.

**If neither skip gate fired: do not proceed until the user approves.**

> **MANDATORY** — call `$SM checkpoint` before presenting to the user. This sets `currentPhaseStatus: "awaiting_human"`, which is required for `phase-complete` to succeed (enforced by hook guard).

```bash
$SM checkpoint {workspace} checkpoint-a
```

1. Read `{workspace}/review-design.md` for the AI reviewer's verdict and notes.
2. Present to the user:
   - Read `{workspace}/review-design.md` and use the `## Orchestrator Summary` section (approach, key changes, risk level, verdict) — do NOT read `design.md` for this summary.
   - Present: approach chosen, key changes, risk level, AI review verdict and any MINOR findings from `review-design.md`.
   - The workspace path `{workspace}` (so the user can reference it if the session is interrupted)
3. Ask: "Does this design look right? Approve to continue to task decomposition, or share feedback to revise."
4. If the user requests changes: re-run Phase 3 with user feedback appended, then re-run Phase 3b, and re-present.
5. Once approved:
   ```bash
   $SM phase-complete {workspace} checkpoint-a
   ```

---

### Phase 4 — Task Decomposition

> **Skip gate:** If `phase-4` is in `{skipped_phases}` (present in the supplemental skip set for `bugfix`, `docs`, and `investigation` — not in any template base skip set): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `task-decomposer`
**Output**: Return value → orchestrator writes to `tasks.md`

```bash
$SM phase-start {workspace} phase-4
```

Spawn the `task-decomposer` agent with:

```
{workspace} = {workspace}
```

If this is a revision:
- Append: `This is a revision. Also read {workspace}/review-tasks.md for AI review findings to address.`
- Run: `$SM revision-bump {workspace} tasks`

Write the return value to `{workspace}/tasks.md`.

```bash
$SM phase-log {workspace} phase-4 {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-4
```

---

### Phase 4b — Tasks AI Review

> **Skip gate:** If `phase-4b` is in `{skipped_phases}` (present in the base skip set for `lite` and `light` templates, and in the supplemental skip set for `bugfix`, `docs`, and `investigation` — and their unions): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `task-reviewer`
**Output**: Return value → orchestrator writes to `review-tasks.md`

```bash
$SM phase-start {workspace} phase-4b
```

Immediately after Phase 4 completes, spawn the `task-reviewer` agent with:

```
{workspace} = {workspace}
```

Write the return value to `{workspace}/review-tasks.md`.

```bash
$SM phase-log {workspace} phase-4b {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-4b
```

- If verdict is **REVISE** (contains at least one CRITICAL finding): re-run Phase 4 with revision context, then re-run Phase 4b. Max 2 cycles before escalating to the human.
- If verdict is **APPROVE** (no findings): continue to Checkpoint B.
- If verdict is **APPROVE_WITH_NOTES** (MINOR findings only): continue to Checkpoint B. Include the MINOR findings from review-tasks.md in the checkpoint presentation.

---

### Checkpoint B — Task Review (Human)

> **Skip gate 1 (task-type/template):** If `checkpoint-b` is in `{skipped_phases}` (present in the base skip set for `lite` and `light` templates, and in the supplemental skip set for `bugfix`, `docs`, and `investigation` — and their unions): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

> **Skip gate 2 (auto-approve):** If `{auto_approve}` is `true` AND the AI reviewer verdict in `{workspace}/review-tasks.md` is APPROVE or APPROVE_WITH_NOTES (no CRITICAL findings): skip this checkpoint.
>
> IMPORTANT: Auto-approve only fires when BOTH conditions are true:
> - `{auto_approve}` is `true`
> - `review-tasks.md` verdict is APPROVE or APPROVE_WITH_NOTES (no CRITICAL findings)
>
> If either condition is false, continue to the human approval path below. Do NOT conflate the auto-approve path with the human path.
>
> Print: "Auto-approving Checkpoint B (AI verdict: APPROVE_WITH_NOTES)." (or APPROVE)
> Call:
> ```bash
> $SM checkpoint {workspace} checkpoint-b
> $SM phase-complete {workspace} checkpoint-b
> ```
> Proceed to the change-request step below.

**Human approval path — STOP AND WAIT**

If neither skip gate fired:

> **MANDATORY** — call `$SM checkpoint` before presenting to the user. This sets `currentPhaseStatus: "awaiting_human"`, which is required for `phase-complete` to succeed (enforced by hook guard).

1. Call:
   ```bash
   $SM checkpoint {workspace} checkpoint-b
   ```
   This sets `currentPhaseStatus = "awaiting_human"`. **DO NOT call `phase-complete` until the user replies.**

2. Read `{workspace}/review-tasks.md` for the AI reviewer's verdict and notes.
3. Present to the user:
   - Read `{workspace}/review-tasks.md` and use the `## Orchestrator Summary` section (approach, key changes, risk level, verdict) — do NOT read `tasks.md` for this summary.
   - Present: task overview from the summary, risk level, AI review verdict and any MINOR findings from `review-tasks.md`.
   - The workspace path `{workspace}` (for session-recovery reference)
4. Ask: "Do these tasks cover everything? Approve to start implementation, or share feedback to revise."

5. **WAIT FOR USER RESPONSE. Do not proceed further in this message.**

6. **Change-request step** — If the user requests changes: re-run Phase 4 with user feedback appended, then re-run Phase 4b, and re-present.
7. Once the user approves, call:
   ```bash
   $SM phase-complete {workspace} checkpoint-b
   ```

8. **Populate task state** — parse `tasks.md` and initialize task tracking (runs after human approval OR after the auto-approve path above):
   ```bash
   $SM task-init {workspace} '{
     "1": {"title": "Task 1 title", "executionMode": "sequential", "implStatus": "pending", "implRetries": 0, "reviewStatus": "pending", "reviewRetries": 0},
     "2": {"title": "Task 2 title", "executionMode": "parallel", "implStatus": "pending", "implRetries": 0, "reviewStatus": "pending", "reviewRetries": 0}
   }'
   ```

---

### Phase 5 — Implementation

> **Skip gate:** If `phase-5` is in `{skipped_phases}` (present in the supplemental skip set for `investigation` — and its union with any template): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `implementer` (one per task)
**One agent per task** (parallel for `[parallel]` tasks, sequential for `[sequential]` tasks)

```bash
$SM phase-start {workspace} phase-5
```

**Edge case — zero tasks:** If `tasks.md` contains no implementation tasks (e.g., the design concluded no code changes are needed), skip Phase 5-6 and proceed directly to Final Verification.

**Before the first task:** create a feature branch and check it out — **unless** the user chose to use the current branch:
- If `{use_current_branch}` is `true`: skip branch creation. The branch was already recorded during Workspace Setup via `set-use-current-branch`. Log:
  > "Using existing branch: `{existing_branch}` (no new branch created)."
- Otherwise: create a new feature branch as usual:
  ```bash
  git checkout -b feature/{spec-name}
  $SM set-branch {workspace} feature/{spec-name}
  ```
All implementation agents work on this branch. Do NOT use `isolation: worktree`.

**Commit strategy:**
- For `[sequential]` tasks: the agent commits its own changes before finishing.
- For `[parallel]` task groups: agents write file changes but do NOT commit (`git commit`).
  After all agents in the group finish, the main agent does one batch commit covering the
  whole group. This avoids git race conditions.

For each task, update state and spawn the `implementer` agent:

```bash
$SM task-update {workspace} {N} implStatus in_progress
```

```
You are implementing Task {N}: {title}.
{workspace} = {workspace}
{spec-name} = {spec-name}
{branch} = {branch}
Task number: {N}
Commit mode: {sequential|parallel}

Dependencies completed: Tasks {deps}
Dependency review files: {for each dep: `{workspace}/review-{dep}.md`}

Acceptance criteria:
{paste the task's acceptance criteria}
```

After the agent completes:
```bash
$SM phase-log {workspace} "task-{N}-impl" {total_tokens} {duration_ms} {model}
$SM task-update {workspace} {N} implStatus completed
```

For `[parallel]` tasks: launch all agents in the group simultaneously. Wait for all to finish,
then the main agent does one batch commit. Then start the next group.

For `[sequential]` tasks: launch one at a time and wait for completion (each agent self-commits).

---

### Phase 6 — Implementation Review

> **Skip gate:** If `phase-6` is in `{skipped_phases}` (present in the base skip set for `direct` and `lite` templates, and in the supplemental skip set for `investigation` — and their unions): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `impl-reviewer` (one per completed task)
**Output**: Return value → orchestrator writes to `review-{N}.md`

**Review batching optimization:** When multiple completed tasks touch different files, the orchestrator MAY batch them into a single `impl-reviewer` invocation (e.g., "Review Tasks 1, 2, 3"). The reviewer produces per-task verdicts in one response. This saves agent spawn overhead and token cost. Tasks that touch the same file SHOULD be reviewed together so the reviewer sees the cumulative state. Write the combined review output to `{workspace}/review-{first_N}.md` and reference it for all included tasks.

After each task (or batch of tasks) completes, update state and spawn the `impl-reviewer` agent:

```bash
$SM task-update {workspace} {N} reviewStatus in_progress
```

```
Review Task {N}.
{workspace} = {workspace}
Task number: {N}
```

Write the review output to `{workspace}/review-{N}.md`.

After review:
```bash
$SM phase-log {workspace} "task-{N}-review" {total_tokens} {duration_ms} {model}

# If PASS or PASS_WITH_NOTES:
$SM task-update {workspace} {N} reviewStatus completed_pass

# If FAIL — increment retry counter and re-run:
$SM task-update {workspace} {N} reviewStatus completed_fail
$SM task-update {workspace} {N} implRetries 1  # increment: 0→1→2
```

If a review returns `FAIL`: re-run Phase 5 for that task (passing the review file as additional context), then re-run Phase 6. Max 2 attempts per task — check `implRetries` before retrying. If retries exhausted, report to the user and ask how to proceed.

When all tasks are reviewed and passing:
```bash
$SM phase-complete {workspace} phase-5
$SM phase-complete {workspace} phase-6
```

---

### Phase 7 — Comprehensive Review

> **Skip gate:** If `phase-7` is in `{skipped_phases}` (present in the base skip set for `lite`, `light`, and `direct` templates, and in the supplemental skip set for `bugfix`, `docs`, `investigation`, and `refactor` — and their unions): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `comprehensive-reviewer`
**Output**: Return value → orchestrator writes to `comprehensive-review.md`

After all tasks pass individual review, run a holistic review across the entire feature branch.

```bash
$SM phase-start {workspace} phase-7
```

Spawn the `comprehensive-reviewer` agent with:

```
{workspace} = {workspace}
{spec-name} = {spec-name}
```

Write the return value to `{workspace}/comprehensive-review.md`.

```bash
$SM phase-log {workspace} phase-7 {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} phase-7
```

If the comprehensive reviewer made fixes (verdict: IMPROVED), those changes are already committed by the agent. Proceed to Final Verification to confirm nothing is broken.

---

## Final Verification

> **Skip gate:** If `final-verification` is in `{skipped_phases}` (present in the supplemental skip set for `investigation` — and its union with any template): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

**Agent**: `verifier`

```bash
$SM phase-start {workspace} final-verification
```

Spawn the `verifier` agent with:

```
{workspace} = {workspace}
{spec-name} = {spec-name}
```

If new failures are found: the verifier will fix them. If it cannot, report to the user.

```bash
$SM phase-log {workspace} final-verification {total_tokens} {duration_ms} {model}
$SM phase-complete {workspace} final-verification
```

---

## PR Creation

> **Skip gate 1 (task-type/template):** If `pr-creation` is in `{skipped_phases}` (present in the supplemental skip set for `investigation` — and its union with any template): do NOT call phase-start or spawn an agent. Proceed directly to the next phase block.

> **Skip gate 2 (--nopr):** If `{skip_pr}` is `true`: run the stage-commit step and the push step, but skip the gh-pr-create and capture-PR-number steps. Set `{pr-number}` to `none`. Print:
> "Skipping PR creation (--nopr flag). Branch pushed to origin."

```bash
$SM phase-start {workspace} pr-creation
```

Create a pull request for the feature branch:

1. **Stage and commit** any remaining uncommitted changes (e.g., workspace artifact files):
   ```bash
   git add -A
   git commit -m "chore: forge artifacts for {spec-name}"
   ```
   If there are no uncommitted changes, skip this step.

2. **Push the branch** (use the branch name from state — `feature/{spec-name}` or the existing branch if `{use_current_branch}` is `true`):
   ```bash
   git push -u origin {branch}
   ```

3. **Create the pull request** using `gh pr create`. Derive the title from `request.md` (short, under 70 chars). Build the body from `design.md` and `tasks.md`:
   ```bash
   gh pr create --title "<title>" --body "$(cat <<'EOF'
   ## Summary
   <2-3 bullet points from design.md>

   ## Changes
   <task list from tasks.md with completion status>

   ## Test plan
   <from design.md test strategy section>

   ---
   Source: <source_url from request.md, if applicable>
   Generated by [claude-forge](https://github.com/hiromaily/claude-forge/)
   EOF
   )"
   ```

4. **Capture the PR number** from the `gh pr create` output (it prints the PR URL). Extract the number and store as `{pr-number}`.

```bash
$SM phase-complete {workspace} pr-creation
```

---

## Final Summary

```bash
$SM phase-start {workspace} final-summary
```

**Dispatch on `{task_type}`** — select exactly one block below and follow only those steps:

---

### If `{task_type}` is `feature` or `refactor`

1. Read all `review-{N}.md` files and `comprehensive-review.md`.
2. Run `$SM phase-stats {workspace}` and capture its output.
3. Write `{workspace}/summary.md` with this structure:
   ```markdown
   # Pipeline Summary

   **Request:** <one-line description from request.md>
   **Feature branch:** `{branch}`
   **Pull Request:** #<pr-number> (<PR URL>)   ← omit this line if {pr-number} is `none`
   **Date:** {date}

   ## Tasks

   | # | Title | Verdict |
   |---|-------|---------|
   | 1 | … | PASS / PASS_WITH_NOTES / FAIL |
   …

   ## Comprehensive Review

   <Verdict (CLEAN/IMPROVED) and key findings from comprehensive-review.md>

   ## Notes

   <Any PASS_WITH_NOTES items or observations worth recording>

   ## Test Results

   <Final pass/fail counts from the verification step>

   ## Execution Stats

   | Phase | Tokens | Duration | Model |
   |-------|--------|----------|-------|
   | phase-1 | … | …s | sonnet |
   …
   | **TOTAL** | **…** | **…s** | |
   ```
4. Present the contents of `summary.md` to the user.
5. **Update the commit to include summary.md**:
   ```bash
   git add {workspace}/summary.md
   git commit --amend --no-edit
   git push --force-with-lease
   ```

---

### If `{task_type}` is `bugfix` or `docs`

Phase 7 (Comprehensive Review) was skipped for both `bugfix` and `docs`. Phase 4 (Task Decomposition) was also skipped (stub tasks.md used instead). Do NOT read `comprehensive-review.md` or build a Tasks table — neither exists.

1. Read all `review-{N}.md` files (Phase 6 ran for both `bugfix` and `docs`).
2. Run `$SM phase-stats {workspace}` and capture its output.
3. Write `{workspace}/summary.md` with this structure:
   ```markdown
   # Pipeline Summary

   **Request:** <one-line description from request.md>
   **Feature branch:** `{branch}`
   **Pull Request:** #<pr-number> (<PR URL>)   ← omit this line if {pr-number} is `none`
   **Date:** {date}

   ## Review Findings

   <Key findings from review-{N}.md files>

   ## Notes

   <Any observations worth recording>

   ## Execution Stats

   | Phase | Tokens | Duration | Model |
   |-------|--------|----------|-------|
   | phase-1 | … | …s | sonnet |
   …
   | **TOTAL** | **…** | **…s** | |
   ```
4. Present the contents of `summary.md` to the user.
5. **Update the commit to include summary.md**:
   ```bash
   git add {workspace}/summary.md
   git commit --amend --no-edit
   git push --force-with-lease
   ```

---

### If `{task_type}` is `investigation`

> **Terminal phase for investigation flow.** Phases 3 (Design), 3b, checkpoint-a, 4, 4b, checkpoint-b, 5, 6, 7, Final Verification, and PR Creation were all skipped. There is no `design.md`, no feature branch, no PR, and no commit to amend. After `final-summary` completes, `post-to-source` still runs so findings are posted back to the source issue.

1. Read `{workspace}/analysis.md` and `{workspace}/investigation.md`.
2. Run `$SM phase-stats {workspace}` and capture its output.
3. Write `{workspace}/summary.md` with this structure:
   ```markdown
   # Investigation Summary

   **Request:** <one-line description from request.md>
   **Date:** {date}

   ## Findings

   <Key findings and conclusions from investigation.md>

   ## Key Questions Answered

   <Distilled Q&A or hypothesis outcomes from investigation.md>

   ## Recommendations

   <Actionable next steps or follow-up tasks if any>

   ## Execution Stats

   | Phase | Tokens | Duration | Model |
   |-------|--------|----------|-------|
   | phase-1 | … | …s | sonnet |
   …
   | **TOTAL** | **…** | **…s** | |
   ```
4. Present the contents of `summary.md` to the user.
5. **Do NOT run commit-amend or push** — no feature branch exists for `investigation` flows.

---

### If `{task_type}` is none of the above

Stop immediately and report an error:

> Pipeline error: `{task_type}` is not a recognised task type. Expected one of: `feature`, `refactor`, `bugfix`, `docs`, `investigation`. The pipeline is in an unexpected state — do not proceed.

---

### Post-dispatch epilogue <!-- anchor: final-summary-epilogue -->

Runs for all task types after the per-type dispatch block above completes.

1. **Run debug-report** (reports on forge skill operation — pipeline metrics, anomalies, token outliers, revision cycles): if `{debug_mode}` is `true`, execute the debug-report block below; otherwise skip it.

2. **Run improvement-report** (reports on target-repository friction — documentation gaps, code readability, conventions — that would have helped complete the task): always execute the improvement-report block below.

---

### Debug Report (conditional — all task types) <!-- anchor: debug-report -->

_Reports on the **operation of the forge skill itself**: pipeline execution flow, phase metrics, token outliers, retry counts, and revision cycles. Triggered only when `{debug_mode}` is `true`._

If `{debug_mode}` is `false`, skip this section entirely and proceed to the improvement-report block.

If `{debug_mode}` is `true`:

1. Run `$SM resume-info {workspace}` and capture its JSON output as `{debug_data}`.
   (Note: `currentPhaseStatus` in `{debug_data}` will show `in_progress` rather than
   `completed` at this point — `phase-complete final-summary` has not yet been called.
   This is expected; the debug report does not read or display `currentPhaseStatus`.)

   Also reuse the `phase-stats` output already captured in the dispatch block above.

2. Evaluate the following heuristics against `{debug_data}`:

   **H1 — Token outlier phases**
   Compute median token count across all `phaseLog` entries in `{debug_data}`.
   Flag any entry where `tokens > 2 × median` as "high token phase."
   If `phaseLog` is empty or has fewer than 2 entries, skip H1.

   **H2 — Retry signal**
   Inspect `{debug_data}.tasksWithRetries` (list of tasks with non-zero retry counts, already
   projected by `resume-info`). If the list is non-empty, flag each entry as having retries.

   **H3 — Revision signal**
   If `{debug_data}.revisions.designRevisions > 1`: flag as "multiple design revision cycles."
   If `{debug_data}.revisions.taskRevisions > 1`: flag as "multiple task revision cycles."

   **H4 — Phase-log coverage**
   `phaseLogEntries` is the count from `{debug_data}`.
   Expected logged phases = count of `completedPhases` entries minus phases that never call
   `phase-log` (i.e., `checkpoint-a`, `checkpoint-b`, `pr-creation`, `post-to-source`,
   `final-summary`, `setup`).
   If `phaseLogEntries` < expected: flag as "missing phase-log entries."

3. Append a `## Debug Report` section to `{workspace}/summary.md` with this structure:

   ```markdown
   ## Debug Report

   _Generated by `--debug` flag. Data source: state.json metrics only._

   **Flow template:** {flow_template}
   **Total tokens:** {totalTokens}
   **Total duration:** {totalDuration_ms / 1000}s

   ### Anomalies Detected

   <For each triggered heuristic, one bullet per finding. If no heuristic fires, write:>
   - No anomalies detected.

   ### Phase Token Breakdown

   <Repeat the phase-stats table already in the Execution Stats section, with an additional
   "Flag" column: "HIGH" if H1 fired for that phase, blank otherwise.>

   ### Improvement Suggestions

   <For each triggered heuristic, one actionable suggestion:>
   - H1 (high token phase): "Consider splitting <phase> into smaller sub-tasks, or investigate
     what input files are causing large context for that phase."
   - H2 (retries): "Task <N> required <implRetries> impl retries and <reviewRetries> review
     retries. Review the task scope — it may be too broad for a single implementation unit."
   - H3 (revisions): "Design/task revision cycles detected. Review the review-design.md or
     review-tasks.md verdict rationale to understand recurring issues."
   - H4 (missing phase-log): "Some phases did not record metrics. Ensure `phase-log` calls are
     present in all phase execution blocks."
   ```

4. After appending the debug section, proceed to the improvement-report block.

---

### Improvement Report (all task types) <!-- anchor: improvement-report -->

_Reports on friction in the **target repository** — documentation gaps, code readability issues, or conventions — that would have helped complete the assigned task. Always runs._

Always execute this block for every task type and flow template.

**If `{flow_template}` is `direct`**, append to `{workspace}/summary.md`:

````markdown
## Improvement Report

_This run used the `direct` flow template (effort XS/S). No analysis or investigation
phases ran. Insufficient data for a meaningful retrospective._
````

**Otherwise**, determine which artifact files are available:

- `{workspace}/analysis.md` — present for all non-direct flows
- `{workspace}/investigation.md` — present **except** when:
  - `{task_type}` is `docs` (phase-2 was skipped), or
  - `{flow_template}` is `lite` AND `{task_type}` is `investigation` (output merged into analysis.md)

Read the available artifact files (skip any that are absent). Do not read target-repository source code or CLAUDE.md — this would violate the token economy rule.

Write the following section based on friction signals observed in those files. Friction signals include: information that had to be inferred rather than read directly, open questions that required significant investigation, conventions that were unclear, interfaces that were undocumented, or tooling that was missing.

Append to `{workspace}/summary.md`:

````markdown
## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

<Specific gaps in docs, README, CLAUDE.md, or inline documentation that caused friction.
If none, write "No friction observed.">

### Code Readability

<Aspects of codebase structure, naming, or organization that slowed analysis.
If none, write "No friction observed.">

### AI Agent Support (Skills / Rules)

<Missing skills, unclear conventions, or repeated patterns that could be expressed as
rules or automated. If none, write "No friction observed.">

### Other

<Any other improvement suggestions. Omit this sub-section entirely if empty — do not write a placeholder.>
````

For `lite` + `investigation` flows: after writing the section, add a note in the header italics line:
_Retrospective on what would have made this work easier. Note: this run used the `lite` flow template — analyst and investigator output was merged into `analysis.md`._

---

```bash
$SM phase-complete {workspace} final-summary
```

---

## Post to Source

After Final Summary, check `request.md` front matter for `source_type`:

```bash
$SM phase-start {workspace} post-to-source
```

| `source_type` | Action |
|--------------|--------|
| `github_issue` | Post summary as a comment: `gh issue comment <source_id> --repo <owner>/<repo> --body "$(cat {workspace}/summary.md)"` |
| `jira_issue` | Post summary as a comment using `mcp__atlassian__addCommentToJiraIssue` with `issueIdOrKey: <source_id>` and the summary content as the comment body. |
| `text` | Skip — no external posting. |

```bash
$SM phase-complete {workspace} post-to-source
```

---

## Token Economy Rules

- **Never read implementation files in the main agent context** — only subagents read code. The main agent reads only the small artifact files (`analysis.md`, `design.md`, etc.).
- **Truncate subagent prompts to essentials** — do not paste file contents into prompts; pass file paths and instruct subagents to read them.
- **One subagent per phase** — do not chain phases inside a single subagent invocation.
- **Dedicated agents have their own system prompts** — do not duplicate agent instructions in the orchestrator prompts. Pass only phase-specific context (workspace path, task number, etc.).

---

## Error Handling

| Situation | Action |
|-----------|--------|
| Subagent returns empty or incoherent output | Retry once with the same prompt; if it fails again, run `$SM phase-fail` and report to user |
| Design checkpoint rejected | Revise design with user feedback and re-present (max 2 revisions before asking user to clarify the request) |
| Task checkpoint rejected | Revise tasks and re-present |
| Implementation FAIL review | Re-implement with review as context (max 2 attempts per task); track with `$SM task-update` |
| Test suite fails after implementation | Run `$SM phase-fail` and present the failure to the user |
| Final verification finds new failures | Fix before summarizing — do not leave a broken branch |
| Residual imports of deleted code found in final verification | Spawn a fix agent to update all callers; re-run verification |
| Pipeline interrupted | On next invocation, pass workspace path as `$ARGUMENTS` to resume from `state.json` |
