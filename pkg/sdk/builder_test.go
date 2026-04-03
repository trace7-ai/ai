package sdk

import (
	"testing"

	"mira/pkg/contract"
)

func TestNewRequestBuildsMinimalValidRequest(t *testing.T) {
	request, err := NewRequest("hello").Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if request.Version != contract.SchemaVersion {
		t.Fatalf("version = %s", request.Version)
	}
	if request.Role != "assistant" {
		t.Fatalf("role = %s", request.Role)
	}
	if request.ContentFormat != "auto" {
		t.Fatalf("content format = %s", request.ContentFormat)
	}
	if request.Session.Mode != "ephemeral" || request.Session.SessionID != nil {
		t.Fatalf("unexpected session: %+v", request.Session)
	}
	if request.FileManifest.Mode != "none" {
		t.Fatalf("file manifest mode = %s", request.FileManifest.Mode)
	}
	if request.MaxTokens != contract.DefaultMaxTokens {
		t.Fatalf("max tokens = %d", request.MaxTokens)
	}
	if request.TimeoutSec != contract.DefaultTimeoutSec {
		t.Fatalf("timeout = %d", request.TimeoutSec)
	}
}

func TestRequestBuilderBuildsStickySessionWithHints(t *testing.T) {
	request, err := NewRequest("task").
		WithSession("session-1").
		WithWorkspaceRoot("/tmp/work").
		WithGitBranch("main").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if request.Session.Mode != "sticky" {
		t.Fatalf("session mode = %s", request.Session.Mode)
	}
	if request.Session.SessionID == nil || *request.Session.SessionID != "session-1" {
		t.Fatalf("session id = %v", request.Session.SessionID)
	}
	if request.Session.ContextHint.WorkspaceRoot == nil || *request.Session.ContextHint.WorkspaceRoot != "/tmp/work" {
		t.Fatalf("workspace root = %v", request.Session.ContextHint.WorkspaceRoot)
	}
	if request.Session.ContextHint.GitBranch == nil || *request.Session.ContextHint.GitBranch != "main" {
		t.Fatalf("git branch = %v", request.Session.ContextHint.GitBranch)
	}
}

func TestRequestBuilderRejectsConflictingSessionModes(t *testing.T) {
	_, err := NewRequest("task").WithSession("session-1").WithEphemeral().Build()
	if err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestRequestBuilderAccumulatesContextInOrder(t *testing.T) {
	source := "https://example.com/doc"
	title := "Example Doc"
	request, err := NewRequest("task").
		WithDiff("diff --git a/a.go b/a.go").
		AddFile("a.go", "package a").
		AddContextFile(contract.ContextFile{
			Path:    "b.go",
			Content: "package b",
			Source:  &source,
			Title:   &title,
		}).
		AddContextDoc(contract.ContextDoc{
			Content: "doc one",
			Source:  &source,
			Title:   &title,
		}).
		AddConstraint("only list blockers").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if request.Context.Diff != "diff --git a/a.go b/a.go" {
		t.Fatalf("diff = %s", request.Context.Diff)
	}
	if len(request.Context.Files) != 2 || request.Context.Files[0].Path != "a.go" || request.Context.Files[1].Path != "b.go" {
		t.Fatalf("files = %+v", request.Context.Files)
	}
	if request.Context.Files[1].Source == nil || *request.Context.Files[1].Source != source {
		t.Fatalf("file source = %v", request.Context.Files[1].Source)
	}
	if request.Context.Files[1].Title == nil || *request.Context.Files[1].Title != title {
		t.Fatalf("file title = %v", request.Context.Files[1].Title)
	}
	if len(request.Context.Docs) != 1 || request.Context.Docs[0].Content != "doc one" {
		t.Fatalf("docs = %+v", request.Context.Docs)
	}
	if request.Context.Docs[0].Source == nil || *request.Context.Docs[0].Source != source {
		t.Fatalf("doc source = %v", request.Context.Docs[0].Source)
	}
	if request.Context.Docs[0].Title == nil || *request.Context.Docs[0].Title != title {
		t.Fatalf("doc title = %v", request.Context.Docs[0].Title)
	}
	if len(request.Constraints) != 1 || request.Constraints[0] != "only list blockers" {
		t.Fatalf("constraints = %+v", request.Constraints)
	}
}

func TestRequestBuilderValidatesTimeoutAndMaxTokens(t *testing.T) {
	if _, err := NewRequest("task").WithTimeoutSec(10).WithMaxTokens(256).Build(); err != nil {
		t.Fatalf("expected valid boundary request: %v", err)
	}
	if _, err := NewRequest("task").WithTimeoutSec(5).Build(); err == nil {
		t.Fatalf("expected invalid timeout error")
	}
	if _, err := NewRequest("task").WithMaxTokens(0).Build(); err == nil {
		t.Fatalf("expected invalid max token error")
	}
}

func TestRequestBuilderCarriesPromptOverrides(t *testing.T) {
	request, err := NewRequest("task").
		WithPromptProtocol("<protocol>\nCustom\n</protocol>").
		WithOutputInstructions("Return only YAML.").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if request.PromptOverrides == nil || request.PromptOverrides.Protocol == nil {
		t.Fatalf("missing protocol override: %+v", request.PromptOverrides)
	}
	if *request.PromptOverrides.Protocol != "<protocol>\nCustom\n</protocol>" {
		t.Fatalf("protocol override = %v", request.PromptOverrides.Protocol)
	}
	if request.PromptOverrides.OutputInstructions == nil || *request.PromptOverrides.OutputInstructions != "Return only YAML." {
		t.Fatalf("output override = %v", request.PromptOverrides.OutputInstructions)
	}
}
