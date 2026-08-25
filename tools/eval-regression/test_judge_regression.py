"""The four ADR-043 behaviours this harness mirrors, each with a test that bites.

Not CI, same standing as the harness itself: no model call, no database, no money.
`python tools/eval-regression/test_judge_regression.py` runs it with nothing
installed; pytest collects it too.

Each test is written against the behaviour ADR-043 names, not against the shape of
the code, so removing the mirrored line is what turns it red - which is how it was
checked (AGENTS.md rule 9).
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from judge_regression import (  # noqa: E402
    MATCH_EXACT,
    MATCH_NORMALIZED,
    MATCH_NOT_CHECKED,
    MIN_NORMALIZED_QUOTE,
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


# --- §4: normalisation, and the floor that bounds it -------------------------


def test_a_quote_with_a_trailing_structural_fragment_still_resolves():
    """G8: the model's own serialisation leaked `}],` into a correct quote, and two
    correct `failed` verdicts were thrown away by a bare substring search."""
    payload = {"kind": "final", "text": "已完成品牌檢查，報告已寫出。"}
    ref = {"kind": "trace_event", "trace_event_id": EVENT,
           "quote": "已完成品牌檢查，報告已寫出。}],"}
    stored, why = verify(ref, digest_of(payload), [], "")
    assert why == "", why
    assert stored["match"] == MATCH_NORMALIZED, stored


def test_whitespace_and_nfc_differences_do_not_lose_a_quote():
    payload = {"text": "the quarterly figures\nwere restated in full"}
    ref = {"kind": "agent_output", "quote": "the quarterly figures were restated"}
    stored, why = verify(ref, {}, [], "the quarterly figures\nwere restated in full")
    assert why == "", why
    assert stored["match"] == MATCH_NORMALIZED, stored
    # And the exact hit is still reported as exact, not swallowed by the widening.
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


# --- §1, §2: the quote is re-verified by content, and reattributed ------------


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


# --- §4: the third state ------------------------------------------------------


def test_an_artifact_citation_reports_that_its_quote_was_checked_against_nothing():
    """`not_checked` is a weaker claim than "we looked and it was absent", and the
    two must not be the same value: artifact bytes are never sent."""
    ref = {"kind": "artifact", "artifact_path": "report.md", "quote": "invented sentence"}
    stored, why = verify(ref, {}, [{"path": "report.md"}], "")
    assert why == "", why
    assert stored["match"] == MATCH_NOT_CHECKED, stored


# --- §3: an artifact citation cannot satisfy `evidence_required` -------------


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
