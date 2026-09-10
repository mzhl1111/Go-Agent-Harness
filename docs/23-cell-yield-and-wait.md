# 23 — Yield preserves a cell; wait resumes it

Agent turns and code cells have different pause semantics. When an agent turn
ends, its next model response begins a new sampling step. When a cell yields,
the runtime keeps the same cell, ID, local state, and nested-tool sequence.

```text
running
  ├─ Yield(output) → yielded
  │                  └─ Wait() → running
  ├─ Complete() → completed
  └─ Cancel() → cancelled
```

Only a running cell may start a nested tool. A yielded cell must be explicitly
resumed with `Wait`; cancellation is allowed from either running or yielded.
Terminal cells cannot resume.

Codex's `code_mode.wait` tool asks its code-mode service either to wait for a
live cell or terminate it. The service returns another runtime response, which
may itself be yielded, completed, or terminated. Our state machine is the
smallest version of that contract; later lessons attach its cell-level output
to the turn loop, while timers, JavaScript execution, and transport remain out
of scope.
