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
