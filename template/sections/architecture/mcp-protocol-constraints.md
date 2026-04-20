## MCP Protocol Constraints

This document captures the protocol-level constraints of MCP (Model Context Protocol) that shape forge-state's architecture. These are hard constraints — they cannot be worked around without changes to the MCP specification or the host application (Claude Code).

### Transport Model

MCP uses JSON-RPC 2.0 over stdio (stdin/stdout). The transport is strictly **request-response**:

- The **client** (Claude Code) initiates all tool calls.
- The **server** (forge-state) can only respond to requests — it cannot push data to the client unprompted.
- Server-to-client notifications exist (`notifications/message` for logging, `notifications/tools/list_changed` for tool discovery) but cannot carry arbitrary data or trigger client-side actions.

**Implication**: The server cannot notify the client when an external event occurs (e.g., a Dashboard user clicks "Approve"). The client must poll or long-poll for state changes.

### Tool Call Timeout

Claude Code enforces a **default 60-second timeout** on MCP tool calls. Configurable via the `MCP_TIMEOUT` environment variable (value in milliseconds):

```bash
MCP_TIMEOUT=120000 claude  # 2-minute timeout
```

**Implication**: Long-poll durations inside MCP tool handlers must stay below this threshold. forge-state uses 50 seconds (10-second safety margin) for checkpoint long-polls.

### No Streaming Responses

MCP tool calls return a single `CallToolResult`. There is no mechanism for streaming partial results or sending multiple response chunks from a single tool invocation.

**Implication**: A tool call that needs to both present data and wait for a subsequent event cannot do so in a single call. This requires a two-phase pattern: return data on the first call, long-poll on the second call.

### Orchestrator Is an LLM

The MCP client is driven by an LLM (Claude) interpreting SKILL.md instructions. This introduces non-determinism:

- The LLM may deviate from prescribed tool-call sequences.
- Polling loops (`if still_waiting, call again`) are unreliable because the LLM may instead ask the user for input.
- The fewer tool calls required for a given workflow, the more deterministic the outcome.

**Implication**: Design patterns should minimize the number of LLM-dependent tool calls. Absorb multi-step workflows into single tool calls where possible. When polling is unavoidable, maximize the poll duration to minimize iterations.

### Design Patterns for forge-state

| Pattern | Description | Example |
|---------|-------------|---------|
| **Long-poll absorption** | Block inside a tool handler waiting for an event, rather than returning and expecting the LLM to re-call | `pipeline_next_action` checkpoint long-poll (50s) |
| **Internal state transitions** | Perform state transitions inside the tool handler rather than requiring the LLM to call a separate tool | Checkpoint absorption: `pipeline_next_action` calls `sm.Checkpoint()` internally |
| **P1-P7 dispatch loops** | Absorb skip, task_init, batch_commit, rename_branch, push_branch actions internally without returning to the LLM | `pipeline_next_action` P1-P7 loops |
| **Two-phase checkpoint** | 1st call: return checkpoint text (instant). 2nd call: long-poll for approval (50s) | Checkpoint-a, checkpoint-b |

### Timeout Calculation

```
MCP_TIMEOUT (default 60s) - safety_margin (10s) = long_poll_timeout (50s)
```

The 10-second margin accounts for:
- JSON serialization/deserialization overhead
- Network latency (negligible for stdio, but defensive)
- State reload and engine computation after the long-poll wakes up
