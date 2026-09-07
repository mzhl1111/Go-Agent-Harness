# 09 — Structured dispatch events

`ToolRegistry` now emits lifecycle events through an optional `DispatchObserver`.

```text
started
  → waiting_approval | blocked | retrying → completed | failed
```

Each `DispatchEvent` includes the call ID, tool name, attempt count and a concise message. The observer boundary keeps session and executor code free of UI or logging dependencies: a caller may write events to a terminal, telemetry system, event log or test collector.

Tool futures run in concurrent goroutines, so observer implementations must be safe for concurrent `OnDispatch` calls. The test collector uses a mutex and verifies this retry sequence:

```text
started(call_observed)
retrying(call_observed, attempt 1)
completed(call_observed, attempt 2)
```

The registry emits the final `completed` or `failed` event only after the final dispatch result is known. This matches the earlier rule that post-hooks see only the final result.
