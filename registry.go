package main

import (
	"context"
	"sync"
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

// ToolRegistry is the orchestration boundary around reusable executors.
type ToolRegistry struct {
	executors   map[string]ToolExecutor
	pre         []PreHook
	post        []PostHook
	retry       RetryPolicy
	observer    DispatchObserver
	serialMu    sync.Mutex
	serialGates map[string]chan struct{}
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
	return r.start(ctx, call, false)
}

func (r *ToolRegistry) StartAfterApproval(ctx context.Context, call ToolCall) ToolFuture {
	return r.start(ctx, call, true)
}

func (r *ToolRegistry) start(ctx context.Context, call ToolCall, approvalGranted bool) ToolFuture {
	result := make(chan ToolDispatchOutcome, 1)
	go func() {
		if !r.supportsParallelToolCalls(call.Name) {
			gate := r.serialGate(call.Name)
			select {
			case gate <- struct{}{}:
				defer func() { <-gate }()
			case <-ctx.Done():
				failure := ToolResult{CallID: call.ID, IsError: true, Output: ctx.Err().Error()}
				r.emit(DispatchEvent{Kind: DispatchFailed, CallID: call.ID, Tool: call.Name, Message: failure.Output})
				result <- ToolDispatchOutcome{Result: &failure}
				return
			}
		}
		result <- r.dispatchAnyWithTerminalOutcome(ctx, call, approvalGranted)
	}()
	return ToolFuture{call: call, result: result}
}

func (r *ToolRegistry) supportsParallelToolCalls(name string) bool {
	executor, ok := r.executors[name]
	if !ok {
		return false
	}
	parallel, ok := executor.(ParallelToolExecutor)
	return ok && parallel.SupportsParallelToolCalls()
}

func (r *ToolRegistry) serialGate(name string) chan struct{} {
	r.serialMu.Lock()
	defer r.serialMu.Unlock()
	if r.serialGates == nil {
		r.serialGates = make(map[string]chan struct{})
	}
	if gate, ok := r.serialGates[name]; ok {
		return gate
	}
	gate := make(chan struct{}, 1)
	r.serialGates[name] = gate
	return gate
}
