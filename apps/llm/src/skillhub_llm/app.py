"""Internal LLM service. Only Go calls this; it is never exposed publicly (ADR-016).

Endpoints:
  POST /embed                 - generate embeddings via text-embedding-3-small (ADR-013, PDM-003)
  POST /match-reasons         - generate match reason sentences for search results (ADR-013 §3)
  POST /v1/enrich-skill       - index-time enrichment of a Skill Version (ADR-013 section 1)
  POST /suggest-criteria      - propose acceptance criteria for a test case (TEST-002)
  POST /judge-run             - judge one Run against its acceptance criteria (EVAL-001)
  POST /suggest-improvements  - propose package changes from one evaluation (EVAL-002)
"""

from __future__ import annotations

import logging
import os
import secrets
from typing import Annotated

from fastapi import Depends, FastAPI, HTTPException, Request, status
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from skillhub_llm.enrich import router as enrich_router
from skillhub_llm.evaluate import router as evaluate_router
from skillhub_llm.gateway import gateway
from skillhub_llm.generate import router as generate_router

service_bearer = HTTPBearer(auto_error=False)


def require_service_token(
    credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(service_bearer)],
) -> None:
    """Authenticate the Go control plane before any LLM capability runs."""
    expected = os.getenv("LLM_SERVICE_TOKEN", "")
    if not expected:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="LLM service authentication is not configured",
        )
    if credentials is None or not secrets.compare_digest(credentials.credentials, expected):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid service credential",
            headers={"WWW-Authenticate": "Bearer"},
        )


app = FastAPI(
    title="Skill Hub LLM Service",
    version="0.1.0",
)
protected = [Depends(require_service_token)]
app.include_router(enrich_router, dependencies=protected)
app.include_router(evaluate_router, dependencies=protected)
app.include_router(generate_router, dependencies=protected)
logger = logging.getLogger("skillhub_llm")


@app.exception_handler(RequestValidationError)
async def request_validation_error(
    _request: Request, _error: RequestValidationError
) -> JSONResponse:
    """Keep FastAPI's runtime 422 body aligned with the OpenAPI Error schema."""
    return JSONResponse(status_code=422, content={"detail": "request validation failed"})


EMBED_MODEL = "text-embedding-3-small"
MATCH_REASON_MODEL = os.getenv("MATCH_REASON_MODEL", "gpt-5.6-luna")
# Suggestion-class work runs on the mini tier (PDM-003 §11.6: the flagship buys
# nothing measurable here and costs 6.7x).
SUGGEST_CRITERIA_MODEL = os.getenv("SUGGEST_CRITERIA_MODEL", "gpt-5.4-mini")


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


@app.post("/embed", response_model=EmbedResponse, dependencies=protected)
async def embed(req: EmbedRequest) -> EmbedResponse:
    """Generate embeddings for one or more texts.

    Uses text-embedding-3-small (1536 dims) routed through LiteLLM (ADR-017).
    """
    import litellm

    base_url, api_key = gateway()

    try:
        response = await litellm.aembedding(
            model=EMBED_MODEL,
            input=req.texts,
            api_base=base_url,
            api_key=api_key,
        )
    except Exception as e:
        logger.exception("embedding call failed")
        raise HTTPException(status_code=502, detail=f"embedding provider error: {e}") from e

    try:
        vectors = [item["embedding"] for item in response.data]
        valid = len(vectors) == len(req.texts) and all(
            isinstance(vector, list) and len(vector) == 1536 for vector in vectors
        )
    except (AttributeError, KeyError, TypeError):
        valid = False
        vectors = []
    if not valid:
        logger.warning("embedding provider returned a malformed envelope")
        raise HTTPException(status_code=502, detail="embedding provider returned malformed output")
    dims = 1536
    return EmbedResponse(embeddings=vectors, model=EMBED_MODEL, dimensions=dims)


# --- Match-reasons endpoint (DISC-002) ---


class SkillCandidate(BaseModel):
    skill_id: str
    name: str
    summary: str


class MatchReasonsRequest(BaseModel):
    query: str = Field(..., min_length=1, max_length=2000)
    candidates: list[SkillCandidate] = Field(..., min_length=1, max_length=20)


class MatchReason(BaseModel):
    model_config = ConfigDict(extra="forbid")

    skill_id: str
    reason: str


class MatchReasonsResponse(BaseModel):
    """Also the JSON schema handed to the model, so the wire shape and the shape
    the prompt asks for cannot drift apart again (import-report.md §6.1 bug 3).
    """

    model_config = ConfigDict(extra="forbid")

    reasons: list[MatchReason]


@app.post("/match-reasons", response_model=MatchReasonsResponse, dependencies=protected)
async def match_reasons(req: MatchReasonsRequest) -> MatchReasonsResponse:
    """Generate human-readable match reasons for search result candidates.

    ADR-013 section 3: Top-N results get LLM-polished reasons. Anything this
    endpoint cannot get from the model is simply absent from the answer — Go
    fills those candidates with its own template reason and labels them
    `template` (DISC-002 provenance). This service must never invent a filler
    sentence, because Go would then label the filler as model-generated.
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
        'Respond with a JSON object {"reasons": [...]}, where each entry has '
        "the keys 'skill_id' and 'reason'. Use the skill_id exactly as given."
    )

    user_prompt = (
        f"User's task:\n{req.query}\n\n"
        f"Candidate Skills:\n{candidates_text}\n\n"
        "Produce the match reasons."
    )

    base_url, api_key = gateway()

    try:
        response = await litellm.acompletion(
            model=MATCH_REASON_MODEL,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            api_base=base_url,
            api_key=api_key,
            temperature=0.3,
            max_tokens=1024,
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "match_reasons",
                    "strict": True,
                    "schema": MatchReasonsResponse.model_json_schema(),
                },
            },
        )
    except Exception as e:
        logger.exception("match-reasons LLM call failed")
        raise HTTPException(status_code=502, detail=f"match-reasons provider error: {e}") from e

    try:
        raw = (response.choices[0].message.content or "").strip()
    except (AttributeError, IndexError, TypeError):
        logger.warning("match-reasons provider returned a malformed envelope")
        raise HTTPException(
            status_code=502, detail="match-reasons provider returned malformed output"
        ) from None
    try:
        parsed = MatchReasonsResponse.model_validate_json(raw)
    except ValidationError:
        # A gateway that ignores the schema can still hand back another shape
        # (the observed one was {"skills": [...]}). Returning nothing is the
        # honest answer: Go then shows template reasons, labelled as template.
        logger.warning("match-reasons: model output did not match the schema")
        return MatchReasonsResponse(reasons=[])

    # Only answer for what was asked about: a hallucinated skill_id would be
    # dropped by Go anyway, and dropping it here keeps the response auditable.
    wanted = {c.skill_id for c in req.candidates}
    return MatchReasonsResponse(
        reasons=[r for r in parsed.reasons if r.skill_id in wanted and r.reason]
    )


# --- Suggest-criteria endpoint (TEST-002) ---

MAX_SUGGESTED_CRITERIA = 8


class DatasetField(BaseModel):
    """One column of an uploaded dataset, described by its shape only.

    Iron rule 11 and 02:TEST-002 資料使用範圍: Go sends the field NAME and an
    inferred type, never a value from a row. A user's uploaded rows are their
    private data; they do not need to reach a model for the model to propose
    "the output covers every row of `amount`". Go is the enforcement point (see
    internal/testlab/suggest.go) — this schema is the second statement of the
    same rule, so a future caller cannot quietly start sending cell contents.
    """

    model_config = ConfigDict(extra="forbid")

    name: str
    inferred_type: str


class DatasetOutline(BaseModel):
    model_config = ConfigDict(extra="forbid")

    file_name: str
    content_type: str = ""
    fields: list[DatasetField] = Field(default_factory=list)


class SuggestCriteriaRequest(BaseModel):
    skill_name: str = ""
    # The Skill's summary or an excerpt of SKILL.md — whichever the caller has.
    skill_summary: str = ""
    user_prompt: str = Field(..., min_length=1)
    datasets: list[DatasetOutline] = Field(default_factory=list, max_length=20)


class SuggestedCriterion(BaseModel):
    model_config = ConfigDict(extra="forbid")

    text: str


class SuggestCriteriaResponse(BaseModel):
    """Also the JSON schema handed to the model, so the wire shape and the shape
    the prompt asks for cannot drift apart (same rule as MatchReasonsResponse).
    """

    model_config = ConfigDict(extra="forbid")

    criteria: list[SuggestedCriterion]


@app.post("/suggest-criteria", response_model=SuggestCriteriaResponse, dependencies=protected)
async def suggest_criteria(req: SuggestCriteriaRequest) -> SuggestCriteriaResponse:
    """Propose acceptance criteria for one test case (02:TEST-001 自動建議).

    A proposal, not a decision: Go stores whatever comes back with
    `source = "suggested"` and the user edits, confirms or deletes it. Nothing
    here decides what is acceptable, and no criterion is confirmed on the user's
    behalf (ADR-016 iron rule 6).

    An unusable answer comes back as an empty list rather than as invented text —
    the user then writes their own criteria, which is the documented manual path.
    """
    import litellm

    dataset_text = (
        "\n".join(
            f"- {d.file_name} ({d.content_type or 'unknown type'}): "
            + (
                ", ".join(f"{f.name}:{f.inferred_type}" for f in d.fields)
                if d.fields
                else "no field names available"
            )
            for d in req.datasets
        )
        or "(no files attached)"
    )

    system_prompt = (
        "You help a creator write acceptance criteria for a test run of an Agent Skill. "
        "Each criterion must be one short sentence that a reviewer can judge as met or "
        "not met by looking at the run's output. Prefer observable, checkable statements "
        "over vague quality words. Do not invent data values; you are given column names "
        "and types only. Write the criteria in the language of the user's task description. "
        f"Return at most {MAX_SUGGESTED_CRITERIA} criteria as "
        '{"criteria": [{"text": "..."}]}.'
    )
    user_prompt = (
        f"Skill: {req.skill_name}\n"
        f"What the Skill does: {req.skill_summary}\n\n"
        f"User's task:\n{req.user_prompt}\n\n"
        f"Attached data (field names and inferred types only, no rows):\n{dataset_text}\n\n"
        "Propose the acceptance criteria."
    )

    base_url, api_key = gateway()

    try:
        response = await litellm.acompletion(
            model=SUGGEST_CRITERIA_MODEL,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            api_base=base_url,
            api_key=api_key,
            temperature=0.3,
            max_tokens=1024,
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "suggested_criteria",
                    "strict": True,
                    "schema": SuggestCriteriaResponse.model_json_schema(),
                },
            },
        )
    except Exception as e:
        logger.exception("suggest-criteria LLM call failed")
        raise HTTPException(status_code=502, detail=f"suggest-criteria provider error: {e}") from e

    raw = (response.choices[0].message.content or "").strip()
    try:
        parsed = SuggestCriteriaResponse.model_validate_json(raw)
    except ValidationError:
        logger.warning("suggest-criteria: model output did not match the schema")
        return SuggestCriteriaResponse(criteria=[])

    seen: set[str] = set()
    kept: list[SuggestedCriterion] = []
    for c in parsed.criteria:
        text = c.text.strip()
        if not text or text in seen:
            continue
        seen.add(text)
        kept.append(SuggestedCriterion(text=text))
        if len(kept) == MAX_SUGGESTED_CRITERIA:
            break
    return SuggestCriteriaResponse(criteria=kept)
