## Four things that make it different

### 1. SDD is still manual — claude-forge isn't

SDD tells you *what* to do at each phase. It doesn't *run* the phases. You still decide when to move from analysis to design, when to approve, when to iterate.

claude-forge automates the full handoff chain. Each phase writes a markdown artifact. The next phase reads it. No context sharing, no conversation history — just structured files as the API between agents.

### 2. Improvement loop — automatic, not optional

Most teams measure AI output by the artifact: did it ship? But the real cost is invisible.

AI spent 40% of its tokens re-reading docs it couldn't find quickly. Context had to be re-established multiple times because agents shared a session. You never see this. You just see a PR.

After every run, claude-forge emits an **Improvement Report** — appended to `summary.md` — identifying exactly where the pipeline got stuck:

- Documentation gaps that slowed agents down
- Missing conventions that caused repeated clarification loops
- Token-heavy phases caused by poorly structured context

Most teams de-prioritize this under deadline pressure. claude-forge makes it automatic on every run.

To act on it, feed the report back into a new pipeline:

```text
/forge Review and implement the improvement suggestions in .specs/{date}-{name}/summary.md
```

This turns every completed run into a compounding investment — the codebase progressively gets easier for both humans and future AI runs.

### 3. Flow optimization — effort-aware scaling

Not every task needs 11 phases and 3 review cycles.

claude-forge selects the pipeline template based on effort level (S / M / L) — from a lean light pipeline to a full 11-phase run with mandatory human checkpoints.

A small task doesn't go through task review. A large one doesn't skip it. The workflow adapts to the effort, not the other way around.

### 4. MCP-driven determinism — engine and hooks, not just prompts

LLM instructions are probabilistic. A well-prompted agent *usually* follows them. But "usually" isn't enough when the cost of a mistake is high.

claude-forge removes phase-transition decisions from the LLM entirely. A Go engine (`forge-state-mcp`) owns all orchestration logic: which phase runs next, retry counts, skip conditions, artifact validation. The LLM executes typed actions returned by the engine — it cannot invent steps or skip them.

This determinism runs at two layers:

**Engine layer (MCP)** — all transition decisions are deterministic functions of `state.json`. Phase sequencing, artifact validation, retry limits, review verdict handling, and checkpoint gating — none of it is subject to LLM interpretation.

**Hook layer (shell)** — critical invariants enforced at the OS level:
- **Read-only guard** — blocks source edits during analysis phases (exit 2)
- **Commit guard** — prevents git commits during parallel task execution
- **Stop guard** — prevents session termination while a pipeline is in progress (exit 2)

Neither layer depends on the LLM following instructions. They're hard stops.

---
