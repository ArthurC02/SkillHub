"""Internal LLM service. Only Go calls this; it is never exposed publicly."""

from __future__ import annotations

import logging
import os
import secrets
from contextlib import asynccontextmanager
from typing import Annotated, Literal, NamedTuple

from fastapi import Depends, FastAPI, HTTPException, Request, status
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from skillhub_llm.agent import router as agent_router
from skillhub_llm.creation import router as creation_router
from skillhub_llm.disconnect import CancelWhenCallerLeaves
from skillhub_llm.enrich import router as enrich_router
from skillhub_llm.evaluate import router as evaluate_router
from skillhub_llm.gateway import (
    GatewayUsage,
    close_client,
    completion_usage,
    embedding_usage,
    failure_label,
    gateway_failure,
    json_schema_format,
    key_is_budgeted,
    request_metadata,
    require_budgeted_service_key,
    served_model,
    within,
)
from skillhub_llm.gateway import client as _client
from skillhub_llm.generate import router as generate_router
from skillhub_llm.intent import router as intent_router
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
    if credentials is None or not secrets.compare_digest(
        credentials.credentials.encode(), expected.encode()
    ):
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
budgeted = [Depends(require_service_token), Depends(require_budgeted_service_key)]
app.add_middleware(CancelWhenCallerLeaves)
app.include_router(enrich_router, dependencies=budgeted)
app.include_router(evaluate_router, dependencies=budgeted)
app.include_router(generate_router, dependencies=budgeted)
app.include_router(creation_router, dependencies=protected)
app.include_router(agent_router, dependencies=protected)
app.include_router(intent_router, dependencies=budgeted)
logger = logging.getLogger("skillhub_llm")


@app.exception_handler(RequestValidationError)
async def request_validation_error(
    _request: Request, _error: RequestValidationError
) -> JSONResponse:
    """Keep FastAPI's runtime 422 body aligned with the OpenAPI Error schema."""
    return JSONResponse(status_code=422, content={"detail": "request validation failed"})


EMBED_MODEL = "text-embedding-3-small"
EMBEDDING_DIMENSIONS = 1536  # one-number: embeddingDimensions
MATCH_REASON_MODEL = os.getenv("MATCH_REASON_MODEL") or "skillhub-match-reason"
SUGGEST_CRITERIA_MODEL = os.getenv("SUGGEST_CRITERIA_MODEL") or "skillhub-suggest-criteria"

# budget-ceiling: app.EMBED_TIMEOUT_SECONDS
EMBED_TIMEOUT_SECONDS = 20.0
# budget-ceiling: app.MATCH_REASONS_TIMEOUT_SECONDS
MATCH_REASONS_TIMEOUT_SECONDS = 12.0
MATCH_REASONS_MAX_TOKENS = 2048
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


GatewayVariable = Literal["LITELLM_BASE_URL", "LITELLM_API_KEY"]


class Readiness(BaseModel):
    status: Literal["ready", "not_ready"]
    gateway_configured: bool
    key_budgeted: bool
    missing: list[GatewayVariable]


@app.get("/readyz", dependencies=[Depends(require_service_token)])
async def readyz() -> Readiness:
    """Readiness: valid service token, configured gateway, and a budgeted key.

    Asks the gateway about this service's own key once; a confirmed key is
    remembered, so later probes do not call the gateway again.
    """
    base_url = os.getenv("LITELLM_BASE_URL", "")
    api_key = os.getenv("LITELLM_API_KEY", "")
    gateway_variables: tuple[tuple[GatewayVariable, str], ...] = (
        ("LITELLM_BASE_URL", base_url),
        ("LITELLM_API_KEY", api_key),
    )
    missing = [name for name, value in gateway_variables if not value]
    budgeted = not missing and await key_is_budgeted(api_key)
    return Readiness(
        status="ready" if budgeted else "not_ready",
        gateway_configured=not missing,
        key_budgeted=budgeted,
        missing=missing,
    )


class EmbedRequest(BaseModel):
    texts: list[str] = Field(..., min_length=1, max_length=64)
    timeout_seconds: float | None = Field(None, gt=0)


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    model: str
    dimensions: int
    usage: GatewayUsage | None = None


@app.post("/embed", response_model=EmbedResponse, dependencies=budgeted)
async def embed(req: EmbedRequest) -> EmbedResponse:
    """Generate embeddings for one or more texts via text-embedding-3-small."""
    client = _client(within(EMBED_TIMEOUT_SECONDS, req.timeout_seconds))

    try:
        raw = await client.embeddings.with_raw_response.create(
            model=EMBED_MODEL,
            input=req.texts,
            extra_body=request_metadata(operation="embed"),
        )
        response = raw.parse()
    except Exception as e:
        logger.warning("embedding call failed (%s)", failure_label(e))
        raise gateway_failure(e) from e

    try:
        vectors = [item.embedding for item in response.data]
        valid = len(vectors) == len(req.texts) and all(
            isinstance(vector, list) and len(vector) == EMBEDDING_DIMENSIONS for vector in vectors
        )
    except AttributeError, KeyError, TypeError:
        valid = False
        vectors = []
    if not valid:
        logger.warning("embedding provider returned a malformed envelope")
        raise HTTPException(status_code=502, detail="embedding provider returned malformed output")
    return EmbedResponse(
        embeddings=vectors,
        model=EMBED_MODEL,
        dimensions=EMBEDDING_DIMENSIONS,
        usage=embedding_usage(response, raw.headers),
    )


CandidateTag = Annotated[str, Field(max_length=200)]  # one-number: skillCandidateTagRunes
MAX_CANDIDATE_TAGS = 8  # one-number: skillCandidateTagsMaxItems


class SkillCandidate(BaseModel):
    skill_id: str
    name: str
    summary: str
    inputs: list[CandidateTag] | None = Field(None, max_length=MAX_CANDIDATE_TAGS)
    outputs: list[CandidateTag] | None = Field(None, max_length=MAX_CANDIDATE_TAGS)


def _candidate_line(c: SkillCandidate) -> str:
    line = f"- [{_scrub(c.skill_id)}] {_scrub(c.name)}: {_scrub(c.summary)}"
    if c.inputs:
        line += f"\n  takes: {_scrub('; '.join(c.inputs))}"
    if c.outputs:
        line += f"\n  produces: {_scrub('; '.join(c.outputs))}"
    return line


class MatchReasonsRequest(BaseModel):
    query: str = Field(..., min_length=1, max_length=2000)
    candidates: list[SkillCandidate] = Field(..., min_length=1, max_length=20)
    timeout_seconds: float | None = Field(None, gt=0)


class MatchReason(BaseModel):
    model_config = ConfigDict(extra="forbid")

    skill_id: str
    reason: str
    fits: bool


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


class _JsonCall(NamedTuple):
    operation: str
    model: str
    schema_name: str
    schema: type[BaseModel]
    max_tokens: int = 1024


async def _ask_for_json(call: _JsonCall, system_prompt: str, user_prompt: str, timeout: float):
    operation, model, schema_name, schema, max_tokens = call
    client = _client(timeout)

    try:
        raw = await client.chat.completions.with_raw_response.create(
            model=model,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            temperature=0.3,
            max_tokens=max_tokens,
            response_format=json_schema_format(schema_name, schema),
            extra_body=request_metadata(operation=operation),
        )
        response = raw.parse()
    except Exception as e:
        logger.warning("%s LLM call failed (%s)", operation, failure_label(e))
        raise gateway_failure(e) from e

    try:
        content = (response.choices[0].message.content or "").strip()
    except AttributeError, IndexError, TypeError:
        logger.warning("%s provider returned a malformed envelope", operation)
        raise HTTPException(
            status_code=502, detail=f"{operation} provider returned malformed output"
        ) from None
    try:
        parsed = schema.model_validate_json(content)
    except ValidationError:
        logger.warning("%s: model output did not match the schema", operation)
        parsed = None
    return response, raw.headers, parsed


@app.post("/match-reasons", response_model=MatchReasonsResponse, dependencies=budgeted)
async def match_reasons(req: MatchReasonsRequest) -> MatchReasonsResponse:
    """Generate human-readable match reasons for search result candidates.

    A candidate this cannot produce a reason for is absent from the response,
    never a fabricated one.
    """
    candidates_text = "\n".join(_candidate_line(c) for c in req.candidates)

    system_prompt = (
        "You are a search result explainer for a Skill marketplace. "
        "Given a user's task description and a list of candidate Skills, "
        "produce a one-sentence match reason of at most 25 words "
        "(or 50 characters in Chinese) for each candidate "
        "explaining why it is relevant to the user's task. "
        "Be specific about which capabilities match which needs. "
        "Set 'fits' to false only when the Skill clearly cannot help with the user's task, "
        "and then say in the reason what it does instead. A Skill that covers part of "
        "the task, or offers a reasonable way to approach it, fits. "
        "When a candidate lists what it takes and produces, judge against those: "
        "a Skill that cannot take what the user has, or cannot produce what the user "
        "wants, does not fit. "
        + data_block_rules(
            DATA_TAG, "the user's own task text and summaries supplied with the packages"
        )
        + ' Respond with a JSON object {"reasons": [...]}, where each entry has '
        "the keys 'skill_id', 'reason' and 'fits'. Use the skill_id exactly as given."
    )

    user_prompt = (
        fence(
            DATA_TAG,
            f"User's task:\n{_scrub(req.query)}\n\nCandidate Skills:\n{candidates_text}",
        )
        + "\n\nProduce the match reasons."
    )

    response, headers, parsed = await _ask_for_json(
        _JsonCall(
            "match-reasons",
            MATCH_REASON_MODEL,
            "match_reasons",
            MatchReasons,
            MATCH_REASONS_MAX_TOKENS,
        ),
        system_prompt,
        user_prompt,
        within(MATCH_REASONS_TIMEOUT_SECONDS, req.timeout_seconds),
    )
    if parsed is None:
        return MatchReasonsResponse(
            reasons=[],
            model=served_model(response, headers, MATCH_REASON_MODEL),
            usage=completion_usage(response, headers),
        )

    wanted = {c.skill_id for c in req.candidates}
    return MatchReasonsResponse(
        reasons=[r for r in parsed.reasons if r.skill_id in wanted and r.reason],
        model=served_model(response, headers, MATCH_REASON_MODEL),
        usage=completion_usage(response, headers),
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
    timeout_seconds: float | None = Field(None, gt=0)


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


@app.post("/suggest-criteria", response_model=SuggestCriteriaResponse, dependencies=budgeted)
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

    response, headers, parsed = await _ask_for_json(
        _JsonCall(
            "suggest-criteria", SUGGEST_CRITERIA_MODEL, "suggested_criteria", SuggestedCriteria
        ),
        system_prompt,
        user_prompt,
        within(SUGGEST_CRITERIA_TIMEOUT_SECONDS, req.timeout_seconds),
    )
    usage = completion_usage(response, headers)
    if parsed is None:
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
