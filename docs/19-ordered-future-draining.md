# 19 — Execute concurrently, commit deterministically

When one model response contains two tool calls, the harness starts both
futures as their output items complete. Their handlers may finish in either
order. That is an execution detail, not a reason to make the next model input
nondeterministic.

`Session.drainPendingFutures()` waits for pending work in original
completed-item order and only then commits each result. A pending item may be a
direct tool future or a code-cell future; direct results emit `TurnToolResult`,
while a cell publishes one `TurnCellOutput`.

```text
model items:       call_1 ───────── call_2
handler finishes:                call_2 ─ call_1
history/events:    call_1 result ─ call_2 result
```

The second handler can already be finished while the turn waits for the first.
Its buffered future result is then collected immediately after the first one.
This trades a little presentation latency for a stable transcript and a stable
next model request.

Codex makes this policy explicit with `FuturesOrdered`: each completed tool
item adds a future with `push_back`, and `drain_in_flight()` awaits them in that
same order after the response stream ends. It is different from `FuturesUnordered`,
which would expose completion order instead.
