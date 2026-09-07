package main

import "context"

// ScriptedModel stands in for the Responses API. Each loop iteration obtains
// one response; tool results accumulated in TurnContext are its next context.
type ScriptedModel struct{ responseNumber int }

func (m *ScriptedModel) Stream(ctx context.Context, _ []HistoryItem) <-chan ModelEvent {
	m.responseNumber++
	if m.responseNumber == 1 {
		return modelEventStream(ctx,
			ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "tool_call", Tool: "exec_command", Input: "echo hello", CallID: "call_1"}},
			ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "tool_call", Tool: "exec_command", Input: "echo gated", CallID: "call_2"}},
		)
	}
	return modelEventStream(ctx,
		ModelEvent{Kind: ModelTextDelta, Delta: "The approved tool completed"},
		ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "text", Text: "The approved tool completed; this turn is complete."}},
	)
}
