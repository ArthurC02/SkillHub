import json
from unittest.mock import patch

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


def case(name="Date that does not exist", criteria=None):
    return {
        "name": name,
        "prompt": "Price this stay: check in 2026-02-30, one night.",
        "criteria": criteria or ["Names 2026-02-30 as a date that does not exist."],
    }


def invoke_in_turn(req, *results):
    calls = []
    seq = stub_seq(list(results), calls)
    with patch.object(creation, "client", lambda _: seq):
        response = client.post("/v1/creation/step", headers=HEADERS, json=req)
    return response, calls


def test_a_brief_to_confirm_comes_back_with_challenge_cases_written_from_it_alone():
    response, calls = invoke_in_turn(
        request(), json.dumps(PROPOSAL), json.dumps({"cases": [case()]})
    )

    assert response.status_code == 200
    body = response.json()
    assert body["outcome"] == "confirm_brief"
    assert body["challenge_cases"] == [case()]
    assert len(calls) == 2
    challenge = calls[1]
    assert challenge["messages"][0]["content"] == creation.CHALLENGE_INSTRUCTIONS
    shown = challenge["messages"][1]["content"]
    assert BRIEF in shown and CRITERIA[0] in shown and SAMPLE in shown
    assert "Help me check invoices" not in shown
    assert challenge["response_format"]["json_schema"]["name"] == "challenge_cases"
    assert body["usage"]["prompt_tokens"] == 20
    assert body["usage"]["completion_tokens"] == 10
    assert body["usage"]["cost_usd"] == 0.002


def test_challenge_cases_past_a_bound_are_dropped_and_at_most_three_are_kept():
    name_at_limit = "n" * creation.CHALLENGE_NAME_MAX_CHARS
    written = [
        case(name=""),
        case(name=name_at_limit + "n"),
        case(criteria=["c"] * (creation.MAX_CHALLENGE_CRITERIA + 1)),
        case(name=name_at_limit),
        case(name="Different wording", criteria=["c"] * creation.MAX_CHALLENGE_CRITERIA),
        case(name="Cap overflow"),
        case(name="Fourth valid case"),
    ]

    response, _ = invoke_in_turn(request(), json.dumps(PROPOSAL), json.dumps({"cases": written}))

    names = [c["name"] for c in response.json()["challenge_cases"]]
    assert names == [name_at_limit, "Different wording", "Cap overflow"]


def test_an_unusable_challenge_reply_leaves_the_brief_to_confirm_without_cases():
    response, calls = invoke_in_turn(request(), json.dumps(PROPOSAL), "not json")

    assert response.status_code == 200
    body = response.json()
    assert body["outcome"] == "confirm_brief"
    assert body["brief"] == BRIEF
    assert body["challenge_cases"] == []
    assert len(calls) == 2


def test_only_a_brief_to_confirm_asks_for_challenge_cases():
    response, calls = invoke_in_turn(request(), json.dumps(decision()))

    assert response.json()["outcome"] == "clarification"
    assert response.json()["challenge_cases"] == []
    assert len(calls) == 1


def test_a_refused_brief_asks_for_no_challenge_cases():
    unchanged = decision(outcome="confirm_brief", message="Confirm again?", brief="agreed")

    response, calls = invoke_in_turn(
        request(brief="agreed", brief_confirmed=True), json.dumps(unchanged)
    )

    assert response.json()["reason"] == "brief_missing"
    assert response.json()["challenge_cases"] == []
    assert len(calls) == 1


def test_confirmed_challenge_cases_reach_the_author_and_four_are_refused():
    confirmed = request(brief=BRIEF, brief_confirmed=True, challenge_cases=[case()])

    response, calls = invoke_in_turn(confirmed, json.dumps(decision()))

    assert response.status_code == 200
    assert "2026-02-30" in calls[0]["messages"][1]["content"]
    too_many = request(challenge_cases=[case()] * (creation.MAX_CHALLENGE_CASES + 1))
    assert client.post("/v1/creation/step", headers=HEADERS, json=too_many).status_code == 422
