# 01 — Runtime skeleton

The purpose of this exercise is to trace one action around the loop, not to reproduce every Codex subsystem.

## The correspondence

| Codex (Rust) | Mini harness (Go) | Role |
| --- | --- | --- |
| `core/src/session/turn.rs:run_turn` | `Session.runTurn` | Owns the outer agent-turn loop. |
| `core/src/stream_events_utils.rs:handle_output_item_done` | `Session.handleOutputItemDone` | Translates a completed model item into a tool call and records its result. |
| `core/src/tools/registry.rs:dispatch_any_with_terminal_outcome` | `ToolRegistry.DispatchAnyWithTerminalOutcome` | Places policy hooks around a reusable executor. |
| `tools/src/tool_executor.rs:ToolExecutor` | `ToolExecutor` | The reusable execution contract. |
| `core/src/tools/handlers/unified_exec/exec_command.rs:ExecCommandHandler` | `ExecCommandHandler` | A concrete executor implementation. |
| `core/src/hook_runtime.rs:run_turn_stop_hooks` | `stopHooks` | Decides whether the harness needs another model response. |

The upstream checkout currently used for this mapping is commit `93ac341410b9698c8b4badd5df5b7561d93c4ef9`.

## Run the trace

`go run .` should produce this logical sequence:

```text
pre hook: exec_command
post hook: call_1 error= false
tool result: hello harness
stop hook requested follow-up: send tool result back to model
assistant: The executor returned its result; the turn is complete.
```

The important control-flow shape is:

```text
runTurn
  └─ model.Next
       └─ handleOutputItemDone
            └─ registry: PreHook → executor.Handle → PostHook
  └─ stopHooks
       └─ needsFollowUp? yes → continue → model.Next again
```

`ScriptedModel` stands in for a real Responses client. It makes the two model responses explicit, allowing us to focus on harness ownership: the model asks for a tool, the harness executes it, then the harness re-enters the loop with the result available.

The stop hook records its prior decision in `followUpReason`. Hooks run after *every* model response, so using only `len(toolResults) == 1` would request a follow-up forever: the result remains in the turn context on the second pass. This small guard is the first example of an orchestration invariant rather than a tool-executor concern.

## First reading pass in the upstream source

1. Open `reference/codex/codex-rs/core/src/session/turn.rs` at `run_turn`. Ignore setup and find the loop plus the `needs_follow_up` branch.
2. Read `handle_output_item_done` only far enough to follow `ResponseItem → ToolCall → tool_future`.
3. Read registry dispatch as three regions: pre-hooks, handler dispatch, post-hooks.
4. Compare the trait contract with the `ExecCommandHandler` implementation. The registry owns orchestration; the handler owns one tool's work.
5. Return to `run_turn` and trace how the stop-hook result induces the next iteration.

## Deliberate simplifications

- No streaming transport: a response is a small slice of `ResponseItem` values.
- No real subprocesses: `ExecCommandHandler` only accepts `echo`.
- No approval flow, cancellation, parallel calls or persistence yet.

These omissions are intentional. Adding them one at a time is the rest of the course.
