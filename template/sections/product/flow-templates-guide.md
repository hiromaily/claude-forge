# Flow Templates

The effort level determines which flow template is used and which phases run.

## Template Selection

| Effort | Template | Description |
| --- | --- | --- |
| **S** | `light` | Lean pipeline — skips task review, Checkpoint B, and comprehensive review |
| **M** | `standard` | Balanced — skips task review and Checkpoint B |
| **L** | `full` | All phases mandatory, `--auto` ignored for checkpoints |

> XS effort is not supported. Use S for small tasks.

## Effort Detection

Effort is determined in this priority order:

1. `--effort=` CLI flag (explicit override)
2. Jira story points (if source is a Jira issue)
3. Heuristic detection (LLM-based)
4. Default: `M`

## Phase Execution Matrix

| Phase | Task | S (light) | M (standard) | L (full) |
| ----- | ---- | :-------: | :----------: | :------: |
| 0 | Input Validation | ✅ | ✅ | ✅ |
| 1 | Workspace Setup | ✅ | ✅ | ✅ |
| 2 | Detect Effort | ✅ | ✅ | ✅ |
| 3 | Situation Analysis | ✅ | ✅ | ✅ |
| 4 | Investigation | ✅ | ✅ | ✅ |
| 5 | Design | ✅ | ✅ | ✅ |
| 6 | Design Review | ✅ | ✅ | ✅ |
| 7 | Checkpoint A | ✅ | ✅ | ✅ |
| 8 | Task Decomposition | ✅ | ✅ | ✅ |
| 9 | Tasks Review | | | ✅ |
| 10 | Checkpoint B | | | ✅ |
| 11 | Implementation | ✅ | ✅ | ✅ |
| 12 | Code Review | ✅ | ✅ | ✅ |
| 13 | Comprehensive Review | | ✅ | ✅ |
| 14 | Final Verification | ✅ | ✅ | ✅ |
| 15 | PR Creation | ✅ | ✅ | ✅ |
| 16 | Final Summary | ✅ | ✅ | ✅ |
| 17 | Final Commit | ✅ | ✅ | ✅ |
| 18 | Post to Source | ✅ | ✅ | ✅ |
| 19 | Done | ✅ | ✅ | ✅ |

## Checkpoint Behavior

- **Checkpoint A** is always blocking when design ran. Use `--auto` to allow AI reviewer verdict to auto-approve.
- **Checkpoint B** runs only for effort L. `--auto` is ignored for effort L — human approval is always required.
- Checkpoints are skipped entirely for `investigation` tasks (Checkpoint A) and for `bugfix`, `docs`, `investigation`, `refactor` tasks (Checkpoint B).
