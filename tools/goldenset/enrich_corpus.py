"""Generate real index-time enrichment for the golden-set corpus (ADR-013 section 1).

Calls the platform's own LLM service endpoint POST /v1/enrich-skill once per
corpus document, so the golden set measures the production prompt and schema
rather than a re-implementation of them. Output lands in ./corpus_enriched as
one JSON file per skill, carrying the model id and prompt version the service
reported.

The output doubles as the cache: a skill that already has a file is skipped, so
re-running costs nothing. Delete a file to re-enrich just that one.

Usage
  # start the LLM service first (see golden-query-set.md section 10)
  python enrich_corpus.py                       # enrich whatever is missing
  python enrich_corpus.py --url http://host:port
"""

from __future__ import annotations

import json
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).parent
CORPUS_DIR = ROOT / "corpus"
OUT_DIR = ROOT / "corpus_enriched"
DEFAULT_URL = "http://127.0.0.1:8099"
TIMEOUT = 180  # the service's own ceiling is 60s per gateway call


def enrich_one(base_url: str, skill_id: str, skill_md: str) -> dict:
    """POST one document to the service. The corpus is bare SKILL.md files, so
    file_tree stays empty — the service renders that as '(not provided)'."""
    body = json.dumps(
        {"skill_name": skill_id, "skill_md": skill_md, "file_tree": [], "language": "zh-Hant"}
    ).encode()
    req = urllib.request.Request(
        f"{base_url}/v1/enrich-skill", data=body, headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        return json.load(resp)


def main(base_url: str) -> None:
    manifest = json.loads((ROOT / "manifest.json").read_text(encoding="utf-8"))
    entries = manifest["documents"]
    done = failed = skipped = 0

    for i, entry in enumerate(entries, 1):
        out_path = OUT_DIR / entry["category"] / f"{entry['id']}.json"
        if out_path.exists():
            skipped += 1
            continue
        src = CORPUS_DIR / entry["category"] / f"{entry['id']}.md"
        skill_md = src.read_text(encoding="utf-8", errors="replace")
        print(f"[{i}/{len(entries)}] enriching {entry['id']}...", file=sys.stderr)
        try:
            payload = enrich_one(base_url, entry["id"], skill_md)
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as e:
            detail = e.read().decode("utf-8", "replace") if hasattr(e, "read") else str(e)
            print(f"  FAILED {entry['id']}: {detail[:300]}", file=sys.stderr)
            failed += 1
            continue
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(
            json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8"
        )
        done += 1

    print(f"\nenriched {done}, cached {skipped}, failed {failed}", file=sys.stderr)
    if failed:
        sys.exit(f"{failed} document(s) failed; rerun to retry only those")


if __name__ == "__main__":
    url = DEFAULT_URL
    if "--url" in sys.argv:
        url = sys.argv[sys.argv.index("--url") + 1]
    main(url.rstrip("/"))
