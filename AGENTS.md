# Mira Repo Rules

These rules apply to the whole `/Users/bytedance/.mira` repository.

- If the task involves invoking Mira from this repo, read `.codex/skills/mira-cli/SKILL.md` before the first call.
- In this repo, the authoritative entrypoints are repo-local `./mira` and `go run ./cmd/mira`.
- Do not guess paths from stale PATH wrappers when the repo already defines its own CLI entrypoints.

## Mira URL-First Policy

- For Feishu/Lark/wiki/doc URLs provided by the user, Mira must be the first reader.
- Do not prefetch those cloud documents with `lark-cli`, `docs +fetch`, or `wiki` before the first Mira attempt.
- Fall back to prepared local context only after one explicit Mira failure for that URL-scoped task.
- Reuse the same Mira session for retries and followups inside one user task.
- Use `--new-session` only for unrelated work. Use `--ephemeral` only for one-shot health checks.
