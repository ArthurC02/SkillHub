import pytest


@pytest.fixture(autouse=True)
def llm_service_token(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("LLM_SERVICE_TOKEN", "test-service-token")


@pytest.fixture(autouse=True)
def litellm_gateway(monkeypatch: pytest.MonkeyPatch) -> None:
    """A configured gateway is the normal case, so it is the default here.

    Tests that want the missing-gateway refusal delenv these two themselves.
    """
    # Port 9 is the discard port: an unpatched client fails instantly with
    # connection refused instead of waiting on a real network call.
    monkeypatch.setenv("LITELLM_BASE_URL", "http://127.0.0.1:9")
    monkeypatch.setenv("LITELLM_API_KEY", "sk-test-not-a-real-key")
