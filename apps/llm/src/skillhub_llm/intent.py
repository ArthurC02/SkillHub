from __future__ import annotations

import os
from typing import Annotated, Literal

from fastapi import APIRouter, HTTPException
from openai import OpenAIError
from pydantic import BaseModel, ConfigDict, Field, StringConstraints, ValidationError

from skillhub_llm.gateway import GatewayUsage, _metadata, _usage, client, within
from skillhub_llm.untrusted import data_block_rules, fence, scrub

router = APIRouter()
INTENT_MODEL = os.getenv("INTENT_MODEL") or "gpt-5.6-luna"
PROMPT_VERSION = "search-intent/v2"
# budget-ceiling: intent.TIMEOUT_SECONDS
TIMEOUT_SECONDS = 8.0
MAX_OUTPUT_TOKENS = 1600
DATA_TAG = "untrusted_search_query"
MAX_QUERY_RUNES = 2000  # one-number: searchMaxQueryRunes
MAX_KEYWORD_RUNES = 128  # one-number: intentMaxKeywordRunes
IntentText = Annotated[
    str, StringConstraints(min_length=1, max_length=MAX_QUERY_RUNES, pattern=r"\S")
]
Keyword = Annotated[
    str, StringConstraints(min_length=1, max_length=MAX_KEYWORD_RUNES, pattern=r"\S")
]


class AnalyzeSearchIntentRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    query: IntentText
    timeout_seconds: float = Field(gt=0, allow_inf_nan=False)


class ProposedIntent(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    input: str | None
    output: str | None
    tools: str | None
    data: str | None
    environment: str | None


class SearchIntent(ProposedIntent):
    input: IntentText | None
    output: IntentText | None
    tools: IntentText | None
    data: IntentText | None
    environment: IntentText | None


class ProposedFilters(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    script: Literal["yes", "no"] | None
    validation: Literal["passed", "unverified"] | None
    agent: Literal["native", "transpiled", "failed", "unverified"] | None
    tier: Literal["curated", "indexed"] | None
    category: Literal["documents", "writing", "data"] | None


class SearchFilters(ProposedFilters):
    script: Literal["yes", "no"] | None = None
    validation: Literal["passed", "unverified"] | None = None
    agent: Literal["native", "transpiled", "failed", "unverified"] | None = None
    tier: Literal["curated", "indexed"] | None = None
    category: Literal["documents", "writing", "data"] | None = None


class IntentProposal(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    intent: ProposedIntent
    keywords: list[str]
    filters: ProposedFilters


class ValidatedIntentProposal(IntentProposal):
    intent: SearchIntent
    keywords: list[Keyword] = Field(max_length=8)  # one-number: intentMaxKeywords


class AnalyzeSearchIntentResponse(BaseModel):
    valid: bool
    model: str
    prompt_version: str
    intent: SearchIntent | None = None
    keywords: list[Keyword] | None = None
    filters: SearchFilters | None = None
    usage: GatewayUsage | None = None


SYSTEM_PROMPT = (
    data_block_rules(DATA_TAG, "a user's task description")
    + """
Extract only explicitly mentioned input, output, tools, data and environment.
Each non-null intent field must be an exact, contiguous quote from the query.
Use null for anything not mentioned; never infer tools, formats or prerequisites.
Propose concise retrieval terms in the query's language, preserving named tools
and formats. Remove conversational filler, pronouns and incidental circumstances.
Keep distinct task concepts as separate keywords, not entire request clauses.
Filters restrict which skills may appear; they do not describe the task.
Default EVERY filter to null. Set a filter only when the user explicitly limits
the returned skills by that catalog facet. Never classify a task into a category.
For example, converting a spreadsheet, making slides or editing a document does
not request any category filter. Mentioning code does not request script=yes.
"Only verified skills" requests validation=passed; "only skills in the writing
category" requests category=writing. Without such a restriction keep null.
Intent input is the supplied material; output is the requested deliverable;
tools are explicitly named tools; data is explicitly described data; environment
is an explicitly stated execution environment, not an action or desired result.
Do not answer the task, execute it, browse, or request credentials.
"""
)


@router.post(
    "/v1/analyze-intent",
    response_model=AnalyzeSearchIntentResponse,
    response_model_exclude_unset=True,
)
async def analyze_intent(req: AnalyzeSearchIntentRequest) -> AnalyzeSearchIntentResponse:
    gateway = client(within(TIMEOUT_SECONDS, req.timeout_seconds))
    try:
        raw = await gateway.chat.completions.with_raw_response.create(
            model=INTENT_MODEL,
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": fence(DATA_TAG, scrub(DATA_TAG, req.query))},
            ],
            max_completion_tokens=MAX_OUTPUT_TOKENS,
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "search_intent",
                    "strict": True,
                    "schema": IntentProposal.model_json_schema(),
                },
            },
            extra_body=_metadata(operation="analyze-intent", prompt_version=PROMPT_VERSION),
        )
        completion = raw.parse()
    except OpenAIError as error:
        raise HTTPException(status_code=502, detail="gateway error") from error

    response = AnalyzeSearchIntentResponse(
        valid=False, model=INTENT_MODEL, prompt_version=PROMPT_VERSION
    )
    usage = _usage(completion, raw.headers)
    if usage is not None:
        response.usage = usage
    try:
        choice = completion.choices[0]
        if choice.finish_reason != "stop" or choice.message.refusal is not None:
            return response
        proposal = ValidatedIntentProposal.model_validate_json(choice.message.content or "")
        if any(
            value is not None and value not in req.query
            for value in proposal.intent.model_dump().values()
        ):
            return response
    except ValidationError, IndexError, AttributeError, TypeError:
        return response
    response.valid = True
    response.intent = proposal.intent
    response.keywords = proposal.keywords
    response.filters = SearchFilters.model_validate(proposal.filters.model_dump(exclude_none=True))
    return response
