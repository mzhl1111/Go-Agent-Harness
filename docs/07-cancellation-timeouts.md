# 07 — Cancellation and per-tool timeouts

Cancellation is a capability carried by `context.Context`, not a concern hidden inside `ToolRegistry`.

```text
parent turn context
  → Session derives a per-tool timeout context
  → ToolRegistry forwards it unchanged
  → ToolExecutor watches ctx.Done()
```

`Session.toolTimeout` is optional. When set, `startTool` uses `context.WithTimeout` for that call and stores the cancellation function on its `ToolFuture`. The session releases the timer immediately after receiving the future's outcome.

This only works when an executor cooperates. A context-aware executor must select on or otherwise check `ctx.Done()` while it waits. An executor that ignores its context cannot be forcibly stopped safely by this small Go harness.

`TestSessionToolTimeoutCancelsContextAwareExecutor` uses a handler that waits for cancellation. It verifies that the session returns an error result for the original call ID with `context deadline exceeded` rather than hanging forever.
