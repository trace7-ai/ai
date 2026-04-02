import json

from mira_roles import ROLE_REGISTRY, get_role

SCHEMA_VERSION = "v1"
DEFAULT_MAX_TOKENS = 4000
MAX_MAX_TOKENS = 16000
DEFAULT_TIMEOUT_SEC = 180
MAX_TIMEOUT_SEC = 600
SESSION_MODES = {"sticky", "branch", "ephemeral"}
FILE_MANIFEST_MODES = {"none", "explicit"}


def _require_string(value, field: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{field} must be a non-empty string")
    return value


def _optional_string(value, field: str):
    if value is None:
        return None
    return _require_string(value, field)


def _normalize_context_items(items, required_fields, label: str) -> list:
    if items is None:
        return []
    if not isinstance(items, list):
        raise ValueError(f"{label} must be a list")
    normalized = []
    for index, item in enumerate(items):
        if not isinstance(item, dict):
            raise ValueError(f"{label}[{index}] must be an object")
        normalized_item = {}
        for field in required_fields:
            normalized_item[field] = _require_string(item.get(field), f"{label}[{index}].{field}")
        for field, value in item.items():
            if field in required_fields or value is None:
                continue
            normalized_item[field] = _require_string(value, f"{label}[{index}].{field}")
        normalized.append(normalized_item)
    return normalized


def _normalize_session(raw_session) -> dict:
    if raw_session is None:
        raw_session = {}
    if not isinstance(raw_session, dict):
        raise ValueError("session must be an object")
    mode = raw_session.get("mode", "ephemeral")
    if mode not in SESSION_MODES:
        raise ValueError(f"unsupported session mode: {mode}")
    session_id = _optional_string(raw_session.get("session_id"), "session.session_id")
    parent_session_id = _optional_string(
        raw_session.get("parent_session_id"),
        "session.parent_session_id",
    )
    if mode == "sticky" and not session_id:
        raise ValueError("sticky session requires session.session_id")
    if mode == "branch":
        raise ValueError("branch session is not implemented in v1")
    context_hint = raw_session.get("context_hint", {})
    if context_hint is None:
        context_hint = {}
    if not isinstance(context_hint, dict):
        raise ValueError("session.context_hint must be an object")
    return {
        "mode": mode,
        "session_id": session_id,
        "parent_session_id": parent_session_id,
        "context_hint": {
            "workspace_root": _optional_string(
                context_hint.get("workspace_root"),
                "session.context_hint.workspace_root",
            ),
            "task_description": _optional_string(
                context_hint.get("task_description"),
                "session.context_hint.task_description",
            ),
            "git_branch": _optional_string(
                context_hint.get("git_branch"),
                "session.context_hint.git_branch",
            ),
        },
    }


def _normalize_file_manifest(raw_manifest) -> dict:
    if raw_manifest is None:
        raw_manifest = {}
    if not isinstance(raw_manifest, dict):
        raise ValueError("file_manifest must be an object")
    mode = raw_manifest.get("mode", "none")
    if mode not in FILE_MANIFEST_MODES:
        raise ValueError(f"unsupported file_manifest mode: {mode}")
    paths = raw_manifest.get("paths", [])
    if not isinstance(paths, list):
        raise ValueError("file_manifest.paths must be a list")
    max_total_bytes = raw_manifest.get("max_total_bytes", 512 * 1024)
    if not isinstance(max_total_bytes, int) or max_total_bytes <= 0:
        raise ValueError("file_manifest.max_total_bytes must be a positive integer")
    read_only = raw_manifest.get("read_only", True)
    if read_only is not True:
        raise ValueError("file_manifest.read_only must be true")
    return {
        "mode": mode,
        "paths": [_require_string(item, f"file_manifest.paths[{index}]") for index, item in enumerate(paths)],
        "max_total_bytes": max_total_bytes,
        "read_only": True,
    }


def normalize_request(raw: dict) -> dict:
    if not isinstance(raw, dict):
        raise ValueError("request body must be a JSON object")
    version = raw.get("version", SCHEMA_VERSION)
    if version != SCHEMA_VERSION:
        raise ValueError(f"unsupported version: {version}")
    role_name = _require_string(raw.get("role"), "role")
    role = get_role(role_name)
    if role is None:
        raise ValueError(f"unsupported role: {role_name}")
    task = _require_string(raw.get("task"), "task")
    context = raw.get("context")
    if not isinstance(context, dict):
        raise ValueError("context must be an object")
    constraints = raw.get("constraints", [])
    if not isinstance(constraints, list):
        raise ValueError("constraints must be a list")
    max_tokens = raw.get("max_tokens", DEFAULT_MAX_TOKENS)
    if not isinstance(max_tokens, int) or not 256 <= max_tokens <= MAX_MAX_TOKENS:
        raise ValueError("max_tokens must be an integer between 256 and 16000")
    timeout_sec = raw.get("timeout_sec", DEFAULT_TIMEOUT_SEC)
    if not isinstance(timeout_sec, int) or not 10 <= timeout_sec <= MAX_TIMEOUT_SEC:
        raise ValueError("timeout_sec must be an integer between 10 and 600")
    session = _normalize_session(raw.get("session"))
    file_manifest = _normalize_file_manifest(raw.get("file_manifest"))
    if file_manifest["mode"] == "explicit" and not session["context_hint"]["workspace_root"]:
        raise ValueError("explicit file_manifest requires session.context_hint.workspace_root")
    return {
        "version": SCHEMA_VERSION,
        "role": role.name,
        "request_id": _optional_string(raw.get("request_id"), "request_id"),
        "session": session,
        "file_manifest": file_manifest,
        "task": task,
        "constraints": [
            _require_string(item, f"constraints[{index}]")
            for index, item in enumerate(constraints)
        ],
        "context": {
            "diff": context.get("diff", "") if isinstance(context.get("diff", ""), str) else "",
            "files": _normalize_context_items(
                context.get("files", []),
                ("path", "content"),
                "context.files",
            ),
            "docs": _normalize_context_items(
                context.get("docs", []),
                ("content",),
                "context.docs",
            ),
        },
        "max_tokens": max_tokens,
        "timeout_sec": timeout_sec,
    }


def build_prompt(request: dict) -> str:
    role = get_role(request["role"])
    if role is None:
        raise ValueError(f"unsupported role: {request['role']}")
    return role.build_prompt(request)


def validate_result(role_name: str, result: dict) -> dict:
    role = get_role(role_name)
    if role is None:
        raise ValueError(f"unsupported role: {role_name}")
    return role.validate_result(result)


def build_success_response(role: str, request_id, model: str, result: dict) -> dict:
    return {
        "version": SCHEMA_VERSION,
        "status": "ok",
        "role": role,
        "request_id": request_id,
        "model": model,
        "result": result,
        "errors": [],
        "token_usage": {"input": None, "output": None},
        "truncated": False,
        "files_read": [],
        "session": None,
    }


def build_error_response(code: str, message: str, role=None, request_id=None, model=None) -> dict:
    return {
        "version": SCHEMA_VERSION,
        "status": "error",
        "role": role,
        "request_id": request_id,
        "model": model,
        "result": None,
        "errors": [{"code": code, "message": message}],
        "token_usage": {"input": None, "output": None},
        "truncated": False,
        "files_read": [],
        "session": None,
    }


def extract_json_object(text: str) -> dict:
    raw = text.strip()
    if raw.startswith("```"):
        lines = raw.splitlines()
        raw = "\n".join(lines[1:-1]).strip() if len(lines) >= 3 else raw
    decoder = json.JSONDecoder()
    if raw.startswith("{"):
        value = decoder.decode(raw)
    else:
        value = None
        for index, char in enumerate(raw):
            if char != "{":
                continue
            try:
                candidate, _ = decoder.raw_decode(raw[index:])
            except json.JSONDecodeError:
                continue
            value = candidate
            break
        if value is None:
            raise ValueError("model response did not contain a JSON object")
    if not isinstance(value, dict):
        raise ValueError("model response JSON must be an object")
    return value
