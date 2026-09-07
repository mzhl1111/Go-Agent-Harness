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
}

func (r *ToolRegistry) dispatchAnyWithTerminalOutcome(ctx context.Context, call ToolCall, approvalGranted bool) ToolDispatchOutcome {
	for _, hook := range r.pre {
		switch outcome := hook(call); outcome.Decision {
		case PreHookBlocked:
			return ToolDispatchOutcome{Result: &ToolResult{CallID: call.ID, IsError: true, Output: outcome.Reason}}
		case PreHookNeedsApproval:
			if approvalGranted {
				continue
			}
			return ToolDispatchOutcome{Approval: &ApprovalRequest{
				CallID: call.ID, Tool: call.Name, Input: call.Input, Reason: outcome.Reason,
			}}
		}
	}
	executor, ok := r.executors[call.Name]
	if !ok {
		result := ToolResult{CallID: call.ID, IsError: true, Output: "unknown tool: " + call.Name}
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
			return ToolDispatchOutcome{Result: &result}
		}
		if r.retry.Backoff > 0 {
			timer := time.NewTimer(r.retry.Backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				result := ToolResult{CallID: call.ID, IsError: true, Output: ctx.Err().Error(), Attempts: attempt}
				return ToolDispatchOutcome{Result: &result}
			case <-timer.C:
			}
		}
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
