from __future__ import annotations

import json
import pathlib

from jsonschema import Draft202012Validator


def check_sample(validator: Draft202012Validator, path: pathlib.Path, noun: str) -> int:
    lines = [ln for ln in path.read_text(encoding="utf-8").splitlines() if ln.strip()]
    if not lines:
        print(f"FAIL  sample {path.name} is empty")
        return 1
    bad = 0
    for number, line in enumerate(lines, start=1):
        instance = json.loads(line)
        for err in sorted(validator.iter_errors(instance), key=lambda e: e.path):
            bad += 1
            pointer = "/".join(map(str, err.path))
            print(f"FAIL  {path.name}:{number} /{pointer}: {err.message}")
    if bad:
        return 1
    print(f"ok    sample {path.name} ({len(lines)} {noun})")
    return 0


def check_counterexamples(validator: Draft202012Validator, cases: list[tuple[str, dict]]) -> int:
    failures = 0
    for label, case in cases:
        if validator.is_valid(case):
            failures += 1
            print(f"FAIL  counterexample accepted: {label}")
        else:
            print(f"ok    counterexample rejected: {label}")
    return failures
