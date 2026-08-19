import pytest


@pytest.fixture(autouse=True)
def llm_service_token(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("LLM_SERVICE_TOKEN", "test-service-token")
