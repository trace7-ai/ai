package transport

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSSEStreamReturnsContentAndDone(t *testing.T) {
	body := strings.Join([]string{
		`data: {"Message":"{\"event\":\"content\",\"data\":{\"type\":\"result\",\"text\":\"hello\"}}"}`,
		`data: {"done":true}`,
		"",
	}, "\n")
	stream := NewSSEStream(&http.Response{Body: ioNopCloser{Reader: strings.NewReader(body)}})

	content, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("next content: %v", err)
	}
	if content.Type != "content" || content.Text != "hello" || !content.FromContent {
		t.Fatalf("unexpected content event: %+v", content)
	}
	done, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("next done: %v", err)
	}
	if done.Type != "done" {
		t.Fatalf("unexpected done event: %+v", done)
	}
}

func TestSSEStreamErrorsOnUnexpectedEOF(t *testing.T) {
	body := `data: {"Message":"{\"event\":\"content\",\"data\":{\"type\":\"result\",\"text\":\"partial\"}}"}` + "\n"
	stream := NewSSEStream(&http.Response{Body: ioNopCloser{Reader: strings.NewReader(body)}})
	if _, err := stream.Next(context.Background()); err != nil {
		t.Fatalf("first next: %v", err)
	}
	if _, err := stream.Next(context.Background()); err == nil || err.Error() != "stream ended unexpectedly" {
		t.Fatalf("err = %v", err)
	}
}

func TestSSEStreamParsesStopReasonOnMessageDelta(t *testing.T) {
	body := strings.Join([]string{
		`data: {"Message":"{\"event\":\"reason\",\"data\":{\"event\":{\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"},\"usage\":{\"output_tokens\":42}}}}"}`,
		`data: {"done":true}`,
		"",
	}, "\n")
	stream := NewSSEStream(&http.Response{Body: ioNopCloser{Reader: strings.NewReader(body)}})

	event, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("next usage: %v", err)
	}
	if event.Type != "usage" || event.StopReason != "max_tokens" {
		t.Fatalf("unexpected usage event: %+v", event)
	}
	if event.Usage == nil || event.Usage.Output == nil || *event.Usage.Output != 42 {
		t.Fatalf("usage = %+v", event.Usage)
	}
}

func TestSSEStreamSupportsNestedContentResult(t *testing.T) {
	body := strings.Join([]string{
		`data: {"Message":"{\"event\":\"content\",\"data\":{\"content\":{\"type\":\"result\",\"result\":\"hello\",\"stop_reason\":\"end_turn\"}}}"}`,
		`data: {"done":true}`,
		"",
	}, "\n")
	stream := NewSSEStream(&http.Response{Body: ioNopCloser{Reader: strings.NewReader(body)}})

	event, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("next content: %v", err)
	}
	if event.Type != "content" || event.Text != "hello" || !event.FromContent || event.StopReason != "end_turn" {
		t.Fatalf("unexpected content event: %+v", event)
	}
}

type ioNopCloser struct {
	Reader *strings.Reader
}

func (closer ioNopCloser) Read(p []byte) (int, error) {
	return closer.Reader.Read(p)
}

func (closer ioNopCloser) Close() error {
	return nil
}
