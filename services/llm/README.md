# services/llm

Internal Python LLM service (FastAPI + uv), per ADR-016. Called by the Go platform
over internal HTTP only — it never consumes the queue and holds no business rules.

```bash
uv sync
uv run pytest
uv run ruff check .
uv run uvicorn skillhub_llm.app:app --reload
```
