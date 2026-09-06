"""Where a lexical leg matters and the golden set cannot show it: queries that
are a skill's name, and queries that are one distinctive token (a term that
occurs in exactly one document's index text). Vector-only vs bigram lexical.
Uses the scratch table left by search_f1_hybrid2.py; name/token queries are
embedded through apps/llm (tiny spend, the owner approved embedding tests).
"""
import json, os, re, subprocess, sys, urllib.request, collections
from pathlib import Path
ROOT = Path(r"C:/Users/a8022/OneDrive/Desktop/SkillHub"); GS = ROOT / "tools" / "goldenset"
sys.path.insert(0, str(GS)); os.chdir(GS)
import evaluate  # noqa: E402
TOKEN = "measure-local-2026-09-06"

def psql(sql):
    out = subprocess.run(["docker", "exec", "-i", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", "skillhub_f1", "-At", "-F", "|", "-v", "ON_ERROR_STOP=1"], input=sql.encode("utf-8"), capture_output=True)
    if out.returncode != 0: raise SystemExit(out.stderr.decode("utf-8", "replace"))
    return [l for l in out.stdout.decode("utf-8", "replace").splitlines() if l.strip()]
def q(s): return s.replace("'", "''")
def embed(texts):
    req = urllib.request.Request("http://localhost:8000/embed", data=json.dumps({"texts": texts}).encode(), headers={"Content-Type": "application/json", "Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=120) as r: body = json.load(r)
    return body["embeddings"], (body.get("usage") or {}).get("cost_usd")

docs = []
for cat in ("data", "documents", "writing"):
    for md in sorted((GS / "corpus" / cat).glob("*.md")):
        fm = re.search(r"^---\n(.*?)\n---", md.read_text(encoding="utf-8"), re.S).group(1)
        name = re.search(r"^name:\s*(.+)$", fm, re.M).group(1).strip()
        enriched = json.loads((GS / "corpus_enriched" / cat / (md.stem + ".json")).read_text(encoding="utf-8"))
        docs.append({"id": md.stem, "name": name, "tokens": set(evaluate.tokenize(evaluate.enriched_index_text(name, enriched)))})
# distinctive tokens: latin, length >= 4, in exactly one document
count = collections.Counter(t for d in docs for t in d["tokens"])
distinct = []
for d in docs:
    cands = sorted(t for t in d["tokens"] if count[t] == 1 and re.fullmatch(r"[a-z][a-z0-9+.#_-]{3,}", t))
    if cands: distinct.append((cands[0], d["id"]))
distinct = distinct[:25]
names = [(d["name"], d["id"]) for d in docs]
queries = [n for n, _ in names] + [t for t, _ in distinct]
vecs, cost = embed(queries)
V = {}
for qq, v in zip(queries, vecs):
    lit = "[" + ",".join(f"{x:.6f}" for x in v) + "]"
    V[qq] = [(l.split("|")[0], float(l.split("|")[1])) for l in psql(f"SELECT id, embedding <=> '{lit}'::vector FROM search_docs ORDER BY 2 ASC LIMIT 3;")]
def bigram(qq):
    t = " | ".join(dict.fromkeys(evaluate.tokenize(qq)))
    return [l.split("|")[0] for l in psql(f"SELECT id, ts_rank_cd(bigram, to_tsquery('simple','{q(t)}')) FROM search_docs WHERE bigram @@ to_tsquery('simple','{q(t)}') ORDER BY 2 DESC LIMIT 3;")] if t.strip() else []
def report(label, pairs):
    v1 = v1cut = b1 = 0
    misses = []
    for qq, gold in pairs:
        top = V[qq][0] if V[qq] else (None, 9)
        v1 += top[0] == gold; v1cut += (top[0] == gold and top[1] <= 0.55)
        b = bigram(qq); b1 += bool(b) and b[0] == gold
        if not (top[0] == gold and top[1] <= 0.55): misses.append((qq, gold, top[0], round(top[1], 3), b[:1]))
    print(f"{label}: n={len(pairs)} vector_top1={v1} vector_top1_within0.55={v1cut} bigram_top1={b1}")
    for m in misses[:12]: print("   miss:", m)
report("name queries", names)
report("distinctive-token queries", distinct)
print(f"embed_cost_usd={cost}")
