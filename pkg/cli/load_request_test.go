package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReaderDefaultsToMarkdown(t *testing.T) {
	request, err := LoadRequestFromCLI([]string{"--role", "reader", "--task", "read this doc"}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.ContentFormat != "markdown" {
		t.Fatalf("content format = %s, want markdown", request.ContentFormat)
	}
}

func TestInferPlannerFromPlanKeyword(t *testing.T) {
	request, err := LoadRequestFromCLI([]string{"Plan the implementation steps"}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Role != "planner" {
		t.Fatalf("role = %s, want planner", request.Role)
	}
}

func TestInferReviewerFromDiffContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "context.json")
	body, _ := json.Marshal(map[string]any{
		"context": map[string]any{"diff": "diff --git a/app.py b/app.py", "files": []any{}, "docs": []any{}},
	})
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	request, err := LoadRequestFromCLI([]string{"--context-file", path, "--task", "please check this patch"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Role != "reviewer" {
		t.Fatalf("role = %s, want reviewer", request.Role)
	}
}

func TestRejectInvalidJSONContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "context.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	_, err := LoadRequestFromCLI([]string{"--context-file", path, "summarize this"}, dir)
	if err == nil || err.Error() != "context file is not valid JSON: "+path {
		t.Fatalf("error = %v", err)
	}
}
