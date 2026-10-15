import asyncio
import json
from types import SimpleNamespace
from unittest.mock import patch

import pytest
from test_timeout_ceilings import ENDPOINTS, IDS

from skillhub_llm.app import app


def _never_answering_gateway(started: asyncio.Event, cancelled: asyncio.Event):
    async def create(**_):
        started.set()
        try:
            await asyncio.Future()
        finally:
            cancelled.set()

    calls = SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
    return lambda *_: SimpleNamespace(embeddings=calls, chat=SimpleNamespace(completions=calls))


async def _post_and_leave(path: str, body: dict, started: asyncio.Event) -> list[dict]:
    pending = [{"type": "http.request", "body": json.dumps(body).encode(), "more_body": False}]

    async def receive():
        if pending:
            return pending.pop()
        await started.wait()
        return {"type": "http.disconnect"}

    sent: list[dict] = []

    async def send(message):
        sent.append(message)

    scope = {
        "type": "http",
        "asgi": {"version": "3.0"},
        "http_version": "1.1",
        "method": "POST",
        "scheme": "http",
        "path": path,
        "raw_path": path.encode(),
        "query_string": b"",
        "root_path": "",
        "headers": [
            (b"content-type", b"application/json"),
            (b"authorization", b"Bearer test-service-token"),
        ],
        "client": ("test", 1),
        "server": ("test", 80),
    }
    await asyncio.wait_for(app(scope, receive, send), 5)
    return sent


@pytest.mark.parametrize("path, body, module, attr, ceiling", ENDPOINTS, ids=IDS)
def test_a_caller_that_leaves_cancels_the_gateway_call(
    path: str, body: dict, module, attr: str, ceiling: float
):
    async def scenario():
        started, cancelled = asyncio.Event(), asyncio.Event()
        with patch.object(module, attr, _never_answering_gateway(started, cancelled)):
            await _post_and_leave(path, body, started)
        assert started.is_set(), "the request never reached the gateway"
        assert cancelled.is_set(), "the gateway call kept running after the caller left"

    asyncio.run(scenario())


def test_a_caller_that_stays_gets_its_answer():
    async def scenario():
        async def receive():
            return {"type": "http.request", "body": b"", "more_body": False}

        sent: list[dict] = []

        async def send(message):
            sent.append(message)

        scope = {
            "type": "http",
            "asgi": {"version": "3.0"},
            "http_version": "1.1",
            "method": "GET",
            "scheme": "http",
            "path": "/healthz",
            "raw_path": b"/healthz",
            "query_string": b"",
            "root_path": "",
            "headers": [],
            "client": ("test", 1),
            "server": ("test", 80),
        }
        await asyncio.wait_for(app(scope, receive, send), 5)
        assert sent[0]["type"] == "http.response.start"
        assert sent[0]["status"] == 200

    asyncio.run(scenario())
