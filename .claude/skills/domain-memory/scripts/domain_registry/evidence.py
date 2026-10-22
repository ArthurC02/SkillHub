from __future__ import annotations

import hashlib
import io
import re
from collections.abc import Callable
from functools import cache
from pathlib import Path
from typing import Any

from .common import MAX_EVIDENCE_BYTES, MAX_NESTING_DEPTH, load_json, review_field
from .git import run_git_bytes
from .sources import repository_files, source_kind_index


DERIVED_EVIDENCE_KEYS = ("content_sha256", "source_kind")
WIDEN_TO_UNIQUE ="Widen the citation to the enclosing block, so its lines occur once in the file."


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
    return content, _rows(content)


def _rows(content: bytes) -> list[str]:
    return io.StringIO(content.decode("utf-8", errors="replace")).readlines()


def _excerpt(rows: list[str], start: int, end: int) -> bytes:
    return "".join(rows[start - 1 : end]).encode("utf-8")


def classifier(source_map: dict[str, Any]) -> Callable[[Any], Any]:
    if not source_map:
        return lambda reference: reference
    kind_of = cache(lambda: source_kind_index(source_map))

    def classify(reference: Any) -> Any:
        if not isinstance(reference, dict) or not isinstance(reference.get("path"), str):
            return reference
        return reference | {"source_kind": kind_of()(reference["path"])}

    return classify


def classified(reference: Any, source_map: dict[str, Any]) -> Any:
    return classifier(source_map)(reference)


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


def without_derived_keys(value: Any) -> Any:
    return with_references(
        value,
        lambda reference: {key: item for key, item in reference.items() if key not in DERIVED_EVIDENCE_KEYS}
        if isinstance(reference, dict)
        else reference,
    )


def fields_beyond_evidence(asset: str, before: dict[str, Any], after: dict[str, Any]) -> list[str]:
    settled_on_apply = ("verified", review_field(asset))
    stripped = [
        with_references({key: value for key, value in record.items() if key not in settled_on_apply}, lambda _: None)
        for record in (before, after)
    ]
    return sorted(
        key for key in stripped[0].keys() | stripped[1].keys()
        if key not in stripped[0] or key not in stripped[1] or stripped[0][key] != stripped[1][key]
    )


def classify_all(value: Any, source_map: dict[str, Any]) -> Any:
    return with_references(value, classifier(source_map))


def _cited_file(root: Path, relative: str) -> Path:
    path = (root / relative).resolve()
    if not path.is_relative_to(root):
        raise ValueError(f"evidence path escapes the repository: {relative}")
    if not path.is_file():
        raise ValueError(f"no such file to cite: {relative}")
    return path


FUNCTION_START = r"^(?P<indent>[ \t]*)(?:(?P<go>func)(?:\s*\([^)]*\))?\s+|(?:async\s+)?def\s+){name}\s*[(\[]"


def function_span(repo_root: Path, relative: str, name: str) -> tuple[int, int]:
    rows = _rows(_cited_file(repo_root.resolve(), relative).read_bytes())
    pattern = re.compile(FUNCTION_START.format(name=re.escape(name)))
    starts = [(number, found) for number, row in enumerate(rows, 1) if (found := pattern.match(row))]
    if len(starts) != 1:
        raise ValueError(
            f"{relative} defines no Go func or Python def named {name}{_defined_elsewhere(repo_root.resolve(), relative, pattern)}"
            if not starts
            else f"{relative} defines {name} {len(starts)} times; cite the one you mean with --lines"
        )
    start, found = starts[0]
    end = _go_end(rows, start, found["indent"]) if found["go"] else _python_end(rows, start, len(found["indent"]))
    if end is None:
        raise ValueError(f"{relative}: the body of {name} starting on line {start} does not end; cite it with --lines")
    return start, end


SOURCE_SUFFIXES = (".go", ".py")


def _defined_elsewhere(repo_root: Path, relative: str, pattern: re.Pattern[str]) -> str:
    suffix = Path(relative).suffix
    if suffix not in SOURCE_SUFFIXES:
        return ""
    found = []
    for path in repository_files(repo_root):
        if path == relative or not path.endswith(suffix):
            continue
        rows = _rows((repo_root / path).read_bytes())
        found.extend(f"{path}:{number}" for number, row in enumerate(rows, 1) if pattern.match(row))
    found.sort()
    if not found:
        return ""
    return f"; it is defined in {found[0]}" if len(found) == 1 else "; it is defined in several files: " + ", ".join(found)


def _go_end(rows: list[str], start: int, indent: str) -> int | None:
    if rows[start - 1].rstrip().endswith("}"):
        return start
    return next((number for number in range(start + 1, len(rows) + 1) if rows[number - 1].rstrip() == indent + "}"), None)


def _python_end(rows: list[str], start: int, indent: int) -> int | None:
    signature_end = next((number for number in range(start, len(rows) + 1) if rows[number - 1].rstrip().endswith(":")), None)
    if signature_end is None:
        return None
    end = signature_end
    for number in range(signature_end + 1, len(rows) + 1):
        row = rows[number - 1]
        if not row.strip():
            continue
        if len(row) - len(row.lstrip()) <= indent:
            break
        end = number
    return end


def citation(repo_root: Path, relative: str, start: int, end: int) -> dict[str, Any]:
    root = repo_root.resolve()
    path = _cited_file(root, relative)
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
    if _source_verdict(path, relative, repo_root.resolve()):
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


def _source_verdict(path: Path, path_text: str, root: Path) -> dict[str, Any] | None:
    if not path.is_relative_to(root):
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
    return verifier(repo_root)(reference)


ContentReader = Callable[[str], bytes | None]


def staged_content(repo_root: Path) -> ContentReader:
    root = repo_root.resolve()

    def git(*arguments: str) -> bytes:
        result = run_git_bytes(root, *arguments)
        if result.returncode != 0:
            reason = result.stderr.decode("utf-8", errors="replace").strip()
            raise ValueError(f"could not read the Git index of {root}: {reason}")
        return result.stdout

    @cache
    def read(path_text: str) -> bytes | None:
        wanted = path_text.encode("utf-8")
        for entry in git("ls-files", "--stage", "-z", "--", f":(literal){path_text}").split(b"\0"):
            meta, _, listed = entry.partition(b"\t")
            mode, _, rest = meta.partition(b" ")
            blob, _, stage = rest.partition(b" ")
            if listed == wanted and stage == b"0" and mode.startswith(b"100"):
                return git("cat-file", "blob", blob.decode("ascii"))
        return None

    return read


def _rows_from(root: Path, path_text: str, content_of: ContentReader) -> dict[str, Any] | list[str]:
    if not (root / path_text).resolve().is_relative_to(root):
        return {"status": "invalid", "reason": "evidence path escapes repository"}
    content = content_of(path_text)
    if content is None:
        return {"status": "missing", "path": path_text}
    if len(content) > MAX_EVIDENCE_BYTES:
        return {"status": "invalid", "reason": f"evidence source exceeds {MAX_EVIDENCE_BYTES} bytes", "path": path_text}
    return _rows(content)


def _disk_rows(root: Path, path_text: str) -> dict[str, Any] | list[str]:
    path = (root / path_text).resolve()
    return _source_verdict(path, path_text, root) or _read_lines(path)[1]


def verifier(repo_root: Path, content_of: ContentReader | None = None) -> Callable[[Any], dict[str, Any]]:
    root = repo_root.resolve()
    sources: dict[str, dict[str, Any] | list[str]] = {}

    def source(path_text: str) -> dict[str, Any] | list[str]:
        if path_text not in sources:
            sources[path_text] = (
                _disk_rows(root, path_text) if content_of is None else _rows_from(root, path_text, content_of)
            )
        return sources[path_text]

    def check(reference: Any) -> dict[str, Any]:
        shape = _shape_error(reference)
        if shape:
            return {"status": "invalid", "reason": shape}
        rows = source(reference["path"])
        if isinstance(rows, dict):
            return dict(rows)
        return {"cited_lines": dict(reference["lines"]), **_cited_lines_verdict(reference, reference["path"], rows)}

    return check


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


def cited_excerpts(record: dict[str, Any]) -> set[tuple[str, str]]:
    return {
        (reference["path"], reference["excerpt_sha256"])
        for reference in all_references(record)
        if isinstance(reference, dict) and isinstance(reference.get("path"), str)
        and isinstance(reference.get("excerpt_sha256"), str)
    }


def one_cites_within_the_other(one: dict[str, Any], other: dict[str, Any]) -> bool:
    mine, theirs = cited_excerpts(one), cited_excerpts(other)
    return bool(mine and theirs) and (mine <= theirs or theirs <= mine)


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
