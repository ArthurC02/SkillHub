"""Every platform agent's instructions, one module per agent, looked up by its registered name."""

from __future__ import annotations

from skillhub_llm.agents.base import AgentInstructions
from skillhub_llm.agents.daily_report import DAILY_REPORT

INSTRUCTIONS: dict[str, AgentInstructions] = {"daily-report": DAILY_REPORT}

__all__ = ["INSTRUCTIONS", "AgentInstructions"]
