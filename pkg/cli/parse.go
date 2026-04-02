package cli

import (
	"fmt"
	"strings"

	"mira/pkg/contract"
)

func ParseAskArgs(args []string) (ParsedArgs, error) {
	parsed := ParsedArgs{
		Format:        "json",
		ContentFormat: "auto",
		Files:         []string{},
		TimeoutSec:    contract.DefaultTimeoutSec,
		MaxTokens:     contract.DefaultMaxTokens,
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "--") {
			parsed.TaskParts = append(parsed.TaskParts, arg)
			continue
		}
		value, next, err := parseFlagValue(args, index, arg)
		if err != nil {
			return ParsedArgs{}, err
		}
		index = next
		if err := applyFlag(&parsed, arg, value); err != nil {
			return ParsedArgs{}, err
		}
	}
	if parsed.Task != "" && len(parsed.TaskParts) > 0 {
		return ParsedArgs{}, fmt.Errorf("cannot combine --task with positional task")
	}
	parsed.InlineTask = parsed.Task
	if parsed.InlineTask == "" {
		parsed.InlineTask = strings.TrimSpace(strings.Join(parsed.TaskParts, " "))
	}
	if parsed.Format != "json" {
		return ParsedArgs{}, fmt.Errorf("only --format json is supported in v1")
	}
	if parsed.InputFile != "" && parsed.InlineTask != "" {
		return ParsedArgs{}, fmt.Errorf("cannot combine --input-file with inline task")
	}
	if parsed.InputFile == "" && parsed.InlineTask == "" {
		return ParsedArgs{}, fmt.Errorf("missing task or --input-file")
	}
	return parsed, nil
}

func parseFlagValue(args []string, index int, name string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("flag needs an argument: %s", name)
	}
	return args[index+1], index + 1, nil
}

func applyFlag(parsed *ParsedArgs, name, value string) error {
	switch name {
	case "--input-file":
		parsed.InputFile = value
	case "--format":
		parsed.Format = value
	case "--content-format":
		parsed.ContentFormat = value
	case "--role":
		parsed.Role = value
	case "--task":
		parsed.Task = value
	case "--session":
		parsed.Session = value
	case "--context-file":
		parsed.ContextFile = value
	case "--file":
		parsed.Files = append(parsed.Files, value)
	case "--workspace-root":
		parsed.WorkspaceRoot = value
	case "--task-description":
		parsed.TaskDesc = value
	case "--git-branch":
		parsed.GitBranch = value
	case "--timeout-sec":
		return assignInt(&parsed.TimeoutSec, value, "timeout-sec")
	case "--max-tokens":
		return assignInt(&parsed.MaxTokens, value, "max-tokens")
	case "--request-id":
		parsed.RequestID = value
	default:
		return fmt.Errorf("unrecognized arguments: %s", name)
	}
	return nil
}
