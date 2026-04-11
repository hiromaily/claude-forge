## Flow templates

The effort level determines the flow template. XS effort is not supported; use S for small tasks.

| Effort | Template | Skipped phases |
| --- | --- | --- |
| **S** | `light` | Task review (4b), Checkpoint B, Comprehensive Review (7) |
| **M** | `standard` | Task review (4b), Checkpoint B |
| **L** | `full` | _(none)_ — all checkpoints mandatory, `--auto` ignored |

Effort is detected from: `--effort=` flag > Jira story points > heuristic > default `M`.

---
