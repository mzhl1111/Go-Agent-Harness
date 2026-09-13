# 30 — Approval pauses a host invocation; it does not replace it

A nested host tool call can be stopped by exactly the same pre-hook policy as
a direct call. The extra rule at a transport boundary is correlation:
approval must not lose the host's `invocation_id`.

`HostNestedToolAdapter` owns a small pending map:

```text
host ToolCall(invocation_id = host_invocation_9)
  → ToolRegistry.Start
  → PreHookNeedsApproval
  → adapter stores:
      host_invocation_9 → original ToolCall
  → UI receives ApprovalRequest

user approves host_invocation_9
  → adapter.takePending(host_invocation_9)
  → ToolRegistry.StartAfterApproval(original ToolCall)
  → executor runs once
  → CompleteToolCall(host_invocation_9, result)
```

There are three deliberate choices here:

1. The pending map is in `HostNestedToolAdapter`, not `ToolRegistry`.
   The registry owns policy and execution; only the adapter knows which opaque
   host callback must eventually be completed.
2. `ResumeApproved` uses `StartAfterApproval`, so the same pre-hook does not
   request a second approval and the executor does not need a special mode.
3. `takePending` removes the entry before execution. A duplicate approval
   decision cannot execute or complete the same host invocation twice.

This mirrors the earlier direct-call and in-process cell lessons: approval is
a continuation boundary. The only new responsibility introduced by a remote
host is retaining the external correlation ID across that boundary.
