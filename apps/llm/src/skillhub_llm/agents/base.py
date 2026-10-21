from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class AgentInstructions:
    system: str
    result_schema: dict
    prompt_version: str


CITES = {"type": "array", "minItems": 1, "items": {"type": "string", "pattern": "^/"}}

REPORT_ITEMS = {
    "type": "array",
    "minItems": 1,
    "items": {
        "type": "object",
        "additionalProperties": False,
        "required": ["status", "text", "cites"],
        "properties": {
            "status": {"type": "string", "enum": ["fine", "attention"]},
            "text": {"type": "string", "minLength": 1},
            "cites": CITES,
        },
    },
}
