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
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("planner", "structured"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: string(payload)}, {Type: "done"}},
	})
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %s", response.Status)
	}
}

func TestExecuteRequestReaderMarkdownSuccess(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("reader", "markdown"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "# Summary"}, {Type: "done"}},
	})
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.ContentType == nil || *response.ContentType != "markdown" {
		t.Fatalf("content type = %v", response.ContentType)
	}
}

func TestExecuteRequestStructuredParseFailure(t *testing.T) {
	response, exitCode := ExecuteRequest(context.Background(), baseRequest("planner", "structured"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "not json"}, {Type: "done"}},
	})
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
	response, exitCode := ExecuteRequest(ctx, baseRequest("reader", "text"), transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "late"}, {Type: "done"}},
		Delay:  100 * time.Millisecond,
	})
	if exitCode != ExitTimeout {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Errors[0].Code != "timeout" {
		t.Fatalf("code = %s", response.Errors[0].Code)
	}
}
