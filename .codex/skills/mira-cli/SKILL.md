---
name: mira-cli
description: Repo-local Mira invocation contract for /Users/bytedance/.mira. Entrypoints, validation, and acceptance only.
---

# Mira CLI

## Authoritative Spec

- The source of truth is [references/contract.md](references/contract.md).
- Use this file as an index, not a duplicate of the contract.
- If this file and the contract disagree, follow the contract.

## Repo Entrypoints

- Preferred binary entrypoint: `./mira`
- Source entrypoint: `go run ./cmd/mira ...`
- In-process entrypoint: `pkg/sdk.Ask(contract.Request, *sdk.Options)`
- Do not use PATH wrappers or `which mira` as the authority for this repo.

## Minimal Pre-flight

- Verify `go.mod` and `cmd/mira/main.go` exist.
- If `./mira` is missing, build it with `go build -o mira ./cmd/mira`.
- Smoke test with `./mira ask --ephemeral --task "Reply with OK only"`.

## Invocation Choice

- Use `sdk.Ask(...)` when Codex already has a typed request in memory.
- Use `./mira ask ...` when stdout JSON transport is the integration boundary.
- Use `./mira submit ...` plus `./mira wait <job-id>` for long detached work.
- Use top-level `./mira "task text"` only for the thinnest one-shot entry.

## Context Rules

- Prefer passing Feishu/Lark/wiki/doc URLs directly to Mira first.
- Use `--context-file` for prepared `diff/files/docs`.
- Use `--file` only for explicit local files that matter to the task.
- Validate that prepared context is non-empty before sending it.

## Session Rules

- Session reuse and retry policy are governed by the global `mira-orchestrator` skill.
- Repo-local entrypoints still support `--session`, `--new-session`, and `--ephemeral` as defined in the contract.
- Do not change sessions because of timeout, phrasing, or content-format tweaks.

## Response Acceptance

- Stdout is always a JSON envelope.
- Check `status` before reading `result`.
- Reject empty or non-substantive `result` as failure.
- Treat `truncated = true` as a continuation signal, not a full failure.
- Exit code `0` is success, `1` execution failure, `2` invalid request, `3` timeout.

## Capability Boundaries

- Mira is a read-only sidecar from this repo.
- Do not assume shell execution, file writes, or arbitrary browsing.
- If Mira claims behavior that conflicts with the local contract, trust the local contract.
