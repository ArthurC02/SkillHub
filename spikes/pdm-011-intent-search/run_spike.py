"""PDM-011 intent search spike.

Minimal hybrid retrieval over real public SKILL.md samples:
  - BM25 (lexical)
  - TF-IDF cosine (semantic stand-in; see LIMITATIONS in README)
  - RRF fusion of both rankings

Default mode: no Postgres, no external deps, no network. Stdlib only.
  python run_spike.py

v2 mode: replaces the TF-IDF leg with real OpenAI text-embedding-3-small
vectors (needs OPENAI_API_KEY in the repo-root .env; never inlined here).
  python run_spike.py --embedding
"""

from __future__ import annotations

import hashlib
import json
import math
import os
import re
import sys
import urllib.request
from collections import Counter
from pathlib import Path

SAMPLES_DIR = Path(__file__).parent / "samples"
CACHE_PATH = Path(__file__).parent / "embeddings_cache.json"
ENV_PATH = Path(__file__).parents[2] / ".env"

EMBED_MODEL = "text-embedding-3-small"  # PDM-003 proposal; 1536 dims
EMBED_URL = "https://api.openai.com/v1/embeddings"
# text-embedding-3-small caps at 8191 tokens; claude-api.md alone is 72KB.
EMBED_CHAR_LIMIT = 20000

# BM25 knobs and the frontmatter boost. The boost is a cheap stand-in for
# ADR-013's index-time LLM enrichment: curated fields count more than body prose.
K1, B, RRF_K = 1.5, 0.75, 60
FRONTMATTER_WEIGHT = 3

CJK_RE = re.compile(r"[一-鿿]+")
WORD_RE = re.compile(r"[a-z0-9][a-z0-9+.#_-]*")


def tokenize(text: str) -> list[str]:
    """Latin words plus CJK character bigrams (no segmenter dependency)."""
    low = text.lower()
    tokens = WORD_RE.findall(low)
    for run in CJK_RE.findall(low):
        if len(run) == 1:
            tokens.append(run)
        else:
            tokens.extend(run[i : i + 2] for i in range(len(run) - 1))
    return tokens


def parse_skill(path: Path) -> dict:
    raw = path.read_text(encoding="utf-8", errors="replace")
    name, description, body = path.stem, "", raw
    if raw.startswith("---"):
        end = raw.find("\n---", 3)
        if end != -1:
            front, body = raw[3:end], raw[end + 4 :]
            for key in ("name", "description"):
                m = re.search(rf"^{key}:\s*(.*)$", front, re.MULTILINE)
                if not m:
                    continue
                value = m.group(1).strip()
                if value in ("|", "|-", ">", ">-", ""):
                    # block scalar: take the indented lines that follow
                    tail = front[m.end() :].splitlines()
                    lines = []
                    for line in tail:
                        if line.strip() and not line.startswith((" ", "\t")):
                            break
                        lines.append(line.strip())
                    value = " ".join(x for x in lines if x)
                value = value.strip("\"'")
                if key == "name":
                    name = value or name
                else:
                    description = value
    indexed = " ".join([name, description] * FRONTMATTER_WEIGHT + [body])
    return {
        "id": path.stem,
        "name": name,
        "description": description,
        "tokens": tokenize(indexed),
        # two embedding granularities, per spike report section 6.2-2
        "summary": f"{name}: {description}",
        "fulltext": f"{name}: {description}\n\n{body}"[:EMBED_CHAR_LIMIT],
    }


def api_key() -> str:
    """Key comes from the environment or the gitignored repo-root .env. Never a literal."""
    key = os.environ.get("OPENAI_API_KEY")
    if not key and ENV_PATH.exists():
        for line in ENV_PATH.read_text(encoding="utf-8-sig", errors="replace").splitlines():
            name, sep, value = line.partition("=")
            if sep and name.strip() == "OPENAI_API_KEY":
                key = value.strip().strip("\"'")
                break
    if not key:
        sys.exit(
            "--embedding needs OPENAI_API_KEY (environment or repo-root .env). "
            "Run without the flag for the stdlib-only TF-IDF mode."
        )
    return key


def embed(texts: list[str]) -> dict[str, list[float]]:
    """Cached embeddings. Cache holds text hashes and vectors only, never the key."""
    cache = json.loads(CACHE_PATH.read_text(encoding="utf-8")) if CACHE_PATH.exists() else {}
    keys = {t: hashlib.sha256(f"{EMBED_MODEL}\n{t}".encode()).hexdigest() for t in texts}
    todo = sorted({t for t in texts if keys[t] not in cache})
    for i in range(0, len(todo), 16):
        batch = todo[i : i + 16]
        req = urllib.request.Request(
            EMBED_URL,
            data=json.dumps({"model": EMBED_MODEL, "input": batch}).encode(),
            headers={"Authorization": f"Bearer {api_key()}", "Content-Type": "application/json"},
        )
        print(f"  embedding {len(batch)} uncached texts...", file=sys.stderr)
        with urllib.request.urlopen(req, timeout=60) as resp:
            payload = json.load(resp)
        for item in payload["data"]:
            cache[keys[batch[item["index"]]]] = item["embedding"]
    if todo:
        CACHE_PATH.write_text(json.dumps(cache), encoding="utf-8")
    return {t: cache[keys[t]] for t in texts}


def cosine(a: list[float], b: list[float]) -> float:
    # OpenAI embeddings ship L2-normalised, but do not bank on it.
    na = math.sqrt(sum(x * x for x in a)) or 1.0
    nb = math.sqrt(sum(x * x for x in b)) or 1.0
    return sum(x * y for x, y in zip(a, b)) / (na * nb)


class Index:
    def __init__(self, docs: list[dict]) -> None:
        self.docs = docs
        self.tf = [Counter(d["tokens"]) for d in docs]
        self.lengths = [len(d["tokens"]) for d in docs]
        self.avg_len = sum(self.lengths) / len(docs)
        df = Counter()
        for counts in self.tf:
            df.update(counts.keys())
        n = len(docs)
        self.bm25_idf = {t: math.log(1 + (n - c + 0.5) / (c + 0.5)) for t, c in df.items()}
        self.tfidf_idf = {t: math.log(n / c) + 1 for t, c in df.items()}
        self.vectors = [self._tfidf_vector(counts) for counts in self.tf]

    def _tfidf_vector(self, counts: Counter) -> dict[str, float]:
        vec = {t: (1 + math.log(f)) * self.tfidf_idf.get(t, 0.0) for t, f in counts.items()}
        norm = math.sqrt(sum(v * v for v in vec.values())) or 1.0
        return {t: v / norm for t, v in vec.items()}

    def bm25(self, query: str) -> list[float]:
        q = tokenize(query)
        scores = []
        for i, counts in enumerate(self.tf):
            norm = K1 * (1 - B + B * self.lengths[i] / self.avg_len)
            scores.append(
                sum(
                    self.bm25_idf[t] * counts[t] * (K1 + 1) / (counts[t] + norm)
                    for t in q
                    if counts.get(t)
                )
            )
        return scores

    def tfidf(self, query: str) -> list[float]:
        qv = self._tfidf_vector(Counter(tokenize(query)))
        return [sum(w * doc.get(t, 0.0) for t, w in qv.items()) for doc in self.vectors]

    def matched_terms(self, query: str, doc_i: int, top: int = 4) -> list[str]:
        counts = self.tf[doc_i]
        hits = {t: self.bm25_idf[t] for t in set(tokenize(query)) if counts.get(t)}
        # single CJK bigrams read as noise in a reason string; keep the longer ones
        ranked = sorted(hits.items(), key=lambda kv: -kv[1])
        return [t for t, _ in ranked if len(t) > 2][:top] or [t for t, _ in ranked][:top]


def rank(scores: list[float], docs: list[dict]) -> list[str]:
    order = sorted(range(len(scores)), key=lambda i: (-scores[i], docs[i]["id"]))
    return [docs[i]["id"] for i in order]


def rrf(*rankings: list[str]) -> list[str]:
    fused: Counter = Counter()
    for ranking in rankings:
        for pos, doc_id in enumerate(ranking):
            fused[doc_id] += 1 / (RRF_K + pos + 1)
    return [doc_id for doc_id, _ in fused.most_common()]


# 10 golden queries: 5 zh-TW, 5 en. `rewrite` is a hand-written English
# paraphrase standing in for ADR-013 section 2's LLM query rewrite step.
QUERIES = [
    {"id": "Q01", "lang": "zh", "gold": "docx",
     "query": "我要把一份會議記錄整理成有目錄和頁碼的 Word 文件",
     "rewrite": "turn meeting notes into a Word document with a table of contents and page numbers"},
    {"id": "Q02", "lang": "zh", "gold": "pdf",
     "query": "幫我從掃描的檔案裡抽出表格資料，再把好幾份合併成一份",
     "rewrite": "extract tables from scanned PDF files and merge several PDFs into one"},
    {"id": "Q03", "lang": "zh", "gold": "pptx",
     "query": "我想做一份給投資人看的簡報投影片，還要有講者備忘稿",
     "rewrite": "build an investor pitch deck presentation with speaker notes"},
    {"id": "Q04", "lang": "zh", "gold": "internal-comms",
     "query": "幫我寫給全公司的每週進度更新與狀態報告",
     "rewrite": "write a weekly company-wide progress update and status report"},
    {"id": "Q05", "lang": "zh", "gold": "algorithmic-art",
     "query": "我想用程式碼生成可以互動調參數的生成式藝術動畫",
     "rewrite": "generate interactive generative art animation with code and tunable parameters"},
    {"id": "Q06", "lang": "en", "gold": "mcp-builder",
     "query": "build a server that lets an LLM call our internal REST API as tools"},
    {"id": "Q07", "lang": "en", "gold": "claude-api",
     "query": "what does each model cost per token and how do I turn on prompt caching"},
    {"id": "Q08", "lang": "en", "gold": "skill-creator",
     "query": "package my reusable agent instructions and run evals to see if they trigger"},
    {"id": "Q09", "lang": "en", "gold": "frontend-design",
     "query": "design a landing page that does not look like a templated bootstrap default"},
    {"id": "Q10", "lang": "en", "gold": "brand-guidelines",
     "query": "apply our company official colors and typography to this deliverable"},
]

METHODS = ("bm25", "tfidf", "rrf")


def evaluate(index: Index, use_rewrite: bool, dense=None, dense_name: str = "tfidf"):
    """A hit requires a positive retrieval score, not just a lucky tie-break.

    With an all-zero score vector every doc ties and the alphabetically first id
    wins position 1, which would otherwise be scored as a Top-1 hit. Dense
    vectors never degenerate that way, so the same guard passes trivially there.

    `dense` replaces the TF-IDF leg with any scorer(text) -> list[float].
    """
    methods = ("bm25", dense_name, "rrf")
    stats = {m: {"top1": 0, "top3": 0, "n": 0} for m in methods}
    rows = []
    for q in QUERIES:
        text = q.get("rewrite", q["query"]) if use_rewrite else q["query"]
        bm_scores = index.bm25(text)
        tf_scores = dense(text) if dense else index.tfidf(text)
        bm, tf = rank(bm_scores, index.docs), rank(tf_scores, index.docs)
        gold_i = [d["id"] for d in index.docs].index(q["gold"])
        scored = {
            "bm25": bm_scores[gold_i] > 0,
            dense_name: tf_scores[gold_i] > 0,
        }
        scored["rrf"] = scored["bm25"] or scored[dense_name]
        # A leg that matched nothing has no candidate set; its "ranking" is just
        # the alphabetical tie-break. Fusing that noise is a harness artifact, not
        # something a real FTS + pgvector pipeline would do — so drop empty legs.
        legs = [r for r, sc in ((bm, bm_scores), (tf, tf_scores)) if any(s > 0 for s in sc)]
        rankings = {"bm25": bm, dense_name: tf, "rrf": rrf(*legs) if legs else bm}
        row = {"id": q["id"], "lang": q["lang"], "gold": q["gold"]}
        for m, r in rankings.items():
            pos = r.index(q["gold"]) + 1 if scored[m] else None
            stats[m]["n"] += 1
            stats[m]["top1"] += pos == 1
            stats[m]["top3"] += pos is not None and pos <= 3
            row[m] = pos
        row["overlap"] = len(set(bm[:3]) & set(tf[:3])) / 3
        rows.append(row)
    return stats, rows


def reason(index: Index, query: str, doc_i: int) -> str:
    """ADR-013 section 3 template path. No LLM polish available in this environment."""
    terms = index.matched_terms(query, doc_i)
    doc = index.docs[doc_i]
    if not terms:
        return f"{doc['name']}: no direct lexical overlap with the query."
    return (
        f"{doc['name']} matches on {', '.join(terms)} "
        f"— declared purpose: {doc['description'][:110].strip()}..."
    )


def table(title: str, stats: dict) -> str:
    lines = [f"### {title}", "", "| method | Top-1 | Top-3 |", "| --- | --- | --- |"]
    for m, s in stats.items():
        lines.append(
            f"| {m} | {s['top1']}/{s['n']} ({s['top1'] / s['n']:.0%}) "
            f"| {s['top3']}/{s['n']} ({s['top3'] / s['n']:.0%}) |"
        )
    return "\n".join(lines) + "\n"


def split_stats(rows: list[dict], lang: str, methods=METHODS) -> dict:
    subset = [r for r in rows if r["lang"] == lang]
    return {
        m: {
            "n": len(subset),
            "top1": sum(r[m] == 1 for r in subset),
            "top3": sum(r[m] is not None and r[m] <= 3 for r in subset),
        }
        for m in methods
    }


def main() -> None:
    paths = sorted(SAMPLES_DIR.glob("*.md"))
    if not paths:
        sys.exit(f"no samples in {SAMPLES_DIR}; see README.md")
    index = Index([parse_skill(p) for p in paths])
    print(f"corpus: {len(index.docs)} skills, {len(QUERIES)} golden queries\n")

    stats_raw, rows_raw = evaluate(index, use_rewrite=False)
    stats_rw, rows_rw = evaluate(index, use_rewrite=True)

    print(table("A. queries as written (zh stays zh)", stats_raw))
    print(table("A-en. english queries only", split_stats(rows_raw, "en")))
    print(table("A-zh. chinese queries only", split_stats(rows_raw, "zh")))
    print(table("B. with LLM-rewrite stand-in (zh translated to en)", stats_rw))
    print(table("B-zh. chinese queries only, after rewrite", split_stats(rows_rw, "zh")))

    print("### per-query gold rank (1 = perfect)\n")
    print("| query | lang | gold | bm25 | tfidf | rrf | rrf (rewritten) |")
    print("| --- | --- | --- | --- | --- | --- | --- |")
    fmt = lambda v: "miss" if v is None else str(v)
    for r, rw in zip(rows_raw, rows_rw):
        print(
            f"| {r['id']} | {r['lang']} | {r['gold']} | {fmt(r['bm25'])} "
            f"| {fmt(r['tfidf'])} | {fmt(r['rrf'])} | {fmt(rw['rrf'])} |"
        )

    same = sum(r["overlap"] == 1.0 for r in rows_rw)
    print(
        f"\nsignal diversity: bm25 and tfidf return the same top-3 set on "
        f"{same}/{len(rows_rw)} rewritten queries "
        f"(mean overlap {sum(r['overlap'] for r in rows_rw) / len(rows_rw):.0%}). "
        "RRF cannot add value where both legs agree."
    )

    print("\n### match reasons (template path, no LLM polish)\n")
    ids = [d["id"] for d in index.docs]
    for q in QUERIES[:4]:
        text = q.get("rewrite", q["query"])
        top = rrf(rank(index.bm25(text), index.docs), rank(index.tfidf(text), index.docs))[0]
        print(f"- {q['id']} \"{q['query'][:44]}...\"\n  -> {reason(index, text, ids.index(top))}")


def main_embedding() -> None:
    """v2: real text-embedding-3-small vectors replace the TF-IDF leg."""
    paths = sorted(SAMPLES_DIR.glob("*.md"))
    if not paths:
        sys.exit(f"no samples in {SAMPLES_DIR}; see README.md")
    docs = [parse_skill(p) for p in paths]
    index = Index(docs)

    texts = [d["summary"] for d in docs] + [d["fulltext"] for d in docs]
    texts += [q["query"] for q in QUERIES] + [q.get("rewrite", q["query"]) for q in QUERIES]
    vectors = embed(texts)
    dims = len(next(iter(vectors.values())))

    def scorer(field: str):
        doc_vecs = [vectors[d[field]] for d in docs]
        return lambda text: [cosine(vectors[text], dv) for dv in doc_vecs]

    print(
        f"corpus: {len(docs)} skills, {len(QUERIES)} golden queries, "
        f"model {EMBED_MODEL} ({dims} dims)\n"
    )

    results = {}
    for grain in ("summary", "fulltext"):
        dense = scorer(grain)
        for mode, rewrite in (("raw", False), ("rewritten", True)):
            results[grain, mode] = evaluate(index, rewrite, dense=dense, dense_name="emb")

    methods = ("bm25", "emb", "rrf")
    for grain in ("summary", "fulltext"):
        for mode in ("raw", "rewritten"):
            stats, rows = results[grain, mode]
            label = f"index={grain}, queries {mode}"
            print(table(f"{label} — all 10", stats))
            print(table(f"{label} — zh only", split_stats(rows, "zh", methods)))
            print(table(f"{label} — en only", split_stats(rows, "en", methods)))

    print("### per-query gold rank, queries as written (1 = perfect)\n")
    print("| query | lang | gold | bm25 | emb(summary) | emb(fulltext) | rrf(summary) |")
    print("| --- | --- | --- | --- | --- | --- | --- |")
    fmt = lambda v: "miss" if v is None else str(v)
    sum_raw, full_raw = results["summary", "raw"][1], results["fulltext", "raw"][1]
    for rs, rf in zip(sum_raw, full_raw):
        print(
            f"| {rs['id']} | {rs['lang']} | {rs['gold']} | {fmt(rs['bm25'])} "
            f"| {fmt(rs['emb'])} | {fmt(rf['emb'])} | {fmt(rs['rrf'])} |"
        )

    print("\n## key questions\n")

    zh_bm, zh_emb = split_stats(sum_raw, "zh", methods)["bm25"], split_stats(sum_raw, "zh", methods)["emb"]
    zh_emb_full = split_stats(full_raw, "zh", methods)["emb"]
    print(
        f"A. cross-lingual recall, zh queries as written vs english corpus:\n"
        f"   bm25 Top-1 {zh_bm['top1']}/{zh_bm['n']}, Top-3 {zh_bm['top3']}/{zh_bm['n']}; "
        f"embedding(summary) Top-1 {zh_emb['top1']}/{zh_emb['n']}, Top-3 {zh_emb['top3']}/{zh_emb['n']}; "
        f"embedding(fulltext) Top-1 {zh_emb_full['top1']}/{zh_emb_full['n']}, "
        f"Top-3 {zh_emb_full['top3']}/{zh_emb_full['n']}"
    )

    print("\nB. does RRF beat its best single leg?")
    for (grain, mode), (stats, rows) in results.items():
        best = max(stats["bm25"]["top1"], stats["emb"]["top1"])
        delta = stats["rrf"]["top1"] - best
        overlap = sum(r["overlap"] for r in rows) / len(rows)
        print(
            f"   {grain}/{mode}: rrf Top-1 {stats['rrf']['top1']}/{stats['rrf']['n']} vs "
            f"best leg {best}/{stats['rrf']['n']} ({delta:+d}); "
            f"rrf Top-3 {stats['rrf']['top3']}/{stats['rrf']['n']} vs "
            f"best leg {max(stats['bm25']['top3'], stats['emb']['top3'])}/{stats['rrf']['n']}; "
            f"mean top-3 overlap between legs {overlap:.0%}"
        )

    print("\nC. summary index vs fulltext index (long-document dilution, Q06 case):")
    for rs, rf in zip(sum_raw, full_raw):
        if rs["emb"] != rf["emb"]:
            print(f"   {rs['id']} ({rs['gold']}): emb rank {fmt(rf['emb'])} fulltext -> {fmt(rs['emb'])} summary")
    s_sum, s_full = results["summary", "raw"][0], results["fulltext", "raw"][0]
    print(
        f"   overall emb Top-1 {s_full['emb']['top1']}/10 fulltext -> {s_sum['emb']['top1']}/10 summary; "
        f"Top-3 {s_full['emb']['top3']}/10 -> {s_sum['emb']['top3']}/10"
    )


def _selfcheck() -> None:
    """Smallest thing that fails if the retrieval logic breaks."""
    docs = [
        {"id": "a", "name": "pdf", "description": "merge and split pdf files", "tokens": tokenize("pdf merge split pages")},
        {"id": "b", "name": "art", "description": "generative art", "tokens": tokenize("art p5js flow field")},
    ]
    idx = Index(docs)
    assert rank(idx.bm25("merge pdf pages"), docs)[0] == "a"
    assert rank(idx.tfidf("generative flow field"), docs)[0] == "b"
    assert rrf(["a", "b"], ["a", "b"])[0] == "a"
    assert rrf(["b", "a"], ["a", "b"]) in (["a", "b"], ["b", "a"])
    assert tokenize("合併檔案") == ["合併", "併檔", "檔案"]
    assert abs(cosine([3.0, 0.0], [5.0, 0.0]) - 1.0) < 1e-9
    assert abs(cosine([1.0, 0.0], [0.0, 2.0])) < 1e-9
    parsed = parse_skill(SAMPLES_DIR / "pdf.md") if (SAMPLES_DIR / "pdf.md").exists() else None
    if parsed:
        assert parsed["name"] == "pdf" and "PDF" in parsed["description"]
    print("selfcheck ok")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
    elif "--embedding" in sys.argv:
        main_embedding()
    else:
        main()
