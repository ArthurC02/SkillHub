from types import SimpleNamespace
from unittest.mock import patch

import pytest
from fastapi.testclient import TestClient
from test_enrich import REQUEST as ENRICH_REQUEST
from test_evaluate import IMPROVE_REQUEST, JUDGE_REQUEST

from skillhub_llm import enrich, evaluate, generate, intent
from skillhub_llm.app import app

client = TestClient(app, headers={"Authorization": "Bearer test-service-token"})


def _no_choices(_timeout):
    completion = SimpleNamespace(
        choices=[], usage=SimpleNamespace(prompt_tokens=7, completion_tokens=0)
    )

    async def create(*_args, **_kwargs):
        return SimpleNamespace(parse=lambda: completion, headers={})

    return SimpleNamespace(
        chat=SimpleNamespace(
            completions=SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
        )
    )


@pytest.mark.parametrize(
    "path, body, module, detail",
    [
        ("/v1/enrich-skill", ENRICH_REQUEST, enrich, "enrichment model returned malformed output"),
        ("/judge-run", JUDGE_REQUEST, evaluate, "judge model returned malformed output"),
        (
            "/suggest-improvements",
            IMPROVE_REQUEST,
            evaluate,
            "suggestion model returned malformed output",
        ),
        (
            "/v1/generate-skill",
            {"task_description": "把逐字稿整理成決議摘要的 Skill"},
            generate,
            "generate model returned malformed output",
        ),
    ],
    ids=["enrich", "judge", "suggest-improvements", "generate"],
)
def test_a_completion_with_no_choices_is_a_malformed_answer(path, body, module, detail):
    with patch.object(module, "client", _no_choices):
        response = client.post(path, json=body)

    assert response.status_code == 502
    assert response.json() == {"detail": detail}


def test_an_intent_completion_with_no_choices_is_an_invalid_proposal_that_still_reports_usage():
    with patch.object(intent, "client", _no_choices):
        response = client.post("/v1/analyze-intent", json={"query": "CSV", "timeout_seconds": 2.0})

    assert response.status_code == 200
    body = response.json()
    assert body["valid"] is False
    assert body["usage"]["prompt_tokens"] == 7
