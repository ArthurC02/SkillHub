"""Tests for judge_regression.py's mirrored Judge behaviours. No model call,
no database, no money — runnable directly or collected by pytest."""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from judge_regression import (  # noqa: E402
    JudgeOutcome,
    MATCH_EXACT,
    MATCH_NORMALIZED,
    MATCH_NOT_CHECKED,
    MIN_NORMALIZED_QUOTE,
    RegressionRun,
    record,
    store,
    verify,
)

EVENT = "11111111-1111-4111-8111-111111111111"


def digest_of(payload) -> dict:
    """The map verify() checks a trace citation against, in build_request's shape."""
    return {EVENT: {"payload": payload}}


def request_of(*, criteria, rubric_items=None, complete=True, truncation=()):
    req = {
        "criteria": [{"id": cid, "text": text, "evidence_excerpt": None}
                     for cid, text in criteria],
        "trace_digest": {"complete": complete},
        "truncation": list(truncation),
    }
    if rubric_items is not None:
        req["rubric"] = {"items": rubric_items}
    return req


def verdict_of(criterion_id, result, refs):
    return {"criterion_results": [{
        "criterion_id": criterion_id, "result": result,
        "reason": "because the evidence says so", "evidence_refs": refs,
    }]}



def test_a_quote_with_a_trailing_structural_fragment_still_resolves():
    """G8: the model's own serialisation leaked `}],` into a correct quote, and two
    correct `failed` verdicts were thrown away by a bare substring search."""
    payload = {"kind": "final", "text": "已完成品牌檢查，報告已寫出。"}
    ref = {"kind": "trace_event", "trace_event_id": EVENT,
           "quote": "已完成品牌檢查，報告已寫出。}],"}
    stored, why = verify(ref, digest_of(payload), [], "")
    assert why == "", why
    assert stored["match"] == MATCH_NORMALIZED, stored


def test_a_precomposed_quote_matches_a_decomposed_source_as_normalized():
    composed_quote = "café au lait on the menu"
    decomposed_source = "the café au lait on the menu today"
    assert composed_quote not in decomposed_source
    stored, why = verify({"kind": "agent_output", "quote": composed_quote}, {}, [], decomposed_source)
    assert why == "", why
    assert stored["match"] == MATCH_NORMALIZED, stored


def test_whitespace_differences_do_not_lose_a_quote():
    ref = {"kind": "agent_output", "quote": "the quarterly figures were restated"}
    stored, why = verify(ref, {}, [], "the quarterly figures\nwere restated in full")
    assert why == "", why
    assert stored["match"] == MATCH_NORMALIZED, stored
    exact, why = verify({"kind": "agent_output", "quote": "were restated in full"},
                        {}, [], "the quarterly figures\nwere restated in full")
    assert why == "" and exact["match"] == MATCH_EXACT, exact


def test_a_short_quote_is_accepted_only_on_an_exact_hit():
    """§4's floor. Normalisation widens matching, and a short string in a widened
    comparison hits by accident."""
    short = "abc def"
    assert len(short) < MIN_NORMALIZED_QUOTE, "the floor no longer excludes anything"
    payload = {"text": "abc def ghi"}
    _, why = verify({"kind": "trace_event", "trace_event_id": EVENT, "quote": "abc\ndef"},
                    digest_of(payload), [], "")
    assert why != "", "a 7-character quote matched under normalisation"
    stored, why = verify({"kind": "trace_event", "trace_event_id": EVENT, "quote": short},
                         digest_of(payload), [], "")
    assert why == "" and stored["match"] == MATCH_EXACT, (stored, why)



def test_a_citation_filed_under_the_wrong_source_is_reattributed_not_refused():
    """The A round's finding: every quote sampled from the 6 `passed` verdicts
    resting on `artifact` citations was verbatim in that run's trace_events. The
    model had read the trace and written the wrong label on it."""
    payload = {"tool": "write_file", "content": "the tone was flattened deliberately"}
    ref = {"kind": "artifact", "artifact_path": "report.md",
           "quote": "the tone was flattened deliberately"}
    stored, why = verify(ref, digest_of(payload), [{"path": "report.md"}], "")
    assert why == "", why
    assert stored["kind"] == "trace_event", stored
    assert stored["trace_event_id"] == EVENT, stored
    assert stored["reattributed_from"] == "artifact", stored
    assert stored["match"] == MATCH_EXACT, stored


def test_a_quote_in_no_verifiable_source_is_still_refused():
    """Reattribution must not become "look harder until something passes"."""
    ref = {"kind": "agent_output", "quote": "a sentence nothing in this run contains"}
    stored, why = verify(ref, digest_of({"text": "unrelated"}), [], "also unrelated")
    assert stored is None and why != "", (stored, why)



def test_an_artifact_citation_reports_that_its_quote_was_checked_against_nothing():
    """`not_checked` is a weaker claim than "we looked and it was absent", and the
    two must not be the same value: artifact bytes are never sent."""
    ref = {"kind": "artifact", "artifact_path": "report.md", "quote": "invented sentence"}
    stored, why = verify(ref, {}, [{"path": "report.md"}], "")
    assert why == "", why
    assert stored["match"] == MATCH_NOT_CHECKED, stored



def _stored_for(evidence_required: bool):
    request = request_of(
        criteria=[("r1", "the rewrite keeps the author's voice")],
        rubric_items=[{"id": "r1", "evidence_required": evidence_required}],
    )
    verdict = verdict_of("r1", "passed", [
        {"kind": "artifact", "artifact_path": "report.md", "quote": "reads like a person"},
    ])
    return store(verdict, request, {}, [{"path": "report.md"}], "")[0]


def test_a_rubric_item_that_demands_a_quote_is_not_answered_by_a_file_listing():
    got = _stored_for(True)
    assert got["result"] == "undetermined", got
    assert got["downgrade"] == "evidence_unverifiable", got
    assert got["model_result"] == "passed", got


def test_outside_evidence_required_an_artifact_citation_still_supports_a_verdict():
    """Not the general rule: "the file is there" is a fact the platform checked."""
    got = _stored_for(False)
    assert got["result"] == "passed", got
    assert got["downgrade"] is None, got


def test_a_verified_quote_satisfies_evidence_required():
    request = request_of(
        criteria=[("r1", "the rewrite keeps the author's voice")],
        rubric_items=[{"id": "r1", "evidence_required": True}],
    )
    verdict = verdict_of("r1", "passed", [
        {"kind": "agent_output", "quote": "reads like a person wrote it"},
    ])
    got = store(verdict, request, {}, [], "the draft now reads like a person wrote it.")[0]
    assert got["result"] == "passed", got


def test_a_passed_verdict_on_an_incomplete_trace_is_downgraded():
    request = request_of(criteria=[("r1", "produces a report")], complete=False)
    verdict = verdict_of("r1", "passed", [{"kind": "agent_output", "quote": "report"}])
    got = store(verdict, request, {}, [], "report")[0]
    assert got["result"] == "undetermined", got
    assert got["downgrade"] == "incomplete_evidence", got


def test_a_verdict_without_citations_is_undetermined():
    request = request_of(criteria=[("r1", "produces a report")])
    for result in ("passed", "failed"):
        got = store(verdict_of("r1", result, []), request, {}, [], "report")[0]
        assert got["result"] == "undetermined", got
        assert got["downgrade"] == "evidence_unverifiable", got


def test_model_uncertainty_is_not_scored_as_a_wrong_answer():
    request = request_of(criteria=[("c1", "Run 的 trace 中出現對指定 Skill 的 skill_activation")])
    request.update(evaluation_id="evaluation", artifacts=[])
    row = dict(skill_name="example", runtime_image="baseline", run_id="run", run_status="succeeded")
    response = dict(model="judge", prompt_version="v1", temperature=0, seed=123,
                    verdict=dict(overall="undetermined", summary="insufficient evidence"))
    results = store(verdict_of("c1", "undetermined", []), request, {}, [], "")
    run = RegressionRun("regression", "start", "", "explicit_run_ids")
    line = record(run, row, request, {"activation": "passed"}, JudgeOutcome(results, response, {}))
    assert line["criteria"][0]["outcome"] == "undetermined", line
    assert line["temperature_requested"] == 0, line
    assert line["seed_requested"] == 123, line


EXCERPT_CUT_DECISIONS = [
    ("at the limit, whole trace_event pass stays passed", 8000, "trace_event", False, "passed"),
    ("at the limit, whole agent_output pass stays passed", 8000, "agent_output", False, "passed"),
    ("at the limit, whole artifact pass stays passed", 8000, "artifact", False, "passed"),
    ("at the limit, batch cut downgrades trace_event pass", 8000, "trace_event", True, "undetermined"),
    ("at the limit, batch cut downgrades agent_output pass", 8000, "agent_output", True, "undetermined"),
    ("at the limit, batch cut downgrades artifact pass", 8000, "artifact", True, "undetermined"),
    ("past the limit, trimmed trace_event pass is downgraded", 8001, "trace_event", False, "undetermined"),
    ("past the limit, agent_output pass is not about the trimmed source", 8001, "agent_output", False, "passed"),
    ("past the limit, artifact pass is not about the trimmed source", 8001, "artifact", False, "passed"),
    ("past the limit with batch cut, trace_event pass is downgraded", 8001, "trace_event", True, "undetermined"),
    ("past the limit with batch cut, agent_output pass is downgraded", 8001, "agent_output", True, "undetermined"),
    ("past the limit with batch cut, artifact pass is downgraded", 8001, "artifact", True, "undetermined"),
]


def stored_result_after_cut(size, kind, model_result, batch_cut):
    payload = {"text": "x" * (size - len(json.dumps({"text": ""})))}
    assert len(json.dumps(payload, ensure_ascii=False)) == size
    cuts = ["trace_digest.entries[].excerpt"] if size > 8000 else []
    if batch_cut:
        cuts.append("trace_digest.entries")
    request = request_of(criteria=[("r1", "produces a report")], truncation=cuts)
    ref = {"kind": kind, "trace_event_id": EVENT, "artifact_path": "report.md"}
    if kind == "agent_output":
        ref["quote"] = "report"
    return store(verdict_of("r1", model_result, [ref]), request, digest_of(payload),
                 [{"path": "report.md"}], "report")[0]


def test_excerpt_cuts_only_downgrade_passes_using_the_trimmed_source():
    assert len(EXCERPT_CUT_DECISIONS) == 12
    for name, size, kind, batch_cut, expected in EXCERPT_CUT_DECISIONS:
        got = stored_result_after_cut(size, kind, "passed", batch_cut)
        assert got["result"] == expected, (name, got)


def test_excerpt_cuts_never_change_a_failed_verdict():
    for _, size, kind, batch_cut, _ in EXCERPT_CUT_DECISIONS:
        got = stored_result_after_cut(size, kind, "failed", batch_cut)
        assert got["result"] == "failed", (size, kind, batch_cut, got)


def test_a_verdict_citing_an_unresolvable_reference_is_downgraded():
    request = request_of(criteria=[("r1", "produces a report")])
    verdict = verdict_of("r1", "passed", [
        {"kind": "trace_event", "trace_event_id": "missing"},
    ])
    got = store(verdict, request, {}, [], "")[0]
    assert got["result"] == "undetermined", got
    assert got["downgrade"] == "evidence_unverifiable", got


def test_an_unrecognised_reference_kind_is_refused():
    stored, why = verify({"kind": "bogus"}, {}, [], "")
    assert stored is None, stored
    assert why == "reference kind 'bogus' is not one this platform can resolve", why


def test_an_artifact_citation_outside_the_manifest_is_refused():
    ref = {"kind": "artifact", "artifact_path": "missing.md", "quote": "nothing to match here"}
    stored, why = verify(ref, {}, [{"path": "report.md"}], "")
    assert stored is None, stored
    assert why == "cited artifact 'missing.md' is not in this run's manifest", why


def main() -> int:
    failed = 0
    for name, fn in sorted(globals().items()):
        if not name.startswith("test_") or not callable(fn):
            continue
        try:
            fn()
        except AssertionError as e:  # noqa: PERF203 - a test runner catches per test
            failed += 1
            print(f"FAIL {name}: {e}")
        else:
            print(f"ok   {name}")
    print(f"\n{failed} failed")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
