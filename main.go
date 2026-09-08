package main

import (
	"context"
	"fmt"
)

type consoleTurnObserver struct{}

func (consoleTurnObserver) OnTurn(event TurnEvent) {
	switch event.Kind {
	case TurnTextDelta:
		fmt.Print(event.Content)
	case TurnItemCompleted:
		if event.CallID == "" {
			fmt.Println()
			fmt.Println("assistant:", event.Content)
		}
	case TurnToolResult:
		fmt.Printf("tool result (%s): %s\n", event.CallID, event.Content)
	case TurnApprovalNeeded:
		fmt.Printf("approval required (%s): %s\n", event.CallID, event.Message)
	case TurnFollowUp:
		fmt.Println("follow-up requested:", event.Message)
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
	session := &Session{model: &ScriptedModel{}, tools: tools, initialInput: "Run the demo tools.", observer: consoleTurnObserver{}}
	turn := session.runTurn(context.Background())
	if len(turn.pendingApprovals) > 0 {
		session.resumeApproved(context.Background(), turn, turn.pendingApprovals[0].CallID)
	}
}
