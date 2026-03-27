# claude-forge MCP Server — Advanced Orchestration Design

## 1. Background

### Problem

The current claude-forge architecture works as follows:

- **SKILL.md** (~1,600 lines): LLM-interpreted orchestration instructions containing 26 branching decisions
- **state-manager.sh** (26 commands): jq-based state management CLI
- **Hook scripts** (5 scripts): Deterministic guards for PreToolUse / PostToolUse / Stop

This architecture has two core problems:

1. **Non-determinism**: The LLM interprets 1,600 lines of SKILL.md, causing deviation at phase transitions, skip gates, and retry decisions.
2. **Shallow moat**: The skill, agents, and shell scripts are all text files — copying them produces an equivalent product.

### Goal

Build a Go MCP Server that:

- Moves orchestration logic into program code to **maximize determinism**
- Creates **hard-to-replicate differentiation** via data accumulation and learning
- Compresses SKILL.md to ~50 lines, minimizing the LLM's interpretation surface

### Constraints

- **No API key**: Must run on Claude subscription plan. No external LLM API calls.
- **Hooks remain shell**: Claude Code's hook system executes `type: "command"` shell scripts. Cannot be replaced by MCP tools.
- **macOS compatible**: No `flock`. Must remain compatible with the existing mkdir-based locking.

---

## 2. Architecture Overview

```text
Before:
  SKILL.md (1,600 lines) → LLM interprets → decides next phase → spawns Agent
                             ↑ non-deterministic

After:
  Go MCP Server (deterministic) → next_action → LLM just executes → report_result
  Hook scripts (thin shell) → read state.json → last line of defense
```

### Data Flow

```text
┌─────────────────────────────────────────────────────┐
│ Claude Code Session (subscription)                  │
│                                                     │
│  ┌──────────────┐     ┌─────────────────────────┐   │
│  │ SKILL.md     │     │ forge-state MCP Server  │   │
│  │ (~50 lines)  │◄───►│                         │   │
│  │              │ MCP │ ┌─────────────────────┐ │   │
│  │ 1. next()    │     │ │ Orchestration Engine│ │   │
│  │ 2. execute   │     │ │ (state machine)     │ │   │
│  │ 3. report()  │     │ └─────────────────────┘ │   │
│  │ 4. goto 1    │     │ ┌─────────────────────┐ │   │
│  └──────────────┘     │ │ State Manager       │ │   │
│                       │ │ (in-memory + file)  │─┼───┼──► state.json
│  ┌──────────────┐     │ └─────────────────────┘ │   │     ▲
│  │ Agent tools  │     │ ┌─────────────────────┐ │   │     │ read
│  │ (11 agents)  │     │ │ History Index       │ │   │  ┌──┴──────────┐
│  └──────────────┘     │ │ (TF-IDF, patterns)  │ │   │  │ Hook scripts │
│                       │ └─────────────────────┘ │   │  │ (thin shell) │
│  ┌──────────────┐     │ ┌─────────────────────┐ │   │  └─────────────┘
│  │ Bash/Edit/   │     │ │ Repo Profile Cache  │ │   │
│  │ Write tools  │     │ └─────────────────────┘ │   │
│  └──────────────┘     │ ┌─────────────────────┐ │   │
│                       │ │ Analytics Engine    │ │   │
│                       │ └─────────────────────┘ │   │
│                       └─────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### Dual-Write Strategy

The MCP Server treats in-memory state as the source of truth and syncs to `state.json`:

- **MCP Server**: In-memory mutex for fast exclusive control. All logic lives here.
- **state.json**: Persistent cache read by hook scripts. Restoration source on session restart.
- **Hook scripts**: Read `state.json` via jq and return exit 0 (allow) or exit 2 (block).

---

## 3. Capability Summary

### Public Tools (called directly by LLM — 17 tools)

| # | Tool Name | Category | Description |
| --- | --- | --- | --- |
| 1 | `pipeline_init` | Orchestration | Parse flags and determine `source_type`. Returns `fetch_needed` if external data is required. |
| 2 | `pipeline_init_with_context` | Orchestration | Receive external context (Jira/GitHub), finalize `task_type`/`effort`/`flow_template`/skip sequence. |
| 3 | `pipeline_next_action` | Orchestration | State machine transition. Returns next action (5 types: `spawn_agent`/`checkpoint`/`exec`/`write_file`/`done`). |
| 4 | `pipeline_report_result` | Orchestration | Process agent results: log phase metrics, parse verdict, validate artifacts, transition state, accumulate patterns. |
| 5 | `state_get` | State | Read any field (for debugging). |
| 6 | `state_phase_stats` | State | Execution statistics table (for Final Summary). |
| 7 | `state_resume_info` | State | Retrieve resume information. |
| 8 | `state_abandon` | State | Abort the pipeline. |
| 9 | `history_search` | Data Flywheel | TF-IDF/BM25 search over similar past pipelines. |
| 10 | `history_get_patterns` | Data Flywheel | Frequently occurring review finding patterns in this repo. |
| 11 | `history_get_friction_map` | Data Flywheel | AI friction map accumulated from past improvement reports. |
| 12 | `profile_get` | Repo Profile | Repository profile (languages, test framework, CI, linter, directory conventions, etc.). |
| 13 | `analytics_pipeline_summary` | Analytics | Current pipeline cost/time/quality summary. |
| 14 | `analytics_repo_dashboard` | Analytics | Cumulative statistics for the entire repository. |
| 15 | `analytics_estimate` | Analytics | Cost/time prediction from historical data. |
| 16 | `validate_input` | Validation | Input validation (replaces `validate-input.sh`). |
| 17 | `validate_artifact` | Validation | Artifact structure validation (verdict presence, required sections). |

### Internal Functions (not exposed as tools — 18 functions)

| # | Function | Package | Description |
| --- | --- | --- | --- |
| 18 | State machine engine | `orchestrator/engine.go` | Implements all 26 branching decisions in Go code. Replaces SKILL.md 1,600 lines. |
| 19 | Phase definitions and transitions | `orchestrator/phases.go` | 17 phase ordering, skip gates, next-phase calculation. |
| 20 | Flow template table | `orchestrator/flow_templates.go` | 5 templates: skip sequences, stub synthesis rules. |
| 21 | task_type/effort detection | `orchestrator/detection.go` | Flag → Jira → GitHub → heuristic → default priority cascade. |
| 22 | Verdict parsing | `orchestrator/verdict.go` | Extract `APPROVE`/`REVISE`/`FAIL` + `CRITICAL`/`MINOR` from `review-*.md`. |
| 23 | Prompt construction | `prompt/builder.go` | 4-layer dynamic prompt generation (agent `.md` + artifacts + profile + history). |
| 24 | In-memory state management | `state/manager.go` | Mutex exclusion + `state.json` dual-write. Equivalent to 26 shell commands. |
| 25 | Schema migration | `state/migration.go` | Automatic `state.json` v1 → v2 conversion. |
| 26 | TF-IDF index builder | `history/index.go` | Scan `.specs/`, extract keywords from `request.md`, persist index. |
| 27 | Pattern accumulation | `history/patterns.go` | Categorize review findings, group by Levenshtein distance. |
| 28 | Friction map accumulation | `history/friction.go` | Extract and classify friction points from improvement reports. |
| 29 | Repository analysis | `profile/analyzer.go` | Build profile from `git ls-files`, `go.mod`, `package.json`, CI configs, etc. |
| 30 | Profile cache | `profile/cache.go` | Persist to `.specs/repo-profile.json`, 7-day cache. |
| 31 | Metrics collection | `analytics/collector.go` | Aggregate from `phaseLog`. |
| 32 | Cost/time prediction | `analytics/estimator.go` | P50/P90 prediction per `(task_type, effort)` pair. |
| 33 | Crash recovery | Server startup | Scan `.specs/` → restore in-memory state from `state.json` of active pipeline. |
| 34 | Multi-pipeline management | `PipelineRegistry` | Map of workspace → StateManager. |
| 35 | MCP JSON-RPC server | `main.go` (existing) | MCP protocol over stdio transport via `github.com/mark3labs/mcp-go`. Already implemented; no new code required. |

### Persistent Data (managed by MCP Server — 5 files)

| # | File | Purpose | When Updated |
| --- | --- | --- | --- |
| 36 | `.specs/*/state.json` | Pipeline state | Every state transition (dual-write) |
| 37 | `.specs/index.json` | TF-IDF history index | Differential update on startup + pipeline completion |
| 38 | `.specs/patterns.json` | Review finding pattern DB | On pipeline completion |
| 39 | `.specs/friction.json` | AI friction map | On pipeline completion |
| 40 | `.specs/repo-profile.json` | Repository profile | On first analysis + after 7 days |

### Functions Remaining in Shell (not in MCP Server — 5 scripts)

| # | Script | Reason |
| --- | --- | --- |
| 41 | `pre-tool-hook.sh` | Hook system requires shell. Rule 1 (read-only), Rule 2 (parallel commit block), Rule 5 (main checkout block). |
| 42 | `post-agent-hook.sh` | Same. Advisory only (exit 0). |
| 43 | `stop-hook.sh` | Same. Blocks premature stop on active pipeline. |
| 44 | `common.sh` | `find_active_workspace` shared function. |
| 45 | `test-hooks.sh` | Test suite. |

**Total: 17 public tools + 18 internal functions + 5 persistent files + 5 shell scripts = 45**

---

## 4. MCP Tools Design (Detailed)

### 4.1 Orchestration Engine (replaces SKILL.md 1,600 lines)

All 26 branching decisions currently interpreted by the LLM are moved into Go code.

#### `pipeline_init` (Stage 1: Flag parsing + source_type determination)

First stage of pipeline initialization. Executes flag parsing and `source_type` determination.
If external data (Jira issue type, GitHub labels, story points) is needed, returns `fetch_needed` to ask the LLM to retrieve it.

> **Design decision**: The MCP Server cannot directly call other MCP tools (Jira MCP, `gh` CLI, etc.).
> Fetching external data is the LLM's responsibility, so `init` is split into two stages.

**Parameters:**

```json
{
  "arguments": "string — raw $ARGUMENTS from skill invocation",
  "current_branch": "string — output of git branch --show-current"
}
```

**Returns:**

```json
{
  "workspace": ".specs/20260327-fix-auth-timeout",
  "spec_name": "fix-auth-timeout",
  "source_type": "github_issue",
  "source_url": "https://github.com/hiromaily/claude-forge/issues/54",
  "source_id": "54",
  "flags": {
    "auto": false,
    "skip_pr": false,
    "debug": false,
    "type_override": null,
    "effort_override": null
  },
  "fetch_needed": {
    "type": "github",
    "fields": ["labels", "title", "body"],
    "instruction": "Fetch GitHub issue #54 via gh and call pipeline_init_with_context"
  },
  "errors": null
}
```

When `source_type` is `text`, `fetch_needed` is null and `init_with_context` can be called directly.

#### `pipeline_init_with_context` (Stage 2: Finalize task_type/effort)

Receives external data fetched by the LLM, then executes: task_type/effort detection → flow_template determination → skip sequence calculation.

**Parameters:**

```json
{
  "workspace": ".specs/20260327-fix-auth-timeout",
  "external_context": {
    "github_labels": ["bug", "priority:high"],
    "github_title": "Fix auth timeout on session renewal",
    "github_body": "..."
  },
  "user_confirmation": {
    "task_type": "bugfix",
    "effort": "S"
  }
}
```

When `user_confirmation` is null, returns heuristic detection results and asks for confirmation. Called again after user confirmation.

**Returns:**

```json
{
  "workspace": ".specs/20260327-fix-auth-timeout",
  "spec_name": "fix-auth-timeout",
  "task_type": "bugfix",
  "effort": "S",
  "flow_template": "lite",
  "auto_approve": false,
  "skip_pr": false,
  "debug": false,
  "use_current_branch": false,
  "skipped_phases": ["phase-3b", "checkpoint-a", "phase-4", "phase-4b", "checkpoint-b", "phase-7"],
  "source_type": "github_issue",
  "needs_user_confirmation": {
    "type": "task_type_and_effort",
    "detected_task_type": "bugfix",
    "detected_effort": "S",
    "is_heuristic": true,
    "message": "Detected: bugfix (S). Correct? [Y/n]"
  },
  "request_md_content": "---\nsource_type: github_issue\nsource_id: 54\ntask_type: bugfix\n---\n...",
  "ready": true,
  "errors": null
}
```

**Decisions made deterministic (init + init_with_context):**

| # | Decision | Current (SKILL.md) | After migration (Go) | Stage |
| --- | --- | --- | --- | --- |
| 1 | Resume detection | LLM interprets `$ARGUMENTS` | `strings.Contains(args, ".specs/")` | init |
| 2 | `--type=` flag parsing | LLM extracts from text | `regexp.MustCompile` | init |
| 3 | `--effort=` flag parsing | Same | Same | init |
| 4 | `--auto` / `--nopr` / `--debug` | Same | Same | init |
| 5 | `source_type` determination | LLM URL pattern match | `url.Parse` + regexp | init |
| 6 | Jira issue type → task_type | LLM maps it | `switch issueType` | init_with_context |
| 7 | GitHub labels → task_type | LLM keyword match | `labelToTaskType map` | init_with_context |
| 8 | Text heuristic | LLM guesses | Go keyword scoring (LLM fallback available) | init_with_context |
| 9 | Effort detection cascade | LLM tries 4 stages sequentially | `detectEffort()` chain | init_with_context |
| 10 | flow_template derivation | LLM reads 2D table | `flowTemplateMatrix[taskType][effort]` | init_with_context |
| 11 | skip sequence calculation | LLM reads 20-cell table | `skipTable[flowTemplate]` | init_with_context |
| 12 | full + auto conflict check | LLM interprets rules | `if template == "full" && autoApprove` | init_with_context |
| 13 | Stub synthesis determination | LLM interprets per-template conditions | `shouldSynthesizeStubs()` | init_with_context |

#### `pipeline_next_action`

State machine transition. Returns the next action to execute based on current phase and state.

**Parameters:**

```json
{
  "workspace": ".specs/20260327-fix-auth-timeout",
  "user_response": "string | null — human response at checkpoints"
}
```

**Returns (Action type, 5 variants):**

```json
// Type 1: Spawn an agent
{
  "type": "spawn_agent",
  "agent": "architect",
  "prompt": "...(history/profile/pattern injected)...",
  "model": "sonnet",
  "phase": "phase-3",
  "input_files": ["request.md", "analysis.md", "investigation.md"],
  "output_file": "design.md"
}

// Type 2: Human checkpoint
{
  "type": "checkpoint",
  "name": "checkpoint-a",
  "present_to_user": "## Design Review\n\nApproach: ...\nAI Verdict: APPROVE_WITH_NOTES\n...",
  "options": ["approve", "revise"]
}

// Type 3: Execute commands
{
  "type": "exec",
  "commands": [
    "git checkout -b feature/fix-auth-timeout",
    "git add -A",
    "git commit -m 'feat: ...'"
  ],
  "phase": "pr-creation"
}

// Type 4: Write a file
{
  "type": "write_file",
  "path": ".specs/20260327-fix-auth-timeout/summary.md",
  "content": "...",
  "phase": "final-summary"
}

// Type 5: Pipeline complete
{
  "type": "done",
  "summary": "Pipeline completed. PR #123 created.",
  "summary_path": ".specs/20260327-fix-auth-timeout/summary.md"
}
```

**Decisions made deterministic:**

| # | Decision | Detail |
| --- | --- | --- |
| 14 | Phase skip gate | `contains(skippedPhases, phase)` |
| 15 | lite template → analyst agent | `if flowTemplate == "lite" && phase == "phase-1"` |
| 16 | docs M/L stub synthesis timing | `if taskType == "docs" && phase == "phase-1" && completed` |
| 17 | bugfix stub synthesis timing | `if taskType == "bugfix" && phase == "phase-3" && completed` |
| 18 | Design review verdict → branch | `parseVerdict(review-design.md)` → REVISE/APPROVE |
| 19 | Task review verdict → branch | Same |
| 20 | Auto-approve gate | `if autoApprove && verdict in (APPROVE, APPROVE_WITH_NOTES)` |
| 21 | Retry limit check | `if revisionCount >= 2 → escalate to human` |
| 22 | Phase 5 sequential/parallel determination | `parseExecutionMode(tasks.md)` |
| 23 | Phase 6 PASS/FAIL → retry or next task | `if verdict == "FAIL" && retries < 2` |
| 24 | PR creation skip determination | `if skipPr \|\| phase in skippedPhases` |
| 25 | Final Summary template selection | `switch taskType` |
| 26 | Post-to-source dispatch | `switch sourceType` |

#### `pipeline_report_result`

Processes agent execution results and updates state.

**Parameters:**

```json
{
  "workspace": ".specs/20260327-fix-auth-timeout",
  "phase": "phase-3b",
  "agent_output": "string — agent's returned text",
  "tokens_used": 5000,
  "duration_ms": 30000,
  "model": "sonnet"
}
```

**Returns:**

```json
{
  "state_updated": true,
  "artifact_written": "review-design.md",
  "verdict_parsed": "APPROVE_WITH_NOTES",
  "findings": [
    {"severity": "MINOR", "description": "..."}
  ],
  "next_action_hint": "proceed to checkpoint-a"
}
```

**Internal processing:**

1. Record `phase-log` (tokens, duration, model)
2. Parse verdict from artifact file
3. **Artifact existence validation** — migrate current `pre-tool-hook.sh` Rule 3a–3i into MCP Server. Validate artifact file existence and structure during `report_result`; return error if invalid. Remove from hook side.
4. State transition (`phase-complete` or retry determination)
5. Accumulate review findings into patterns DB (history feature)
6. Input validation guard — replace current marker-file approach. Manage `validated: true` in MCP Server in-memory state.

#### Prompt construction (internal function)

> **Design decision**: `build_prompt` is not exposed as a standalone tool. When `next_action` returns `spawn_agent`,
> it internally calls `buildPrompt()` and includes the completed prompt in the `prompt` field.
> This keeps the SKILL.md loop to exactly three steps: `next_action → execute → report_result`.

Prompts are built in 4 layers:

```text
[Layer 1: Agent base instructions (from agents/*.md)]

[Layer 2: Input artifacts (request.md, analysis.md, etc.)]

[Layer 3: Repository Context (from profile)]
- Language: Go (65%), TypeScript (30%)
- Test framework: go test
- CI: GitHub Actions
- Linter: golangci-lint

[Layer 4: Data Flywheel Context (from history)]

## Past Reference (top 3 similar runs)
- 20260320-fix-auth-middleware: Similar auth-related bugfix. Design chose X approach.
- 20260315-add-retry-logic: retry pattern in this repo uses backoff.Retry().

## Common Review Findings in This Repository
- CRITICAL: "Missing error handling in gRPC interceptors" (seen 3 times)
- MINOR: "Import grouping does not follow convention" (seen 5 times)

## Known AI Friction Points
- Implementer tends to write to wrong directory for test fixtures
- Reviewer misses linter-specific nolint directives
```

Layers 3–4 are omitted on first run when no data has been accumulated.

---

### 4.2 State Management (replaces state-manager.sh)

Most of the 26 commands run automatically inside `next_action` / `report_result`. The LLM only needs to call these directly:

| Tool | Purpose | When LLM calls directly |
| --- | --- | --- |
| `state_get` | Read any field | Debugging, displaying state to user |
| `state_phase_stats` | Execution statistics table | Final Summary creation |
| `state_resume_info` | Resume information | Session resume |
| `state_abandon` | Abort pipeline | User requests abort |

**Implementation:**

```go
type StateManager struct {
    mu       sync.RWMutex
    state    *PipelineState
    filePath string
}

// In-memory read (also syncs to state.json for hooks)
func (sm *StateManager) Get(field string) (interface{}, error)

// Atomic update (mutex + file write)
func (sm *StateManager) Update(fn func(*PipelineState)) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    fn(sm.state)
    sm.state.Timestamps.LastUpdated = time.Now().UTC()
    return sm.persistToFile()
}

// On session start: load state.json into memory
func (sm *StateManager) LoadFromFile() error

type PipelineState struct {
    Version          int                    `json:"version"`
    SpecName         string                 `json:"specName"`
    Workspace        string                 `json:"workspace"`
    Branch           *string                `json:"branch"`
    TaskType         *string                `json:"taskType"`
    Effort           *string                `json:"effort"`
    FlowTemplate     *string                `json:"flowTemplate"`
    AutoApprove      bool                   `json:"autoApprove"`
    SkipPr           bool                   `json:"skipPr"`
    UseCurrentBranch bool                   `json:"useCurrentBranch"`
    Debug            bool                   `json:"debug"`
    SkippedPhases    []string               `json:"skippedPhases"`
    CurrentPhase     string                 `json:"currentPhase"`
    CurrentStatus    PhaseStatus            `json:"currentPhaseStatus"`
    CompletedPhases  []string               `json:"completedPhases"`
    Revisions        Revisions              `json:"revisions"`
    Tasks            map[string]*TaskState  `json:"tasks"`
    PhaseLog         []PhaseLogEntry        `json:"phaseLog"`
    Timestamps       Timestamps             `json:"timestamps"`
    Error            *ErrorInfo             `json:"error"`
}
```

---

### 4.3 Data Flywheel (new feature — competitive moat core)

Learns from accumulated `.specs/` execution history. Improves with every pipeline run.

#### `history_search`

Search for past pipelines similar to the current request.

**Parameters:**

```json
{
  "query": "Fix null pointer crash in auth middleware",
  "limit": 3,
  "task_type_filter": "bugfix"
}
```

**Returns:**

```json
{
  "results": [
    {
      "spec_name": "20260320-fix-auth-timeout",
      "similarity": 0.82,
      "task_type": "bugfix",
      "effort": "S",
      "flow_template": "lite",
      "one_liner": "Fixed auth middleware timeout by adding context deadline propagation",
      "design_excerpt": "Approach: propagate context.WithTimeout from gRPC interceptor...",
      "outcome": "completed",
      "tokens_total": 15000,
      "duration_total_ms": 120000
    }
  ],
  "index_size": 47
}
```

**Implementation (no API key required):**

```go
// Local TF-IDF based similarity search
type HistoryIndex struct {
    entries  []IndexEntry
    tfidf    *TFIDFIndex    // standard TF-IDF, computed locally
    loaded   time.Time
}

type IndexEntry struct {
    SpecName     string
    OneLiner     string   // first non-empty line after YAML front matter in request.md (no LLM needed)
    TaskType     string
    Effort       string
    FlowTemplate string
    Tags         []string // extracted keywords
    Outcome      string   // completed, abandoned
    TokensTotal  int
    DurationMs   int
    CreatedAt    time.Time
}

// On SessionStart: scan .specs/ and build index
func (h *HistoryIndex) Build(specsDir string) error {
    entries, _ := filepath.Glob(filepath.Join(specsDir, "*/state.json"))
    for _, e := range entries {
        idx := h.parseEntry(e)
        h.entries = append(h.entries, idx)
    }
    h.tfidf = NewTFIDF(h.entries)
    return nil
}
```

#### `history_get_patterns`

Returns frequently occurring review finding patterns in this repository.

**Parameters:**

```json
{
  "agent_filter": "impl-reviewer",
  "severity_filter": "CRITICAL",
  "limit": 10
}
```

**Returns:**

```json
{
  "patterns": [
    {
      "pattern": "Missing error handling in gRPC interceptors",
      "severity": "CRITICAL",
      "frequency": 3,
      "first_seen": "20260310-add-auth",
      "last_seen": "20260325-fix-status",
      "agent": "impl-reviewer"
    },
    {
      "pattern": "Import grouping does not follow project convention",
      "severity": "MINOR",
      "frequency": 5,
      "agent": "comprehensive-reviewer"
    }
  ],
  "total_reviews_analyzed": 47
}
```

**Implementation:**

```go
// Extract and accumulate findings from review-*.md and comprehensive-review.md
type PatternAccumulator struct {
    patterns map[string]*FindingPattern
    mu       sync.RWMutex
}

type FindingPattern struct {
    Pattern    string
    Severity   string   // CRITICAL, MINOR
    Frequency  int
    Agent      string
    FirstSeen  string
    LastSeen   string
    Examples   []string // up to 3 concrete examples
}

// Automatically accumulate on pipeline completion
func (pa *PatternAccumulator) IngestReview(reviewFile string) {
    findings := parseFindings(reviewFile)
    for _, f := range findings {
        key := normalizePattern(f.Description)
        if existing, ok := pa.patterns[key]; ok {
            existing.Frequency++
            existing.LastSeen = f.SpecName
        } else {
            pa.patterns[key] = &FindingPattern{...}
        }
    }
}

// Pattern normalization: local processing, no API key
// 1. lowercase + stopword removal + stemming (Porter stemmer)
// 2. Classify into fixed categories (error_handling, import_order, test_coverage,
//    naming_convention, type_safety, security, performance, etc.)
// 3. Within same category, treat as same pattern if Levenshtein distance < 0.3
func normalizePattern(desc string) string { ... }
```

#### `history_get_friction_map`

AI friction map accumulated from past improvement reports.

**Returns:**

```json
{
  "friction_points": [
    {
      "category": "file_targeting",
      "description": "Implementer writes test fixtures to wrong directory",
      "frequency": 2,
      "mitigation": "Specify testdata output directory explicitly in prompt"
    },
    {
      "category": "linter_rules",
      "description": "Reviewer misses golangci-lint nolint directives",
      "frequency": 3,
      "mitigation": "Include .golangci.yml path in reviewer context"
    }
  ],
  "total_reports_analyzed": 12
}
```

---

### 4.4 Repository Profile (new feature — per-repo learning)

#### `profile_get`

Returns cached repository profile. Analyzes automatically if not yet cached.

**Returns:**

```json
{
  "languages": [
    {"name": "Go", "percentage": 85},
    {"name": "Shell", "percentage": 10},
    {"name": "Markdown", "percentage": 5}
  ],
  "test_framework": "go test",
  "ci_system": "GitHub Actions",
  "linter_configs": ["golangci-lint"],
  "dir_conventions": {
    "agents/": "agent definition files",
    "scripts/": "shell scripts",
    "mcp-server/": "Go MCP server",
    "skills/": "Claude Code skill definitions"
  },
  "branch_naming": "feature/{name}",
  "build_command": "make build",
  "test_command": "make test",
  "monorepo": false,
  "last_updated": "2026-03-27T10:00:00Z",
  "staleness": "fresh"
}
```

**Implementation:**

```go
type RepoProfiler struct {
    cachePath string  // .specs/repo-profile.json
    profile   *RepoProfile
}

func (p *RepoProfiler) AnalyzeOrUpdate() (*RepoProfile, error) {
    cached := p.loadCache()
    if cached != nil && time.Since(cached.LastUpdated) < 7*24*time.Hour {
        return cached, nil
    }

    profile := &RepoProfile{}

    // 1. Language detection: git ls-files + extension counting
    profile.Languages = detectLanguages()

    // 2. Test framework: go.mod (testing), package.json (jest/vitest)
    profile.TestFramework = detectTestFramework()

    // 3. CI: .github/workflows/, .circleci/, Jenkinsfile
    profile.CISystem = detectCI()

    // 4. Linter: .golangci.yml, .eslintrc, pyproject.toml
    profile.LinterConfigs = detectLinters()

    // 5. Directory conventions: top-level directory Go convention matching
    profile.DirConventions = detectConventions()

    // 6. Branch naming: extract patterns from git branch -r
    profile.BranchNaming = detectBranchNaming()

    // 7. Build/test commands: Makefile, package.json scripts
    profile.BuildCommand, profile.TestCommand = detectCommands()

    p.saveCache(profile)
    return profile, nil
}
```

---

### 4.5 Analytics Engine (new feature — ROI demonstration)

#### `analytics_pipeline_summary`

Cost and time summary for the current pipeline.

**Returns:**

```json
{
  "pipeline": "20260327-fix-auth-timeout",
  "task_type": "bugfix",
  "effort": "S",
  "flow_template": "lite",
  "total_tokens": 25000,
  "total_duration_ms": 180000,
  "estimated_cost_usd": 0.15,
  "phases_executed": 6,
  "phases_skipped": 8,
  "retries": 0,
  "review_findings": {"critical": 0, "minor": 2}
}
```

#### `analytics_repo_dashboard`

Cumulative statistics for the entire repository.

**Returns:**

```json
{
  "total_pipelines": 47,
  "completed": 42,
  "abandoned": 5,
  "by_task_type": {
    "feature": {"count": 20, "avg_tokens": 45000, "avg_duration_min": 12},
    "bugfix": {"count": 15, "avg_tokens": 18000, "avg_duration_min": 5}
  },
  "by_flow_template": {
    "standard": {"count": 18, "avg_tokens": 50000},
    "lite": {"count": 12, "avg_tokens": 15000}
  },
  "total_tokens": 1500000,
  "estimated_total_cost_usd": 9.50,
  "review_pass_rate": 0.85,
  "avg_retries_per_pipeline": 0.3,
  "most_common_findings": [
    {"pattern": "Missing error handling", "count": 12}
  ]
}
```

#### `analytics_estimate`

Predict cost and time for a new task from historical data.

**Parameters:**

```json
{
  "task_type": "feature",
  "effort": "M"
}
```

**Returns:**

```json
{
  "sample_size": 8,
  "tokens": {"p50": 45000, "p90": 72000},
  "duration_min": {"p50": 12, "p90": 22},
  "cost_usd": {"p50": 0.28, "p90": 0.45},
  "confidence": "medium",
  "note": "Based on 8 previous feature/M pipelines in this repository"
}
```

---

### 4.6 Validation (hook support)

#### `validate_input`

Input validation. Replaces `validate-input.sh`.

**Parameters:**

```json
{
  "arguments": "string — raw $ARGUMENTS"
}
```

**Returns:**

```json
{
  "valid": true,
  "errors": [],
  "parsed": {
    "flags": {"type": "bugfix", "effort": "S", "auto": true},
    "core_text": "Fix null pointer crash in auth middleware",
    "source_type": "text"
  }
}
```

#### `validate_artifact`

Artifact structure validation. Validates verdict presence and required section existence.

**Parameters:**

```json
{
  "workspace": ".specs/20260327-fix-auth",
  "phase": "phase-3b"
}
```

**Returns:**

```json
{
  "valid": true,
  "file": "review-design.md",
  "verdict_found": "APPROVE_WITH_NOTES",
  "findings_count": {"CRITICAL": 0, "MINOR": 2},
  "missing_sections": []
}
```

---

## 5. SKILL.md After Migration

Compressed from ~1,600 lines to ~50 lines:

```markdown
# claude-forge Orchestrator

## Execution Loop

You are a pipeline orchestrator. Follow this loop exactly.

### Step 1: Initialize or Resume

If $ARGUMENTS contains an existing workspace path (.specs/ or state.json):
  - Call `pipeline_init` with arguments to check resume
  - If resume: call `state_resume_info` and continue from Step 2

Otherwise:
  1. Call `pipeline_init` with $ARGUMENTS and current branch
  2. If `fetch_needed` is set:
     - Fetch the requested external data (GitHub issue via gh, Jira via MCP)
     - Call `pipeline_init_with_context` with the fetched data
  3. If `fetch_needed` is null (text input):
     - Call `pipeline_init_with_context` directly
  4. If `needs_user_confirmation` is set:
     - Present the message to the user, wait for response
     - Call `pipeline_init_with_context` again with their confirmation

### Step 2: Main Loop

Repeat until done:

1. Call `pipeline_next_action(workspace)`
2. Execute the returned action:
   - `spawn_agent`: Call the Agent tool with the given agent name, prompt, and model
   - `checkpoint`: Present the summary to the user. Wait for their response.
     Pass their response as `user_response` in the next `pipeline_next_action` call.
   - `exec`: Execute each command via the Bash tool
   - `write_file`: Write the content via the Write tool
   - `done`: Present the summary to the user. Stop.
3. Call `pipeline_report_result(workspace, phase, output, tokens, duration, model)`
4. Go to step 1

### Rules

- NEVER skip calling `report_result` after an agent completes
- NEVER make orchestration decisions yourself — always defer to `next_action`
- If `next_action` or `report_result` returns an error, present it to the user and stop
- At checkpoints, you MUST wait for the user's response before continuing
```

---

## 6. Hook Scripts After Migration

Hooks are specialized to "last line of defense" only. Logic is minimal.

**Migration policy:**

- **Rule 3a–3i (artifact guard)**: Migrate into `report_result` inside MCP Server. Rationale: since MCP Server has full control of orchestration, artifact validation before phase completion belongs there.
- **Rule 6 (input validation guard)**: Retire marker-file approach. Manage `validated: true` in MCP Server in-memory state.
- **Rule 1, 2, 5 (read-only, parallel commit block, main checkout block)**: Keep in hooks. Required as last line of defense if LLM bypasses `next_action` and calls tools directly.
- **`find_active_workspace`**: Extract as shared shell function into `scripts/common.sh`, sourced by each hook.

### pre-tool-hook.sh (simplified)

```bash
#!/bin/bash
# MCP Server controls orchestration. Hook enforces critical rules only.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/common.sh"  # provides find_active_workspace()

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty') || exit 0
command -v jq &>/dev/null || exit 0

WS=$(find_active_workspace) || exit 0
STATE="$WS/state.json"
[ -f "$STATE" ] || exit 0

PHASE=$(jq -r '.currentPhase' "$STATE") || exit 0
STATUS=$(jq -r '.currentPhaseStatus' "$STATE") || exit 0

# Rule 1: Block source edits during Phase 1-2
if [[ "$PHASE" =~ ^phase-[12]$ ]] && [ "$STATUS" = "in_progress" ]; then
  # Block Edit/Write touching files outside workspace
fi

# Rule 2: Block git commit during parallel Phase 5
if [ "$PHASE" = "phase-5" ] && [ "$STATUS" = "in_progress" ]; then
  # Block git commit if parallel task is in_progress
fi

# Rule 5: Block main/master checkout
if [ "$TOOL" = "Bash" ]; then
  CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
  if [[ "$CMD" =~ git\ (checkout|switch)\ .*(main|master) ]]; then
    echo "Blocked: cannot switch to main/master during active pipeline" >&2
    exit 2
  fi
fi

exit 0
```

### stop-hook.sh (simplified)

```bash
#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

command -v jq &>/dev/null || exit 0

WS=$(find_active_workspace) || exit 0
STATE="$WS/state.json"
[ -f "$STATE" ] || exit 0

STATUS=$(jq -r '.currentPhaseStatus' "$STATE") || exit 0

case "$STATUS" in
  completed|abandoned|awaiting_human) exit 0 ;;
  *) echo "Forge pipeline is still active (status: $STATUS)" >&2; exit 2 ;;
esac
```

---

## 7. Go Project Structure

The MCP server already lives at `mcp-server/` as a separate Go module (`github.com/hiromaily/claude-forge/mcp-server`). New packages are added alongside existing ones:

```text
mcp-server/
├── main.go                             # Existing entry point (stdio transport)
├── tools/                              # Existing tool handlers
│   ├── registry.go
│   ├── state_*.go                      # Existing: 26 state-manager commands
│   ├── ast_*.go                        # Existing: ast_summary, ast_find_definition, dependency_graph, impact_scope
│   ├── search_patterns.go              # Existing: BM25 search over .specs/index.json
│   ├── subscribe_events.go             # Existing: SSE event streaming
│   ├── pipeline_init.go                # NEW
│   ├── pipeline_next_action.go         # NEW
│   ├── pipeline_report_result.go       # NEW
│   ├── history_*.go                    # NEW
│   ├── profile_get.go                  # NEW
│   ├── analytics_*.go                  # NEW
│   └── validate_*.go                   # NEW
├── state/                              # Existing state management
│   ├── manager.go                      # Extend with in-memory + dual-write
│   └── schema.go                       # Extend with migration support
├── ast/                                # Existing AST analysis
├── search/                             # Existing BM25 search
├── events/                             # Existing SSE streaming
├── orchestrator/                       # NEW
│   ├── engine.go                       # Main state machine (all 26 decision branches)
│   ├── engine_test.go                  # Unit tests for all 26 branches
│   ├── phases.go                       # 17 phase definitions, ordering, transitions
│   ├── actions.go                      # Action type definitions (5 types)
│   ├── flow_templates.go               # 5 templates, skip tables, stub synthesis rules
│   ├── flow_templates_test.go
│   ├── detection.go                    # task_type/effort detection cascades
│   ├── detection_test.go
│   ├── verdict.go                      # Review verdict parsing (APPROVE/REVISE/FAIL)
│   └── verdict_test.go
├── history/                            # NEW
│   ├── index.go                        # .specs/ scanner + TF-IDF index builder
│   ├── index_test.go
│   ├── search.go                       # BM25 similarity search (no API key needed)
│   ├── patterns.go                     # Review finding pattern accumulator
│   ├── patterns_test.go
│   └── friction.go                     # AI friction map from improvement reports
├── profile/                            # NEW
│   ├── analyzer.go                     # Repository analysis (languages, CI, linter, etc.)
│   ├── analyzer_test.go
│   └── cache.go                        # Profile persistence (.specs/repo-profile.json)
├── analytics/                          # NEW
│   ├── collector.go                    # Per-pipeline metrics collection
│   ├── estimator.go                    # Cost/time prediction from historical data
│   ├── estimator_test.go
│   └── reporter.go                     # Dashboard generation
├── prompt/                             # NEW
│   ├── builder.go                      # Dynamic prompt construction
│   ├── builder_test.go
│   ├── templates.go                    # Per-agent prompt templates
│   └── context.go                      # History/profile/pattern context injection
├── validation/                         # NEW
│   ├── input.go                        # Input validation (replaces validate-input.sh)
│   ├── input_test.go
│   └── artifact.go                     # Artifact structure validation
├── go.mod                              # module github.com/hiromaily/claude-forge/mcp-server
└── go.sum
```

---

## 8. MCP Tool Registration

The new tools are registered in the same `forge-state` MCP server (`forge-state-mcp` binary). Updated tool counts:

**Current**: 32 total (26 state-manager commands + 6 MCP-only tools)
**After migration**: 23 total (17 new public tools + 6 existing MCP-only tools)

The 26 individual state-manager commands are **absorbed** into `pipeline_next_action` and `pipeline_report_result` as internal logic. They are no longer exposed as separate MCP tools. Four simplified state tools (`state_get`, `state_phase_stats`, `state_resume_info`, `state_abandon`) replace them for the cases the LLM still needs direct access.

Updated canonical command table additions:

| Shell command | MCP tool | Category |
| --- | --- | --- |
| _(MCP-only)_ | `pipeline_init` | Orchestration |
| _(MCP-only)_ | `pipeline_init_with_context` | Orchestration |
| _(MCP-only)_ | `pipeline_next_action` | Orchestration |
| _(MCP-only)_ | `pipeline_report_result` | Orchestration |
| _(MCP-only)_ | `history_search` | Data Flywheel |
| _(MCP-only)_ | `history_get_patterns` | Data Flywheel |
| _(MCP-only)_ | `history_get_friction_map` | Data Flywheel |
| _(MCP-only)_ | `profile_get` | Repo Profile |
| _(MCP-only)_ | `analytics_pipeline_summary` | Analytics |
| _(MCP-only)_ | `analytics_repo_dashboard` | Analytics |
| _(MCP-only)_ | `analytics_estimate` | Analytics |
| _(MCP-only)_ | `validate_input` | Validation |
| _(MCP-only)_ | `validate_artifact` | Validation |

No changes to binary name (`forge-state-mcp`), server registration in `.claude/settings.json`, or build/install process (`make install`).

---

## 9. Reliability

### Crash Recovery

If the MCP Server crashes mid-pipeline (process abort, OOM, etc.):

1. **In-memory state is lost** — cannot be restored
2. **state.json remains** — persisted up to the last state update via dual-write
3. **Automatic restore on session resume**:
   - On MCP Server startup, scan `.specs/` for `state.json` files with `currentPhaseStatus` of `in_progress` or `awaiting_human`
   - If found, restore in-memory state from `state.json`
   - On first `pipeline_next_action` call, re-execute the interrupted phase

```go
// Automatically runs on MCP Server startup
func (s *Server) RecoverActivePipeline(specsDir string) error {
    entries, _ := filepath.Glob(filepath.Join(specsDir, "*/state.json"))
    for _, e := range entries {
        state, _ := LoadState(e)
        if state.CurrentStatus == "in_progress" || state.CurrentStatus == "awaiting_human" {
            s.registry.RestoreFrom(state)
            log.Printf("Recovered active pipeline: %s (phase: %s)", state.SpecName, state.CurrentPhase)
            return nil
        }
    }
    return nil // no active pipeline
}
```

### Multi-Pipeline Management

The current design assumes one active pipeline per session. The MCP Server manages multiple pipelines via workspace key:

```go
type PipelineRegistry struct {
    mu        sync.RWMutex
    pipelines map[string]*StateManager  // workspace path → StateManager
    active    string                     // currently active workspace
}
```

Since Claude Code's Agent tool executes sequentially, two pipelines cannot run in parallel. This design supports the use case: "start a new pipeline without abandoning the previous one → resume later."

### Logging and Debugging

MCP Server debug logs output to stderr (MCP protocol uses stdin/stdout):

- Control log level via `DEV_PIPELINE_LOG_LEVEL=debug` environment variable
- Log every tool call request/response
- Log all state transitions (`phase-1:pending → phase-1:in_progress`)
- File output to `.specs/mcp-server.log` (only when `--debug` flag is set)

---

## 10. Competitive Moat Analysis

| Feature | Copyable? | Why it's a moat |
| --- | --- | --- |
| Go MCP Server code | Yes | But testing + edge case coverage takes months to accumulate |
| History Index (TF-IDF) | Code: yes, Data: no | 100 past pipeline runs cannot be replicated by copying code |
| Review Pattern DB | Code: yes, Data: no | Repository-specific patterns require actual usage to accumulate |
| AI Friction Map | Code: yes, Data: no | Friction points are discovered through real pipeline failures |
| Repo Profile Cache | Partially | Profile itself is re-generable, but improvement-report-driven updates are unique |
| Cost/Time Estimator | Code: yes, Data: no | Predictions improve with usage; day-1 estimates are useless |
| Deterministic Orchestration | Yes | But 26 decision branches × 5 flow templates = complex test matrix |

**Core insight**: The code is the delivery mechanism. The data accumulated through usage is the moat. Every pipeline run enriches the history index, pattern DB, friction map, and estimator — making the next run better. Competitors can copy the code on day 1 but cannot replicate months of accumulated repository-specific intelligence.

---

## 11. Migration Plan (Phased)

### Phase A: Foundation (state management + orchestration engine)

Corresponds to GitHub issue #46.

1. Extend `state/` package: full `PipelineState` struct, in-memory manager with dual-write
2. New `orchestrator/` package: engine with all 26 decision branches
3. New `validation/` package: input validation (replaces `validate-input.sh`)
4. Register `pipeline_init`, `pipeline_init_with_context`, `pipeline_next_action`, `pipeline_report_result` tools
5. SKILL.md rewrite (~50 lines)
6. Hook script simplification
7. **Test**: Run existing `test-hooks.sh` + new Go unit tests for all 26 branches

### Phase B: Data Flywheel

Corresponds to GitHub issues #43, #44, #45.

1. New `history/` package: `.specs/` scanner, TF-IDF index, BM25 search
2. New `history/` package: review pattern accumulator, friction map
3. New `prompt/` package: dynamic prompt builder with context injection
4. Register `history_search`, `history_get_patterns`, `history_get_friction_map` tools
5. **Test**: Run pipeline on real tasks, verify history context improves agent output

### Phase C: Repository Intelligence

1. New `profile/` package: repo analyzer, cache
2. Profile context injection into prompts via `prompt/builder.go`
3. Register `profile_get` tool
4. **Test**: First-run analysis on multiple repos, verify profile accuracy

### Phase D: Analytics

1. New `analytics/` package: collector, estimator, reporter
2. Dashboard integration into Final Summary phase
3. Register `analytics_pipeline_summary`, `analytics_repo_dashboard`, `analytics_estimate` tools
4. **Test**: Verify predictions against actual pipeline runs

---

## 12. Open Questions

1. **Prompt template management**: Should agent `.md` files remain as-is or be embedded in Go? → Recommendation: keep `.md` files. Claude Code's Agent tool references them by file path. `buildPrompt()` reads the `.md` file and appends additional context (Layers 3–4).

2. **History index persistence**: Persist TF-IDF index to `.specs/index.json` or rebuild each time? → Recommendation: persist + differential update on startup. Full rebuild becomes slow as `.specs/` grows. (Note: the existing `search/` package and `build-specs-index.sh` already handle this — integrate rather than replace.)

3. **Backward compatibility**: How to maintain compatibility with existing `.specs/` directories (`state.json` v1)? → Implement v1 → v2 conversion in `state/migration.go`.

4. **Pattern DB persistence**: Persist `PatternAccumulator` data to `.specs/patterns.json`. Load on MCP Server startup, write on pipeline completion.

5. **Cross-repo learning**: Future mechanism for aggregating patterns across multiple repositories.

6. **Existing search/ package**: The current `search/` package already implements BM25 over `.specs/index.json`. The `history/` package should reuse this rather than duplicating the search logic.
