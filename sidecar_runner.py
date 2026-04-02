import json
import time
from pathlib import Path

from sidecar_contract import (
    build_error_response,
    build_prompt,
    build_success_response,
    extract_json_object,
    normalize_request,
    validate_result,
)

MAX_REQUEST_BYTES = 256 * 1024
MAX_RESPONSE_CHARS = 200000
EXIT_OK = 0
EXIT_EXECUTION_FAILED = 1
EXIT_INVALID_REQUEST = 2
EXIT_TIMEOUT = 3
SESSION_ERROR_HINTS = (
    "invalid session",
    "session expired",
    "session not found",
    "conversation not found",
    "invalid conversation",
    "会话失效",
    "会话不存在",
    "会话已过期",
)


def load_request_file(path: str) -> dict:
    request_path = Path(path)
    if request_path.stat().st_size > MAX_REQUEST_BYTES:
        raise ValueError(f"request file too large: {request_path}")
    raw = json.loads(request_path.read_text(encoding="utf-8"))
    return normalize_request(raw)


def collect_stream_text(stream, timeout_sec: int) -> str:
    chunks = []
    had_reason_text = False
    total_chars = 0
    deadline = time.monotonic() + timeout_sec
    try:
        for event in stream:
            if time.monotonic() > deadline:
                raise TimeoutError(f"sidecar request timed out after {timeout_sec}s")
            event_type = event.get("type", "")
            if event_type == "error":
                error = event.get("error", {})
                raise RuntimeError(error.get("message", "unknown stream error"))
            if event_type != "content_block_delta":
                continue
            delta = event.get("delta", {})
            if delta.get("type") != "text_delta":
                continue
            from_content = event.get("_from_content", False)
            if from_content and had_reason_text:
                continue
            if not from_content:
                had_reason_text = True
            text = delta.get("text", "")
            chunks.append(text)
            total_chars += len(text)
            if total_chars > MAX_RESPONSE_CHARS:
                raise ValueError("model response exceeded sidecar size limit")
    finally:
        stream.close()
    return "".join(chunks).strip()


def is_session_error_message(message: str) -> bool:
    lowered = message.lower()
    return any(hint in lowered for hint in SESSION_ERROR_HINTS)


def _execution_error_details(exc: Exception) -> tuple[str, int]:
    if is_session_error_message(str(exc)):
        return "invalid_session", EXIT_INVALID_REQUEST
    return "execution_failed", EXIT_EXECUTION_FAILED


def execute_request(client, request: dict) -> tuple[dict, int]:
    model_name = getattr(client.config, "model_name", None)
    try:
        prompt = build_prompt(request)
        text = collect_stream_text(client.stream_chat(prompt), request["timeout_sec"])
        result = validate_result(request["role"], extract_json_object(text))
        return build_success_response(
            request["role"],
            request.get("request_id"),
            model_name,
            result,
        ), EXIT_OK
    except TimeoutError as exc:
        response = build_error_response(
            "timeout",
            str(exc),
            role=request.get("role"),
            request_id=request.get("request_id"),
            model=model_name,
        )
        return response, EXIT_TIMEOUT
    except Exception as exc:
        error_code, exit_code = _execution_error_details(exc)
        response = build_error_response(
            error_code,
            str(exc),
            role=request.get("role"),
            request_id=request.get("request_id"),
            model=model_name,
        )
        return response, exit_code


def print_response(response: dict):
    print(json.dumps(response, ensure_ascii=False, indent=2))
