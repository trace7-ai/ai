package prompt

import (
	"fmt"
	"sort"
	"strings"

	"mira/pkg/contract"
	"mira/pkg/transport"
)

const (
	contextTotalBudget    = 24000
	contextItemBudget     = 8000
	contextTruncatedLabel = "\n[truncated]"
	contextNoteReserve    = 160
)

func Build(request contract.Request) transport.Prompt {
	var builder strings.Builder
	builder.WriteString(buildProtocolBlock(request.PromptOverrides))
	builder.WriteString("\n\n")
	builder.WriteString(buildTaskBlock(request))
	builder.WriteString("\n\n")
	contextBlock, promptMeta := buildContextBlock(request.Context)
	builder.WriteString(contextBlock)
	builder.WriteString("\n\n")
	builder.WriteString(buildOutputBlock(request.ContentFormat, request.PromptOverrides))
	return transport.Prompt{Text: builder.String(), Meta: promptMeta}
}

func buildProtocolBlock(overrides *contract.PromptOverrides) string {
	if overrides != nil && overrides.Protocol != nil {
		return *overrides.Protocol
	}
	return `<protocol>
You are Mira, a read-only sidecar assistant used by an AI orchestrator.
Answer only from the task and provided context.

Capabilities:
- Analyze provided code, diffs, and documents.
- Read caller-provided Feishu/Lark cloud document URLs.
- Produce grounded summaries, reviews, explanations, and structured judgments.

Boundaries:
- No shell execution, file writes, or arbitrary browsing.
- Remote document reading is limited to caller-provided URLs.
- If required context or a caller-provided URL cannot be read, say so explicitly.
</protocol>`
}

func buildTaskBlock(request contract.Request) string {
	var builder strings.Builder
	builder.WriteString("<task")
	writeAttrIfPresent(&builder, "request_id", request.RequestID)
	writeAttrIfPresent(&builder, "session_id", request.Session.SessionID)
	writeAttrIfPresent(&builder, "workspace_root", request.Session.ContextHint.WorkspaceRoot)
	writeAttrIfPresent(&builder, "git_branch", request.Session.ContextHint.GitBranch)
	if request.TimeoutSec > 0 {
		fmt.Fprintf(&builder, ` timeout_sec=%q`, fmt.Sprintf("%d", request.TimeoutSec))
	}
	fmt.Fprintf(&builder, ` max_tokens=%q`, fmt.Sprintf("%d", request.MaxTokens))
	fmt.Fprintf(&builder, ` content_format=%q`, request.ContentFormat)
	builder.WriteString(">\n")
	if len(request.Constraints) > 0 {
		builder.WriteString("Constraints:\n")
		for _, item := range request.Constraints {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("User task:\n")
	builder.WriteString(request.Task)
	if request.TimeoutSec > 0 {
		builder.WriteString("\n\nPriority guidance:\n")
		builder.WriteString(timeoutInstruction(request.TimeoutSec))
	}
	builder.WriteString("\n</task>")
	return builder.String()
}

func buildContextBlock(context contract.Context) (string, *contract.PromptMeta) {
	var inner strings.Builder
	remaining := contextTotalBudget
	totalItems := countContextItems(context)
	included := 0
	omitted := 0
	if strings.TrimSpace(context.Diff) != "" {
		if !appendBudgetedBlock(&inner, "diff", nil, context.Diff, &remaining, contextNoteReserve) {
			omitted++
		} else {
			included++
		}
	}
	for _, file := range context.Files {
		attrs := map[string]string{"path": file.Path}
		if file.Source != nil && *file.Source != "" {
			attrs["source"] = *file.Source
		}
		if file.Title != nil && *file.Title != "" {
			attrs["title"] = *file.Title
		}
		if !appendBudgetedBlock(&inner, "file", attrs, file.Content, &remaining, contextNoteReserve) {
			omitted++
		} else {
			included++
		}
	}
	for _, doc := range context.Docs {
		attrs := map[string]string{}
		if doc.Source != nil && *doc.Source != "" {
			attrs["source"] = *doc.Source
		}
		if doc.Title != nil && *doc.Title != "" {
			attrs["title"] = *doc.Title
		}
		if !appendBudgetedBlock(&inner, "doc", attrs, doc.Content, &remaining, contextNoteReserve) {
			omitted++
		} else {
			included++
		}
	}
	if omitted > 0 {
		appendBudgetedBlock(&inner, "note", nil, fmt.Sprintf("Context truncated: %d additional item(s) omitted due to prompt budget.", omitted), &remaining, 0)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "<context total_items=%q included=%q omitted=%q>\n", fmt.Sprintf("%d", totalItems), fmt.Sprintf("%d", included), fmt.Sprintf("%d", omitted))
	builder.WriteString(inner.String())
	builder.WriteString("</context>")
	return builder.String(), &contract.PromptMeta{
		ContextTotalItems: totalItems,
		ContextIncluded:   included,
		ContextOmitted:    omitted,
	}
}

func buildOutputBlock(contentFormat string, overrides *contract.PromptOverrides) string {
	if overrides != nil && overrides.OutputInstructions != nil {
		return "<output>\n" + outputOverrideText(contentFormat, *overrides.OutputInstructions) + "\n</output>"
	}
	return "<output>\n" + outputInstructions(contentFormat) + "\n</output>"
}

func writeAttrIfPresent(builder *strings.Builder, name string, value *string) {
	if value == nil || *value == "" {
		return
	}
	fmt.Fprintf(builder, ` %s=%q`, name, *value)
}

func timeoutInstruction(timeoutSec int) string {
	if timeoutSec <= 0 {
		return ""
	}
	return fmt.Sprintf("The caller may terminate this request after %d seconds. Prioritize a correct, concise answer over exhaustive coverage.", timeoutSec)
}

func outputInstructions(contentFormat string) string {
	switch contentFormat {
	case "auto":
		return "Default to markdown. Return structured JSON only when the task explicitly asks for machine-readable output. Return plain text only when the task explicitly asks for plain text or a minimal single-string reply."
	case "structured":
		return "Return exactly one JSON value with no markdown fences, no prose preamble, and no trailing explanation."
	case "markdown":
		return "Return a direct markdown answer. Use headings and bullets when they improve scanability. Do not return JSON. Do not wrap the entire answer in a single code fence."
	default:
		return "Return direct plain text only. Do not return JSON, markdown headings, or code fences."
	}
}

func outputOverrideText(contentFormat, override string) string {
	if contentFormat == "auto" {
		return override
	}
	return "Content format requested: " + contentFormat + ".\n" + override
}

func writeContentBlock(builder *strings.Builder, name string, attrs map[string]string, content string) {
	builder.WriteString("<")
	builder.WriteString(name)
	for _, key := range sortedKeys(attrs) {
		fmt.Fprintf(builder, ` %s=%q`, key, attrs[key])
	}
	builder.WriteString(">\n")
	builder.WriteString(content)
	if content == "" || !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("</")
	builder.WriteString(name)
	builder.WriteString(">\n")
}

func appendBudgetedBlock(builder *strings.Builder, name string, attrs map[string]string, content string, remaining *int, reserve int) bool {
	budget := minInt(contextItemBudget, *remaining-reserve)
	for budget > 0 {
		budgeted := budgetContent(content, budget)
		if budgeted == "" {
			return false
		}
		var temp strings.Builder
		writeContentBlock(&temp, name, attrs, budgeted)
		block := temp.String()
		blockSize := len([]rune(block))
		if blockSize <= *remaining {
			builder.WriteString(block)
			*remaining -= blockSize
			return true
		}
		budget -= blockSize - *remaining
	}
	return false
}

func budgetContent(content string, budget int) string {
	if budget <= 0 {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= budget {
		return content
	}
	labelRunes := []rune(contextTruncatedLabel)
	if budget <= len(labelRunes) {
		return ""
	}
	trimmed := strings.TrimRight(string(runes[:budget-len(labelRunes)]), "\n")
	return trimmed + contextTruncatedLabel
}

func countContextItems(context contract.Context) int {
	total := len(context.Files) + len(context.Docs)
	if strings.TrimSpace(context.Diff) != "" {
		total++
	}
	return total
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
