"""Every platform agent's instructions, one module per agent, looked up by its registered name."""

from __future__ import annotations

from skillhub_llm.agents.base import AgentInstructions
from skillhub_llm.agents.daily_report import DAILY_REPORT
from skillhub_llm.agents.exposure_review import EXPOSURE_REVIEW

INSTRUCTIONS: dict[str, AgentInstructions] = {
    "daily-report": DAILY_REPORT,
    "exposure-review": EXPOSURE_REVIEW,
}

__all__ = ["INSTRUCTIONS", "AgentInstructions"]
