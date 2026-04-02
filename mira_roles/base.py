import json
from abc import ABC


class BaseRole(ABC):
    name = ""
    summary = ""
    required_keys = ()
    result_example = {}
    allowed_capabilities = frozenset()

    def build_prompt(self, request: dict) -> str:
        payload = json.dumps(request["context"], ensure_ascii=False, indent=2)
        example = json.dumps(self.result_example, ensure_ascii=False, indent=2)
        constraints = request["constraints"] or ["Return only machine-readable JSON."]
        constraint_lines = "\n".join(f"- {item}" for item in constraints)
        session_id = request["session"]["session_id"] or "none"
        return (
            f"You are Mira acting as a {self.name} sidecar subagent.\n"
            "The caller already prepared the relevant context.\n"
            f"Goal: {self.summary}\n"
            f"Task:\n{request['task']}\n\n"
            f"Constraints:\n{constraint_lines}\n\n"
            "Return exactly one JSON object with no markdown fences.\n"
            f"Keep the answer within approximately {request['max_tokens']} tokens.\n"
            f"Assume the caller will terminate the request after {request['timeout_sec']} seconds.\n"
            f"Request ID: {request['request_id'] or 'none'}\n"
            f"Session ID: {session_id}\n"
            "Return only the result object matching this shape:\n"
            f"{example}\n\n"
            "Prepared context:\n"
            f"{payload}"
        )

    def validate_result(self, result: dict) -> dict:
        if not isinstance(result, dict):
            raise ValueError("model result must be a JSON object")
        for key in self.required_keys:
            if key not in result:
                raise ValueError(f"model result missing required key: {key}")
        return result
