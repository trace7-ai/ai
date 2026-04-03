package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"mira/pkg/cli"
	"mira/pkg/contract"
	jobpkg "mira/pkg/job"
	"mira/pkg/sdk"
	"mira/pkg/session"
)

const version = "1.0"

const askHelp = `Mira AI-First CLI

Usage
  mira "task text"
  mira ask --task "..." [advanced overrides]
  mira submit --task "..." [advanced overrides]
  mira jobs [--limit N]
  mira poll <job-id>
  mira wait <job-id>
  mira history [session-id] [--limit N] [--query text]
  mira ask --input-file request.json --format json

Unified Entry
  The CLI focuses on context collection, sticky sessions, and response transport.
  It no longer routes tasks into planner / reader / reviewer modes.
  --role is accepted only as a legacy compatibility alias and maps to assistant.
  Prompt-mode asks reuse the current workspace task-chain session by default unless --ephemeral is set.
  Use --new-session to detach from the current task chain and start a fresh sticky session.
  By default, the CLI does not apply a local timeout unless --timeout-sec is set.
`

var defaultTransportFactory = sdk.DefaultTransport

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
	case "submit":
		return runSubmit(args[1:], stdout)
	case "jobs":
		return runJobs(args[1:], stdout)
	case "poll":
		return runPoll(args[1:], stdout)
	case "wait":
		return runWait(args[1:], stdout)
	case "history":
		return runHistory(args[1:], stdout)
	case "__job-worker":
		return runJobWorker(args[1:], stderr)
	case "login", "status", "model", "update", "mcp":
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
	response, exitCode, err := sdk.Ask(request, &sdk.Options{TransportFactory: defaultTransportFactory})
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

type historyResponse struct {
	Sessions []session.SessionSummary `json:"sessions,omitempty"`
	Session  *session.Record          `json:"session,omitempty"`
	Entries  []session.JournalEntry   `json:"entries,omitempty"`
	Query    string                   `json:"query,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

func runHistory(args []string, stdout io.Writer) int {
	store, err := session.New("")
	if err != nil {
		return printHistory(stdout, historyResponse{Error: err.Error()}, 2)
	}
	query, sessionID, limit, err := parseHistoryArgs(args)
	if err != nil {
		return printHistory(stdout, historyResponse{Error: err.Error()}, 2)
	}
	if sessionID != "" {
		record, err := store.Load(sessionID)
		if err != nil {
			return printHistory(stdout, historyResponse{Error: err.Error()}, 2)
		}
		entries, err := store.ReadJournal(sessionID, limit)
		if err != nil {
			return printHistory(stdout, historyResponse{Error: err.Error()}, 2)
		}
		return printHistory(stdout, historyResponse{Session: record, Entries: entries}, 0)
	}
	if query != "" {
		entries, err := store.SearchJournal(query, limit)
		if err != nil {
			return printHistory(stdout, historyResponse{Error: err.Error()}, 2)
		}
		return printHistory(stdout, historyResponse{Query: query, Entries: entries}, 0)
	}
	sessions, err := store.ListSessions(limit)
	if err != nil {
		return printHistory(stdout, historyResponse{Error: err.Error()}, 2)
	}
	return printHistory(stdout, historyResponse{Sessions: sessions}, 0)
}

func parseHistoryArgs(args []string) (string, string, int, error) {
	query := ""
	sessionID := ""
	limit := 20
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--query":
			if index+1 >= len(args) {
				return "", "", 0, fmt.Errorf("flag needs an argument: %s", arg)
			}
			query = args[index+1]
			index++
		case "--limit":
			if index+1 >= len(args) {
				return "", "", 0, fmt.Errorf("flag needs an argument: %s", arg)
			}
			value, err := strconv.Atoi(args[index+1])
			if err != nil || value <= 0 {
				return "", "", 0, fmt.Errorf("limit must be a positive integer")
			}
			limit = value
			index++
		default:
			if len(arg) > 2 && arg[:2] == "--" {
				return "", "", 0, fmt.Errorf("unrecognized arguments: %s", arg)
			}
			if sessionID != "" {
				return "", "", 0, fmt.Errorf("history accepts at most one session id")
			}
			sessionID = arg
		}
	}
	return query, sessionID, limit, nil
}

func printHistory(writer io.Writer, payload historyResponse, exitCode int) int {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
	return exitCode
}

type jobEnvelope struct {
	Job      *jobpkg.Record     `json:"job,omitempty"`
	Jobs     []jobpkg.Record    `json:"jobs,omitempty"`
	Response *contract.Response `json:"response,omitempty"`
	Error    string             `json:"error,omitempty"`
}

func runSubmit(args []string, stdout io.Writer) int {
	cwd, request, err := loadCLIRequest(args)
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	store, err := jobpkg.New("")
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	record, err := store.Create(request)
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	pid, err := startWorkerProcess(record.JobID, workerDir(request, cwd))
	if err != nil {
		failed := record.Fail(err.Error(), 2, nowISO())
		_ = store.Save(failed)
		return printJobEnvelope(stdout, jobEnvelope{Job: &failed, Error: err.Error()}, 2)
	}
	record.WorkerPID = &pid
	if err := store.Save(record); err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	return printJobEnvelope(stdout, jobEnvelope{Job: &record}, 0)
}

func runJobs(args []string, stdout io.Writer) int {
	limit, err := parseJobsArgs(args)
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	store, err := jobpkg.New("")
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	jobs, err := store.List(limit)
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	return printJobEnvelope(stdout, jobEnvelope{Jobs: jobs}, 0)
}

func runPoll(args []string, stdout io.Writer) int {
	jobID, err := parseSingleJobID(args, "poll")
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	return printJobState(stdout, jobID)
}

func runWait(args []string, stdout io.Writer) int {
	jobID, err := parseSingleJobID(args, "wait")
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	store, err := jobpkg.New("")
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	for {
		record, err := store.Load(jobID)
		if err != nil {
			return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
		}
		if record == nil {
			return printJobEnvelope(stdout, jobEnvelope{Error: "job not found: " + jobID}, 2)
		}
		if jobpkg.IsTerminal(record.Status) {
			return printLoadedJobState(stdout, store, *record)
		}
		time.Sleep(time.Second)
	}
}

func runJobWorker(args []string, stderr io.Writer) int {
	jobID, err := parseWorkerArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return 2
	}
	exitCode, err := jobpkg.Execute(jobID, defaultTransportFactory)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return exitCode
	}
	return exitCode
}

func loadCLIRequest(args []string) (string, contract.Request, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", contract.Request{}, err
	}
	request, err := cli.LoadRequestFromCLI(args, cwd)
	if err != nil {
		return "", contract.Request{}, err
	}
	return cwd, request, nil
}

func parseJobsArgs(args []string) (int, error) {
	limit := 20
	for index := 0; index < len(args); index++ {
		if args[index] != "--limit" {
			return 0, fmt.Errorf("unrecognized arguments: %s", args[index])
		}
		if index+1 >= len(args) {
			return 0, fmt.Errorf("flag needs an argument: %s", args[index])
		}
		value, err := strconv.Atoi(args[index+1])
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("limit must be a positive integer")
		}
		limit = value
		index++
	}
	return limit, nil
}

func parseSingleJobID(args []string, name string) (string, error) {
	if len(args) != 1 || args[0] == "" || args[0][0] == '-' {
		return "", fmt.Errorf("%s requires exactly one job id", name)
	}
	return args[0], nil
}

func parseWorkerArgs(args []string) (string, error) {
	if len(args) != 2 || args[0] != "--job-id" || args[1] == "" {
		return "", fmt.Errorf("__job-worker requires --job-id <id>")
	}
	return args[1], nil
}

func printJobState(stdout io.Writer, jobID string) int {
	store, err := jobpkg.New("")
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	record, err := store.Load(jobID)
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	if record == nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: "job not found: " + jobID}, 2)
	}
	return printLoadedJobState(stdout, store, *record)
}

func printLoadedJobState(stdout io.Writer, store *jobpkg.Store, record jobpkg.Record) int {
	response, err := store.LoadResponse(record.JobID)
	if err != nil {
		return printJobEnvelope(stdout, jobEnvelope{Error: err.Error()}, 2)
	}
	return printJobEnvelope(stdout, jobEnvelope{Job: &record, Response: response}, 0)
}

func printJobEnvelope(writer io.Writer, payload jobEnvelope, exitCode int) int {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
	return exitCode
}

func startWorkerProcess(jobID string, dir string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer devNull.Close()
	cmd := exec.Command(executable, "__job-worker", "--job-id", jobID)
	cmd.Dir = dir
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}

func workerDir(request contract.Request, fallback string) string {
	if request.Session.ContextHint.WorkspaceRoot != nil && *request.Session.ContextHint.WorkspaceRoot != "" {
		return *request.Session.ContextHint.WorkspaceRoot
	}
	return fallback
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05+00:00")
}
