import logging
from types import SimpleNamespace
from unittest.mock import patch

import httpx
import openai
import pytest
from test_timeout_ceilings import ENDPOINTS, IDS, client

ECHOED_PROMPT = "the user's private task text"


def _refusing_gateway():
    async def create(**_):
        raise openai.BadRequestError(
            message=f"Error code: 400 - {ECHOED_PROMPT}",
            response=httpx.Response(400, request=httpx.Request("POST", "http://gateway")),
            body={"error": {"message": ECHOED_PROMPT}},
        )

    calls = SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
    return lambda *_: SimpleNamespace(embeddings=calls, chat=SimpleNamespace(completions=calls))


@pytest.mark.parametrize("path, body, module, attr, ceiling", ENDPOINTS, ids=IDS)
def test_a_gateway_refusal_is_logged_without_its_body(
    path: str, body: dict, module, attr: str, ceiling: float, caplog
):
    with caplog.at_level(logging.DEBUG), patch.object(module, attr, _refusing_gateway()):
        response = client.post(path, json=body)

    assert response.status_code == 502
    assert "BadRequestError 400" in caplog.text
    assert ECHOED_PROMPT not in caplog.text


PLATFORM_BUDGET_MESSAGE = "Budget has been exceeded! Current cost: 50.0112, Max budget: 50.0"
KEY_BUDGET_MESSAGE = "Budget has been exceeded! Key=88dc28d0f0 Spend=0.5103 Max Budget=0.5"


def budget_refusal(message: str) -> openai.RateLimitError:
    body = {"message": message, "type": "budget_exceeded", "param": None, "code": "429"}
    return openai.RateLimitError(
        message=f"Error code: 429 - {{'error': {body}}}",
        response=httpx.Response(429, request=httpx.Request("POST", "http://gateway")),
        body=body,
    )


def _gateway_raising(error: Exception):
    async def create(**_):
        raise error

    calls = SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
    return lambda *_: SimpleNamespace(embeddings=calls, chat=SimpleNamespace(completions=calls))


@pytest.mark.parametrize("path, body, module, attr, ceiling", ENDPOINTS, ids=IDS)
def test_a_spent_platform_budget_is_a_503_naming_it(
    path: str, body: dict, module, attr: str, ceiling: float
):
    refusing = _gateway_raising(budget_refusal(PLATFORM_BUDGET_MESSAGE))
    with patch.object(module, attr, refusing):
        response = client.post(path, json=body)

    assert response.status_code == 503
    assert response.json() == {"detail": "model gateway platform budget exhausted"}


@pytest.mark.parametrize("path, body, module, attr, ceiling", ENDPOINTS, ids=IDS)
def test_a_spent_key_budget_stays_a_gateway_error(
    path: str, body: dict, module, attr: str, ceiling: float
):
    with patch.object(module, attr, _gateway_raising(budget_refusal(KEY_BUDGET_MESSAGE))):
        response = client.post(path, json=body)

    assert response.status_code == 502


def test_a_rate_limit_that_is_not_a_budget_stays_a_gateway_error():
    error = openai.RateLimitError(
        message="Error code: 429 - rate limited",
        response=httpx.Response(429, request=httpx.Request("POST", "http://gateway")),
        body={"message": "rate limited", "type": "rate_limit_error"},
    )
    path, body, module, attr, _ = ENDPOINTS[0]
    with patch.object(module, attr, _gateway_raising(error)):
        response = client.post(path, json=body)

    assert response.status_code == 502
