package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mira/pkg/transport"
)

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := Run([]string{"--version"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if stdout.String() != "mira v1.0\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestUnsupportedCommand(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := Run([]string{"login"}, &bytes.Buffer{}, &stderr)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if stderr.String() != "unsupported in ai-first shell: login\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAskInvalidContextReturnsJSONError(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := Run([]string{"ask", "--context-file", "/tmp/does-not-exist.json", "summarize"}, &stdout, &bytes.Buffer{})
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if payload["status"] != "error" {
		t.Fatalf("status = %v, want error", payload["status"])
	}
}

func TestAskHydratesFilesAndSessionPayload(t *testing.T) {
	previous := defaultTransportFactory
	defaultTransportFactory = func() (transport.Transport, error) {
		return transport.NullTransport{}, nil
	}
	defer func() {
		defaultTransportFactory = previous
	}()
	workspace := t.TempDir()
	miraHome := t.TempDir()
	filePath := filepath.Join(workspace, "note.md")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	prevDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(prevDir)
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("MIRA_HOME", miraHome)
	var stdout bytes.Buffer
	exitCode := Run([]string{"ask", "--session", "demo-session", "--file", "note.md", "--task", "read this doc"}, &stdout, &bytes.Buffer{})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if payload["status"] != "error" {
		t.Fatalf("status = %v, want error", payload["status"])
	}
	filesRead := payload["files_read"].([]any)
	if len(filesRead) != 1 {
		t.Fatalf("files_read len = %d, want 1", len(filesRead))
	}
	sessionPayload := payload["session"].(map[string]any)
	if sessionPayload["session_id"] != "demo-session" {
		t.Fatalf("session_id = %v", sessionPayload["session_id"])
	}
	if sessionPayload["status"] != "error" {
		t.Fatalf("session status = %v, want error", sessionPayload["status"])
	}
}

func TestAskUsesFakeTransportSuccess(t *testing.T) {
	previous := defaultTransportFactory
	defaultTransportFactory = func() (transport.Transport, error) {
		return transport.FakeTransport{
			Events: []transport.Event{
				{Type: "content", Text: "{\"summary\":\"ok\",\"plan\":[],\"risks\":[],\"open_questions\":[],\"validation\":[]}"},
				{Type: "done"},
			},
		}, nil
	}
	defer func() {
		defaultTransportFactory = previous
	}()
	var stdout bytes.Buffer
	exitCode := Run([]string{"ask", "--role", "planner", "--task", "plan this"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("status = %v, want ok", payload["status"])
	}
}
