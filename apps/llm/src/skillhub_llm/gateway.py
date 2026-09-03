"""The one place the LiteLLM gateway's address and key are read (Iron Rule 8).

Three call sites used to read them independently and disagree about what an
unconfigured process should do. `enrich` refused with 503. `/embed`,
`/match-reasons` and `/suggest-criteria` carried module-level defaults —
`http://localhost:4000` and the literal dev key `sk-1234` — captured at import
time, so an unconfigured deployment did not report itself as unconfigured: it
produced a 502 that Go cannot tell from a provider outage, and Go's answer to a
provider outage is to degrade search to FTS-only, quietly and for as long as the
misconfiguration lasts (M1 audit, 2026-08-24).

Read at call time, never at import, so a missing variable is a 503 on the
request rather than a crash at startup.

Also the home of the three things every gateway call needs and no endpoint owns:
the spend metadata, the usage reading taken off LiteLLM's own header, and the
pinned sampling parameters. They lived in `evaluate.py` until `generate.py`
imported three private names out of it and inherited the judge's 120s ceiling
with them (M-audit, 2026-08-29); the ceiling was the one thing that was never
shared, which is what made the import the wrong shape.
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

# Pinned sampling for every call whose output is recorded and later compared.
#
# ADR-026 makes a stored verdict identify its own ruler - judge_prompt_version,
# rubric_version, judge_model - so that "passed last week, not today" can be
# attributed to a named change. Sampling was the hole in that: under the
# provider default (1.0) the same prompt version, the same model and the same
# evidence could answer differently, and no column in `evaluations` explained
# it. The same hole sat under report-judge-regression.md's two rounds and under
# ADR-051's 19/20-vs-16/20 model choice, whose noise floor nobody had measured.
#
# Reported back next to `model` and `prompt_version` rather than kept here,
# because a record that reproduces a package (02:GEN-001) has to name the
# sampling that produced it.
#
# ⚠️ MEASURED 2026-08-30, and it undoes the paragraph above for three of the four
# tiers. `gpt-5.6-sol`, `gpt-5.6-terra` and `gpt-5.6-luna` reject any temperature
# but their default: the provider answers 400 "Unsupported value: 'temperature'
# does not support 0.0 with this model". LiteLLM's global `drop_params: true`
# does NOT catch it - its supported-params map says these models take
# temperature - so until that day every enrichment, every LLM judgement, every
# generation and every match-reason call through the real gateway failed with a
# 400, and nothing in the repository had ever called them through one. The three
# entries in infra/compose/litellm-config.yaml now carry `temperature` in
# `additional_drop_params`, the same escape hatch already used there for
# `reasoning_effort`.
#
# So for those three tiers this value is *asked for and dropped*, and the
# provider samples at its own default. Sampling is pinned only on the mini tier.
# It is still reported as what was asked - the same rule SEED already states
# below - but ADR-026's premise that a stored verdict names the sampling that
# produced it now holds for no judge model. That consequence is a decision, not a
# comment: see 05 R-31.
TEMPERATURE = 0.0
# Arbitrary, and only its stability matters. Best-effort at every provider - the
# gateway drops it for models that do not take it (`drop_params: true`) - so it
# is reported as what was asked for, never as a promise that two calls match.
SEED = 20260829


def gateway() -> tuple[str, str]:
    """Base URL and key for the LiteLLM gateway, or 503 if either is missing.

    No provider fallback and no default address. Reaching a provider directly is
    the thing Iron Rule 8 forbids, and a default address is how a process reaches
    one it was never pointed at.
    """
    base_url = os.getenv("LITELLM_BASE_URL")
    api_key = os.getenv("LITELLM_API_KEY")
    if not base_url or not api_key:
        raise HTTPException(
            status_code=503,
            detail="LiteLLM gateway not configured: set LITELLM_BASE_URL and LITELLM_API_KEY",
        )
    if api_key == os.getenv("LITELLM_MASTER_KEY"):
        # ADR-017: the provider keys live at the gateway and every other
        # component holds a Virtual Key. LiteLLM's master key is not "a key with
        # a large budget", it is the gateway's admin credential - it mints keys,
        # reads every key's spend, and can move model routes. This process
        # handles untrusted package content and user prompts; it must not hold
        # that. Loud rather than fail-closed: every recorded real run in this
        # repo was launched with LITELLM_API_KEY=$LITELLM_MASTER_KEY, so failing
        # closed here would break the one workflow that exists today.
        logger.error(
            "LITELLM_API_KEY is the gateway master key. It must be a Virtual Key "
            "with its own budget and model allowlist (ADR-017); the master key "
            "is the gateway's admin credential and this process must not hold it."
        )
    return base_url, api_key


def client(timeout: float) -> AsyncOpenAI:
    """OpenAI-compatible client for the gateway, with one attempt and one ceiling.

    `max_retries=0` is the load-bearing half. The SDK's default is 2 retries and
    `timeout` applies PER ATTEMPT, so a client built without it has a real
    ceiling of 3x `timeout` - and Go's budgets are sized so that this service's
    ceiling surfaces as its own error rather than as Go's deadline
    (foundation/integration/llmclient/client.go). Retry policy is Go's either
    way (ADR-016 rule 6); this is the code saying so instead of a comment.
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
    """Gateway metadata so the spend lands on the right Run and operation (ADR-017).

    Correlation, never authority: this service has no database to look an id up
    in and no workspace to scope it to.

    `operation` is on every call, including the four that carry no id at all.
    Without it the whole service's spend arrives at the gateway under one static
    key in one bucket, and index-time enrichment - once per Skill Version, on the
    flagship tier - is indistinguishable from a search embedding.
    """
    return {"metadata": {k: v for k, v in pairs.items() if v}}


class GatewayUsage(BaseModel):
    """What the gateway charged for one call, as the gateway reported it.

    Outside every model-facing schema on purpose: a judged run must not get to
    write its own bill. Token counts come from the response body, cost from
    LiteLLM's `x-litellm-response-cost` header - the body carries no cost field.
    """

    model_config = ConfigDict(extra="forbid")

    prompt_tokens: int = Field(..., ge=0)
    completion_tokens: int = Field(..., ge=0)
    cost_usd: float | None = Field(None, ge=0)
    # Only ever `gateway`: this service does not price calls itself, so it has no
    # `estimated` to report.
    cost_source: Literal["gateway"] | None = None


def _usage(completion, headers) -> GatewayUsage | None:
    """The call's cost, or None when the gateway reported nothing usable.

    Omitted rather than zero-filled: an absent reading means unreported, and a
    zero here would be read downstream as a free call (same rule as the trace
    usage event's `cost_usd`).
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

    An embeddings response reports `prompt_tokens` and `total_tokens` and no
    `completion_tokens` at all, so zero is the fact here rather than an absent
    reading. `_usage` treats a missing completion count as a malformed chat
    envelope and reports nothing, which is right for chat and would have made
    /embed - 64 texts a call, the highest-volume model call the platform makes -
    the one endpoint whose new bill always read "unreported".
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
