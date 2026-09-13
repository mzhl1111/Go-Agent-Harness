# 31 — Host cancellation wins over a nested approval

The real code-mode protocol sends session events and nested tool calls on
independent streams. Its `ToolCallCancelled` comment explicitly warns that a
cancellation may arrive *before* the corresponding `ToolCall`.

`HostNestedToolAdapter.CancelInvocation` therefore records cancellation by
host `invocation_id` and removes any approval-paused call:

```text
ToolCallCancelled(invocation_id)
  → adapter.cancelled[invocation_id] = true
  → remove pending approval, if present

later ToolCall(invocation_id)
  → adapter rejects it
  → executor is never started
```

The same rule prevents a later approval click from resuming a call that the
host has already retired. The test covers both event orders: cancellation after
approval pause, and cancellation before the tool callback arrives.

This scope is deliberately narrow. The adapter does not yet cancel an
*already-running* executor; that needs a per-invocation context owned by the
host transport. The direct turn cancellation lesson already shows that
context-cancellation mechanism at the session boundary.
