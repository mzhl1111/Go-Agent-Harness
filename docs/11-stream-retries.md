# 11 — Retry the model stream, not every completed tool

`ModelStream` now has two channels:

```go
type ModelStream struct {
    Events <-chan ModelEvent
    Err    <-chan *StreamFailure
}
```

`StreamFailure` explicitly says whether a transport failure is retryable. `Session.streamResponse` applies `streamRetry` around a whole model-stream request.

```text
open model stream
  → consume events
  → clean close                     → continue turn
  → retryable failure, no completed item
      → bounded backoff → open a new stream
  → failure after completed item
      → retain failure; do not replay
```

The last branch is deliberately conservative. If an `output_item_done` tool call has already appeared, reopening the stream could emit it again and run a side effect twice. Real Codex has much richer persisted response history, response IDs and executed-tool-call tracking to recover safely. This teaching harness therefore makes the safety boundary visible instead of pretending a generic retry is always correct.

The tests verify both cases:

- a retryable connection failure before any completed item reconnects once and succeeds;
- a retryable failure after a completed tool call does not reconnect and does not execute that tool a second time.
