import argparse
import json
import os
from pathlib import Path

from mira_roles import ROLE_REGISTRY
from sidecar_contract import DEFAULT_MAX_TOKENS, DEFAULT_TIMEOUT_SEC, normalize_request
from sidecar_runner import load_request_file


class SidecarCLIError(ValueError):
    pass


class _SidecarArgumentParser(argparse.ArgumentParser):
    def error(self, message):
        raise SidecarCLIError(message)


def load_request_from_cli(args: list[str]) -> dict:
    parsed = _parse_args(args)
    if parsed.input_file:
        return _load_request_file_mode(parsed)
    return _build_prompt_mode_request(parsed)


def _parse_args(args: list[str]):
    parser = _SidecarArgumentParser(add_help=False, prog="mira ask")
    parser.add_argument("--input-file")
    parser.add_argument("--format", default="json")
    parser.add_argument("--content-format", choices=("auto", "structured", "markdown", "text"), default="auto")
    parser.add_argument("--role", choices=sorted(ROLE_REGISTRY))
    parser.add_argument("--task")
    parser.add_argument("--session")
    parser.add_argument("--context-file")
    parser.add_argument("--file", action="append", dest="files", default=[])
    parser.add_argument("--workspace-root")
    parser.add_argument("--task-description")
    parser.add_argument("--git-branch")
    parser.add_argument("--timeout-sec", type=int, default=DEFAULT_TIMEOUT_SEC)
    parser.add_argument("--max-tokens", type=int, default=DEFAULT_MAX_TOKENS)
    parser.add_argument("--request-id")
    parser.add_argument("task_parts", nargs="*")
    parsed = parser.parse_args(args)
    if parsed.task and parsed.task_parts:
        raise SidecarCLIError("cannot combine --task with positional task")
    parsed.inline_task = parsed.task or " ".join(parsed.task_parts).strip()
    if parsed.format != "json":
        raise SidecarCLIError("only --format json is supported in v1")
    if parsed.input_file and parsed.inline_task:
        raise SidecarCLIError("cannot combine --input-file with inline task")
    if not parsed.input_file and not parsed.inline_task:
        raise SidecarCLIError("missing task or --input-file")
    return parsed


def _load_request_file_mode(parsed) -> dict:
    _reject_file_mode_overrides(parsed)
    request = load_request_file(parsed.input_file, normalize_body=False)
    if parsed.role:
        request["role"] = parsed.role
    if parsed.content_format != "auto":
        request["content_format"] = parsed.content_format
    return normalize_request(request)


def _reject_file_mode_overrides(parsed):
    for name in ("session", "context_file", "files", "request_id", "task_description", "git_branch"):
        value = getattr(parsed, name)
        if value not in (None, [], ""):
            raise SidecarCLIError(f"{name.replace('_', '-')} is not supported with --input-file")
    if parsed.workspace_root is not None:
        raise SidecarCLIError("workspace-root is not supported with --input-file")
    if parsed.timeout_sec != DEFAULT_TIMEOUT_SEC:
        raise SidecarCLIError("timeout-sec is not supported with --input-file")
    if parsed.max_tokens != DEFAULT_MAX_TOKENS:
        raise SidecarCLIError("max-tokens is not supported with --input-file")


def _build_prompt_mode_request(parsed) -> dict:
    workspace_root = _resolve_workspace_root(parsed.workspace_root)
    request = {
        "version": "v1",
        "role": parsed.role or "planner",
        "request_id": parsed.request_id,
        "content_format": parsed.content_format,
        "session": {
            "mode": "sticky" if parsed.session else "ephemeral",
            "session_id": parsed.session,
            "context_hint": {
                "workspace_root": workspace_root,
                "task_description": parsed.task_description or parsed.inline_task,
                "git_branch": parsed.git_branch,
            },
        },
        "file_manifest": {
            "mode": "explicit" if parsed.files else "none",
            "paths": parsed.files,
            "max_total_bytes": 512 * 1024,
            "read_only": True,
        },
        "task": parsed.inline_task,
        "constraints": [],
        "context": _load_context_payload(parsed.context_file),
        "max_tokens": parsed.max_tokens,
        "timeout_sec": parsed.timeout_sec,
    }
    return normalize_request(request)


def _resolve_workspace_root(raw_path: str | None) -> str:
    path = Path(raw_path or os.getcwd()).expanduser().resolve()
    return str(path)


def _load_context_payload(raw_path: str | None) -> dict:
    if not raw_path:
        return {"diff": "", "files": [], "docs": []}
    path = Path(raw_path).expanduser().resolve()
    text = path.read_text(encoding="utf-8", errors="replace")
    if path.suffix.lower() == ".json":
        try:
            data = json.loads(text)
        except json.JSONDecodeError:
            pass
        else:
            return _normalize_context_payload(data, path)
    return {
        "diff": "",
        "files": [],
        "docs": [{"source": str(path), "content": text}],
    }


def _normalize_context_payload(data, path: Path) -> dict:
    if not isinstance(data, dict):
        raise SidecarCLIError(f"context file must contain an object: {path}")
    if "context" in data:
        data = data["context"]
    if not isinstance(data, dict):
        raise SidecarCLIError(f"context field must be an object: {path}")
    return {
        "diff": data.get("diff", ""),
        "files": data.get("files", []),
        "docs": data.get("docs", []),
    }
