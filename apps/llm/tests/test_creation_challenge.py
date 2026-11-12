import json
from unittest.mock import patch

import pytest
from test_creation import HEADERS, client, decision, request, stub_seq

from skillhub_llm import creation

BRIEF = "Price a hotel stay: 100 per night, 10% surcharge on weekends."
CRITERIA = ["The total for two weekday nights is 200."]
SAMPLE = "Price this stay: check in Monday 2026-10-12, two nights."
PROPOSAL = decision(
    outcome="confirm_brief",
    message="Please confirm.",
    brief=BRIEF,
    acceptance_criteria=CRITERIA,
    sample_input=SAMPLE,
)


def case(name="Missing arrival date", criteria=None):
    return {
        "name": name,
        "prompt": "Price my two-night stay.",
        "criteria": criteria or ["Asks for the arrival date without inventing a total."],
    }


def direct_case():
    return {
        "name": "Complete weekday stay",
        "prompt": "Price my stay: arrive Monday 2026-10-12, two nights.",
        "criteria": ["Returns a total of 200 without asking for another fact."],
    }


def modeled(case, interaction="question"):
    return case | {"expected_interaction": interaction}


def invoke_in_turn(req, *results):
    calls = []
    seq = stub_seq(list(results), calls)
    with patch.object(creation, "client", lambda _: seq):
        response = client.post("/v1/creation/step", headers=HEADERS, json=req)
    return response, calls


def test_a_brief_to_confirm_comes_back_with_challenge_cases_written_from_it_alone():
    response, calls = invoke_in_turn(
        request(),
        json.dumps(PROPOSAL),
        json.dumps({"cases": [modeled(case()), modeled(direct_case(), "direct_answer")]}),
    )

    assert response.status_code == 200
    body = response.json()
    assert body["outcome"] == "confirm_brief"
    assert body["challenge_cases"] == [modeled(case()), modeled(direct_case(), "direct_answer")]
    assert len(calls) == 2
    challenge = calls[1]
    assert challenge["messages"][0]["content"] == creation.EVALUATION_CASE_INSTRUCTIONS
    shown = challenge["messages"][1]["content"]
    assert BRIEF in shown and CRITERIA[0] in shown and SAMPLE in shown
    assert "Help me check invoices" not in shown
    assert challenge["response_format"]["json_schema"]["name"] == "challenge_cases"
    assert body["usage"]["prompt_tokens"] == 20
    assert body["usage"]["completion_tokens"] == 10
    assert body["usage"]["cost_usd"] == 0.002


def test_paired_evaluation_cases_keep_question_and_direct_answer_expectations():
    cases = [
        {
            "name": "Missing arrival date",
            "prompt": "Price my two-night stay.",
            "criteria": [
                "Asks for the arrival date because the weekend surcharge depends on it.",
                "Does not invent an arrival date or a final price.",
            ],
        },
        direct_case(),
    ]
    response, calls = invoke_in_turn(
        request(),
        json.dumps(PROPOSAL),
        json.dumps({"cases": [modeled(cases[0]), modeled(cases[1], "direct_answer")]}),
    )

    assert response.status_code == 200
    assert response.json()["challenge_cases"] == [
        modeled(cases[0]),
        modeled(cases[1], "direct_answer"),
    ]
    assert len(calls) == 2
    instructions = calls[1]["messages"][0]["content"]
    assert "You try to break" not in instructions
    assert "never a question back" not in instructions
    assert "ordinary reading gives" not in instructions
    assert [case["expected_interaction"] for case in response.json()["challenge_cases"]] == [
        "question",
        "direct_answer",
    ]


def test_challenge_cases_past_a_bound_are_dropped_and_at_most_three_are_kept():
    name_at_limit = "n" * creation.CHALLENGE_NAME_MAX_CHARS
    written = [
        modeled(case(name="")),
        modeled(case(name=name_at_limit + "n")),
        modeled(case(criteria=["c"] * (creation.MAX_CHALLENGE_CRITERIA + 1))),
        modeled(case(name=name_at_limit)),
        modeled(
            case(name="Different wording", criteria=["c"] * creation.MAX_CHALLENGE_CRITERIA)
            | {"prompt": "Price a stay for two nights; arrival date is not stated."}
        ),
        modeled(case(name="Cap overflow")),
        modeled(
            case(name="Another missing date")
            | {"prompt": "Price three nights; I have not decided which date."}
        ),
        modeled(direct_case(), "direct_answer"),
    ]

    response, _ = invoke_in_turn(request(), json.dumps(PROPOSAL), json.dumps({"cases": written}))

    names = [c["name"] for c in response.json()["challenge_cases"]]
    assert names == [name_at_limit, "Complete weekday stay", "Different wording"]


def test_duplicate_optional_inputs_do_not_displace_a_distinct_case():
    extra = case(name="A different missing date") | {
        "prompt": "What is the price of a three-night stay?"
    }
    written = [
        modeled(case()),
        modeled(direct_case(), "direct_answer"),
        modeled(case(name="Same input")),
        modeled(extra),
    ]
    response, _ = invoke_in_turn(request(), json.dumps(PROPOSAL), json.dumps({"cases": written}))

    assert response.status_code == 200
    assert response.json()["challenge_cases"] == [
        modeled(case()),
        modeled(direct_case(), "direct_answer"),
        modeled(extra),
    ]


@pytest.mark.parametrize(
    "interactions", [[], ["question"], ["direct_answer"], ["question"] * 2, ["direct_answer"] * 2]
)
def test_a_missing_side_of_the_evaluation_pair_does_not_return_a_partial_set(interactions):
    written = [
        modeled(case() if kind == "question" else direct_case(), kind) for kind in interactions
    ]
    response, calls = invoke_in_turn(
        request(), json.dumps(PROPOSAL), json.dumps({"cases": written})
    )

    assert response.status_code == 200
    assert response.json()["challenge_cases"] == []
    assert response.json()["usage"]["cost_usd"] == 0.002
    assert len(calls) == 2


@pytest.mark.parametrize(
    "invalid",
    [
        {"name": ""},
        {"name": " "},
        {"name": "n" * (creation.CHALLENGE_NAME_MAX_CHARS + 1)},
        {"prompt": " "},
        {"prompt": "p" * (creation.SAMPLE_INPUT_MAX_CHARS + 1)},
        {"criteria": []},
        {"criteria": [" "]},
        {"criteria": ["c"] * (creation.MAX_CHALLENGE_CRITERIA + 1)},
        {"criteria": ["c" * (creation.CRITERION_MAX_CHARS + 1)]},
    ],
)
def test_an_invalid_required_case_cannot_complete_the_evaluation_pair(invalid):
    written = [modeled(case() | invalid), modeled(direct_case(), "direct_answer")]
    response, _ = invoke_in_turn(request(), json.dumps(PROPOSAL), json.dumps({"cases": written}))

    assert response.status_code == 200
    assert response.json()["challenge_cases"] == []


def test_the_same_input_cannot_be_both_sides_of_the_evaluation_pair():
    repeated = direct_case() | {"prompt": " " + case()["prompt"] + " "}
    response, _ = invoke_in_turn(
        request(),
        json.dumps(PROPOSAL),
        json.dumps({"cases": [modeled(case()), modeled(repeated, "direct_answer")]}),
    )

    assert response.status_code == 200
    assert response.json()["challenge_cases"] == []


@pytest.mark.parametrize("kind", [None, "other"])
def test_an_unclassified_model_case_does_not_pretend_to_complete_the_pair(kind):
    unclassified = case() if kind is None else modeled(case(), kind)
    response, _ = invoke_in_turn(
        request(),
        json.dumps(PROPOSAL),
        json.dumps({"cases": [unclassified, modeled(direct_case(), "direct_answer")]}),
    )

    assert response.status_code == 200
    assert response.json()["challenge_cases"] == []


def test_an_unusable_challenge_reply_leaves_the_brief_to_confirm_without_cases():
    response, calls = invoke_in_turn(request(), json.dumps(PROPOSAL), "not json")

    assert response.status_code == 200
    body = response.json()
    assert body["outcome"] == "confirm_brief"
    assert body["brief"] == BRIEF
    assert body["challenge_cases"] == []
    assert len(calls) == 2
    assert body["usage"]["prompt_tokens"] == 20
    assert body["usage"]["completion_tokens"] == 10
    assert body["usage"]["cost_usd"] == 0.002


def test_only_a_brief_to_confirm_asks_for_challenge_cases():
    response, calls = invoke_in_turn(request(), json.dumps(decision()))

    assert response.json()["outcome"] == "clarification"
    assert response.json()["challenge_cases"] == []
    assert len(calls) == 1


def test_a_brief_go_will_refuse_asks_for_no_challenge_cases():
    unchanged = decision(outcome="confirm_brief", message="Confirm again?", brief="agreed")

    response, calls = invoke_in_turn(
        request(
            brief="agreed",
            brief_confirmed=True,
            challenge_cases=[modeled(case()), modeled(direct_case(), "direct_answer")],
        ),
        json.dumps(unchanged),
    )

    assert response.json()["challenge_cases"] == []
    assert len(calls) == 1
    response, calls = invoke_in_turn(
        request(brief="  "), json.dumps(decision(outcome="confirm_brief", brief=""))
    )
    assert response.json()["challenge_cases"] == []
    assert len(calls) == 1


@pytest.mark.parametrize(
    "legacy_cases", [[], [case()], [case(), case()], [case() | {"criteria": [" "]}, direct_case()]]
)
def test_a_confirmed_legacy_brief_can_acquire_a_pair_without_changing_its_text(legacy_cases):
    proposed = decision(outcome="confirm_brief", brief=BRIEF)
    response, calls = invoke_in_turn(
        request(brief=BRIEF, brief_confirmed=True, challenge_cases=legacy_cases),
        json.dumps(proposed),
        json.dumps({"cases": [modeled(case()), modeled(direct_case(), "direct_answer")]}),
    )

    assert response.status_code == 200
    assert response.json()["challenge_cases"] == [
        modeled(case()),
        modeled(direct_case(), "direct_answer"),
    ]
    assert response.json()["brief"] == BRIEF
    assert len(calls) == 2


def test_confirmed_challenge_cases_reach_the_author_and_four_are_refused():
    confirmed = request(brief=BRIEF, brief_confirmed=True, challenge_cases=[case()])

    response, calls = invoke_in_turn(confirmed, json.dumps(decision()))

    assert response.status_code == 200
    assert "Price my two-night stay." in calls[0]["messages"][1]["content"]
    too_many = request(challenge_cases=[case()] * (creation.MAX_CHALLENGE_CASES + 1))
    assert client.post("/v1/creation/step", headers=HEADERS, json=too_many).status_code == 422


@pytest.mark.parametrize(
    "legacy_cases",
    [
        [case(), direct_case()],
        [modeled(case()), modeled(direct_case())],
        [modeled(case(), "direct_answer"), modeled(direct_case(), "direct_answer")],
        [modeled(case()), direct_case()],
    ],
    ids=["unclassified", "questions_only", "answers_only", "one_missing"],
)
def test_a_legacy_pair_without_both_classifications_is_regenerated(legacy_cases):
    response, calls = invoke_in_turn(
        request(brief=BRIEF, brief_confirmed=True, challenge_cases=legacy_cases),
        json.dumps(decision(outcome="confirm_brief", brief=BRIEF)),
        json.dumps({"cases": [modeled(case()), modeled(direct_case(), "direct_answer")]}),
    )
    assert response.status_code == 200
    assert response.json()["brief"] == BRIEF
    assert response.json()["challenge_cases"] == [
        modeled(case()),
        modeled(direct_case(), "direct_answer"),
    ]
    assert len(calls) == 2


def test_an_unknown_request_classification_is_refused_by_the_internal_contract():
    req = request(
        challenge_cases=[modeled(case(), "other"), modeled(direct_case(), "direct_answer")]
    )
    response, calls = invoke_in_turn(req, json.dumps(decision()))
    assert response.status_code == 422
    assert response.json() == {"detail": "request validation failed"}
    assert calls == []
