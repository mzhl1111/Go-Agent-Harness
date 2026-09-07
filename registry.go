package main

import "context"

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
	executors map[string]ToolExecutor
	pre       []PreHook
	post      []PostHook
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
	result := executor.Handle(ctx, call.Input)
	result.CallID = call.ID
	for _, hook := range r.post {
		hook(call, result)
	}
	return ToolDispatchOutcome{Result: &result}
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
