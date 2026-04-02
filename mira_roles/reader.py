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

    def context_guidance(self) -> str:
        return (
            "Use the prepared context first. If the task directly references readable "
            "resources such as URLs, wiki pages, or documents, read them when needed."
        )

    def output_style_guidance(self, content_format: str) -> str:
        if content_format == "structured":
            return "Preserve key terminology and keep the structure faithful to the source material."
        return (
            "Prefer faithful reading over over-compression. Preserve headings, key bullets, "
            "and important wording when they matter."
        )
