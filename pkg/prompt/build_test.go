package prompt

import (
	"strings"
	"testing"

	"mira/pkg/contract"
)

func TestBuildPromptUsesStructuredBlocks(t *testing.T) {
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "auto",
		Task:          "Plan the implementation steps",
		MaxTokens:     512,
		TimeoutSec:    0,
		Session: contract.Session{
			SessionID: nil,
			ContextHint: contract.SessionContextHint{
				WorkspaceRoot: stringPtr("/tmp/work"),
				GitBranch:     stringPtr("main"),
			},
		},
		Context: contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
	}
	prompt := Build(request)
	if !strings.Contains(prompt.Text, "<protocol>") {
		t.Fatalf("prompt missing protocol block")
	}
	if !strings.Contains(prompt.Text, "read-only sidecar assistant") {
		t.Fatalf("prompt missing sidecar identity")
	}
	if !strings.Contains(prompt.Text, "Read caller-provided Feishu/Lark cloud document URLs.") {
		t.Fatalf("prompt missing caller-provided cloud doc capability")
	}
	if !strings.Contains(prompt.Text, "Remote document reading is limited to caller-provided URLs.") {
		t.Fatalf("prompt missing scoped URL boundary")
	}
	if !strings.Contains(prompt.Text, "<task") {
		t.Fatalf("prompt missing task block")
	}
	if !strings.Contains(prompt.Text, `workspace_root="/tmp/work"`) {
		t.Fatalf("prompt missing workspace root attribute")
	}
	if !strings.Contains(prompt.Text, "<context ") {
		t.Fatalf("prompt missing context block")
	}
	if !strings.Contains(prompt.Text, "<output>") {
		t.Fatalf("prompt missing output block")
	}
	if prompt.Meta == nil {
		t.Fatalf("prompt missing meta")
	}
}

func TestBuildPromptDoesNotForbidCallerProvidedURLReading(t *testing.T) {
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "text",
		Task:          "请审阅 https://bytedance.larkoffice.com/wiki/demo",
		MaxTokens:     512,
		Session:       contract.Session{SessionID: nil},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
	}
	prompt := Build(request)
	if strings.Contains(prompt.Text, "Do not assume shell access, file modification, network access, browsing, or extra tools.") {
		t.Fatalf("prompt still forbids caller-provided URL reading")
	}
}

func TestBuildPromptOmitsCarryForwardByDefault(t *testing.T) {
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "text",
		Task:          "Summarize this file",
		MaxTokens:     512,
		TimeoutSec:    0,
		Session:       contract.Session{SessionID: nil},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
	}
	prompt := Build(request)
	if strings.Contains(prompt.Text, "carry_forward") || strings.Contains(prompt.Text, "append a line containing only `---`") {
		t.Fatalf("prompt should not inject carry_forward by default")
	}
}

func TestBuildPromptWithExplicitTimeoutAddsPriorityGuidance(t *testing.T) {
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "text",
		Task:          "Summarize this file",
		MaxTokens:     512,
		TimeoutSec:    120,
		Session:       contract.Session{SessionID: nil},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
	}
	prompt := Build(request)
	if !strings.Contains(prompt.Text, "terminate this request after 120 seconds") {
		t.Fatalf("prompt missing explicit timeout priority guidance")
	}
}

func TestBuildPromptRendersContextAsCompactBlocks(t *testing.T) {
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "text",
		Task:          "Explain this file",
		MaxTokens:     512,
		Session:       contract.Session{SessionID: nil},
		Context: contract.Context{
			Diff: "diff --git a/app.go b/app.go",
			Files: []contract.ContextFile{{
				Path:    "app.go",
				Content: "line one\nline \"two\"\n",
			}},
			Docs: []contract.ContextDoc{{
				Content: "doc line one\ndoc line two",
			}},
		},
	}
	prompt := Build(request)
	if strings.Contains(prompt.Text, "\"files\":") {
		t.Fatalf("prompt should not render context as JSON: %s", prompt.Text)
	}
	if !strings.Contains(prompt.Text, `<context total_items="3" included="3" omitted="0">`) {
		t.Fatalf("prompt missing context completeness attributes")
	}
	if !strings.Contains(prompt.Text, "<file path=\"app.go\">") {
		t.Fatalf("prompt missing compact file block")
	}
	if !strings.Contains(prompt.Text, "line one\nline \"two\"") {
		t.Fatalf("prompt should keep raw file content without JSON escaping")
	}
	if !strings.Contains(prompt.Text, "<doc>") {
		t.Fatalf("prompt missing compact doc block")
	}
}

func TestBuildPromptAppliesContextBudgetAndOmissionNote(t *testing.T) {
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "text",
		Task:          "Explain these files",
		MaxTokens:     512,
		Session:       contract.Session{SessionID: nil},
		Context: contract.Context{
			Files: []contract.ContextFile{
				{Path: "a.go", Content: "FILE_A\n" + strings.Repeat("A", 9000)},
				{Path: "b.go", Content: "FILE_B\n" + strings.Repeat("B", 9000)},
				{Path: "c.go", Content: "FILE_C\n" + strings.Repeat("C", 9000)},
				{Path: "d.go", Content: "FILE_D\n" + strings.Repeat("D", 9000)},
			},
			Docs: []contract.ContextDoc{},
		},
	}
	prompt := Build(request)
	if !strings.Contains(prompt.Text, "[truncated]") {
		t.Fatalf("prompt should mark oversized context blocks as truncated")
	}
	if !strings.Contains(prompt.Text, `<context total_items="4"`) {
		t.Fatalf("prompt should report context item totals")
	}
	if !strings.Contains(prompt.Text, `omitted="1"`) {
		t.Fatalf("prompt should report omitted context items")
	}
	if !strings.Contains(prompt.Text, "Context truncated: 1 additional item(s) omitted due to prompt budget.") {
		t.Fatalf("prompt should note omitted context items")
	}
	if strings.Contains(prompt.Text, "FILE_D") {
		t.Fatalf("prompt should omit files beyond the context budget")
	}
}

func TestBuildContextBlockRespectsBudgetIncludingWrapperOverhead(t *testing.T) {
	context := contract.Context{
		Files: []contract.ContextFile{
			{Path: strings.Repeat("very-long-path-", 40) + "a.go", Content: strings.Repeat("A", 9000)},
			{Path: strings.Repeat("very-long-path-", 40) + "b.go", Content: strings.Repeat("B", 9000)},
			{Path: strings.Repeat("very-long-path-", 40) + "c.go", Content: strings.Repeat("C", 9000)},
			{Path: strings.Repeat("very-long-path-", 40) + "d.go", Content: strings.Repeat("D", 9000)},
		},
	}
	block, blockMeta := buildContextBlock(context)
	if blockMeta == nil || blockMeta.ContextTotalItems != 4 || blockMeta.ContextOmitted != 1 {
		t.Fatalf("unexpected context meta: %+v", blockMeta)
	}
	start := strings.Index(block, ">\n")
	end := strings.LastIndex(block, "</context>")
	if start < 0 || end < 0 || end < start+2 {
		t.Fatalf("context block wrapper malformed: %s", block)
	}
	inner := block[start+2 : end]
	if len([]rune(inner)) > contextTotalBudget {
		t.Fatalf("context block exceeds budget: %d > %d", len([]rune(inner)), contextTotalBudget)
	}
}

func TestAutoInstructionsDefaultToMarkdownUnlessTaskDemandsStructure(t *testing.T) {
	instructions := outputInstructions("auto")
	if !strings.Contains(instructions, "Default to markdown") {
		t.Fatalf("auto instructions should default to markdown")
	}
	if !strings.Contains(instructions, "explicitly asks for machine-readable output") {
		t.Fatalf("auto instructions should describe deterministic structured fallback")
	}
}

func TestBuildPromptUsesPromptOverridesWhenProvided(t *testing.T) {
	protocol := "<protocol>\nCustom protocol\n</protocol>"
	output := "Return only YAML."
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "auto",
		Task:          "demo",
		MaxTokens:     512,
		Session:       contract.Session{Mode: "ephemeral"},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		PromptOverrides: &contract.PromptOverrides{
			Protocol:           &protocol,
			OutputInstructions: &output,
		},
	}
	prompt := Build(request)
	if !strings.Contains(prompt.Text, protocol) {
		t.Fatalf("prompt missing overridden protocol")
	}
	if strings.Contains(prompt.Text, "read-only sidecar assistant") {
		t.Fatalf("prompt should not include default protocol when overridden")
	}
	if !strings.Contains(prompt.Text, "<output>\nReturn only YAML.\n</output>") {
		t.Fatalf("prompt missing overridden output instructions")
	}
}

func TestBuildPromptPreservesRequestedContentFormatWithOutputOverride(t *testing.T) {
	output := "Return exactly one JSON value."
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "structured",
		Task:          "demo",
		MaxTokens:     512,
		Session:       contract.Session{Mode: "ephemeral"},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		PromptOverrides: &contract.PromptOverrides{
			OutputInstructions: &output,
		},
	}
	prompt := Build(request)
	if !strings.Contains(prompt.Text, "<output>\nContent format requested: structured.\nReturn exactly one JSON value.\n</output>") {
		t.Fatalf("prompt should preserve content format when output override is used")
	}
}

func TestBuildPromptExposesContextMeta(t *testing.T) {
	request := contract.Request{
		Role:          "assistant",
		ContentFormat: "text",
		Task:          "demo",
		MaxTokens:     512,
		Session:       contract.Session{Mode: "ephemeral"},
		Context: contract.Context{
			Diff: "diff",
			Files: []contract.ContextFile{
				{Path: "a.go", Content: "a"},
			},
			Docs: []contract.ContextDoc{
				{Content: "doc"},
			},
		},
	}
	prompt := Build(request)
	if prompt.Meta == nil {
		t.Fatalf("prompt meta missing")
	}
	if prompt.Meta.ContextTotalItems != 3 || prompt.Meta.ContextIncluded != 3 || prompt.Meta.ContextOmitted != 0 {
		t.Fatalf("prompt meta = %+v", prompt.Meta)
	}
}

func stringPtr(value string) *string {
	return &value
}
