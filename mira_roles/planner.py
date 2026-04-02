from mira_roles.base import BaseRole


class PlannerRole(BaseRole):
    name = "planner"
    summary = "Produce an execution plan with risks and validation steps."
    required_keys = ("summary", "plan", "risks", "open_questions", "validation")
    allowed_capabilities = frozenset({"file_read"})
    result_example = {
        "summary": "string",
        "plan": [{"step": "string", "why": "string"}],
        "risks": [{"severity": "high", "title": "string", "why": "string"}],
        "open_questions": ["string"],
        "validation": ["string"],
    }
