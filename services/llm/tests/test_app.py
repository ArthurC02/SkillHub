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
    response = client.post("/match-reasons", json={
        "query": "",
        "candidates": [{"skill_id": "x", "name": "n", "summary": "s"}],
    })
    assert response.status_code == 422


def test_match_reasons_rejects_empty_candidates():
    response = client.post("/match-reasons", json={
        "query": "build a PDF",
        "candidates": [],
    })
    assert response.status_code == 422


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
