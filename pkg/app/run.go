package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"mira/pkg/cli"
	"mira/pkg/contract"
	"mira/pkg/service"
	"mira/pkg/session"
	"mira/pkg/transport"
)

const version = "1.0"

const askHelp = `Mira AI-First CLI

Usage
  mira "task text"
  mira ask --task "..." [advanced overrides]
  mira ask --input-file request.json --format json

Auto Routing
  No --role needed for normal use.
  The CLI infers reader / planner / reviewer from the task and context.
  Use --role or --content-format only when you want to override the default route.
`

var defaultTransportFactory = func() (transport.Transport, error) {
	config, err := transport.LoadConfig()
	if err != nil {
		return nil, err
	}
	return transport.NewMiraClient(config), nil
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stdout, askHelp)
		return 0
	}
	switch args[0] {
	case "help", "--help", "-h":
		_, _ = io.WriteString(stdout, askHelp)
		return 0
	case "version", "--version", "-v":
		_, _ = io.WriteString(stdout, fmt.Sprintf("mira v%s\n", version))
		return 0
	case "ask":
		return runAsk(args[1:], stdout)
	case "login", "status", "model", "update", "history", "mcp":
		_, _ = fmt.Fprintf(stderr, "unsupported in ai-first shell: %s\n", args[0])
		return 2
	default:
		if args[0] != "" && args[0][0] == '-' {
			_, _ = fmt.Fprintf(stderr, "unknown option: %s\n", args[0])
			return 2
		}
		return runAsk(args, stdout)
	}
}

func runAsk(args []string, stdout io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		return printResponse(stdout, contract.BuildErrorResponse("invalid_request", err.Error(), nil, nil, nil), 2)
	}
	request, err := cli.LoadRequestFromCLI(args, cwd)
	if err != nil {
		return printResponse(stdout, contract.BuildErrorResponse("invalid_request", err.Error(), nil, nil, nil), 2)
	}
	client, err := defaultTransportFactory()
	if err != nil {
		role := request.Role
		return printResponse(stdout, contract.BuildErrorResponse("invalid_request", err.Error(), &role, request.RequestID, nil), 2)
	}
	store, err := session.New("")
	if err != nil {
		return printResponse(stdout, contract.BuildErrorResponse("invalid_request", err.Error(), nil, request.RequestID, nil), 2)
	}
	response, exitCode, err := service.Service{Client: client, Store: store}.Run(request)
	if err != nil {
		role := request.Role
		return printResponse(stdout, contract.BuildErrorResponse("invalid_request", err.Error(), &role, request.RequestID, nil), 2)
	}
	return printResponse(stdout, response, exitCode)
}

func printResponse(writer io.Writer, response contract.Response, exitCode int) int {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(response)
	return exitCode
}
