# Learning tracking

## Current milestone: 1 — runtime skeleton

- [x] Clone the `openai/codex` source locally as a read-only reference.
- [x] Locate the five primary runtime entry points in the current upstream source.
- [x] Implement a minimal `runTurn → ToolRegistry → ToolExecutor → StopHook` flow.
- [x] Demonstrate `needsFollowUp` re-entering the outer loop.
- [x] Guard the stop-hook decision so an unchanged tool result cannot cause an infinite follow-up loop.
- [x] Run the mini harness and `go test ./...` with Go 1.27.1.
- [ ] Review `run_turn` in the upstream source line by line, limited to the loop and stop-hook branch.
- [ ] Compare this Go trace against one real Codex tool call.

## Current milestone: 2 — tool futures and parallelism

- [x] Replace the synchronous executor result with a future/channel abstraction.
- [x] Permit multiple completed tool-call items in one model response.
- [x] Preserve stable call IDs when results return.
- [x] Add deterministic tests for concurrent starts, result ordering and errors.

## Current milestone: 3 — approvals and terminal outcomes

- [x] Let a pre-hook continue, block, or require approval.
- [x] Represent an approval request as a terminal dispatch outcome.
- [x] Ensure an approval request skips the executor and post-hook.
- [x] Retain pending approval details in turn context with the original call ID.
- [x] Resume an approved tool call without replaying the whole model response.

## Current milestone: 4 — context and bounded outputs

- [x] Make the model's input an explicit, inspectable turn-history value.
- [x] Serialize tool results and approval states into that history.
- [x] Add a hard cap to tool output inserted into context.
- [x] Test that the model receives append-only history with truncated tool output.

## Ground rules

- Keep every milestone runnable and small.
- Model the control-flow boundary before modeling transport details.
- Add a test whenever a new behavior changes the turn loop.
- Keep the upstream clone local and unmodified; write our notes and code here.

## Structure

- [x] Split the teaching implementation into session, registry, executor, model and shared-contract modules.

## Current milestone: 5 — cancellation, retry and observability

- [x] Propagate a per-tool timeout context from session to executor.
- [x] Test that a context-aware executor returns a bounded timeout error.
- [x] Add retry classification and a bounded retry policy.
- [x] Add structured dispatch events for observability.

## Deferred for streaming

- [x] Contrast executor-level retry in this teaching harness with Codex's response-stream retry loop and explain idempotency.

## Current milestone: 6 — streaming model output

- [x] Replace batch model output with an event stream.
- [x] Keep text deltas out of durable model history.
- [x] Route only completed output items into tool dispatch.
- [x] Add stream transport errors and move general retry to the model-stream layer.
- [x] Buffer streamed tool-input deltas by call ID until the output item completes.
- [x] Derive follow-up from completed tool items instead of demo-specific stop-hook logic.
- [x] Buffer assistant text deltas by item ID and commit them only at item completion.
- [x] Separate router validation errors from executor/registry errors.
- [x] Make tool parallelism an explicit executor capability with a safe serial default.

## Current milestone: 7 — turn cancellation ownership

- [x] Drain every started tool future after parent-context cancellation.
- [x] Stop the turn before a cancellation can trigger a model follow-up.
- [x] Preserve the cancelled tool result in history for inspection.

## Current milestone: 8 — session events

- [x] Replace session-local printing and delta callbacks with a unified `TurnObserver`.
- [x] Keep runtime events separate from durable model history.
- [x] Demonstrate a replaceable CLI observer and test event delivery.

## Current milestone: 9 — deterministic future draining

- [x] Make the ordered future-drain boundary explicit in `Session`.
- [x] Verify tool handlers may finish out of order while history and turn events remain in model-item order.
