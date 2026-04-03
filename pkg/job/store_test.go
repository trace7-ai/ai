package job

import (
	"testing"

	"mira/pkg/contract"
)

func TestCreateLoadAndList(t *testing.T) {
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
	if record.Status != QueuedStatus {
		t.Fatalf("status = %s", record.Status)
	}
	loaded, err := store.Load(record.JobID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil || loaded.JobID != record.JobID {
		t.Fatalf("loaded = %+v", loaded)
	}
	jobs, err := store.List(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 1 || jobs[0].JobID != record.JobID {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func testRequest() contract.Request {
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
