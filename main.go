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

type PreHook func(ToolCall) error
type PostHook func(ToolCall, ToolResult)

type ToolCall struct {
	Name, Input, ID string
}

// ToolRegistry is the orchestration boundary around reusable executors.
type ToolRegistry struct {
	executors map[string]ToolExecutor
	pre       []PreHook
	post      []PostHook
}

func (r *ToolRegistry) DispatchAnyWithTerminalOutcome(ctx context.Context, call ToolCall) ToolResult {
	for _, hook := range r.pre {
		if err := hook(call); err != nil {
			return ToolResult{IsError: true, Output: err.Error()}
		}
	}
	executor, ok := r.executors[call.Name]
	if !ok {
		return ToolResult{IsError: true, Output: "unknown tool: " + call.Name}
	}
	result := executor.Handle(ctx, call.Input)
	for _, hook := range r.post {
		hook(call, result)
	}
	return result
}

type TurnContext struct {
	toolResults    []ToolResult
	needsFollowUp  bool
	followUpReason string
}

type StopHook func(*TurnContext)

// ScriptedModel stands in for the Responses API. Each loop iteration obtains
// one response; tool results accumulated in TurnContext are its next context.
type ScriptedModel struct{ responseNumber int }

func (m *ScriptedModel) Next(_ *TurnContext) []ResponseItem {
	m.responseNumber++
	if m.responseNumber == 1 {
		return []ResponseItem{{Kind: "tool_call", Tool: "exec_command", Input: "echo hello harness", CallID: "call_1"}}
	}
	return []ResponseItem{{Kind: "text", Text: "The executor returned its result; the turn is complete."}}
}

type Session struct {
	model     *ScriptedModel
	tools     *ToolRegistry
	stopHooks []StopHook
}

func (s *Session) handleOutputItemDone(ctx context.Context, turn *TurnContext, item ResponseItem) {
	if item.Kind == "text" {
		fmt.Println("assistant:", item.Text)
		return
	}
	call := ToolCall{Name: item.Tool, Input: item.Input, ID: item.CallID}
	result := s.tools.DispatchAnyWithTerminalOutcome(ctx, call)
	turn.toolResults = append(turn.toolResults, result) // equivalent to completing tool_future
	fmt.Println("tool result:", result.Output)
}

func (s *Session) runTurn(ctx context.Context) {
	turn := &TurnContext{}
	for {
		for _, item := range s.model.Next(turn) {
			s.handleOutputItemDone(ctx, turn, item)
		}
		for _, hook := range s.stopHooks {
			hook(turn)
		}
		if turn.needsFollowUp {
			fmt.Println("stop hook requested follow-up:", turn.followUpReason)
			turn.needsFollowUp = false
			continue // the crucial re-entry into the outer turn loop
		}
		return
	}
}

func main() {
	tools := &ToolRegistry{
		executors: map[string]ToolExecutor{"exec_command": ExecCommandHandler{}},
		pre:       []PreHook{func(c ToolCall) error { fmt.Println("pre hook:", c.Name); return nil }},
		post:      []PostHook{func(c ToolCall, r ToolResult) { fmt.Println("post hook:", c.ID, "error=", r.IsError) }},
	}
	session := &Session{model: &ScriptedModel{}, tools: tools,
		stopHooks: []StopHook{func(t *TurnContext) {
			// A stop hook sees all accumulated turn state on every loop pass.
			// Marking its decision prevents the same result from requesting an
			// unbounded series of follow-up responses.
			if len(t.toolResults) == 1 && t.followUpReason == "" {
				t.needsFollowUp, t.followUpReason = true, "send tool result back to model"
			}
		}},
	}
	session.runTurn(context.Background())
}
