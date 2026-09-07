# Learning tracking

## Current milestone: 1 — runtime skeleton

- [x] Clone the `openai/codex` source locally as a read-only reference.
- [x] Locate the five primary runtime entry points in the current upstream source.
- [x] Implement a minimal `runTurn → ToolRegistry → ToolExecutor → StopHook` flow.
- [x] Demonstrate `needsFollowUp` re-entering the outer loop.
- [ ] Review `run_turn` in the upstream source line by line, limited to the loop and stop-hook branch.
- [ ] Compare this Go trace against one real Codex tool call.

## Next milestone: 2 — tool futures and parallelism

- [ ] Replace the synchronous executor result with a future/channel abstraction.
- [ ] Permit multiple completed tool-call items in one model response.
- [ ] Preserve stable call IDs when results return.
- [ ] Add deterministic tests for result ordering and errors.

## Ground rules

- Keep every milestone runnable and small.
- Model the control-flow boundary before modeling transport details.
- Add a test whenever a new behavior changes the turn loop.
- Keep the upstream clone local and unmodified; write our notes and code here.
