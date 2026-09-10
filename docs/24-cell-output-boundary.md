# 24 — Nested results return to the cell; cell output returns to the model

A code cell may call several nested tools. Their results are values for the
running program, not separate tool outputs for the model. The agent receives
one output only when the cell yields, completes, or is terminated.

```text
cell_1 → nested exec → "file list"  ┐
cell_1 → nested read → "contents"   ├─ local cell state
cell_1 → Yield("two files found")   ┘
                                      ↓
agent history: tool(call_code_1,
  "Script running with cell ID cell_1 ...")
```

`recordCellOutput` is this bridge in the mini harness. It records the output
under the originating top-level call ID, requests the next model response, and
emits one `TurnCellOutput` event. It deliberately does not append each nested
call result to agent history.

Codex follows the same shape in `handle_runtime_response`: a runtime
`Yielded`, `Result`, or `Terminated` response becomes one model-visible output
with a script-status header. Nested calls are dispatched through the normal
tool runtime, but their typed results return to the code cell first.
