#!/usr/bin/env python3
"""TRACE-001 contract check: every example in trace-event.schema.json validates,
a deliberately broken event does not, and every recorded sample of real pipeline
output still conforms.

The samples are the part the examples cannot catch: they are written by the
producer and by the storage layer themselves (contracts/events/samples/README.md
says how each is generated), so a change to the harness or to the masker that
stops matching the contract fails here rather than in a user's timeline.

Run: python tools/contracts/validate_trace_events.py
"""

from __future__ import annotations

import json
import pathlib
import sys

from jsonschema import Draft202012Validator

EVENTS_DIR = pathlib.Path(__file__).resolve().parents[2] / "contracts" / "events"
SCHEMA_PATH = EVENTS_DIR / "trace-event.schema.json"
SAMPLES_DIR = EVENTS_DIR / "samples"

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
    (
        "a sandbox claiming to have evaluated the run",
        {
            "schema_version": "1.2",
            "event_id": "00000000-0000-4000-8000-000000000005",
            "run_id": "00000000-0000-4000-8000-000000000002",
            "attempt": 1,
            "seq": 4,
            "occurred_at": "2026-08-16T09:12:03Z",
            # Evaluation runs in the control plane after the sandbox is gone, so
            # this is a forged verdict from untrusted input, not a late event.
            "emitted_by": "sandbox",
            "type": "evaluation_completed",
            "masked": True,
            "payload": {
                "evaluation_id": "00000000-0000-4000-8000-000000000006",
                "overall": "met",
                "criteria_total": 1,
                "criteria_passed": 1,
                "criteria_failed": 0,
                "criteria_undetermined": 0,
                "evidence_complete": True,
            },
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

    samples = sorted(SAMPLES_DIR.glob("*.jsonl")) if SAMPLES_DIR.is_dir() else []
    for path in samples:
        lines = [ln for ln in path.read_text(encoding="utf-8").splitlines() if ln.strip()]
        if not lines:
            print(f"FAIL  sample {path.name} is empty")
            failures += 1
            continue
        bad = 0
        for number, line in enumerate(lines, start=1):
            instance = json.loads(line)
            for err in sorted(validator.iter_errors(instance), key=lambda e: e.path):
                bad += 1
                pointer = "/".join(map(str, err.path))
                print(f"FAIL  {path.name}:{number} /{pointer}: {err.message}")
        if bad:
            failures += 1
        else:
            print(f"ok    sample {path.name} ({len(lines)} events)")

    for label, case in NEGATIVE_CASES:
        if validator.is_valid(case):
            failures += 1
            print(f"FAIL  counterexample accepted: {label}")
        else:
            print(f"ok    counterexample rejected: {label}")

    print(
        f"\n{len(examples)} examples, {len(samples)} sample file(s), "
        f"{len(NEGATIVE_CASES)} counterexamples, "
        f"{failures} failure(s)"
    )
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
