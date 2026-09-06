"""The hybrid rule scored on three query sets at once: the 60 golden queries
(task phrasing, with distractors), 31 skill-name queries, 25 distinctive-token
queries. Vector leg = cosine within cut. Lexical leg = bigram tsvector ranked
by ts_rank_cd, admitted when the query's tokens are covered by the document
(coverage >= c). Union keeps vector order first, then lexical admissions.
"""
import json, os, re, subprocess, sys, urllib.request, collections
from pathlib import Path
ROOT = Path(r"C:/Users/a8022/OneDrive/Desktop/SkillHub"); GS = ROOT / "tools" / "goldenset"
sys.path.insert(0, str(GS)); os.chdir(GS)
import evaluate  # noqa: E402
TOKEN = "measure-local-2026-09-06"
OUT = Path(sys.argv[1]) if len(sys.argv) > 1 else None

def psql(sql):
    out = subprocess.run(["docker", "exec", "-i", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", "skillhub_f1", "-At", "-F", "|", "-v", "ON_ERROR_STOP=1"], input=sql.encode("utf-8"), capture_output=True)
    if out.returncode != 0: raise SystemExit(out.stderr.decode("utf-8", "replace"))
    return [l for l in out.stdout.decode("utf-8", "replace").splitlines() if l.strip()]
def q(s): return s.replace("'", "''")
def embed(texts):
    req = urllib.request.Request("http://localhost:8000/embed", data=json.dumps({"texts": texts}).encode(), headers={"Content-Type": "application/json", "Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=120) as r: return json.load(r)["embeddings"]

docs = []
for cat in ("data", "documents", "writing"):
    for md in sorted((GS / "corpus" / cat).glob("*.md")):
        fm = re.search(r"^---\n(.*?)\n---", md.read_text(encoding="utf-8"), re.S).group(1)
        name = re.search(r"^name:\s*(.+)$", fm, re.M).group(1).strip()
        enriched = json.loads((GS / "corpus_enriched" / cat / (md.stem + ".json")).read_text(encoding="utf-8"))
        text = evaluate.enriched_index_text(name, enriched)
        docs.append({"id": md.stem, "name": name, "index_text": text, "tokens": set(evaluate.tokenize(text))})
toks_of = {d["id"]: d["tokens"] for d in docs}
gold = json.load(open(GS / "queries.json", encoding="utf-8"))["queries"]
count = collections.Counter(t for d in docs for t in d["tokens"])
distinct = []
for d in docs:
    cands = sorted(t for t in d["tokens"] if count[t] == 1 and re.fullmatch(r"[a-z][a-z0-9+.#_-]{3,}", t))
    if cands: distinct.append({"query": cands[0], "rel": {d["id"]}, "set": "token"})
distinct = distinct[:25]
sets = ([{"query": x["query"], "rel": set(x["gold_primary"]) | set(x["gold_acceptable"]), "set": "golden", "lang": x["lang"]} for x in gold]
        + [{"query": d["name"], "rel": {d["id"]}, "set": "name"} for d in docs] + distinct)
cached = evaluate.embed([x["query"] for x in gold], allow_api=False)
extra = [x["query"] for x in sets if x["set"] != "golden"]
vecs = dict(zip(extra, embed(extra)))
def vector_of(qq): return cached.get(qq) or vecs[qq]

V, B = {}, {}
for x in sets:
    qq = x["query"]
    lit = "[" + ",".join(f"{v:.6f}" for v in vector_of(qq)) + "]"
    V[qq] = [(l.split("|")[0], float(l.split("|")[1])) for l in psql(f"SELECT id, embedding <=> '{lit}'::vector FROM search_docs ORDER BY 2 ASC LIMIT 10;")]
    qt = list(dict.fromkeys(evaluate.tokenize(qq)))
    if qt:
        t = " | ".join(qt)
        ranked = [l.split("|")[0] for l in psql(f"SELECT id, ts_rank_cd(bigram, to_tsquery('simple','{q(t)}')) FROM search_docs WHERE bigram @@ to_tsquery('simple','{q(t)}') ORDER BY 2 DESC LIMIT 10;")]
        B[qq] = [(i, sum(1 for tk in qt if tk in toks_of[i]) / len(qt)) for i in ranked]
    else:
        B[qq] = []

def pick(qq, cut, cov, lex_k, vec_first=True):
    got = [i for i, d in V[qq] if d <= cut]
    lex = [i for i, c in B[qq] if c >= cov][:lex_k]
    if vec_first:
        return got + [i for i in lex if i not in got]
    # lexical-first when it fully covers the query (a name or a distinctive term)
    full = [i for i, c in B[qq] if c >= 1.0][:lex_k]
    return full + [i for i in got if i not in full] + [i for i in lex if i not in got and i not in full]

def f1(got, rel):
    hit = len([g for g in got if g in rel]); p = hit / len(got) if got else 0.0; r = hit / len(rel)
    return (2 * p * r / (p + r) if p + r else 0.0)
def score(label, fn, k):
    res = {}
    for name in ("golden", "name", "token"):
        sub = [x for x in sets if x["set"] == name]
        tasks = [x for x in sub if x["rel"]]; dis = [x for x in sub if not x["rel"]]
        f = sum(f1(fn(x["query"])[:k], x["rel"]) for x in tasks) / len(tasks)
        top1 = sum(1 for x in tasks if fn(x["query"])[:1] and fn(x["query"])[0] in x["rel"])
        rej = sum(1 for x in dis if not fn(x["query"])[:k])
        res[name] = (f, top1, len(tasks), rej, len(dis))
    g, n, t = res["golden"], res["name"], res["token"]
    combined = (g[0] * g[2] + n[0] * n[2] + t[0] * t[2]) / (g[2] + n[2] + t[2])
    return combined, f"{label:<40} k={k}: golden F1={g[0]:.3f} top1={g[1]}/{g[2]} rej={g[3]}/{g[4]} | names F1={n[0]:.3f} top1={n[1]}/{n[2]} | tokens F1={t[0]:.3f} top1={t[1]}/{t[2]} | all F1={combined:.3f}"

rows = []
for k in (1, 2, 3):
    rows.append(score("vector cut0.55 only", lambda qq: [i for i, d in V[qq] if d <= 0.55], k))
    rows.append(score("vector cut0.75 only", lambda qq: [i for i, d in V[qq] if d <= 0.75], k))
    for cut in (0.55, 0.6):
        for cov in (0.6, 0.8, 1.0):
            for lk in (1, 2):
                rows.append(score(f"V<={cut} + lex cov>={cov} top{lk}", lambda qq, a=cut, b=cov, c=lk: pick(qq, a, b, c), k))
                rows.append(score(f"lexfull-first, V<={cut}, lex cov>={cov} top{lk}", lambda qq, a=cut, b=cov, c=lk: pick(qq, a, b, c, vec_first=False), k))
rows.sort(key=lambda r: -r[0])
text = "\n".join(r[1] for r in rows)
print(text[:5200])
if OUT: OUT.write_text(text + "\n", encoding="utf-8")
