from unittest.mock import AsyncMock, patch

from fastapi.testclient import TestClient

from skillhub_llm.app import app

client = TestClient(app)


def test_healthz_returns_ok():
    response = client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_embed_rejects_empty_texts():
    response = client.post("/embed", json={"texts": []})
    assert response.status_code == 422  # pydantic validation


def test_embed_rejects_missing_texts():
    response = client.post("/embed", json={})
    assert response.status_code == 422


def test_match_reasons_rejects_empty_query():
    response = client.post(
        "/match-reasons",
        json={
            "query": "",
            "candidates": [{"skill_id": "x", "name": "n", "summary": "s"}],
        },
    )
    assert response.status_code == 422


def test_match_reasons_rejects_empty_candidates():
    response = client.post(
        "/match-reasons",
        json={
            "query": "build a PDF",
            "candidates": [],
        },
    )
    assert response.status_code == 422


def _match_reasons_litellm(content: str):
    """A stub litellm whose completion returns `content` as the model's message."""
    from unittest.mock import MagicMock

    mock_litellm = MagicMock()
    message = MagicMock()
    message.content = content
    choice = MagicMock()
    choice.message = message
    response = MagicMock()
    response.choices = [choice]
    mock_litellm.acompletion = AsyncMock(return_value=response)
    return mock_litellm


CANDIDATES = [
    {"skill_id": "s1", "name": "invoice-parser", "summary": "reads invoices"},
    {"skill_id": "s2", "name": "csv-cleaner", "summary": "normalises csv"},
]


def test_match_reasons_returns_one_reason_per_candidate():
    """DISC-002: the batch call answers for every candidate it was given."""
    body = (
        '{"reasons": [{"skill_id": "s1", "reason": "it parses invoices"}, '
        '{"skill_id": "s2", "reason": "it cleans csv"}]}'
    )
    with patch.dict("sys.modules", {"litellm": _match_reasons_litellm(body)}):
        response = client.post(
            "/match-reasons", json={"query": "read my invoices", "candidates": CANDIDATES}
        )

    assert response.status_code == 200
    reasons = {r["skill_id"]: r["reason"] for r in response.json()["reasons"]}
    assert reasons == {"s1": "it parses invoices", "s2": "it cleans csv"}


def test_match_reasons_asks_the_gateway_for_the_shape_it_parses():
    """import-report.md §6.1 bug 3: the prompt asked for an array while the call
    forced a bare json_object, so every real answer arrived under a key this
    endpoint did not read. Schema and parser now come from the same model."""
    body = '{"reasons": [{"skill_id": "s1", "reason": "it parses invoices"}]}'
    stub = _match_reasons_litellm(body)
    with patch.dict("sys.modules", {"litellm": stub}):
        client.post("/match-reasons", json={"query": "read my invoices", "candidates": CANDIDATES})

    fmt = stub.acompletion.await_args.kwargs["response_format"]
    assert fmt["type"] == "json_schema"
    assert fmt["json_schema"]["strict"] is True
    assert "reasons" in fmt["json_schema"]["schema"]["properties"]


def test_match_reasons_returns_a_partial_answer_as_partial():
    """A skipped candidate comes back absent, not filled with a stock sentence:
    Go labels what it gets here as model-generated (DISC-002)."""
    body = '{"reasons": [{"skill_id": "s1", "reason": "it parses invoices"}]}'
    with patch.dict("sys.modules", {"litellm": _match_reasons_litellm(body)}):
        response = client.post(
            "/match-reasons", json={"query": "read my invoices", "candidates": CANDIDATES}
        )

    assert response.status_code == 200
    assert [r["skill_id"] for r in response.json()["reasons"]] == ["s1"]


def test_match_reasons_rejects_an_off_schema_wrapper():
    """The shape a real gpt-4o-mini produced under json_object. It must not be
    replaced by a template sentence that Go would then label `model`."""
    body = '{"skills": [{"skill_id": "s1", "reason": "it parses invoices"}]}'
    with patch.dict("sys.modules", {"litellm": _match_reasons_litellm(body)}):
        response = client.post(
            "/match-reasons", json={"query": "read my invoices", "candidates": CANDIDATES}
        )

    assert response.status_code == 200
    assert response.json()["reasons"] == []


def test_match_reasons_survives_unparseable_model_output():
    """Go treats 200-with-gaps and 502 differently; unusable JSON is not a crash."""
    with patch.dict("sys.modules", {"litellm": _match_reasons_litellm("not json at all")}):
        response = client.post(
            "/match-reasons", json={"query": "read my invoices", "candidates": CANDIDATES}
        )

    assert response.status_code == 200
    assert response.json()["reasons"] == []


def test_match_reasons_drops_ids_that_were_not_asked_about():
    body = '{"reasons": [{"skill_id": "made-up", "reason": "nope"}]}'
    with patch.dict("sys.modules", {"litellm": _match_reasons_litellm(body)}):
        response = client.post(
            "/match-reasons", json={"query": "read my invoices", "candidates": CANDIDATES}
        )

    assert response.json()["reasons"] == []


def test_match_reasons_reports_provider_failure_as_502():
    """Go's degradation path keys off the status code, so it has to be 502."""
    from unittest.mock import MagicMock

    mock_litellm = MagicMock()
    mock_litellm.acompletion = AsyncMock(side_effect=RuntimeError("gateway down"))
    with patch.dict("sys.modules", {"litellm": mock_litellm}):
        response = client.post(
            "/match-reasons", json={"query": "read my invoices", "candidates": CANDIDATES}
        )

    assert response.status_code == 502


def test_embed_success():
    """Embed endpoint calls LiteLLM and returns the vectors."""
    from unittest.mock import MagicMock

    # litellm is imported inside the endpoint body, so patching sys.modules
    # below is sufficient; there is no module-level attribute to patch.
    mock_litellm = MagicMock()
    mock_response = AsyncMock()
    mock_response.data = [
        {"embedding": [0.1] * 1536, "index": 0},
        {"embedding": [0.2] * 1536, "index": 1},
    ]
    mock_litellm.aembedding = AsyncMock(return_value=mock_response)

    # The import inside the function body needs patching.
    with patch.dict("sys.modules", {"litellm": mock_litellm}):
        response = client.post("/embed", json={"texts": ["hello", "world"]})

    assert response.status_code == 200
    body = response.json()
    assert body["model"] == "text-embedding-3-small"
    assert body["dimensions"] == 1536
    assert len(body["embeddings"]) == 2
