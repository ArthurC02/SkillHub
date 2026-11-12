from types import SimpleNamespace
from unittest.mock import AsyncMock

from openai import APIConnectionError


def _raw_chat(create):
    return SimpleNamespace(
        chat=SimpleNamespace(
            completions=SimpleNamespace(with_raw_response=SimpleNamespace(create=create))
        )
    )


def fake_chat_client(
    content: str,
    capture: list | None = None,
    *,
    usage,
    cost: str,
    headers: dict | None = None,
    finish_reason=None,
    served=None,
    served_header=None,
):
    """Stand-in for AsyncOpenAI whose completion returns `content`.

    The gateway reports cost in a header, never in the body, so the stub
    answers `with_raw_response`. `finish_reason` None means "not truncated".
    """

    async def create(**kwargs):
        if capture is not None:
            capture.append(kwargs)
        choice = SimpleNamespace(
            message=SimpleNamespace(content=content), finish_reason=finish_reason
        )
        completion = SimpleNamespace(choices=[choice], usage=usage)
        if served is not None:
            completion.model = served
        answered = {"x-litellm-response-cost": cost} if headers is None else dict(headers)
        if served_header is not None:
            answered["x-litellm-model-name"] = served_header
        return SimpleNamespace(parse=lambda: completion, headers=answered)

    return _raw_chat(create)


def failing_chat_client(*_args):
    return _raw_chat(AsyncMock(side_effect=APIConnectionError(request=None)))
