"""Push the three poisoned SKILL.md files through the platform's own
index-time enrichment (POST /v1/enrich-skill, the prompt version the service
runs) and store what the index would hold for them — the fair LLM04 test:
not "what if the index text were the golden queries" (search_f1_score.py
--poison, the worst case) but "what does a keyword-stuffed package become
once the enrichment prompt has restated it".

    LLM_SERVICE_TOKEN=... python enrich_poison.py [--url http://127.0.0.1:8000]
Output: ./poison-enriched/<name>.json, then
    python ../search-f1/search_f1_score.py --docs ... --poison-dir docs/plans/mvp/m5/creation-measure/injection/poison-enriched out.txt
"""
import json
import os
import re
import sys
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
url = "http://127.0.0.1:8000"
if "--url" in sys.argv:
    url = sys.argv[sys.argv.index("--url") + 1].rstrip("/")
out_dir = HERE / "poison-enriched"
out_dir.mkdir(exist_ok=True)
for md in sorted((HERE / "poison").glob("*.md")):
    text = md.read_text(encoding="utf-8")
    name = re.search(r"^name:\s*(.+)$", text, re.M).group(1).strip()
    body = json.dumps({"skill_name": name, "skill_md": text, "file_tree": [], "language": "zh-Hant"}).encode()
    headers = {"Content-Type": "application/json"}
    if os.environ.get("LLM_SERVICE_TOKEN"):
        headers["Authorization"] = "Bearer " + os.environ["LLM_SERVICE_TOKEN"]
    req = urllib.request.Request(url + "/v1/enrich-skill", data=body, headers=headers)
    with urllib.request.urlopen(req, timeout=180) as resp:
        payload = json.load(resp)
    (out_dir / f"{name}.json").write_text(json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")
    print(name, payload.get("prompt_version"), len(payload.get("task_examples", [])), "examples;", len(payload.get("checks", [])), "checks")
