package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingExecutor struct {
	started chan string
	release <-chan struct{}
}

func (blockingExecutor) SupportsParallelToolCalls() bool { return true }

type serialBlockingExecutor struct {
	started chan string
	release <-chan struct{}
}

func (e serialBlockingExecutor) Handle(_ context.Context, input string) ToolResult {
	e.started <- input
	<-e.release
	return ToolResult{Output: input}
}

type countingExecutor struct{ calls int }

type contextWaitingExecutor struct{}

type cancellationRecordingExecutor struct {
	started chan struct{}
}

type flakyExecutor struct {
	failuresBeforeSuccess int
	calls                 int
}

func (e *flakyExecutor) Handle(_ context.Context, _ string) ToolResult {
	e.calls++
	if e.calls <= e.failuresBeforeSuccess {
		return ToolResult{IsError: true, Retryable: true, Output: "temporary failure"}
	}
	return ToolResult{Output: "success"}
}

type permanentFailureExecutor struct{ calls int }

type inputRecordingExecutor struct{ input string }

func (e *inputRecordingExecutor) Handle(_ context.Context, input string) ToolResult {
	e.input = input
	return ToolResult{Output: input}
}

type recordingObserver struct {
	mu     sync.Mutex
	events []DispatchEvent
}

type recordingTurnObserver struct {
	events []TurnEvent
}

func (o *recordingTurnObserver) OnTurn(event TurnEvent) {
	o.events = append(o.events, event)
}

func (o *recordingTurnObserver) contents(kind TurnEventKind) []string {
	var contents []string
	for _, event := range o.events {
		if event.Kind == kind {
			contents = append(contents, event.Content)
		}
	}
	return contents
}

func (o *recordingObserver) OnDispatch(event DispatchEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)
}

func (o *recordingObserver) snapshot() []DispatchEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]DispatchEvent(nil), o.events...)
}

func (e *permanentFailureExecutor) Handle(_ context.Context, _ string) ToolResult {
	e.calls++
	return ToolResult{IsError: true, Output: "permanent failure"}
}

func (contextWaitingExecutor) Handle(ctx context.Context, _ string) ToolResult {
	<-ctx.Done()
	return ToolResult{IsError: true, Output: ctx.Err().Error()}
}

func (e cancellationRecordingExecutor) Handle(ctx context.Context, _ string) ToolResult {
	close(e.started)
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

type oneResponseModel struct {
	items []ResponseItem
	sent  bool
	calls int
}

func (m *oneResponseModel) Stream(ctx context.Context, _ []HistoryItem) ModelStream {
	m.calls++
	if m.sent {
		return modelEventStream(ctx)
	}
	m.sent = true
	events := make([]ModelEvent, 0, len(m.items))
	for _, item := range m.items {
		events = append(events, ModelEvent{Kind: ModelOutputItemDone, Item: item})
	}
	return modelEventStream(ctx, events...)
}

type historyRecordingModel struct {
	inputs         [][]HistoryItem
	responseNumber int
}

func (m *historyRecordingModel) Stream(ctx context.Context, input []HistoryItem) ModelStream {
	m.inputs = append(m.inputs, append([]HistoryItem(nil), input...))
	m.responseNumber++
	if m.responseNumber == 1 {
		return modelEventStream(ctx,
			ModelEvent{Kind: ModelTextDelta, ItemID: "draft_1", Delta: "draft text"},
			ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "tool_call", Tool: "echo", Input: "a very long tool result for context", CallID: "call_1"}},
		)
	}
	return modelEventStream(ctx, ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "text", Text: "done"}})
}

type flakyStreamModel struct {
	attempts          int
	failAfterToolCall bool
}

type toolInputDeltaModel struct{ sent bool }

func (m *toolInputDeltaModel) Stream(ctx context.Context, _ []HistoryItem) ModelStream {
	if m.sent {
		return modelEventStream(ctx)
	}
	m.sent = true
	return modelEventStream(ctx,
		ModelEvent{Kind: ModelToolInputDelta, CallID: "call_delta", Delta: "echo "},
		ModelEvent{Kind: ModelToolInputDelta, CallID: "call_delta", Delta: "assembled"},
		ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "tool_call", Tool: "exec", CallID: "call_delta"}},
	)
}

type streamedTextModel struct{}

func (streamedTextModel) Stream(ctx context.Context, _ []HistoryItem) ModelStream {
	return modelEventStream(ctx,
		ModelEvent{Kind: ModelTextDelta, ItemID: "message_1", Delta: "Hello, "},
		ModelEvent{Kind: ModelTextDelta, ItemID: "message_1", Delta: "streaming world."},
		ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{ID: "message_1", Kind: "text"}},
	)
}

func (m *flakyStreamModel) Stream(ctx context.Context, _ []HistoryItem) ModelStream {
	m.attempts++
	if m.attempts == 1 {
		if m.failAfterToolCall {
			return modelEventStreamWithFailure(ctx, &StreamFailure{Message: "connection lost", Retryable: true},
				ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "tool_call", Tool: "count", CallID: "call_once"}},
			)
		}
		return modelEventStreamWithFailure(ctx, &StreamFailure{Message: "connection lost", Retryable: true})
	}
	return modelEventStream(ctx, ModelEvent{Kind: ModelOutputItemDone, Item: ResponseItem{Kind: "text", Text: "reconnected"}})
}

func TestToolCallsStartConcurrentlyAndKeepModelOrder(t *testing.T) {
	release := make(chan struct{})
	started := make(chan string, 2)
	session := &Session{
		model: &oneResponseModel{items: []ResponseItem{
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

func TestToolCallsDefaultToSerialExecutionWhenExecutorDoesNotOptIn(t *testing.T) {
	release := make(chan struct{})
	started := make(chan string, 2)
	registry := &ToolRegistry{executors: map[string]ToolExecutor{
		"serial": serialBlockingExecutor{started: started, release: release},
	}}
	first := registry.Start(context.Background(), ToolCall{Name: "serial", Input: "first", ID: "call_1"})
	<-started
	second := registry.Start(context.Background(), ToolCall{Name: "serial", Input: "second", ID: "call_2"})
	select {
	case call := <-started:
		t.Fatalf("second serial call started before first completed: %q", call)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-started // The second handler can start only after the first releases the gate.
	<-first.result
	<-second.result
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
	observer := &recordingTurnObserver{}
	session := &Session{
		model:        model,
		initialInput: "test task",
		observer:     observer,
		tools:        &ToolRegistry{executors: map[string]ToolExecutor{"echo": ExecCommandHandler{}}},
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
	if got, want := strings.Join(observer.contents(TurnTextDelta), ""), "draft text"; got != want {
		t.Errorf("streamed deltas = %q, want %q", got, want)
	}
	for _, item := range secondInput {
		if item.Content == "draft text" {
			t.Error("text delta leaked into authoritative model history")
		}
	}
}

func TestSessionToolTimeoutCancelsContextAwareExecutor(t *testing.T) {
	session := &Session{
		model:       &oneResponseModel{items: []ResponseItem{{Kind: "tool_call", Tool: "wait", CallID: "call_timeout"}}},
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

func TestCancelledTurnDrainsStartedToolsButDoesNotRequestFollowUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executor := cancellationRecordingExecutor{started: make(chan struct{})}
	model := &oneResponseModel{items: []ResponseItem{{Kind: "tool_call", Tool: "wait", CallID: "call_cancel"}}}
	session := &Session{
		model: model,
		tools: &ToolRegistry{executors: map[string]ToolExecutor{"wait": executor}},
	}
	done := make(chan *TurnContext, 1)
	go func() { done <- session.runTurn(ctx) }()
	<-executor.started
	cancel()
	turn := <-done

	if turn.cancellationErr != context.Canceled {
		t.Fatalf("cancellation error = %v, want context canceled", turn.cancellationErr)
	}
	if got, want := len(turn.toolResults), 1; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
	if got, want := turn.toolResults[0].Output, context.Canceled.Error(); got != want {
		t.Errorf("cancelled tool output = %q, want %q", got, want)
	}
	if got, want := model.calls, 1; got != want {
		t.Errorf("model stream calls = %d, want %d", got, want)
	}
}

func TestRetryPolicyRetriesOnlyRetryableFailuresWithinLimit(t *testing.T) {
	executor := &flakyExecutor{failuresBeforeSuccess: 2}
	registry := &ToolRegistry{
		executors: map[string]ToolExecutor{"flaky": executor},
		retry:     RetryPolicy{MaxAttempts: 3},
	}
	result := (<-registry.Start(context.Background(), ToolCall{Name: "flaky", ID: "call_retry"}).result).Result
	if result.IsError || result.Attempts != 3 || executor.calls != 3 {
		t.Fatalf("unexpected retry result: %#v, calls=%d", result, executor.calls)
	}

	exhausted := &flakyExecutor{failuresBeforeSuccess: 3}
	registry.executors["flaky"] = exhausted
	result = (<-registry.Start(context.Background(), ToolCall{Name: "flaky", ID: "call_exhausted"}).result).Result
	if !result.IsError || result.Attempts != 3 || exhausted.calls != 3 {
		t.Fatalf("retry limit was not respected: %#v, calls=%d", result, exhausted.calls)
	}
}

func TestRetryPolicyDoesNotRetryPermanentFailure(t *testing.T) {
	executor := &permanentFailureExecutor{}
	registry := &ToolRegistry{
		executors: map[string]ToolExecutor{"permanent": executor},
		retry:     RetryPolicy{MaxAttempts: 3},
	}
	result := (<-registry.Start(context.Background(), ToolCall{Name: "permanent", ID: "call_permanent"}).result).Result
	if !result.IsError || result.Attempts != 1 || executor.calls != 1 {
		t.Fatalf("permanent failure was retried: %#v, calls=%d", result, executor.calls)
	}
}

func TestDispatchObserverReceivesRetryLifecycle(t *testing.T) {
	executor := &flakyExecutor{failuresBeforeSuccess: 1}
	observer := &recordingObserver{}
	registry := &ToolRegistry{
		executors: map[string]ToolExecutor{"flaky": executor},
		retry:     RetryPolicy{MaxAttempts: 2},
		observer:  observer,
	}
	<-registry.Start(context.Background(), ToolCall{Name: "flaky", ID: "call_observed"}).result
	events := observer.snapshot()
	if got, want := len(events), 3; got != want {
		t.Fatalf("event count = %d, want %d: %#v", got, want, events)
	}
	if events[0].Kind != DispatchStarted || events[1].Kind != DispatchRetrying || events[2].Kind != DispatchCompleted {
		t.Fatalf("unexpected event sequence: %#v", events)
	}
	if events[1].Attempt != 1 || events[2].Attempt != 2 || events[2].CallID != "call_observed" {
		t.Fatalf("unexpected event details: %#v", events)
	}
}

func TestSessionRetriesRetryableStreamBeforeAnyCompletedItem(t *testing.T) {
	model := &flakyStreamModel{}
	session := &Session{model: model, streamRetry: RetryPolicy{MaxAttempts: 2}}
	turn := session.runTurn(context.Background())
	if model.attempts != 2 || turn.streamFailure != nil {
		t.Fatalf("stream was not retried successfully: attempts=%d failure=%#v", model.attempts, turn.streamFailure)
	}
	if got, want := len(turn.history), 2; got != want { // user input + final assistant item
		t.Fatalf("history count = %d, want %d", got, want)
	}
}

func TestSessionDoesNotRetryStreamAfterCompletedToolItem(t *testing.T) {
	model := &flakyStreamModel{failAfterToolCall: true}
	executor := &countingExecutor{}
	session := &Session{
		model:       model,
		streamRetry: RetryPolicy{MaxAttempts: 2},
		tools:       &ToolRegistry{executors: map[string]ToolExecutor{"count": executor}},
	}
	turn := session.runTurn(context.Background())
	if model.attempts != 1 || executor.calls != 1 {
		t.Fatalf("completed stream item was replayed: attempts=%d executor calls=%d", model.attempts, executor.calls)
	}
	if turn.streamFailure == nil || turn.streamFailure.Message != "connection lost" {
		t.Fatalf("stream failure was not retained: %#v", turn.streamFailure)
	}
}

func TestToolInputDeltasAreAssembledOnlyWhenItemCompletes(t *testing.T) {
	executor := &inputRecordingExecutor{}
	observer := &recordingTurnObserver{}
	session := &Session{
		model:    &toolInputDeltaModel{},
		tools:    &ToolRegistry{executors: map[string]ToolExecutor{"exec": executor}},
		observer: observer,
	}
	turn := session.runTurn(context.Background())
	if got, want := executor.input, "echo assembled"; got != want {
		t.Fatalf("executor input = %q, want %q", got, want)
	}
	var deltas []string
	for _, event := range observer.events {
		if event.Kind == TurnToolInputDelta {
			deltas = append(deltas, event.CallID+":"+event.Content)
		}
	}
	if got, want := strings.Join(deltas, ","), "call_delta:echo ,call_delta:assembled"; got != want {
		t.Errorf("observed input deltas = %q, want %q", got, want)
	}
	if _, ok := turn.streamedToolInputs["call_delta"]; ok {
		t.Error("completed tool call still has a streamed input buffer")
	}
}

func TestAssistantTextDeltasAreCommittedOnlyWhenItemCompletes(t *testing.T) {
	observer := &recordingTurnObserver{}
	session := &Session{
		model:    streamedTextModel{},
		observer: observer,
	}
	turn := session.runTurn(context.Background())
	var observed []string
	for _, event := range observer.events {
		if event.Kind == TurnTextDelta {
			observed = append(observed, event.ItemID+":"+event.Content)
		}
	}
	if got, want := strings.Join(observed, ","), "message_1:Hello, ,message_1:streaming world."; got != want {
		t.Errorf("observed deltas = %q, want %q", got, want)
	}
	if got, want := turn.history[1].Content, "Hello, streaming world."; got != want {
		t.Errorf("assistant history = %q, want %q", got, want)
	}
	if _, ok := turn.streamedAssistantText["message_1"]; ok {
		t.Error("completed text item still has a streamed text buffer")
	}
}

func TestMalformedToolCallBecomesModelVisibleErrorAndFollowUp(t *testing.T) {
	session := &Session{
		model: &oneResponseModel{items: []ResponseItem{{Kind: "tool_call", CallID: "call_bad"}}},
		tools: &ToolRegistry{executors: map[string]ToolExecutor{}},
	}
	turn := session.runTurn(context.Background())
	if turn.fatalError != nil || len(turn.toolResults) != 0 {
		t.Fatalf("malformed call took the wrong path: fatal=%#v results=%#v", turn.fatalError, turn.toolResults)
	}
	if got, want := turn.history[1], (HistoryItem{Role: "tool_error", CallID: "call_bad", Content: "tool call is missing a tool name"}); got != want {
		t.Errorf("model-visible tool error = %#v, want %#v", got, want)
	}
}

func TestUnsupportedCompletedItemIsFatal(t *testing.T) {
	session := &Session{model: &oneResponseModel{items: []ResponseItem{{Kind: "unsupported"}}}}
	turn := session.runTurn(context.Background())
	if turn.fatalError == nil || turn.fatalError.Kind != ItemFatal {
		t.Fatalf("unsupported item was not fatal: %#v", turn.fatalError)
	}
}
