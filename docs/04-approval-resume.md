# 04 — Resume one approved call

An approval does not restart an agent turn. The harness retains an `ApprovalRequest` containing the original call ID, tool name and input. When the user approves it, `Session.resumeApproved`:

1. finds that exact request by `CallID`;
2. calls `ToolRegistry.StartAfterApproval` for that one call;
3. removes the pending request and records the tool result; and
4. re-enters `continueTurn` for the model's *next* response.

```text
response 1: [call_1, call_2]
  ├─ call_1 executes
  └─ call_2 requests approval → turn pauses

user approves call_2
  └─ resume call_2 only → response 2
```

`StartAfterApproval` does not bypass all pre-hooks. It only treats a repeated `PreHookNeedsApproval` decision as satisfied for the call the session already matched to a pending approval request. A later `PreHookBlocked` result would still prevent execution.

The integration test asserts that the model response counter is `1` at suspension and `2` after resuming: response 1 was not replayed.
