"""GEN-002 tests.

Six cases, chosen because each one fails silently if it regresses. The happy
path anchors the shape; the rest guard rules that exist because of a
measurement and would otherwise degrade into "it still returns something".
"""

import json
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient
from openai import APIConnectionError

from skillhub_llm import generate
from skillhub_llm.app import app

client = TestClient(app, headers={"Authorization": "Bearer test-service-token"})

TASK = "我每個月要把廠商寄來的掃描單據整理成一份表格交出去。"

GOOD_SKILL = {
    "name": "scanned-invoice-table",
    "description": (
        "從掃描的單據影像抽出表格內容並合併成一份檔案。"
        "當使用者手上是掃描件、需要把每份裡的表格彙整成一份時使用。"
    ),
    "compatibility": "需要能讀取影像或 PDF 的工具。",
    "metadata": {"category": "documents"},
    "allowed_tools": "Read Write",
    "body": (
        "# 掃描單據轉表格\n\n1. 確認每份檔案是影像還是可選取文字的 PDF。\n"
        "2. 逐份抽出表格。\n3. 合併並回報每份的來源。"
    ),
    "files": [],
}


def _fake_client(content: str, capture: list | None = None, finish_reason=None):
    """Stand-in for AsyncOpenAI, shaped like the one in test_evaluate.

    `finish_reason` defaults to None rather than "stop" so that a stub written
    before the truncation branch existed keeps meaning "not truncated".
    """

    async def create(**kwargs):
        if capture is not None:
            capture.append(kwargs)
        choice = SimpleNamespace(
            message=SimpleNamespace(content=content), finish_reason=finish_reason
        )
        completion = SimpleNamespace(
            choices=[choice],
            usage=SimpleNamespace(prompt_tokens=300, completion_tokens=1100),
        )
        return SimpleNamespace(
            parse=lambda: completion, headers={"x-litellm-response-cost": "0.0055"}
        )

    return SimpleNamespace(
        chat=SimpleNamespace(
            completions=SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
        )
    )


@pytest.fixture
def capture(monkeypatch):
    calls: list = []

    def use(content: str, **stub):
        monkeypatch.setattr(generate, "_client", lambda: _fake_client(content, calls, **stub))
        return calls

    return use


def test_generates_a_skill_and_reports_its_own_provenance(capture):
    calls = capture(json.dumps(GOOD_SKILL))
    r = client.post("/v1/generate-skill", json={"task_description": TASK})
    assert r.status_code == 200, r.text
    body = r.json()

    assert body["skill"]["name"] == "scanned-invoice-table"
    # model / prompt_version / usage are this service's own and are what the
    # generated version's provenance record is built from (02:GEN-002).
    assert body["model"] == generate.GENERATE_SKILL_MODEL
    assert body["prompt_version"] == generate.GENERATE_SKILL_PROMPT_VERSION
    assert body["usage"]["cost_usd"] == pytest.approx(0.0055)
    assert calls[0]["max_tokens"] == generate.MAX_OUTPUT_TOKENS


def test_the_schema_handed_to_the_model_cannot_carry_a_licence(capture):
    """ADR-046 決策 5 on the schema, not in the prompt.

    03:GEN-001 asks for exactly this: a prohibition that lives only in prompt
    text holds until the next prompt revision. Asserting the absence of the
    property is the only way this stays true when the prompt is rewritten.
    """
    calls = capture(json.dumps(GOOD_SKILL))
    client.post("/v1/generate-skill", json={"task_description": TASK})

    schema = calls[0]["response_format"]["json_schema"]["schema"]
    assert "license" not in schema["properties"]
    assert schema["additionalProperties"] is False


def test_a_licence_field_in_the_model_output_is_refused(capture):
    """The other half of the same rule: forbidding it in the schema handed out
    is worth nothing if the parse on the way back accepts it anyway.
    """
    capture(json.dumps({**GOOD_SKILL, "license": "MIT"}))
    r = client.post("/v1/generate-skill", json={"task_description": TASK})
    assert r.status_code == 502
    assert "malformed" in r.json()["detail"]


def test_truncation_is_a_different_failure_from_malformed_output(capture):
    """ADR-047 決策 2: truncation must not be retried at the same cap, because
    the cap covers reasoning plus output and a second call buys the same
    answer. Go can only act on that if the two are distinguishable here.

    The round-A failure this guards against emitted an EMPTY string after
    spending all 8000 tokens reasoning - so a truncated call looks exactly like
    a malformed one unless finish_reason is checked first.
    """
    capture("", finish_reason="length")
    r = client.post("/v1/generate-skill", json={"task_description": TASK})
    assert r.status_code == 502
    detail = r.json()["detail"]
    assert "truncated" in detail
    assert "malformed" not in detail


def test_gateway_failure_is_502(monkeypatch):
    monkeypatch.setattr(
        generate,
        "_client",
        lambda: SimpleNamespace(
            chat=SimpleNamespace(
                completions=SimpleNamespace(
                    with_raw_response=SimpleNamespace(
                        create=AsyncMock(side_effect=APIConnectionError(request=None))
                    )
                )
            )
        ),
    )
    r = client.post("/v1/generate-skill", json={"task_description": TASK})
    assert r.status_code == 502


def test_whitespace_only_never_reaches_the_gateway(capture):
    """Ten spaces clears `min_length=8`, which counts characters.

    Without the validator this buys a paid call that can only fail, and the
    test that used three spaces would have passed for the wrong reason - the
    length floor caught it, not the emptiness.
    """
    calls = capture(json.dumps(GOOD_SKILL))
    r = client.post("/v1/generate-skill", json={"task_description": " " * 10})
    assert r.status_code == 422
    assert calls == []


def test_the_truncation_sentence_is_the_one_go_matches_on(capture):
    """The Go side classifies truncation by this exact sentence.

    It used to match the bare word "truncated", which the other 502 — the
    gateway exception, quoted verbatim — could contain by accident, and then the
    user was told to shorten a task that was never too long. Changing the wording
    here without changing llmclient.truncationMarker puts every truncation into
    the "malformed" branch, where it gets retried at the same ceiling and buys
    the same answer (ADR-047 決策 2). Nothing else would report that.
    """
    capture("", finish_reason="length")
    r = client.post("/v1/generate-skill", json={"task_description": TASK})
    assert r.json()["detail"] == "generate model output was truncated at the token ceiling"
