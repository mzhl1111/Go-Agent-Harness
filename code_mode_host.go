package main

import (
	"context"
	"fmt"
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
	tools *ToolRegistry
	sink  HostToolCompletionSink
}

func (a HostNestedToolAdapter) Dispatch(ctx context.Context, hostCall HostToolCall) HostToolCallFuture {
	result := make(chan HostToolCallOutcome, 1)
	if err := hostCall.validate(); err != nil {
		result <- HostToolCallOutcome{Err: err}
		return HostToolCallFuture{result: result}
	}
	call := ToolCall{
		Name:  hostCall.Tool,
		Input: hostCall.Input,
		ID:    fmt.Sprintf("%s_%s", hostCall.CellID, hostCall.RuntimeToolCallID),
		Source: ToolCallSource{
			Kind:              ToolCallCodeMode,
			CellID:            hostCall.CellID,
			RuntimeToolCallID: hostCall.RuntimeToolCallID,
			HostInvocationID:  hostCall.InvocationID,
		},
	}
	future := a.tools.Start(ctx, call)
	go func() {
		outcome := <-future.result
		if outcome.Approval != nil {
			result <- HostToolCallOutcome{Approval: outcome.Approval}
			return
		}
		completion := HostToolCompletion{InvocationID: hostCall.InvocationID, Result: *outcome.Result}
		if a.sink == nil {
			result <- HostToolCallOutcome{Err: fmt.Errorf("code-mode host completion sink is not configured")}
			return
		}
		if err := a.sink.CompleteToolCall(ctx, completion); err != nil {
			result <- HostToolCallOutcome{Err: err}
			return
		}
		result <- HostToolCallOutcome{Completion: &completion}
	}()
	return HostToolCallFuture{result: result}
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
