from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class AgentInstructions:
    system: str
    result_schema: dict
    prompt_version: str


CITES = {"type": "array", "minItems": 1, "items": {"type": "string", "pattern": "^/"}}
