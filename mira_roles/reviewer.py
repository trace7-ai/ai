from mira_roles.base import BaseRole


class ReviewerRole(BaseRole):
    name = "reviewer"
    summary = "Review supplied changes for bugs, regressions, and missing checks."
    required_keys = ("summary", "verdict", "findings", "open_questions")
    allowed_capabilities = frozenset({"file_read"})
    result_example = {
        "summary": "string",
        "verdict": "pass|needs_changes",
        "findings": [
            {
                "severity": "critical|high|medium|low",
                "title": "string",
                "why": "string",
                "evidence": "string",
            }
        ],
        "open_questions": ["string"],
    }
