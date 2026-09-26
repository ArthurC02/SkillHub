from types import SimpleNamespace
from unittest.mock import patch

import pytest
from fastapi.testclient import TestClient
from test_enrich import REQUEST as ENRICH_REQUEST
from test_evaluate import IMPROVE_REQUEST, JUDGE_REQUEST

from skillhub_llm import app as app_module
from skillhub_llm import enrich, evaluate, generate, intent
from skillhub_llm.app import app

client = TestClient(app, headers={"Authorization": "Bearer test-service-token"})

CANDIDATES = [{"skill_id": "s1", "name": "docx", "summary": "Creates Word documents."}]
SUGGEST_BODY = {"user_prompt": "把逐字稿整理成決議摘要"}
GENERATE_BODY = {"task_description": "把逐字稿整理成決議摘要的 Skill"}

ENDPOINTS = [
    (
        "/v1/analyze-intent",
        {"query": "CSV", "timeout_seconds": intent.TIMEOUT_SECONDS},
        intent,
        "client",
        "TIMEOUT_SECONDS",
    ),
    ("/embed", {"texts": ["one"]}, app_module, "_client", "EMBED_TIMEOUT_SECONDS"),
    (
        "/match-reasons",
        {"query": "read my invoices", "candidates": CANDIDATES},
        app_module,
        "_client",
        "MATCH_REASONS_TIMEOUT_SECONDS",
    ),
    ("/suggest-criteria", SUGGEST_BODY, app_module, "_client", "SUGGEST_CRITERIA_TIMEOUT_SECONDS"),
    ("/v1/enrich-skill", ENRICH_REQUEST, enrich, "client", "LLM_TIMEOUT_SECONDS"),
    ("/judge-run", JUDGE_REQUEST, evaluate, "client", "LLM_TIMEOUT_SECONDS"),
    ("/suggest-improvements", IMPROVE_REQUEST, evaluate, "client", "LLM_TIMEOUT_SECONDS"),
    ("/v1/generate-skill", GENERATE_BODY, generate, "client", "LLM_TIMEOUT_SECONDS"),
]

IDS = [e[0] for e in ENDPOINTS]


def _recorder(asked: list[float]):
    """A gateway client that records the timeout it was built with.

    What it answers with does not matter: every assertion here is about the
    number handed to the builder, which happens before any parsing.
    """

    parsed = SimpleNamespace(
        data=[],
        usage=None,
        choices=[SimpleNamespace(message=SimpleNamespace(content="{}"), finish_reason="stop")],
    )

    async def _create(*_args, **_kwargs):
        return SimpleNamespace(parse=lambda: parsed, headers={}, **vars(parsed))

    def build(timeout):
        asked.append(timeout)
        return SimpleNamespace(
            embeddings=SimpleNamespace(with_raw_response=SimpleNamespace(create=_create)),
            chat=SimpleNamespace(
                completions=SimpleNamespace(with_raw_response=SimpleNamespace(create=_create))
            ),
        )

    return build


@pytest.mark.parametrize("path, body, module, attr, ceiling_name", ENDPOINTS, ids=IDS)
def test_every_endpoint_lets_a_caller_lower_its_ceiling_and_never_raise_it(
    path: str, body: dict, module, attr: str, ceiling_name: str
):
    """Go owns the deadline, so every endpoint must honour the number Go sends
    when it is the smaller one, and ignore it when it is not.
    """
    ceiling = getattr(module, ceiling_name)
    asked: list[float] = []

    with patch.object(module, attr, _recorder(asked)):
        client.post(path, json={**body, "timeout_seconds": 1})
        client.post(path, json={**body, "timeout_seconds": ceiling})
        client.post(path, json={**body, "timeout_seconds": ceiling + 1})
        client.post(path, json=body)

    assert asked == [1, ceiling, ceiling, ceiling]


@pytest.mark.parametrize("path, body, module, attr, ceiling_name", ENDPOINTS, ids=IDS)
@pytest.mark.parametrize("bad", [0, -1])
def test_every_endpoint_rejects_a_ceiling_of_zero_or_less(
    bad: float, path: str, body: dict, module, attr: str, ceiling_name: str
):
    """`min()` would accept 0 and time the call out before it started."""
    asked: list[float] = []
    with patch.object(module, attr, _recorder(asked)):
        response = client.post(path, json={**body, "timeout_seconds": bad})

    assert response.status_code == 422
    assert asked == []
