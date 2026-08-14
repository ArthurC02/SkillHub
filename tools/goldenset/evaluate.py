"""M1 golden query set evaluation (ADR-013 / DISC-001).

Runs the 60-query golden set against the pinned SKILL.md corpus in ./corpus and
reports, per category and per language:

  * Top-1 / Top-3 / recall@5 for the vector leg, the BM25 leg and RRF
  * the similarity distribution of distractor queries (gold = "no result")
    against the distribution of real queries' gold documents
  * the cosine cut-off sweep those two distributions imply

Usage
  python evaluate.py --selfcheck   # retrieval + threshold logic, no network
  python evaluate.py               # embeds anything not in the cache, then reports
  python evaluate.py --no-api      # cache only; fails loudly if a text is missing

The embedding key is read from the environment or the gitignored repo-root .env.
It is never written to the cache, the output, or this file.
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

ROOT = Path(__file__).parent
CORPUS_DIR = ROOT / "corpus"
CACHE_PATH = ROOT / "embeddings_cache.json"  # gitignored: hashes + float arrays only
ENV_PATH = ROOT.parents[1] / ".env"

EMBED_MODEL = "text-embedding-3-small"  # PDM-003, 1536 dims
EMBED_URL = "https://api.openai.com/v1/embeddings"
EMBED_CHAR_LIMIT = 20000  # model caps at 8191 tokens

K1, B, RRF_K = 1.5, 0.75, 60
FRONTMATTER_WEIGHT = 3  # BM25-side stand-in for ADR-013 index-time enrichment

# Acceptance targets handed over from ADR-013 -> M1.
TARGET_DISTRACTOR_REJECT = 0.75
TARGET_RECALL_LOSS = 0.05
MAX_REPO_SHARE = 0.20  # a source repo may own at most 20% of a category's queries

CJK_RE = re.compile(r"[一-鿿]+")
WORD_RE = re.compile(r"[a-z0-9][a-z0-9+.#_-]*")


# --------------------------------------------------------------------------- corpus


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


def parse_skill(path: Path, doc_id: str) -> dict:
    raw = path.read_text(encoding="utf-8", errors="replace")
    name, description, body = doc_id, "", raw
    if raw.startswith("---"):
        end = raw.find("\n---", 3)
        if end != -1:
            front, body = raw[3:end], raw[end + 4 :]
            for key in ("name", "description"):
                m = re.search(rf"^{key}:\s*(.*)$", front, re.MULTILINE)
                if not m:
                    continue
                value = m.group(1).strip()
                if value in ("|", "|-", ">", ">-", ""):  # block scalar
                    lines = []
                    for line in front[m.end() :].splitlines():
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
        "id": doc_id,
        "name": name,
        "description": description,
        "tokens": tokenize(indexed),
        "summary": f"{name}: {description}",
        "fulltext": f"{name}: {description}\n\n{body}"[:EMBED_CHAR_LIMIT],
    }


def load_corpus() -> tuple[list[dict], dict]:
    manifest = json.loads((ROOT / "manifest.json").read_text(encoding="utf-8"))
    docs = []
    for entry in manifest["documents"]:
        path = CORPUS_DIR / entry["category"] / f"{entry['id']}.md"
        doc = parse_skill(path, entry["id"])
        doc["category"], doc["repo"] = entry["category"], entry["repo"]
        docs.append(doc)
    return docs, manifest


# --------------------------------------------------------------------------- embeddings


def api_key() -> str:
    key = os.environ.get("OPENAI_API_KEY")
    if not key and ENV_PATH.exists():
        for line in ENV_PATH.read_text(encoding="utf-8-sig", errors="replace").splitlines():
            n, sep, v = line.partition("=")
            if sep and n.strip() == "OPENAI_API_KEY":
                key = v.strip().strip("\"'")
                break
    if not key:
        sys.exit("no embedding credential in the environment or repo-root .env; use --no-api")
    return key


def embed(texts: list[str], allow_api: bool = True) -> dict[str, list[float]]:
    """Cached embeddings. The cache holds text hashes and vectors only."""
    cache = json.loads(CACHE_PATH.read_text(encoding="utf-8")) if CACHE_PATH.exists() else {}
    keys = {t: hashlib.sha256(f"{EMBED_MODEL}\n{t}".encode()).hexdigest() for t in texts}
    todo = sorted({t for t in texts if keys[t] not in cache})
    if todo and not allow_api:
        sys.exit(f"--no-api but {len(todo)} texts are not in {CACHE_PATH.name}; rerun without it")
    for i in range(0, len(todo), 16):
        batch = todo[i : i + 16]
        req = urllib.request.Request(
            EMBED_URL,
            data=json.dumps({"model": EMBED_MODEL, "input": batch}).encode(),
            headers={"Authorization": f"Bearer {api_key()}", "Content-Type": "application/json"},
        )
        print(f"  embedding {len(batch)} uncached texts...", file=sys.stderr)
        with urllib.request.urlopen(req, timeout=90) as resp:
            payload = json.load(resp)
        for item in payload["data"]:
            cache[keys[batch[item["index"]]]] = item["embedding"]
    if todo:
        CACHE_PATH.write_text(json.dumps(cache), encoding="utf-8")
    return {t: cache[keys[t]] for t in texts}


def cosine(a: list[float], b: list[float]) -> float:
    na = math.sqrt(sum(x * x for x in a)) or 1.0
    nb = math.sqrt(sum(x * x for x in b)) or 1.0
    return sum(x * y for x, y in zip(a, b)) / (na * nb)


# --------------------------------------------------------------------------- retrieval


class Bm25:
    def __init__(self, docs: list[dict]) -> None:
        self.tf = [Counter(d["tokens"]) for d in docs]
        self.lengths = [len(d["tokens"]) for d in docs]
        self.avg_len = sum(self.lengths) / len(docs)
        df = Counter()
        for counts in self.tf:
            df.update(counts.keys())
        n = len(docs)
        self.idf = {t: math.log(1 + (n - c + 0.5) / (c + 0.5)) for t, c in df.items()}

    def score(self, query: str) -> list[float]:
        q = tokenize(query)
        out = []
        for i, counts in enumerate(self.tf):
            norm = K1 * (1 - B + B * self.lengths[i] / self.avg_len)
            out.append(
                sum(
                    self.idf[t] * counts[t] * (K1 + 1) / (counts[t] + norm)
                    for t in q
                    if counts.get(t)
                )
            )
        return out


def rank(scores: list[float], ids: list[str]) -> list[str]:
    order = sorted(range(len(scores)), key=lambda i: (-scores[i], ids[i]))
    return [ids[i] for i in order]


def rrf(*rankings: list[str]) -> list[str]:
    fused: Counter = Counter()
    for ranking in rankings:
        for pos, doc_id in enumerate(ranking):
            fused[doc_id] += 1 / (RRF_K + pos + 1)
    return [doc_id for doc_id, _ in fused.most_common()]


def first_hit(ranking: list[str], relevant: set[str]) -> int | None:
    for pos, doc_id in enumerate(ranking, 1):
        if doc_id in relevant:
            return pos
    return None


# --------------------------------------------------------------------------- evaluation


def run(docs: list[dict], queries: list[dict], vectors: dict, grain: str) -> list[dict]:
    ids = [d["id"] for d in docs]
    bm25 = Bm25(docs)
    doc_vecs = [vectors[d[grain]] for d in docs]
    rows = []
    for q in queries:
        sims = [cosine(vectors[q["query"]], dv) for dv in doc_vecs]
        bm_scores = bm25.score(q["query"])
        emb_rank = rank(sims, ids)
        bm_rank = rank(bm_scores, ids)
        # A leg that scored nothing has no candidate set; fusing its alphabetical
        # tie-break is a harness artifact, not what FTS + pgvector would do.
        legs = [r for r, sc in ((bm_rank, bm_scores), (emb_rank, sims)) if any(s > 0 for s in sc)]
        relevant = set(q["gold_primary"]) | set(q["gold_acceptable"])
        sim_of = dict(zip(ids, sims))
        rows.append(
            {
                **{k: q[k] for k in ("id", "category", "lang", "kind")},
                "relevant": relevant,
                "primary": set(q["gold_primary"]),
                "emb": first_hit(emb_rank, relevant) if relevant else None,
                "bm25": first_hit(bm_rank, relevant) if any(s > 0 for s in bm_scores) and relevant else None,
                "rrf": first_hit(rrf(*legs) if legs else emb_rank, relevant) if relevant else None,
                "emb_top1_primary": bool(q["gold_primary"]) and emb_rank[0] in q["gold_primary"],
                "top_sim": max(sims),
                "top_id": emb_rank[0],
                # similarity of the best relevant doc that survives into the top 5:
                # the value a cut-off would have to stay under to keep the hit.
                "gold_sim": max((sim_of[i] for i in emb_rank[:5] if i in relevant), default=None),
            }
        )
    return rows


def metrics(rows: list[dict], method: str) -> dict:
    gold = [r for r in rows if r["relevant"]]
    if not gold:
        return {"n": 0, "top1": 0, "top3": 0, "r5": 0}
    hit = lambda r, k: r[method] is not None and r[method] <= k
    return {
        "n": len(gold),
        "top1": sum(hit(r, 1) for r in gold),
        "top3": sum(hit(r, 3) for r in gold),
        "r5": sum(hit(r, 5) for r in gold),
    }


def pct(part: int, total: int) -> str:
    return f"{part}/{total} ({part / total:.0%})" if total else "—"


def table(title: str, rows: list[dict]) -> str:
    out = [f"### {title}", "", "| 檢索腿 | Top-1 | Top-3 | recall@5 |", "| --- | --- | --- | --- |"]
    for m, label in (("emb", "向量"), ("bm25", "BM25"), ("rrf", "RRF")):
        s = metrics(rows, m)
        out.append(f"| {label} | {pct(s['top1'], s['n'])} | {pct(s['top3'], s['n'])} | {pct(s['r5'], s['n'])} |")
    return "\n".join(out) + "\n"


# --------------------------------------------------------------------------- thresholds


def sweep(rows: list[dict]) -> list[dict]:
    """Cosine cut-off sweep over the two similarity distributions.

    A cut-off `t` drops every result scoring below it. For a distractor that is
    the desired behaviour (empty result set); for a real query it is recall lost.
    """
    distractors = [r for r in rows if not r["relevant"]]
    hits = [r for r in rows if r["relevant"] and r["gold_sim"] is not None]
    cuts = sorted({round(x / 200, 3) for x in range(0, 200)})
    out = []
    for t in cuts:
        rejected = sum(r["top_sim"] < t for r in distractors)
        lost = sum(r["gold_sim"] < t for r in hits)
        out.append(
            {
                "t": t,
                "reject": rejected / len(distractors) if distractors else 0.0,
                "loss": lost / len(hits) if hits else 0.0,
                "n_reject": rejected,
                "n_lost": lost,
            }
        )
    return out


def recommend(curve: list[dict]) -> dict | None:
    """Highest rejection rate that still respects the recall-loss budget."""
    ok = [c for c in curve if c["loss"] <= TARGET_RECALL_LOSS]
    if not ok:
        return None
    best = max(ok, key=lambda c: (c["reject"], c["t"]))
    return best if best["reject"] >= TARGET_DISTRACTOR_REJECT else best


def quantiles(values: list[float]) -> tuple[float, float, float, float, float]:
    v = sorted(values)
    q = lambda p: v[min(len(v) - 1, int(p * (len(v) - 1) + 0.5))]
    return v[0], q(0.25), q(0.5), q(0.75), v[-1]


# --------------------------------------------------------------------------- checks


def check_repo_share(docs: list[dict], queries: list[dict]) -> list[str]:
    """ADR-013: one source repo may own at most 20% of a category's queries."""
    repo_of = {d["id"]: d["repo"] for d in docs}
    per_cat: dict[str, Counter] = {}
    totals: Counter = Counter()
    for q in queries:
        totals[q["category"]] += 1
        for g in q["gold_primary"]:
            per_cat.setdefault(q["category"], Counter())[repo_of[g]] += 1
    lines = []
    for cat, counts in sorted(per_cat.items()):
        cap = int(totals[cat] * MAX_REPO_SHARE)
        for repo, n in counts.most_common():
            flag = "超標" if n > cap else "ok"
            lines.append(f"| {cat} | {repo} | {n}/{totals[cat]} ({n / totals[cat]:.0%}) | {flag} |")
    return lines


def _selfcheck() -> None:
    docs = [
        {"id": "a", "name": "pdf", "description": "merge and split pdf files",
         "tokens": tokenize("pdf merge split pages")},
        {"id": "b", "name": "art", "description": "generative art",
         "tokens": tokenize("art p5js flow field")},
    ]
    ids = [d["id"] for d in docs]
    bm = Bm25(docs)
    assert rank(bm.score("merge pdf pages"), ids)[0] == "a"
    assert rrf(["a", "b"], ["a", "b"])[0] == "a"
    assert tokenize("合併檔案") == ["合併", "併檔", "檔案"]
    assert abs(cosine([3.0, 0.0], [5.0, 0.0]) - 1.0) < 1e-9
    assert abs(cosine([1.0, 0.0], [0.0, 2.0])) < 1e-9
    assert first_hit(["x", "y", "a"], {"a"}) == 3 and first_hit(["x"], {"a"}) is None

    # threshold maths: one distractor at 0.30, two gold hits at 0.50 / 0.55.
    fake = [
        {"relevant": set(), "top_sim": 0.30, "gold_sim": None},
        {"relevant": {"a"}, "top_sim": 0.50, "gold_sim": 0.50},
        {"relevant": {"b"}, "top_sim": 0.55, "gold_sim": 0.55},
    ]
    curve = {c["t"]: c for c in sweep(fake)}
    assert curve[0.25]["reject"] == 0.0 and curve[0.25]["loss"] == 0.0
    assert curve[0.40]["reject"] == 1.0 and curve[0.40]["loss"] == 0.0
    assert curve[0.525]["loss"] == 0.5, "a cut above one gold must be charged as recall loss"
    best = recommend(sweep(fake))
    assert best["reject"] == 1.0 and best["loss"] == 0.0

    # queries.json is internally consistent with the corpus manifest
    docs_real, _ = load_corpus()
    known = {d["id"] for d in docs_real}
    qs = json.loads((ROOT / "queries.json").read_text(encoding="utf-8"))["queries"]
    assert len(qs) == 60, len(qs)
    seen = set()
    for q in qs:
        assert q["id"] not in seen
        seen.add(q["id"])
        assert set(q["gold_primary"]) <= known, q["id"]
        assert set(q["gold_acceptable"]) <= known, q["id"]
        assert (q["kind"] == "distractor") == (not q["gold_primary"]), q["id"]
    for cat in ("documents", "writing", "data"):
        sub = [q for q in qs if q["category"] == cat]
        assert len(sub) == 20, cat
        assert sum(q["lang"] == "zh" for q in sub) == 12, cat
        assert sum(q["kind"] == "distractor" for q in sub) >= 4, cat
    assert not [l for l in check_repo_share(docs_real, qs) if "超標" in l]
    print("selfcheck ok")


# --------------------------------------------------------------------------- main


def main(allow_api: bool) -> None:
    docs, manifest = load_corpus()
    queries = json.loads((ROOT / "queries.json").read_text(encoding="utf-8"))["queries"]

    texts = [d["summary"] for d in docs] + [d["fulltext"] for d in docs]
    texts += [q["query"] for q in queries]
    vectors = embed(texts, allow_api=allow_api)

    print(
        f"corpus: {len(docs)} skills / {len({d['repo'] for d in docs})} repos, "
        f"{len(queries)} queries, model {EMBED_MODEL} "
        f"({len(next(iter(vectors.values())))} dims)\n"
    )

    results = {g: run(docs, queries, vectors, g) for g in ("summary", "fulltext")}
    rows = results["summary"]

    print("## 1. 命中率（索引欄位 = summary，即 ADR-013 §1 索引時增強產出的代理）\n")
    print(table("全部 48 條有正解的查詢", rows))
    for cat in ("documents", "writing", "data"):
        print(table(f"類別 {cat}", [r for r in rows if r["category"] == cat]))
    for lang in ("zh", "en"):
        print(table(f"語言 {lang}", [r for r in rows if r["lang"] == lang]))
    for cat in ("documents", "writing", "data"):
        for lang in ("zh", "en"):
            print(table(f"{cat} / {lang}", [r for r in rows if r["category"] == cat and r["lang"] == lang]))

    print("### 索引粒度對照（向量腿）\n")
    print("| 索引欄位 | Top-1 | Top-3 | recall@5 |")
    print("| --- | --- | --- | --- |")
    for g in ("summary", "fulltext"):
        s = metrics(results[g], "emb")
        print(f"| {g} | {pct(s['top1'], s['n'])} | {pct(s['top3'], s['n'])} | {pct(s['r5'], s['n'])} |")

    print("\n## 2. 相似度分布\n")
    print("| 分布 | n | min | p25 | 中位 | p75 | max |")
    print("| --- | --- | --- | --- | --- | --- | --- |")
    for label, values in (
        ("干擾查詢的最高相似度", [r["top_sim"] for r in rows if not r["relevant"]]),
        ("正常查詢的 gold 相似度", [r["gold_sim"] for r in rows if r["gold_sim"] is not None]),
        ("正常查詢的最高相似度", [r["top_sim"] for r in rows if r["relevant"]]),
    ):
        lo, q1, med, q3, hi = quantiles(values)
        print(f"| {label} | {len(values)} | {lo:.3f} | {q1:.3f} | {med:.3f} | {q3:.3f} | {hi:.3f} |")

    print("\n### 干擾查詢逐條（最高相似度＝若無門檻會被回傳的第一名）\n")
    print("| 查詢 | 類別 | 語言 | 最高相似度 | 誤命中的 Skill |")
    print("| --- | --- | --- | --- | --- |")
    for r in sorted((r for r in rows if not r["relevant"]), key=lambda r: -r["top_sim"]):
        print(f"| {r['id']} | {r['category']} | {r['lang']} | {r['top_sim']:.3f} | {r['top_id']} |")

    print("\n## 3. 門檻 trade-off 曲線\n")
    curve = sweep(rows)
    print("| 餘弦相似度門檻 | 餘弦距離門檻 | 干擾查詢正確回「無結果」 | 正常查詢召回損失 |")
    print("| --- | --- | --- | --- |")
    shown = {c["t"] for c in curve if 0.15 <= c["t"] <= 0.55 and abs(c["t"] * 100 - round(c["t"] * 100)) < 1e-9}
    for c in curve:
        if c["t"] in shown and round(c["t"] * 100) % 2 == 0:
            print(
                f"| {c['t']:.2f} | {1 - c['t']:.2f} | {pct(c['n_reject'], 12)} | "
                f"{pct(c['n_lost'], sum(1 for r in rows if r['gold_sim'] is not None))} |"
            )

    best = recommend(curve)
    print(
        f"\n目標：干擾拒答 >= {TARGET_DISTRACTOR_REJECT:.0%} 且召回損失 <= {TARGET_RECALL_LOSS:.0%}。"
    )
    if best:
        verdict = "達標" if best["reject"] >= TARGET_DISTRACTOR_REJECT else "未達標"
        print(
            f"召回損失預算內能達到的最高拒答率：相似度 {best['t']:.3f}"
            f"（距離 {1 - best['t']:.3f}）→ 拒答 {best['reject']:.0%}、"
            f"召回損失 {best['loss']:.0%}　**{verdict}**"
        )
    for name, need in (("拒答 75%", 0.75), ("拒答 90%", 0.90), ("拒答 100%", 1.0)):
        cand = [c for c in curve if c["reject"] >= need]
        if cand:
            c = min(cand, key=lambda c: c["t"])
            print(f"  {name} 最低門檻：相似度 {c['t']:.3f}（距離 {1 - c['t']:.3f}），召回損失 {c['loss']:.0%}")

    print("\n## 4. 跨 repo 抽樣（ADR-013：單一 repo 至多 20% 題目）\n")
    print("| 類別 | 來源 repo | 題數 | 判定 |")
    print("| --- | --- | --- | --- |")
    for line in check_repo_share(docs, queries):
        print(line)

    print("\n## 5. 未命中的查詢（向量腿 recall@5 miss）\n")
    misses = [r for r in rows if r["relevant"] and (r["emb"] is None or r["emb"] > 5)]
    if not misses:
        print("（無）")
    for r in misses:
        print(f"- {r['id']} ({r['category']}/{r['lang']}) 正解 {sorted(r['relevant'])}，向量腿名次 {r['emb']}")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
    else:
        main(allow_api="--no-api" not in sys.argv)
