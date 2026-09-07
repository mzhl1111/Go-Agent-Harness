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
	session := &Session{model: &ScriptedModel{}, tools: tools, initialInput: "Run the demo tools.",
		stopHooks: []StopHook{requestFollowUpAfterAllTools},
	}
	turn := session.runTurn(context.Background())
	if len(turn.pendingApprovals) > 0 {
		session.resumeApproved(context.Background(), turn, turn.pendingApprovals[0].CallID)
	}
}

func requestFollowUpAfterAllTools(turn *TurnContext) {
	if turn.lastResponseHadToolCalls && len(turn.toolResults) == 2 && len(turn.pendingApprovals) == 0 && turn.followUpReason == "" {
		turn.needsFollowUp, turn.followUpReason = true, "send tool result back to model"
	}
}
