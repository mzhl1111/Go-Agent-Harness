package main

// buildToolCall separates protocol/item validation from tool dispatch.
func buildToolCall(item ResponseItem) (*ToolCall, *ItemError) {
	switch item.Kind {
	case "text":
		return nil, nil
	case "tool_call":
		if item.Tool == "" {
			return nil, &ItemError{Kind: ItemRespondToModel, Message: "tool call is missing a tool name"}
		}
		if item.CallID == "" {
			return nil, &ItemError{Kind: ItemRespondToModel, Message: "tool call is missing a call ID"}
		}
		return &ToolCall{Name: item.Tool, Input: item.Input, ID: item.CallID, Source: ToolCallSource{Kind: ToolCallDirect}}, nil
	default:
		return nil, &ItemError{Kind: ItemFatal, Message: "unsupported completed response item: " + item.Kind}
	}
}
