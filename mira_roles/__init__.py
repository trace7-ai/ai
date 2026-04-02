from mira_roles.planner import PlannerRole
from mira_roles.reviewer import ReviewerRole

ROLE_REGISTRY = {
    "planner": PlannerRole(),
    "reviewer": ReviewerRole(),
}


def get_role(name: str):
    return ROLE_REGISTRY.get(name)
