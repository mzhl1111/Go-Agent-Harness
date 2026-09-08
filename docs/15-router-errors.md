# 15 — Router errors happen before executor errors

`router.go` performs the small but important conversion:

```text
completed ResponseItem → ToolCall | no tool call | ItemError
```

It has two error outcomes:

| Error | Meaning | Harness action |
| --- | --- | --- |
| `respond_to_model` | The model emitted an invalid tool-call shape, such as no tool name or call ID. | Append a `tool_error` history item and follow up with the model. |
| `fatal` | The completed protocol item itself has an unsupported kind. | Set `TurnContext.fatalError` and end the turn. |

This is distinct from an **unknown registered tool**. That path has a valid `ToolCall`, enters `ToolRegistry`, and returns an error `ToolResult` associated with the call ID. Router validation happens earlier, before a future or executor exists.

```text
completed item
  → router
      ├─ valid tool call → registry → executor
      ├─ model-visible validation error → history → follow-up
      └─ fatal protocol error → end turn
```

The two tests exercise a missing tool name and an unsupported item kind. This mirrors Codex's `ToolRouter::build_tool_call()` match inside `handle_output_item_done()`, where `RespondToModel` produces a function-call output for the next model request while `Fatal` aborts the operation.
