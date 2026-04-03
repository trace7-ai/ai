package runner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mira/pkg/contract"
	"mira/pkg/transport"
)

func baseRequest(role string, contentFormat string) contract.Request {
	return contract.Request{
		Version:       "v1",
		Role:          role,
		ContentFormat: contentFormat,
		Task:          "demo",
		Constraints:   []string{},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		MaxTokens:     512,
		TimeoutSec:    30,
		Session:       contract.Session{Mode: "ephemeral"},
		FileManifest:  contract.FileManifest{Mode: "none", Paths: []string{}, MaxTotalBytes: 512 * 1024, ReadOnly: true},
	}
}

func TestExecuteRequestPlannerSuccess(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"summary": "ok", "plan": []any{}, "risks": []any{}, "open_questions": []any{}, "validation": []any{},
	})
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "structured"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: string(payload)}, {Type: "done"}},
	}, nil)
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %s", response.Status)
	}
}

func TestExecuteRequestReaderMarkdownSuccess(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "markdown"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "# Summary"}, {Type: "done"}},
	}, nil)
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.ContentType == nil || *response.ContentType != "markdown" {
		t.Fatalf("content type = %v", response.ContentType)
	}
}

func TestExecuteRequestStructuredParseFailure(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "structured"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "not json"}, {Type: "done"}},
	}, nil)
	if exitCode != ExitExecutionError {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Errors[0].Code != "execution_failed" {
		t.Fatalf("code = %s", response.Errors[0].Code)
	}
}

func TestExecuteRequestTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	response, exitCode := ExecuteRequest(ctx, baseRequest("assistant", "text"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "late"}, {Type: "done"}},
		Delay:  100 * time.Millisecond,
	}, nil)
	if exitCode != ExitTimeout {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Errors[0].Code != "timeout" {
		t.Fatalf("code = %s", response.Errors[0].Code)
	}
}

func TestExecuteRequestMarksTruncatedOnStopReason(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "text"), transport.FakeTransport{
		Events: []transport.Event{
			{Type: "content", Text: "partial"},
			{Type: "usage", StopReason: "max_tokens"},
			{Type: "done"},
		},
	}, nil)
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if !response.Truncated {
		t.Fatalf("truncated = %v, want true", response.Truncated)
	}
}

func TestExecuteRequestAutoParsesJSONWhenModelChoosesIt(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "auto"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "{\"summary\":\"ok\",\"carry_forward\":\"next\"}"}, {Type: "done"}},
	}, nil)
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.ContentType == nil || *response.ContentType != "structured" {
		t.Fatalf("content type = %v", response.ContentType)
	}
}

func TestExecuteRequestAutoFallsBackToText(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "auto"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "plain answer\n---\ncarry"}, {Type: "done"}},
	}, nil)
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.ContentType == nil || *response.ContentType != "text" {
		t.Fatalf("content type = %v", response.ContentType)
	}
}

func TestExecuteRequestReturnsPromptMeta(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "text"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "ok"}, {Type: "done"}},
	}, nil)
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.PromptMeta == nil {
		t.Fatalf("missing prompt meta")
	}
	if response.PromptMeta.ContextTotalItems != 0 || response.PromptMeta.ContextIncluded != 0 || response.PromptMeta.ContextOmitted != 0 {
		t.Fatalf("prompt meta = %+v", response.PromptMeta)
	}
}

func TestExecuteRequestStreamsDedupedChunksToCallback(t *testing.T) {
	chunks := make([]string, 0, 2)
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("assistant", "text"), transport.FakeTransport{
		Events: []transport.Event{
			{Type: "content", Text: "hello ", FromContent: false},
			{Type: "content", Text: "hello world", FromContent: true},
			{Type: "content", Text: "world", FromContent: false},
			{Type: "done"},
		},
	}, func(chunk []byte) {
		chunks = append(chunks, string(chunk))
	})
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Result != "hello world" {
		t.Fatalf("result = %#v", response.Result)
	}
	if len(chunks) != 2 || chunks[0] != "hello " || chunks[1] != "world" {
		t.Fatalf("chunks = %+v", chunks)
	}
}
