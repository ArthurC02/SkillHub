from __future__ import annotations

import asyncio
from collections.abc import Coroutine
from typing import Any

from fastapi import HTTPException, Request


async def answer_unless_disconnected[T](
    request: Request,
    work: Coroutine[Any, Any, T],
    *,
    timeout_seconds: float | None,
    disconnected_detail: str,
    timeout_detail: str,
) -> T:
    finished = asyncio.Event()

    async def disconnected() -> None:
        # is_disconnected() can swallow a cancellation, so the loop also watches a flag.
        while not finished.is_set() and not await request.is_disconnected():
            await asyncio.sleep(0.1)

    step = asyncio.create_task(work)
    disconnect = asyncio.create_task(disconnected())
    try:
        async with asyncio.timeout(timeout_seconds):
            done, _ = await asyncio.wait({step, disconnect}, return_when=asyncio.FIRST_COMPLETED)
            if step not in done:
                raise HTTPException(status_code=499, detail=disconnected_detail)
            return step.result()
    except TimeoutError:
        raise HTTPException(status_code=502, detail=timeout_detail) from None
    finally:
        finished.set()
        for task in (step, disconnect):
            if not task.done():
                task.cancel()
        await asyncio.gather(step, disconnect, return_exceptions=True)
