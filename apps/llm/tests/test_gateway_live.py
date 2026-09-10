"""Every configured model tier, asked for with the sampling this service sends.

Calls the gateway live; opt-in only via SKILLHUB_LIVE_GATEWAY=1, since it
costs money and every other test here mocks the gateway.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest
import yaml
from openai import OpenAI

from skillhub_llm.gateway import SEED, TEMPERATURE

CONFIG = Path(__file__).resolve().parents[3] / "infra" / "compose" / "litellm-config.yaml"

# Captured at import, before conftest's autouse fixture points every other
# test at the discard port - this is the one file meant to reach a real gateway.
BASE_URL = os.getenv("LITELLM_BASE_URL", "")
API_KEY = os.getenv("LITELLM_API_KEY", "")

live = pytest.mark.skipif(
    os.getenv("SKILLHUB_LIVE_GATEWAY") != "1",
    reason="SKILLHUB_LIVE_GATEWAY=1 not set: this test calls real models and costs money",
)


def chat_models() -> list[str]:
    """The model names a caller may ask this deployment for, embeddings aside.

    Read from the deployed config rather than listed here, so a tier added
    there is covered without anyone remembering to add it.
    """
    doc = yaml.safe_load(CONFIG.read_text(encoding="utf-8"))
    return [
        entry["model_name"] for entry in doc["model_list"] if "embedding" not in entry["model_name"]
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
    assert completion.choices, f"{model} answered with no choices"
