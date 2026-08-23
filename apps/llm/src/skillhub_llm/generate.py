"""POST /v1/generate-skill - a task description in, a Skill package out (GEN-001/002).

A capability provider and nothing else: no product authorisation, no writes, no
idea what a Skill Version is. Go packages what comes back, runs it through
admission's one validation path, and writes a version only if the bytes pass
(Iron Rule 6, 02:GEN-003). A 200 here is not a package that exists yet.

Single-shot. If a "generate, validate, retry" loop is ever needed it belongs in
Go, which already owns the zip, `skillpkg.Validate` and the retry policy; this
module would not change (ADR-046 決策 3).

**Why this returns typed frontmatter instead of a SKILL.md string.** Measured
across 59 generated packages in three rounds (m5/report-generate-spike.md and
report-generate-baseline.md), every blocking failure was the model damaging a
frontmatter *key*: a leading space before `description`, an Arabic letter glued
to it, `说明: invalid` in its place. Six of twenty task descriptions failed that
way at least once. A key the model never writes cannot be damaged, so Go
serialises the YAML and `frontmatter-invalid-yaml` and `frontmatter-unknown-field`
stop being possible rather than merely unlikely - four of the six observed
failures. `description-missing` is not in that list: it was, back when this
module put `min_length=1` on the field, and strict `json_schema` does not accept
that keyword. It is `skillpkg.Validate`'s to catch, which is where 02:GEN-003
wants it anyway - a blocking finding reaches the user verbatim, a 502 from here
reaches them as "generation failed".

`license` is absent from GeneratedSkill and `extra="forbid"` keeps it that way.
ADR-046 決策 5: a model-written licence string must not occupy the "已宣告"
state, which means a person declared it. A prohibition that lives in a prompt
holds until the next prompt revision.
"""

from __future__ import annotations

import logging
import os

from fastapi import APIRouter, HTTPException
from openai import OpenAIError
from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator

from skillhub_llm.evaluate import GatewayUsage, _client, _clip, _metadata, _usage

logger = logging.getLogger("skillhub_llm.generate")

router = APIRouter()

# ADR-051: mini is the default because it measured better here, not because it
# is cheaper and good enough. 19/20 against flagship's 16/20 at 1/21 the cost -
# every failure lands on a frontmatter key and mini writes 3.5x fewer tokens,
# so it has less surface to damage. It also wrote no scripts in twenty
# attempts; whether that is better is GEN-009 and is not known (ADR-051 決策 2).
GENERATE_SKILL_MODEL = os.getenv("GENERATE_SKILL_MODEL", "gpt-5.4-mini")
GENERATE_SKILL_PROMPT_VERSION = "generate-skill/v1"

# ADR-047 決策 2, corrected by the B round: the cap covers reasoning *plus*
# output, and reasoning varies far more than output does. The one round-A
# failure spent all 8000 of its tokens reasoning and emitted an empty string,
# then succeeded at 16000 having used 7552 in total.
MAX_OUTPUT_TOKENS = 16000  # one-number: generateMaxOutputTokens

# Contract caps (llm-internal.yaml). They cannot be expressed in the schema the
# model is given - strict `json_schema` rejects `maxLength` and `maxItems` - so
# they are applied to the answer instead, the way evaluate.py already does it.
MAX_EXTRA_FILES = 10
MAX_FILE_BYTES = 100_000
MAX_BODY_CHARS = 60_000
MAX_NAME_CHARS = 64
MAX_DESCRIPTION_CHARS = 1024
MAX_COMPATIBILITY_CHARS = 500
MAX_PATH_CHARS = 255


class GeneratedFile(BaseModel):
    """One package file besides SKILL.md.

    `..` is not checked here. It is a blocking finding read off the raw zip
    entry names before `fs.Sub` rewrites them (`entry-path-escape`, 04 丙-15),
    which is both earlier and harder to bypass than a regex on this side.
    """

    model_config = ConfigDict(extra="forbid")

    path: str
    content: str


class GeneratedSkill(BaseModel):
    """The frontmatter as fields plus the body. Also the schema handed to the
    model, so the wire shape and the shape the prompt asks for cannot drift
    apart (same rule as SuggestCriteriaResponse).

    **No `Field(...)` constraints and no defaults, and that is load-bearing.**
    Strict `json_schema` rejects `maxLength`/`minLength`/`pattern`/`maxItems`
    and requires every property to appear in `required` - a default takes a
    property out of `required`, which is why the four sibling schemas have
    none either. This class had four defaults, a `dict[str, str]` (whose
    `additionalProperties: {"type": "string"}` strict also refuses) and six
    length constraints, so the gateway answered 400 to every call this endpoint
    ever made. Nothing caught it: all seven tests monkeypatch the client, and
    the measured rounds called the gateway from a spike rather than through
    here. `tests/test_strict_schemas.py` is now the thing that would.

    The caps live below, applied to the answer. The two rules that were doing
    real work - a well-formed `name` and a non-empty `description` - are
    `skillpkg.Validate`'s already, and it hands the user the verbatim finding
    (02:GEN-003) where a 502 from here would say only that generation failed.
    """

    model_config = ConfigDict(extra="forbid")

    name: str
    description: str
    compatibility: str
    # A single string, not a list: the specification defines it that way and the
    # validator warns on a YAML list.
    allowed_tools: str
    body: str
    files: list[GeneratedFile]


class GenerateSkillRequest(BaseModel):
    task_description: str = Field(..., min_length=8, max_length=4000)  # one-number: generateMaxTaskRunes

    @field_validator("task_description")
    @classmethod
    def _not_only_whitespace(cls, v: str) -> str:
        """`min_length` counts characters, so ten spaces would clear it and buy
        a paid gateway call that can only fail. Go refuses blank input before
        this point (02:GEN-001); this is the backstop, not the product rule.
        """
        if not v.strip():
            raise ValueError("task_description must not be only whitespace")
        return v


class GenerateSkillResponse(BaseModel):
    skill: GeneratedSkill
    model: str
    prompt_version: str
    usage: GatewayUsage | None = None


SYSTEM_PROMPT = """You write one Agent Skill from a description of a task.

A Skill is a set of instructions an AI agent loads and follows when it recognises
the task. You are writing those instructions, for an agent, not documentation for
a person.

Return the frontmatter as fields and the instructions as `body`. Do not write YAML
front matter yourself; do not write `---` delimiters; do not include a licence.

- `name`: lowercase letters, digits and single hyphens, at most 64 characters.
- `description`: what the skill does AND when to use it, in one or two sentences.
  This is the only thing an agent sees when deciding whether to load the skill, so
  say the trigger, not just the capability.
- `compatibility`: environment requirements, only if there are real ones.
- `allowed_tools`: a single space-separated string, only if the skill needs
  specific tools.
- `body`: the actual instructions, in Markdown. Concrete steps a competent agent
  can follow. No placeholders for someone to fill in later, no "TODO", no
  "insert X here" - if you do not know a value, write instructions for finding it.
- `files`: only when a script genuinely does the work better than instructions.
  Prefer instructions.

Write in the language of the task description.

If the task is not something a Skill can do - it needs live network access, a
purchase, a physical action, or a login you were not given - say so plainly in
`body` and keep the skill to what an agent CAN do: the checklist, the questions
to ask, the information to gather."""


@router.post("/v1/generate-skill", response_model=GenerateSkillResponse)
async def generate_skill(req: GenerateSkillRequest) -> GenerateSkillResponse:
    """Generate one Skill package (02:GEN-001).

    No existing Skill Version is an input. Starting from one is improvement and
    belongs to /suggest-improvements - keeping the two apart at the input is
    what stops one endpoint carrying two meanings for "evidence", since from
    nothing there is no excerpt to verify a citation against (ADR-046 決策 3).
    """
    try:
        raw = await _client().chat.completions.with_raw_response.create(
            model=GENERATE_SKILL_MODEL,
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": req.task_description},
            ],
            max_tokens=MAX_OUTPUT_TOKENS,
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "generated_skill",
                    "strict": True,
                    "schema": GeneratedSkill.model_json_schema(),
                },
            },
            extra_body=_metadata(operation="generate-skill"),
        )
        completion = raw.parse()
    except OpenAIError as e:
        logger.exception("generate-skill: gateway call failed")
        raise HTTPException(status_code=502, detail=f"generate gateway error: {e}") from e

    # Truncation is a different failure from a malformed answer and must not be
    # retried at the same cap: the cap covers reasoning plus output, so a second
    # call buys the same answer (ADR-047 決策 2). Go decides what to do; this
    # side only has to make the two distinguishable.
    finish = getattr(completion.choices[0], "finish_reason", None) if completion.choices else None
    if finish == "length":
        logger.warning("generate-skill: output hit the token ceiling")
        raise HTTPException(
            status_code=502,
            detail="generate model output was truncated at the token ceiling",
        )

    try:
        skill = GeneratedSkill.model_validate_json(completion.choices[0].message.content or "")
    except (ValidationError, IndexError, AttributeError) as e:
        # Never echoed back: model output may carry injected content, and this
        # one is a whole package. Go records a failed generation rather than
        # writing a version from something that did not parse.
        logger.warning("generate-skill: model returned unusable output")
        raise HTTPException(
            status_code=502, detail="generate model returned malformed output"
        ) from e

    # An empty body is refused rather than clipped, and it is the one answer-side
    # rule that is not a cap: the B round produced a 38-character SKILL.md with
    # no body at all, blocked only because its key was damaged too. Had the key
    # been right, a syntactically perfect package with nothing in it would have
    # passed every check skillpkg has.
    if not skill.body.strip():
        logger.warning("generate-skill: model returned an empty body")
        raise HTTPException(
            status_code=502, detail="generate model returned malformed output"
        )

    skill.name = _clip(skill.name, MAX_NAME_CHARS)
    skill.description = _clip(skill.description, MAX_DESCRIPTION_CHARS)
    skill.compatibility = _clip(skill.compatibility, MAX_COMPATIBILITY_CHARS)
    skill.body = _clip(skill.body, MAX_BODY_CHARS)
    skill.files = skill.files[:MAX_EXTRA_FILES]
    for f in skill.files:
        f.path = _clip(f.path, MAX_PATH_CHARS)
        f.content = _clip(f.content, MAX_FILE_BYTES)

    return GenerateSkillResponse(
        skill=skill,
        model=GENERATE_SKILL_MODEL,
        prompt_version=GENERATE_SKILL_PROMPT_VERSION,
        usage=_usage(completion, raw.headers),
    )
