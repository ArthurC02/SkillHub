#!/usr/bin/env python3
import contextlib
import hashlib
import io
import json
import sys
import tempfile
from pathlib import Path
from unittest import mock

import validate_trace_events as trace


def run_main(**patches):
    out = io.StringIO()
    with contextlib.ExitStack() as stack:
        for name, value in patches.items():
            stack.enter_context(mock.patch.object(trace, name, value))
        stack.enter_context(contextlib.redirect_stdout(out))
        code = trace.main()
    return code, out.getvalue()


def broken_contract(folder):
    schema = json.loads(trace.SCHEMA_PATH.read_text(encoding="utf-8"))
    examples = schema["examples"]
    dropped = examples[-1]["type"]
    schema["examples"] = [e for e in examples if e["type"] != dropped]
    del schema["examples"][0]["run_id"]
    schema_path = Path(folder) / "trace-event.schema.json"
    schema_path.write_text(json.dumps(schema), encoding="utf-8")
    samples = Path(folder) / "samples"
    samples.mkdir()
    (samples / "a-empty.jsonl").write_text("\n  \n", encoding="utf-8")
    (samples / "b-mixed.jsonl").write_text(
        json.dumps(examples[1]) + "\n" + json.dumps({"schema_version": "1.0"}) + "\n", encoding="utf-8")
    (samples / "c-good.jsonl").write_text(json.dumps(examples[1]) + "\n", encoding="utf-8")
    negatives = [("a valid event", examples[1]), trace.NEGATIVE_CASES[0]]
    return dict(SCHEMA_PATH=schema_path, SAMPLES_DIR=samples, NEGATIVE_CASES=negatives)


def test_the_committed_contract_still_validates_with_the_same_report():
    code, out = run_main()
    assert code == 0
    assert out.splitlines()[-1] == "11 examples, 3 sample file(s), 4 counterexamples, 0 failure(s)", out
    assert hashlib.sha256(out.encode()).hexdigest() == EXPECTED_REAL_DIGEST, out


def test_every_kind_of_defect_is_reported_and_fails_the_run():
    with tempfile.TemporaryDirectory() as folder:
        code, out = run_main(**broken_contract(folder))
    assert code == 1
    assert out == EXPECTED_BROKEN_REPORT, out


def test_a_missing_samples_directory_is_not_itself_a_failure():
    with tempfile.TemporaryDirectory() as folder:
        code, out = run_main(SAMPLES_DIR=Path(folder) / "absent")
    assert code == 0
    assert out.splitlines()[-1] == "11 examples, 0 sample file(s), 4 counterexamples, 0 failure(s)", out


EXPECTED_REAL_DIGEST = "82fa1b39050610b943546459ed6b4be7ef18f2afeb67f746df0cb8c9637dd9b1"
MISSING = ("event_id", "run_id", "attempt", "seq", "occurred_at", "emitted_by", "type", "masked",
           "payload")
EXPECTED_BROKEN_REPORT = "".join([
    "FAIL  no example for event type(s): evaluation_completed\n",
    "FAIL  example run_lifecycle (0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01)\n",
    "        /: 'run_id' is a required property\n",
    *(f"ok    example {kind}\n" for kind in (
        "skill_activation", "resource_read", "tool_call", "script_log", "agent_output", "usage",
        "error", "mcp_call", "evaluation_started")),
    "FAIL  sample a-empty.jsonl is empty\n",
    *(f"FAIL  b-mixed.jsonl:2 /: '{key}' is a required property\n" for key in MISSING),
    "ok    sample c-good.jsonl (1 events)\n",
    "FAIL  counterexample accepted: a valid event\n",
    "ok    counterexample rejected: payload does not match the declared type\n",
    "\n10 examples, 3 sample file(s), 2 counterexamples, 5 failure(s)\n",
])


if __name__ == "__main__":
    failed = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"ok   {name}")
            except AssertionError as exc:
                failed += 1
                print(f"FAIL {name}: {exc}")
    sys.exit(1 if failed else 0)
