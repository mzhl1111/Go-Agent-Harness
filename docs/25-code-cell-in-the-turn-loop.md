# 25 — A code cell is a completed model item with its own future

The mini harness now recognizes a completed `code_cell` model item. Its
`CellProgram` is a teaching substitute for the JavaScript runtime:

```text
ModelOutputItemDone(code_cell)
  → Session starts CellProgram as a CellFuture
  → cell calls nested tools through ToolRegistry
  → cell yields or completes
  → Session records exactly one CellOutput
  → outer loop samples the model again
```

`PendingFuture` holds either a direct `ToolFuture` or a `CellFuture`. This
keeps their collection in completed-item order while allowing both to run
concurrently.

The end-to-end test uses a program that calls nested `echo`, then yields. The
next model request receives the yielded script output under `call_code`; it
does not receive the nested `cell_1_tool_1` result as an independent history
item. This is the essential runtime distinction between a direct tool call and
code mode.
