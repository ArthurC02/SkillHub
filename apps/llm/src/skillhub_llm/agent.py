"""One step of a platform agent run: the next tool intent, or the final result.

Go owns the run, which tools exist, executing them and recording every step;
this module only turns the steps so far into the model's next decision.
"""

from __future__ import annotations

import asyncio
import json
import os
from dataclasses import dataclass
from typing import Literal

from fastapi import APIRouter, Header, HTTPException, Request
from openai import APIError
from pydantic import BaseModel, ConfigDict, Field

from skillhub_llm.gateway import GatewayUsage, _metadata, _usage, client, served_model
from skillhub_llm.untrusted import data_block_rules, fence, scrub

router = APIRouter()

FINISH_TOOL = "finish"
RESULT_TAG = "tool_result"


@dataclass(frozen=True)
class AgentInstructions:
    system: str
    result_schema: dict
    prompt_version: str


DAILY_REPORT = AgentInstructions(
    system=(
        "You write the platform's daily maintenance report for its operators. "
        "Call `maintenance_report` once to read today's facts, then finish with a short report "
        "in Traditional Chinese, plain words, no jargon. "
        "Each item is either `fine` or `attention`: attention means a prediction crosses the "
        "restore budget within 90 days, a scheduled job has gone more than two periods without "
        "succeeding, or the restore rate is still an unmeasured default. "
        "Every item cites the facts it rests on as JSON Pointers into the tool's answer, such as "
        "`/capacity/days_until_budget` or `/maintenance_jobs/purge-audit/overdue_ratio`. "
        "Cite only facts the answer contains; a report citing anything else is rejected. "
        "When a job is attention because it has gone more than two periods without succeeding and "
        "its facts carry an `action`, you may add one proposal for that action: name it exactly, "
        "say why in one plain sentence, and cite the facts it rests on. An operator decides every "
        "proposal; propose nothing else, and nothing for a job without an `action`."
    ),
    result_schema={
        "type": "object",
        "additionalProperties": False,
        "required": ["items"],
        "properties": {
            "items": {
                "type": "array",
                "minItems": 1,
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["status", "text", "cites"],
                    "properties": {
                        "status": {"type": "string", "enum": ["fine", "attention"]},
                        "text": {"type": "string", "minLength": 1},
                        "cites": {
                            "type": "array",
                            "minItems": 1,
                            "items": {"type": "string", "pattern": "^/"},
                        },
                    },
                },
            },
            "proposals": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["action", "reason", "cites"],
                    "properties": {
                        "action": {"type": "string", "minLength": 1},
                        "reason": {"type": "string", "minLength": 1},
                        "cites": {
                            "type": "array",
                            "minItems": 1,
                            "items": {"type": "string", "pattern": "^/"},
                        },
                    },
                },
            },
        },
    },
    prompt_version="daily-report-v2",
)

INSTRUCTIONS: dict[str, AgentInstructions] = {"daily-report": DAILY_REPORT}


class AgentTool(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str = Field(..., pattern=r"^[a-z][a-z0-9_]*$")
    description: str = Field(..., max_length=1000)
    parameters: dict


class AgentStepRecord(BaseModel):
    model_config = ConfigDict(extra="forbid")

    tool: str
    arguments: str = Field(..., max_length=4000)
    result: str = Field(..., max_length=60000)


class AgentStepRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent: str = Field(..., min_length=1)
    run_id: str = Field(..., min_length=1)
    model_role: str = Field(..., min_length=1)
    tools: list[AgentTool] = Field(..., max_length=10)
    steps: list[AgentStepRecord] = Field(..., max_length=20)
    timeout_seconds: int = Field(..., ge=1, le=120)
    max_output_tokens: int = Field(..., ge=1, le=16000)


class AgentToolIntent(BaseModel):
    model_config = ConfigDict(extra="forbid")

    tool: str
    arguments: str = Field(..., max_length=4000)


class AgentStepResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    outcome: Literal["tool_intent", "final"]
    tool_intent: AgentToolIntent | None = None
    result: str | None = Field(None, max_length=60000)
    model: str
    prompt_version: str
    usage: GatewayUsage | None = None


def _system(instructions: AgentInstructions) -> str:
    return (
        f"{instructions.system}\n\n"
        f"Every tool answer arrives inside a <{RESULT_TAG}> block. "
        + data_block_rules(RESULT_TAG, "facts the platform returned for your tool call")
        + f"\n\nWhen you have what you need, call `{FINISH_TOOL}` with your result. "
        "Call exactly one tool per turn."
    )


def _messages(req: AgentStepRequest, instructions: AgentInstructions) -> list[dict]:
    messages: list[dict] = [
        {"role": "system", "content": _system(instructions)},
        {"role": "user", "content": "Begin."},
    ]
    for index, step in enumerate(req.steps):
        call_id = f"call_{index}"
        messages.append(
            {
                "role": "assistant",
                "content": None,
                "tool_calls": [
                    {
                        "id": call_id,
                        "type": "function",
                        "function": {"name": step.tool, "arguments": step.arguments},
                    }
                ],
            }
        )
        messages.append(
            {
                "role": "tool",
                "tool_call_id": call_id,
                "content": fence(RESULT_TAG, scrub(RESULT_TAG, step.result)),
            }
        )
    return messages


def _tools(req: AgentStepRequest, instructions: AgentInstructions) -> list[dict]:
    offered = [
        {
            "type": "function",
            "function": {
                "name": tool.name,
                "description": tool.description,
                "parameters": tool.parameters,
            },
        }
        for tool in req.tools
    ]
    offered.append(
        {
            "type": "function",
            "function": {
                "name": FINISH_TOOL,
                "description": "Submit the run's final result.",
                "parameters": instructions.result_schema,
            },
        }
    )
    return offered


def decide(req: AgentStepRequest, completion) -> AgentToolIntent | str:
    """The model's one tool call as an intent, or its finish arguments as the result."""
    choices = getattr(completion, "choices", None) or []
    calls = getattr(choices[0].message, "tool_calls", None) if choices else None
    if not calls:
        raise HTTPException(status_code=502, detail="the model called no tool")
    call = calls[0].function
    try:
        json.loads(call.arguments)
    except TypeError, ValueError:
        raise HTTPException(
            status_code=502, detail="the model's tool arguments are not JSON"
        ) from None
    if call.name == FINISH_TOOL:
        return call.arguments
    if call.name not in {tool.name for tool in req.tools}:
        raise HTTPException(status_code=502, detail="the model called a tool it was not offered")
    return AgentToolIntent(tool=call.name, arguments=call.arguments)


async def _step(
    req: AgentStepRequest, instructions: AgentInstructions, gateway_key: str
) -> AgentStepResponse:
    try:
        raw = await (
            client(req.timeout_seconds)
            .with_options(api_key=gateway_key)
            .chat.completions.with_raw_response.create(
                model=req.model_role,
                messages=_messages(req, instructions),
                tools=_tools(req, instructions),
                tool_choice="required",
                max_tokens=req.max_output_tokens,
                extra_body=_metadata(operation=f"agent:{req.agent}", run_id=req.run_id),
            )
        )
    except APIError as error:
        raise HTTPException(status_code=502, detail="model gateway call failed") from error
    completion = raw.parse()
    decision = decide(req, completion)
    common = {
        "model": served_model(completion, raw.headers, req.model_role),
        "prompt_version": instructions.prompt_version,
        "usage": _usage(completion, raw.headers),
    }
    if isinstance(decision, AgentToolIntent):
        return AgentStepResponse(outcome="tool_intent", tool_intent=decision, **common)
    return AgentStepResponse(outcome="final", result=decision, **common)


@router.post("/v1/agent/step", response_model=AgentStepResponse)
async def agent_step(
    req: AgentStepRequest,
    request: Request,
    x_agent_gateway_key: str | None = Header(default=None),
) -> AgentStepResponse:
    if not x_agent_gateway_key or x_agent_gateway_key == os.getenv("LITELLM_MASTER_KEY"):
        raise HTTPException(
            status_code=503, detail="an agent step requires a scoped gateway Virtual Key"
        )

    instructions = INSTRUCTIONS.get(req.agent)
    if instructions is None:
        raise HTTPException(status_code=422, detail=f"no instructions for agent {req.agent!r}")

    finished = asyncio.Event()

    async def disconnected():
        # is_disconnected() can swallow a cancellation, so the loop also watches a flag.
        while not finished.is_set() and not await request.is_disconnected():
            await asyncio.sleep(0.1)

    work = asyncio.create_task(_step(req, instructions, x_agent_gateway_key))
    disconnect = asyncio.create_task(disconnected())
    try:
        async with asyncio.timeout(req.timeout_seconds):
            done, _ = await asyncio.wait({work, disconnect}, return_when=asyncio.FIRST_COMPLETED)
            if work not in done:
                raise HTTPException(status_code=499, detail="agent step request disconnected")
            return work.result()
    except TimeoutError:
        raise HTTPException(status_code=502, detail="agent step timed out") from None
    finally:
        finished.set()
        for task in (work, disconnect):
            if not task.done():
                task.cancel()
        await asyncio.gather(work, disconnect, return_exceptions=True)
