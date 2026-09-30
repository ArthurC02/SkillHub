from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Any

from .common import MAX_EVIDENCE_BYTES, MAX_NESTING_DEPTH, load_json
from .sources import source_kind_for


def digest(value: bytes) -> str:
    return f"sha256:{hashlib.sha256(value).hexdigest()}"


def source_map_for(registry_root: Path) -> dict[str, Any]:
    path = registry_root / "source-map.json"
    if not path.is_file():
        return {}
    return load_json(path)


def _valid_line_range(start: Any, end: Any) -> bool:
    return (
        isinstance(start, int)
        and isinstance(end, int)
        and start >= 1
        and end >= start
    )


def _read_lines(path: Path) -> tuple[bytes, list[str]]:
    content = path.read_bytes()
    return content, content.decode("utf-8", errors="replace").splitlines(keepends=True)


def _excerpt(rows: list[str], start: int, end: int) -> bytes:
    return "".join(rows[start - 1 : end]).encode("utf-8")


def classified(reference: Any, source_map: dict[str, Any]) -> Any:
    if (
        not source_map
        or not isinstance(reference, dict)
        or not isinstance(reference.get("path"), str)
    ):
        return reference
    return reference | {"source_kind": source_kind_for(source_map, reference["path"])}


def classify_all(value: Any, source_map: dict[str, Any], depth: int = 0) -> Any:
    if depth > MAX_NESTING_DEPTH:
        raise ValueError(f"evidence document exceeds nesting depth {MAX_NESTING_DEPTH}")
    if isinstance(value, dict):
        return {
            key: [classified(item, source_map) for item in child]
            if key == "evidence" and isinstance(child, list)
            else classify_all(child, source_map, depth + 1)
            for key, child in value.items()
        }
    if isinstance(value, list):
        return [classify_all(child, source_map, depth + 1) for child in value]
    return value


def citation(repo_root: Path, relative: str, start: int, end: int) -> dict[str, Any]:
    root = repo_root.resolve()
    path = (root / relative).resolve()
    if not path.is_relative_to(root):
        raise ValueError(f"evidence path escapes the repository: {relative}")
    if not path.is_file():
        raise ValueError(f"no such file to cite: {relative}")
    if path.stat().st_size > MAX_EVIDENCE_BYTES:
        raise ValueError(
            f"evidence source exceeds {MAX_EVIDENCE_BYTES} bytes: {relative}"
        )
    if not _valid_line_range(start, end):
        raise ValueError(
            f"a citation needs 1 <= start <= end, got start={start} end={end}"
        )
    content, rows = _read_lines(path)
    if end > len(rows):
        raise ValueError(
            f"{relative} has {len(rows)} lines; the citation asks for line {end}"
        )
    return {
        "path": path.relative_to(root).as_posix(),
        "lines": {"start": start, "end": end},
        "content_sha256": digest(content),
        "excerpt_sha256": digest(_excerpt(rows, start, end)),
    }


def unclassified_paths(value: Any) -> list[str]:
    return sorted(
        {
            reference["path"]
            for reference in all_references(value)
            if isinstance(reference, dict)
            and reference.get("source_kind") == "unclassified"
        }
    )


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
    if not _valid_line_range(start, end):
        return {"status": "invalid", "reason": "evidence line range is invalid"}
    path = (repo_root / path_text).resolve()
    if not path.is_relative_to(repo_root.resolve()):
        return {"status": "invalid", "reason": "evidence path escapes repository"}
    if not path.is_file():
        return {"status": "missing", "path": path_text}
    if path.stat().st_size > MAX_EVIDENCE_BYTES:
        return {
            "status": "invalid",
            "reason": f"evidence source exceeds {MAX_EVIDENCE_BYTES} bytes",
            "path": path_text,
        }
    _, rows = _read_lines(path)
    if end > len(rows):
        return {
            "status": "invalid",
            "reason": "evidence line range exceeds source",
            "path": path_text,
        }
    if reference.get("excerpt_sha256") != digest(_excerpt(rows, start, end)):
        return {"status": "stale", "path": path_text}
    return {
        "status": "current",
        "path": path_text,
        "source_kind": reference.get("source_kind", "unclassified"),
    }


def migrate_legacy(reference: Any, repo_root: Path) -> Any:
    if not isinstance(reference, str):
        return reference
    path_text, separator, line_text = reference.rpartition(":")
    if not separator or not line_text.isdigit() or int(line_text) < 1:
        return reference
    path = (repo_root / path_text).resolve()
    if (
        not path.is_relative_to(repo_root.resolve())
        or not path.is_file()
        or path.stat().st_size > MAX_EVIDENCE_BYTES
    ):
        return reference
    content, rows = _read_lines(path)
    line = int(line_text)
    if line > len(rows):
        return reference
    return {
        "path": path_text.replace("\\", "/"),
        "lines": {"start": line, "end": line},
        "content_sha256": digest(content),
        "excerpt_sha256": digest(_excerpt(rows, line, line)),
    }


def all_references(value: Any, depth: int = 0) -> list[Any]:
    if depth > MAX_NESTING_DEPTH:
        raise ValueError(f"evidence document exceeds nesting depth {MAX_NESTING_DEPTH}")
    if isinstance(value, dict):
        result = []
        for key, child in value.items():
            if key == "evidence" and isinstance(child, list):
                result.extend(child)
            result.extend(all_references(child, depth + 1))
        return result
    if isinstance(value, list):
        return [entry for child in value for entry in all_references(child, depth + 1)]
    return []
