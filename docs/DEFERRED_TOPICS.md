# Deferred deep dives

## Streaming: where Codex retries actually live

When we build the streaming `Model` client, compare it with `codex-rs/core/src/session/turn.rs` and `responses_retry.rs`.

- Codex's general automatic retry is primarily around a failed Responses sampling stream.
- It retries the model request with bounded backoff and may switch transport.
- Tool handlers return `FunctionCallError::RespondToModel` or `Fatal`; this does not generally mean that the registry silently reruns a potentially side-effecting tool.
- Our current `ToolResult.Retryable` policy is intentionally simpler and must be presented as appropriate only for explicitly idempotent tools.

Revisit this before implementing streaming so the mini harness does not accidentally teach that every tool failure is safe to replay.
