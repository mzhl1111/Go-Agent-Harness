# 02 — Tool futures and parallelism

In the first version, `handleOutputItemDone` blocked while one tool ran. That means a response containing two independent calls would execute them serially.

Now `ToolRegistry.Start` returns a `ToolFuture`: the registry launches dispatch in a goroutine and returns a receive-only result channel immediately. `runTurn` first starts every tool call in the response, then awaits each future.

```text
model response: [call_1, call_2]
       │
       ├─ Start(call_1) ── goroutine ── result channel 1
       ├─ Start(call_2) ── goroutine ── result channel 2
       │
       └─ await channel 1, then channel 2
             └─ append `{CallID, ToolResult}` in model-item order
```

Execution order and result order are different choices:

- The tool work may finish in any order.
- The harness deliberately sends tool outputs back in original model-item order.
- `CallID` makes every output unambiguously belong to its original request even when tools run concurrently.

`main_test.go` uses two blocked executors. The test cannot release either one until it has observed that both started, proving that the second call did not wait for the first to finish. It also checks the deterministic result order and that an unknown tool produces an error associated with its call ID.
