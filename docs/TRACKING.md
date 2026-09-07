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

## Ground rules

- Keep every milestone runnable and small.
- Model the control-flow boundary before modeling transport details.
- Add a test whenever a new behavior changes the turn loop.
- Keep the upstream clone local and unmodified; write our notes and code here.
