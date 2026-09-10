"""The one place the LiteLLM gateway's address and key are read.

Read at call time, never at import, so a missing variable is a 503 on the
request rather than a crash at startup.
"""

from __future__ import annotations

import logging
import math
import os
from functools import lru_cache
from typing import Literal

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
    # An unset master key is None, and api_key already passed the truthiness
    # check above, so an unset master key can never match here.
    if api_key == os.getenv("LITELLM_MASTER_KEY"):
        logger.error(
            "LITELLM_API_KEY is the gateway master key. It must be a Virtual Key "
            "with its own budget and model allowlist (ADR-017); the master key "
            "is the gateway's admin credential and this process must not hold it."
        )
        raise HTTPException(
            status_code=503,
            detail=(
                "LITELLM_API_KEY 是閘道的 master key；"
                "這個服務只能拿有預算與模型白名單的 Virtual Key（ADR-017）"
            ),
        )
    return base_url, api_key


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


def _metadata(**pairs: str) -> dict:
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


def _usage(completion, headers) -> GatewayUsage | None:
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


def _embedding_usage(response, headers) -> GatewayUsage | None:
    """`_usage` for an embeddings response, which has no completion half.

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
    except (KeyError, TypeError, ValueError):
        cost = None
    if cost is not None and (not math.isfinite(cost) or cost < 0):
        cost = None
    return GatewayUsage(
        prompt_tokens=prompt_tokens,
        completion_tokens=completion_tokens,
        cost_usd=cost,
        cost_source="gateway" if cost is not None else None,
    )
