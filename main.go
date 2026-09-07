package main

import (
	"context"
	"fmt"
	"strings"
)

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

// ToolExecutor is the reusable tool contract (like codex-tools).
// Session, approvals, looping, and hooks deliberately do not live here.
type ToolExecutor interface {
	Handle(context.Context, string) ToolResult
}

// ExecCommandHandler is one concrete executor. It knows only how to execute
// an "echo" command; it knows nothing about turns or model responses.
type ExecCommandHandler struct{}

func (ExecCommandHandler) Handle(_ context.Context, input string) ToolResult {
	parts := strings.Fields(input)
	if len(parts) == 0 || parts[0] != "echo" {
		return ToolResult{IsError: true, Output: "only `echo ...` is allowed in this demo"}
	}
	return ToolResult{Output: strings.Join(parts[1:], " ")}
}

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

type ToolCall struct {
	Name, Input, ID string
}

// ToolFuture represents a started tool call whose result will be available later.
type ToolFuture struct {
	call   ToolCall
	result <-chan ToolDispatchOutcome
}

type ApprovalRequest struct {
	CallID, Tool, Input, Reason string
}

// ToolDispatchOutcome makes "do not run the handler" explicit.
type ToolDispatchOutcome struct {
	Result   *ToolResult
	Approval *ApprovalRequest
}

// ToolRegistry is the orchestration boundary around reusable executors.
type ToolRegistry struct {
	executors map[string]ToolExecutor
	pre       []PreHook
	post      []PostHook
}

func (r *ToolRegistry) dispatchAnyWithTerminalOutcome(ctx context.Context, call ToolCall) ToolDispatchOutcome {
	for _, hook := range r.pre {
		switch outcome := hook(call); outcome.Decision {
		case PreHookBlocked:
			return ToolDispatchOutcome{Result: &ToolResult{CallID: call.ID, IsError: true, Output: outcome.Reason}}
		case PreHookNeedsApproval:
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
	go func() { result <- r.dispatchAnyWithTerminalOutcome(ctx, call) }()
	return ToolFuture{call: call, result: result}
}

type TurnContext struct {
	toolResults      []ToolResult
	pendingApprovals []ApprovalRequest
	needsFollowUp    bool
	followUpReason   string
}

type StopHook func(*TurnContext)

type Model interface {
	Next(*TurnContext) []ResponseItem
}

// ScriptedModel stands in for the Responses API. Each loop iteration obtains
// one response; tool results accumulated in TurnContext are its next context.
type ScriptedModel struct{ responseNumber int }

func (m *ScriptedModel) Next(_ *TurnContext) []ResponseItem {
	m.responseNumber++
	if m.responseNumber == 1 {
		return []ResponseItem{
			{Kind: "tool_call", Tool: "exec_command", Input: "echo hello", CallID: "call_1"},
			{Kind: "tool_call", Tool: "exec_command", Input: "echo gated", CallID: "call_2"},
		}
	}
	return []ResponseItem{{Kind: "text", Text: "call_1 completed; call_2 is waiting for approval."}}
}

type Session struct {
	model     Model
	tools     *ToolRegistry
	stopHooks []StopHook
}

func (s *Session) handleOutputItemDone(ctx context.Context, item ResponseItem) *ToolFuture {
	if item.Kind == "text" {
		fmt.Println("assistant:", item.Text)
		return nil
	}
	call := ToolCall{Name: item.Tool, Input: item.Input, ID: item.CallID}
	future := s.tools.Start(ctx, call)
	return &future
}

func (s *Session) runTurn(ctx context.Context) *TurnContext {
	turn := &TurnContext{}
	for {
		var toolFutures []*ToolFuture
		for _, item := range s.model.Next(turn) {
			if future := s.handleOutputItemDone(ctx, item); future != nil {
				toolFutures = append(toolFutures, future)
			}
		}
		// Every call is already running. Await in model-item order so the next
		// response receives deterministic, CallID-associated tool outputs.
		for _, future := range toolFutures {
			outcome := <-future.result
			if outcome.Approval != nil {
				turn.pendingApprovals = append(turn.pendingApprovals, *outcome.Approval)
				fmt.Printf("approval required (%s): %s\n", outcome.Approval.CallID, outcome.Approval.Reason)
				continue
			}
			turn.toolResults = append(turn.toolResults, *outcome.Result)
			fmt.Printf("tool result (%s): %s\n", outcome.Result.CallID, outcome.Result.Output)
		}
		for _, hook := range s.stopHooks {
			hook(turn)
		}
		if turn.needsFollowUp {
			fmt.Println("stop hook requested follow-up:", turn.followUpReason)
			turn.needsFollowUp = false
			continue // the crucial re-entry into the outer turn loop
		}
		return turn
	}
}

func main() {
	tools := &ToolRegistry{
		executors: map[string]ToolExecutor{"exec_command": ExecCommandHandler{}},
		pre: []PreHook{func(c ToolCall) PreHookOutcome {
			fmt.Println("pre hook:", c.Name)
			if c.Input == "echo gated" {
				return PreHookOutcome{Decision: PreHookNeedsApproval, Reason: "demo policy requires approval"}
			}
			return PreHookOutcome{Decision: PreHookContinue}
		}},
		post: []PostHook{func(c ToolCall, r ToolResult) { fmt.Println("post hook:", c.ID, "error=", r.IsError) }},
	}
	session := &Session{model: &ScriptedModel{}, tools: tools,
		stopHooks: []StopHook{func(t *TurnContext) {
			// A stop hook sees all accumulated turn state on every loop pass.
			// Marking its decision prevents the same result from requesting an
			// unbounded series of follow-up responses.
			if len(t.toolResults)+len(t.pendingApprovals) == 2 && t.followUpReason == "" {
				t.needsFollowUp, t.followUpReason = true, "send tool result back to model"
			}
		}},
	}
	session.runTurn(context.Background())
}
