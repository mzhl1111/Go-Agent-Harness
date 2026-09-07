package main

import "context"

// ResponseItem is the small subset of a model response that our harness needs.
type ResponseItem struct {
	Kind   string // "text" or "tool_call"
	Text   string
	Tool   string
	Input  string
	CallID string
}

type ToolResult struct {
	CallID  string
	Output  string
	IsError bool
}

type HistoryItem struct {
	Role, Content, CallID string
}

type ToolCall struct {
	Name, Input, ID string
}

type ApprovalRequest struct {
	CallID, Tool, Input, Reason string
}

// ToolExecutor is the reusable tool contract (like codex-tools).
// Session, approvals, looping, and hooks deliberately do not live here.
type ToolExecutor interface {
	Handle(context.Context, string) ToolResult
}

// ToolFuture represents a started tool call whose result will be available later.
type ToolFuture struct {
	call   ToolCall
	result <-chan ToolDispatchOutcome
	cancel func()
}

// ToolDispatchOutcome makes "do not run the handler" explicit.
type ToolDispatchOutcome struct {
	Result   *ToolResult
	Approval *ApprovalRequest
}
