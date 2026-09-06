"""Replay the product's retrieval legs on the golden query set and score F1@3.

Scratch database skillhub_f1 (same Postgres as dev), table shaped like
search_documents: tsv = product's generated column (english, name A + summary
B), embedding = the enriched index text vector from the goldenset cache
(text-embedding-3-small, the product's model). Three legs, exactly the SQL the
product runs: lexical (PublicSearchSkills), vector (the vec CTE), hybrid
(PublicHybridSearchSkills: vec top-50 UNION fts top-50, ranked by distance,
distance > 0.75 dropped by the Go side).
"""
import json, os, re, subprocess, sys
from pathlib import Path

ROOT = Path(r"C:/Users/a8022/OneDrive/Desktop/SkillHub")
GS = ROOT / "tools" / "goldenset"
sys.path.insert(0, str(GS))
os.chdir(GS)
import evaluate  # noqa: E402  (the frozen tool; read-only use of its functions)

ALLOW_API = os.environ.get("F1_ALLOW_API") == "1"
MAX_DIST = 0.75
K = 3

def psql(sql, db="skillhub_f1"):
    out = subprocess.run(["docker", "exec", "-i", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", db, "-At", "-F", "|", "-v", "ON_ERROR_STOP=1"], input=sql.encode("utf-8"), capture_output=True)
    if out.returncode != 0:
        raise SystemExit(out.stderr.decode("utf-8", "replace"))
    return [l for l in out.stdout.decode("utf-8", "replace").splitlines() if l.strip()]

def q(s):
    return s.replace("'", "''")

# ---- corpus: name + description (package summary) + enriched index text
docs = []
for cat in ("data", "documents", "writing"):
    for md in sorted((GS / "corpus" / cat).glob("*.md")):
        text = md.read_text(encoding="utf-8")
        m = re.search(r"^---\n(.*?)\n---", text, re.S)
        fm = m.group(1) if m else ""
        name = re.search(r"^name:\s*(.+)$", fm, re.M).group(1).strip()
        desc = re.search(r"^description:\s*(.+)$", fm, re.M)
        desc = desc.group(1).strip() if desc else ""
        enriched = json.loads((GS / "corpus_enriched" / cat / (md.stem + ".json")).read_text(encoding="utf-8"))
        docs.append({"id": md.stem, "name": name, "summary": desc, "index_text": evaluate.enriched_index_text(name, enriched)})
queries = json.load(open(GS / "queries.json", encoding="utf-8"))["queries"]

# ---- vectors (cache first; the API only when allowed)
vectors = evaluate.embed([d["index_text"] for d in docs] + [x["query"] for x in queries], allow_api=ALLOW_API)

# ---- scratch database
subprocess.run(["docker", "exec", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", "postgres", "-At", "-c", "DROP DATABASE IF EXISTS skillhub_f1"], check=True, capture_output=True)
subprocess.run(["docker", "exec", "skillhub-postgres-1", "psql", "-U", "skillhub", "-d", "postgres", "-At", "-c", "CREATE DATABASE skillhub_f1"], check=True, capture_output=True)
psql("""CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE search_docs (
  id text primary key, name text not null, summary text not null,
  tsv tsvector GENERATED ALWAYS AS (setweight(to_tsvector('english', coalesce(name, '')), 'A') || setweight(to_tsvector('english', coalesce(summary, '')), 'B')) STORED,
  embedding vector(1536));""")
rows = []
for d in docs:
    vec = "[" + ",".join(f"{x:.6f}" for x in vectors[d["index_text"]]) + "]"
    rows.append(f"('{q(d['id'])}','{q(d['name'])}','{q(d['summary'])}','{vec}')")
psql("INSERT INTO search_docs (id,name,summary,embedding) VALUES " + ",".join(rows) + ";")

def lexical(query):
    return psql(f"SELECT id FROM search_docs WHERE tsv @@ websearch_to_tsquery('english','{q(query)}') ORDER BY ts_rank_cd(tsv, websearch_to_tsquery('english','{q(query)}')) DESC LIMIT {K};")

def vector(query):
    vec = "[" + ",".join(f"{x:.6f}" for x in vectors[query]) + "]"
    return psql(f"SELECT id FROM search_docs WHERE embedding IS NOT NULL AND embedding <=> '{vec}'::vector <= {MAX_DIST} ORDER BY embedding <=> '{vec}'::vector ASC LIMIT {K};")

def hybrid(query):
    vec = "[" + ",".join(f"{x:.6f}" for x in vectors[query]) + "]"
    return psql(f"""WITH vec AS (SELECT id, embedding <=> '{vec}'::vector AS distance FROM search_docs WHERE embedding IS NOT NULL ORDER BY embedding <=> '{vec}'::vector ASC LIMIT 50),
fts AS (SELECT id, embedding <=> '{vec}'::vector AS distance FROM search_docs WHERE tsv @@ websearch_to_tsquery('english','{q(query)}') ORDER BY ts_rank_cd(tsv, websearch_to_tsquery('english','{q(query)}')) DESC LIMIT 50),
c AS (SELECT id, distance FROM vec UNION SELECT id, distance FROM fts)
SELECT id FROM c WHERE distance IS NULL OR distance <= {MAX_DIST} ORDER BY distance ASC NULLS LAST LIMIT {K};""")

def score(leg):
    tp = fp = fn = 0
    p_sum = r_sum = f_sum = 0.0
    top1 = 0
    tasks = 0
    rejected = distractors = 0
    for x in queries:
        got = leg(x["query"])
        rel = set(x["gold_primary"]) | set(x["gold_acceptable"])
        if not rel:
            distractors += 1
            rejected += 1 if not got else 0
            continue
        tasks += 1
        hit = len([g for g in got if g in rel])
        p = hit / len(got) if got else 0.0
        r = hit / len(rel)
        f = 2 * p * r / (p + r) if p + r else 0.0
        p_sum += p; r_sum += r; f_sum += f
        top1 += 1 if got and got[0] in rel else 0
        tp += hit; fp += len(got) - hit; fn += len(rel) - hit
    micro_p = tp / (tp + fp) if tp + fp else 0.0
    micro_r = tp / (tp + fn) if tp + fn else 0.0
    micro_f = 2 * micro_p * micro_r / (micro_p + micro_r) if micro_p + micro_r else 0.0
    return dict(tasks=tasks, top1=top1, P=p_sum / tasks, R=r_sum / tasks, F1=f_sum / tasks, microF1=micro_f, distractors=distractors, rejected=rejected)

print(f"docs={len(docs)} queries={len(queries)} k={K} max_distance={MAX_DIST}")
for name, leg in (("lexical (search_catalog today)", lexical), ("vector only", vector), ("hybrid (product SQL)", hybrid)):
    s = score(leg)
    print(f"{name}: tasks={s['tasks']} top1={s['top1']}/{s['tasks']} P@3={s['P']:.3f} R@3={s['R']:.3f} F1@3={s['F1']:.3f} microF1={s['microF1']:.3f} distractors_rejected={s['rejected']}/{s['distractors']}")
for lang in ("zh", "en"):
    sub = [x for x in queries if x["lang"] == lang and (x["gold_primary"] or x["gold_acceptable"])]
    for name, leg in (("lexical", lexical), ("vector", vector), ("hybrid", hybrid)):
        f = 0.0
        for x in sub:
            got = leg(x["query"]); rel = set(x["gold_primary"]) | set(x["gold_acceptable"])
            hit = len([g for g in got if g in rel]); p = hit / len(got) if got else 0.0; r = hit / len(rel)
            f += 2 * p * r / (p + r) if p + r else 0.0
        print(f"  {lang} ({len(sub)} task queries) {name}: F1@3={f / len(sub):.3f}")

# ---- sweep: k and distance cutoff for the creation tool (vector leg)
print("\nsweep (vector leg, task queries): k x max_distance -> F1@k / P / R / distractors rejected")
def vec_sorted(query):
    vec = "[" + ",".join(f"{x:.6f}" for x in vectors[query]) + "]"
    return [(l.split("|")[0], float(l.split("|")[1])) for l in psql(f"SELECT id, (embedding <=> '{vec}'::vector) FROM search_docs ORDER BY 2 ASC LIMIT 10;")]
ranked = {x["query"]: vec_sorted(x["query"]) for x in queries}
best = None
for k in (1, 2, 3):
    for cut in (0.45, 0.5, 0.55, 0.6, 0.65, 0.7, 0.75):
        f_sum = p_sum = r_sum = 0.0; tasks = 0; rejected = 0; distractors = 0
        for x in queries:
            rel = set(x["gold_primary"]) | set(x["gold_acceptable"])
            got = [i for i, d in ranked[x["query"]] if d <= cut][:k]
            if not rel:
                distractors += 1; rejected += 1 if not got else 0; continue
            tasks += 1
            hit = len([g for g in got if g in rel]); p = hit / len(got) if got else 0.0; r = hit / len(rel)
            f = 2 * p * r / (p + r) if p + r else 0.0
            f_sum += f; p_sum += p; r_sum += r
        row = (f_sum / tasks, k, cut, p_sum / tasks, r_sum / tasks, rejected, distractors)
        print(f"  k={k} cut={cut:.2f}: F1={row[0]:.3f} P={row[3]:.3f} R={row[4]:.3f} rejected={rejected}/{distractors}")
        if best is None or row[0] > best[0]: best = row
print(f"best: F1={best[0]:.3f} at k={best[1]} cut={best[2]} (P={best[3]:.3f} R={best[4]:.3f} rejected={best[5]}/{best[6]})")
