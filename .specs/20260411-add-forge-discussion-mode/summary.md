# Summary: add-forge-discussion-mode

## What was built

A `--discuss` flag was added to the forge pipeline that enables a pre-pipeline clarification dialogue for text-source pipelines. When a user invokes the pipeline with `--discuss`, the `pipeline_init_with_context` tool now operates on a three-call protocol: the first call detects effort and returns a `needs_discussion` prompt containing three targeted clarifying questions; the second (discussion) call receives the user's answers and returns a `needs_user_confirmation` response with an `enriched_request_body` field; the third (confirmation) call carries the enriched body through `user_confirmation` into `initWorkspace`, which writes it to `request.md`. All downstream pipeline agents then receive the enriched task description. The discussion path is skipped automatically when `--auto` is set or when the source is GitHub/Jira.

As a collateral improvement, `buildRequestMD` was renamed to `buildRequestMDWithBody` and updated to include the original task text as the `request.md` body for all text-source pipelines, fixing a pre-existing gap where `request.md` contained only YAML front matter with no body. No new MCP tools were added, no new phases were introduced, and the `State` struct was not modified since the `Discuss` flag only affects workspace initialisation.

## PR

hiromaily/claude-forge#142

## Changes

- `mcp-server/internal/tools/pipeline_init_with_context.go` — Core implementation: three-call discriminator, `handleDiscussionCall`, `buildEnrichedRequestBody`, `buildRequestMDWithBody`, new structs and fields.
- `mcp-server/internal/tools/registry.go` — Added `task_text` and `discussion_answers` optional parameters to `pipeline_init_with_context` tool.
- `mcp-server/internal/tools/pipeline_init_with_context_test.go` — 11 new test cases covering all discussion-mode paths.
- `mcp-server/internal/tools/pipeline_integration_test.go` — End-to-end 4-call integration test with 5 content assertions on `request.md`.
- `skills/forge/SKILL.md` — Updated Step 1b for `task_text`, added `needs_discussion` handling block, added `--discuss` flag documentation.
- `mcp-server/internal/validation/input.go` — `--discuss` flag regex, parsing, stripping.
- `mcp-server/internal/tools/pipeline_init.go` — `Discuss bool` on `PipelineInitFlags`, `CoreText` on `PipelineInitResult`.

## Pipeline Statistics

- Total tokens: 935,328
- Total duration: 2,113,419 ms
- Estimated cost: $5.61
- Phases executed: 13
- Phases skipped: 0
- Retries: 0
- Review findings: 0 critical, 6 minor

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The `request.md` had only YAML front matter — the spec name was the only signal. This forced the analysis and investigation phases to infer scope from the BACKLOG and codebase patterns. A one-sentence description would have immediately narrowed the four candidate interpretations to one. The `pipeline_init_with_context.go` file also has no file-level doc comment describing the existing two-call protocol.

### Code Readability

The two-call discriminator was compact but opaque — the three-call extension required a careful read before the correct insertion point was clear. A named dispatch function or explicit comment block above the discriminator would make the routing logic easier to extend in future. The `pipelineFlags` and `PipelineInitFlags` parallel-type mirroring is intentional but undocumented.

### AI Agent Support (Skills / Rules)

A `validate_artifact` rule that cross-references the design checklist against `git diff --name-only` could automate detection of divergence between design and implementation. The design checklist in Section 6 of `design.md` was useful but not machine-checked.

### Other

The `analytics_pipeline_summary` tool returns lower statistics mid-run (at comprehensive-review time) vs. end-of-run — this is the documented post-PR #126 known issue. The convention to always use end-of-run values is correctly documented in the verifier agent instructions.
