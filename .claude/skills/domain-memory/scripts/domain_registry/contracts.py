from __future__ import annotations

import json
from pathlib import Path
from typing import Any


def validate_schema(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {"status": "missing", "path": str(path)}
    if path.suffix.lower() not in {".json"}:
        return {"status": "unverified", "path": str(path), "reason": "only JSON schemas are parsed locally"}
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {"status": "invalid", "path": str(path), "reason": "invalid JSON"}
    if not isinstance(value, dict):
        return {"status": "invalid", "path": str(path), "reason": "schema must be an object"}
    if "openapi" in value:
        return {"status": "valid", "kind": "openapi", "path": str(path)}
    if "$schema" in value or "type" in value or "properties" in value:
        return {"status": "valid", "kind": "json-schema", "path": str(path)}
    return {"status": "invalid", "path": str(path), "reason": "unrecognized JSON schema"}
