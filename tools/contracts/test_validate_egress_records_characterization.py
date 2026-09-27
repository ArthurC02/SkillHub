#!/usr/bin/env python3
import contextlib
import hashlib
import io
import json
import sys
import tempfile
from pathlib import Path
from unittest import mock

import validate_egress_records as egress


def run_main(**patches):
    out = io.StringIO()
    with contextlib.ExitStack() as stack:
        for name, value in patches.items():
            stack.enter_context(mock.patch.object(egress, name, value))
        stack.enter_context(contextlib.redirect_stdout(out))
        code = egress.main()
    return code, out.getvalue()


def broken_contract(folder):
    schema = json.loads(egress.SCHEMA_PATH.read_text(encoding="utf-8"))
    schema["examples"] = [e for e in schema["examples"] if e.get("decision") != "blocked"]
    schema["examples"][0]["bytes_in"] = -5
    schema_path = Path(folder) / "egress-record.schema.json"
    schema_path.write_text(json.dumps(schema), encoding="utf-8")
    samples = Path(folder) / "samples"
    samples.mkdir()
    (samples / "a-empty.jsonl").write_text("\n", encoding="utf-8")
    (samples / "b-mixed.jsonl").write_text(
        json.dumps(egress.FLOW) + "\n" + json.dumps(egress.FLOW | {"destination_port": "x"}) + "\n",
        encoding="utf-8")
    (samples / "c-good.jsonl").write_text(
        json.dumps(egress.FLOW) + "\n" + json.dumps(egress.ADDRESS) + "\n", encoding="utf-8")
    negatives = [("a valid flow", egress.FLOW), egress.NEGATIVE_CASES[0]]
    return dict(SCHEMA_PATH=schema_path, SAMPLES_DIR=samples, NEGATIVE_CASES=negatives)


def test_the_committed_contract_still_validates_with_the_same_report():
    code, out = run_main()
    assert code == 0
    assert out.splitlines()[-1] == "5 examples, 1 sample file(s), 8 counterexamples, 0 failure(s)", out
    assert hashlib.sha256(out.encode()).hexdigest() == EXPECTED_REAL_DIGEST, out


def test_every_kind_of_defect_is_reported_and_fails_the_run():
    with tempfile.TemporaryDirectory() as folder:
        code, out = run_main(**broken_contract(folder))
    assert code == 1
    assert out == EXPECTED_BROKEN_REPORT, out


def test_no_recorded_sample_at_all_is_a_failure():
    with tempfile.TemporaryDirectory() as folder:
        absent = Path(folder) / "absent"
        code, out = run_main(SAMPLES_DIR=absent)
    assert code == 1
    assert f"FAIL  no recorded sample under {absent}" in out.splitlines(), out
    assert out.splitlines()[-1] == "5 examples, 0 sample file(s), 8 counterexamples, 1 failure(s)", out


EXPECTED_REAL_DIGEST = "b581e893f8d3515dcdeae0ed5f85c568ce46ce11ebc21530b0ddac171c418b4c"
NOT_ONE_OF = " is not valid under any of the given schemas\n"
EXPECTED_BROKEN_REPORT = "".join([
    "FAIL  no example for: blocked\n",
    "FAIL  example 0 (egress_flow)\n",
    "        /: " + repr(egress.FLOW | {"at": "2026-09-18T09:00:12.481204Z", "bytes_in": -5}) + NOT_ONE_OF,
    "ok    example 1 (run_address)\n",
    "ok    example 2 (run_address)\n",
    "FAIL  sample a-empty.jsonl is empty\n",
    "FAIL  b-mixed.jsonl:2 /: " + repr(egress.FLOW | {"destination_port": "x"}) + NOT_ONE_OF,
    "ok    sample c-good.jsonl (2 records)\n",
    "FAIL  counterexample accepted: a valid flow\n",
    "ok    counterexample rejected: an accepted flow naming a rule that blocked it\n",
    "\n3 examples, 3 sample file(s), 2 counterexamples, 5 failure(s)\n",
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
