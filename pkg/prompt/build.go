package prompt

import (
	"fmt"
	"strings"

	"mira/pkg/contract"
	"mira/pkg/roles"
	"mira/pkg/transport"
)

func Build(request contract.Request, role roles.Spec) transport.Prompt {
	constraintsBlock := buildConstraintsBlock(request.Constraints)
	styleBlock := buildStyleBlock(role.OutputStyleGuidance(request.ContentFormat))
	return transport.Prompt{
		Text: fmt.Sprintf(
			"You are Mira acting as a %s sidecar subagent.\n%s\nGoal: %s\nTask:\n%s\n\n%s%s%s\nKeep the answer within approximately %d tokens.\nAssume the caller will terminate the request after %d seconds.\nRequest ID: %s\nSession ID: %s\nContent format: %s\n\nPrepared context:\n%s",
			role.Name,
			role.ContextGuidance,
			role.Summary,
			request.Task,
			constraintsBlock,
			styleBlock,
			outputInstructions(request.ContentFormat, role.ResultExample),
			request.MaxTokens,
			request.TimeoutSec,
			nullableString(request.RequestID),
			nullableString(request.Session.SessionID),
			request.ContentFormat,
			mustPrettyJSON(request.Context),
		),
	}
}

func buildConstraintsBlock(constraints []string) string {
	if len(constraints) == 0 {
		return ""
	}
	lines := []string{"Constraints:"}
	for _, item := range constraints {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func buildStyleBlock(style string) string {
	if style == "" {
		return ""
	}
	return style + "\n"
}
