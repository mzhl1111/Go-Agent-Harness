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
	streamedAssistantText    map[string]string
	streamedToolInputs       map[string]string
	lastResponseHadToolCalls bool
	needsFollowUp            bool
	followUpReason           string
	streamFailure            *StreamFailure
}

type StopHook func(*TurnContext)

type Session struct {
	model            Model
	tools            *ToolRegistry
	stopHooks        []StopHook
	initialInput     string
	toolTimeout      time.Duration
	streamRetry      RetryPolicy
	onTextDelta      func(itemID, delta string)
	onToolInputDelta func(callID, delta string)
}

func (s *Session) handleOutputItemDone(ctx context.Context, turn *TurnContext, item ResponseItem) OutputItemResult {
	if item.Kind == "text" {
		turn.history = append(turn.history, HistoryItem{Role: "assistant", Content: item.Text})
		fmt.Println("assistant:", item.Text)
		return OutputItemResult{}
	}
	call := ToolCall{Name: item.Tool, Input: item.Input, ID: item.CallID}
	turn.history = append(turn.history, HistoryItem{Role: "assistant_tool_call", CallID: call.ID, Content: call.Name + " " + call.Input})
	future := s.startTool(ctx, call, false)
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
		toolFutures, needsFollowUp, streamFailure := s.streamResponse(ctx, turn)
		turn.lastResponseHadToolCalls = len(toolFutures) > 0
		turn.needsFollowUp = turn.needsFollowUp || needsFollowUp
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
			turn.needsFollowUp = false // Approval resume itself will request the next model response.
			return turn                // The UI can now render approval requests and wait for a user decision.
		}
		if turn.needsFollowUp {
			fmt.Println("follow-up requested:", turn.followUpReason)
			turn.needsFollowUp = false
			continue
		}
		return turn
	}
}

func (s *Session) streamResponse(ctx context.Context, turn *TurnContext) ([]*ToolFuture, bool, *StreamFailure) {
	maxAttempts := s.streamRetry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for attempt := 1; ; attempt++ {
		stream := s.model.Stream(ctx, turn.modelInput())
		var toolFutures []*ToolFuture
		needsFollowUp := false
		completedItems := 0
		for event := range stream.Events {
			switch event.Kind {
			case ModelTextDelta:
				turn.streamedAssistantText[event.ItemID] += event.Delta
				if s.onTextDelta != nil {
					s.onTextDelta(event.ItemID, event.Delta)
				}
			case ModelToolInputDelta:
				turn.streamedToolInputs[event.CallID] += event.Delta
				if s.onToolInputDelta != nil {
					s.onToolInputDelta(event.CallID, event.Delta)
				}
			case ModelOutputItemDone:
				completedItems++
				item := turn.finalizeStreamedAssistantText(event.Item)
				item = turn.finalizeStreamedToolInput(item)
				output := s.handleOutputItemDone(ctx, turn, item)
				needsFollowUp = needsFollowUp || output.NeedsFollowUp
				if output.ToolFuture != nil {
					toolFutures = append(toolFutures, output.ToolFuture)
				}
			}
		}
		failure := <-stream.Err
		if failure == nil {
			return toolFutures, needsFollowUp, nil
		}
		// Replaying a stream after a completed item could duplicate an already
		// executed side effect. Production Codex has richer history recovery;
		// this teaching version retries only before a semantic item is complete.
		if !failure.Retryable || completedItems > 0 || attempt == maxAttempts {
			return toolFutures, needsFollowUp, failure
		}
		if s.streamRetry.Backoff > 0 {
			timer := time.NewTimer(s.streamRetry.Backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return toolFutures, needsFollowUp, &StreamFailure{Message: ctx.Err().Error()}
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
