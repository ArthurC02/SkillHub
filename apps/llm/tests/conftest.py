import pytest


@pytest.fixture(autouse=True)
def llm_service_token(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("LLM_SERVICE_TOKEN", "test-service-token")


@pytest.fixture(autouse=True)
def litellm_gateway(monkeypatch: pytest.MonkeyPatch) -> None:
    """A configured gateway is the normal case, so it is the default here.

    Every model call now refuses a process that was never told where the gateway
    is (gateway.py). Tests that want that refusal delenv these two themselves.
    """
    # A literal address on the discard port: a test that forgot to patch its
    # client fails instantly with connection refused, rather than paying for a
    # DNS lookup first.
    monkeypatch.setenv("LITELLM_BASE_URL", "http://127.0.0.1:9")
    monkeypatch.setenv("LITELLM_API_KEY", "sk-test-not-a-real-key")
