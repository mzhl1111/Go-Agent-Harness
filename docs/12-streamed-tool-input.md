# 12 — Streamed tool input is not executable input

Tool arguments can arrive in pieces before the model finishes the tool-call item. The mini harness represents that with:

```text
tool_input_delta(call_1, "echo ")
tool_input_delta(call_1, "hello")
output_item_done(tool_call call_1)
```

`TurnContext.streamedToolInputs` buffers the deltas by call ID. On `output_item_done`, `finalizeStreamedToolInput` fills the completed item's empty input from that buffer, deletes the buffer, and only then calls `handleOutputItemDone`.

```text
deltas → CallID-keyed buffer → completed item → ToolCall → ToolFuture
```

The buffer is intentionally not placed in durable history and is never sent to an executor early. A partial command or partial JSON argument is presentation/progress state, not an authorized tool request.

This is a simplified analogue of Codex creating an active tool-argument-diff consumer on `ResponseEvent::OutputItemAdded`, then dispatching only when the completed item reaches `handle_output_item_done()`.
