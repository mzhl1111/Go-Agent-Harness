package main

import (
	"context"
	"time"
)

type PreHookDecision int

const (
	PreHookContinue PreHookDecision = iota
	PreHookBlocked
	PreHookNeedsApproval
)

type PreHookOutcome struct {
	Decision PreHookDecision
	Reason   string
}

type PreHook func(ToolCall) PreHookOutcome
type PostHook func(ToolCall, ToolResult)

type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

// ToolRegistry is the orchestration boundary around reusable executors.
type ToolRegistry struct {
	executors map[string]ToolExecutor
	pre       []PreHook
	post      []PostHook
	retry     RetryPolicy
	observer  DispatchObserver
}

func (r *ToolRegistry) dispatchAnyWithTerminalOutcome(ctx context.Context, call ToolCall, approvalGranted bool) ToolDispatchOutcome {
	r.emit(DispatchEvent{Kind: DispatchStarted, CallID: call.ID, Tool: call.Name})
	for _, hook := range r.pre {
		switch outcome := hook(call); outcome.Decision {
		case PreHookBlocked:
			r.emit(DispatchEvent{Kind: DispatchBlocked, CallID: call.ID, Tool: call.Name, Message: outcome.Reason})
			return ToolDispatchOutcome{Result: &ToolResult{CallID: call.ID, IsError: true, Output: outcome.Reason}}
		case PreHookNeedsApproval:
			if approvalGranted {
				continue
			}
			r.emit(DispatchEvent{Kind: DispatchWaitingApproval, CallID: call.ID, Tool: call.Name, Message: outcome.Reason})
			return ToolDispatchOutcome{Approval: &ApprovalRequest{
				CallID: call.ID, Tool: call.Name, Input: call.Input, Reason: outcome.Reason,
			}}
		}
	}
	executor, ok := r.executors[call.Name]
	if !ok {
		result := ToolResult{CallID: call.ID, IsError: true, Output: "unknown tool: " + call.Name}
		r.emit(DispatchEvent{Kind: DispatchFailed, CallID: call.ID, Tool: call.Name, Message: result.Output})
		return ToolDispatchOutcome{Result: &result}
	}
	maxAttempts := r.retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for attempt := 1; ; attempt++ {
		result := executor.Handle(ctx, call.Input)
		result.CallID, result.Attempts = call.ID, attempt
		if !result.IsError || !result.Retryable || attempt == maxAttempts {
			for _, hook := range r.post {
				hook(call, result)
			}
			kind := DispatchCompleted
			if result.IsError {
				kind = DispatchFailed
			}
			r.emit(DispatchEvent{Kind: kind, CallID: call.ID, Tool: call.Name, Attempt: attempt, Message: result.Output})
			return ToolDispatchOutcome{Result: &result}
		}
		r.emit(DispatchEvent{Kind: DispatchRetrying, CallID: call.ID, Tool: call.Name, Attempt: attempt, Message: result.Output})
		if r.retry.Backoff > 0 {
			timer := time.NewTimer(r.retry.Backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				result := ToolResult{CallID: call.ID, IsError: true, Output: ctx.Err().Error(), Attempts: attempt}
				r.emit(DispatchEvent{Kind: DispatchFailed, CallID: call.ID, Tool: call.Name, Attempt: attempt, Message: result.Output})
				return ToolDispatchOutcome{Result: &result}
			case <-timer.C:
			}
		}
	}
}

func (r *ToolRegistry) emit(event DispatchEvent) {
	if r.observer != nil {
		r.observer.OnDispatch(event)
	}
}

func (r *ToolRegistry) Start(ctx context.Context, call ToolCall) ToolFuture {
	result := make(chan ToolDispatchOutcome, 1)
	go func() { result <- r.dispatchAnyWithTerminalOutcome(ctx, call, false) }()
	return ToolFuture{call: call, result: result}
}

func (r *ToolRegistry) StartAfterApproval(ctx context.Context, call ToolCall) ToolFuture {
	result := make(chan ToolDispatchOutcome, 1)
	go func() { result <- r.dispatchAnyWithTerminalOutcome(ctx, call, true) }()
	return ToolFuture{call: call, result: result}
}
