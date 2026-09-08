# 14 — Streamed assistant text has item identity

Assistant text delta is keyed by `ItemID`, not by tool `CallID`:

```text
text_delta(message_1, "Hello, ")
text_delta(message_1, "streaming world.")
output_item_done(text message_1)
```

`TurnContext.streamedAssistantText` buffers those fragments. `onTextDelta(itemID, delta)` can update the UI immediately, while `finalizeStreamedAssistantText` writes a single complete assistant item to history only when the output item is done.

```text
ItemID-keyed text fragments → UI
                         └→ completed text item → durable assistant history
```

`ItemID` and `CallID` represent different identities. An item ID identifies a streamed response item such as an assistant message; a call ID identifies a tool invocation. Keeping separate maps prevents text and tool-argument streams from corrupting each other's state.

The test verifies that two UI deltas become one `Hello, streaming world.` history entry and that the temporary buffer is deleted at completion.
