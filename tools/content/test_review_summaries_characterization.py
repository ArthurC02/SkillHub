#!/usr/bin/env python3
import contextlib
import io
import json
import sys
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import review_summaries as review

SUMMARIES = {"summaries": [
    {"id": "a-1", "skill": "alpha", "summary": "rec alpha"},
    {"id": "b-2", "skill": "beta", "summary": "rec beta"},
    {"id": "c-3", "skill": "gamma", "summary": "rec gamma"},
]}
ONLINE = {name: {"enrichment": {"summary": "on " + name}} for name in ("alpha", "beta", "gamma")}
VECTORS = {
    "on alpha": [1.0, 0.0], "rec alpha": [0.8, 0.6],
    "on beta": [0.0, 1.0], "rec beta": [0.0, 1.0],
    "on gamma": [0.6, 0.8], "rec gamma": [0.0, 1.0],
}
VERDICTS = {"alpha": "pass", "beta": "needs-fix"}


def fake_review(row, online, key, mechanical_only):
    if row["skill"] not in VERDICTS:
        raise RuntimeError("judge unavailable for " + row["skill"])
    return {"id": row["id"], "skill": row["skill"], "verdict": VERDICTS[row["skill"]],
            "key": key, "mechanical": mechanical_only}


def fake_embed(texts, key):
    review.USAGE["embed"] += len(texts)
    return [VECTORS[t] for t in texts]


def run(folder, prior=None, online=ONLINE, **flags):
    summaries, out = Path(folder) / "summaries.json", Path(folder) / "review-results.json"
    summaries.write_text(json.dumps(SUMMARIES), encoding="utf-8")
    if prior is not None:
        out.write_text(json.dumps(prior), encoding="utf-8")
    args = SimpleNamespace(api="http://api", only=None, mechanical_only=False, kpi6_only=False,
                           workers=1)
    for name, value in flags.items():
        setattr(args, name, value)
    err = io.StringIO()
    with mock.patch.object(review, "SUMMARIES", summaries), mock.patch.object(review, "OUT", out), \
            mock.patch.object(review, "load_key", lambda: "k"), \
            mock.patch.object(review, "fetch_online", lambda api: online), \
            mock.patch.object(review, "review_one", fake_review), \
            mock.patch.object(review, "embed", fake_embed), \
            mock.patch.dict(review.USAGE, {"in": 0, "out": 0, "embed": 0, "calls": 0}), \
            contextlib.redirect_stderr(err):
        code = review.run(args)
    return code, json.loads(out.read_text(encoding="utf-8")), err.getvalue()


def test_a_mechanical_review_records_every_verdict_and_no_drift():
    with tempfile.TemporaryDirectory() as folder:
        code, payload, err = run(folder, mechanical_only=True)
    assert code == 0
    assert payload["results"] == [
        {"id": "a-1", "skill": "alpha", "verdict": "pass", "key": "", "mechanical": True},
        {"id": "b-2", "skill": "beta", "verdict": "needs-fix", "key": "", "mechanical": True},
        {"id": "c-3", "skill": "gamma", "verdict": "error", "reasons": ["judge unavailable for gamma"]},
    ], payload["results"]
    assert payload["counts"] == {"pass": 1, "needs-fix": 1, "error": 1}
    assert payload["drift_calibration"] == {}
    assert payload["usage"] == {"in": 0, "out": 0, "embed": 0, "calls": 0, "cost_usd": 0.0}
    assert err.splitlines()[:4] == [
        "fetching online catalogue from http://api ...",
        "[1/3] alpha                        pass",
        "[2/3] beta                         needs-fix",
        "[3/3] gamma                        error",
    ], err
    assert err.splitlines()[4:6] == ["", "wrote " + str(Path(folder) / "review-results.json")]


def test_a_full_review_flags_drift_and_notes_it_only_on_a_pass():
    with tempfile.TemporaryDirectory() as folder:
        code, payload, err = run(folder)
    assert code == 0
    alpha, beta, gamma = payload["results"]
    assert (alpha["kpi6_online_cosine"], alpha["kpi6_flagged"]) == (0.8, True)
    assert alpha["notes"] == ["KPI6 線上一致性：與 summaries.json 餘弦 0.8 < 0.9，審核對象為線上文字（本筆判定即對線上版本生效）"]
    assert (beta["kpi6_online_cosine"], beta["kpi6_flagged"], "notes" in beta) == (1.0, False, False)
    assert (gamma["kpi6_online_cosine"], gamma["kpi6_flagged"], "notes" in gamma) == (0.8, True, False)
    assert alpha["key"] == "k"
    assert payload["drift_calibration"] == {"max_cross_skill_cosine": 0.8, "pair": ["beta", "gamma"]}
    assert payload["usage"]["embed"] == 6 and payload["usage"]["cost_usd"] == 0.0
    assert "embedding online vs recorded summaries ..." in err.splitlines()


def test_a_cosine_exactly_at_the_floor_is_not_flagged():
    vectors = dict(VECTORS, **{"rec alpha": [0.9, 0.4358898943540674]})
    with tempfile.TemporaryDirectory() as folder, mock.patch.dict(VECTORS, vectors):
        _, payload, _ = run(folder)
    alpha = payload["results"][0]
    assert (alpha["kpi6_online_cosine"], alpha["kpi6_flagged"], "notes" in alpha) == (0.9, False, False)


def test_a_partial_rerun_merges_into_the_prior_results_and_carries_usage():
    prior = {"usage": {"in": 1000000, "out": 0, "embed": 5, "calls": 3, "cost_usd": 9},
             "results": [{"id": "c-3", "skill": "gamma", "verdict": "pass"},
                         {"id": "a-1", "skill": "alpha", "verdict": "needs-fix"}]}
    with tempfile.TemporaryDirectory() as folder:
        _, payload, _ = run(folder, prior=prior, only="a-,zz")
    assert [(r["id"], r["verdict"]) for r in payload["results"]] == [("a-1", "pass"), ("c-3", "pass")]
    assert payload["results"][0]["kpi6_flagged"] is True and "kpi6_flagged" not in payload["results"][1]
    assert payload["usage"] == {"in": 1000000, "out": 0, "embed": 7, "calls": 3, "cost_usd": 2.0}
    assert payload["counts"] == {"pass": 2, "needs-fix": 0, "error": 0}


def test_kpi6_only_keeps_prior_verdicts_and_refreshes_cosines():
    prior = {"results": [{"id": "b-2", "skill": "beta", "verdict": "pass"},
                         {"id": "a-1", "skill": "alpha", "verdict": "pass"},
                         {"id": "c-3", "skill": "gamma", "verdict": "needs-fix"}]}
    with tempfile.TemporaryDirectory() as folder:
        _, payload, err = run(folder, prior=prior, kpi6_only=True)
    assert [(r["id"], r["verdict"], r["kpi6_flagged"]) for r in payload["results"]] == [
        ("a-1", "pass", True), ("b-2", "pass", False), ("c-3", "needs-fix", True)]
    assert "notes" in payload["results"][0] and not any(line.startswith("[") for line in err.splitlines())


def test_a_skill_missing_online_stops_before_reviewing_anything():
    online = {name: ONLINE[name] for name in ("alpha", "gamma")}
    with tempfile.TemporaryDirectory() as folder:
        try:
            run(folder, online=online, mechanical_only=True)
        except SystemExit as exc:
            assert str(exc) == "not in the online catalogue: ['beta']", exc
            assert not (Path(folder) / "review-results.json").exists()
        else:
            raise AssertionError("a missing skill was reviewed anyway")


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
