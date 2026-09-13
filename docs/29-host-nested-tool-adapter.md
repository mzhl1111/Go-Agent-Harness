# 29 — A host adapter preserves two identities

`HostNestedToolAdapter` is our first code-mode transport seam. It accepts a
host-pushed nested call, converts it into the existing `ToolRegistry` contract,
then sends the resulting completion back through `HostToolCompletionSink`.

```text
host ToolCall
  (invocation_id, cell_id, runtime_tool_call_id)
        |
        v
HostNestedToolAdapter.Dispatch
        |
        v
ToolRegistry.Start(ToolCall)
        |
        +-- approval --> HostToolCallOutcome.Approval (do not complete yet)
        |
        +-- result ----> CompleteToolCall(invocation_id, result)
```

The adapter deliberately stores two different identifiers:

| Identifier | Owner | Purpose |
| --- | --- | --- |
| `ToolCall.ID` | teaching harness | Internal registry, history, and retry identity; derived from `cell_id` + `runtime_tool_call_id`. |
| `ToolCallSource.HostInvocationID` | code-mode host | The opaque callback correlation key that must be supplied unchanged when completing the host call. |

This distinction prevents a subtle distributed-systems bug: a client must not
invent or rewrite the host correlation ID just because it has a convenient
local ID. The test dispatches `invocation_42` and verifies that the registry
uses `cell_remote_7_runtime_3` internally while the sink receives exactly
`invocation_42`.

`HostToolCompletionSink` is intentionally only an interface. A later adapter
can implement it with the generated gRPC client's `CompleteToolCall` method;
the turn loop and the reusable registry do not need to know about that change.

The remaining gap is approval. When the registry yields an approval request,
the host call must stay pending and, after a user decision, the exact same host
`invocation_id` must receive the completion. That is the next lifecycle rule.
