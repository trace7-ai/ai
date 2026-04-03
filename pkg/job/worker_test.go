package job

import (
	"testing"

	"mira/pkg/transport"
)

func TestExecuteWritesResponseAndCompletes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MIRA_HOME", root)
	store, err := New("")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := store.Create(testRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	exitCode, err := Execute(record.JobID, func() (transport.Transport, error) {
		return transport.FakeTransport{
			Events: []transport.Event{
				{Type: "content", Text: "{\"summary\":\"ok\"}"},
				{Type: "done"},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d", exitCode)
	}
	loaded, err := store.Load(record.JobID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil || loaded.Status != CompletedStatus {
		t.Fatalf("loaded = %+v", loaded)
	}
	response, err := store.LoadResponse(record.JobID)
	if err != nil {
		t.Fatalf("response: %v", err)
	}
	if response == nil || response.Status != "ok" {
		t.Fatalf("response = %+v", response)
	}
}
