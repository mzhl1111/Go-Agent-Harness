package main

import "context"

// ResponseItem is the small subset of a model response that our harness needs.
type ResponseItem struct {
	ID     string
	Kind   string // "text" or "tool_call"
	Text   string
	Tool   string
	Input  string
	CallID string
}

type ToolResult struct {
	CallID    string
	Output    string
	IsError   bool
	Retryable bool
	Attempts  int
}

type HistoryItem struct {
	Role, Content, CallID string
}

type ToolCall struct {
	Name, Input, ID string
}

type ItemErrorKind string

const (
	ItemRespondToModel ItemErrorKind = "respond_to_model"
	ItemFatal          ItemErrorKind = "fatal"
)

type ItemError struct {
	Kind    ItemErrorKind
	Message string
}

type ApprovalRequest struct {
	CallID, Tool, Input, Reason string
}

// ToolExecutor is the reusable tool contract (like codex-tools).
// Session, approvals, looping, and hooks deliberately do not live here.
type ToolExecutor interface {
	Handle(context.Context, string) ToolResult
}

// ParallelToolExecutor is an opt-in capability. Executors that do not
// implement it are serialized by tool name.
type ParallelToolExecutor interface {
	ToolExecutor
	SupportsParallelToolCalls() bool
}

// ToolFuture represents a started tool call whose result will be available later.
type ToolFuture struct {
	call   ToolCall
	result <-chan ToolDispatchOutcome
	cancel func()
}

// OutputItemResult reports the runtime consequences of one completed model item.
type OutputItemResult struct {
	ToolFuture    *ToolFuture
	NeedsFollowUp bool
	FatalError    *ItemError
}

// ToolDispatchOutcome makes "do not run the handler" explicit.
type ToolDispatchOutcome struct {
	Result   *ToolResult
	Approval *ApprovalRequest
}

type DispatchEventKind string

const (
	DispatchStarted         DispatchEventKind = "started"
	DispatchBlocked         DispatchEventKind = "blocked"
	DispatchWaitingApproval DispatchEventKind = "waiting_approval"
	DispatchRetrying        DispatchEventKind = "retrying"
	DispatchCompleted       DispatchEventKind = "completed"
	DispatchFailed          DispatchEventKind = "failed"
)

type DispatchEvent struct {
	Kind    DispatchEventKind
	CallID  string
	Tool    string
	Attempt int
	Message string
}

// DispatchObserver receives lifecycle notifications. Implementations must be
// safe for concurrent calls because each tool future runs in its own goroutine.
type DispatchObserver interface {
	OnDispatch(DispatchEvent)
}
