"""The one place the LiteLLM gateway's address and key are read (Iron Rule 8).

Three call sites used to read them independently and disagree about what an
unconfigured process should do. `enrich` refused with 503. `/embed`,
`/match-reasons` and `/suggest-criteria` carried module-level defaults —
`http://localhost:4000` and the literal dev key `sk-1234` — captured at import
time, so an unconfigured deployment did not report itself as unconfigured: it
produced a 502 that Go cannot tell from a provider outage, and Go's answer to a
provider outage is to degrade search to FTS-only, quietly and for as long as the
misconfiguration lasts (M1 audit, 2026-08-24).

Read at call time, never at import, so a missing variable is a 503 on the
request rather than a crash at startup.
"""

from __future__ import annotations

import os

from fastapi import HTTPException


def gateway() -> tuple[str, str]:
    """Base URL and key for the LiteLLM gateway, or 503 if either is missing.

    No provider fallback and no default address. Reaching a provider directly is
    the thing Iron Rule 8 forbids, and a default address is how a process reaches
    one it was never pointed at.
    """
    base_url = os.getenv("LITELLM_BASE_URL")
    api_key = os.getenv("LITELLM_API_KEY")
    if not base_url or not api_key:
        raise HTTPException(
            status_code=503,
            detail="LiteLLM gateway not configured: set LITELLM_BASE_URL and LITELLM_API_KEY",
        )
    return base_url, api_key
