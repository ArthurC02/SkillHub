"""Compare two creation-measure runs on the frozen test set, in aggregates only.

Usage: python tools/eval-regression/creation_frozen_compare.py \
    --corpus FROZEN.json --baseline RUN_DIR [--candidate RUN_DIR]

Prints pass rates with Wilson intervals (judge and code checks), per category, and for two
runs the task-clustered paired bootstrap of the difference and the discordant-pair counts.
It never prints a case's output or the judge's reasons: the frozen set's failures stay unread.
"""

from __future__ import annotations

import argparse
import json
import math
import pathlib
import random
import re

CATEGORIES = {"FM": "money", "FT": "time", "FX": "text", "FJ": "judge"}
ISO_DATE = re.compile(r"^(\d{4})-(\d{2})-(\d{2})$")
CLOCK = re.compile(r"^(\d{1,3}):(\d{2})$")
DIGITS = re.compile(r"^-?\d+(?:\.\d+)?$")


def wilson(passed: int, total: int, z: float = 1.96) -> tuple[float, float]:
    if total == 0:
        return (0.0, 0.0)
    p = passed / total
    centre = (p + z * z / (2 * total)) / (1 + z * z / total)
    half = z * math.sqrt(p * (1 - p) / total + z * z / (4 * total * total)) / (1 + z * z / total)
    return (max(0.0, centre - half), min(1.0, centre + half))


def grouped(number: str) -> str:
    whole, _, fraction = number.lstrip("-").partition(".")
    text = f"{int(whole):,}" + (f".{fraction}" if fraction else "")
    return ("-" if number.startswith("-") else "") + text


def written_forms(token: str) -> set[str]:
    if match := ISO_DATE.match(token):
        year, month, day = match.group(1), int(match.group(2)), int(match.group(3))
        return {
            token,
            f"{year}/{month:02d}/{day:02d}",
            f"{year}/{month}/{day}",
            f"{year}年{month}月{day}日",
            f"{month}月{day}日",
            f"{month}/{day}",
            f"{month:02d}/{day:02d}",
        }
    if match := CLOCK.match(token):
        hours, minutes = int(match.group(1)), int(match.group(2))
        forms = {token, f"{hours}小時{minutes}分"}
        if minutes == 0:
            forms.add(f"{hours}小時")
        return forms
    if DIGITS.match(token):
        return {token, grouped(token)}
    return {token}


def mentions(output: str, token: str) -> bool:
    text = re.sub(r"\s+", "", output)
    for form in written_forms(token):
        pattern = re.escape(form)
        if form[0].isdigit() or form[0] == "-":
            pattern = r"(?<![\d.,])" + pattern
        if form[-1].isdigit():
            pattern += r"(?![\d]|[.,]\d)"
        if re.search(pattern, text):
            return True
    return False


def frozen_cases(corpus: dict) -> list[dict]:
    cases = []
    for task in corpus["reference"]:
        for index, holdout in enumerate(task["holdout"]):
            cases.append(
                {
                    "task": task["id"],
                    "index": index,
                    "category": CATEGORIES.get(task["id"][:2], "other"),
                    "expected": holdout.get("expected_numbers") or [],
                }
            )
    return cases


def case_outcome(run_dir: pathlib.Path, case: dict) -> dict:
    record_path = run_dir / f"{case['task']}-holdout-{case['index'] + 1}-trial-r0.json"
    if not record_path.exists():
        return {"ran": False, "judge": False, "code": None}
    record = json.loads(record_path.read_text(encoding="utf-8"))
    output = record.get("final_output") or ""
    judged = (record.get("evaluation") or {}).get("overall") == "met"
    code = all(mentions(output, t) for t in case["expected"]) if case["expected"] else None
    return {"ran": True, "judge": judged, "code": code}


def rate(outcomes: list[dict], key: str) -> dict:
    graded = [o[key] for o in outcomes if o[key] is not None]
    passed = sum(graded)
    low, high = wilson(passed, len(graded))
    return {"passed": passed, "total": len(graded), "low": round(low, 3), "high": round(high, 3)}


def summary(cases: list[dict], outcomes: list[dict]) -> dict:
    result = {
        "ran": sum(o["ran"] for o in outcomes),
        "judge": rate(outcomes, "judge"),
        "code": rate(outcomes, "code"),
        "by_category": {},
    }
    for category in sorted({c["category"] for c in cases}):
        picked = [o for c, o in zip(cases, outcomes, strict=True) if c["category"] == category]
        result["by_category"][category] = rate(picked, "judge")
    return result


def paired_bootstrap(
    cases: list[dict], base: list[bool], cand: list[bool], reps: int = 10_000, seed: int = 0
) -> dict:
    by_task: dict[str, list[int]] = {}
    for i, case in enumerate(cases):
        by_task.setdefault(case["task"], []).append(i)
    tasks = sorted(by_task)
    diff = (sum(cand) - sum(base)) / len(cases)
    rng = random.Random(seed)
    draws = []
    for _ in range(reps):
        picked = [i for _ in tasks for i in by_task[rng.choice(tasks)]]
        draws.append(sum(cand[i] - base[i] for i in picked) / len(picked))
    draws.sort()
    return {
        "difference": round(diff, 3),
        "low": round(draws[int(0.025 * reps)], 3),
        "high": round(draws[int(0.975 * reps) - 1], 3),
        "only_candidate_passed": sum(c and not b for b, c in zip(base, cand, strict=True)),
        "only_baseline_passed": sum(b and not c for b, c in zip(base, cand, strict=True)),
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--corpus", required=True, type=pathlib.Path)
    parser.add_argument("--baseline", required=True, type=pathlib.Path)
    parser.add_argument("--candidate", type=pathlib.Path)
    args = parser.parse_args(argv)
    cases = frozen_cases(json.loads(args.corpus.read_text(encoding="utf-8")))
    base = [case_outcome(args.baseline, c) for c in cases]
    report = {"cases": len(cases), "baseline": summary(cases, base)}
    if args.candidate:
        cand = [case_outcome(args.candidate, c) for c in cases]
        report["candidate"] = summary(cases, cand)
        report["paired_judge"] = paired_bootstrap(
            cases, [o["judge"] for o in base], [o["judge"] for o in cand]
        )
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
