package main

import (
	"context"
	"testing"
)

type blockingExecutor struct {
	started chan string
	release <-chan struct{}
}

func (e blockingExecutor) Handle(_ context.Context, input string) ToolResult {
	e.started <- input
	<-e.release
	return ToolResult{Output: input}
}

type oneResponseModel struct{ items []ResponseItem }

func (m oneResponseModel) Next(*TurnContext) []ResponseItem { return m.items }

func TestToolCallsStartConcurrentlyAndKeepModelOrder(t *testing.T) {
	release := make(chan struct{})
	started := make(chan string, 2)
	session := &Session{
		model: oneResponseModel{items: []ResponseItem{
			{Kind: "tool_call", Tool: "block", Input: "first", CallID: "call_1"},
			{Kind: "tool_call", Tool: "block", Input: "second", CallID: "call_2"},
		}},
		tools: &ToolRegistry{executors: map[string]ToolExecutor{"block": blockingExecutor{started: started, release: release}}},
	}

	done := make(chan *TurnContext, 1)
	go func() { done <- session.runTurn(context.Background()) }()
	<-started
	<-started // Both started before either can complete.
	close(release)
	turn := <-done

	if got, want := len(turn.toolResults), 2; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
	if got, want := turn.toolResults[0].CallID, "call_1"; got != want {
		t.Errorf("first result CallID = %q, want %q", got, want)
	}
	if got, want := turn.toolResults[1].CallID, "call_2"; got != want {
		t.Errorf("second result CallID = %q, want %q", got, want)
	}
}

func TestUnknownToolProducesCallAssociatedError(t *testing.T) {
	registry := &ToolRegistry{}
	future := registry.Start(context.Background(), ToolCall{Name: "missing", ID: "call_missing"})
	result := <-future.result
	if !result.IsError || result.CallID != "call_missing" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
