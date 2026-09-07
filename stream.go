package main

import "context"

type ModelEventKind string

const (
	ModelTextDelta      ModelEventKind = "text_delta"
	ModelOutputItemDone ModelEventKind = "output_item_done"
)

type ModelEvent struct {
	Kind  ModelEventKind
	Delta string
	Item  ResponseItem
}

// Model streams presentation deltas and authoritative completed response items.
type Model interface {
	Stream(context.Context, []HistoryItem) <-chan ModelEvent
}

func modelEventStream(ctx context.Context, events ...ModelEvent) <-chan ModelEvent {
	stream := make(chan ModelEvent)
	go func() {
		defer close(stream)
		for _, event := range events {
			select {
			case stream <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return stream
}
