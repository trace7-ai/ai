package sdk

import (
	"testing"

	"mira/pkg/contract"
	"mira/pkg/runner"
	"mira/pkg/session"
	"mira/pkg/transport"
)

func TestAskUsesProvidedTransportFactoryAndReturnsEphemeralSession(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	request := contract.Request{
		Version:       "v1",
		Role:          "assistant",
		ContentFormat: "text",
		Session:       contract.Session{Mode: "ephemeral"},
		FileManifest:  contract.FileManifest{Mode: "none", Paths: []string{}, MaxTotalBytes: 512 * 1024, ReadOnly: true},
		Task:          "demo",
		Constraints:   []string{},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		MaxTokens:     512,
		TimeoutSec:    0,
	}
	response, exitCode, err := Ask(request, &Options{
		TransportFactory: func() (transport.Transport, error) {
			return transport.FakeTransport{
				Events: []transport.Event{
					{Type: "content", Text: "OK"},
					{Type: "done"},
				},
			}, nil
		},
		Store: store,
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if exitCode != runner.ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %s", response.Status)
	}
	if response.Session == nil {
		t.Fatalf("expected session payload")
	}
	if response.Session.Status != "ephemeral" {
		t.Fatalf("session status = %s, want ephemeral", response.Session.Status)
	}
}

func TestAskWithBuilderBuildsRequestAndExecutes(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	response, exitCode, err := AskWithBuilder(NewRequest("demo"), &Options{
		TransportFactory: func() (transport.Transport, error) {
			return transport.FakeTransport{
				Events: []transport.Event{
					{Type: "content", Text: "OK"},
					{Type: "done"},
				},
			}, nil
		},
		Store: store,
	})
	if err != nil {
		t.Fatalf("ask with builder: %v", err)
	}
	if exitCode != runner.ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %s", response.Status)
	}
}

func TestAskStreamsChunksThroughOptions(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	chunks := make([]string, 0, 2)
	_, exitCode, err := Ask(contract.Request{
		Version:       "v1",
		Role:          "assistant",
		ContentFormat: "text",
		Session:       contract.Session{Mode: "ephemeral"},
		FileManifest:  contract.FileManifest{Mode: "none", Paths: []string{}, MaxTotalBytes: 512 * 1024, ReadOnly: true},
		Task:          "demo",
		Constraints:   []string{},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		MaxTokens:     512,
		TimeoutSec:    0,
	}, &Options{
		TransportFactory: func() (transport.Transport, error) {
			return transport.FakeTransport{
				Events: []transport.Event{
					{Type: "content", Text: "hello ", FromContent: false},
					{Type: "content", Text: "hello world", FromContent: true},
					{Type: "content", Text: "world", FromContent: false},
					{Type: "done"},
				},
			}, nil
		},
		Store: store,
		OnChunk: func(chunk []byte) {
			chunks = append(chunks, string(chunk))
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if exitCode != runner.ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if len(chunks) != 2 || chunks[0] != "hello " || chunks[1] != "world" {
		t.Fatalf("chunks = %+v", chunks)
	}
}
