# 34 — Context compaction is a replacement-history transaction

Codex does not simply delete old messages when a context window is full. Its
normal compaction path creates a separate model request, asks for a handoff
summary, then replaces the old history with retained user context plus that
summary. The next regular sampling request sees the replacement history.

The upstream prompt begins by asking the model to “Create a handoff summary
for another LLM that will resume the task.” Its essential content is progress,
decisions, constraints, user preferences, remaining work, and critical
references. Our `DefaultCompactionPrompt` keeps exactly those requirements in
a short teaching version.

```text
normal sampling produces a tool result
  → turn needs follow-up
  → context threshold exceeded
  → ContextCompactor receives the complete old history
  → retained user items + checkpoint summary replace old history
  → next model request samples the replacement history
```

`ContextCompactor` is intentionally separate from `Model` and `ToolExecutor`.
That mirrors Codex's dedicated compact request and prevents the compaction
prompt or its intermediate response from becoming normal user conversation.

The teaching harness counts history items rather than tokens. Its
`CompactionPolicy` is therefore a control-flow demonstration, not a production
token estimator. It retains the newest configured user messages and appends a
summary as the last history item. The summary is responsible for preserving
older assistant and tool state.

This lesson also corrects the stop-hook boundary: stop hooks run only after
there is no tool, pending input, or compaction reason to continue. This matches
the original `run_turn` ordering.

Relevant upstream locations:

- `codex-rs/prompts/templates/compact/prompt.md`
- `codex-rs/core/src/compact.rs`
- `codex-rs/core/src/session/turn.rs`
