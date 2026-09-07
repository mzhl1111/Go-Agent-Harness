package main

import (
	"context"
	"fmt"
	"time"
)

const maxToolOutputChars = 24

type TurnContext struct {
	toolResults              []ToolResult
	pendingApprovals         []ApprovalRequest
	history                  []HistoryItem
	lastResponseHadToolCalls bool
	needsFollowUp            bool
	followUpReason           string
	streamFailure            *StreamFailure
}

type StopHook func(*TurnContext)

type Session struct {
	model        Model
	tools        *ToolRegistry
	stopHooks    []StopHook
	initialInput string
	toolTimeout  time.Duration
	streamRetry  RetryPolicy
	onTextDelta  func(string)
}

func (s *Session) handleOutputItemDone(ctx context.Context, turn *TurnContext, item ResponseItem) *ToolFuture {
	if item.Kind == "text" {
		turn.history = append(turn.history, HistoryItem{Role: "assistant", Content: item.Text})
		fmt.Println("assistant:", item.Text)
		return nil
	}
	call := ToolCall{Name: item.Tool, Input: item.Input, ID: item.CallID}
	turn.history = append(turn.history, HistoryItem{Role: "assistant_tool_call", CallID: call.ID, Content: call.Name + " " + call.Input})
	future := s.startTool(ctx, call, false)
	return &future
}

func (s *Session) runTurn(ctx context.Context) *TurnContext {
	turn := &TurnContext{history: []HistoryItem{{Role: "user", Content: s.initialInput}}}
	return s.continueTurn(ctx, turn)
}

func (s *Session) continueTurn(ctx context.Context, turn *TurnContext) *TurnContext {
	for {
		toolFutures, streamFailure := s.streamResponse(ctx, turn)
		turn.lastResponseHadToolCalls = len(toolFutures) > 0
		for _, future := range toolFutures {
			outcome := <-future.result
			if future.cancel != nil {
				future.cancel()
			}
			if outcome.Approval != nil {
				turn.pendingApprovals = append(turn.pendingApprovals, *outcome.Approval)
				turn.history = append(turn.history, HistoryItem{Role: "approval", CallID: outcome.Approval.CallID, Content: outcome.Approval.Reason})
				fmt.Printf("approval required (%s): %s\n", outcome.Approval.CallID, outcome.Approval.Reason)
				continue
			}
			turn.toolResults = append(turn.toolResults, *outcome.Result)
			turn.recordToolResult(*outcome.Result)
			fmt.Printf("tool result (%s): %s\n", outcome.Result.CallID, outcome.Result.Output)
		}
		if streamFailure != nil {
			turn.streamFailure = streamFailure
			return turn
		}
		for _, hook := range s.stopHooks {
			hook(turn)
		}
		if len(turn.pendingApprovals) > 0 {
			return turn // The UI can now render approval requests and wait for a user decision.
		}
		if turn.needsFollowUp {
			fmt.Println("stop hook requested follow-up:", turn.followUpReason)
			turn.needsFollowUp = false
			continue
		}
		return turn
	}
}

func (s *Session) streamResponse(ctx context.Context, turn *TurnContext) ([]*ToolFuture, *StreamFailure) {
	maxAttempts := s.streamRetry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for attempt := 1; ; attempt++ {
		stream := s.model.Stream(ctx, turn.modelInput())
		var toolFutures []*ToolFuture
		completedItems := 0
		for event := range stream.Events {
			switch event.Kind {
			case ModelTextDelta:
				if s.onTextDelta != nil {
					s.onTextDelta(event.Delta)
				}
			case ModelOutputItemDone:
				completedItems++
				if future := s.handleOutputItemDone(ctx, turn, event.Item); future != nil {
					toolFutures = append(toolFutures, future)
				}
			}
		}
		failure := <-stream.Err
		if failure == nil {
			return toolFutures, nil
		}
		// Replaying a stream after a completed item could duplicate an already
		// executed side effect. Production Codex has richer history recovery;
		// this teaching version retries only before a semantic item is complete.
		if !failure.Retryable || completedItems > 0 || attempt == maxAttempts {
			return toolFutures, failure
		}
		if s.streamRetry.Backoff > 0 {
			timer := time.NewTimer(s.streamRetry.Backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return toolFutures, &StreamFailure{Message: ctx.Err().Error()}
			case <-timer.C:
			}
		}
	}
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
			fmt.Printf("approved tool result (%s): %s\n", outcome.Result.CallID, outcome.Result.Output)
		}
		return s.continueTurn(ctx, turn)
	}
	return turn
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
