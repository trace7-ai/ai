package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mira/pkg/contract"
	jobpkg "mira/pkg/job"
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
	t.Setenv("MIRA_HOME", t.TempDir())
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
	exitCode := Run([]string{"ask", "--task", "plan this"}, &stdout, &bytes.Buffer{})
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

func TestAskEphemeralReturnsStructuredSessionPayload(t *testing.T) {
	t.Setenv("MIRA_HOME", t.TempDir())
	previous := defaultTransportFactory
	defaultTransportFactory = func() (transport.Transport, error) {
		return transport.FakeTransport{
			Events: []transport.Event{
				{Type: "content", Text: "OK"},
				{Type: "done"},
			},
		}, nil
	}
	defer func() {
		defaultTransportFactory = previous
	}()
	var stdout bytes.Buffer
	exitCode := Run([]string{"ask", "--ephemeral", "--task", "Reply with OK only"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	sessionPayload := payload["session"].(map[string]any)
	if sessionPayload["status"] != "ephemeral" {
		t.Fatalf("session status = %v, want ephemeral", sessionPayload["status"])
	}
	if sessionPayload["reused"] != false {
		t.Fatalf("session reused = %v, want false", sessionPayload["reused"])
	}
	if sessionPayload["session_id"] != "" {
		t.Fatalf("session_id = %v, want empty", sessionPayload["session_id"])
	}
}

func TestHistoryListsSessions(t *testing.T) {
	miraHome := t.TempDir()
	t.Setenv("MIRA_HOME", miraHome)
	if err := os.MkdirAll(filepath.Join(miraHome, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	record := []byte("{\"session_id\":\"demo\",\"status\":\"active\",\"last_active_at\":\"2026-04-03T00:00:00Z\",\"turn_count\":2}")
	if err := os.WriteFile(filepath.Join(miraHome, "sessions", "demo.json"), record, 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
	var stdout bytes.Buffer
	exitCode := Run([]string{"history"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	sessions := payload["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %v", payload["sessions"])
	}
}

func TestJobsAndPollReadPersistedJob(t *testing.T) {
	miraHome := t.TempDir()
	t.Setenv("MIRA_HOME", miraHome)
	store, err := jobpkg.New("")
	if err != nil {
		t.Fatalf("new job store: %v", err)
	}
	record, err := store.Create(jobpkgTestRequest())
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	record = record.Complete(0, "2026-04-03T00:00:01Z")
	if err := store.Save(record); err != nil {
		t.Fatalf("save job: %v", err)
	}
	if err := store.SaveResponse(record.JobID, jobpkgTestResponse()); err != nil {
		t.Fatalf("save response: %v", err)
	}

	var jobsOut bytes.Buffer
	if exitCode := Run([]string{"jobs"}, &jobsOut, &bytes.Buffer{}); exitCode != 0 {
		t.Fatalf("jobs exit code = %d", exitCode)
	}
	var jobsPayload map[string]any
	if err := json.Unmarshal(jobsOut.Bytes(), &jobsPayload); err != nil {
		t.Fatalf("jobs unmarshal: %v", err)
	}
	if len(jobsPayload["jobs"].([]any)) != 1 {
		t.Fatalf("jobs payload = %v", jobsPayload)
	}

	var pollOut bytes.Buffer
	if exitCode := Run([]string{"poll", record.JobID}, &pollOut, &bytes.Buffer{}); exitCode != 0 {
		t.Fatalf("poll exit code = %d", exitCode)
	}
	var pollPayload map[string]any
	if err := json.Unmarshal(pollOut.Bytes(), &pollPayload); err != nil {
		t.Fatalf("poll unmarshal: %v", err)
	}
	if pollPayload["response"] == nil {
		t.Fatalf("poll payload missing response: %v", pollPayload)
	}
}

func jobpkgTestRequest() contract.Request {
	return contract.Request{
		Version:       "v1",
		Role:          "assistant",
		ContentFormat: "structured",
		Session:       contract.Session{Mode: "ephemeral"},
		FileManifest:  contract.FileManifest{Mode: "none", Paths: []string{}, MaxTotalBytes: 512 * 1024, ReadOnly: true},
		Task:          "demo",
		Constraints:   []string{},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		MaxTokens:     512,
		TimeoutSec:    0,
	}
}

func jobpkgTestResponse() contract.Response {
	return contract.BuildSuccessResponse("assistant", nil, nil, map[string]any{"summary": "ok"}, "structured")
}
