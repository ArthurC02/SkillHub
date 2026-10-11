from __future__ import annotations

from skillhub_llm.agents.base import CITES, AgentInstructions

DAILY_REPORT = AgentInstructions(
    system=(
        "You write the platform's daily maintenance report for its operators. "
        "Call `maintenance_report` once to read today's facts, then finish with a short report "
        "in Traditional Chinese, plain words, no jargon. "
        "Each item is either `fine` or `attention`: attention means a prediction crosses the "
        "restore budget within 90 days, a scheduled job has gone more than two periods without "
        "succeeding, or the restore rate is still an unmeasured default. "
        "Every item cites the facts it rests on as JSON Pointers into the tool's answer, such as "
        "`/capacity/days_until_budget` or `/maintenance_jobs/purge-audit/overdue_ratio`. "
        "Cite only facts the answer contains; a report citing anything else is rejected. "
        "When a job is attention because it has gone more than two periods without succeeding and "
        "its facts carry an `action`, you may add one proposal for that action: name it exactly, "
        "say why in one plain sentence, and cite the facts it rests on. An operator decides every "
        "proposal; propose nothing else, and nothing for a job without an `action`."
    ),
    result_schema={
        "type": "object",
        "additionalProperties": False,
        "required": ["items"],
        "properties": {
            "items": {
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
            },
            "proposals": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["action", "reason", "cites"],
                    "properties": {
                        "action": {"type": "string", "minLength": 1},
                        "reason": {"type": "string", "minLength": 1},
                        "cites": CITES,
                    },
                },
            },
        },
    },
    prompt_version="daily-report-v2",
)
