# 13 — A completed item returns runtime consequences

`Session.handleOutputItemDone` now returns `OutputItemResult`:

```go
type OutputItemResult struct {
    ToolFuture     *ToolFuture
    NeedsFollowUp bool
}
```

For a completed text item, both fields are empty. For a completed tool-call item, the session creates a `ToolFuture` and sets `NeedsFollowUp` to true.

`streamResponse` ORs `NeedsFollowUp` across all completed items in the response. After awaiting tool futures, `continueTurn` consumes that flag and asks the model for the next response. This replaces the previous demo-only rule that guessed from the number of tool results.

```text
output_item_done(tool call)
  → { ToolFuture, NeedsFollowUp: true }
  → await tool result
  → next model stream
```

Stop hooks remain available for extra policy, but the fundamental follow-up requirement now comes from the item handler itself. This mirrors Codex's `OutputItemResult`, where `handle_output_item_done()` supplies both a tool future and `needs_follow_up` to the outer turn loop.

If an approval pauses the turn, the pending follow-up flag is cleared: approving the suspended call explicitly resumes the session and requests the next model response exactly once.
