package main

import (
	"context"
	"strings"
)

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
