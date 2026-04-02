import json
import os
import tempfile
from datetime import datetime, timedelta
from pathlib import Path

DEFAULT_TTL_SECONDS = 3600


class SessionStore:
    def __init__(self, root: Path | None = None):
        base_root = root or Path(os.environ.get("MIRA_HOME", Path.home() / ".mira"))
        self.root = base_root / "sessions"
        self.root.mkdir(parents=True, exist_ok=True)

    def _path(self, session_id: str) -> Path:
        return self.root / f"{session_id}.json"

    def load(self, session_id: str):
        path = self._path(session_id)
        if not path.exists():
            return None
        return json.loads(path.read_text())

    def save(self, session_id: str, data: dict):
        path = self._path(session_id)
        fd, temp_path = tempfile.mkstemp(
            dir=str(path.parent),
            prefix=f".{session_id}.",
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

    def is_valid(self, data: dict | None) -> bool:
        if not data or data.get("status") != "active":
            return False
        if not data.get("remote_session_id"):
            return False
        last_active_at = data.get("last_active_at")
        if not last_active_at:
            return False
        ttl_seconds = int(data.get("ttl_seconds", DEFAULT_TTL_SECONDS))
        expires_at = datetime.fromisoformat(last_active_at) + timedelta(seconds=ttl_seconds)
        return expires_at >= datetime.now()

    def build_record(
        self,
        request: dict,
        remote_session_id: str,
        *,
        existing=None,
        reset_turn_count=False,
    ) -> dict:
        now = datetime.now().isoformat()
        hint = request["session"]["context_hint"]
        turn_count = 1 if reset_turn_count else (existing or {}).get("turn_count", 0) + 1
        return {
            "session_id": request["session"]["session_id"],
            "parent_session_id": request["session"]["parent_session_id"],
            "created_at": (existing or {}).get("created_at", now),
            "last_active_at": now,
            "remote_session_id": remote_session_id,
            "turn_count": turn_count,
            "workspace_root": hint["workspace_root"],
            "task_description": hint["task_description"],
            "git_branch": hint["git_branch"],
            "ttl_seconds": (existing or {}).get("ttl_seconds", DEFAULT_TTL_SECONDS),
            "status": "active",
        }
