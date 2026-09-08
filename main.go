package main

import (
	"context"
	"fmt"
)

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
	session := &Session{model: &ScriptedModel{}, tools: tools, initialInput: "Run the demo tools."}
	turn := session.runTurn(context.Background())
	if len(turn.pendingApprovals) > 0 {
		session.resumeApproved(context.Background(), turn, turn.pendingApprovals[0].CallID)
	}
}
