#!/usr/bin/env python3
"""Generate the plain-language summaries for the seed skills.

Every summary comes from the platform's own ``POST /v1/enrich-skill``
(``apps/llm``), either called here or reused from an earlier golden-set call
against the same pinned bytes.

Usage
  # start the LLM service first (apps/llm README), then:
  python generate_summaries.py                 # fill in whatever is missing
  python generate_summaries.py --url http://127.0.0.1:8099
  python generate_summaries.py --only a,b      # re-call just these ids (v1 -> v2 rebuild)
  python generate_summaries.py --dry-run       # report reuse/new split, no calls
  python generate_summaries.py --selftest      # offline checks
"""

from __future__ import annotations

import argparse
import datetime as dt
import io
import json
import pathlib
import sys
import tempfile
import urllib.error
import urllib.request
import zipfile

sys.path.insert(0, str(pathlib.Path(__file__).parent))
from import_seed import download_repo_zip, repack_skill

HERE = pathlib.Path(__file__).parent
SEED = HERE / "seed-skills.json"
OUT = HERE / "summaries.json"
GOLDENSET = HERE.parent / "goldenset"
DEFAULT_URL = "http://127.0.0.1:8099"
TIMEOUT = 180



def reusable(seed: dict, manifest: dict) -> dict[str, pathlib.Path]:
    """Map seed skill id -> existing enrichment file, for exact-content matches.

    Same repo, same path, same commit means the golden set enriched the very
    bytes this seed entry pins.
    """
    sources = seed["sources"]
    by_name = {s["name"]: s for s in seed["skills"]}
    out: dict[str, pathlib.Path] = {}
    for doc in manifest["documents"]:
        skill = by_name.get(doc["id"])
        if skill is None:
            continue
        src = sources[skill["source_id"]]
        if (
            doc["repo"] in src["url"]
            and doc["path"] == skill["skill_md_path"]
            and src["commit"].startswith(doc["commit"])
        ):
            path = GOLDENSET / "corpus_enriched" / doc["category"] / f"{doc['id']}.json"
            if path.exists():
                out[skill["id"]] = path
    return out



def package_content(seed: dict, skill: dict, cache: pathlib.Path) -> tuple[str, list[str]]:
    """SKILL.md text and package file tree, as the importer would build them."""
    source = seed["sources"][skill["source_id"]]
    repo_zip = download_repo_zip(source, cache)
    packed = io.BytesIO(repack_skill(repo_zip, skill["skill_md_path"], source))
    with zipfile.ZipFile(packed) as pkg:
        tree = sorted(n for n in pkg.namelist() if not n.endswith("/"))
        skill_md = pkg.read("SKILL.md").decode("utf-8", "replace")
    return skill_md, tree


def enrich_one(base_url: str, name: str, skill_md: str, file_tree: list[str]) -> dict:
    body = json.dumps(
        {
            "skill_name": name,
            "skill_md": skill_md,
            "file_tree": file_tree,
            "language": "zh-Hant",
        }
    ).encode()
    req = urllib.request.Request(
        f"{base_url}/v1/enrich-skill", data=body, headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        return json.load(resp)


def record(skill: dict, source: dict, payload: dict, origin: str, generated_at: str) -> dict:
    """One reviewable row. `limitations: null` means the prompt version that
    produced this payload had no such field — distinct from `[]`, which means
    the model found nothing to state."""
    return {
        "id": skill["id"],
        "skill": skill["name"],
        "category": skill["category"],
        "tier": skill["tier"],
        "source_url": source["url"],
        "source_commit": source["commit"],
        "skill_md_path": skill["skill_md_path"],
        "origin": origin,
        "summary": payload["summary"],
        "task_examples": payload["task_examples"],
        "tags": payload["tags"],
        "limitations": payload.get("limitations"),
        "model": payload["model"],
        "prompt_version": payload["prompt_version"],
        "generated_at": generated_at,
    }


def run(args) -> int:
    seed = json.loads(SEED.read_text(encoding="utf-8"))
    manifest = json.loads((GOLDENSET / "manifest.json").read_text(encoding="utf-8"))
    reuse = reusable(seed, manifest)

    existing = {}
    if OUT.exists() and not args.refresh:
        existing = {r["id"]: r for r in json.loads(OUT.read_text(encoding="utf-8"))["summaries"]}

    picked = args.only.split(",") if args.only else []
    refetch = lambda sid: any(p and p in sid for p in picked)  # noqa: E731

    todo = [
        s
        for s in seed["skills"]
        if refetch(s["id"]) or (s["id"] not in existing and s["id"] not in reuse)
    ]
    print(
        f"45 seed skills: {len(reuse)} reusable from corpus_enriched, "
        f"{len(existing)} already in summaries.json, {len(todo)} need a new call",
        file=sys.stderr,
    )
    if args.dry_run:
        for s in todo:
            print(f"  new call: {s['id']}", file=sys.stderr)
        return 0

    cache = pathlib.Path(args.cache)
    rows, failed, calls = [], 0, 0
    for i, skill in enumerate(seed["skills"], 1):
        source = seed["sources"][skill["source_id"]]
        if skill["id"] in existing and not refetch(skill["id"]):
            rows.append(existing[skill["id"]])
            continue
        if skill["id"] in reuse and not refetch(skill["id"]):
            payload = json.loads(reuse[skill["id"]].read_text(encoding="utf-8"))
            rows.append(
                record(skill, source, payload, "reused:goldenset", manifest["fetched_at"][:10])
            )
            continue
        print(f"[{i}/45] enriching {skill['id']}...", file=sys.stderr)
        try:
            skill_md, tree = package_content(seed, skill, cache)
            payload = enrich_one(args.url, skill["name"], skill_md, tree)
        except (urllib.error.URLError, TimeoutError, OSError, json.JSONDecodeError, KeyError) as e:
            detail = e.read().decode("utf-8", "replace") if hasattr(e, "read") else str(e)
            print(f"  FAILED {skill['id']}: {detail[:300]}", file=sys.stderr)
            failed += 1
            continue
        calls += 1
        rows.append(record(skill, source, payload, "new", dt.date.today().isoformat()))

    OUT.write_text(
        json.dumps(
            {
                "$comment": (
                    "CONTENT-005 plain-language summaries for the seed catalogue. Every row is "
                    "output of the production POST /v1/enrich-skill; origin says whether it was "
                    "called here or loaded from tools/goldenset/corpus_enriched. Human review "
                    "state lives in docs/plans/mvp/content/content-summaries.md, not here."
                ),
                "generated_at": dt.date.today().isoformat(),
                "endpoint": "POST /v1/enrich-skill (apps/llm)",
                "counts": {
                    "total": len(rows),
                    "reused": sum(1 for r in rows if r["origin"] == "reused:goldenset"),
                    "new_calls": calls,
                },
                "summaries": rows,
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )
    print(f"\nwrote {OUT} — {len(rows)} rows, {calls} new calls, {failed} failed", file=sys.stderr)
    return 1 if failed else 0



def selftest() -> int:
    seed = json.loads(SEED.read_text(encoding="utf-8"))
    manifest = json.loads((GOLDENSET / "manifest.json").read_text(encoding="utf-8"))

    assert len(seed["skills"]) == 45, len(seed["skills"])

    reuse = reusable(seed, manifest)
    bent = json.loads(json.dumps(seed))
    for src in bent["sources"].values():
        src["commit"] = "0" * 40
    assert not reusable(bent, manifest), "commit mismatch must block reuse"

    for skill_id, path in reuse.items():
        payload = json.loads(path.read_text(encoding="utf-8"))
        assert payload["model"] and payload["prompt_version"], skill_id
        assert payload["summary"] and payload["task_examples"], skill_id

    src = seed["sources"][seed["skills"][0]["source_id"]]
    row = record(
        seed["skills"][0],
        src,
        {
            "summary": "x",
            "task_examples": [],
            "tags": {},
            "model": "m",
            "prompt_version": "enrich-skill/v1",
        },
        "reused:goldenset",
        "2026-08-15",
    )
    assert row["limitations"] is None, "v1 payload must record limitations as absent, not empty"

    print(f"selftest ok — 45 seed skills, {len(reuse)} reusable")
    return 0


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--url", default=DEFAULT_URL, help="apps/llm base URL")
    p.add_argument(
        "--cache", default=str(pathlib.Path(tempfile.gettempdir()) / "skillhub-seed-cache")
    )
    p.add_argument(
        "--dry-run", action="store_true", help="report the reuse/new split, call nothing"
    )
    p.add_argument("--refresh", action="store_true", help="ignore existing summaries.json rows")
    p.add_argument(
        "--only",
        help="comma-separated substrings of skill ids to re-call, ignoring any existing row or reuse",
    )
    p.add_argument("--selftest", action="store_true", help="offline checks")
    args = p.parse_args()
    args.url = args.url.rstrip("/")
    return selftest() if args.selftest else run(args)


if __name__ == "__main__":
    sys.exit(main())
