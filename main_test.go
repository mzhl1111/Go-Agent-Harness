package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

type blockingExecutor struct {
	started chan string
	release <-chan struct{}
}

type countingExecutor struct{ calls int }

type contextWaitingExecutor struct{}

func (contextWaitingExecutor) Handle(ctx context.Context, _ string) ToolResult {
	<-ctx.Done()
	return ToolResult{IsError: true, Output: ctx.Err().Error()}
}

func (e *countingExecutor) Handle(_ context.Context, input string) ToolResult {
	e.calls++
	return ToolResult{Output: input}
}

func (e blockingExecutor) Handle(_ context.Context, input string) ToolResult {
	e.started <- input
	<-e.release
	return ToolResult{Output: input}
}

type oneResponseModel struct{ items []ResponseItem }

func (m oneResponseModel) Next([]HistoryItem) []ResponseItem { return m.items }

type historyRecordingModel struct {
	inputs         [][]HistoryItem
	responseNumber int
}

func (m *historyRecordingModel) Next(input []HistoryItem) []ResponseItem {
	m.inputs = append(m.inputs, append([]HistoryItem(nil), input...))
	m.responseNumber++
	if m.responseNumber == 1 {
		return []ResponseItem{{Kind: "tool_call", Tool: "echo", Input: "a very long tool result for context", CallID: "call_1"}}
	}
	return []ResponseItem{{Kind: "text", Text: "done"}}
}

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
	result := (<-future.result).Result
	if !result.IsError || result.CallID != "call_missing" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestApprovalRequestSkipsExecutorAndPreservesCallIdentity(t *testing.T) {
	executor := &countingExecutor{}
	registry := &ToolRegistry{
		executors: map[string]ToolExecutor{"exec": executor},
		pre: []PreHook{func(ToolCall) PreHookOutcome {
			return PreHookOutcome{Decision: PreHookNeedsApproval, Reason: "needs user consent"}
		}},
	}
	future := registry.Start(context.Background(), ToolCall{Name: "exec", Input: "echo sensitive", ID: "call_approval"})
	outcome := <-future.result

	if executor.calls != 0 {
		t.Fatalf("executor ran %d times, want 0", executor.calls)
	}
	if outcome.Approval == nil || outcome.Approval.CallID != "call_approval" {
		t.Fatalf("unexpected approval outcome: %#v", outcome)
	}
}

func TestApprovedCallResumesWithoutReplayingTheOriginalResponse(t *testing.T) {
	executor := &countingExecutor{}
	model := &ScriptedModel{}
	session := &Session{
		model: model,
		tools: &ToolRegistry{
			executors: map[string]ToolExecutor{"exec_command": executor},
			pre: []PreHook{func(call ToolCall) PreHookOutcome {
				if call.ID == "call_2" {
					return PreHookOutcome{Decision: PreHookNeedsApproval, Reason: "needs approval"}
				}
				return PreHookOutcome{Decision: PreHookContinue}
			}},
		},
	}
	turn := session.runTurn(context.Background())
	if model.responseNumber != 1 || len(turn.pendingApprovals) != 1 || executor.calls != 1 {
		t.Fatalf("turn did not suspend as expected: responses=%d approvals=%d calls=%d", model.responseNumber, len(turn.pendingApprovals), executor.calls)
	}

	turn = session.resumeApproved(context.Background(), turn, "call_2")
	if model.responseNumber != 2 || len(turn.pendingApprovals) != 0 || executor.calls != 2 {
		t.Fatalf("resume replayed or did not execute: responses=%d approvals=%d calls=%d", model.responseNumber, len(turn.pendingApprovals), executor.calls)
	}
	if got, want := len(turn.toolResults), 2; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
}

func TestModelReceivesAppendOnlyHistoryWithBoundedToolOutput(t *testing.T) {
	model := &historyRecordingModel{}
	session := &Session{
		model:        model,
		initialInput: "test task",
		tools:        &ToolRegistry{executors: map[string]ToolExecutor{"echo": ExecCommandHandler{}}},
		stopHooks: []StopHook{func(turn *TurnContext) {
			if turn.lastResponseHadToolCalls && len(turn.toolResults) == 1 && turn.followUpReason == "" {
				turn.needsFollowUp, turn.followUpReason = true, "return tool output to model"
			}
		}},
	}
	session.runTurn(context.Background())

	if got, want := len(model.inputs), 2; got != want {
		t.Fatalf("model calls = %d, want %d", got, want)
	}
	secondInput := model.inputs[1]
	if got, want := secondInput[0].Content, "test task"; got != want {
		t.Errorf("initial history = %q, want %q", got, want)
	}
	toolOutput := secondInput[len(secondInput)-1]
	if toolOutput.Role != "tool" || toolOutput.CallID != "call_1" {
		t.Fatalf("unexpected last history item: %#v", toolOutput)
	}
	if got := len([]rune(toolOutput.Content)); got > maxToolOutputChars {
		t.Errorf("tool output length = %d, exceeds cap %d", got, maxToolOutputChars)
	}
	if !strings.HasSuffix(toolOutput.Content, "...") {
		t.Errorf("truncated output = %q, want truncation marker", toolOutput.Content)
	}
}

func TestSessionToolTimeoutCancelsContextAwareExecutor(t *testing.T) {
	session := &Session{
		model:       oneResponseModel{items: []ResponseItem{{Kind: "tool_call", Tool: "wait", CallID: "call_timeout"}}},
		tools:       &ToolRegistry{executors: map[string]ToolExecutor{"wait": contextWaitingExecutor{}}},
		toolTimeout: 10 * time.Millisecond,
	}
	turn := session.runTurn(context.Background())
	if got, want := len(turn.toolResults), 1; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
	result := turn.toolResults[0]
	if !result.IsError || result.CallID != "call_timeout" || result.Output != context.DeadlineExceeded.Error() {
		t.Fatalf("unexpected timeout result: %#v", result)
	}
}
