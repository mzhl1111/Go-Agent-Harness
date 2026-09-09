# 22 — A cell is not a turn

The first code-mode layer is deliberately small and in-process. `CellManager`
creates a persistent `Cell` for an originating agent-level code call. A cell can
then request nested tools through the existing `ToolRegistry`.

```text
agent turn
  └─ originating code call: outer_code_call
       └─ cell_1 (running)
            └─ nested call: cell_1_tool_1
                 source = CodeMode { cell_1, tool_1 }
                 → ToolRegistry → existing executor
```

The executor is unchanged. The difference is provenance and lifecycle: the
nested result belongs to the cell, rather than being immediately appended as a
direct model tool result in the agent turn's history.

For teaching simplicity, the generated nested call ID combines the cell ID and
runtime sequence. Codex keeps an outer call ID and a per-cell runtime tool-call
ID as separate protocol values. The important invariant is the same: each cell
has a stable identity and each nested call has a monotonic identity inside it.

This version supports `running → completed` and `running → cancelled` state
transitions. The next layer will add a cell program and its `yield / wait`
behavior; only then will we connect a cell's final output back to an agent turn.
