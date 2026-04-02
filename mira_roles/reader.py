from mira_roles.base import BaseRole


class ReaderRole(BaseRole):
    name = "reader"
    summary = "Read supplied context and return clear notes, extraction, or synthesis."
    required_keys = ("summary", "key_points")
    result_example = {
        "summary": "string",
        "key_points": ["string"],
        "sections": [{"title": "string", "content": "string"}],
        "open_questions": ["string"],
    }
    allowed_capabilities = frozenset({"file_read"})
    default_content_format = "markdown"
    supported_content_formats = frozenset({"structured", "markdown", "text"})
