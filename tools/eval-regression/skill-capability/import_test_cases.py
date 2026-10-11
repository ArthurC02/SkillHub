"""Create one task's holdout cases as test cases of a Skill on the platform.

    python import_test_cases.py --task SC01 --skill <UUID> --dry-run
    SKILLHUB_SESSION=<sh_session cookie> python import_test_cases.py --task SC01 --skill <UUID>

Each holdout case becomes a test case: its prompt, its criteria in order, and
the facts the person gives when asked. A criterion that names every expected
number is checked for those numbers before the judge reads it; the first
criterion of an `ask` case is checked for a question before the judge reads it.
Calls only the public API, as the person whose session cookie is given."""

from __future__ import annotations

import argparse
import json
import os
import pathlib
import sys
import urllib.request

TASKS = pathlib.Path(__file__).with_name("tasks-v1.json")
MAX_NAME_BYTES = 200


def judged_after(check: dict) -> dict:
    return {"kind": "sequence", "children": [check, {"kind": "judge"}]}


def criteria_of(case: dict) -> list[dict]:
    numbers = case.get("expected_numbers") or []
    out = []
    for i, text in enumerate(case["criteria"]):
        criterion = {"text": text}
        if i == 0 and case.get("enrichment") == "ask":
            criterion["check"] = judged_after({"kind": "asks"})
        elif numbers and all(n in text for n in numbers):
            criterion["check"] = judged_after({"kind": "contains_values", "values": numbers})
        out.append(criterion)
    return out


def test_cases_of(task: dict) -> list[dict]:
    cases = []
    for case in task["holdout"]:
        name = f"{task['id']} {case['name']}"
        while len(name.encode()) > MAX_NAME_BYTES:
            name = name[:-1]
        cases.append({
            "name": name,
            "user_prompt": case["prompt"],
            "simulated_user_facts": case.get("user_facts") or [],
            "criteria": criteria_of(case),
        })
    return cases


def find_task(task_id: str) -> dict:
    for task in json.loads(TASKS.read_text(encoding="utf-8"))["reference"]:
        if task["id"] == task_id:
            return task
    raise SystemExit(f"{TASKS.name} has no task {task_id}")


class Platform:
    def __init__(self, base: str, session: str):
        self.base, self.session = base.rstrip("/"), session

    def call(self, method: str, path: str, body: dict) -> dict:
        req = urllib.request.Request(
            self.base + path, method=method, data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json", "Cookie": f"sh_session={self.session}"},
        )
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.loads(r.read())

    def create(self, skill_id: str, case: dict) -> str:
        created = self.call("POST", "/test-cases", {
            "skill_id": skill_id, "name": case["name"], "user_prompt": case["user_prompt"],
        })
        test_case_id = created["test_case_id"]
        if case["simulated_user_facts"]:
            self.call("PATCH", f"/test-cases/{test_case_id}", {"simulated_user_facts": case["simulated_user_facts"]})
        for criterion in case["criteria"]:
            self.call("POST", f"/test-cases/{test_case_id}/criteria", criterion)
        return test_case_id


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--task", required=True)
    parser.add_argument("--skill", required=True)
    parser.add_argument("--base-url", default=os.environ.get("SKILLHUB_API", "http://localhost:8080"))
    parser.add_argument("--dry-run", action="store_true", help="print the test cases; call nothing")
    args = parser.parse_args()
    sys.stdout.reconfigure(encoding="utf-8")
    cases = test_cases_of(find_task(args.task))
    if args.dry_run:
        print(json.dumps(cases, ensure_ascii=False, indent=1))
        return
    session = os.environ.get("SKILLHUB_SESSION")
    if not session:
        raise SystemExit("set SKILLHUB_SESSION to the sh_session cookie of the person who owns the skill")
    platform = Platform(args.base_url, session)
    for case in cases:
        print(platform.create(args.skill, case), case["name"])


if __name__ == "__main__":
    main()
