from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

ASSET_KEYS = {
    "contexts": "contexts",
    "vocabulary": "terms",
    "aggregates": "aggregates",
    "rules": "rules",
    "contracts": "contracts",
    "interactions": "interactions",
    "decisions": "decisions",
    "events": "events",
    "capabilities": "capabilities",
    "value-objects": "value_objects",
    "dependency-policies": "dependencies",
}
TITLE_FIELDS = {"contexts": "name", "vocabulary": "name"}
PROSE_FIELDS = {
    "contexts": "responsibility",
    "vocabulary": "definition",
    "rules": "statement",
    "decisions": "statement",
    "events": "meaning",
    "capabilities": "meaning",
    "value-objects": "meaning",
    "dependency-policies": "rationale",
}
STATUS_OF_OKF = {"draft": "candidate", "stable": "reviewed", "deprecated": "deprecated"}
RESERVED_KEYS = ("type", "title", "status", "registry_status", "decision_status", "sources")
RELATIONS_HEADING = "Relations"
YAML_KEY = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")
INTEGER = re.compile(r"-?\d+")
DECIMAL = re.compile(r"-?\d+(\.\d+)?[eE][+-]?\d+|-?\d+\.\d+")
CITED_RANGE = re.compile(r"(.+)#L(\d+)-(\d+)")


def read_records(registry_root: Path, asset: str) -> list[dict[str, Any]]:
    directory = registry_root / "registry" / asset
    return [decode(asset, path.stem, path.read_text(encoding="utf-8"))
            for path in sorted(directory.glob("*.md")) if path.name != "index.md"]


def review_field(asset: str) -> str:
    return "review_status" if asset == "decisions" else "status"


def decode(asset: str, record_id: str, text: str) -> dict[str, Any]:
    front, body = split_concept(text)
    record: dict[str, Any] = {"id": record_id}
    title_field = TITLE_FIELDS.get(asset)
    if title_field and "title" in front:
        record[title_field] = front["title"]
    prose, sections = _parse_body(body)
    if prose and asset in PROSE_FIELDS:
        record[PROSE_FIELDS[asset]] = prose
    record.update({key: value for key, value in front.items() if key not in RESERVED_KEYS})
    record.update(sections)
    if "sources" in front:
        record["evidence"] = [_evidence(source) for source in front["sources"]]
    record.update(_registry_status(asset, front))
    return record


def split_concept(text: str) -> tuple[dict[str, Any], str]:
    lines = text.replace("\r\n", "\n").split("\n")
    end = lines.index("---", 1)
    return load_yaml(lines[1:end]), "\n".join(lines[end + 1:])


def _registry_status(asset: str, front: dict[str, Any]) -> dict[str, Any]:
    keys: dict[str, Any] = {}
    if asset == "decisions" and "decision_status" in front:
        keys["status"] = front["decision_status"]
    if "registry_status" in front:
        keys[review_field(asset)] = front["registry_status"]
    elif "status" in front:
        keys[review_field(asset)] = STATUS_OF_OKF.get(front["status"], front["status"])
    return keys


def _evidence(source: Any) -> Any:
    if not isinstance(source, dict) or not isinstance(source.get("resource"), str):
        return source
    cited = CITED_RANGE.fullmatch(source["resource"])
    rest = {key: value for key, value in source.items() if key != "resource"}
    if cited is None:
        return {"path": source["resource"], **rest}
    path, start, end = cited.groups()
    return {"path": path, "lines": {"start": int(start), "end": int(end)}, **rest}


def _parse_body(body: str) -> tuple[str, dict[str, list[str]]]:
    prose_lines: list[str] = []
    sections: dict[str, list[str]] = {}
    heading = None
    for line in body.split("\n"):
        if line.startswith("# "):
            heading = line[2:].strip()
        elif heading is None:
            prose_lines.append(line)
        elif heading != RELATIONS_HEADING and line.startswith("- "):
            sections.setdefault(heading.lower().replace(" ", "_"), []).append(line[2:])
    return "\n".join(prose_lines).strip(), sections


def _parse_scalar(text: str) -> Any:
    if text.startswith('"'):
        return json.loads(text)
    if text in ("[]", "{}", "null", "true", "false") or INTEGER.fullmatch(text) or DECIMAL.fullmatch(text):
        return json.loads(text)
    return text


class _Lines:
    def __init__(self, lines: list[str]) -> None:
        self.rows = [(len(line) - len(line.lstrip(" ")), line.strip())
                     for line in lines if line.strip() and not line.lstrip().startswith("#")]
        self.at = 0

    def peek(self) -> tuple[int, str] | None:
        return self.rows[self.at] if self.at < len(self.rows) else None


def load_yaml(lines: list[str]) -> Any:
    source = _Lines(lines)
    return _parse_block(source, source.peek()[0]) if source.peek() is not None else {}


def _parse_block(source: _Lines, indent: int) -> Any:
    row = source.peek()
    if row[1] == "-" or row[1].startswith("- "):
        return _parse_sequence(source, indent)
    return _parse_mapping(source, indent)


def _parse_mapping(source: _Lines, indent: int) -> dict[str, Any]:
    mapping: dict[str, Any] = {}
    while (row := source.peek()) is not None and row[0] == indent and not row[1].startswith("-"):
        key, _, rest = row[1].partition(":")
        rest = rest.strip()
        source.at += 1
        if rest:
            mapping[key] = _parse_scalar(rest)
            continue
        nested = source.peek()
        if nested is None or nested[0] < indent or (nested[0] == indent and not nested[1].startswith("-")):
            mapping[key] = None
            continue
        mapping[key] = _parse_block(source, nested[0])
    return mapping


def _parse_sequence(source: _Lines, indent: int) -> list[Any]:
    items: list[Any] = []
    while (row := source.peek()) is not None and row[0] == indent and (row[1] == "-" or row[1].startswith("- ")):
        content = row[1][1:].strip()
        if not content:
            source.at += 1
            items.append(_parse_block(source, source.peek()[0]))
        elif re.match(r"[A-Za-z_][A-Za-z0-9_]*:( |$)", content):
            source.rows[source.at] = (indent + 2, content)
            items.append(_parse_mapping(source, indent + 2))
        else:
            source.at += 1
            items.append(_parse_scalar(content))
    return items
