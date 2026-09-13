# 27 — A cancelled turn owns every active cell

Direct tool futures inherit the turn context, but a yielded code cell may have
no future running when its parent turn is cancelled. Leaving it alive would
create an orphaned program that could later accept an approval or issue work.

`Session.cancelActiveCells` therefore terminates every `running`, `yielded`, or
`waiting_approval` cell, records one terminal cell output, removes its pending
cell approvals, and clears any follow-up request created by that output.

```text
turn context cancelled
  → drain current futures
  → CancelAll active cells
  → Script terminated output for each cell
  → remove cell approvals
  → return cancelled turn; never sample the model again
```

Codex follows the same ownership principle: its code-mode service can
`interrupt_active_cells`, terminating each active runtime cell during an
interrupted turn. The teaching version remains in-process but preserves the
important lifecycle rule.
