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

type StreamFailure struct {
	Message   string
	Retryable bool
}

type ModelStream struct {
	Events <-chan ModelEvent
	Err    <-chan *StreamFailure
}

// Model streams presentation deltas and authoritative completed response items.
type Model interface {
	Stream(context.Context, []HistoryItem) ModelStream
}

func modelEventStream(ctx context.Context, events ...ModelEvent) ModelStream {
	return modelEventStreamWithFailure(ctx, nil, events...)
}

func modelEventStreamWithFailure(ctx context.Context, failure *StreamFailure, events ...ModelEvent) ModelStream {
	stream := make(chan ModelEvent)
	errs := make(chan *StreamFailure, 1)
	go func() {
		defer close(stream)
		defer close(errs)
		for _, event := range events {
			select {
			case stream <- event:
			case <-ctx.Done():
				errs <- &StreamFailure{Message: ctx.Err().Error()}
				return
			}
		}
		errs <- failure
	}()
	return ModelStream{Events: stream, Err: errs}
}
