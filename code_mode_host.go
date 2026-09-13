package main

import (
	"context"
	"fmt"
	"sync"
)

// HostToolCall is the host-facing form of one nested code-mode invocation.
// InvocationID belongs to the host protocol; RuntimeToolCallID belongs to the
// code runtime. They are deliberately separate identities.
type HostToolCall struct {
	InvocationID      string
	CellID            string
	RuntimeToolCallID string
	Tool              string
	Input             string
}

// HostToolCompletion is what must be returned to the host for one particular
// invocation. It intentionally carries the unchanged host correlation ID.
type HostToolCompletion struct {
	InvocationID string
	Result       ToolResult
}

// HostToolCompletionSink is the small transport seam that will later be backed
// by the real CompleteToolCall RPC.
type HostToolCompletionSink interface {
	CompleteToolCall(context.Context, HostToolCompletion) error
}

// HostToolCallOutcome separates a registry result from an approval pause. A
// paused call cannot be completed to the host until its approval is resolved.
type HostToolCallOutcome struct {
	Completion *HostToolCompletion
	Approval   *ApprovalRequest
	Err        error
}

type HostToolCallFuture struct {
	result <-chan HostToolCallOutcome
}

// HostNestedToolAdapter bridges a host-pushed nested call into the existing
// policy/registry path. It neither executes cell code nor owns an agent turn.
type HostNestedToolAdapter struct {
	tools     *ToolRegistry
	sink      HostToolCompletionSink
	mu        sync.Mutex
	pending   map[string]ToolCall
	cancelled map[string]struct{}
}

func (a *HostNestedToolAdapter) Dispatch(ctx context.Context, hostCall HostToolCall) HostToolCallFuture {
	result := make(chan HostToolCallOutcome, 1)
	if err := hostCall.validate(); err != nil {
		result <- HostToolCallOutcome{Err: err}
		return HostToolCallFuture{result: result}
	}
	if a.tools == nil {
		result <- HostToolCallOutcome{Err: fmt.Errorf("code-mode tool registry is not configured")}
		return HostToolCallFuture{result: result}
	}
	if a.isCancelled(hostCall.InvocationID) {
		result <- HostToolCallOutcome{Err: fmt.Errorf("host tool call is cancelled: %s", hostCall.InvocationID)}
		return HostToolCallFuture{result: result}
	}
	call := hostCall.toolCall()
	future := a.tools.Start(ctx, call)
	go func() {
		outcome := <-future.result
		if outcome.Approval != nil {
			if err := a.savePending(hostCall.InvocationID, call); err != nil {
				result <- HostToolCallOutcome{Err: err}
				return
			}
			result <- HostToolCallOutcome{Approval: outcome.Approval}
			return
		}
		result <- a.complete(ctx, hostCall.InvocationID, outcome)
	}()
	return HostToolCallFuture{result: result}
}

// CancelInvocation records a host-side cancellation. A gRPC session event may
// arrive before its ToolCall subscription item, so the cancellation remains
// recorded and prevents a later dispatch from reaching the executor.
//
// This teaching adapter owns only approval-paused calls; cancelling an already
// running executor requires propagating a per-invocation runtime context and
// is intentionally a later concern.
func (a *HostNestedToolAdapter) CancelInvocation(invocationID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelled == nil {
		a.cancelled = make(map[string]struct{})
	}
	a.cancelled[invocationID] = struct{}{}
	delete(a.pending, invocationID)
}

// ResumeApproved starts exactly the host call that was paused for approval.
// It uses StartAfterApproval, so policy hooks are not asked to approve it a
// second time, and it completes the original host invocation ID.
func (a *HostNestedToolAdapter) ResumeApproved(ctx context.Context, invocationID string) HostToolCallFuture {
	result := make(chan HostToolCallOutcome, 1)
	call, ok := a.takePending(invocationID)
	if !ok {
		result <- HostToolCallOutcome{Err: fmt.Errorf("no pending host tool call for invocation ID: %s", invocationID)}
		return HostToolCallFuture{result: result}
	}
	future := a.tools.StartAfterApproval(ctx, call)
	go func() {
		result <- a.complete(ctx, invocationID, <-future.result)
	}()
	return HostToolCallFuture{result: result}
}

func (a *HostNestedToolAdapter) complete(ctx context.Context, invocationID string, outcome ToolDispatchOutcome) HostToolCallOutcome {
	if outcome.Result == nil {
		return HostToolCallOutcome{Err: fmt.Errorf("host tool call returned no result")}
	}
	completion := HostToolCompletion{InvocationID: invocationID, Result: *outcome.Result}
	if a.sink == nil {
		return HostToolCallOutcome{Err: fmt.Errorf("code-mode host completion sink is not configured")}
	}
	if err := a.sink.CompleteToolCall(ctx, completion); err != nil {
		return HostToolCallOutcome{Err: err}
	}
	return HostToolCallOutcome{Completion: &completion}
}

func (a *HostNestedToolAdapter) savePending(invocationID string, call ToolCall) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, cancelled := a.cancelled[invocationID]; cancelled {
		return fmt.Errorf("host tool call is cancelled: %s", invocationID)
	}
	if a.pending == nil {
		a.pending = make(map[string]ToolCall)
	}
	if _, exists := a.pending[invocationID]; exists {
		return fmt.Errorf("host tool call is already pending: %s", invocationID)
	}
	a.pending[invocationID] = call
	return nil
}

func (a *HostNestedToolAdapter) isCancelled(invocationID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, cancelled := a.cancelled[invocationID]
	return cancelled
}

func (a *HostNestedToolAdapter) takePending(invocationID string) (ToolCall, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	call, ok := a.pending[invocationID]
	if ok {
		delete(a.pending, invocationID)
	}
	return call, ok
}

func (call HostToolCall) toolCall() ToolCall {
	return ToolCall{
		Name:  call.Tool,
		Input: call.Input,
		ID:    fmt.Sprintf("%s_%s", call.CellID, call.RuntimeToolCallID),
		Source: ToolCallSource{
			Kind:              ToolCallCodeMode,
			CellID:            call.CellID,
			RuntimeToolCallID: call.RuntimeToolCallID,
			HostInvocationID:  call.InvocationID,
		},
	}
}

func (call HostToolCall) validate() error {
	switch {
	case call.InvocationID == "":
		return fmt.Errorf("host tool call is missing invocation ID")
	case call.CellID == "":
		return fmt.Errorf("host tool call is missing cell ID")
	case call.RuntimeToolCallID == "":
		return fmt.Errorf("host tool call is missing runtime tool call ID")
	case call.Tool == "":
		return fmt.Errorf("host tool call is missing tool name")
	default:
		return nil
	}
}
