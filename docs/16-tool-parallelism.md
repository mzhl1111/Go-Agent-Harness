# 16 — Parallelism is a tool capability

Starting multiple futures does not mean every tool is safe to execute at the same time. The mini harness now uses an opt-in interface:

```go
type ParallelToolExecutor interface {
    ToolExecutor
    SupportsParallelToolCalls() bool
}
```

An executor that does not implement this interface is serialized with other calls to the same tool name. `ExecCommandHandler` opts in explicitly.

```text
parallel-capable tool → handlers may run concurrently
default tool          → same-name handlers wait behind a one-slot gate
```

The gate is per tool name, so unrelated tools can still progress independently. This teaching implementation serializes the entire dispatch lifecycle for a default tool; a production system may choose a narrower handler-only lock depending on its hook and telemetry requirements.

This follows Codex's `ToolExecutor::supports_parallel_tool_calls()` default of `false`. The registry exposes the capability to routing code instead of assuming every model-proposed tool call can be parallelized.
