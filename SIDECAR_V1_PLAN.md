# Mira Sidecar V1 Plan

## Goal

Turn Mira into a high-value sidecar for a stronger external orchestrator.

Version 1 scope:
- `planner`
- `reviewer`

The sidecar is not a second general executor.

## Current Direction

- Keep the interactive CLI working.
- Remove TUI code from the sidecar runtime path.
- Keep sidecar JSON-in / JSON-out.
- Default to read-only behavior.
- Introduce sticky session reuse for the same task chain.
- Allow controlled local file reads through `file_manifest`.

## Session Model

Supported modes:
- `sticky`
- `branch`
- `ephemeral`

Version 1 implementation target:
- Implement `sticky`
- Keep `ephemeral`
- Defer `branch`

The orchestrator owns session identity and decides when to reuse or rotate sessions.

## Request Shape Additions

Planned request fields:
- `session.mode`
- `session.session_id`
- `session.parent_session_id` later
- `session.context_hint`

Planned response fields:
- `session.session_id`
- `session.turn_index`
- `session.status`
- `session.context_window_usage` later

## Module Split

Near-term target modules:
- `mira_sidecar.py`
- `mira_session.py`
- `mira_roles/base.py`
- `mira_roles/planner.py`
- `mira_roles/reviewer.py`

Keep these in `mira.py` for now:
- transport
- config
- TUI

But `mira ask` should delegate into `mira_sidecar.py`.

## Implementation Order

1. Extract `mira ask` logic into `mira_sidecar.py`
2. Add `SessionStore` in `mira_session.py`
3. Implement sticky session reuse in sidecar mode
4. Replace hardcoded role handling with `BaseRole` registry
5. Add `file_manifest` for explicit read-only file injection
6. Handle sticky session reconnect and atomic session writes
7. Re-review with Mira

## Deferred

- branch sessions
- context summary generation
- TUI extraction into its own module
- token usage accounting from upstream
