# 05 — Explicit model history and bounded tool output

`TurnContext` now owns an append-only `[]HistoryItem`. The `Model` interface receives `turn.modelInput()`, which is a copy of that history rather than the mutable turn object.

```text
user input
  → assistant_tool_call (call ID + tool input)
  → tool result (call ID + bounded output)
  → approval request, when applicable
  → next model request
```

This makes a central harness question observable: *exactly what can the model see on its next request?*

`recordToolResult` truncates output to `maxToolOutputChars` before it enters history. The truncation marker is included inside the cap, so no single tool result can grow the next model request without bound. The original `ToolResult.Output` remains available to the harness; only the model-facing history representation is shortened.

`TestModelReceivesAppendOnlyHistoryWithBoundedToolOutput` checks that the second model request retains the initial user entry, includes the matching tool call ID, and receives a truncated output rather than the full tool result.
