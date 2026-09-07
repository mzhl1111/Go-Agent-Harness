# 10 — Streaming: deltas versus completed items

The model boundary is now:

```go
Stream(context.Context, []HistoryItem) <-chan ModelEvent
```

There are two teaching events:

| Event | Consumer | Durable history? | Can start a tool? |
| --- | --- | --- | --- |
| `text_delta` | UI callback | No | No |
| `output_item_done` | `Session.handleOutputItemDone` | Yes | Yes |

`text_delta` is presentation progress. It can arrive incomplete, be revised in a real protocol, or stop mid-message on a network failure. The harness therefore sends it only to `onTextDelta`.

`output_item_done` is the semantic boundary. A finished tool-call item becomes a `ToolCall` and starts a future; a finished text item becomes an assistant history item. This mirrors Codex's `handle_output_item_done()`: it records the completed response item and queues the tool future only after the output item is done.

The test sends a `draft text` delta before a completed tool call. It verifies that the UI callback receives the delta but the next model request's history does not contain it.

## Deferred comparison: retries

At this boundary, revisit [the deferred retry note](DEFERRED_TOPICS.md). Codex's general retry loop surrounds the sampling stream, not arbitrary tool handlers. In a production stream client, a retry must carefully rebuild history and avoid attributing a new response's tool calls to a failed prior response.
