# 03 — Approvals are terminal dispatch outcomes

The executor must not decide whether it is permitted to run. That decision belongs at the harness boundary, before the handler.

`PreHook` now returns one of three decisions:

| Decision | Handler runs? | Outcome |
| --- | --- | --- |
| `PreHookContinue` | Yes | `ToolResult` |
| `PreHookBlocked` | No | Error `ToolResult` for the model |
| `PreHookNeedsApproval` | No | `ApprovalRequest` stored in `TurnContext` |

```text
ToolCall
  → PreHook
      ├─ continue       → executor → post hook → ToolResult
      ├─ blocked        → error ToolResult
      └─ needs approval → ApprovalRequest (no executor, no post hook)
```

An approval request is terminal for *this dispatch attempt*: the goroutine completes, but the tool has not executed. The session keeps the request's call ID, tool name, input and explanation in `pendingApprovals`. A real UI can render that request and later resume the exact call after the user decides.

`TestApprovalRequestSkipsExecutorAndPreservesCallIdentity` verifies the key safety property: an approval outcome does not invoke the executor at all.
