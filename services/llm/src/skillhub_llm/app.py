"""Internal LLM service. Only Go calls this; it is never exposed publicly (ADR-016).

Endpoints:
  POST /embed            - generate embeddings via text-embedding-3-small (ADR-013, PDM-003)
  POST /match-reasons    - generate match reason sentences for search results (ADR-013 section 3)
  POST /v1/enrich-skill  - index-time enrichment of a Skill Version (ADR-013 section 1)
"""

from __future__ import annotations

import logging
import os

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from skillhub_llm.enrich import router as enrich_router

app = FastAPI(title="Skill Hub LLM Service", version="0.1.0")
app.include_router(enrich_router)
logger = logging.getLogger("skillhub_llm")

# LiteLLM gateway base URL (Iron Rule 8: all model calls via LiteLLM, never direct).
LITELLM_BASE_URL = os.getenv("LITELLM_BASE_URL", "http://localhost:4000")
LITELLM_API_KEY = os.getenv("LITELLM_API_KEY", "sk-1234")

EMBED_MODEL = "text-embedding-3-small"
MATCH_REASON_MODEL = os.getenv("MATCH_REASON_MODEL", "gpt-4o-mini")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


# --- Embedding endpoint (DISC-001) ---


class EmbedRequest(BaseModel):
    texts: list[str] = Field(..., min_length=1, max_length=64)


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    model: str
    dimensions: int


@app.post("/embed", response_model=EmbedResponse)
async def embed(req: EmbedRequest) -> EmbedResponse:
    """Generate embeddings for one or more texts.

    Uses text-embedding-3-small (1536 dims) routed through LiteLLM (ADR-017).
    """
    import litellm

    try:
        response = await litellm.aembedding(
            model=EMBED_MODEL,
            input=req.texts,
            api_base=LITELLM_BASE_URL,
            api_key=LITELLM_API_KEY,
        )
    except Exception as e:
        logger.exception("embedding call failed")
        raise HTTPException(status_code=502, detail=f"embedding provider error: {e}") from e

    vectors = [item["embedding"] for item in response.data]
    dims = len(vectors[0]) if vectors else 0
    return EmbedResponse(embeddings=vectors, model=EMBED_MODEL, dimensions=dims)


# --- Match-reasons endpoint (DISC-002) ---


class SkillCandidate(BaseModel):
    skill_id: str
    name: str
    summary: str


class MatchReasonsRequest(BaseModel):
    query: str = Field(..., min_length=1)
    candidates: list[SkillCandidate] = Field(..., min_length=1, max_length=20)


class MatchReason(BaseModel):
    skill_id: str
    reason: str


class MatchReasonsResponse(BaseModel):
    reasons: list[MatchReason]


@app.post("/match-reasons", response_model=MatchReasonsResponse)
async def match_reasons(req: MatchReasonsRequest) -> MatchReasonsResponse:
    """Generate human-readable match reasons for search result candidates.

    ADR-013 section 3: Top-N results get LLM-polished reasons; timeout or failure
    degrades to template version on the Go side.
    """
    import litellm

    # Build the prompt: ask the model to explain why each skill matches the query.
    candidates_text = "\n".join(f"- [{c.skill_id}] {c.name}: {c.summary}" for c in req.candidates)

    system_prompt = (
        "You are a search result explainer for a Skill marketplace. "
        "Given a user's task description and a list of candidate Skills, "
        "produce a brief (1-2 sentence) match reason for each candidate "
        "explaining why it is relevant to the user's task. "
        "Be specific about which capabilities match which needs. "
        "Respond as a JSON array of objects with keys 'skill_id' and 'reason'. "
        "Output only the JSON array, no markdown fences."
    )

    user_prompt = (
        f"User's task:\n{req.query}\n\n"
        f"Candidate Skills:\n{candidates_text}\n\n"
        "Produce a JSON array of match reasons."
    )

    try:
        response = await litellm.acompletion(
            model=MATCH_REASON_MODEL,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            api_base=LITELLM_BASE_URL,
            api_key=LITELLM_API_KEY,
            temperature=0.3,
            max_tokens=1024,
            response_format={"type": "json_object"},
        )
    except Exception as e:
        logger.exception("match-reasons LLM call failed")
        raise HTTPException(status_code=502, detail=f"match-reasons provider error: {e}") from e

    # Parse the LLM response.
    import json

    raw = response.choices[0].message.content.strip()
    try:
        parsed = json.loads(raw)
        # Handle both {"reasons": [...]} wrapper and bare array.
        if isinstance(parsed, dict):
            items = parsed.get("reasons", parsed.get("results", []))
        elif isinstance(parsed, list):
            items = parsed
        else:
            items = []
    except json.JSONDecodeError:
        logger.warning("match-reasons: unparseable LLM response, returning empty")
        items = []

    reasons = []
    for item in items:
        if isinstance(item, dict) and "skill_id" in item and "reason" in item:
            reasons.append(MatchReason(skill_id=item["skill_id"], reason=item["reason"]))

    # Fill in template fallbacks for any candidates not covered by the LLM response.
    covered = {r.skill_id for r in reasons}
    for c in req.candidates:
        if c.skill_id not in covered:
            reasons.append(
                MatchReason(
                    skill_id=c.skill_id,
                    reason=f"{c.name} may be relevant to your task.",
                )
            )

    return MatchReasonsResponse(reasons=reasons)
