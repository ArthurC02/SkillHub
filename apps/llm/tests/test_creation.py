import asyncio
import json
from types import SimpleNamespace
from unittest.mock import patch

import pytest
from fastapi.testclient import TestClient

from skillhub_llm import creation
from skillhub_llm.app import app

client = TestClient(app, headers={"Authorization": "Bearer test-service-token"})
HEADERS = {"X-Creation-Gateway-Key": "sk-step"}
SKILL = {
    "name": "invoice-check",
    "description": "Check invoices when asked.",
    "compatibility": "",
    "allowed_tools": "",
    "body": "Read the input and report missing totals.",
    "files": [],
}


def request(**changes):
    value = {
        "session_id": "s1",
        "revision": 1,
        "messages": [{"role": "user", "content": "Help me check invoices"}],
        "brief": "",
        "brief_confirmed": False,
        "diagram_understanding": "",
        "diagram_confirmed": False,
        "references": [],
        "allowed_tools": [],
        "timeout_seconds": 10,
        "max_output_tokens": 100,
    }
    return value | changes


def decision(**changes):
    return {
        "outcome": "clarification",
        "message": "What output do you need?",
        "brief": None,
        "diagram_understanding": None,
        "tool_intent": None,
        "draft": None,
    } | changes


def stub(result, calls, finish="stop"):
    response = SimpleNamespace(
        choices=[
            SimpleNamespace(
                message=SimpleNamespace(content=json.dumps(result)), finish_reason=finish
            )
        ],
        usage=SimpleNamespace(prompt_tokens=10, completion_tokens=5),
    )
    raw = SimpleNamespace(parse=lambda: response, headers={"x-litellm-response-cost": "0.001"})

    async def create(**kwargs):
        calls.append(kwargs)
        return raw

    value = SimpleNamespace(
        chat=SimpleNamespace(
            completions=SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
        )
    )
    value.with_options = lambda **_: value
    return value


def invoke(req, result, finish="stop"):
    calls = []
    with patch.object(creation, "client", lambda _: stub(result, calls, finish)):
        response = client.post("/v1/creation/step", headers=HEADERS, json=req)
    return response, calls


def test_multiround_confirmation_and_tool_observation_revision():
    messages = request()["messages"]
    response, _ = invoke(request(), decision())
    assert response.status_code == 200
    assert response.json()["outcome"] == "clarification"
    messages += [
        {"role": "assistant", "content": response.json()["message"]},
        {"role": "user", "content": "Give me a CSV of missing invoice totals."},
    ]
    response, _ = invoke(
        request(messages=messages),
        decision(outcome="confirm_brief", brief="Report missing invoice totals as CSV."),
    )
    assert response.json()["outcome"] == "confirm_brief"
    brief = response.json()["brief"]
    confirmed = request(messages=messages, brief=brief, brief_confirmed=True)
    response, calls = invoke(confirmed, decision(outcome="draft", draft=SKILL))
    assert response.status_code == 200
    assert response.json()["draft"]["body"] == SKILL["body"]
    assert response.json()["usage"]["cost_usd"] == 0.001
    assert calls[0]["max_tokens"] == 100
    assert calls[0]["response_format"]["json_schema"]["strict"] is True

    response, _ = invoke(
        confirmed | {"draft": SKILL, "allowed_tools": ["validate_draft"]},
        decision(outcome="tool_intent", tool_intent={"kind": "validate_draft", "query": ""}),
    )
    assert response.json()["tool_intent"]["kind"] == "validate_draft"
    revised = SKILL | {"body": "Report missing totals as CSV, with invoice_id and reason columns."}
    observations = messages + [{"role": "tool", "content": "Validation: CSV columns are missing."}]
    response, calls = invoke(
        confirmed | {"messages": observations, "draft": SKILL},
        decision(outcome="draft", draft=revised),
    )
    assert response.json()["draft"]["body"] == revised["body"]
    assert "CSV columns are missing" in calls[0]["messages"][1]["content"]
    assert SKILL["body"] != revised["body"]  # The prior immutable snapshot was not edited.


@pytest.mark.parametrize(
    "changes,outcome",
    [
        ({}, "confirm_brief"),
        (
            {
                "brief": "agreed",
                "brief_confirmed": True,
                "diagram_understanding": "A -> B",
                "diagram_confirmed": False,
            },
            "confirm_diagram",
        ),
    ],
)
def test_unconfirmed_inputs_cannot_produce_draft(changes, outcome):
    response, _ = invoke(
        request(**changes),
        decision(
            outcome="draft",
            draft=SKILL,
            brief=changes.get("brief", "proposal"),
            diagram_understanding=changes.get("diagram_understanding"),
        ),
    )
    assert response.status_code == 200
    assert response.json()["outcome"] == outcome
    assert response.json()["draft"] is None


@pytest.mark.parametrize(
    "field,outcome", [("brief", "confirm_brief"), ("diagram_understanding", "confirm_diagram")]
)
def test_changed_confirmed_input_requires_new_confirmation(field, outcome):
    req = request(
        brief="agreed", brief_confirmed=True, diagram_understanding="A -> B", diagram_confirmed=True
    )
    response, _ = invoke(req, decision(outcome="draft", draft=SKILL, **{field: "changed"}))
    assert response.status_code == 200
    assert response.json()["outcome"] == outcome
    assert response.json()["draft"] is None


def test_nonconfirmation_cannot_replace_confirmed_input():
    response, _ = invoke(request(brief="agreed", brief_confirmed=True), decision(brief="changed"))
    assert response.json()["brief"] == "agreed"


def test_image_is_only_multimodal_and_requires_confirmation():
    # Same minimal PNG accepted by the existing diagram validator.
    diagram = {
        "media_type": "image/png",
        "data": (
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAA"
            "C0lEQVR42mP8/x8AAwMCAO+aP8sAAAAASUVORK5CYII="
        ),
    }
    response, calls = invoke(
        request(diagram=diagram),
        decision(outcome="confirm_diagram", diagram_understanding="Input -> validation -> CSV"),
    )
    assert response.status_code == 200
    parts = calls[0]["messages"][1]["content"]
    assert diagram["data"] not in parts[0]["text"]
    assert parts[1]["image_url"]["url"].endswith(diagram["data"])
    assert diagram["data"] not in response.text


def test_unauthorized_tool_never_escapes():
    response, _ = invoke(
        request(),
        decision(outcome="tool_intent", tool_intent={"kind": "search_catalog", "query": "invoice"}),
    )
    assert response.json()["outcome"] == "clarification"
    assert response.json()["tool_intent"] is None


def test_search_intent_and_returned_observations_use_separate_jobs():
    response, _ = invoke(
        request(allowed_tools=["search_catalog"]),
        decision(outcome="tool_intent", tool_intent={"kind": "search_catalog", "query": "invoice"}),
    )
    assert response.json()["tool_intent"]["query"] == "invoice"
    response, calls = invoke(
        request(
            messages=[
                {
                    "role": "tool",
                    "content": "Catalog: invoice-check, fixed version v1; awaiting selection",
                }
            ]
        ),
        decision(message="Would you like to use invoice-check as a reference?"),
    )
    assert response.json()["outcome"] == "clarification"
    assert "fixed version v1" in calls[0]["messages"][1]["content"]


@pytest.mark.parametrize(
    "result,finish",
    [
        (decision(outcome="draft", draft=None), "stop"),
        (decision(outcome="draft", draft=SKILL | {"body": ""}), "stop"),
        (
            decision(
                outcome="draft", draft=SKILL | {"files": [{"path": "x", "content": "a" * 100001}]}
            ),
            "stop",
        ),
        (decision(message="x" * 20001), "stop"),
        (decision(), "length"),
        ({"unexpected": "shape"}, "stop"),
    ],
)
def test_malformed_truncated_or_oversized_output_is_refused(result, finish):
    response, _ = invoke(request(brief="agreed", brief_confirmed=True), result, finish)
    assert response.status_code == 502


def test_missing_usage_is_unknown():
    calls = []
    value = stub(decision(), calls)
    old_create = value.chat.completions.with_raw_response.create

    async def create(**kwargs):
        raw = await old_create(**kwargs)
        raw.parse().usage = None
        return raw

    value.chat.completions.with_raw_response.create = create
    with patch.object(creation, "client", lambda _: value):
        response = client.post("/v1/creation/step", headers=HEADERS, json=request())
    assert response.json()["usage"] is None


def test_tracing_disabled_even_when_environment_enables_it(monkeypatch):
    monkeypatch.setenv("LANGSMITH_TRACING", "true")
    monkeypatch.setenv("LANGCHAIN_TRACING_V2", "true")
    monkeypatch.setenv("LANGSMITH_API_KEY", "must-not-be-used")
    with patch("langsmith.Client.create_run") as create_run:
        response, calls = invoke(request(), decision())
    assert response.status_code == 200
    create_run.assert_not_called()
    assert HEADERS["X-Creation-Gateway-Key"] not in json.dumps(calls)


def test_strict_decision_schema_has_no_optional_properties_or_bound_keywords():
    schema = creation.CreationDecision.model_json_schema()

    def walk(value):
        if isinstance(value, dict):
            assert not set(value) & {"default", "minLength", "maxLength", "maxItems", "pattern"}
            if value.get("type") == "object":
                assert value.get("additionalProperties") is False
                assert set(value.get("required", [])) == set(value.get("properties", {}))
            for child in value.values():
                walk(child)
        elif isinstance(value, list):
            for child in value:
                walk(child)

    walk(schema)


def test_cancellation_reaches_gateway_await():
    async def scenario():
        started, cancelled = asyncio.Event(), asyncio.Event()

        async def create(**_):
            started.set()
            try:
                await asyncio.Future()
            finally:
                cancelled.set()

        value = SimpleNamespace(
            chat=SimpleNamespace(
                completions=SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
            )
        )
        value.with_options = lambda **_: value

        async def connected():
            return False

        with patch.object(creation, "client", lambda _: value):
            task = asyncio.create_task(
                creation.creation_step(
                    creation.CreationStepRequest(**request()),
                    SimpleNamespace(is_disconnected=connected),
                    "sk-step",
                )
            )
            await started.wait()
            task.cancel()
            with pytest.raises(asyncio.CancelledError):
                await task
            assert cancelled.is_set()

    asyncio.run(scenario())


def test_service_and_scoped_key_required(monkeypatch):
    assert TestClient(app).post("/v1/creation/step", json=request()).status_code == 401
    assert client.post("/v1/creation/step", json=request()).status_code == 503
    monkeypatch.setenv("LITELLM_MASTER_KEY", "master")
    assert (
        client.post(
            "/v1/creation/step", headers={"X-Creation-Gateway-Key": "master"}, json=request()
        ).status_code
        == 503
    )
