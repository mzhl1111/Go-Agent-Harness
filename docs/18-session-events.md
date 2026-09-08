# 18 — Runtime events are not model history

A harness has two outward-facing products during a turn:

- **Durable history** is the compact sequence given back to the model on its
  next request.
- **Runtime events** are progress signals for a CLI, UI, telemetry collector,
  or test.

The mini harness now sends the second product through `TurnObserver`:

```text
model text/input delta → TurnTextDelta / TurnToolInputDelta
completed output item  → TurnItemCompleted
tool finishes          → TurnToolResult
approval / follow-up   → TurnApprovalNeeded / TurnFollowUp
terminal state         → TurnCompleted | TurnCancelled | TurnStreamFailed | TurnFatal
```

`Session` emits these events in its own goroutine and in turn order. Tool
executors still have the separate `DispatchObserver` because they run in
concurrent futures and expose lower-level handler lifecycle.

This is a scaled-down version of Codex's session event boundary: the core turn
code sends `EventMsg` values such as content deltas, completed items, approval
requests, errors, and turn completion. The UI is therefore a consumer of the
harness runtime, not a dependency of its control flow.

The console in `main.go` is now just one observer implementation. Replacing it
with a web socket, a terminal UI, or an event recorder requires no change to
the turn loop.
