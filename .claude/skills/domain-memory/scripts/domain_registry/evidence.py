from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

from .sources import source_kind_for


def digest(value: bytes) -> str:
    return f"sha256:{hashlib.sha256(value).hexdigest()}"


def source_map_for(registry_root: Path) -> dict[str, Any]:
    path = registry_root / "source-map.json"
    if not path.is_file():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}


def classified(reference: Any, source_map: dict[str, Any]) -> Any:
    if not source_map or not isinstance(reference, dict) or not isinstance(reference.get("path"), str):
        return reference
    return reference | {"source_kind": source_kind_for(source_map, reference["path"])}


def classify_all(value: Any, source_map: dict[str, Any]) -> Any:
    if isinstance(value, dict):
        return {
            key: [classified(item, source_map) for item in child] if key == "evidence" and isinstance(child, list)
            else classify_all(child, source_map)
            for key, child in value.items()
        }
    if isinstance(value, list):
        return [classify_all(child, source_map) for child in value]
    return value


def verify(reference: Any, repo_root: Path) -> dict[str, Any]:
    if isinstance(reference, str):
        return {"status": "legacy-unverified", "reference": reference}
    if not isinstance(reference, dict):
        return {"status": "invalid", "reason": "evidence must be an object"}
    path_text = reference.get("path")
    lines = reference.get("lines")
    if not isinstance(path_text, str) or not isinstance(lines, dict):
        return {"status": "invalid", "reason": "evidence requires path and lines"}
    start, end = lines.get("start"), lines.get("end")
    if not isinstance(start, int) or not isinstance(end, int) or start < 1 or end < start:
        return {"status": "invalid", "reason": "evidence line range is invalid"}
    path = (repo_root / path_text).resolve()
    try:
        path.relative_to(repo_root.resolve())
    except ValueError:
        return {"status": "invalid", "reason": "evidence path escapes repository"}
    if not path.is_file():
        return {"status": "missing", "path": path_text}
    content = path.read_bytes()
    rows = content.decode("utf-8", errors="replace").splitlines(keepends=True)
    if end > len(rows):
        return {"status": "invalid", "reason": "evidence line range exceeds source", "path": path_text}
    excerpt = "".join(rows[start - 1:end]).encode("utf-8")
    if reference.get("content_sha256") != digest(content) or reference.get("excerpt_sha256") != digest(excerpt):
        return {"status": "stale", "path": path_text}
    return {"status": "current", "path": path_text, "source_kind": reference.get("source_kind", "unclassified")}


def migrate_legacy(reference: Any, repo_root: Path) -> Any:
    if not isinstance(reference, str):
        return reference
    path_text, separator, line_text = reference.rpartition(":")
    if not separator or not line_text.isdigit() or int(line_text) < 1:
        return reference
    path = (repo_root / path_text).resolve()
    try:
        path.relative_to(repo_root.resolve())
    except ValueError:
        return reference
    if not path.is_file():
        return reference
    content = path.read_bytes()
    rows = content.decode("utf-8", errors="replace").splitlines(keepends=True)
    line = int(line_text)
    if line > len(rows):
        return reference
    excerpt = rows[line - 1].encode("utf-8")
    return {"path": path_text.replace("\\", "/"), "lines": {"start": line, "end": line}, "content_sha256": digest(content), "excerpt_sha256": digest(excerpt)}


def all_references(value: Any) -> list[Any]:
    if isinstance(value, dict):
        result = []
        for key, child in value.items():
            if key == "evidence" and isinstance(child, list):
                result.extend(child)
            result.extend(all_references(child))
        return result
    if isinstance(value, list):
        return [entry for child in value for entry in all_references(child)]
    return []
