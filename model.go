package main

type Model interface {
	Next([]HistoryItem) []ResponseItem
}

// ScriptedModel stands in for the Responses API. Each loop iteration obtains
// one response; tool results accumulated in TurnContext are its next context.
type ScriptedModel struct{ responseNumber int }

func (m *ScriptedModel) Next(_ []HistoryItem) []ResponseItem {
	m.responseNumber++
	if m.responseNumber == 1 {
		return []ResponseItem{
			{Kind: "tool_call", Tool: "exec_command", Input: "echo hello", CallID: "call_1"},
			{Kind: "tool_call", Tool: "exec_command", Input: "echo gated", CallID: "call_2"},
		}
	}
	return []ResponseItem{{Kind: "text", Text: "The approved tool completed; this turn is complete."}}
}
