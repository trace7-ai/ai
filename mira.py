#!/usr/bin/env python3
"""Thin AI-first dispatcher for Mira."""

import sys

VERSION = "1.0"
ASK_HELP = """Mira AI-First CLI

Usage
  mira ask --task "..." [--role planner|reviewer] [--session ID] [--context-file PATH] [--file PATH] [--format json]
  mira ask --input-file request.json --format json
  mira "task text"

Compatibility
  mira login
  mira status
  mira model [name]
"""

LEGACY_COMMANDS = {"login", "status", "model"}


def _run_ask(args: list[str]) -> int:
    from mira_sidecar_main import main as sidecar_main

    return sidecar_main(args)


def _run_legacy() -> int:
    from mira_legacy_cli import main as legacy_main

    result = legacy_main()
    return 0 if result is None else int(result)


def _print_help() -> int:
    print(ASK_HELP)
    return 0


def main() -> int:
    args = sys.argv[1:]
    if not args:
        return _print_help()
    cmd = args[0]
    if cmd in ("help", "--help", "-h"):
        return _print_help()
    if cmd in ("version", "--version", "-v"):
        print(f"mira v{VERSION}")
        return 0
    if cmd == "ask":
        return _run_ask(args[1:])
    if cmd in LEGACY_COMMANDS:
        return _run_legacy()
    if cmd in {"update", "history", "mcp"}:
        print(f"unsupported in ai-first shell: {cmd}", file=sys.stderr)
        return 2
    if cmd.startswith("-"):
        print(f"unknown option: {cmd}", file=sys.stderr)
        return 2
    return _run_ask(args)


if __name__ == "__main__":
    sys.exit(main())
