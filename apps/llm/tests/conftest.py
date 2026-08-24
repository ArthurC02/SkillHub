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
    monkeypatch.setenv("LITELLM_BASE_URL", "http://litellm.test:4000")
    monkeypatch.setenv("LITELLM_API_KEY", "sk-test-not-a-real-key")
