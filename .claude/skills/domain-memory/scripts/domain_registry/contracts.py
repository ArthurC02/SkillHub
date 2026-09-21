from __future__ import annotations

from pathlib import Path
from typing import Any

from .common import load_json


def validate_schema(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {"status": "missing", "path": str(path)}
    if path.suffix.lower() not in {".json"}:
        return {
            "status": "unverified",
            "path": str(path),
            "reason": "only JSON schemas are parsed locally",
        }
    try:
        value = load_json(path)
    except ValueError:
        return {"status": "invalid", "path": str(path), "reason": "invalid JSON"}
    if not isinstance(value, dict):
        return {
            "status": "invalid",
            "path": str(path),
            "reason": "schema must be an object",
        }
    if "openapi" in value:
        return {"status": "valid", "kind": "openapi", "path": str(path)}
    if "$schema" in value or "type" in value or "properties" in value:
        return {"status": "valid", "kind": "json-schema", "path": str(path)}
    return {
        "status": "invalid",
        "path": str(path),
        "reason": "unrecognized JSON schema",
    }
