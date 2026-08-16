# PDM-011 — Intent Search Quality Spike

Throwaway experiment backing the decision gate on
[ADR-013](../../adr/ADR-013-intent-search-architecture.md). **Not product code.**

Question: can "natural-language intent -> ranked Skill candidates + match reason"
work well enough on a small curated corpus with hybrid retrieval (lexical + dense, RRF fused)?

## Setup

No virtualenv, no dependencies — Python 3.11+ stdlib only (verified on 3.14.0).

```bash
python run_spike.py --selfcheck   # asserts the retrieval logic still works
python run_spike.py               # v1: prints the full result tables
python run_spike.py --embedding   # v2: real embeddings instead of the TF-IDF leg
```

`results.txt` and `results-embedding.txt` hold the committed output of the last run
of each mode.

`--embedding` reads `OPENAI_API_KEY` from the environment or the gitignored
repo-root `.env` and exits with a message if neither has it; the default mode
never touches the network. Vectors are cached in `embeddings_cache.json`
(gitignored, hashes and floats only, no credentials) so re-runs are free.

## What it does

| Stage | Implementation |
| --- | --- |
| Corpus | 12 real `SKILL.md` files from `github.com/anthropics/skills` in `samples/` |
| Lexical | BM25 (k1=1.5, b=0.75), hand-rolled |
| Dense (default) | TF-IDF cosine — **stand-in**, see limitations |
| Dense (`--embedding`) | OpenAI `text-embedding-3-small` (1536 dims), cosine |
| Fusion | Reciprocal Rank Fusion (k=60), legs that matched nothing are dropped |
| Match reason | Template over the highest-IDF matched terms (ADR-013 section 3 default path) |

`--embedding` indexes each Skill at two granularities — `summary`
(`name: description` frontmatter only) and `fulltext` (frontmatter + body,
truncated at 20000 chars for the 8191-token model limit) — to test the spike
report's section 6.2-2 recommendation to retrieve against generated summaries
rather than raw long documents.

Frontmatter (`name` + `description`) is weighted 3x against body prose
(`FRONTMATTER_WEIGHT`) as a cheap stand-in for ADR-013's index-time LLM
enrichment. It is the main tuning knob here.

Chinese text is tokenized as character bigrams so no segmenter dependency is needed.

## Golden queries

10 hand-labelled queries in `run_spike.py` (`QUERIES`), 5 zh-TW + 5 English,
each with exactly one gold Skill. zh queries also carry a hand-written English
`rewrite`, standing in for the LLM query-rewrite step in ADR-013 section 2.

A result only counts as a hit if the gold document scored above zero — an all-zero
score vector ties every document and the alphabetically first id would otherwise
be credited with a Top-1 hit.

## Limitations

- **Default mode uses no real embeddings.** Its dense leg is TF-IDF, which is lexical,
  so v1's conclusions about vector recall and RRF value were unproven. `--embedding`
  closes that gap; the numbers live in `results-embedding.txt`.
- **The `fulltext` granularity is truncated at 20000 chars**, which by itself removes
  most of the length advantage `claude-api.md` (72KB) had over shorter documents.
  The summary-vs-fulltext comparison therefore understates long-document dilution.
- **No real LLM was called.** Index-time enrichment, query rewriting, and match-reason
  polish are all simulated (field weighting, hand-written translations, templates).
- 12 documents and 10 queries. ADR-013 asks for 20+ queries per launch category;
  this is a feasibility probe, not that golden set.
- Single-label ground truth: near-miss ranks are not credited, so absolute
  numbers read pessimistically versus a graded-relevance evaluation.
- Latency (NFR-004 p95 < 2s) was not measured; in-memory scoring over 12 docs
  says nothing about pgvector + FTS at scale.
