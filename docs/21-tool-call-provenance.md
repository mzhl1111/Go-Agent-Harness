# 21 — Every tool call has a source

An executor needs the same input regardless of who requested it, but the
harness must retain the requester. The mini harness now carries a
`ToolCallSource` through admission, registry dispatch, results, and turn
events.

```text
today:  Direct
later:  CodeMode { cell_id, runtime_tool_call_id }
```

The direct model-output router creates an explicit `Direct` source. Registry
callers that construct a teaching `ToolCall` manually receive `Direct` as a
safe default.

This mirrors Codex's distinction between `ToolCallSource::Direct` and
`ToolCallSource::CodeMode`. It will let the later cell layer preserve
correlation, cancellation, output policy, and telemetry without creating a
second executor abstraction.
