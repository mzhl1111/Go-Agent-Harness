# 33 — Steer changes the next model request; interrupt cancels work

`Session.Steer` mirrors the core Codex distinction between user steering and
turn interruption. It appends a user item to the active turn's protected
pending-input queue, then makes the next outer-loop pass request another model
response.

```text
current model stream or tool future
  → user calls Steer("change direction")
  → pending user input is queued
  → current work finishes normally
  → queued input is appended to history
  → model samples again with prior result + new user input
```

No parent context is cancelled. That matters for tools with possible side
effects: a changed user goal is not permission to abandon a partly executed
command or remote mutation.

Some tools are useful only while no new input has arrived. They opt into the
new `SteerAwareToolExecutor` contract and receive a one-shot steer signal.
The teaching `steerWaitingExecutor` test represents Codex's `wait` and `sleep`
handlers: it wakes early, returns a normal tool result, and lets the same turn
perform its follow-up sampling.

| Signal | Ordinary executor | Steer-aware waiting executor |
| --- | --- | --- |
| `Steer` | Continues | May return early by choice |
| Parent context cancellation | Receives cancellation | Receives cancellation |

The two end-to-end tests prove both branches. They also show that the usual
bounded-output rule still applies: the model receives the truncated tool result
followed by the queued steer input.
