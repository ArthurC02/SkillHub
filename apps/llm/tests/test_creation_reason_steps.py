import json
import logging
from unittest.mock import patch

import pytest
import test_creation as base

from skillhub_llm import creation

BODY_EDIT = {
    "criterion": "checkbox list",
    "cause": "table",
    "target": "body",
    "edit": "say checkbox",
}


def review_request(**changes):
    return (
        base.request(
            brief="b",
            brief_confirmed=True,
            draft=base.SKILL,
            draft_validation={"content_hash": "c" * 64, "report": "{}", "blocked": False},
            messages=base.request()["messages"]
            + [{"role": "tool", "content": base.UNMET_EVALUATION}],
            allowed_tools=["validate_draft"],
        )
        | changes
    )


def post(req, results):
    calls = []
    seq = base.stub_seq(results, calls)
    with patch.object(creation, "client", lambda _: seq):
        response = base.client.post("/v1/creation/step", headers=base.HEADERS, json=req)
    return response, calls


def system_of(call):
    return call["messages"][0]["content"]


def test_a_diagnosis_with_no_edits_skips_the_rewrite_and_adds_no_guidance():
    response, calls = post(
        review_request(),
        [{"edits": []}, base.decision(outcome="draft", message="ok", draft=base.SKILL)],
    )
    assert response.status_code == 200
    assert [c["response_format"]["json_schema"]["name"] for c in calls] == [
        "review_diagnosis",
        "creation_decision",
    ]
    diagnosis_system, decision_system = system_of(calls[0]), system_of(calls[1])
    assert diagnosis_system != decision_system
    assert diagnosis_system.endswith(decision_system)
    assert "A trial run of the draft Skill was judged against its acceptance criteria" in (
        diagnosis_system.removesuffix(decision_system)
    )
    assert "A trial run of the draft Skill was judged" not in decision_system
    assert response.json()["usage"]["cost_usd"] == 0.002
    assert response.json()["usage"]["cost_source"] == "gateway"


def test_a_rewrite_that_only_changes_whitespace_keeps_the_draft_and_claims_no_rewrite():
    response, calls = post(
        review_request(),
        [
            {"edits": [BODY_EDIT]},
            {"body": "\n" + base.SKILL["body"] + "  \n", "files": []},
            base.decision(outcome="draft", message="ok", draft=base.SKILL),
        ],
    )
    assert response.status_code == 200
    assert len(calls) == 3
    main_system = system_of(calls[2])
    assert "Edits you decided on for this revision — apply every one" in main_system
    assert "- [body] [checkbox list] table -> say checkbox" in main_system
    assert "already been rewritten" not in main_system
    assert response.json()["draft"]["body"] == base.SKILL["body"]


def test_an_unusable_diagnosis_is_skipped_but_its_call_is_still_billed(caplog):
    with caplog.at_level(logging.WARNING, logger="skillhub_llm.creation"):
        response, calls = post(
            review_request(),
            ["not json", base.decision(outcome="draft", message="ok", draft=base.SKILL)],
        )
    assert response.status_code == 200
    assert len(calls) == 2
    assert "Edits you decided on" not in system_of(calls[1])
    assert response.json()["usage"]["cost_usd"] == 0.002
    assert "creation review diagnosis skipped (ValidationError) session=s1" in caplog.text


def test_a_diagnosis_whose_cost_the_gateway_did_not_report_leaves_the_step_cost_unknown():
    calls = []
    seq = base.stub_seq(
        [{"edits": []}, base.decision(outcome="draft", message="ok", draft=base.SKILL)], calls
    )
    reported = seq.chat.completions.with_raw_response.create

    async def unreported_diagnosis(**kwargs):
        raw = await reported(**kwargs)
        if len(calls) == 1:
            raw.parse().usage = None
        return raw

    seq.chat.completions.with_raw_response.create = unreported_diagnosis
    with patch.object(creation, "client", lambda _: seq):
        response = base.client.post(
            "/v1/creation/step", headers=base.HEADERS, json=review_request()
        )
    assert response.status_code == 200
    assert len(calls) == 2
    assert response.json()["usage"] is None


def test_an_unusable_rewrite_keeps_the_draft_but_bills_the_diagnosis_and_the_rewrite():
    response, calls = post(
        review_request(),
        [
            {"edits": [BODY_EDIT]},
            "not json",
            base.decision(outcome="draft", message="ok", draft=base.SKILL),
        ],
    )
    assert response.status_code == 200
    assert len(calls) == 3
    assert "Edits you decided on" not in system_of(calls[2])
    assert response.json()["draft"]["body"] == base.SKILL["body"]
    assert response.json()["usage"]["cost_usd"] == 0.003


@pytest.mark.parametrize(
    "max_output_tokens,diagnosis_budget",
    [(3999, 3999), (4000, 4000), (4001, 4000)],
    ids=["below-cap", "at-cap", "over-cap"],
)
def test_the_diagnosis_budget_is_the_step_budget_capped_at_4000(
    max_output_tokens, diagnosis_budget
):
    response, calls = post(
        review_request(max_output_tokens=max_output_tokens),
        [{"edits": []}, base.decision(outcome="draft", message="ok", draft=base.SKILL)],
    )
    assert response.status_code == 200
    assert calls[0]["max_tokens"] == diagnosis_budget
    assert calls[1]["max_tokens"] == max_output_tokens


def search(query):
    return {"kind": "search_catalog", "query": query, "queries": None}


@pytest.mark.parametrize(
    "result,status",
    [
        (base.decision(message="x" * 20000), 200),
        (base.decision(message="x" * 20001), 502),
        (base.decision(outcome="confirm_brief", brief="b" * 20000), 200),
        (base.decision(outcome="confirm_brief", brief="b" * 20001), 502),
        (base.decision(outcome="tool_intent", tool_intent=search("q" * 4000)), 200),
        (base.decision(outcome="tool_intent", tool_intent=search("q" * 4001)), 502),
        (base.decision(outcome="confirm_brief", brief="b", acceptance_criteria=["c" * 500]), 200),
        (base.decision(outcome="confirm_brief", brief="b", acceptance_criteria=["c" * 501]), 502),
        (base.decision(outcome="confirm_brief", brief="b", sample_input="s" * 4000), 200),
        (base.decision(outcome="confirm_brief", brief="b", sample_input="s" * 4001), 502),
    ],
    ids=[
        "message-at-cap",
        "message-over-cap",
        "brief-at-cap",
        "brief-over-cap",
        "tool-query-at-cap",
        "tool-query-over-cap",
        "criterion-at-cap",
        "criterion-over-cap",
        "sample-input-at-cap",
        "sample-input-over-cap",
    ],
)
def test_decision_text_is_accepted_at_its_cap_and_refused_one_past_it(result, status):
    response, _ = base.invoke(base.request(allowed_tools=["search_catalog"]), result)
    assert response.status_code == status


@pytest.mark.parametrize(
    "req_changes,decision_changes,message",
    [
        ({}, {"outcome": "tool_intent", "tool_intent": search("x")}, "tool unavailable"),
        (
            {"allowed_tools": ["search_catalog"]},
            {"outcome": "tool_intent", "tool_intent": search("  ")},
            "search query missing",
        ),
        (
            {"allowed_tools": ["fetch_url"]},
            {
                "outcome": "tool_intent",
                "tool_intent": {"kind": "fetch_url", "query": "docs", "queries": None},
            },
            "fetch url missing",
        ),
        (
            {"diagram_understanding": base.diagram_text()},
            {"outcome": "draft", "draft": base.SKILL},
            "confirm diagram first",
        ),
        ({}, {"outcome": "draft", "draft": base.SKILL}, "confirm brief first"),
        (
            {"brief": "agreed", "brief_confirmed": True},
            {"outcome": "draft", "draft": None},
            "draft missing",
        ),
        (
            {"brief": "agreed", "brief_confirmed": True},
            {"outcome": "draft", "draft": base.SKILL},
            "validation unavailable",
        ),
        ({}, {"outcome": "confirm_brief", "brief": " "}, "brief missing"),
    ],
    ids=[
        "tool-unavailable",
        "search-query-missing",
        "fetch-url-missing",
        "confirm-diagram-first",
        "confirm-brief-first",
        "draft-missing",
        "validation-unavailable",
        "brief-missing",
    ],
)
def test_a_guard_rail_refusal_says_its_reason_in_words(req_changes, decision_changes, message):
    response, _ = base.invoke(base.request(**req_changes), base.decision(**decision_changes))
    assert response.status_code == 200
    assert response.json()["message"] == message
    assert response.json()["draft"] is None
    assert response.json()["tool_intent"] is None


def test_an_uploaded_diagram_without_any_description_is_unusable_output():
    response, _ = base.invoke(
        base.request(diagram={"media_type": "image/png", "data": "iVBORw0KGgo="}),
        base.decision(outcome="confirm_diagram_description", message="  "),
    )
    assert response.status_code == 502
    assert response.json()["detail"] == "creation model returned unusable output"


def test_an_unmet_evaluation_outside_review_makes_a_single_call():
    response, calls = base.invoke(
        review_request(draft_validation=None),
        base.decision(outcome="draft", message="ok", draft=base.SKILL),
    )
    assert response.status_code == 200
    assert "Current phase: revise" in system_of(calls[0])
    assert len(calls) == 1


def test_a_confirmed_legacy_diagram_echoed_in_another_json_layout_does_not_reopen_it():
    understanding = base.diagram_text("A -> B")
    response, _ = base.invoke(
        base.request(
            diagram_understanding=understanding,
            diagram_confirmed=True,
            brief="agreed",
            brief_confirmed=True,
            allowed_tools=["validate_draft"],
        ),
        base.decision(
            outcome="draft",
            draft=base.SKILL,
            diagram_understanding=json.dumps(json.loads(understanding), indent=2),
        ),
    )
    assert response.status_code == 200
    assert response.json()["reason"] is None
    assert response.json()["tool_intent"]["kind"] == "validate_draft"
