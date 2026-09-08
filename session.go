package main

import (
	"context"
	"time"
)

const maxToolOutputChars = 24

type TurnContext struct {
	toolResults              []ToolResult
	pendingApprovals         []ApprovalRequest
	history                  []HistoryItem
	streamedAssistantText    map[string]string
	streamedToolInputs       map[string]string
	lastResponseHadToolCalls bool
	needsFollowUp            bool
	followUpReason           string
	streamFailure            *StreamFailure
	fatalError               *ItemError
	cancellationErr          error
}

type StopHook func(*TurnContext)

type Session struct {
	model        Model
	tools        *ToolRegistry
	stopHooks    []StopHook
	initialInput string
	toolTimeout  time.Duration
	streamRetry  RetryPolicy
	observer     TurnObserver
}

func (s *Session) handleOutputItemDone(ctx context.Context, turn *TurnContext, item ResponseItem) OutputItemResult {
	call, itemError := buildToolCall(item)
	if itemError != nil {
		if itemError.Kind == ItemRespondToModel {
			turn.history = append(turn.history, HistoryItem{Role: "tool_error", CallID: item.CallID, Content: itemError.Message})
			return OutputItemResult{NeedsFollowUp: true}
		}
		return OutputItemResult{FatalError: itemError}
	}
	if call == nil {
		turn.history = append(turn.history, HistoryItem{Role: "assistant", Content: item.Text})
		s.emit(TurnEvent{Kind: TurnItemCompleted, ItemID: item.ID, Content: item.Text})
		return OutputItemResult{}
	}
	turn.history = append(turn.history, HistoryItem{Role: "assistant_tool_call", CallID: call.ID, Content: call.Name + " " + call.Input})
	s.emit(TurnEvent{Kind: TurnItemCompleted, CallID: call.ID, Tool: call.Name, Content: call.Input})
	future := s.startTool(ctx, *call, false)
	return OutputItemResult{ToolFuture: &future, NeedsFollowUp: true}
}

func (s *Session) runTurn(ctx context.Context) *TurnContext {
	turn := &TurnContext{
		history:               []HistoryItem{{Role: "user", Content: s.initialInput}},
		streamedAssistantText: make(map[string]string),
		streamedToolInputs:    make(map[string]string),
	}
	return s.continueTurn(ctx, turn)
}

func (s *Session) continueTurn(ctx context.Context, turn *TurnContext) *TurnContext {
	for {
		toolFutures, needsFollowUp, streamFailure, fatalError := s.streamResponse(ctx, turn)
		turn.lastResponseHadToolCalls = len(toolFutures) > 0
		turn.needsFollowUp = turn.needsFollowUp || needsFollowUp
		s.drainToolFutures(turn, toolFutures)
		// The parent context owns this complete turn, including every future it
		// started. We first drain those futures so their cancellation outcomes are
		// recorded, then stop before asking the model for another response.
		if err := ctx.Err(); err != nil {
			turn.cancellationErr = err
			s.emit(TurnEvent{Kind: TurnCancelled, Message: err.Error()})
			return turn
		}
		if streamFailure != nil {
			turn.streamFailure = streamFailure
			s.emit(TurnEvent{Kind: TurnStreamFailed, Message: streamFailure.Message})
			return turn
		}
		if fatalError != nil {
			turn.fatalError = fatalError
			s.emit(TurnEvent{Kind: TurnFatal, Message: fatalError.Message})
			return turn
		}
		for _, hook := range s.stopHooks {
			hook(turn)
		}
		if len(turn.pendingApprovals) > 0 {
			turn.needsFollowUp = false // Approval resume itself will request the next model response.
			return turn                // The UI can now render approval requests and wait for a user decision.
		}
		if turn.needsFollowUp {
			s.emit(TurnEvent{Kind: TurnFollowUp, Message: turn.followUpReason})
			turn.needsFollowUp = false
			continue
		}
		s.emit(TurnEvent{Kind: TurnCompleted})
		return turn
	}
}

// drainToolFutures collects tool outcomes in completed-output-item order, not
// handler completion order. That keeps model history deterministic even when
// handlers execute concurrently.
func (s *Session) drainToolFutures(turn *TurnContext, toolFutures []*ToolFuture) {
	for _, future := range toolFutures {
		outcome := <-future.result
		if future.cancel != nil {
			future.cancel()
		}
		if outcome.Approval != nil {
			turn.pendingApprovals = append(turn.pendingApprovals, *outcome.Approval)
			turn.history = append(turn.history, HistoryItem{Role: "approval", CallID: outcome.Approval.CallID, Content: outcome.Approval.Reason})
			s.emit(TurnEvent{Kind: TurnApprovalNeeded, CallID: outcome.Approval.CallID, Tool: outcome.Approval.Tool, Message: outcome.Approval.Reason})
			continue
		}
		turn.toolResults = append(turn.toolResults, *outcome.Result)
		turn.recordToolResult(*outcome.Result)
		s.emit(TurnEvent{Kind: TurnToolResult, CallID: outcome.Result.CallID, Content: outcome.Result.Output})
	}
}

func (s *Session) streamResponse(ctx context.Context, turn *TurnContext) ([]*ToolFuture, bool, *StreamFailure, *ItemError) {
	maxAttempts := s.streamRetry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for attempt := 1; ; attempt++ {
		stream := s.model.Stream(ctx, turn.modelInput())
		var toolFutures []*ToolFuture
		needsFollowUp := false
		completedItems := 0
		var fatalError *ItemError
		for event := range stream.Events {
			if fatalError != nil {
				continue
			}
			switch event.Kind {
			case ModelTextDelta:
				turn.streamedAssistantText[event.ItemID] += event.Delta
				s.emit(TurnEvent{Kind: TurnTextDelta, ItemID: event.ItemID, Content: event.Delta})
			case ModelToolInputDelta:
				turn.streamedToolInputs[event.CallID] += event.Delta
				s.emit(TurnEvent{Kind: TurnToolInputDelta, CallID: event.CallID, Content: event.Delta})
			case ModelOutputItemDone:
				completedItems++
				item := turn.finalizeStreamedAssistantText(event.Item)
				item = turn.finalizeStreamedToolInput(item)
				output := s.handleOutputItemDone(ctx, turn, item)
				needsFollowUp = needsFollowUp || output.NeedsFollowUp
				fatalError = output.FatalError
				if output.ToolFuture != nil {
					toolFutures = append(toolFutures, output.ToolFuture)
				}
			}
		}
		failure := <-stream.Err
		if fatalError != nil {
			return toolFutures, needsFollowUp, nil, fatalError
		}
		if failure == nil {
			return toolFutures, needsFollowUp, nil, nil
		}
		// Replaying a stream after a completed item could duplicate an already
		// executed side effect. Production Codex has richer history recovery;
		// this teaching version retries only before a semantic item is complete.
		if !failure.Retryable || completedItems > 0 || attempt == maxAttempts {
			return toolFutures, needsFollowUp, failure, nil
		}
		if s.streamRetry.Backoff > 0 {
			timer := time.NewTimer(s.streamRetry.Backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return toolFutures, needsFollowUp, &StreamFailure{Message: ctx.Err().Error()}, nil
			case <-timer.C:
			}
		}
	}
}

func (t *TurnContext) finalizeStreamedAssistantText(item ResponseItem) ResponseItem {
	if item.Kind != "text" {
		return item
	}
	if text, ok := t.streamedAssistantText[item.ID]; ok {
		if item.Text == "" {
			item.Text = text
		}
		delete(t.streamedAssistantText, item.ID)
	}
	return item
}

func (t *TurnContext) finalizeStreamedToolInput(item ResponseItem) ResponseItem {
	if item.Kind != "tool_call" {
		return item
	}
	if input, ok := t.streamedToolInputs[item.CallID]; ok {
		if item.Input == "" {
			item.Input = input
		}
		delete(t.streamedToolInputs, item.CallID)
	}
	return item
}

// resumeApproved runs precisely one previously suspended call, then asks the
// model for its next response with the same turn context. It never replays the
// model response that originally created the approval request.
func (s *Session) resumeApproved(ctx context.Context, turn *TurnContext, callID string) *TurnContext {
	for index, request := range turn.pendingApprovals {
		if request.CallID != callID {
			continue
		}
		call := ToolCall{Name: request.Tool, Input: request.Input, ID: request.CallID}
		future := s.startTool(ctx, call, true)
		outcome := <-future.result
		if future.cancel != nil {
			future.cancel()
		}
		turn.pendingApprovals = append(turn.pendingApprovals[:index], turn.pendingApprovals[index+1:]...)
		if outcome.Result != nil {
			turn.toolResults = append(turn.toolResults, *outcome.Result)
			turn.recordToolResult(*outcome.Result)
			s.emit(TurnEvent{Kind: TurnToolResult, CallID: outcome.Result.CallID, Content: outcome.Result.Output})
		}
		return s.continueTurn(ctx, turn)
	}
	return turn
}

func (s *Session) emit(event TurnEvent) {
	if s.observer != nil {
		s.observer.OnTurn(event)
	}
}

func (s *Session) startTool(ctx context.Context, call ToolCall, approvalGranted bool) ToolFuture {
	toolCtx := ctx
	var cancel context.CancelFunc
	if s.toolTimeout > 0 {
		toolCtx, cancel = context.WithTimeout(ctx, s.toolTimeout)
	}
	var future ToolFuture
	if approvalGranted {
		future = s.tools.StartAfterApproval(toolCtx, call)
	} else {
		future = s.tools.Start(toolCtx, call)
	}
	future.cancel = cancel
	return future
}

func (t *TurnContext) modelInput() []HistoryItem {
	return append([]HistoryItem(nil), t.history...)
}

func (t *TurnContext) recordToolResult(result ToolResult) {
	t.history = append(t.history, HistoryItem{
		Role: "tool", CallID: result.CallID, Content: truncateToolOutput(result.Output, maxToolOutputChars),
	})
}

func truncateToolOutput(output string, maxChars int) string {
	runes := []rune(output)
	if len(runes) <= maxChars {
		return output
	}
	marker := "..."
	if maxChars <= len(marker) {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-len(marker)]) + marker
}
