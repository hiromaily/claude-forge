## The problem with SDD today

The AI development landscape has evolved through three phases:

**1. Vibe coding** — "Write me a function that does X." Works for small tasks. Breaks as complexity grows. The model loses focus, context fills up, nothing is reproducible.

**2. Spec-Driven Development (SDD)** — Write a spec first, then hand it to AI. Better. But you're still the orchestrator. You manage each handoff, watch for quality regressions, decide when to move on. It's an improvement — but it's still manual.

**3. Pipeline automation** — You describe a task once; the system runs the full workflow, enforces constraints, reviews its own output, and self-reports on where it got stuck.

Anthropic's own research puts it plainly: ["Measuring Agent Autonomy in Practice"](https://www.anthropic.com/research/measuring-agent-autonomy) found a significant _deployment overhang_ — models can handle far more autonomy than humans actually grant them. The bottleneck isn't model intelligence. It's how humans structure workflows around the models.

claude-forge is built for phase 3.

---
