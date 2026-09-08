# 17 — A turn owns the futures it starts

Starting a tool in its own goroutine transfers no ownership away from the
turn. The parent context still owns both the response stream and every tool
future created while processing that response.

When the parent context is cancelled, the mini harness does two things in this
order:

1. It drains the already-started futures. Context-aware executors observe the
   cancellation and return a normal, call-associated error result.
2. It records `cancellationErr` and returns before the outer loop can request a
   follow-up model response.

```text
completed tool call → start future
user cancels turn   → executor context is cancelled
                   → collect its result into history
                   → return the turn; do not continue the model loop
```

The order matters. Returning immediately would leave an unobserved goroutine
and lose a tool outcome; continuing would let cancellation accidentally behave
like an ordinary tool failure and trigger another model request.

Codex's `ToolCallRuntime` follows the same ownership principle with a
`CancellationToken`. It races the dispatch task against cancellation; if the
call has not reached a terminal outcome, it aborts the task and produces an
explicit aborted-tool output. Our Go version is intentionally smaller: it
requires the executor to cooperate through `context.Context`.
