package transport

import (
	"context"

	"mira/pkg/contract"
)

type Prompt struct {
	Text string
}

type Options struct {
	MaxTokens  int
	TimeoutSec int
	RequestID  *string
	SessionID  *string
}

type Event struct {
	Type        string
	Text        string
	FromContent bool
	Usage       *contract.TokenUsage
}

type Stream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
}

type Transport interface {
	Execute(ctx context.Context, prompt Prompt, opts Options) (Stream, error)
}

type SessionAware interface {
	SetRemoteSessionID(sessionID string)
	RemoteSessionID() string
}

type ModelAware interface {
	ModelName() *string
	HasAuth() bool
}
