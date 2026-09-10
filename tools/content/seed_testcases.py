#!/usr/bin/env python3
"""Seed the curated Test Cases into a Skill Hub deployment.

Every curated Skill gets an example Dataset, an example User Prompt and
acceptance criteria, and the `writing` ones a rubric on top. Run it as the
catalog curator: a test case seeded into a personal workspace is excluded
from every export as `not_curated`.

Usage:
    python seed_testcases.py --api http://localhost:8080 --user catalog-curator
    python seed_testcases.py --dry-run          # resolve and print, change nothing
    python seed_testcases.py --replace          # re-seed: delete then recreate
    python seed_testcases.py --only humanizer   # substring filter on skill name
    python seed_testcases.py --selftest         # offline check of the builders
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
import pathlib
import sys
import uuid

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parents[1]
SEED = HERE / "seed-skills.json"
SUMMARIES = HERE / "summaries.json"
DATASETS = HERE / "seed-testcases"
RUBRICS = REPO / "tools" / "eval-regression" / "rubric-content-007-writing-v1.json"
DEFAULT_API = os.environ.get("SKILLHUB_API", "http://localhost:8080")

SLUG_PREFIX = "content-007/"

PROMPT_TEMPLATE = """請使用「{name}」這個 Skill 完成以下任務：{task}

執行環境說明：
1. 輸入檔案只有兩個，都在 /work/data/：data.csv（表格資料）與 draft.md（一段文字草稿）。
   上面的任務若提到其他檔名，一律改用這兩個檔案裡合適的那一個。
2. 所有產出檔案必須寫到 /out/artifacts/ 目錄；寫在其他地方的檔案不會被保存。
3. 完成後用一行文字說明你產出了哪些檔案。"""

WRITING_PROMPT_RULE = """
4. 最終回覆必須完整貼出這次產出的正文，不能只說明檔名。
   評估只讀得到你的最終回覆與檔案清單，讀不到檔案內容。"""

BASELINE_CRITERIA = [
    "trace 中出現對指定 Skill 的 skill_activation 事件。",
    "/out/artifacts/ 至少產出一個檔案。",
    "最終回覆說明了這次產出哪些檔案。",
]


def _load_import_seed():
    """Reuse the sibling tool's HTTP helpers rather than writing a second client."""
    spec = importlib.util.spec_from_file_location("import_seed", HERE / "import_seed.py")
    if spec is None or spec.loader is None:  # pragma: no cover - environment
        raise SystemExit("cannot load import_seed.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod



def curated_plan() -> list[dict]:
    """One entry per curated Skill: name, prompt, criteria, and rubric if any."""
    seed = json.loads(SEED.read_text(encoding="utf-8"))
    summaries = {
        row["skill"]: row
        for row in json.loads(SUMMARIES.read_text(encoding="utf-8"))["summaries"]
    }
    rubrics = json.loads(RUBRICS.read_text(encoding="utf-8"))

    plan = []
    for skill in seed["skills"]:
        if skill["tier"] != "curated":
            continue
        name = skill["name"]
        row = summaries.get(name)
        if not row or not row.get("task_examples"):
            raise SystemExit(f"{name}: no task example in summaries.json; cannot build a prompt")
        task = row["task_examples"][0]["zh_hant"]

        entry = {
            "skill_name": name,
            "category": skill["category"],
            "test_case_name": SLUG_PREFIX + name,
            "user_prompt": PROMPT_TEMPLATE.format(name=name, task=task),
            "criteria": list(BASELINE_CRITERIA),
            "rubric": None,
        }
        if name in rubrics["skills"]:
            spec = rubrics["skills"][name]
            entry["user_prompt"] += "\n" + WRITING_PROMPT_RULE.strip("\n")
            entry["criteria"] += [c["text"] for c in spec["criteria"]]
            entry["rubric"] = {
                "version": rubrics["rubric_version"],
                "items": [dict(i) for i in spec["rubric"]["items"]],
                "_curated_criteria": spec["criteria"],
            }
        plan.append(entry)
    return plan


def resolve_rubric_ids(rubric: dict, created_criteria: list[dict]) -> dict:
    """Repoint rubric item ids at the criterion ids the server assigned.

    Matched by criterion text, since criterion ids are minted server-side.
    """
    by_text = {c["text"]: c["id"] for c in created_criteria}
    curated_by_id = {c["id"]: c["text"] for c in rubric["_curated_criteria"]}

    items = []
    for item in rubric["items"]:
        text = curated_by_id.get(item["id"])
        if text is None:
            raise SystemExit(f"rubric item {item['id']} names no curated criterion")
        server_id = by_text.get(text)
        if server_id is None:
            raise SystemExit(f"rubric item {item['id']}: criterion text not found on the draft")
        items.append({k: v for k, v in item.items() if k != "id"} | {"id": server_id})
    return {"version": rubric["version"], "items": items}



def multipart(field: str, filename: str, data: bytes) -> tuple[bytes, str]:
    boundary = "----skillhub" + uuid.uuid4().hex
    body = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="{field}"; filename="{filename}"\r\n'
        f"Content-Type: application/octet-stream\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


class Client:
    """Thin wrapper over import_seed's opener so failures read as one line."""

    def __init__(self, mod, api: str):
        self.mod, self.api = mod, api
        self.opener = mod.make_opener()

    def call(self, method: str, path: str, *, json_body=None, raw=None, ctype=None, want=(200, 201)):
        import urllib.request

        url = self.api + path
        data = None
        if json_body is not None:
            data, ctype = json.dumps(json_body).encode(), "application/json"
        elif raw is not None:
            data = raw
        req = urllib.request.Request(url, data=data, method=method)
        if ctype:
            req.add_header("Content-Type", ctype)
        import urllib.error

        try:
            with self.opener.open(req, timeout=120) as resp:
                status, body = resp.status, resp.read()
        except urllib.error.HTTPError as e:
            status, body = e.code, e.read()
        except urllib.error.URLError as e:
            raise SystemExit(f"{method} {path}: {e.reason}")
        if status not in want:
            raise SystemExit(f"{method} {path} -> {status}: {body[:300].decode('utf-8', 'replace')}")
        return json.loads(body) if body else {}



def seed_one(client: Client, entry: dict, skill_id: str, files: dict[str, bytes]) -> dict:
    tc = client.call("POST", "/test-cases", json_body={
        "skill_id": skill_id,
        "name": entry["test_case_name"],
        "user_prompt": entry["user_prompt"],
    })
    tcid = tc["test_case_id"]

    for filename, data in files.items():
        body, ctype = multipart("file", filename, data)
        client.call("POST", f"/test-cases/{tcid}/datasets", raw=body, ctype=ctype)

    for text in entry["criteria"]:
        tc = client.call("POST", f"/test-cases/{tcid}/criteria", json_body={"text": text})

    if entry["rubric"]:
        rubric = resolve_rubric_ids(entry["rubric"], tc["acceptance_criteria"])
        tc = client.call("PATCH", f"/test-cases/{tcid}", json_body={"rubric": rubric})

    return tc


def run(args) -> int:
    mod = _load_import_seed()
    plan = curated_plan()
    if args.only:
        plan = [e for e in plan if args.only in e["skill_name"]]
    files = {p.name: p.read_bytes() for p in sorted(DATASETS.iterdir()) if p.is_file()}
    if not files:
        raise SystemExit(f"no dataset files in {DATASETS}")

    client = Client(mod, args.api)
    mod.dev_login(client.opener, args.api, args.user)

    skills = {s["name"]: s["skill_id"] for s in client.call("GET", "/skills")["skills"]}
    existing = {
        (t["skill_id"], t["name"]): t["test_case_id"]
        for t in client.call("GET", "/test-cases")["test_cases"]
    }

    results = []
    for entry in plan:
        name = entry["skill_name"]
        row = {"skill": name, "rubric_items": len(entry["rubric"]["items"]) if entry["rubric"] else 0}
        skill_id = skills.get(name)
        if skill_id is None:
            row["status"] = "skill_absent"
            results.append(row)
            print(f"  {name:<28} skill_absent (run import_seed.py as this user first)")
            continue
        row["skill_id"] = skill_id

        prior = existing.get((skill_id, entry["test_case_name"]))
        if prior and not args.replace:
            row["status"] = "exists_skipped"
            row["test_case_id"] = prior
            results.append(row)
            print(f"  {name:<28} exists_skipped ({prior})")
            continue

        if args.dry_run:
            row["status"] = "would_replace" if prior else "would_create"
            results.append(row)
            print(f"  {name:<28} {row['status']}")
            continue

        if prior:
            client.call("DELETE", f"/test-cases/{prior}")
        tc = seed_one(client, entry, skill_id, files)
        row["status"] = "replaced" if prior else "created"
        row["test_case_id"] = tc["test_case_id"]
        row["criteria"] = len(tc["acceptance_criteria"])
        row["rubric_version"] = (tc.get("rubric") or {}).get("version")
        results.append(row)
        print(f"  {name:<28} {row['status']} ({row['test_case_id']}) "
              f"criteria={row['criteria']} rubric={row['rubric_version'] or '-'}")

    print()
    for bucket in sorted({r["status"] for r in results}):
        print(f"  {bucket}: {sum(1 for r in results if r['status'] == bucket)}")
    if args.out:
        pathlib.Path(args.out).write_text(
            json.dumps({"api": args.api, "user": args.user, "results": results},
                       ensure_ascii=False, indent=1), encoding="utf-8")
        print(f"\nwrote {args.out}")
    return 0 if all(r["status"] != "skill_absent" for r in results) else 1



def selftest() -> int:
    plan = curated_plan()
    assert len(plan) == 15, len(plan)

    with_rubric = [e for e in plan if e["rubric"]]
    assert len(with_rubric) == 5, [e["skill_name"] for e in with_rubric]
    assert sum(len(e["rubric"]["items"]) for e in with_rubric) == 22

    for e in plan:
        assert e["test_case_name"] == SLUG_PREFIX + e["skill_name"]
        assert e["skill_name"] in e["user_prompt"], e["skill_name"]
        assert "/out/artifacts/" in e["user_prompt"]
        assert e["criteria"][:3] == BASELINE_CRITERIA
        assert len(set(e["criteria"])) == len(e["criteria"]), f"{e['skill_name']} duplicate criterion"
        if e["rubric"]:
            assert "最終回覆必須完整貼出" in e["user_prompt"], e["skill_name"]
            assert len(e["criteria"]) == 3 + len(e["rubric"]["_curated_criteria"])
        else:
            assert "最終回覆必須完整貼出" not in e["user_prompt"]

    e = next(x for x in plan if x["skill_name"] == "humanizer")
    created = [{"id": f"srv-{i}", "text": t} for i, t in enumerate(e["criteria"])]
    resolved = resolve_rubric_ids(e["rubric"], created)
    assert resolved["version"] == "content-007/writing/v1"
    assert [i["id"] for i in resolved["items"]] == ["srv-3", "srv-4", "srv-5", "srv-6"], resolved
    assert all(i["id"].startswith("srv-") for i in resolved["items"])
    assert resolved["items"][0]["evidence_required"] is True
    assert "_curated_criteria" not in resolved

    try:
        resolve_rubric_ids(e["rubric"], created[:3])
    except SystemExit:
        pass
    else:
        raise AssertionError("a missing criterion must abort")

    files = {p.name: p.read_bytes() for p in sorted(DATASETS.iterdir()) if p.is_file()}
    assert set(files) == {"data.csv", "draft.md"}, sorted(files)
    for name, data in files.items():
        text = data.decode("utf-8")
        assert "@" not in text, f"{name}: looks like it carries an address"
        assert "BEGIN" not in text and "SECRET" not in text.upper(), name

    body, ctype = multipart("file", "data.csv", b"a,b\n1,2\n")
    assert ctype.startswith("multipart/form-data; boundary=")
    assert b'filename="data.csv"' in body and body.endswith(b"--\r\n")

    print(f"selftest ok (15 test cases, {sum(len(e['criteria']) for e in plan)} criteria, 22 rubric items)")
    return 0


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--api", default=DEFAULT_API)
    p.add_argument("--user", default="catalog-curator",
                   help="dev-login user; must own the workspace with is_catalog = true")
    p.add_argument("--only", help="substring filter on skill name")
    p.add_argument("--replace", action="store_true", help="delete and recreate existing seeds")
    p.add_argument("--dry-run", action="store_true", help="resolve and report, change nothing")
    p.add_argument("--out", help="write the per-skill result rows to this file")
    p.add_argument("--selftest", action="store_true", help="offline check, no API needed")
    args = p.parse_args()
    if args.selftest:
        return selftest()
    return run(args)


if __name__ == "__main__":
    sys.exit(main())
