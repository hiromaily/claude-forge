---
layout: home

hero:
  name: claude-forge
  text: Pipeline Automation for AI Development
  tagline: A Claude Code plugin that replaces ad-hoc AI workflows with structured, multi-phase pipelines — isolated subagents, deterministic guardrails, and state that survives restarts.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/introduction
    - theme: alt
      text: View on GitHub
      link: https://github.com/hiromaily/claude-forge

features:
  - title: Effort-Aware Scaling
    details: Effort level (S/M/L) selects one of 3 flow templates — from a lean pipeline to a full multi-phase run with mandatory human checkpoints.
    icon: ⚡
  - title: Deterministic Guardrails
    details: Critical constraints enforced at the shell level via hooks — not just prompt instructions. Read-only guards, commit blocks, and checkpoint gates.
    icon: 🛡️
  - title: AI Review Loops
    details: Design and task plans go through APPROVE/REVISE cycles with dedicated reviewer agents before implementation begins.
    icon: 🔄
  - title: Disk-Based State
    details: All progress tracked in state.json via Go MCP server (44 tools). Pipelines survive context compaction and session restarts.
    icon: 💾
  - title: 10 Specialist Agents
    details: Each phase handled by a dedicated agent with its own context window — no shared state, no context pollution.
    icon: 🤖
  - title: Improvement Reports
    details: Every run emits a retrospective identifying documentation gaps, friction points, and token-heavy phases for continuous improvement.
    icon: 📊
---
