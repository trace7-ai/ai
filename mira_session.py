import fcntl
import json
import os
import tempfile
import urllib.parse
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path

DEFAULT_TTL_SECONDS = 3600
LOCAL_TIMEZONE = datetime.now().astimezone().tzinfo or timezone.utc
MISSING_SESSION_STATUS = "missing"
ACTIVE_SESSION_STATUS = "active"
EXPIRED_SESSION_STATUS = "expired"
INVALID_SESSION_STATUS = "invalid"


@dataclass(frozen=True)
class SessionSnapshot:
    status: str
    record: dict | None = None
    reason: str | None = None


def _now_utc() -> datetime:
    return datetime.now(timezone.utc)


def _to_isoformat(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="seconds")


def _parse_time(raw: str, field: str) -> datetime:
    if not isinstance(raw, str) or not raw:
        raise ValueError(f"session {field} must be a non-empty string")
    try:
        parsed = datetime.fromisoformat(raw)
    except ValueError as exc:
        raise ValueError(f"session {field} is not valid ISO datetime: {raw}") from exc
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=LOCAL_TIMEZONE)
    return parsed


def _positive_int(value, field: str) -> int:
    if not isinstance(value, int) or value <= 0:
        raise ValueError(f"session {field} must be a positive integer")
    return value


class SessionStore:
    def __init__(self, root: Path | None = None):
        base_root = root or Path(os.environ.get("MIRA_HOME", Path.home() / ".mira"))
        self.root = base_root / "sessions"
        self.root.mkdir(parents=True, exist_ok=True)

    def _session_name(self, session_id: str) -> str:
        if not isinstance(session_id, str) or not session_id.strip():
            raise ValueError("session_id must be a non-empty string")
        return urllib.parse.quote(session_id.strip(), safe="._-")

    def _path(self, session_id: str) -> Path:
        return self.root / f"{self._session_name(session_id)}.json"

    def _lock_path(self, session_id: str) -> Path:
        return self.root / f"{self._session_name(session_id)}.lock"

    @contextmanager
    def lock(self, session_id: str):
        lock_path = self._lock_path(session_id)
        with lock_path.open("a+", encoding="utf-8") as handle:
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX)
            try:
                yield
            finally:
                fcntl.flock(handle.fileno(), fcntl.LOCK_UN)

    def load(self, session_id: str):
        path = self._path(session_id)
        if not path.exists():
            return None
        data = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(data, dict):
            raise ValueError(f"session record must be a JSON object: {path}")
        return data

    def save(self, session_id: str, data: dict):
        path = self._path(session_id)
        fd, temp_path = tempfile.mkstemp(
            dir=str(path.parent),
            prefix=f".{self._session_name(session_id)}.",
            suffix=".tmp",
        )
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as handle:
                json.dump(data, handle, ensure_ascii=False, indent=2)
            os.replace(temp_path, path)
            path.chmod(0o600)
        except Exception:
            if os.path.exists(temp_path):
                os.unlink(temp_path)
            raise

    def destroy(self, session_id: str):
        self._path(session_id).unlink(missing_ok=True)
        self._lock_path(session_id).unlink(missing_ok=True)

    def inspect(self, session_id: str) -> SessionSnapshot:
        record = self.load(session_id)
        if record is None:
            return SessionSnapshot(MISSING_SESSION_STATUS)
        status = record.get("status")
        if status == ACTIVE_SESSION_STATUS:
            return self._inspect_active(session_id, record)
        if status in {EXPIRED_SESSION_STATUS, INVALID_SESSION_STATUS}:
            return SessionSnapshot(status, record, record.get("last_error"))
        return self._mark_invalid(session_id, record, f"unsupported session status: {status}")

    def _inspect_active(self, session_id: str, record: dict) -> SessionSnapshot:
        remote_session_id = record.get("remote_session_id")
        turn_count = record.get("turn_count")
        if not isinstance(remote_session_id, str) or not remote_session_id:
            return self._mark_invalid(session_id, record, "active session is missing remote_session_id")
        if not isinstance(turn_count, int) or turn_count < 0:
            return self._mark_invalid(session_id, record, "active session has invalid turn_count")
        expires_at = self._expires_at(record)
        if expires_at < _now_utc():
            expired = self._with_status(record, EXPIRED_SESSION_STATUS, "session ttl elapsed")
            self.save(session_id, expired)
            return SessionSnapshot(EXPIRED_SESSION_STATUS, expired, expired["last_error"])
        active = dict(record)
        active["expires_at"] = _to_isoformat(expires_at)
        return SessionSnapshot(ACTIVE_SESSION_STATUS, active)

    def _expires_at(self, record: dict) -> datetime:
        if record.get("expires_at"):
            return _parse_time(record["expires_at"], "expires_at")
        ttl_seconds = _positive_int(record.get("ttl_seconds", DEFAULT_TTL_SECONDS), "ttl_seconds")
        last_active_at = _parse_time(record.get("last_active_at"), "last_active_at")
        return last_active_at + timedelta(seconds=ttl_seconds)

    def _with_status(self, record: dict, status: str, reason: str) -> dict:
        updated_at = _to_isoformat(_now_utc())
        updated = dict(record)
        updated["status"] = status
        updated["last_error"] = reason
        updated["updated_at"] = updated_at
        if status == EXPIRED_SESSION_STATUS:
            updated["expired_at"] = updated_at
        if status == INVALID_SESSION_STATUS:
            updated["invalidated_at"] = updated_at
        return updated

    def _mark_invalid(self, session_id: str, record: dict, reason: str) -> SessionSnapshot:
        invalid = self._with_status(record, INVALID_SESSION_STATUS, reason)
        self.save(session_id, invalid)
        return SessionSnapshot(INVALID_SESSION_STATUS, invalid, reason)

    def build_record(self, request: dict, remote_session_id: str, *, existing=None) -> dict:
        if not isinstance(remote_session_id, str) or not remote_session_id:
            raise ValueError("remote_session_id must be a non-empty string")
        now = _now_utc()
        ttl_seconds = _positive_int(
            (existing or {}).get("ttl_seconds", DEFAULT_TTL_SECONDS),
            "ttl_seconds",
        )
        hint = request["session"]["context_hint"]
        turn_count = (existing or {}).get("turn_count", 0) + 1
        return {
            "session_id": request["session"]["session_id"],
            "parent_session_id": request["session"]["parent_session_id"],
            "created_at": (existing or {}).get("created_at", _to_isoformat(now)),
            "last_active_at": _to_isoformat(now),
            "expires_at": _to_isoformat(now + timedelta(seconds=ttl_seconds)),
            "remote_session_id": remote_session_id,
            "turn_count": turn_count,
            "workspace_root": hint["workspace_root"],
            "task_description": hint["task_description"],
            "git_branch": hint["git_branch"],
            "ttl_seconds": ttl_seconds,
            "status": ACTIVE_SESSION_STATUS,
            "last_error": None,
        }

    def mark_invalid(self, session_id: str, record: dict, reason: str) -> dict:
        invalid = self._with_status(record, INVALID_SESSION_STATUS, reason)
        self.save(session_id, invalid)
        return invalid
