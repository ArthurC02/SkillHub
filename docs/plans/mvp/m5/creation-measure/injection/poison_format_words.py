"""Can a keyword-stuffed package be told from a real one by counting how many
distinct file formats its tags claim? (05 SEC-013, LLM04; report §16.2.)

This is the *other* candidate signal — the surface one. The withdrawn
`enrich_checks` overreach rule counted exactly this: distinct format words
across a document's `tags.inputs` and `tags.outputs`, with "five or more" as
the proposed threshold. The rule was withdrawn because the ranges overlap; this
script is the evidence for that sentence, so the claim stops being an assertion
nobody can re-derive.

No model calls, no network: it reads the enriched JSON that already exists.

    python poison_format_words.py [out.txt]
"""

import glob
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[5].parents[0]
OUT = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else None

# The word list the withdrawn overreach rule used, unchanged. It is deliberately
# a closed list of file formats: "excel" and "word" are product names that stand
# for a format in these tags, so they are counted too.
FORMATS = (
    "pdf", "docx", "doc", "xlsx", "xlsm", "xls", "pptx", "csv", "tsv", "jsonl",
    "json", "html", "markdown", "md", "png", "jpg", "jpeg", "svg", "mp3", "mp4",
    "zip", "excel", "word", "powerpoint", "yaml", "xml", "txt",
)


def formats_of(doc: dict) -> list[str]:
    tags = doc.get("tags") or {}
    joined = " ".join(tags.get("inputs", []) + tags.get("outputs", [])).lower()
    words = set(re.findall(r"[a-z0-9]+", joined))
    return sorted(f for f in FORMATS if f in words)


rows = []
for label, pattern in (
    ("real", str(ROOT / "tools/goldenset/corpus_enriched_v7/*/*.json")),
    ("poison", str(Path(__file__).parent / "poison-enriched/*.json")),
):
    for p in sorted(glob.glob(pattern)):
        found = formats_of(json.loads(Path(p).read_text(encoding="utf-8")))
        rows.append((label, Path(p).stem, found))

rows.sort(key=lambda r: -len(r[2]))
lines = [
    "distinct format words in tags.inputs + tags.outputs",
    f"word list ({len(FORMATS)}): {', '.join(FORMATS)}",
    "",
]
lines += [f"{label:6s} {name:32s} {len(found):2d}  {', '.join(found)}" for label, name, found in rows]
real = [len(f) for l, _, f in rows if l == "real"]
poison = [(n, len(f)) for l, n, f in rows if l == "poison"]
lines += [
    "",
    f"real:   n={len(real)} max={max(real)} p90={sorted(real)[int(0.9 * len(real)) - 1]} median={sorted(real)[len(real) // 2]}",
    "poison: " + ", ".join(f"{n}={c}" for n, c in poison),
    f"separable by a single threshold: {'yes' if min(c for _, c in poison) > max(real) else 'no'} "
    f"(poison min {min(c for _, c in poison)} vs real max {max(real)})",
]
report = "\n".join(lines)
print(report)
if OUT:
    OUT.write_text(report + "\n", encoding="utf-8")
