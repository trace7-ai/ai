import json
import tempfile
import unittest

from mira_sidecar_cli import load_request_from_cli
from sidecar_contract import normalize_request, parse_model_result
from sidecar_runner import EXIT_OK, execute_request


def _base_request(role: str, content_format=None) -> dict:
    request = {
        "version": "v1",
        "role": role,
        "request_id": "req-1",
        "session": {
            "mode": "ephemeral",
            "session_id": None,
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
        "task": "summarize the supplied material",
        "constraints": [],
        "context": {"diff": "", "files": [], "docs": []},
        "max_tokens": 512,
        "timeout_sec": 30,
    }
    if content_format is not None:
        request["content_format"] = content_format
    return request


class _FakeConfig:
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
    def __init__(self, events):
        self.config = _FakeConfig()
        self.events = events

    def stream_chat(self, prompt: str):
        return _FakeStream(self.events)


class SidecarContentFormatTests(unittest.TestCase):
    def test_planner_defaults_to_structured(self):
        request = normalize_request(_base_request("planner"))
        self.assertEqual(request["content_format"], "structured")

    def test_reader_defaults_to_markdown(self):
        request = normalize_request(_base_request("reader"))
        self.assertEqual(request["content_format"], "markdown")

    def test_reader_allows_text_override(self):
        request = normalize_request(_base_request("reader", content_format="text"))
        self.assertEqual(request["content_format"], "text")

    def test_reader_allows_structured_override(self):
        request = normalize_request(_base_request("reader", content_format="structured"))
        self.assertEqual(request["content_format"], "structured")

    def test_planner_rejects_markdown_override(self):
        with self.assertRaisesRegex(ValueError, "role does not support content_format=markdown"):
            normalize_request(_base_request("planner", content_format="markdown"))

    def test_parse_structured_result_keeps_dict(self):
        payload = '{"summary":"ok","plan":[],"risks":[],"open_questions":[],"validation":[]}'
        result = parse_model_result("planner", payload, "structured")
        self.assertIsInstance(result, dict)
        self.assertEqual(result["summary"], "ok")

    def test_parse_markdown_result_keeps_string(self):
        payload = "# Summary\n\n- item"
        result = parse_model_result("reader", payload, "markdown")
        self.assertEqual(result, payload)

    def test_execute_request_supports_reader_markdown(self):
        request = normalize_request(_base_request("reader"))
        response, exit_code = execute_request(
            _FakeClient(
                [
                    {
                        "type": "content_block_delta",
                        "delta": {"type": "text_delta", "text": "# Summary\n\n- item"},
                    }
                ]
            ),
            request,
        )
        self.assertEqual(exit_code, EXIT_OK)
        self.assertEqual(response["content_type"], "markdown")
        self.assertEqual(response["result"], "# Summary\n\n- item")

    def test_execute_request_supports_reader_structured(self):
        request = normalize_request(_base_request("reader", content_format="structured"))
        response, exit_code = execute_request(
            _FakeClient(
                [
                    {
                        "type": "content_block_delta",
                        "delta": {
                            "type": "text_delta",
                            "text": json.dumps(
                                {
                                    "summary": "doc summary",
                                    "key_points": ["a", "b"],
                                    "sections": [],
                                    "open_questions": [],
                                }
                            ),
                        },
                    }
                ]
            ),
            request,
        )
        self.assertEqual(exit_code, EXIT_OK)
        self.assertEqual(response["content_type"], "structured")
        self.assertEqual(response["result"]["summary"], "doc summary")

    def test_cli_reader_defaults_to_markdown(self):
        request = load_request_from_cli(["--role", "reader", "--task", "read this doc"])
        self.assertEqual(request["content_format"], "markdown")

    def test_cli_infers_reader_from_url(self):
        request = load_request_from_cli(
            ["Read https://example.com/article and return the content"],
        )
        self.assertEqual(request["role"], "reader")
        self.assertEqual(request["content_format"], "markdown")

    def test_cli_infers_planner_from_plan_keyword(self):
        request = load_request_from_cli(["Plan the implementation steps for this change"])
        self.assertEqual(request["role"], "planner")
        self.assertEqual(request["content_format"], "structured")

    def test_cli_infers_reviewer_from_diff_context(self):
        with tempfile.NamedTemporaryFile("w", suffix=".json") as handle:
            json.dump({"context": {"diff": "diff --git a/app.py b/app.py", "files": [], "docs": []}}, handle)
            handle.flush()
            request = load_request_from_cli(
                ["--context-file", handle.name, "--task", "please check this patch"],
            )
        self.assertEqual(request["role"], "reviewer")
        self.assertEqual(request["content_format"], "structured")

    def test_cli_explicit_role_overrides_router(self):
        request = load_request_from_cli(
            ["--role", "reader", "--task", "Plan the implementation steps for this change"],
        )
        self.assertEqual(request["role"], "reader")
        self.assertEqual(request["content_format"], "markdown")

    def test_cli_accepts_explicit_content_format(self):
        request = load_request_from_cli(
            ["--role", "reader", "--content-format", "text", "--task", "read this doc"],
        )
        self.assertEqual(request["content_format"], "text")

    def test_cli_input_file_role_override_normalizes_once(self):
        with tempfile.NamedTemporaryFile("w", suffix=".json") as handle:
            json.dump(_base_request("reader"), handle)
            handle.flush()
            request = load_request_from_cli(
                ["--input-file", handle.name, "--role", "planner"],
            )
        self.assertEqual(request["role"], "planner")
        self.assertEqual(request["content_format"], "structured")


if __name__ == "__main__":
    unittest.main()
