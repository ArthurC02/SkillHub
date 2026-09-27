"""Characterization of evaluate.py's two reports over the committed corpus,
with embeddings replaced by an offline hash-derived fake."""

from __future__ import annotations

import contextlib
import hashlib
import io
import sys
from unittest import mock

import evaluate


def fake_embed(texts, allow_api=True):
    return {t: [b / 255 - 0.5 for b in hashlib.sha256(t.encode()).digest()[:8]] for t in texts}


def report(fn, *args, embed=fake_embed):
    out = io.StringIO()
    with mock.patch.object(evaluate, "embed", embed), contextlib.redirect_stdout(out):
        fn(*args)
    return out.getvalue()


def digest(text):
    return hashlib.sha256(text.encode()).hexdigest()


def test_the_frontmatter_report_is_unchanged():
    text = report(evaluate.main, False, "frontmatter")
    assert text.startswith("corpus: "), text[:200]
    assert digest(text) == FRONTMATTER, text


def test_the_enriched_report_is_unchanged():
    text = report(evaluate.main, False, "enriched")
    assert "### v1 兩條 miss 在增強索引下的名次" in text
    assert digest(text) == ENRICHED, text


def test_the_lookup_report_is_unchanged():
    text = report(evaluate.lookup_main, False)
    assert "紅線（不擋 CI，只供人判讀）" in text
    assert digest(text) == LOOKUP, text


def test_the_lookup_report_without_vectors_lists_only_the_query_sets():
    def refuse(texts, allow_api=True):
        raise SystemExit("--no-api but 3 texts are not cached")
    text = report(evaluate.lookup_main, False, embed=refuse)
    assert text.splitlines()[2] == "無法取得向量，只列出查詢集：--no-api but 3 texts are not cached", text
    assert "== " not in text
    assert digest(text) == LOOKUP_OFFLINE, text


def test_a_red_line_passes_exactly_at_its_threshold_and_fails_just_below():
    assert evaluate._red_line("x", 9, 10, 0.90) == "  [PASS] x: 9/10 (90%) (紅線 >= 90%)"
    assert evaluate._red_line("x", 8, 10, 0.90) == "  [FAIL] x: 8/10 (80%) (紅線 >= 90%)"
    assert evaluate._red_line("x", 0, 0, 0.0) == "  [PASS] x: — (紅線 >= 0%)"


FRONTMATTER = "7a021acadc0b7a7289808d98b064f0101f4ef81c454f815e42d37390b2517aa1"
ENRICHED = "c49a5c2a222b0ccbdafcfc7024ddead0ed6b2c67cb388d55709dffbfcbf638bc"
LOOKUP = "f60e374e9899040dee07097c37684182dc934b745b961d09a8b4546667acaa7c"
LOOKUP_OFFLINE = "a828693b057102af0e0d8676dd6917130873c2f5b0310035876c118c0080fc04"


if __name__ == "__main__":
    failed = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"ok   {name}")
            except AssertionError as exc:
                failed += 1
                print(f"FAIL {name}: {exc}")
    sys.exit(1 if failed else 0)
