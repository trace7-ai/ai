# Mira Project Rules

## Goal

This project is evolving Mira from a general terminal assistant into a high-value sidecar subagent that can be called by a stronger external orchestrator.

Version 1 focus:
- `planner`
- `reviewer`

Do not expand role count in early iterations unless the current contract is already stable.

## Product Direction

- Treat Mira as a structured sidecar, not a second general executor.
- Keep the interactive CLI working unless a change explicitly targets TUI behavior.
- Prioritize machine-consumable outputs over terminal polish for sidecar paths.
- Prefer prepared context from the caller in v1. Do not assume Mira should explore the repo on its own.

## Batch Contract

- Version the request and response contract from the start.
- Sidecar commands must return structured JSON on stdout.
- Contract errors must be explicit and machine-readable.
- Do not hide parsing failures behind prose summaries.

## Role Boundaries

- `planner` is read-only.
- `reviewer` is read-only.
- Sidecar roles must not write files, edit files, or run unrestricted shell commands.
- Enforce role boundaries in code, not only in prompts.

## Implementation Priorities

Implement in this order:
1. Request/response schema and validation
2. Role-aware prompt and policy layer
3. Batch runner entrypoint
4. Minimal wiring into `mira.py`
5. Follow-up refactors only when needed

## Editing Guidance

- Prefer adding new focused modules instead of growing `mira.py` further.
- Keep batch logic isolated from TUI rendering and interactive state.
- Avoid broad refactors during feature introduction.
- Do not introduce silent fallback behavior.
- Surface transport, parsing, and contract failures explicitly.

## Validation

- Verify both JSON validity and CLI exit behavior.
- When batch behavior changes, test at least:
  - invalid request
  - planner request
  - reviewer request
  - non-JSON model response handling
