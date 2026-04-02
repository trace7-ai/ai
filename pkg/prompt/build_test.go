package prompt

import (
	"strings"
	"testing"

	"mira/pkg/contract"
	"mira/pkg/roles"
)

func TestBuildPlannerPrompt(t *testing.T) {
	role, _ := roles.Get("planner")
	request := contract.Request{
		Role:          "planner",
		ContentFormat: "structured",
		Task:          "Plan the implementation steps",
		MaxTokens:     512,
		TimeoutSec:    30,
		Session:       contract.Session{SessionID: nil},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
	}
	prompt := Build(request, role)
	if !strings.Contains(prompt.Text, "You are Mira acting as a planner sidecar subagent.") {
		t.Fatalf("prompt missing planner header")
	}
	if !strings.Contains(prompt.Text, "\"summary\": \"string\"") {
		t.Fatalf("prompt missing structured example")
	}
}
