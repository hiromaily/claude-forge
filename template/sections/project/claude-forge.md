# claude-forge

**From `/forge` to PR — automated.**

[![Claude Code Plugin](https://img.shields.io/badge/Claude_Code-plugin-blueviolet?logo=anthropic&logoColor=white)](https://docs.anthropic.com/en/docs/claude-code)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[📖 Documentation](https://hiromaily.github.io/claude-forge/)** | **[📖 ドキュメント (日本語)](https://hiromaily.github.io/claude-forge/ja/)**

```text
/forge "Add retry logic to the API client"
    │
    ├─ Phase 1-2  Situation Analysis + Investigation
    ├─ Phase 3    Design  ──→  Design Review (APPROVE/REVISE)
    ├─ ✋ Checkpoint A — human approval
    ├─ Phase 4    Task Decomposition  ──→  Tasks Review
    ├─ Phase 5-6  Implement + Code Review  (parallel per task)
    ├─ Phase 7    Comprehensive Review
    └─ ✅  Final Verification  →  PR created  →  Summary posted
```

---

Spec-Driven Development got you most of the way there.

You write the spec. AI does the implementation. You review. It works — until you realize you're still managing every handoff manually. You kick off analysis, wait for output, hand off context to the next prompt, watch for mistakes, review intermediate work, decide when to proceed — on every task, on every run.

**The bottleneck is no longer prompting. It's orchestration.**

I built **claude-forge** to automate that layer.

It's a Claude Code plugin that replaces ad-hoc AI development workflows with a structured, multi-phase pipeline — isolated subagents, deterministic guardrails, and state that survives restarts.

Instead of writing better prompts, you build a system where AI development can run predictably.

---

> **Documentation** is managed as a Single Source of Truth using [docs-ssot](https://github.com/hiromaily/docs-ssot). Files such as `README.md`, `CLAUDE.md`, and `ARCHITECTURE.md` are auto-generated — edit the source files under `template/` and run `make docs` to regenerate.
