# 06 — Module map

The mini harness is now split by runtime responsibility instead of by the order in which we learned it.

| File | Owns | Does not own |
| --- | --- | --- |
| `main.go` | Demo wiring and a demo stop hook | Harness runtime logic |
| `types.go` | Shared contracts and data structures | Dispatch policy or execution |
| `model.go` | The `Model` boundary and scripted demo responses | Turn state or tools |
| `stream.go` | Stream event/failure contracts and cancellable demo stream | Turn state or tool policy |
| `retry.go` | Shared bounded-retry configuration | Model or tool behavior |
| `session.go` | Turn loop, history, follow-up, pause/resume and tool timeouts | Tool-specific behavior |
| `registry.go` | Pre-hook, retries, dispatch events, futures, post-hook | Model loop or history |
| `executor.go` | The safe `echo` executor | Approval, hooks or turn state |

```text
main ──wires──> Session ──uses──> Model
                    │
                    └──────────> ToolRegistry ──uses──> ToolExecutor
```

This separation is the Go version of the boundary we saw in Codex: reusable tool contracts remain independent from session/turn orchestration.
