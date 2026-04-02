import json
import tempfile
import unittest
from pathlib import Path

from mira_session import ACTIVE_SESSION_STATUS, SessionStore
from mira_sidecar import SidecarService
from sidecar_runner import EXIT_INVALID_REQUEST, EXIT_OK


class _FakeConfig:
    has_auth = True
    model_name = "test-model"


class _FakeStream:
    def __init__(self, events):
        self._events = iter(events)

    def __iter__(self):
        return self

    def __next__(self):
        return next(self._events)

    def close(self):
        return None


class _FakeClient:
    def __init__(self, config, events, created_session_id):
        self.config = config
        self.events = list(events)
        self.created_session_id = created_session_id
        self.mira_session_id = ""
        self.prompts = []

    def stream_chat(self, prompt: str):
        self.prompts.append(prompt)
        if not self.mira_session_id and self.created_session_id:
            self.mira_session_id = self.created_session_id
        return _FakeStream(self.events)


class _ClientFactory:
    def __init__(self, events, created_session_id="remote-session-1"):
        self.events = events
        self.created_session_id = created_session_id
        self.instances = []

    def __call__(self, config):
        client = _FakeClient(config, self.events, self.created_session_id)
        self.instances.append(client)
        return client


def _request(session_id: str) -> dict:
    return {
        "version": "v1",
        "role": "planner",
        "request_id": "req-1",
        "content_format": "structured",
        "session": {
            "mode": "sticky",
            "session_id": session_id,
            "parent_session_id": None,
            "context_hint": {
                "workspace_root": "/tmp",
                "task_description": "test task",
                "git_branch": "main",
            },
        },
        "file_manifest": {
            "mode": "none",
            "paths": [],
            "max_total_bytes": 512 * 1024,
            "read_only": True,
        },
        "task": "review the plan",
        "constraints": [],
        "context": {"diff": "", "files": [], "docs": []},
        "max_tokens": 512,
        "timeout_sec": 30,
    }


def _planner_events() -> list[dict]:
    payload = {
        "summary": "ok",
        "plan": [],
        "risks": [],
        "open_questions": [],
        "validation": [],
    }
    return [
        {
            "type": "content_block_delta",
            "delta": {"type": "text_delta", "text": json.dumps(payload)},
        }
    ]


class SidecarSessionTests(unittest.TestCase):
    def test_store_marks_expired_session(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            store = SessionStore(Path(temp_dir))
            request = _request("sticky-expired")
            record = store.build_record(request, "remote-session-1")
            record["expires_at"] = "2000-01-01T00:00:00+00:00"
            store.save("sticky-expired", record)

            snapshot = store.inspect("sticky-expired")

            self.assertEqual(snapshot.status, "expired")
            self.assertEqual(store.load("sticky-expired")["status"], "expired")

    def test_service_creates_new_sticky_session(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            store = SessionStore(Path(temp_dir))
            factory = _ClientFactory(_planner_events())
            service = SidecarService(_FakeConfig, factory, session_store=store)

            response, exit_code = service.run(_request("sticky-new"))

            self.assertEqual(exit_code, EXIT_OK)
            self.assertEqual(response["status"], "ok")
            self.assertEqual(response["session"]["status"], ACTIVE_SESSION_STATUS)
            self.assertEqual(store.inspect("sticky-new").status, ACTIVE_SESSION_STATUS)

    def test_service_rejects_expired_session_before_execution(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            store = SessionStore(Path(temp_dir))
            request = _request("sticky-expired")
            record = store.build_record(request, "remote-session-1")
            record["expires_at"] = "2000-01-01T00:00:00+00:00"
            store.save("sticky-expired", record)
            factory = _ClientFactory(_planner_events())
            service = SidecarService(_FakeConfig, factory, session_store=store)

            response, exit_code = service.run(request)

            self.assertEqual(exit_code, EXIT_INVALID_REQUEST)
            self.assertEqual(response["errors"][0]["code"], "session_expired")
            self.assertEqual(len(factory.instances), 0)

    def test_service_marks_remote_invalid_session_without_retry(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            store = SessionStore(Path(temp_dir))
            request = _request("sticky-active")
            store.save("sticky-active", store.build_record(request, "remote-session-1"))
            factory = _ClientFactory(
                [{"type": "error", "error": {"message": "conversation not found"}}],
                created_session_id=None,
            )
            service = SidecarService(_FakeConfig, factory, session_store=store)

            response, exit_code = service.run(request)

            self.assertEqual(exit_code, EXIT_INVALID_REQUEST)
            self.assertEqual(response["errors"][0]["code"], "invalid_session")
            self.assertEqual(response["session"]["status"], "invalid")
            self.assertEqual(store.inspect("sticky-active").status, "invalid")
            self.assertEqual(len(factory.instances), 1)
            self.assertEqual(len(factory.instances[0].prompts), 1)


if __name__ == "__main__":
    unittest.main()
