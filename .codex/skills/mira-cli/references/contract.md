# Mira CLI Contract

## Verified Surface

- Repo-root built entrypoint: `./mira` after `go build -o mira ./cmd/mira`
- Repo-root source entrypoint: `go run ./cmd/mira`
- Advanced entrypoints: `./mira ask`, `./mira submit`, `./mira jobs`, `./mira poll`, `./mira wait`
- Programmatic embedding entrypoint: `pkg/sdk.Ask(contract.Request, *sdk.Options)`
- Schema version: `v1`
- Stable role: `assistant`
- Stable output wrapper: JSON envelope on stdout

Repo-root assumptions:
- `go.mod` exists
- `cmd/mira/main.go` exists
- callers should treat this repository root as the authoritative invocation location

## Embedding

- Codex-style callers that already hold an in-memory `contract.Request` should prefer `pkg/sdk.Ask(...)` over shelling out to `./mira ask ...`.
- `pkg/sdk.NewRequest(task).Build()` is the minimal side-effect-free way to assemble a validated `contract.Request` in-process before calling `pkg/sdk.Ask(...)`.
- `pkg/sdk.AskWithBuilder(builder, options)` is the thinnest happy-path embedding entrypoint when the caller does not need to inspect the intermediate `contract.Request`.
- `contract.Request.prompt_overrides` may override the default prompt protocol text or output instructions for Codex-only embedding when the caller needs tighter prompt control.
- When `prompt_overrides.output_instructions` is set, the prompt still preserves the requested `content_format` semantics in the `<output>` block.
- `sdk.Options.OnChunk` streams deduplicated result text increments to the caller while the request is still running; the callback sees only text that will enter the final assembled result.
- `response.prompt_meta` reports prompt-side context accounting so callers can tell whether missing quality may be caused by prompt-budget truncation rather than model misunderstanding.
- The SDK entrypoint reuses the same `service.Service.Run(...)` path as the CLI, so sticky sessions, journal replay, hydrated file access, and structured `session` payloads keep the same behavior.
- Use the CLI when argument parsing, detached jobs, or stdout JSON transport is the desired integration boundary.

## Top-Level Dispatch

- `./mira "..."` forwards the bare task to `ask` mode.
- `./mira ask ...` accepts advanced flags.
- `./mira submit ...` enqueues a background job and returns immediately with a `job_id`.
- `./mira jobs` lists persisted jobs.
- `./mira poll <job-id>` reads the latest job status and response if available.
- `./mira wait <job-id>` blocks until the job reaches a terminal state and then returns the same payload as `poll`.
- `./mira history ...` reads local session metadata and journal history.
- `go run ./cmd/mira "..."` and `go run ./cmd/mira ask ...` follow the same routing and response contract.
- `help`, `--help`, `-h` print the short AI-first usage text.
- `version`, `--version`, `-v` print `mira v1.0`.
- `login`, `status`, `model`, `update`, `mcp` are explicitly unsupported and exit with code `2`.
- Any unknown top-level option also exits with code `2`.

## Ask Flags

Supported flags under `ask`:

- `--input-file`
- `--format` with `json` only
- `--content-format` with `auto|structured|markdown|text`
- `--ephemeral`
- `--new-session`
- `--role` with legacy aliases `planner|reader|reviewer|assistant`
- `--task`
- `--session`
- `--context-file`
- `--file` repeated
- `--workspace-root`
- `--task-description`
- `--git-branch`
- `--timeout-sec`
- `--max-tokens`
- `--request-id`

Important constraints:

- Do not combine `--task` with positional task text.
- Do not combine `--input-file` with inline task text.
- Do not combine `--session` with `--ephemeral`.
- Do not combine `--session` with `--new-session`.
- Do not combine `--new-session` with `--ephemeral`.
- `--input-file` mode rejects `--session`, `--ephemeral`, `--new-session`, `--context-file`, `--file`, `--request-id`, `--task-description`, `--git-branch`, `--workspace-root`, `--timeout-sec`, and `--max-tokens`.
- `ask --help` is not implemented as argparse help. It currently returns an `invalid_request` JSON response.

## Defaults And Limits

- `DefaultTimeoutSec`: `0` meaning no local timeout
- `MaxTimeoutSec`: `900`
- `DefaultMaxTokens`: `32000`
- `MaxMaxTokens`: `32000`
- `DefaultMaxFileSize`: `512 KiB`
- `timeout_sec` valid values: `0` or `10..900`

## Assistant Contract

- The request role defaults to `assistant` when omitted.
- Legacy role aliases normalize to `assistant`.
- Default content type is `auto`.
- Allowed content types are `auto|structured|markdown|text`.
- `auto` lets the model choose the response shape.
- `structured` still requires valid JSON, but no role-specific schema keys are enforced.

## Context And File Rules

`--context-file` supports two shapes:

1. JSON object with either a top-level `context` object or raw `diff/files/docs`
2. Any non-JSON text file, which becomes a single `docs[0].content` entry

Prompt assembly notes:

- The internal prompt renders context as compact `<diff>`, `<file>`, and `<doc>` blocks instead of pretty-printed JSON, to avoid large prompt inflation from escaped source text.
- The `<context>` block includes `total_items`, `included`, and `omitted` attributes so callers can tell when prompt budget excluded context items.
- Sticky session replay injects a summarized `session_journal` doc. Historical `task`, `summary`, and `carry_forward` text may be truncated for prompt efficiency.
- Prompt assembly applies a builder-side context budget. Oversized blocks may be truncated with `[truncated]`, and later context items may be omitted with an explicit note when the prompt budget is exhausted.

`--file` uses the local file manifest:

- paths must be relative, not absolute
- paths must stay inside `workspace_root`
- duplicates are rejected
- max file count: `20`
- max total bytes: `512 KiB`
- per-file cap: `128 KiB`
- files are attached as read-only inline context

## Session Rules

- The raw request protocol defaults to `ephemeral` when `session.mode` is omitted
- Prompt-mode CLI calls first try to reuse the current workspace task-chain session and only create a fresh auto session when no task-chain session is pinned
- `--session <id>` switches to sticky mode with an explicit id
- `--ephemeral` forces one-shot prompt-mode behavior
- `--new-session` replaces the current workspace task pin with a fresh sticky session
- `MIRA_SESSION_ID` is accepted as a prompt-mode fallback when `--session` is omitted and neither `--new-session` nor `--ephemeral` is set
- sticky mode requires `session.session_id`
- local sticky sessions are durable by default and do not expire on a local TTL
- sticky sessions append local journal entries at `sessions/<session-id>.journal.jsonl`
- sticky sessions inject recent journal summaries back into prepared context on follow-up turns
- ephemeral responses still return a structured `session` payload with `status = "ephemeral"`, `session_id = ""`, and `reused = false`
- local sticky-session compatibility checks only enforce workspace-root consistency
- `branch` mode is defined in schema but rejected as not implemented in `v1`
- sticky session failures now come from real incompatibility or remote invalidation, not local TTL expiry

## History Command

- `mira history` lists local sessions sorted by `last_active_at`
- `mira history <session-id>` returns the stored session record plus journal entries
- `mira history --query text` searches journal entries across sessions

## Async Job Commands

- Job metadata is stored under `~/.mira/jobs/`
- `submit` writes the normalized request to disk, spawns a detached worker, and returns without waiting for the remote response
- `poll` and `wait` read persisted job state instead of holding the original HTTP stream open
- completed jobs keep their response envelope in `<job-id>.response.json`
- jobs sharing the same sticky `session_id` still serialize on the existing session lock

## Response Envelope

Every run prints a JSON object on stdout.

Success fields:

- `version`
- `status = "ok"`
- `role`
- `request_id`
- `model`
- `content_type`
- `content_type = structured` when `auto` output is valid JSON
- `content_type = text` when `auto` output is plain text or markdown-like prose
- `result`
- `errors = []`
- `token_usage`
- `truncated`
  - `true` when the remote stream stop reason reports `max_tokens`
  - `false` otherwise
- `files_read`
- `session`
  - `session.session_id`
  - `session.status = "ephemeral"` with `session.session_id = ""` and `session.reused = false` for one-shot calls
  - `session.reused = true` when the call reused an active sticky session
  - `session.reused = false` for fresh sticky sessions or sticky failures before reuse

Error fields:

- `status = "error"`
- `result = null`
- `errors[0].code`
- `errors[0].message`

Caller-side acceptance:

- a successful process exit is not enough
- callers should inspect `status`
- callers should reject empty-shell responses with missing or non-substantive `result`
- callers should diagnose bad entrypoint or bad context before retrying once

## Exit Codes

- `0`: success
- `1`: execution failed
- `2`: invalid request or invalid session style failure
- `3`: timeout

## Verified Pitfalls

- Build the binary first if you want the stable `./mira` entrypoint.
- For this repo workflow, do not trust PATH-level `mira` wrappers as the authoritative entrypoint.
- Do not use `which mira` as the routing decision for repo-local Mira calls.
- Advanced flags belong after `ask`, not after top-level `./mira`.
- Changing `--content-format` or `--timeout-sec` is not a reason to open a new session for the same task.
- `--role` no longer changes internal behavior. It is a compatibility alias.
- The transport client sends prompts to remote Mira but does not itself prove that remote tools are disabled.
- Because of that, do not use Mira's self-description as the authority for capability discovery. Use local code and tests instead.

## Useful Checks

- CLI dispatch and top-level behavior: `pkg/app/run_test.go`
- Request loading and ask-flag validation: `pkg/cli/load_request_test.go`
- Structured parsing, markdown/text handling, and timeout behavior: `pkg/runner/runner_test.go`
- Sticky session and session-store behavior: `pkg/service/service_test.go`, `pkg/session/store_test.go`
