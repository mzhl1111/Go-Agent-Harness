# 28 — The real code-mode boundary is a host protocol

Our teaching harness keeps `CellManager`, the cell program, and tool dispatch
in one Go process. That was intentional: it exposed the control-flow
invariants before any network or JavaScript-runtime details.

In Codex, `codex-core` does **not** execute a code-mode cell itself. It talks
to a stateful code-mode host through a session-scoped gRPC protocol:

```text
codex-core                         code-mode host
    | OpenSession --------------------> session lease stream
    | <------------------------------ SessionOpened(session_id)
    | SubscribeToToolCalls ----------> nested-call stream
    | <------------------------------ ToolCall(invocation_id, cell_id, ...)
    | existing policy + ToolRegistry
    | CompleteToolCall(invocation_id, outcome) --->
    | Execute / Wait / Terminate ----> cell lifecycle events
```

The relevant upstream contract is
`reference/codex/codex-rs/code-mode-protocol/src/grpc/codex.code_mode.v1.proto`.
The Rust client opens the session and immediately starts the tool subscription
in `code-mode/src/grpc_session/mod.rs`; `codex-core` asks the service to
terminate active cells when an interrupted turn has the code-mode interrupt
feature enabled (`core/src/tasks/mod.rs`).

## Mapping the teaching design to Codex

| Teaching harness | Codex host protocol | Why the boundary matters |
| --- | --- | --- |
| `CellManager.StartProgram` | `Execute` | A direct model `code_cell` call starts one persistent runtime cell. |
| `CellManager.StartTool` | `SubscribeToToolCalls` delivers a `ToolCall` | The runtime emits a nested request; core does not let arbitrary cell code bypass its tool policy. |
| `ToolRegistry.Start` | core's existing policy/approval/executor path | Direct and nested calls share tool execution logic; provenance distinguishes them. |
| `ToolDispatchOutcome` | `CompleteToolCall` success/failure outcome | The harness returns the result *to the runtime*, not directly to agent history. |
| `CellOutput` | `ExecuteEvent` / `Wait` outcome | Only a yielded, completed, or terminated cell response crosses back to the agent turn. |
| `CancelAll` | `Terminate` for each active cell | A turn owns its cells even when they are idle or awaiting approval. |

There is one identity detail worth preserving in our next abstraction. A real
host tool callback includes both:

- `invocation_id`: the host's correlation key; it must be sent unchanged to
  `CompleteToolCall`.
- `runtime_tool_call_id`: an identifier supplied by the code runtime.

Our in-process `ToolCall.ID` currently plays both roles. That is convenient
for a single process but is too weak at a process boundary. An adapter should
therefore retain a host `invocation_id` separately while still constructing a
teaching `ToolCall` with `Source{CodeMode, CellID, RuntimeToolCallID}`.

## What stays the same

Moving the runtime out of process does **not** turn code mode into a second
agent or replace executors. The ownership chain remains:

```text
agent turn → code cell → nested tool request → registry/executor
          ← cell yield/complete ← tool completion
```

The next implementation step is deliberately smaller than a full gRPC client:
define a host-facing nested-call adapter with separate invocation identity,
then test that it routes through the existing registry and returns the exact
completion to the host-facing interface.
