import json
from unittest.mock import patch

import pytest
from test_creation import HEADERS, SKILL, client, decision, invoke, request, stub_seq

from skillhub_llm import creation

WEIGHTS_QUESTION = "數學與自然各占總成績的多少比例？"
WEIGHTS_GAP = {
    "criterion": "Use the course weights supplied by the author.",
    "cause": "The author supplied scores but not the course weights.",
    "target": "context",
    "edit": WEIGHTS_QUESTION,
}
SCALE_GAP = {
    "criterion": "Report the grade on the requested scale.",
    "cause": "The target grade scale was not supplied.",
    "target": "context",
    "edit": "總成績要用百分制還是其他滿分？",
}
BODY_EDIT = {
    "criterion": "Show each course name.",
    "cause": "The output omitted the course names.",
    "target": "body",
    "edit": "Include the course names beside their scores.",
}


@pytest.mark.parametrize(
    "edits, expected",
    [
        ([WEIGHTS_GAP], "數學與自然各占總成績的多少比例？"),
        ([BODY_EDIT, WEIGHTS_GAP], "數學與自然各占總成績的多少比例？"),
        (
            [WEIGHTS_GAP, SCALE_GAP],
            "數學與自然各占總成績的多少比例？\n總成績要用百分制還是其他滿分？",
        ),
    ],
    ids=["missing_rule", "rule_before_rewrite", "two_missing_rules"],
)
def test_review_returns_missing_context_to_the_author_without_rewriting(edits, expected):
    evaluation = json.dumps(
        {
            "evaluation": {
                "criterion_results": [
                    {"result": "undetermined", "reason": "Course weights were not supplied."}
                ]
            }
        }
    )
    req = request(
        brief="Compute a weighted grade from the author's course scores and weights.",
        brief_confirmed=True,
        draft=SKILL,
        draft_validation={"content_hash": "c" * 64, "blocked": False, "report": "{}"},
        messages=[
            {"role": "user", "content": "請算總成績。"},
            {"role": "assistant", "content": "有哪些科目與成績？"},
            {"role": "user", "content": "數學 80 分，自然 90 分。"},
            {"role": "tool", "content": evaluation},
        ],
    )
    calls = []
    with patch.object(
        creation,
        "client",
        lambda _: stub_seq([{"edits": edits}, decision(message=expected)], calls),
    ):
        response = client.post("/v1/creation/step", headers=HEADERS, json=req)

    assert response.status_code == 200, response.text
    body = response.json()
    assert body["outcome"] == "clarification"
    assert body["message"] == expected
    assert body["draft"] is None
    assert body["tool_intent"] is None
    assert (body["brief"], body["acceptance_criteria"], body["sample_input"]) == ("", [], "")
    assert [c["response_format"]["json_schema"]["name"] for c in calls] == ["review_diagnosis"]
    assert body["usage"]["cost_usd"] == 0.001
    assert body["usage"]["prompt_tokens"] == 10
    assert body["usage"]["completion_tokens"] == 5


def test_compose_may_ask_for_an_essential_rule_after_an_earlier_clarification():
    req = request(
        brief="Compute a weighted course grade.",
        brief_confirmed=True,
        messages=[
            {"role": "user", "content": "請算總成績。"},
            {"role": "assistant", "content": "有哪些科目與成績？"},
            {"role": "user", "content": "數學 80 分，自然 90 分。"},
        ],
    )
    response, calls = invoke(req, decision(message=WEIGHTS_QUESTION))

    assert response.status_code == 200
    assert response.json()["outcome"] == "clarification"
    assert response.json()["message"] == "數學與自然各占總成績的多少比例？"
    assert response.json()["draft"] is None
    assert len(calls) == 1
    prompt = calls[0]["messages"][0]["content"]
    assert "Current phase: compose" in prompt
    assert "at most once" not in prompt
    assert "never ask again" not in prompt
    assert "common positive rate" not in prompt
    assert "A Skill whose run ends in a question has failed every criterion" not in prompt


def test_author_supplied_sources_assets_and_scripts_are_preserved_in_the_draft():
    draft = SKILL | {
        "name": "weighted-grades",
        "description": "Compute course grades using the supplied course weights.",
        "body": "Read references/grading.md and assets/weights.csv, then run scripts/grade.py.",
        "files": [
            {"path": "references/grading.md", "content": "Author's rule: math 40%, science 60%."},
            {"path": "assets/weights.csv", "content": "course,weight\nmath,0.4\nscience,0.6\n"},
            {"path": "scripts/grade.py", "content": "print(80 * 0.4 + 90 * 0.6)\n"},
        ],
    }
    req = request(
        brief="Math is 40% and science is 60%; compute the weighted grade.",
        brief_confirmed=True,
        messages=[{"role": "user", "content": "數學占 40%、自然占 60%，成績是 80、90 分。"}],
    )
    response, calls = invoke(req, decision(outcome="draft", draft=draft))

    assert response.status_code == 200
    assert response.json()["draft"] == draft
    assert len(calls) == 1
