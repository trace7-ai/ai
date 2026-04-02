package transport

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrNotImplemented = errors.New("go transport is not implemented yet")

type FakeTransport struct {
	ExecuteErr error
	Events     []Event
	Delay      time.Duration
}

func (transport FakeTransport) Execute(ctx context.Context, prompt Prompt, opts Options) (Stream, error) {
	if transport.ExecuteErr != nil {
		return nil, transport.ExecuteErr
	}
	return &FakeStream{events: transport.Events, delay: transport.Delay}, nil
}

type FakeStream struct {
	events []Event
	delay  time.Duration
	index  int
}

func (stream *FakeStream) Next(ctx context.Context) (Event, error) {
	if stream.index >= len(stream.events) {
		return Event{}, io.EOF
	}
	if stream.delay > 0 {
		timer := time.NewTimer(stream.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		case <-timer.C:
		}
	}
	event := stream.events[stream.index]
	stream.index++
	return event, nil
}

func (stream *FakeStream) Close() error {
	return nil
}

type NullTransport struct{}

func (NullTransport) Execute(ctx context.Context, prompt Prompt, opts Options) (Stream, error) {
	return nil, ErrNotImplemented
}
