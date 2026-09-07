"""Can a keyword-stuffed package be told from a real one by how far apart its
own task examples sit? (05 SEC-013, LLM04; report §16.)

For every enriched document — the 31 real ones (corpus_enriched_v7) and the
three poisoned ones (poison-enriched) — embed each task example (en) and take
the mean pairwise cosine distance. A Skill that does one job phrases it eight
ways (low dispersion); a package claiming every job in three categories has
examples that are about different things (high dispersion). Surface features
(format words in tags, tag counts, summary length) do NOT separate them —
data-analyst legitimately spans as many formats as the poisoned data one — so
this is the candidate that is actually semantic.

    LLM_SERVICE_TOKEN=... python poison_dispersion.py [out.txt]
"""
import glob
import json
import math
import os
import sys
import urllib.request
from pathlib import Path

ROOT = Path(r"C:/Users/a8022/OneDrive/Desktop/SkillHub")
TOKEN = os.environ.get("LLM_SERVICE_TOKEN", "")
OUT = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else None


def embed(texts):
    req = urllib.request.Request("http://localhost:8000/embed", data=json.dumps({"texts": texts}).encode(), headers={"Content-Type": "application/json", "Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=180) as r:
        return json.load(r)["embeddings"]


def cos_dist(a, b):
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(x * x for x in b))
    return 1 - dot / (na * nb)


def dispersion(vecs):
    pairs = [(i, j) for i in range(len(vecs)) for j in range(i + 1, len(vecs))]
    return sum(cos_dist(vecs[i], vecs[j]) for i, j in pairs) / len(pairs) if pairs else 0.0


rows = []
for label, pattern in (("real", str(ROOT / "tools/goldenset/corpus_enriched_v7/*/*.json")), ("poison", str(ROOT / "docs/plans/mvp/m5/creation-measure/injection/poison-enriched/*.json"))):
    for p in sorted(glob.glob(pattern)):
        d = json.loads(Path(p).read_text(encoding="utf-8"))
        examples = [e["en"] for e in d["task_examples"] if e.get("en")]
        if len(examples) < 2:
            continue
        rows.append((label, Path(p).stem, dispersion(embed(examples))))
rows.sort(key=lambda r: -r[2])
lines = ["mean pairwise cosine distance between a document's own task examples (en)", ""]
lines += [f"{label:6s} {name:32s} {d:.3f}" for label, name, d in rows]
real = [d for l, _, d in rows if l == "real"]
poison = [d for l, _, d in rows if l == "poison"]
lines += ["", f"real: n={len(real)} max={max(real):.3f} p90={sorted(real)[int(0.9 * len(real)) - 1]:.3f} median={sorted(real)[len(real) // 2]:.3f}",
          f"poison: {', '.join(f'{d:.3f}' for d in poison)}",
          f"separable by a single threshold: {'yes' if min(poison) > max(real) else 'no'} (gap {min(poison) - max(real):+.3f})"]
report = "\n".join(lines)
print(report)
if OUT:
    OUT.write_text(report + "\n", encoding="utf-8")
