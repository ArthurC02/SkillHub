from __future__ import annotations

import asyncio
from collections.abc import Coroutine
from typing import Any

from fastapi import HTTPException, Request
from starlette.types import ASGIApp, Message, Receive, Scope, Send


async def answer_unless_disconnected[T](
    request: Request,
    work: Coroutine[Any, Any, T],
    *,
    timeout_seconds: float,
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


class _CallerWire:
    def __init__(self, receive: Receive) -> None:
        self._receive = receive
        self.body_read = asyncio.Event()
        self.left = asyncio.Event()

    async def receive(self) -> Message:
        if self.body_read.is_set():
            await self.left.wait()
            return {"type": "http.disconnect"}
        message = await self._receive()
        if message["type"] == "http.disconnect":
            self.left.set()
        elif not message.get("more_body", False):
            self.body_read.set()
        return message

    async def watch(self) -> None:
        # After the body, the only message left on the wire is the disconnect.
        await self.body_read.wait()
        while (await self._receive())["type"] != "http.disconnect":
            pass
        self.left.set()


class CancelWhenCallerLeaves:
    def __init__(self, app: ASGIApp) -> None:
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        wire = _CallerWire(receive)
        handler = asyncio.ensure_future(self.app(scope, wire.receive, send))
        watcher = asyncio.ensure_future(wire.watch())
        leaving = asyncio.ensure_future(wire.left.wait())
        try:
            await asyncio.wait({handler, leaving}, return_when=asyncio.FIRST_COMPLETED)
        finally:
            for task in (handler, watcher, leaving):
                if not task.done():
                    task.cancel()
            await asyncio.gather(handler, watcher, leaving, return_exceptions=True)
        if not handler.cancelled():
            handler.result()
