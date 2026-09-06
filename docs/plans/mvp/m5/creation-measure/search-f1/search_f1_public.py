"""The public search rule (05 R-48) scored on the same three query sets as the
creation tool's hybrid rule (search_f1_hybrid3.py): the 60 golden queries, the
31 skill-name queries, the 25 distinctive-token queries.

Before: vector within 0.75 (candidates widened by an english tsvector nobody
Chinese can hit). After A: the same, plus every bigram-covered document kept
past the cut-off, ranked by its own distance after the vector hits. After B:
the covered documents first (still in their own distance order), then the
vector hits. Both pin the exact name first. The public page is a list, so the
metrics are the page's: Top-1, Top-3, recall@5, and the distractor queries'
rejections at k=5. B is what shipped (05 R-48).

    python search_f1_public.py [out.txt]    (apps/llm on :8000, DB skillhub_f1)
"""
import collections
import json
import os
import re
import subprocess
import sys
import urllib.request
from pathlib import Path

ROOT = Path(r"C:/Users/a8022/OneDrive/Desktop/SkillHub")
GS = ROOT / "tools" / "goldenset"
OUT = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else None
sys.path.insert(0, str(GS))
os.chdir(GS)
import evaluate  # noqa: E402

TOKEN = "measure-local-2026-09-06"


def psql(sql):
    out = subprocess.run(["docker", "exec", "-i", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", "skillhub_f1", "-At", "-F", "|", "-v", "ON_ERROR_STOP=1"], input=sql.encode("utf-8"), capture_output=True)
    if out.returncode != 0:
        raise SystemExit(out.stderr.decode("utf-8", "replace"))
    return [l for l in out.stdout.decode("utf-8", "replace").splitlines() if l.strip()]


def q(s):
    return s.replace("'", "''")


def embed(texts):
    req = urllib.request.Request("http://localhost:8000/embed", data=json.dumps({"texts": texts}).encode(), headers={"Content-Type": "application/json", "Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=120) as r:
        return json.load(r)["embeddings"]


docs = []
for cat in ("data", "documents", "writing"):
    for md in sorted((GS / "corpus" / cat).glob("*.md")):
        fm = re.search(r"^---\n(.*?)\n---", md.read_text(encoding="utf-8"), re.S).group(1)
        name = re.search(r"^name:\s*(.+)$", fm, re.M).group(1).strip()
        enriched = json.loads((GS / "corpus_enriched" / cat / (md.stem + ".json")).read_text(encoding="utf-8"))
        text = evaluate.enriched_index_text(name, enriched)
        docs.append({"id": md.stem, "name": name, "tokens": set(evaluate.tokenize(text))})
toks_of = {d["id"]: d["tokens"] for d in docs}
name_of = {d["id"]: d["name"].lower() for d in docs}
gold = json.load(open(GS / "queries.json", encoding="utf-8"))["queries"]
count = collections.Counter(t for d in docs for t in d["tokens"])
distinct = []
for d in docs:
    cands = sorted(t for t in d["tokens"] if count[t] == 1 and re.fullmatch(r"[a-z][a-z0-9+.#_-]{3,}", t))
    if cands:
        distinct.append({"query": cands[0], "rel": {d["id"]}, "set": "token"})
distinct = distinct[:25]
sets = ([{"query": x["query"], "rel": set(x["gold_primary"]) | set(x["gold_acceptable"]), "set": "golden"} for x in gold]
        + [{"query": d["name"], "rel": {d["id"]}, "set": "name"} for d in docs] + distinct)
cached = evaluate.embed([x["query"] for x in gold], allow_api=False)
extra = [x["query"] for x in sets if x["set"] != "golden"]
vecs = dict(zip(extra, embed(extra)))


def vector_of(qq):
    return cached.get(qq) or vecs[qq]


V, B = {}, {}
for x in sets:
    qq = x["query"]
    lit = "[" + ",".join(f"{v:.6f}" for v in vector_of(qq)) + "]"
    V[qq] = [(l.split("|")[0], float(l.split("|")[1])) for l in psql(f"SELECT id, embedding <=> '{lit}'::vector FROM search_docs ORDER BY 2 ASC LIMIT 50;")]
    qt = list(dict.fromkeys(evaluate.tokenize(qq)))
    if qt:
        t = " & ".join(qt)
        B[qq] = [l.split("|")[0] for l in psql(f"SELECT id, ts_rank_cd(bigram, to_tsquery('simple','{q(t)}')) FROM search_docs WHERE bigram @@ to_tsquery('simple','{q(t)}') ORDER BY 2 DESC LIMIT 5;")]
    else:
        B[qq] = []
dist = {qq: dict(V[qq]) for qq in V}


def before(qq, cut=0.75):
    return [i for i, d in V[qq] if d <= cut]


def after(qq, cut=0.75):
    kept = [i for i, d in V[qq] if d <= cut]
    covered = [i for i in B[qq] if i not in kept]
    # covered rows keep their own distance and rank after the vector hits
    covered.sort(key=lambda i: dist[qq][i])
    page = kept + covered
    pinned = [i for i in page if name_of[i] == qq.strip().lower()]
    return pinned + [i for i in page if i not in pinned]


def covered_first(qq, cut=0.75):
    # the alternative: a document that carries every token of the query is a
    # precise signal, ranked ahead of the vector hits (exact lookups favour the
    # lexical leg); vector order after it, name pinned first of all
    kept = [i for i, d in V[qq] if d <= cut]
    covered = sorted(B[qq], key=lambda i: dist[qq][i])
    page = covered + [i for i in kept if i not in covered]
    pinned = [i for i in page if name_of[i] == qq.strip().lower()]
    return pinned + [i for i in page if i not in pinned]


def score(label, fn):
    lines = [f"== {label}"]
    for name in ("golden", "name", "token"):
        sub = [x for x in sets if x["set"] == name]
        tasks = [x for x in sub if x["rel"]]
        dis = [x for x in sub if not x["rel"]]
        top1 = sum(1 for x in tasks if fn(x["query"])[:1] and fn(x["query"])[0] in x["rel"])
        top3 = sum(1 for x in tasks if any(i in x["rel"] for i in fn(x["query"])[:3]))
        r5 = sum(1 for x in tasks if any(i in x["rel"] for i in fn(x["query"])[:5]))
        rej = sum(1 for x in dis if not fn(x["query"])[:5])
        extra = f" rej@5={rej}/{len(dis)}" if dis else ""
        lines.append(f"  {name:6s} n={len(tasks):2d} top1={top1:2d} top3={top3:2d} recall@5={r5:2d}{extra}")
    return "\n".join(lines)


report = "\n".join([score("before: vector <= 0.75 only", before),
                    score("after A: vector hits, then covered bigram hits kept past the cut-off, exact name pinned", after),
                    score("after B: covered bigram hits first, then vector hits, exact name pinned", covered_first)])
for label, fn in (("A", after), ("B", covered_first)):
    misses = [x["query"] for x in sets if x["rel"] and not (fn(x["query"])[:1] and fn(x["query"])[0] in x["rel"]) and x["set"] != "golden"]
    report += f"\n\nname/token queries not Top-1 under {label}: " + (", ".join(misses) if misses else "(none)")
    gm = [x["query"] for x in sets if x["rel"] and x["set"] == "golden" and not (fn(x["query"])[:1] and fn(x["query"])[0] in x["rel"])]
    report += f"\ngolden queries not Top-1 under {label}: " + (", ".join(gm) if gm else "(none)")
print(report)
if OUT:
    OUT.write_text(report + "\n", encoding="utf-8")
