# 20 — A tool-call ID is an admission key

A completed tool-call item is the boundary that starts a potentially
side-effecting executor. Delivery can be duplicated by a faulty stream client,
a resume protocol, or an integration bug. The harness must not turn duplicate
delivery into duplicate execution.

The mini harness now remembers every admitted `ToolCall` by ID for the lifetime
of one turn.

```text
new call ID                         → record it and start one future
same ID + same tool + same input    → ignore duplicate delivery
same ID + changed contents          → fatal protocol violation
```

The second case protects side effects. The third case cannot be safely guessed:
the harness does not know which meaning the call ID is supposed to represent.

This is deliberately narrower than a production replay protocol. Codex's
`ExecutedToolCallRecorder` records tool metadata for prompts and tracing; it is
not a generic direct-call execution deduplicator. Its code-mode host separately
uses invocation IDs and a bounded seen-ID set to make completion callbacks
idempotent. The teaching rule prepares us for that later `invocation_id` design
without pretending to implement full reconnect recovery.
