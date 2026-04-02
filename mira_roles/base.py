import json
from abc import ABC


class BaseRole(ABC):
    name = ""
    summary = ""
    required_keys = ()
    result_example = {}
    allowed_capabilities = frozenset()
    default_content_format = "structured"
    supported_content_formats = frozenset({"structured"})

    def resolve_content_format(self, requested: str) -> str:
        content_format = requested or self.default_content_format
        if content_format == "auto":
            return self.default_content_format
        if content_format not in self.supported_content_formats:
            raise ValueError(f"role does not support content_format={content_format}: {self.name}")
        return content_format

    def _output_instructions(self, content_format: str) -> str:
        if content_format == "structured":
            example = json.dumps(self.result_example, ensure_ascii=False, indent=2)
            return (
                "Return exactly one JSON object with no markdown fences.\n"
                "Return only the result object matching this shape:\n"
                f"{example}"
            )
        if content_format == "markdown":
            return (
                "Return a direct markdown answer.\n"
                "Use headings, bullets, blockquotes, and code fences when helpful.\n"
                "Do not return JSON.\n"
                "Do not wrap the entire answer in a single code fence."
            )
        if content_format == "text":
            return "Return direct plain text only. Do not return JSON."
        raise ValueError(f"unsupported content_format instructions: {content_format}")

    def build_prompt(self, request: dict) -> str:
        payload = json.dumps(request["context"], ensure_ascii=False, indent=2)
        constraints = request["constraints"] or []
        constraint_lines = "\n".join(f"- {item}" for item in constraints)
        session_id = request["session"]["session_id"] or "none"
        constraints_block = f"Constraints:\n{constraint_lines}\n\n" if constraint_lines else ""
        return (
            f"You are Mira acting as a {self.name} sidecar subagent.\n"
            "The caller already prepared the relevant context.\n"
            f"Goal: {self.summary}\n"
            f"Task:\n{request['task']}\n\n"
            f"{constraints_block}"
            f"{self._output_instructions(request['content_format'])}\n"
            f"Keep the answer within approximately {request['max_tokens']} tokens.\n"
            f"Assume the caller will terminate the request after {request['timeout_sec']} seconds.\n"
            f"Request ID: {request['request_id'] or 'none'}\n"
            f"Session ID: {session_id}\n"
            f"Content format: {request['content_format']}\n\n"
            "Prepared context:\n"
            f"{payload}"
        )

    def validate_result(self, result, content_format: str):
        if content_format != "structured":
            if not isinstance(result, str) or not result.strip():
                raise ValueError("model result must be a non-empty string")
            return result.strip()
        if not isinstance(result, dict):
            raise ValueError("model result must be a JSON object")
        for key in self.required_keys:
            if key not in result:
                raise ValueError(f"model result missing required key: {key}")
        return result
