"""The one place the LiteLLM gateway's address and key are read.

Read at call time, never at import, so a missing variable is a 503 on the
request rather than a crash at startup.
"""

from __future__ import annotations

import hashlib
import logging
import math
import os
from functools import lru_cache
from http import HTTPStatus
from typing import Literal

import httpx
from fastapi import HTTPException
from openai import AsyncOpenAI
from pydantic import BaseModel, ConfigDict, Field

logger = logging.getLogger("skillhub_llm.gateway")

# Some model tiers reject any temperature but their provider default and the
# gateway drops this parameter for them, so sampling is pinned only where the
# provider accepts the override; it is still reported as what was asked for.
TEMPERATURE = 0.0
# Best-effort at every provider - the gateway drops it where unsupported - so
# it is reported as what was requested, never as a promise that two calls match.
SEED = 20260829


def gateway() -> tuple[str, str]:
    """Base URL and key for the LiteLLM gateway, or 503 if either is missing."""
    base_url = os.getenv("LITELLM_BASE_URL")
    api_key = os.getenv("LITELLM_API_KEY")
    if not base_url or not api_key:
        raise HTTPException(
            status_code=503,
            detail="LiteLLM gateway not configured: set LITELLM_BASE_URL and LITELLM_API_KEY",
        )
    return base_url, api_key


KEY_CHECK_TIMEOUT = 5.0
BUDGETED_KEYS_REMEMBERED = 1024
_budgeted_keys: set[str] = set()


def _fingerprint(key: str) -> str:
    return hashlib.sha256(key.encode()).hexdigest()


def gateway_root(base_url: str) -> str:
    """The gateway's own address; LITELLM_BASE_URL may name its /v1 API instead."""
    root = base_url.rstrip("/")
    return root.removesuffix("/v1")


async def key_has_finite_budget(
    base_url: str, key: str, transport: httpx.AsyncBaseTransport | None = None
) -> bool:
    """Ask the gateway whether this key carries a finite budget it has not spent.

    Only a Virtual Key has one; the gateway's admin key has none, so this tells
    the two apart without this process ever holding the admin key.
    """
    try:
        async with httpx.AsyncClient(timeout=KEY_CHECK_TIMEOUT, transport=transport) as http:
            response = await http.get(
                f"{gateway_root(base_url)}/key/info", headers={"Authorization": f"Bearer {key}"}
            )
        if response.status_code != HTTPStatus.OK:
            return False
        info = response.json().get("info") or {}
    except httpx.HTTPError, ValueError, AttributeError:
        return False
    budget, spend = info.get("max_budget"), info.get("spend")
    numbers = all(
        isinstance(v, int | float) and not isinstance(v, bool) and math.isfinite(v)
        for v in (budget, spend)
    )
    return numbers and budget > 0 and 0 <= spend < budget


async def key_is_budgeted(key: str) -> bool:
    base_url, _ = gateway()
    fingerprint = _fingerprint(key)
    if fingerprint in _budgeted_keys:
        return True
    if not await key_has_finite_budget(base_url, key):
        return False
    if len(_budgeted_keys) >= BUDGETED_KEYS_REMEMBERED:
        _budgeted_keys.clear()
    _budgeted_keys.add(fingerprint)
    return True


async def require_budgeted_key(key: str | None) -> None:
    if not key or not await key_is_budgeted(key):
        logger.error(
            "a gateway key without a finite, unspent budget was refused; it must be a "
            "Virtual Key with its own budget and model allowlist, never the admin key"
        )
        raise HTTPException(
            status_code=503,
            detail="這個閘道金鑰不是有預算與模型白名單的 Virtual Key，或閘道無法確認它",
        )


async def require_budgeted_service_key() -> None:
    _, api_key = gateway()
    await require_budgeted_key(api_key)


def within(ceiling: float, requested: float | None) -> float:
    """The smaller of this service's ceiling and the one the caller asked for."""
    if requested is None:
        return ceiling
    return min(ceiling, requested)


def failure_label(error: Exception) -> str:
    """The error's type and status: its message can echo the prompt back."""
    status = getattr(error, "status_code", None)
    return type(error).__name__ if status is None else f"{type(error).__name__} {status}"


def client(timeout: float) -> AsyncOpenAI:
    """OpenAI-compatible client for the gateway, with one attempt and one ceiling.

    `max_retries=0` is load-bearing: the SDK default of 2 retries would make
    the real ceiling 3x `timeout`.
    """
    base_url, api_key = gateway()
    return _shared_client().with_options(base_url=base_url, api_key=api_key, timeout=timeout)


@lru_cache(maxsize=1)
def _shared_client() -> AsyncOpenAI:
    """Own one transport; each request view supplies the current URL and key."""
    return AsyncOpenAI(base_url="http://localhost", api_key="unused", max_retries=0)


async def close_client() -> None:
    """Close the shared HTTP transport during FastAPI shutdown."""
    if _shared_client.cache_info().currsize:
        await _shared_client().close()
        _shared_client.cache_clear()


def json_schema_format(name: str, model: type[BaseModel]) -> dict:
    return {
        "type": "json_schema",
        "json_schema": {"name": name, "strict": True, "schema": model.model_json_schema()},
    }


def request_metadata(**pairs: str) -> dict:
    """Gateway metadata so the spend lands on the right Run and operation.

    Correlation, never authority: this service has no database to look an id
    up in and no workspace to scope it to.
    """
    return {"metadata": {k: v for k, v in pairs.items() if v}}


class GatewayUsage(BaseModel):
    """What the gateway charged for one call, as the gateway reported it.

    Cost comes from LiteLLM's `x-litellm-response-cost` header, never the body.
    """

    model_config = ConfigDict(extra="forbid")

    prompt_tokens: int = Field(..., ge=0)
    completion_tokens: int = Field(..., ge=0)
    cost_usd: float | None = Field(None, ge=0)
    cost_source: Literal["gateway"] | None = None


def completion_usage(completion, headers) -> GatewayUsage | None:
    """The call's cost, or None when the gateway reported nothing usable.

    Omitted rather than zero-filled: a zero here would read downstream as a
    free call.
    """
    reported = getattr(completion, "usage", None)
    if reported is None:
        return None
    return _reading(
        getattr(reported, "prompt_tokens", None),
        getattr(reported, "completion_tokens", None),
        headers,
    )


def served_model(completion, headers, requested: str) -> str:
    """The model the gateway served, so records name a model and not a role.

    The gateway answers a role alias with the alias in the body and the
    provider model in `x-litellm-model-name`, prefixed with the provider.
    """
    named = headers.get("x-litellm-model-name") if headers is not None else None
    if named:
        return named.split("/", 1)[-1]
    return getattr(completion, "model", None) or requested


def embedding_usage(response, headers) -> GatewayUsage | None:
    """`completion_usage` for an embeddings response, which has no completion half.

    An embeddings response has no `completion_tokens`, so zero is the fact
    here rather than an absent reading.
    """
    reported = getattr(response, "usage", None)
    if reported is None:
        return None
    return _reading(getattr(reported, "prompt_tokens", None), 0, headers)


def _reading(prompt_tokens, completion_tokens, headers) -> GatewayUsage | None:
    if (
        not isinstance(prompt_tokens, int)
        or isinstance(prompt_tokens, bool)
        or prompt_tokens < 0
        or not isinstance(completion_tokens, int)
        or isinstance(completion_tokens, bool)
        or completion_tokens < 0
    ):
        return None
    try:
        cost = float(headers["x-litellm-response-cost"])
    except KeyError, TypeError, ValueError:
        cost = None
    if cost is not None and (not math.isfinite(cost) or cost < 0):
        cost = None
    return GatewayUsage(
        prompt_tokens=prompt_tokens,
        completion_tokens=completion_tokens,
        cost_usd=cost,
        cost_source="gateway" if cost is not None else None,
    )
