"""How the gateway client is BUILT, which nothing else here looks at.

Every endpoint test monkeypatches the client away - correctly, because none of
them is about the network. The consequence is that the two properties that bound
a call are invisible to the whole suite: deleting `timeout=` left all 113 tests
green, and `max_retries` was never there to be deleted. The SDK default is 2
retries and `timeout` applies per attempt, so a missing `max_retries=0` silently
triples every ceiling Go's budgets are sized against.

Same reason tests/test_strict_schemas.py exists: that file pins the schema half
of the call, this one pins the client half. No network - conftest supplies
LITELLM_BASE_URL and LITELLM_API_KEY, and building a client connects to nothing.

Kept as a list of builders rather than a scan for `AsyncOpenAI(`, so a new
endpoint means a new line here; a scan would pass silently on the client it
failed to find.
"""

from __future__ import annotations

import os
from collections.abc import Callable

import pytest
from openai import AsyncOpenAI

from skillhub_llm import app as app_module
from skillhub_llm import enrich, evaluate

BUILDERS: list[tuple[str, Callable[[], AsyncOpenAI], float]] = [
    ("enrich", enrich._client, enrich.LLM_TIMEOUT_SECONDS),
    ("evaluate", evaluate._client, evaluate.LLM_TIMEOUT_SECONDS),
    (
        "app:/embed",
        lambda: app_module._client(app_module.EMBED_TIMEOUT_SECONDS),
        app_module.EMBED_TIMEOUT_SECONDS,
    ),
    (
        "app:/match-reasons",
        lambda: app_module._client(app_module.MATCH_REASONS_TIMEOUT_SECONDS),
        app_module.MATCH_REASONS_TIMEOUT_SECONDS,
    ),
    (
        "app:/suggest-criteria",
        lambda: app_module._client(app_module.SUGGEST_CRITERIA_TIMEOUT_SECONDS),
        app_module.SUGGEST_CRITERIA_TIMEOUT_SECONDS,
    ),
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
    # Iron Rule 8: every call leaves through the gateway, never at a provider.
    assert str(client.base_url).rstrip("/") == os.environ["LITELLM_BASE_URL"].rstrip("/"), name


def test_every_ceiling_is_a_real_number() -> None:
    """`timeout=None` is the litellm/6000s defect wearing the SDK's clothes: it
    passes the assertion above if the module constant is also None."""
    for name, _, timeout in BUILDERS:
        assert isinstance(timeout, float) and timeout > 0, name
