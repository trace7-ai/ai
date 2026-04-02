# Mira CLI Contract

## Verified Surface

- Built entrypoint: `./mira` after `go build -o mira ./cmd/mira`
- Source entrypoint: `go run ./cmd/mira`
- Advanced entrypoint: `./mira ask`
- Schema version: `v1`
- Stable roles: `planner`, `reader`, `reviewer`
- Stable output wrapper: JSON envelope on stdout

## Top-Level Dispatch

- `./mira "..."` forwards the bare task to `ask` mode.
- `./mira ask ...` accepts advanced flags.
- `go run ./cmd/mira "..."` and `go run ./cmd/mira ask ...` follow the same routing and response contract.
- `help`, `--help`, `-h` print the short AI-first usage text.
- `version`, `--version`, `-v` print `mira v1.0`.
- `login`, `status`, `model`, `update`, `history`, `mcp` are explicitly unsupported and exit with code `2`.
- Any unknown top-level option also exits with code `2`.

## Ask Flags

Supported flags under `ask`:

- `--input-file`
- `--format` with `json` only
- `--content-format` with `auto|structured|markdown|text`
- `--role` with `planner|reader|reviewer`
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
- `--input-file` mode rejects `--session`, `--context-file`, `--file`, `--request-id`, `--task-description`, `--git-branch`, `--workspace-root`, `--timeout-sec`, and `--max-tokens`.
- `ask --help` is not implemented as argparse help. It currently returns an `invalid_request` JSON response.

## Role Contract

### Planner

- Default content type: `structured`
- Allowed content type: `structured`
- Result keys:
  - `summary`
  - `plan`
  - `risks`
  - `open_questions`
  - `validation`
- Allowed capability set in code: `file_read`

### Reader

- Default content type: `markdown`
- Allowed content types: `structured`, `markdown`, `text`
- Result keys for structured mode:
  - `summary`
  - `key_points`
  - `sections`
  - `open_questions`
- Allowed capability set in code: `file_read`

### Reviewer

- Default content type: `structured`
- Allowed content type: `structured`
- Result keys:
  - `summary`
  - `verdict`
  - `findings`
  - `open_questions`
- Allowed capability set in code: `file_read`

## Auto-Routing Heuristics

- If `context.diff` is non-empty, route to `reviewer`.
- If the task contains review keywords and the attached files look like code, route to `reviewer`.
- If the task contains plan keywords, route to `planner`.
- If context contains docs, or attached files look like docs, route to `reader`.
- If the task contains a URL or read/summarize keywords, route to `reader`.
- Otherwise route to `planner`.

## Context And File Rules

`--context-file` supports two shapes:

1. JSON object with either a top-level `context` object or raw `diff/files/docs`
2. Any non-JSON text file, which becomes a single `docs[0].content` entry

`--file` uses the local file manifest:

- paths must be relative, not absolute
- paths must stay inside `workspace_root`
- duplicates are rejected
- max file count: `20`
- max total bytes: `512 KiB`
- per-file cap: `128 KiB`
- files are attached as read-only inline context

## Session Rules

- Default mode is `ephemeral`
- `--session <id>` switches to sticky mode
- sticky mode requires `session.session_id`
- `branch` mode is defined in schema but rejected as not implemented in `v1`
- invalid or expired sticky sessions return machine-readable session errors

## Response Envelope

Every run prints a JSON object on stdout.

Success fields:

- `version`
- `status = "ok"`
- `role`
- `request_id`
- `model`
- `content_type`
- `result`
- `errors = []`
- `token_usage`
- `truncated`
- `files_read`
- `session`

Error fields:

- `status = "error"`
- `result = null`
- `errors[0].code`
- `errors[0].message`

## Exit Codes

- `0`: success
- `1`: execution failed
- `2`: invalid request or invalid session style failure
- `3`: timeout

## Verified Pitfalls

- Build the binary first if you want the stable `./mira` entrypoint.
- Advanced flags belong after `ask`, not after top-level `./mira`.
- `reader` may self-describe roles or tools that do not exist in this repo contract.
- The transport client sends prompts to remote Mira but does not itself prove that remote tools are disabled.
- Because of that, do not use Mira's self-description as the authority for capability discovery. Use local code and tests instead.

## Useful Checks

- CLI dispatch and top-level behavior: `pkg/app/run_test.go`
- Request loading and ask-flag validation: `pkg/cli/load_request_test.go`
- Structured parsing, markdown/text handling, and timeout behavior: `pkg/runner/runner_test.go`
- Sticky session and session-store behavior: `pkg/service/service_test.go`, `pkg/session/store_test.go`
