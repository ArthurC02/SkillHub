"""Internal LLM service. Only Go calls this; it is never exposed publicly."""

from __future__ import annotations

import logging
import os
import secrets
from contextlib import asynccontextmanager
from typing import Annotated

from fastapi import Depends, FastAPI, HTTPException, Request, status
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from skillhub_llm.creation import router as creation_router
from skillhub_llm.enrich import router as enrich_router
from skillhub_llm.evaluate import router as evaluate_router
from skillhub_llm.gateway import GatewayUsage, _embedding_usage, _metadata, _usage, close_client
from skillhub_llm.gateway import client as _client
from skillhub_llm.generate import router as generate_router
from skillhub_llm.untrusted import data_block_rules, fence, scrub

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


@asynccontextmanager
async def lifespan(_app: FastAPI):
    yield
    await close_client()


app = FastAPI(
    title="Skill Hub LLM Service",
    version="0.1.0",
    lifespan=lifespan,
)
protected = [Depends(require_service_token)]
app.include_router(enrich_router, dependencies=protected)
app.include_router(evaluate_router, dependencies=protected)
app.include_router(generate_router, dependencies=protected)
app.include_router(creation_router, dependencies=protected)
logger = logging.getLogger("skillhub_llm")


@app.exception_handler(RequestValidationError)
async def request_validation_error(
    _request: Request, _error: RequestValidationError
) -> JSONResponse:
    """Keep FastAPI's runtime 422 body aligned with the OpenAPI Error schema."""
    return JSONResponse(status_code=422, content={"detail": "request validation failed"})


EMBED_MODEL = "text-embedding-3-small"
MATCH_REASON_MODEL = os.getenv("MATCH_REASON_MODEL", "gpt-5.6-luna")
SUGGEST_CRITERIA_MODEL = os.getenv("SUGGEST_CRITERIA_MODEL", "gpt-5.4-mini")

# budget-ceiling: app.EMBED_TIMEOUT_SECONDS
EMBED_TIMEOUT_SECONDS = 20.0
# budget-ceiling: app.MATCH_REASONS_TIMEOUT_SECONDS
MATCH_REASONS_TIMEOUT_SECONDS = 8.0
# budget-ceiling: app.SUGGEST_CRITERIA_TIMEOUT_SECONDS
SUGGEST_CRITERIA_TIMEOUT_SECONDS = 30.0

DATA_TAG = "untrusted_catalog_data"


def _scrub(text: str) -> str:
    """Strip the closing delimiter so untrusted content cannot close its own block."""
    return scrub(DATA_TAG, text)


@app.get("/healthz")
def healthz() -> dict[str, str]:
    """Liveness: this process is running. It says nothing about capability."""
    return {"status": "ok"}


@app.get("/readyz", dependencies=[Depends(require_service_token)])
def readyz() -> dict[str, object]:
    """Readiness: is the service token valid and is the gateway configured.

    Checks configuration only; it does not call the gateway.
    """
    base_url = os.getenv("LITELLM_BASE_URL", "")
    api_key = os.getenv("LITELLM_API_KEY", "")
    missing = [
        name
        for name, value in (("LITELLM_BASE_URL", base_url), ("LITELLM_API_KEY", api_key))
        if not value
    ]
    return {
        "status": "ready" if not missing else "not_ready",
        "gateway_configured": not missing,
        "missing": missing,
    }


class EmbedRequest(BaseModel):
    texts: list[str] = Field(..., min_length=1, max_length=64)
    timeout_seconds: float | None = Field(None, gt=0)


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    model: str
    dimensions: int
    usage: GatewayUsage | None = None


@app.post("/embed", response_model=EmbedResponse, dependencies=protected)
async def embed(req: EmbedRequest) -> EmbedResponse:
    """Generate embeddings for one or more texts via text-embedding-3-small."""
    ceiling = EMBED_TIMEOUT_SECONDS
    if req.timeout_seconds is not None:
        ceiling = min(EMBED_TIMEOUT_SECONDS, req.timeout_seconds)
    client = _client(ceiling)

    try:
        raw = await client.embeddings.with_raw_response.create(
            model=EMBED_MODEL,
            input=req.texts,
            extra_body=_metadata(operation="embed"),
        )
        response = raw.parse()
    except Exception as e:
        logger.exception("embedding call failed")
        raise HTTPException(status_code=502, detail="gateway error") from e

    try:
        vectors = [item.embedding for item in response.data]
        valid = len(vectors) == len(req.texts) and all(
            isinstance(vector, list) and len(vector) == 1536  # one-number: embeddingDimensions
            for vector in vectors
        )
    except (AttributeError, KeyError, TypeError):
        valid = False
        vectors = []
    if not valid:
        logger.warning("embedding provider returned a malformed envelope")
        raise HTTPException(status_code=502, detail="embedding provider returned malformed output")
    dims = 1536  # one-number: embeddingDimensions
    return EmbedResponse(
        embeddings=vectors,
        model=EMBED_MODEL,
        dimensions=dims,
        usage=_embedding_usage(response, raw.headers),
    )


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


class MatchReasons(BaseModel):
    """The JSON schema handed to the model, and the shape parsed back out of it."""

    model_config = ConfigDict(extra="forbid")

    reasons: list[MatchReason]


class MatchReasonsResponse(MatchReasons):
    """The wire shape: what the model wrote, plus what the call cost.

    Kept separate from the parent schema: GatewayUsage's defaults would fail
    strict JSON schema validation.
    """

    model: str
    usage: GatewayUsage | None = None


@app.post("/match-reasons", response_model=MatchReasonsResponse, dependencies=protected)
async def match_reasons(req: MatchReasonsRequest) -> MatchReasonsResponse:
    """Generate human-readable match reasons for search result candidates.

    A candidate this cannot produce a reason for is absent from the response,
    never a fabricated one.
    """
    candidates_text = "\n".join(
        f"- [{_scrub(c.skill_id)}] {_scrub(c.name)}: {_scrub(c.summary)}" for c in req.candidates
    )

    system_prompt = (
        "You are a search result explainer for a Skill marketplace. "
        "Given a user's task description and a list of candidate Skills, "
        "produce a brief (1-2 sentence) match reason for each candidate "
        "explaining why it is relevant to the user's task. "
        "Be specific about which capabilities match which needs. "
        + data_block_rules(
            DATA_TAG, "the user's own task text and summaries supplied with the packages"
        )
        + ' Respond with a JSON object {"reasons": [...]}, where each entry has '
        "the keys 'skill_id' and 'reason'. Use the skill_id exactly as given."
    )

    user_prompt = (
        fence(
            DATA_TAG,
            f"User's task:\n{_scrub(req.query)}\n\nCandidate Skills:\n{candidates_text}",
        )
        + "\n\nProduce the match reasons."
    )

    client = _client(MATCH_REASONS_TIMEOUT_SECONDS)

    try:
        raw = await client.chat.completions.with_raw_response.create(
            model=MATCH_REASON_MODEL,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            temperature=0.3,
            max_tokens=1024,
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "match_reasons",
                    "strict": True,
                    "schema": MatchReasons.model_json_schema(),
                },
            },
            extra_body=_metadata(operation="match-reasons"),
        )
        response = raw.parse()
    except Exception as e:
        logger.exception("match-reasons LLM call failed")
        raise HTTPException(status_code=502, detail="gateway error") from e

    try:
        content = (response.choices[0].message.content or "").strip()
    except (AttributeError, IndexError, TypeError):
        logger.warning("match-reasons provider returned a malformed envelope")
        raise HTTPException(
            status_code=502, detail="match-reasons provider returned malformed output"
        ) from None
    try:
        parsed = MatchReasons.model_validate_json(content)
    except ValidationError:
        logger.warning("match-reasons: model output did not match the schema")
        return MatchReasonsResponse(
            reasons=[], model=MATCH_REASON_MODEL, usage=_usage(response, raw.headers)
        )

    wanted = {c.skill_id for c in req.candidates}
    return MatchReasonsResponse(
        reasons=[r for r in parsed.reasons if r.skill_id in wanted and r.reason],
        model=MATCH_REASON_MODEL,
        usage=_usage(response, raw.headers),
    )


MAX_SUGGESTED_CRITERIA = 8  # one-number: suggestCriteriaMaxItems


class DatasetField(BaseModel):
    """One column of an uploaded dataset: its name and inferred type only, never row values."""

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
    skill_summary: str = ""
    user_prompt: str = Field(..., min_length=1)
    datasets: list[DatasetOutline] = Field(default_factory=list, max_length=20)


class SuggestedCriterion(BaseModel):
    model_config = ConfigDict(extra="forbid")

    text: str


class SuggestedCriteria(BaseModel):
    """The JSON schema handed to the model, and the shape parsed back out of it."""

    model_config = ConfigDict(extra="forbid")

    criteria: list[SuggestedCriterion]


class SuggestCriteriaResponse(SuggestedCriteria):
    """The wire shape: the proposals, plus what the call cost."""

    usage: GatewayUsage | None = None


@app.post("/suggest-criteria", response_model=SuggestCriteriaResponse, dependencies=protected)
async def suggest_criteria(req: SuggestCriteriaRequest) -> SuggestCriteriaResponse:
    """Propose acceptance criteria for one test case.

    A proposal, not a decision; an unusable answer comes back as an empty
    list rather than invented text.
    """
    dataset_text = (
        "\n".join(
            f"- {_scrub(d.file_name)} ({_scrub(d.content_type) or 'unknown type'}): "
            + (
                ", ".join(f"{_scrub(f.name)}:{_scrub(f.inferred_type)}" for f in d.fields)
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
        + data_block_rules(
            DATA_TAG, "the user's own task text, a Skill summary and uploaded column names"
        )
        + f" Return at most {MAX_SUGGESTED_CRITERIA} criteria as "
        '{"criteria": [{"text": "..."}]}.'
    )
    user_prompt = (
        fence(
            DATA_TAG,
            f"Skill: {_scrub(req.skill_name)}\n"
            f"What the Skill does: {_scrub(req.skill_summary)}\n\n"
            f"User's task:\n{_scrub(req.user_prompt)}\n\n"
            f"Attached data (field names and inferred types only, no rows):\n{dataset_text}",
        )
        + "\n\nPropose the acceptance criteria."
    )

    client = _client(SUGGEST_CRITERIA_TIMEOUT_SECONDS)

    try:
        raw = await client.chat.completions.with_raw_response.create(
            model=SUGGEST_CRITERIA_MODEL,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            temperature=0.3,
            max_tokens=1024,
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "suggested_criteria",
                    "strict": True,
                    "schema": SuggestedCriteria.model_json_schema(),
                },
            },
            extra_body=_metadata(operation="suggest-criteria"),
        )
        response = raw.parse()
    except Exception as e:
        logger.exception("suggest-criteria LLM call failed")
        raise HTTPException(status_code=502, detail="gateway error") from e

    content = (response.choices[0].message.content or "").strip()
    usage = _usage(response, raw.headers)
    try:
        parsed = SuggestedCriteria.model_validate_json(content)
    except ValidationError:
        logger.warning("suggest-criteria: model output did not match the schema")
        return SuggestCriteriaResponse(criteria=[], usage=usage)

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
    return SuggestCriteriaResponse(criteria=kept, usage=usage)
