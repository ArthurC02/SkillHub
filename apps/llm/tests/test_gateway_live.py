"""Every configured model tier, asked for with the sampling this service sends.

The machine that was missing on 2026-08-30. `TEMPERATURE = 0.0` had been the
pinned sampling for every recorded call since ADR-026, and `gpt-5.6-sol`,
`gpt-5.6-terra` and `gpt-5.6-luna` reject it outright — the provider answers
400 "Unsupported value: 'temperature' does not support 0.0 with this model".
So index-time enrichment, the LLM judge, skill generation and match-reasons had
*never once* succeeded through a real gateway, and every test in this directory
was green, because every one of them mocks the gateway. A mocked gateway cannot
refuse a parameter a provider refuses.

It costs money, so it is opt-in and CI never runs it — the same gate the paid
end-to-end tests use:

    LITELLM_BASE_URL=http://127.0.0.1:4000 \\
    LITELLM_API_KEY=$LITELLM_MASTER_KEY \\
    SKILLHUB_LIVE_GATEWAY=1 uv run --project apps/llm pytest tests/test_gateway_live.py

A few tokens per tier. Run it after any change to the model list, to
`additional_drop_params`, or to TEMPERATURE/SEED — those are exactly the changes
whose breakage is invisible to every other test here.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest
import yaml
from openai import OpenAI

from skillhub_llm.gateway import SEED, TEMPERATURE

CONFIG = Path(__file__).resolve().parents[3] / "infra" / "compose" / "litellm-config.yaml"

# Captured at import, which is before conftest's autouse `litellm_gateway`
# fixture points every test at the discard port. That fixture is right for the
# rest of this directory — a test that forgot to patch its client should fail
# instantly rather than call a provider — and this is the one file that means to.
BASE_URL = os.getenv("LITELLM_BASE_URL", "")
API_KEY = os.getenv("LITELLM_API_KEY", "")

live = pytest.mark.skipif(
    os.getenv("SKILLHUB_LIVE_GATEWAY") != "1",
    reason="SKILLHUB_LIVE_GATEWAY=1 not set: this test calls real models and costs money",
)


def chat_models() -> list[str]:
    """The model names a caller may ask this deployment for, embeddings aside.

    Read from the deployed config rather than listed here, so a tier added there
    is covered without anyone remembering to add it — the failure this test
    exists for arrived with models nobody re-probed.
    """
    doc = yaml.safe_load(CONFIG.read_text(encoding="utf-8"))
    return [
        entry["model_name"]
        for entry in doc["model_list"]
        if "embedding" not in entry["model_name"]
    ]


@live
@pytest.mark.parametrize("model", chat_models())
def test_every_tier_accepts_the_sampling_this_service_sends(model: str) -> None:
    if not BASE_URL or not API_KEY:
        pytest.fail("SKILLHUB_LIVE_GATEWAY=1 needs LITELLM_BASE_URL and LITELLM_API_KEY set too")
    client = OpenAI(base_url=BASE_URL, api_key=API_KEY)
    completion = client.chat.completions.create(
        model=model,
        messages=[{"role": "user", "content": "Reply with the single word ok."}],
        temperature=TEMPERATURE,
        seed=SEED,
        max_completion_tokens=2000,
    )
    # The assertion is that the call returned at all: a tier that refuses
    # TEMPERATURE raises BadRequestError here, which is the whole point. The
    # content check only keeps a 200 carrying nothing from passing as a success.
    assert completion.choices, f"{model} answered with no choices"
