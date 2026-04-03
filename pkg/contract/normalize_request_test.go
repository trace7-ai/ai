package contract

import "testing"

func baseRequest(timeout any) map[string]any {
	return map[string]any{
		"version":     "v1",
		"task":        "demo",
		"context":     map[string]any{"diff": "", "files": []any{}, "docs": []any{}},
		"timeout_sec": timeout,
	}
}

func TestRoleDefaultsToAssistant(t *testing.T) {
	request, err := NormalizeRequest(baseRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Role != "assistant" {
		t.Fatalf("role = %s, want assistant", request.Role)
	}
}

func TestRejectsNonStringDiff(t *testing.T) {
	_, err := NormalizeRequest(map[string]any{
		"version": "v1",
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
		"task":    "demo",
		"session": map[string]any{"mode": "branch"},
		"context": map[string]any{"diff": "", "files": []any{}, "docs": []any{}},
	})
	if err == nil || err.Error() != "unsupported session mode: branch" {
		t.Fatalf("error = %v", err)
	}
}

func TestLegacyRoleAliasNormalizesToAssistant(t *testing.T) {
	request, err := NormalizeRequest(map[string]any{
		"version": "v1",
		"role":    "reviewer",
		"task":    "demo",
		"context": map[string]any{"diff": "", "files": []any{}, "docs": []any{}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Role != "assistant" {
		t.Fatalf("role = %s, want assistant", request.Role)
	}
}

func TestTimeoutSecDefaultsToContractDefault(t *testing.T) {
	request, err := NormalizeRequest(baseRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.TimeoutSec != DefaultTimeoutSec {
		t.Fatalf("timeout_sec = %d, want %d", request.TimeoutSec, DefaultTimeoutSec)
	}
}

func TestTimeoutSecBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		timeout any
		want    int
		wantErr string
	}{
		{name: "accept zero as no timeout", timeout: 0, want: 0},
		{name: "reject below minimum", timeout: 9, wantErr: "timeout_sec must be 0 or an integer between 10 and 900"},
		{name: "accept minimum", timeout: 10, want: 10},
		{name: "accept explicit timeout", timeout: 300, want: 300},
		{name: "accept maximum", timeout: 900, want: 900},
		{name: "reject above maximum", timeout: 901, wantErr: "timeout_sec must be 0 or an integer between 10 and 900"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request, err := NormalizeRequest(baseRequest(test.timeout))
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if request.TimeoutSec != test.want {
				t.Fatalf("timeout_sec = %d, want %d", request.TimeoutSec, test.want)
			}
		})
	}
}

func TestNormalizeRequestPreservesContextFileMetadata(t *testing.T) {
	request, err := NormalizeRequest(map[string]any{
		"version": "v1",
		"task":    "demo",
		"context": map[string]any{
			"diff": "",
			"files": []any{map[string]any{
				"path":    "a.go",
				"content": "package a",
				"source":  "https://example.com/a",
				"title":   "File A",
			}},
			"docs": []any{},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(request.Context.Files) != 1 {
		t.Fatalf("files = %+v", request.Context.Files)
	}
	if request.Context.Files[0].Source == nil || *request.Context.Files[0].Source != "https://example.com/a" {
		t.Fatalf("file source = %v", request.Context.Files[0].Source)
	}
	if request.Context.Files[0].Title == nil || *request.Context.Files[0].Title != "File A" {
		t.Fatalf("file title = %v", request.Context.Files[0].Title)
	}
}

func TestNormalizeRequestPreservesPromptOverrides(t *testing.T) {
	request, err := NormalizeRequest(map[string]any{
		"version": "v1",
		"task":    "demo",
		"context": map[string]any{"diff": "", "files": []any{}, "docs": []any{}},
		"prompt_overrides": map[string]any{
			"protocol":            "<protocol>\nCustom\n</protocol>",
			"output_instructions": "Return only YAML.",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.PromptOverrides == nil {
		t.Fatalf("missing prompt overrides")
	}
	if request.PromptOverrides.Protocol == nil || *request.PromptOverrides.Protocol != "<protocol>\nCustom\n</protocol>" {
		t.Fatalf("protocol = %v", request.PromptOverrides.Protocol)
	}
	if request.PromptOverrides.OutputInstructions == nil || *request.PromptOverrides.OutputInstructions != "Return only YAML." {
		t.Fatalf("output = %v", request.PromptOverrides.OutputInstructions)
	}
}
