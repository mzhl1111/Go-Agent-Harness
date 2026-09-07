# 08 — Bounded retries are dispatch policy

The model emits one tool call. Retrying that call is a local runtime decision inside `ToolRegistry`, not a second request to the model.

`RetryPolicy` has two controls:

- `MaxAttempts` is a hard upper bound; zero defaults to one attempt.
- `Backoff` waits between attempts and remains cancellable through the tool context.

An executor opts into retry by returning `ToolResult{IsError: true, Retryable: true}`. The registry stops immediately for success, a non-retryable failure, or the final allowed attempt. `PostHook` runs once with the final result, not for intermediate failed attempts.

```text
executor result
  ├─ success                 → final result
  ├─ non-retryable failure   → final result
  └─ retryable failure
       ├─ attempts remain    → optional cancellable backoff → retry
       └─ limit reached      → final result
```

The tests cover a success after two transient failures, exhaustion at exactly three attempts, and a permanent failure that is never retried.
