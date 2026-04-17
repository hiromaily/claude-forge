# User Preferences (`/forge-setup`)

## Overview

Add per-repository default flag settings for `/forge` pipelines via `.specs/preferences.json`. A new `/forge-setup` skill provides an interactive setup flow. Two new MCP tools (`preferences_get`, `preferences_set`) manage persistence. Existing `pipeline_init` merges preferences as defaults before explicit flags.

## Constraints

- **No single-run disable**: once a preference is enabled (e.g. `auto: true`), there is no `--no-auto` flag to disable it for a single run. This is intentional -- preferences represent "always-on" defaults. To temporarily disable, edit preferences via `/forge-setup`.
- **Explicit flags always win**: `/forge fix-bug --effort=L` overrides `effort: "M"` in preferences.
- **Unknown fields are ignored**: `preferences_set` silently drops unrecognised keys.

## Preferences File

**Path**: `.specs/preferences.json`

```json
{
  "auto": true,
  "debug": true,
  "effort": "M",
  "nopr": true,
  "discuss": false
}
```

| Field | Type | Valid values | Default (when absent) |
|-------|------|-------------|----------------------|
| `auto` | bool | `true`/`false` | `false` |
| `debug` | bool | `true`/`false` | `false` |
| `effort` | string | `"S"`, `"M"`, `"L"` | `null` (auto-detect) |
| `nopr` | bool | `true`/`false` | `false` |
| `discuss` | bool | `true`/`false` | `false` |

All fields are optional. Missing fields = not set (system default applies).

## MCP Tools

### `preferences_get`

- **Parameters**: none
- **Behaviour**: reads `.specs/preferences.json`. If file or `.specs/` directory does not exist, returns `{}`.
- **Response**: JSON object with current preferences (only fields that are set)
- **specsDir resolution**: uses the same `resolveSpecsDir()` path as the rest of the MCP server (injected at startup)

### `preferences_set`

- **Parameters**: `preferences` (JSON object, required) -- full preferences object to write
- **Behaviour**:
  1. Validate: `effort` must be `"S"` | `"M"` | `"L"` if present; bool fields must be bool
  2. Strip unknown fields
  3. Create `.specs/` directory if it does not exist (`os.MkdirAll`)
  4. Write `.specs/preferences.json` atomically (write to temp, rename)
- **Response**: `{"ok": true}` on success; MCP error on validation failure
- **Write semantics**: full replacement (not merge). The skill collects all values and writes once.

## Go Implementation

### New file: `mcp-server/internal/state/preferences.go`

```go
type Preferences struct {
    Auto    *bool   `json:"auto,omitempty"`
    Debug   *bool   `json:"debug,omitempty"`
    Effort  *string `json:"effort,omitempty"`
    NoPR    *bool   `json:"nopr,omitempty"`
    Discuss *bool   `json:"discuss,omitempty"`
}

func LoadPreferences(specsDir string) (Preferences, error)
func SavePreferences(specsDir string, p Preferences) error
func (p Preferences) Validate() error
```

- All fields are pointers to distinguish "not set" from "set to zero value".
- `LoadPreferences`: returns zero `Preferences{}` if file does not exist (not an error).
- `SavePreferences`: `MkdirAll(specsDir)`, write to `preferences.json.tmp`, rename.
- `Validate`: checks `Effort` is in `ValidEfforts` if non-nil.

### New file: `mcp-server/internal/tools/preferences.go`

Two handler functions:

```go
func PreferencesGetHandler(specsDir string) server.ToolHandlerFunc
func PreferencesSetHandler(specsDir string) server.ToolHandlerFunc
```

- `specsDir` is passed from `RegisterAll` (same value used by other tools).
- `PreferencesGetHandler`: calls `state.LoadPreferences(specsDir)`, returns JSON.
- `PreferencesSetHandler`: parses `preferences` param as JSON, calls `Validate()`, calls `SavePreferences`.

### Modified: `mcp-server/internal/tools/registry.go`

Add two tool registrations in `RegisterAll`. The `specsDir` parameter must be added to the `RegisterAll` signature:

```go
func RegisterAll(
    srv *server.MCPServer, sm *state.StateManager, bus *events.EventBus,
    slack *events.SlackNotifier, eventsPort string, eng *orchestrator.Engine,
    agentDir string, specsDir string, // <-- add specsDir
    histIdx *history.HistoryIndex, kb *history.KnowledgeBase,
    profiler *profile.RepoProfiler,
    col *analytics.Collector, est *analytics.Estimator, rep *analytics.Reporter,
)
```

Registration:

```go
srv.AddTool(
    mcp.NewTool("preferences_get",
        mcp.WithDescription("Read user preferences from .specs/preferences.json. Returns {} if not configured."),
    ),
    PreferencesGetHandler(specsDir),
)

srv.AddTool(
    mcp.NewTool("preferences_set",
        mcp.WithDescription("Write user preferences to .specs/preferences.json. Full replacement."),
        mcp.WithObject("preferences", mcp.Required(), mcp.Description("Preferences JSON object")),
    ),
    PreferencesSetHandler(specsDir),
)
```

### Modified: `mcp-server/cmd/main.go`

Pass `specsDir` to `RegisterAll`:

```go
tools.RegisterAll(srv, sm, bus, slack, eventsPort, eng, agentDir, specsDir, histIdx, kb, profiler, col, est, rep)
```

### Modified: `mcp-server/internal/tools/pipeline_init.go`

Add `mergeWithPreferences` function. Called in `PipelineInitHandler` after `buildFlags`:

```go
// mergeWithPreferences applies preferences as defaults to flags.
// Explicit flags (non-zero values from buildFlags) always take precedence.
func mergeWithPreferences(flags *PipelineInitFlags, prefs state.Preferences) {
    if !flags.Auto && prefs.Auto != nil && *prefs.Auto {
        flags.Auto = true
    }
    if !flags.Debug && prefs.Debug != nil && *prefs.Debug {
        flags.Debug = true
    }
    if !flags.SkipPR && prefs.NoPR != nil && *prefs.NoPR {
        flags.SkipPR = true
    }
    if !flags.Discuss && prefs.Discuss != nil && *prefs.Discuss {
        flags.Discuss = true
    }
    if flags.EffortOverride == nil && prefs.Effort != nil {
        flags.EffortOverride = prefs.Effort
    }
}
```

Merge point in `PipelineInitHandler`:

```go
flags := buildFlags(result.Parsed, currentBranch)

// Apply user preferences as defaults (explicit flags win).
prefs, _ := state.LoadPreferences(specsDir)
mergeWithPreferences(flags, prefs)
```

This requires `PipelineInitHandler` to receive `specsDir`. Change its signature:

```go
func PipelineInitHandler(sm *state.StateManager, specsDir string) server.ToolHandlerFunc
```

Update the registration call in `registry.go` accordingly.

## Skill: `/forge-setup`

### New file: `skills/forge-setup/SKILL.md`

Interactive flow using MCP tools:

1. Call `mcp__forge-state__preferences_get` to load current values
2. For each setting, use AskUserQuestion with current value shown:
   - "`--auto` (skip human confirmation): currently **enabled**. Keep enabled?" → yes/no
   - "`--debug` (debug mode): currently **disabled**. Enable?" → yes/no
   - "Default effort level: currently **M**. Change?" → S / M / L / none
   - "`--nopr` (skip PR creation): currently **disabled**. Enable?" → yes/no
   - "`--discuss` (pre-pipeline discussion): currently **disabled**. Enable?" → yes/no
3. Show summary of all settings for confirmation
4. Call `mcp__forge-state__preferences_set` with the collected values
5. Display saved confirmation

The skill reads/writes only through MCP tools; no direct file manipulation.

## Documentation Updates

Tool count changes from **44 to 46**. Update via docs-ssot:

- `template/pages/CLAUDE.tpl.md` -- tool count, canonical command list, MCP-only tools list
- `template/pages/README.tpl.md` -- if tool count is mentioned
- `template/sections/` -- any section that references tool count

Also update `skills/forge/SKILL.md` Supported Flags section to mention preferences:

> Flags can also be set as persistent defaults via `/forge-setup`. Explicit flags on `/forge` always override preferences.

## Test Plan

### `mcp-server/internal/state/preferences_test.go`

| Test | Asserts |
|------|---------|
| `TestLoadPreferences_FileNotExists` | Returns zero Preferences, no error |
| `TestLoadPreferences_ValidFile` | All fields populated correctly |
| `TestLoadPreferences_PartialFile` | Only set fields are non-nil |
| `TestSavePreferences_CreatesDir` | `.specs/` created if missing |
| `TestSavePreferences_AtomicWrite` | File written atomically |
| `TestPreferences_Validate_ValidEffort` | S/M/L accepted |
| `TestPreferences_Validate_InvalidEffort` | "XS" rejected |
| `TestPreferences_Validate_NilEffort` | nil effort passes |

### `mcp-server/internal/tools/preferences_test.go`

| Test | Asserts |
|------|---------|
| `TestPreferencesGet_NoFile` | Returns `{}` |
| `TestPreferencesGet_WithPrefs` | Returns saved preferences |
| `TestPreferencesSet_Valid` | Writes file, returns ok |
| `TestPreferencesSet_InvalidEffort` | Returns MCP error |
| `TestPreferencesSet_UnknownFields` | Unknown fields stripped |

### `mcp-server/internal/tools/pipeline_init_test.go` (additions)

| Test | Asserts |
|------|---------|
| `TestMergeWithPreferences_DefaultsApplied` | Preferences fill in zero flags |
| `TestMergeWithPreferences_ExplicitFlagsWin` | `--effort=L` overrides prefs `effort: "M"` |
| `TestMergeWithPreferences_EmptyPrefs` | No change to flags |
| `TestMergeWithPreferences_PartialPrefs` | Only set prefs applied |

## File Change Summary

| File | Change |
|------|--------|
| `mcp-server/internal/state/preferences.go` | **New**: Preferences type, Load, Save, Validate |
| `mcp-server/internal/state/preferences_test.go` | **New**: 8 tests |
| `mcp-server/internal/tools/preferences.go` | **New**: preferences_get, preferences_set handlers |
| `mcp-server/internal/tools/preferences_test.go` | **New**: 5 tests |
| `mcp-server/internal/tools/pipeline_init.go` | **Modify**: add mergeWithPreferences, update handler signature |
| `mcp-server/internal/tools/pipeline_init_test.go` | **Modify**: add 4 merge tests |
| `mcp-server/internal/tools/registry.go` | **Modify**: add specsDir param, register 2 tools |
| `mcp-server/internal/tools/registry_test.go` | **Modify**: update tool count assertion |
| `mcp-server/cmd/main.go` | **Modify**: pass specsDir to RegisterAll |
| `skills/forge-setup/SKILL.md` | **New**: interactive setup skill |
| `skills/forge/SKILL.md` | **Modify**: mention preferences in Supported Flags |
| `template/pages/CLAUDE.tpl.md` | **Modify**: tool count 44→46, add to canonical list |
