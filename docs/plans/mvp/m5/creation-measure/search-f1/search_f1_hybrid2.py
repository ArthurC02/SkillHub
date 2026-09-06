"""Hybrid retrieval shapes on the golden query set, scored by F1@k.

Legs on the product-shaped scratch table:
  V  vector (enriched index text, text-embedding-3-small), cosine distance
  B  bigram lexical: the golden set's tokenizer (latin words + CJK character
     bigrams) applied at index and query time, stored as tsvector('simple'),
     ranked by ts_rank_cd  — a lexical leg that can see Chinese without an
     extension
  N  name leg: trigram word_similarity of the query against the skill name
Fusions:
  rrf        reciprocal-rank fusion of V and B
  boost      vector-first: candidates = V within cut; score = (1-d) + beta*Bnorm
  union      vector-first, lexical admitted only when it is the top lexical hit
             AND its vector distance is within cut+slack (a recall guard)
  fallback   vector within cut; if empty, top lexical hits
"""
import json, os, re, subprocess, sys
from pathlib import Path

ROOT = Path(r"C:/Users/a8022/OneDrive/Desktop/SkillHub")
GS = ROOT / "tools" / "goldenset"
sys.path.insert(0, str(GS)); os.chdir(GS)
import evaluate  # noqa: E402
OUT = Path(sys.argv[1]) if len(sys.argv) > 1 else None

def psql(sql, db="skillhub_f1"):
    out = subprocess.run(["docker", "exec", "-i", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", db, "-At", "-F", "|", "-v", "ON_ERROR_STOP=1"], input=sql.encode("utf-8"), capture_output=True)
    if out.returncode != 0:
        raise SystemExit(out.stderr.decode("utf-8", "replace"))
    return [l for l in out.stdout.decode("utf-8", "replace").splitlines() if l.strip()]
def q(s): return s.replace("'", "''")
def toks(text): return " ".join(evaluate.tokenize(text))

docs = []
for cat in ("data", "documents", "writing"):
    for md in sorted((GS / "corpus" / cat).glob("*.md")):
        text = md.read_text(encoding="utf-8")
        fm = re.search(r"^---\n(.*?)\n---", text, re.S).group(1)
        name = re.search(r"^name:\s*(.+)$", fm, re.M).group(1).strip()
        enriched = json.loads((GS / "corpus_enriched" / cat / (md.stem + ".json")).read_text(encoding="utf-8"))
        docs.append({"id": md.stem, "name": name, "index_text": evaluate.enriched_index_text(name, enriched)})
queries = json.load(open(GS / "queries.json", encoding="utf-8"))["queries"]
vectors = evaluate.embed([d["index_text"] for d in docs] + [x["query"] for x in queries], allow_api=False)

for stmt in ("DROP DATABASE IF EXISTS skillhub_f1", "CREATE DATABASE skillhub_f1"):
    subprocess.run(["docker", "exec", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", "postgres", "-At", "-c", stmt], check=True, capture_output=True)
psql("""CREATE EXTENSION IF NOT EXISTS vector; CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE search_docs (id text primary key, name text not null, bigram tsvector not null, embedding vector(1536));""")
psql("INSERT INTO search_docs (id,name,bigram,embedding) VALUES " + ",".join(
    f"('{q(d['id'])}','{q(d['name'])}',to_tsvector('simple','{q(toks(d['index_text']))}'),'[" + ",".join(f"{x:.6f}" for x in vectors[d["index_text"]]) + "]')" for d in docs) + ";")

def vec_ranked(query):
    vec = "[" + ",".join(f"{x:.6f}" for x in vectors[query]) + "]"
    return [(l.split("|")[0], float(l.split("|")[1])) for l in psql(f"SELECT id, embedding <=> '{vec}'::vector FROM search_docs ORDER BY 2 ASC LIMIT 31;")]
def bigram_ranked(query):
    t = " | ".join(dict.fromkeys(evaluate.tokenize(query)))  # OR of the query's tokens
    if not t.strip():
        return []
    return [(l.split("|")[0], float(l.split("|")[1])) for l in psql(f"SELECT id, ts_rank_cd(bigram, to_tsquery('simple','{q(t)}')) FROM search_docs WHERE bigram @@ to_tsquery('simple','{q(t)}') ORDER BY 2 DESC LIMIT 20;")]
def name_ranked(query):
    return [(l.split("|")[0], float(l.split("|")[1])) for l in psql(f"SELECT id, word_similarity(name, '{q(query)}') FROM search_docs ORDER BY 2 DESC LIMIT 5;")]

V = {x["query"]: vec_ranked(x["query"]) for x in queries}
B = {x["query"]: bigram_ranked(x["query"]) for x in queries}
N = {x["query"]: name_ranked(x["query"]) for x in queries}

def f1(got, rel):
    hit = len([g for g in got if g in rel]); p = hit / len(got) if got else 0.0; r = hit / len(rel)
    return (2 * p * r / (p + r) if p + r else 0.0), p, r
def score(name, pick, k):
    fs = ps = rs = 0.0; tasks = rejected = distractors = top1 = 0; zh = zhn = en = enn = 0.0
    for x in queries:
        rel = set(x["gold_primary"]) | set(x["gold_acceptable"]); got = pick(x["query"])[:k]
        if not rel:
            distractors += 1; rejected += (0 if got else 1); continue
        tasks += 1; f, p, r = f1(got, rel); fs += f; ps += p; rs += r; top1 += 1 if got and got[0] in rel else 0
        if x["lang"] == "zh": zh += f; zhn += 1
        else: en += f; enn += 1
    return dict(name=name, k=k, F1=fs / tasks, P=ps / tasks, R=rs / tasks, top1=top1, tasks=tasks, rejected=rejected, distractors=distractors, zh=zh / zhn, en=en / enn)
def fmt(s): return f"{s['name']:<44} k={s['k']}: F1={s['F1']:.3f} P={s['P']:.3f} R={s['R']:.3f} zh={s['zh']:.3f} en={s['en']:.3f} top1={s['top1']}/{s['tasks']} rejected={s['rejected']}/{s['distractors']}"

rows = []
for k in (1, 2, 3):
    rows.append(score("V vector cut0.55", lambda qq: [i for i, d in V[qq] if d <= 0.55], k))
    rows.append(score("V vector cut0.75", lambda qq: [i for i, d in V[qq] if d <= 0.75], k))
    for cut in (0.0, 0.05, 0.1):
        rows.append(score(f"B bigram rank>={cut}", lambda qq, c=cut: [i for i, s in B[qq] if s >= c], k))
    rows.append(score("N name>=0.5", lambda qq: [i for i, s in N[qq] if s >= 0.5], k))
    # rrf
    for vc in (0.55, 0.65, 0.75):
        def rrf(qq, vc=vc):
            sc = {}
            for r, (i, d) in enumerate([p for p in V[qq] if p[1] <= vc], 1): sc[i] = sc.get(i, 0) + 1 / (60 + r)
            for r, (i, s) in enumerate(B[qq], 1): sc[i] = sc.get(i, 0) + 1 / (60 + r)
            return [i for i, _ in sorted(sc.items(), key=lambda kv: -kv[1])]
        rows.append(score(f"rrf V<={vc} + B", rrf, k))
    # boost: vector-first, lexical re-ranks within the vector set
    for vc in (0.55, 0.6, 0.65):
        for beta in (0.05, 0.1, 0.2, 0.3):
            def boost(qq, vc=vc, beta=beta):
                cand = [(i, d) for i, d in V[qq] if d <= vc]
                ls = {i: s for i, s in B[qq]}; lmax = max(ls.values()) if ls else 1.0
                sc = {i: (1 - d) + beta * (ls.get(i, 0.0) / lmax if lmax else 0.0) for i, d in cand}
                return [i for i, _ in sorted(sc.items(), key=lambda kv: -kv[1])]
            rows.append(score(f"boost V<={vc} beta={beta}", boost, k))
    # union with recall guard: the top lexical hit joins if its vector distance is within cut+slack
    for vc in (0.55,):
        for slack in (0.05, 0.1, 0.15):
            def union(qq, vc=vc, slack=slack):
                vd = dict(V[qq]); got = [i for i, d in V[qq] if d <= vc]
                for i, s in B[qq][:1]:
                    if i not in got and vd.get(i, 1.0) <= vc + slack:
                        got.append(i)
                return got
            rows.append(score(f"union V<={vc} + topB within +{slack}", union, k))
    # fallback: lexical only when vector is empty
    rows.append(score("fallback V<=0.55 else topB", lambda qq: [i for i, d in V[qq] if d <= 0.55] or [i for i, s in B[qq][:3]], k))
    # name-first: an exact-ish name hit goes first, then vector
    rows.append(score("name>=0.5 first, then V<=0.55", lambda qq: list(dict.fromkeys([i for i, s in N[qq] if s >= 0.5] + [i for i, d in V[qq] if d <= 0.55])), k))

lines = [fmt(r) for r in sorted(rows, key=lambda r: (-r["F1"], -r["rejected"]))]
text = "\n".join(lines)
print(text[:6000])
if OUT:
    OUT.write_text(text + "\n", encoding="utf-8")
