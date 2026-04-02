package contract

import "testing"

func TestPlannerRejectsMarkdown(t *testing.T) {
	_, err := NormalizeRequest(map[string]any{
		"version":        "v1",
		"role":           "planner",
		"content_format": "markdown",
		"task":           "demo",
		"context":        map[string]any{"diff": "", "files": []any{}, "docs": []any{}},
	})
	if err == nil || err.Error() != "role does not support content_format=markdown: planner" {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsNonStringDiff(t *testing.T) {
	_, err := NormalizeRequest(map[string]any{
		"version": "v1",
		"role":    "planner",
		"task":    "demo",
		"context": map[string]any{"diff": 123, "files": []any{}, "docs": []any{}},
	})
	if err == nil || err.Error() != "context.diff must be a string" {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsUnsupportedSessionMode(t *testing.T) {
	_, err := NormalizeRequest(map[string]any{
		"version": "v1",
		"role":    "planner",
		"task":    "demo",
		"session": map[string]any{"mode": "branch"},
		"context": map[string]any{"diff": "", "files": []any{}, "docs": []any{}},
	})
	if err == nil || err.Error() != "unsupported session mode: branch" {
		t.Fatalf("error = %v", err)
	}
}
