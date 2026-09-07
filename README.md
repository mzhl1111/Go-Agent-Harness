# Go Agent Harness — learning Codex by rebuilding its skeleton

This is a small, deliberate Go implementation of the control flow behind an agent harness. It is not a Codex fork and it does not execute arbitrary commands. The local upstream reference clone lives at `reference/codex` and is excluded from Git.

## Start here

Read [the first study note](docs/01-runtime-skeleton.md), then run:

```sh
go run .
```

The program intentionally supports only `echo ...`, making each run deterministic and safe.

## Learning progression

1. **Runtime skeleton** — outer turn loop, tool dispatch and stop-hook follow-up. *(implemented)*
2. Streaming output items and concurrent tool futures. *(implemented: futures and concurrent calls; streaming remains next)*
3. Approvals and terminal outcomes. *(implemented: approval request suspension; resume is next)*
4. Context/history construction and bounded tool outputs.
5. Cancellation, retries and observability.

See [the tracking checklist](docs/TRACKING.md) for the current state.
