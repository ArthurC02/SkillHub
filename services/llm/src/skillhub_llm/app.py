"""Internal LLM service. Only Go calls this; it is never exposed publicly (ADR-016)."""

from fastapi import FastAPI

app = FastAPI(title="Skill Hub LLM Service", version="0.1.0")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}
