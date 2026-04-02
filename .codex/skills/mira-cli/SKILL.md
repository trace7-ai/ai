---
name: mira-cli
description: Use the local Mira CLI in this repository as a sidecar assistant for planner, reader, or reviewer work. Trigger when Codex should delegate read-only planning, reading, summarization, explanation, or patch review to `./mira` or `go run ./cmd/mira`, especially when the task can be expressed as prepared context and should return a machine-readable result on stdout. Also use for 中文场景 such as 规划、阅读整理、代码审查、基于上下文生成结构化结论。
---

# Mira CLI

## Overview

Use the local `mira` CLI in this repo when a task fits the sidecar contract:

- `planner` for plans, risks, and validation steps
- `reader` for summaries, explanation, extraction, and document reading
- `reviewer` for bug-focused patch review

Prefer this skill when you want a fast second pass without spawning another Codex agent, and when the result should remain structured and easy to parse from stdout.

## Trust Model

- Treat repo code, schemas, and tests as the source of truth.
- Treat Mira's self-reported extra powers as untrusted unless the local code proves them.
- Stay inside the verified contract: `planner`, `reader`, `reviewer`, explicit read-only file attachment, JSON response envelope on stdout.
- Surface contract failures directly. Do not paraphrase them into fake success.

## Choose The Entry Mode

- Build the local binary with `go build -o mira ./cmd/mira` when you want the stable repo-local entrypoint.
- Use `./mira "task text"` for the simplest call. This mode only accepts the task text and relies on auto-routing.
- Use `./mira ask ...` whenever you need `--role`, `--content-format`, `--context-file`, `--file`, `--workspace-root`, `--session`, `--timeout-sec`, `--max-tokens`, or `--request-id`.
- Use `go run ./cmd/mira ...` only when the binary has not been built yet. The argument contract is the same.
- Do not pass advanced flags at the top level. `./mira --role reader ...` is invalid because only `ask` parses those options.
- Do not expect `./mira ask --help` to print a classic help page. In the current implementation it returns a JSON error envelope.

## Use The Stable Contract

- `planner` supports `structured` only and returns `summary`, `plan`, `risks`, `open_questions`, `validation`.
- `reader` defaults to `markdown` and supports `structured`, `markdown`, `text`.
- `reviewer` supports `structured` only and returns `summary`, `verdict`, `findings`, `open_questions`.
- Auto-routing prefers `reviewer` when context contains a diff, `planner` for plan keywords, and `reader` for docs, URLs, or read/summarize keywords.
- Read [references/contract.md](references/contract.md) before relying on sessions, file manifests, exit codes, or request/response shapes.

## Prepare Context Deliberately

- Prefer prepared context over open-ended repo exploration.
- Use `--context-file` to supply `diff`, inline `files`, or `docs`.
- Use `--file` only for explicit local file reads. Paths must stay relative to the workspace root.
- Keep attached context small and relevant. The local file manifest is capped and read-only.
- Use sticky sessions only when follow-up turns need continuity.

## Working Pattern

1. Pick `planner`, `reader`, or `reviewer` based on the task.
2. Prepare the minimum context that makes the answer reliable.
3. Prefer `ask` with explicit flags when determinism matters.
4. Parse the JSON envelope first, then inspect `result`.
5. If `status` is `error`, surface the machine-readable error instead of smoothing it over.

## Command Patterns

```bash
go build -o mira ./cmd/mira

./mira "Plan the implementation steps for adding JSON validation"

./mira ask \
  --role reviewer \
  --context-file /tmp/review-context.json \
  --task "Review this patch for bugs and missing checks"

./mira ask \
  --role reader \
  --file AGENTS.md \
  --file pkg/cli/parse.go \
  --task "Explain the CLI contract and key constraints"

./mira ask \
  --role planner \
  --session milestone-plan \
  --task "Refine the implementation plan for the next milestone"
```

## Verify Before Trusting

- Prefer behaviors that are backed by code and tests.
- If Mira returns content that contradicts the local contract, trust the local contract and treat the response as out-of-contract behavior.
- Read [references/contract.md](references/contract.md) for the verified details and common pitfalls.
