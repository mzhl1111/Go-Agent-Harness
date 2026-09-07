package main

import "context"

// ScriptedModel stands in for the Responses API. Each loop iteration obtains
// one response; tool results accumulated in TurnContext are its next context.
type ScriptedModel struct{ responseNumber int }

func (m *ScriptedModel) Stream(ctx context.Context, _ []HistoryItem) ModelStream {
	m.responseNumber++
	if m.responseNumber == 1 {
		return modelEventStream(ctx,
			ModelEvent{Kind: ModelToolInputDelta, CallID: "call_1", Delta: "echo "},
			ModelEvent{Kind: ModelToolInputDelta, CallID: "call_1", Delta: "hello"},
			ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "tool_call", Tool: "exec_command", CallID: "call_1"}},
			ModelEvent{Kind: ModelToolInputDelta, CallID: "call_2", Delta: "echo gated"},
			ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "tool_call", Tool: "exec_command", CallID: "call_2"}},
		)
	}
	return modelEventStream(ctx,
		ModelEvent{Kind: ModelTextDelta, Delta: "The approved tool completed"},
		ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "text", Text: "The approved tool completed; this turn is complete."}},
	)
}
