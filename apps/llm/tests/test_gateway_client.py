"""How the gateway client is BUILT, which every endpoint test monkeypatches
away. Kept as a list of builders rather than a scan for `AsyncOpenAI(`, so a
new endpoint means a new line here.
"""

from __future__ import annotations

import asyncio
import logging
import os
from collections.abc import Callable

import httpx
import pytest
from fastapi import HTTPException
from openai import AsyncOpenAI

from skillhub_llm import app as app_module
from skillhub_llm import enrich, evaluate, gateway, generate

BUILDERS: list[tuple[str, Callable[[], AsyncOpenAI], float]] = [
    ("enrich", enrich._client, 60.0),
    ("evaluate", evaluate._client, 120.0),
    ("generate", generate._client, 120.0),
    ("creation", lambda: gateway.client(120.0), 120.0),
    ("app", lambda: app_module._client(20.0), 20.0),
]


@pytest.mark.parametrize("name, build, timeout", BUILDERS, ids=[b[0] for b in BUILDERS])
def test_the_client_makes_one_attempt_with_the_declared_ceiling(
    name: str, build: Callable[[], AsyncOpenAI], timeout: float
) -> None:
    client = build()

    assert client.max_retries == 0, (
        f"{name}: the SDK retries twice by default and `timeout` is PER ATTEMPT, so "
        f"without max_retries=0 the real ceiling is 3x {timeout}s - Go's budget for this "
        "call is sized on this service's ceiling surfacing first, and it would not"
    )
    assert client.timeout == timeout, (
        f"{name}: the client must carry the module's declared ceiling, not the SDK "
        f"default; got {client.timeout!r}"
    )
    assert str(client.base_url).rstrip("/") == os.environ["LITELLM_BASE_URL"].rstrip("/"), name


def test_gateway_returns_the_configured_url_and_key(monkeypatch) -> None:
    monkeypatch.setenv("LITELLM_API_KEY", "sk-fake-virtual-key")

    base_url, api_key = gateway.gateway()

    assert (base_url, api_key) == (os.environ["LITELLM_BASE_URL"], "sk-fake-virtual-key")


def key_info(status: int, body: object) -> httpx.MockTransport:
    def answer(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/key/info", request.url
        assert request.headers["Authorization"] == "Bearer sk-asked"
        if isinstance(body, Exception):
            raise body
        if isinstance(body, str):
            return httpx.Response(status, text=body)
        return httpx.Response(status, json=body)

    return httpx.MockTransport(answer)


@pytest.mark.parametrize(
    "base_url",
    [
        "http://gateway:4000",
        "http://gateway:4000/",
        "http://gateway:4000/v1",
        "http://gateway:4000/v1/",
    ],
)
def test_the_key_is_asked_about_at_the_gateway_root_whichever_form_the_url_takes(base_url) -> None:
    budget = {"info": {"max_budget": 10.0, "spend": 1.0}}
    checked = asyncio.run(
        gateway._real_key_check(base_url, "sk-asked", transport=key_info(200, budget))
    )
    assert checked is True


def budget_check(status: int, body: object) -> bool:
    return asyncio.run(
        gateway._real_key_check(
            "http://gateway:4000/", "sk-asked", transport=key_info(status, body)
        )
    )


@pytest.mark.parametrize(
    "status, body, budgeted",
    [
        (200, {"info": {"max_budget": 10.0, "spend": 1.0}}, True),
        (200, {"info": {"max_budget": 10, "spend": 0}}, True),
        (200, {"info": {"max_budget": 10.0, "spend": 9.99}}, True),
        (200, {"info": {"max_budget": 10.0, "spend": 10.0}}, False),
        (200, {"info": {"max_budget": None, "spend": 0.0}}, False),
        (200, {"info": {"spend": 0.0}}, False),
        (200, {"info": {"max_budget": 0, "spend": 0}}, False),
        (200, {"info": {"max_budget": 10.0, "spend": -1.0}}, False),
        (200, {"info": {"max_budget": True, "spend": 0}}, False),
        (200, {"info": {"max_budget": "10", "spend": 0}}, False),
        (200, {"info": {"max_budget": float("inf"), "spend": 0}}, False),
        (200, {}, False),
        (200, ["not", "an", "object"], False),
        (200, "not json", False),
        (401, {"info": {"max_budget": 10.0, "spend": 1.0}}, False),
        (200, httpx.ConnectError("refused"), False),
    ],
    ids=[
        "virtual-key",
        "whole-numbers",
        "just-under-budget",
        "budget-spent",
        "admin-key-no-budget",
        "budget-absent",
        "zero-budget",
        "negative-spend",
        "boolean-budget",
        "string-budget",
        "infinite-budget",
        "no-info",
        "not-an-object",
        "not-json",
        "refused",
        "unreachable",
    ],
)
def test_only_a_key_with_a_finite_unspent_budget_is_confirmed(status, body, budgeted) -> None:
    assert budget_check(status, body) is budgeted


def test_a_confirmed_key_is_not_asked_about_again(monkeypatch) -> None:
    asked = []

    async def check(base_url, key, transport=None):
        asked.append(key)
        return True

    monkeypatch.setattr(gateway, "key_has_finite_budget", check)
    assert asyncio.run(gateway.key_is_budgeted("sk-a")) is True
    assert asyncio.run(gateway.key_is_budgeted("sk-a")) is True
    assert asked == ["sk-a"]


def test_confirmed_keys_are_remembered_only_up_to_the_limit(monkeypatch) -> None:
    asked = []

    async def check(base_url, key, transport=None):
        asked.append(key)
        return True

    monkeypatch.setattr(gateway, "key_has_finite_budget", check)
    monkeypatch.setattr(gateway, "BUDGETED_KEYS_REMEMBERED", 2)
    for key in ["sk-a", "sk-b", "sk-a", "sk-c", "sk-a"]:
        assert asyncio.run(gateway.key_is_budgeted(key)) is True
    assert len(gateway._budgeted_keys) == 2
    assert asked == ["sk-a", "sk-b", "sk-c", "sk-a"]


def test_a_refused_key_is_asked_about_again(monkeypatch) -> None:
    asked = []

    async def check(base_url, key, transport=None):
        asked.append(key)
        return len(asked) > 1

    monkeypatch.setattr(gateway, "key_has_finite_budget", check)
    assert asyncio.run(gateway.key_is_budgeted("sk-a")) is False
    assert asyncio.run(gateway.key_is_budgeted("sk-a")) is True
    assert asked == ["sk-a", "sk-a"]


def test_an_unconfirmed_key_is_refused_and_logged(monkeypatch, caplog) -> None:
    async def refused(base_url, key, transport=None):
        return False

    monkeypatch.setattr(gateway, "key_has_finite_budget", refused)
    with caplog.at_level(logging.ERROR, logger="skillhub_llm.gateway"):
        with pytest.raises(HTTPException) as excinfo:
            asyncio.run(gateway.require_budgeted_service_key())

    assert excinfo.value.status_code == 503
    assert "Virtual Key" in excinfo.value.detail
    assert any("Virtual Key" in r.message for r in caplog.records), caplog.text


def test_a_missing_scoped_key_is_refused_without_asking_the_gateway(monkeypatch) -> None:
    async def never(base_url, key, transport=None):
        raise AssertionError("asked the gateway about no key")

    monkeypatch.setattr(gateway, "key_has_finite_budget", never)
    with pytest.raises(HTTPException) as excinfo:
        asyncio.run(gateway.require_budgeted_key(None))
    assert excinfo.value.status_code == 503


def test_every_ceiling_is_a_real_number() -> None:
    """`timeout=None` is the litellm/6000s defect wearing the SDK's clothes: it
    passes the assertion above if the module constant is also None."""
    for name, _, timeout in BUILDERS:
        assert isinstance(timeout, float) and timeout > 0, name


def test_clients_with_different_timeouts_reuse_the_gateway_transport() -> None:
    short = gateway.client(10.0)
    long = gateway.client(20.0)

    assert short.timeout == 10.0
    assert long.timeout == 20.0
    assert short._client is long._client


def test_rotated_gateway_credentials_reuse_the_transport(monkeypatch) -> None:
    monkeypatch.setenv("LITELLM_BASE_URL", "http://first.example")
    monkeypatch.setenv("LITELLM_API_KEY", "sk-first")
    first = gateway.client(10.0)
    monkeypatch.setenv("LITELLM_BASE_URL", "http://second.example")
    monkeypatch.setenv("LITELLM_API_KEY", "sk-second")
    second = gateway.client(10.0)

    assert first._client is second._client
    assert str(second.base_url).startswith("http://second.example")
    assert second.api_key == "sk-second"


def test_shutdown_closes_and_clears_the_shared_transport() -> None:
    gateway._shared_client()
    assert gateway._shared_client.cache_info().currsize == 1

    asyncio.run(gateway.close_client())

    assert gateway._shared_client.cache_info().currsize == 0


@pytest.mark.parametrize(
    "base_url, api_key",
    [("", "sk-test"), ("http://127.0.0.1:9", ""), ("", "")],
    ids=["no-base-url", "no-key", "neither"],
)
def test_a_gateway_missing_its_url_or_key_is_unavailable(monkeypatch, base_url, api_key) -> None:
    monkeypatch.setenv("LITELLM_BASE_URL", base_url)
    monkeypatch.setenv("LITELLM_API_KEY", api_key)

    with pytest.raises(HTTPException) as excinfo:
        gateway.gateway()

    assert excinfo.value.status_code == 503
    assert "LITELLM_BASE_URL" in excinfo.value.detail


def test_a_gateway_with_its_url_and_key_is_returned_as_configured(monkeypatch) -> None:
    monkeypatch.setenv("LITELLM_BASE_URL", "http://gateway:4000")
    monkeypatch.setenv("LITELLM_API_KEY", "sk-virtual")
    monkeypatch.delenv("LITELLM_MASTER_KEY", raising=False)

    assert gateway.gateway() == ("http://gateway:4000", "sk-virtual")
