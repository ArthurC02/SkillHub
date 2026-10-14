from __future__ import annotations

import hashlib
from collections.abc import Callable
from pathlib import Path
from typing import Any

from .common import MAX_EVIDENCE_BYTES, MAX_NESTING_DEPTH, load_json
from .sources import source_kind_for


WIDEN_TO_UNIQUE = "Widen the citation to the enclosing block, so its lines occur once in the file."


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


def with_references(value: Any, change: Callable[[Any], Any], depth: int = 0) -> Any:
    if depth > MAX_NESTING_DEPTH:
        raise ValueError(f"evidence document exceeds nesting depth {MAX_NESTING_DEPTH}")
    if isinstance(value, dict):
        return {
            key: [change(item) for item in child]
            if key == "evidence" and isinstance(child, list)
            else with_references(child, change, depth + 1)
            for key, child in value.items()
        }
    if isinstance(value, list):
        return [with_references(child, change, depth + 1) for child in value]
    return value


def classify_all(value: Any, source_map: dict[str, Any]) -> Any:
    return with_references(value, lambda reference: classified(reference, source_map))


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


def excerpt_starts(repo_root: Path, relative: str, length: int, excerpt_sha256: str) -> list[int]:
    path = (repo_root / relative).resolve()
    if _source_verdict(path, relative, repo_root):
        return []
    _, rows = _read_lines(path)
    return _starts_of(rows, length, excerpt_sha256)


def repeated_note(repo_root: Path, cited: dict[str, Any]) -> str | None:
    start, end = cited["lines"]["start"], cited["lines"]["end"]
    others = [
        line
        for line in excerpt_starts(repo_root, cited["path"], end - start + 1, cited["excerpt_sha256"])
        if line != start
    ]
    if not others:
        return None
    return (
        f"NOTE: the text of {cited['path']}:{start}-{end} also starts at line "
        f"{', '.join(map(str, others))}, so a later edit cannot tell the copies apart. "
        f"{WIDEN_TO_UNIQUE}"
    )


def hash_all(value: Any, repo_root: Path) -> Any:
    return with_references(value, lambda reference: _hashed(reference, repo_root))


def _hashed(reference: Any, repo_root: Path) -> Any:
    if _shape_error(reference) or "excerpt_sha256" in reference:
        return reference
    lines = reference["lines"]
    return {**reference, **citation(repo_root, reference["path"], lines["start"], lines["end"])}


def unclassified_paths(value: Any) -> list[str]:
    return sorted(
        {
            reference["path"]
            for reference in all_references(value)
            if isinstance(reference, dict)
            and reference.get("source_kind") == "unclassified"
        }
    )


def _shape_error(reference: Any) -> str | None:
    if not isinstance(reference, dict):
        return "evidence must be an object"
    lines = reference.get("lines")
    if not isinstance(reference.get("path"), str) or not isinstance(lines, dict):
        return "evidence requires path and lines"
    if not _valid_line_range(lines.get("start"), lines.get("end")):
        return "evidence line range is invalid"
    return None


def _source_verdict(path: Path, path_text: str, repo_root: Path) -> dict[str, Any] | None:
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
    return None


def verify(reference: Any, repo_root: Path) -> dict[str, Any]:
    if isinstance(reference, str):
        return {"status": "legacy-unverified", "reference": reference}
    shape = _shape_error(reference)
    if shape:
        return {"status": "invalid", "reason": shape}
    path_text = reference["path"]
    path = (repo_root / path_text).resolve()
    problem = _source_verdict(path, path_text, repo_root)
    if problem:
        return problem
    _, rows = _read_lines(path)
    return {"cited_lines": dict(reference["lines"]), **_cited_lines_verdict(reference, path_text, rows)}


def _cited_lines_verdict(reference: dict[str, Any], path_text: str, rows: list[str]) -> dict[str, Any]:
    start, end = reference["lines"]["start"], reference["lines"]["end"]
    if end > len(rows) and "excerpt_sha256" in reference:
        return _moved_or_stale(reference["excerpt_sha256"], path_text, rows, end - start + 1)
    if end > len(rows):
        return {
            "status": "invalid",
            "reason": "evidence line range exceeds source",
            "path": path_text,
        }
    if "excerpt_sha256" not in reference:
        return {
            "status": "unhashed",
            "path": path_text,
            "reason": f"{path_text} was cited without an excerpt hash; regenerate the citation with cite",
        }
    if reference.get("excerpt_sha256") != digest(_excerpt(rows, start, end)):
        return _moved_or_stale(reference["excerpt_sha256"], path_text, rows, end - start + 1)
    return {
        "status": "current",
        "path": path_text,
        "source_kind": reference.get("source_kind", "unclassified"),
    }


def _starts_of(rows: list[str], length: int, excerpt_sha256: str) -> list[int]:
    return [
        start
        for start in range(1, len(rows) - length + 2)
        if digest(_excerpt(rows, start, start + length - 1)) == excerpt_sha256
    ]


def _moved_or_stale(excerpt_sha256: str, path_text: str, rows: list[str], length: int) -> dict[str, Any]:
    starts = _starts_of(rows, length, excerpt_sha256)
    if len(starts) != 1:
        return {
            "status": "stale",
            "path": path_text,
            "reason": f"the cited excerpt is {'repeated' if starts else 'no longer'} in {path_text}; "
            "check that the fact still holds, then recite it" + (f". {WIDEN_TO_UNIQUE}" if starts else ""),
        }
    return {
        "status": "shifted",
        "path": path_text,
        "lines": {"start": starts[0], "end": starts[0] + length - 1},
        "reason": f"the cited lines are unchanged but now sit elsewhere in {path_text}; this passes "
        "every check and needs no Change Package of its own; when one next touches the record, "
        f"relocate-evidence --from {path_text} --to {path_text} re-cites them and prints the supersedes",
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
