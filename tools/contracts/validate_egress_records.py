#!/usr/bin/env python3
"""Validate egress-record.schema.json: every schema example validates, every
record kind and decision has one, the recorded node samples still conform, and
records that would let a reader misattribute traffic are rejected."""

from __future__ import annotations

import json
import pathlib
import sys

from jsonschema import Draft202012Validator

EVENTS_DIR = pathlib.Path(__file__).resolve().parents[2] / "contracts" / "events"
SCHEMA_PATH = EVENTS_DIR / "egress-record.schema.json"
SAMPLES_DIR = EVENTS_DIR / "samples" / "egress"

FLOW = {
    "schema_version": "1.0",
    "record": "egress_flow",
    "at": "2026-09-18T09:00:12.481204Z",
    "decision": "accepted",
    "protocol": "tcp",
    "source": "172.17.0.4",
    "destination": "10.0.0.3",
    "destination_port": 4000,
    "packets_out": 18,
    "bytes_out": 2411,
    "packets_in": 16,
    "bytes_in": 18942,
}

ADDRESS = {
    "schema_version": "1.0",
    "record": "run_address",
    "at": "2026-09-18T09:00:01.004Z",
    "state": "assigned",
    "run_id": "9f1c8f2e-6a4b-4c1d-9e3a-0b7d5c2f1a88",
    "attempt": 1,
    "address": "172.17.0.4",
}

NEGATIVE_CASES: list[tuple[str, dict]] = [
    ("an accepted flow naming a rule that blocked it", FLOW | {"blocked_by": "dns"}),
    ("a blocked flow that does not name the rule", FLOW | {"decision": "blocked"}),
    ("a flow whose decision is not one the ruleset can make", FLOW | {"decision": "logged"}),
    ("a counter below zero", FLOW | {"bytes_in": -1}),
    ("an address record that does not say whether the window opens or closes",
     {k: v for k, v in ADDRESS.items() if k != "state"}),
    ("an address record without the Run it belongs to",
     {k: v for k, v in ADDRESS.items() if k != "run_id"}),
    ("a record kind nobody agreed to", ADDRESS | {"record": "dns_query"}),
    ("a record without the time that ties it to a Run",
     {k: v for k, v in FLOW.items() if k != "at"}),
]


def main() -> int:
    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=Draft202012Validator.FORMAT_CHECKER)

    failures = 0
    examples = schema["examples"]
    kinds = {e["record"] for e in examples}
    missing = set(schema["properties"]["record"]["enum"]) - kinds
    decisions = {e.get("decision") for e in examples}
    missing |= set(schema["$defs"]["egress_flow"]["properties"]["decision"]["enum"]) - decisions
    if missing:
        print(f"FAIL  no example for: {', '.join(sorted(missing))}")
        failures += 1

    for index, example in enumerate(examples):
        errors = sorted(validator.iter_errors(example), key=lambda e: e.path)
        if errors:
            failures += 1
            print(f"FAIL  example {index} ({example['record']})")
            for err in errors:
                print(f"        /{'/'.join(map(str, err.path))}: {err.message}")
        else:
            print(f"ok    example {index} ({example['record']})")

    samples = sorted(SAMPLES_DIR.glob("*.jsonl")) if SAMPLES_DIR.is_dir() else []
    if not samples:
        print(f"FAIL  no recorded sample under {SAMPLES_DIR}")
        failures += 1
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
            print(f"ok    sample {path.name} ({len(lines)} records)")

    for label, case in NEGATIVE_CASES:
        if validator.is_valid(case):
            failures += 1
            print(f"FAIL  counterexample accepted: {label}")
        else:
            print(f"ok    counterexample rejected: {label}")

    print(
        f"\n{len(examples)} examples, {len(samples)} sample file(s), "
        f"{len(NEGATIVE_CASES)} counterexamples, {failures} failure(s)"
    )
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
