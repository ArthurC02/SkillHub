#!/usr/bin/env python3
"""TRACE-001 contract check: every example in trace-event.schema.json validates,
and a deliberately broken event does not.

Run: python tools/contracts/validate_trace_events.py
"""

from __future__ import annotations

import json
import pathlib
import sys

from jsonschema import Draft202012Validator

SCHEMA_PATH = pathlib.Path(__file__).resolve().parents[2] / "contracts" / "events" / "trace-event.schema.json"

# One counterexample per failure mode that actually matters. Each must be rejected.
NEGATIVE_CASES: list[tuple[str, dict]] = [
    (
        "payload does not match the declared type",
        {
            "schema_version": "1.0",
            "event_id": "00000000-0000-4000-8000-000000000001",
            "run_id": "00000000-0000-4000-8000-000000000002",
            "attempt": 1,
            "seq": 1,
            "occurred_at": "2026-08-16T09:12:03Z",
            "emitted_by": "sandbox",
            "type": "usage",
            "masked": True,
            # script_log payload under type=usage: required model/input_tokens missing.
            "payload": {"stream": "stdout", "message": "hi", "truncated": False},
        },
    ),
    (
        "error category outside the ADR-004 taxonomy",
        {
            "schema_version": "1.0",
            "event_id": "00000000-0000-4000-8000-000000000003",
            "run_id": "00000000-0000-4000-8000-000000000002",
            "attempt": 1,
            "seq": 2,
            "occurred_at": "2026-08-16T09:12:03Z",
            "emitted_by": "orchestrator",
            "type": "error",
            "masked": True,
            "payload": {
                "category": "network",
                "code": "x",
                "message": "y",
                "retryable": False,
            },
        },
    ),
    (
        "masking flag absent",
        {
            "schema_version": "1.0",
            "event_id": "00000000-0000-4000-8000-000000000004",
            "run_id": "00000000-0000-4000-8000-000000000002",
            "attempt": 1,
            "seq": 3,
            "occurred_at": "2026-08-16T09:12:03Z",
            "emitted_by": "sandbox",
            "type": "agent_output",
            "payload": {"kind": "final", "text": "done", "truncated": False},
        },
    ),
]


def main() -> int:
    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=Draft202012Validator.FORMAT_CHECKER)

    failures = 0
    examples = schema["examples"]
    covered = {e["type"] for e in examples}
    missing = set(schema["properties"]["type"]["enum"]) - covered
    if missing:
        print(f"FAIL  no example for event type(s): {', '.join(sorted(missing))}")
        failures += 1

    for example in examples:
        errors = sorted(validator.iter_errors(example), key=lambda e: e.path)
        if errors:
            failures += 1
            print(f"FAIL  example {example['type']} ({example['event_id']})")
            for err in errors:
                print(f"        /{'/'.join(map(str, err.path))}: {err.message}")
        else:
            print(f"ok    example {example['type']}")

    for label, case in NEGATIVE_CASES:
        if validator.is_valid(case):
            failures += 1
            print(f"FAIL  counterexample accepted: {label}")
        else:
            print(f"ok    counterexample rejected: {label}")

    print(
        f"\n{len(examples)} examples, {len(NEGATIVE_CASES)} counterexamples, "
        f"{failures} failure(s)"
    )
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
