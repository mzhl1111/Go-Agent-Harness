# 32 — One cell can make many nested tool calls

A code-mode cell is a persistent program, so one execution can call more than
one tool before it yields or completes. The cell-local tool results are not
agent-turn history items.

The new end-to-end test runs a program that calls `count("first")`, then
`count("second")`. It verifies three boundaries:

```text
cell program
  → nested call: cell_1 / tool_1
  → nested call: cell_1 / tool_2
  → one completed cell output to the model
```

Both nested calls have `ToolCallCodeMode` provenance and a distinct
`RuntimeToolCallID`. Yet the next model request receives only:

```text
Script completed
Output:
nested results: first, second
```

This is deliberately sequential because `CellContext.CallTool` awaits each
result. A later exercise could add cell-local concurrent promises; that would
change local program semantics, but not the agent-history boundary.
