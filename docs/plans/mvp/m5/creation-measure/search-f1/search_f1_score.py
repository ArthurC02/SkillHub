"""F1 the way the owner's target is stated ("F1 ≥ 95%"), for both shipped
rules, on the three query sets, against the scratch DB skillhub_f1.

Definition (written down once so the number means the same thing every run):
  * a task query has a relevant set R (gold_primary ∪ gold_acceptable for
    golden; the one document for name and term queries). The page is the
    rule's list cut at k = |R| (a perfect answer is exactly the relevant
    documents), precision = hits/|page|, recall = hits/|R|, F1 per query.
  * a distractor query (golden only, 12) is answered correctly when the rule
    returns nothing in its first 5 (the page says 無結果); F1 = 1 or 0.
  * F1 of a set is the macro mean over its queries; "all" is the macro mean
    over the three sets' 116 queries.
Also printed: F1@1 (the old creation-tool number, capped by two-document
relevant sets) so the two are never confused.

    python search_f1_score.py [--docs <dir>] [--poison] [out.txt]
        --docs: a corpus_enriched-shaped directory to load instead of the
                scratch DB's current rows (rebuilds search_docs from it).
        --poison: insert 3 synthetic documents (poison-1..3, name
                  poison-skill-N) built by concatenating 15-20 golden task
                  queries spanning all three corpus categories, then report,
                  per query set, how often one of them reaches the top-3 / top-1
                  of the vector ranking and whether excluding them from the
                  page changes F1 (05 SEC-013: search results are untrusted
                  input to a model, so a document that can buy itself into a
                  result page is worth knowing about even if the shipped
                  rule's own cut still excludes it).
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
args = [a for a in sys.argv[1:]]
SWEEP = "--sweep" in args
if SWEEP:
    args.remove("--sweep")
POISON = "--poison" in args
if POISON:
    args.remove("--poison")
POISON_DIR = None
if "--poison-dir" in args:
    # the fair LLM04 test: poisoned SKILL.md files that went through the real
    # enrichment (injection/enrich_poison.py); index text = enriched_index_text
    i = args.index("--poison-dir")
    POISON_DIR = Path(args[i + 1]).resolve()
    del args[i:i + 2]
    POISON = True
DOCS_DIR = None
if "--docs" in args:
    i = args.index("--docs")
    DOCS_DIR = Path(args[i + 1]).resolve()
    del args[i:i + 2]
OUT = Path(args[0]).resolve() if args else None
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
    if not texts:
        return []
    out = []
    for i in range(0, len(texts), 32):
        req = urllib.request.Request("http://localhost:8000/embed", data=json.dumps({"texts": texts[i:i + 32]}).encode(), headers={"Content-Type": "application/json", "Authorization": "Bearer " + TOKEN})
        with urllib.request.urlopen(req, timeout=180) as r:
            out += json.load(r)["embeddings"]
    return out


docs = []
for cat in ("data", "documents", "writing"):
    for md in sorted((GS / "corpus" / cat).glob("*.md")):
        fm = re.search(r"^---\n(.*?)\n---", md.read_text(encoding="utf-8"), re.S).group(1)
        name = re.search(r"^name:\s*(.+)$", fm, re.M).group(1).strip()
        src = (DOCS_DIR or (GS / "corpus_enriched")) / cat / (md.stem + ".json")
        enriched = json.loads(src.read_text(encoding="utf-8"))
        text = evaluate.enriched_index_text(name, enriched)
        docs.append({"id": md.stem, "name": name, "index_text": text, "tokens": set(evaluate.tokenize(text)), "cat": cat})

if DOCS_DIR:
    # rebuild the scratch rows from this corpus (embeddings via the service)
    vecs = embed([d["index_text"] for d in docs])
    psql("DROP TABLE IF EXISTS search_docs; CREATE TABLE search_docs (id text primary key, name text not null, embedding vector(1536), bigram tsvector);")
    rows = []
    for d, v in zip(docs, vecs):
        lit = "[" + ",".join(f"{x:.6f}" for x in v) + "]"
        rows.append(f"('{q(d['id'])}','{q(d['name'])}','{lit}'::vector, to_tsvector('simple','{q(' '.join(evaluate.tokenize(d['index_text'])))}'))")
    psql("INSERT INTO search_docs (id,name,embedding,bigram) VALUES " + ",".join(rows) + ";")

name_of = {d["id"]: d["name"].lower() for d in docs}
gold = json.load(open(GS / "queries.json", encoding="utf-8"))["queries"]
count = collections.Counter(t for d in docs for t in d["tokens"])
distinct = []
for d in docs:
    # unique in the Python token set AND under Postgres' own 'simple' parser
    # (which splits hyphenated words: "analytics" also matches
    # high-stakes-analytics), and not a punctuation artefact ("application.").
    for t in sorted(t for t in d["tokens"] if count[t] == 1 and re.fullmatch(r"[a-z][a-z0-9+#_-]{3,}[a-z0-9]", t)):
        n = int(psql(f"SELECT count(*) FROM search_docs WHERE bigram @@ to_tsquery('simple','{q(t)}');")[0])
        if n == 1:
            distinct.append({"query": t, "rel": {d["id"]}, "set": "token"})
            break
distinct = distinct[:25]
sets = ([{"query": x["query"], "rel": set(x["gold_primary"]) | set(x["gold_acceptable"]), "set": "golden", "id": x["id"]} for x in gold]
        + [{"query": d["name"], "rel": {d["id"]}, "set": "name"} for d in docs] + distinct)

poison_ids = set()
if POISON:
    # 15-20 golden task sentences per poison doc, round-robin across the three
    # corpus categories so any contiguous slice still spans all three (a
    # keyword-stuffed adversarial summary, not a real Skill).
    cat_of = {d["id"]: d.get("cat") for d in docs}
    by_cat = collections.defaultdict(list)
    for g in gold:
        for i in set(g["gold_primary"]) | set(g.get("gold_acceptable", [])):
            c = cat_of.get(i)
            if c:
                by_cat[c].append(g["query"])
    cats = ("data", "documents", "writing")
    per_cat = {c: list(dict.fromkeys(by_cat.get(c, []))) for c in cats}
    interleaved, i = [], 0
    while any(len(per_cat[c]) > i for c in cats):
        interleaved += [per_cat[c][i] for c in cats if len(per_cat[c]) > i]
        i += 1
    poison_docs = []
    if POISON_DIR:
        for k, pj in enumerate(sorted(POISON_DIR.glob("*.json"))):
            enriched = json.loads(pj.read_text(encoding="utf-8"))
            text = evaluate.enriched_index_text(pj.stem, enriched)
            poison_docs.append({
                "id": f"poison-{k + 1}", "name": pj.stem,
                "index_text": text, "tokens": set(evaluate.tokenize(text)), "cat": "poison",
            })
    else:
        for k in range(3):
            sentences = (interleaved * 2)[k * 3:k * 3 + 18] or interleaved[:18] or ["generic task"]
            text = " ".join(sentences)
            poison_docs.append({
                "id": f"poison-{k + 1}", "name": f"poison-skill-{k + 1}",
                "index_text": text, "tokens": set(evaluate.tokenize(text)), "cat": "poison",
            })
    docs += poison_docs
    name_of.update({d["id"]: d["name"].lower() for d in poison_docs})
    poison_ids = {d["id"] for d in poison_docs}
    psql("DELETE FROM search_docs WHERE id LIKE 'poison-%';")
    pvecs = embed([d["index_text"] for d in poison_docs])
    prows = []
    for d, v in zip(poison_docs, pvecs):
        lit = "[" + ",".join(f"{x:.6f}" for x in v) + "]"
        prows.append(f"('{q(d['id'])}','{q(d['name'])}','{lit}'::vector, to_tsvector('simple','{q(' '.join(evaluate.tokenize(d['index_text'])))}'))")
    psql("INSERT INTO search_docs (id,name,embedding,bigram) VALUES " + ",".join(prows) + ";")

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


def public_rule(qq, cut=0.75):
    kept = [i for i, d in V[qq] if d <= cut]
    covered = sorted(B[qq], key=lambda i: dist[qq][i])
    page = covered + [i for i in kept if i not in covered]
    pinned = [i for i in page if name_of[i] == qq.strip().lower()]
    return pinned + [i for i in page if i not in pinned]


def creation_rule_2026_09_06(qq, cut=0.55):
    """The creation tool's rule before report §15.4 (vector within 0.55, then one
    covered hit) - kept so the older results files can be re-read."""
    got = [i for i, d in V[qq] if d <= cut]
    return got + [i for i in B[qq][:1] if i not in got]


def creation_variant(qq, cut=0.55, pin=False, covered_first=False, lex_k=1):
    got = [i for i, d in V[qq] if d <= cut]
    lex = [i for i in B[qq][:lex_k]]
    page = (lex + [i for i in got if i not in lex]) if covered_first else (got + [i for i in lex if i not in got])
    if pin:
        pinned = [i for i in page if name_of[i] == qq.strip().lower()]
        page = pinned + [i for i in page if i not in pinned]
    return page


def f1_of(page, rel, k):
    got = page[:k]
    hits = len([g for g in got if g in rel])
    p = hits / len(got) if got else 0.0
    r = hits / len(rel)
    return 2 * p * r / (p + r) if p + r else 0.0


def score(label, fn, cut):
    lines = [f"== {label}"]
    all_f1, all_f1at1 = [], []
    for name in ("golden", "name", "token"):
        sub = [x for x in sets if x["set"] == name]
        f1s, f1at1 = [], []
        for x in sub:
            page = fn(x["query"], cut)
            if x["rel"]:
                f1s.append(f1_of(page, x["rel"], len(x["rel"])))
                f1at1.append(f1_of(page, x["rel"], 1))
            else:
                f1s.append(1.0 if not page[:5] else 0.0)
                f1at1.append(f1s[-1])
        all_f1 += f1s
        all_f1at1 += f1at1
        lines.append(f"  {name:6s} n={len(sub):3d} F1={sum(f1s)/len(f1s):.3f}  (F1@1={sum(f1at1)/len(f1at1):.3f})")
    lines.append(f"  all    n={len(all_f1):3d} F1={sum(all_f1)/len(all_f1):.3f}  (F1@1={sum(all_f1at1)/len(all_f1at1):.3f})")
    return "\n".join(lines), sum(all_f1) / len(all_f1)


def misses(fn, cut):
    out = []
    for x in sets:
        page = fn(x["query"], cut)
        if x["rel"]:
            f = f1_of(page, x["rel"], len(x["rel"]))
            if f < 1.0:
                out.append(f"  [{x['set']}] {x.get('id', '')} {x['query'][:60]!s}: want {sorted(x['rel'])} got {page[:len(x['rel'])]} F1={f:.2f}")
        elif page[:5]:
            out.append(f"  [distractor] {x['id']} {x['query'][:60]}: returned {page[:3]}")
    return out


if SWEEP:
    # the creation rule's shape, swept on the current scratch rows
    print("creation-rule sweep (F1 = k=|rel| macro over golden+name+token; golden alone in brackets)")
    print("public rule at 0.75 (sanity):")
    print(score("public", public_rule, 0.75)[0])
    print(chr(10).join(misses(public_rule, 0.75)))
    for cut in (0.55, 0.6, 0.65, 0.7, 0.75):
        text, total = score("", public_rule, cut)
        rej = sum(1 for x in sets if x["set"] == "golden" and not x["rel"] and not public_rule(x["query"], cut)[:5])
        print(f"public-shape cut={cut:.2f}: all={total:.3f} [{text.split(chr(10))[1].strip()}] rej={rej}/12")
    for cut in (0.55, 0.6, 0.65, 0.7, 0.75):
        for pin in (False, True):
            for cf in (False, True):
                for lex_k in (1, 2):
                    fn = lambda qq, c, pin=pin, cf=cf, lex_k=lex_k: creation_variant(qq, c, pin, cf, lex_k)
                    text, total = score("", fn, cut)
                    golden_line = text.split("\n")[1]
                    rej = sum(1 for x in sets if x["set"] == "golden" and not x["rel"] and not fn(x["query"], cut)[:5])
                    print(f"cut={cut:.2f} pin={int(pin)} covered_first={int(cf)} lex_k={lex_k}: all={total:.3f} [{golden_line.strip()}] rej={rej}/12")
    sys.exit(0)

report = ""
for label, fn, cut in (("public rule = creation search (vector <= 0.75, covered first, name pinned)", public_rule, 0.75), ("duplicate guard (the same shape at 0.55; not a search, no 0.95 target)", public_rule, 0.55), ("creation rule before 2026-09-07 (vector <= 0.55, then one covered)", creation_rule_2026_09_06, 0.55)):
    text, _ = score(label, fn, cut)
    report += text + "\n"
    report += "  misses:\n" + "\n".join(misses(fn, cut)) + "\n\n"

if POISON:
    lines = ["== poison test (05 SEC-013): " + ("enriched poisoned SKILL.md from " + POISON_DIR.name if POISON_DIR else "synthetic poison-1..3 (worst case: index text = the golden sentences)") + " =="]
    for name in ("golden", "name", "token"):
        sub = [x for x in sets if x["set"] == name]
        top3 = sum(1 for x in sub if poison_ids & {i for i, _ in V[x["query"]][:3]})
        top1 = sum(1 for x in sub if V[x["query"]] and V[x["query"]][0][0] in poison_ids)
        with_f1, without_f1 = [], []
        for x in sub:
            page = public_rule(x["query"], 0.75)
            page_np = [i for i in page if i not in poison_ids]
            if x["rel"]:
                k = len(x["rel"])
                with_f1.append(f1_of(page, x["rel"], k))
                without_f1.append(f1_of(page_np, x["rel"], k))
            else:
                with_f1.append(1.0 if not page[:5] else 0.0)
                without_f1.append(1.0 if not page_np[:5] else 0.0)
        changed = sum(1 for a, b in zip(with_f1, without_f1) if abs(a - b) > 1e-9)
        verdict = "RED (poison reached top-3)" if top3 else "ok (never reached top-3)"
        lines.append(
            f"  {name:6s} n={len(sub):3d} top3_hit={top3} top1_hit={top1} "
            f"F1(with poison in page)={sum(with_f1)/len(with_f1):.3f} "
            f"F1(poison excluded)={sum(without_f1)/len(without_f1):.3f} "
            f"changed_queries={changed}/{len(sub)} redline(top3=0)={verdict}"
        )
    lines.append(f"  poison docs: {sorted(poison_ids)}")
    poison_report = "\n".join(lines)
    report += poison_report + "\n"

print(report)
if OUT:
    OUT.write_text(report, encoding="utf-8")
