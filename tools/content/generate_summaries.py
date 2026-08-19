#!/usr/bin/env python3
"""Generate the CONTENT-005 plain-language summaries for the 45 seed skills.

Verification tool, not production code. It produces the *review record* for the
curated content, not a second enrichment implementation: every summary in
``summaries.json`` came out of the platform's own ``POST /v1/enrich-skill``
(``apps/llm``), either freshly called here or loaded from an earlier call.

Two sources, both the production endpoint:

1. ``tools/goldenset/corpus_enriched/`` — 31 documents enriched for the golden
   set. A seed skill reuses one only when repo, path AND pinned commit all
   match, i.e. the same bytes were enriched. Those files are ``enrich-skill/v1``
   output, which predates the ``limitations`` field; reused rows therefore carry
   ``limitations: null`` (absent, not empty) and are marked ``origin: reused``.
   Reusing them also keeps the summary text identical to the one the golden set
   derived MaxCosineDistance = 0.75 from.
2. A live call for everything else. Package content is assembled exactly as the
   importer assembles it — ``import_seed.repack_skill`` on the pinned repo zip —
   so ``skill_md`` and ``file_tree`` are what production would have sent.

Key handling: this tool holds no key at all. ``apps/llm`` reads
``LITELLM_API_KEY`` from its own process environment, exported from the repo-root
``.env`` when the service is started. No key is written to disk by this tool and
none appears in the output.

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
TIMEOUT = 180  # the service's own ceiling is 60s per gateway call


# --- reuse matching ---------------------------------------------------------


def reusable(seed: dict, manifest: dict) -> dict[str, pathlib.Path]:
    """Map seed skill id -> existing enrichment file, for exact-content matches.

    Same repo, same path, same commit means the golden set enriched the very
    bytes this seed entry pins. Anything less (a moved path, a different commit)
    is a different document and gets a fresh call.
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


# --- enrichment -------------------------------------------------------------


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

    # --only forces a fresh call for the named ids even when a row already exists
    # or an exact-content reuse is available: that is how a v1 row gets rebuilt
    # under v2 without re-billing the other 37.
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


# --- checks -----------------------------------------------------------------


def selftest() -> int:
    seed = json.loads(SEED.read_text(encoding="utf-8"))
    manifest = json.loads((GOLDENSET / "manifest.json").read_text(encoding="utf-8"))

    assert len(seed["skills"]) == 45, len(seed["skills"])

    reuse = reusable(seed, manifest)
    # A name collision alone must not qualify: same name, different commit is a
    # different document.
    bent = json.loads(json.dumps(seed))
    for src in bent["sources"].values():
        src["commit"] = "0" * 40
    assert not reusable(bent, manifest), "commit mismatch must block reuse"

    # Every reused file is real production output carrying its provenance.
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
