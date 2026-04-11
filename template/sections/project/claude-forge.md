# claude-forge

**[📖 Documentation](https://hiromaily.github.io/claude-forge/)** | **[📖 ドキュメント (日本語)](https://hiromaily.github.io/claude-forge/ja/)**

---

Spec-Driven Development got you most of the way there.

You write the spec. AI does the implementation. You review. It works — until you realize you're still managing every handoff manually. You kick off analysis, wait for output, hand off context to the next prompt, watch for mistakes, review intermediate work, decide when to proceed — on every task, on every run.

The bottleneck is no longer prompting. It's orchestration.

You start caring about:
- token efficiency
- context isolation
- reproducibility across runs
- structuring artifacts so AI can actually use them

I built **claude-forge** to automate that layer.

It's a Claude Code plugin that replaces ad-hoc AI development workflows with a structured, multi-phase pipeline — isolated subagents, deterministic guardrails, and state that survives restarts.

Instead of writing better prompts, you build a system where AI development can run predictably.

---
