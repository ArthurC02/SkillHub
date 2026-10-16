from __future__ import annotations

import json
import re
from typing import Any

from .common import review_field

OKF_VERSION = "0.2"

CONCEPT_TYPES = {
    "contexts": "Bounded Context",
    "vocabulary": "Term",
    "aggregates": "Aggregate",
    "rules": "Business Rule",
    "contracts": "Contract",
    "interactions": "Interaction",
    "decisions": "Decision",
    "events": "Domain Event",
    "capabilities": "Capability",
    "value-objects": "Value Object",
    "dependency-policies": "Dependency Policy",
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

RELATION_TARGETS = {
    "context": "contexts",
    "contexts": "contexts",
    "owner_context": "contexts",
    "producer_context": "contexts",
    "consumer_context": "contexts",
    "consumer_contexts": "contexts",
    "from_context": "contexts",
    "to_context": "contexts",
    "consumers": "contexts",
    "contract_id": "contracts",
    "not_same_as": "vocabulary",
    "supersedes": "decisions",
}
LIST_RELATIONS = {"contexts", "consumer_contexts", "consumers", "not_same_as", "supersedes"}
CONTEXT_RELATIONS = tuple(field for field, target in RELATION_TARGETS.items() if target == "contexts")


def contexts_named_by(asset: str, record: dict[str, Any]) -> set[str]:
    named = {record.get("id")} if asset == "contexts" else set()
    for field in CONTEXT_RELATIONS:
        value = record.get(field)
        named.update(value if isinstance(value, list) else [value])
    return {context for context in named if isinstance(context, str)}

OKF_STATUSES = {"candidate": "draft", "reviewed": "stable", "deprecated": "deprecated"}
REGISTRY_STATUSES = {okf: registry for registry, okf in OKF_STATUSES.items()}
RESERVED_KEYS = ("type", "title", "status", "registry_status", "decision_status", "sources")
RELATIONS_HEADING = "Relations"

PLAIN_SCALAR = re.compile(r"[A-Za-z][A-Za-z0-9 _./@+()-]*")
YAML_KEY = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")
YAML_WORDS = {"true", "false", "null", "yes", "no", "on", "off", "y", "n", "~"}
CONCEPT_ID = re.compile(r"[^\s()\[\]]+")
RELATION_LINE = re.compile(r"- (.+?): \[[^\]]*\]\(/([a-z-]+)/([^)\s]+)\)")
INTEGER = re.compile(r"-?\d+")
CITED_RANGE = re.compile(r"(.+)#L(\d+)-(\d+)")


def verified_by(approvals: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [
        {"by": f"human:{approval['reviewer']}", "at": approval["approved_at"]}
        for approval in approvals
        if approval.get("decision") == "approved"
    ]


def concept_path(asset: str, record_id: str) -> str:
    return f"/{asset}/{record_id}"


def encode(asset: str, record: dict[str, Any]) -> str:
    remaining = {key: value for key, value in record.items() if key != "id"}
    front: dict[str, Any] = {"type": CONCEPT_TYPES[asset]}
    title_field = TITLE_FIELDS.get(asset)
    if title_field and _is_text(remaining.get(title_field)) and "\n" not in remaining[title_field]:
        front["title"] = remaining.pop(title_field)
    front.update(_status_keys(asset, remaining))
    prose = _take_prose(asset, remaining)
    relations = _take_relations(remaining)
    sections = _take_sections(remaining)
    sources = _take_sources(remaining)
    for key in remaining:
        if key in RESERVED_KEYS:
            raise ValueError(f"{asset}/{record.get('id')} has a field named {key}, which the concept format reserves")
    front.update(remaining)
    if sources is not None:
        front["sources"] = sources
    return _document(front, _body(prose, relations, sections))


def decode(asset: str, record_id: str, text: str) -> dict[str, Any]:
    front, body = split_concept(text)
    record: dict[str, Any] = {"id": record_id}
    title_field = TITLE_FIELDS.get(asset)
    if title_field and "title" in front:
        record[title_field] = front["title"]
    prose, relations, sections = _parse_body(body)
    if prose:
        if asset not in PROSE_FIELDS:
            raise ValueError(f"a {asset} concept has no prose field; move its text into a frontmatter field")
        record[PROSE_FIELDS[asset]] = prose
    for key, value in front.items():
        if key in RESERVED_KEYS:
            continue
        record[key] = value
    record.update(relations)
    record.update(sections)
    if "sources" in front:
        record["evidence"] = [_evidence(source) for source in front["sources"]]
    record.update(_registry_status(asset, front))
    return record


def split_concept(text: str) -> tuple[dict[str, Any], str]:
    lines = text.replace("\r\n", "\n").split("\n")
    if not lines or lines[0] != "---":
        raise ValueError("a concept document starts with a --- frontmatter line")
    try:
        end = lines.index("---", 1)
    except ValueError as error:
        raise ValueError("a concept document's frontmatter has no closing --- line") from error
    front = load_yaml(lines[1:end])
    if not isinstance(front, dict) or not front.get("type"):
        raise ValueError("a concept document's frontmatter needs a non-empty type")
    return front, "\n".join(lines[end + 1:])


def _is_text(value: Any) -> bool:
    return isinstance(value, str) and value == value.strip() and bool(value)


def _status_keys(asset: str, remaining: dict[str, Any]) -> dict[str, Any]:
    keys: dict[str, Any] = {}
    field = review_field(asset)
    if field in remaining:
        status = remaining.pop(field)
        if status in OKF_STATUSES:
            keys["status"] = OKF_STATUSES[status]
        else:
            if status == "superseded":
                keys["status"] = "deprecated"
            keys["registry_status"] = status
    if asset == "decisions" and "status" in remaining:
        keys["decision_status"] = remaining.pop("status")
    return keys


def _registry_status(asset: str, front: dict[str, Any]) -> dict[str, Any]:
    keys: dict[str, Any] = {}
    if asset == "decisions" and "decision_status" in front:
        keys["status"] = front["decision_status"]
    if "registry_status" in front:
        keys[review_field(asset)] = front["registry_status"]
    elif "status" in front:
        keys[review_field(asset)] = REGISTRY_STATUSES.get(front["status"], front["status"])
    return keys


def _take_prose(asset: str, remaining: dict[str, Any]) -> str | None:
    field = PROSE_FIELDS.get(asset)
    value = remaining.get(field) if field else None
    if not _is_text(value) or any(line.startswith("#") for line in value.split("\n")):
        return None
    return remaining.pop(field)


def _relation_ids(field: str, value: Any) -> list[str] | None:
    values = value if field in LIST_RELATIONS else [value]
    if field in LIST_RELATIONS and (not isinstance(value, list) or not value):
        return None
    if all(isinstance(item, str) and CONCEPT_ID.fullmatch(item) for item in values):
        return values
    return None


def _take_relations(remaining: dict[str, Any]) -> list[tuple[str, str, str]]:
    relations = []
    for field in [key for key in remaining if key in RELATION_TARGETS]:
        ids = _relation_ids(field, remaining[field])
        if ids is None:
            continue
        remaining.pop(field)
        relations += [(field, RELATION_TARGETS[field], target) for target in ids]
    return relations


def _label(field: str) -> str:
    return field.replace("_", " ").capitalize()


def _field(label: str) -> str:
    return label.lower().replace(" ", "_")


def _is_section(field: str, value: Any) -> bool:
    return (
        _field(_label(field)) == field
        and _label(field) != RELATIONS_HEADING
        and isinstance(value, list)
        and bool(value)
        and all(_is_text(item) and "\n" not in item for item in value)
    )


def _take_sections(remaining: dict[str, Any]) -> list[tuple[str, list[str]]]:
    fields = [key for key, value in remaining.items() if _is_section(key, value)]
    return [(field, remaining.pop(field)) for field in fields]


def _cited_source(reference: Any) -> dict[str, Any] | None:
    if not isinstance(reference, dict) or not isinstance(reference.get("path"), str):
        return None
    lines = reference.get("lines")
    if not (isinstance(lines, dict) and set(lines) == {"start", "end"}
            and all(type(lines[key]) is int for key in lines)):
        return None
    if "resource" in reference:
        return None
    rest = {key: value for key, value in reference.items() if key not in ("path", "lines")}
    return {"resource": f"{reference['path']}#L{lines['start']}-{lines['end']}", **rest}


def _take_sources(remaining: dict[str, Any]) -> list[dict[str, Any]] | None:
    evidence = remaining.get("evidence")
    if not isinstance(evidence, list) or not evidence:
        return None
    sources = [_cited_source(reference) for reference in evidence]
    if any(source is None for source in sources):
        return None
    remaining.pop("evidence")
    return sources


def _evidence(source: Any) -> Any:
    if not isinstance(source, dict) or not isinstance(source.get("resource"), str):
        return source
    cited = CITED_RANGE.fullmatch(source["resource"])
    rest = {key: value for key, value in source.items() if key != "resource"}
    if cited is None:
        return {"path": source["resource"], **rest}
    path, start, end = cited.groups()
    return {"path": path, "lines": {"start": int(start), "end": int(end)}, **rest}


def _body(prose: str | None, relations: list[tuple[str, str, str]], sections: list[tuple[str, list[str]]]) -> str:
    parts = [prose] if prose else []
    if relations:
        parts.append("\n".join(
            [f"# {RELATIONS_HEADING}"]
            + [f"- {_label(field)}: [{target}]({concept_path(asset, target)})" for field, asset, target in relations]
        ))
    parts += ["\n".join([f"# {_label(field)}"] + [f"- {item}" for item in items]) for field, items in sections]
    return "\n\n".join(parts)


def _parse_body(body: str) -> tuple[str, dict[str, Any], dict[str, list[str]]]:
    prose_lines: list[str] = []
    relations: dict[str, Any] = {}
    sections: dict[str, list[str]] = {}
    heading = None
    for line in body.split("\n"):
        if line.startswith("# "):
            heading = line[2:].strip()
            continue
        if heading is None:
            prose_lines.append(line)
        elif heading == RELATIONS_HEADING and line.strip():
            _add_relation(relations, line)
        elif line.startswith("- "):
            sections.setdefault(_field(heading), []).append(line[2:])
    return "\n".join(prose_lines).strip(), relations, sections


def _add_relation(relations: dict[str, Any], line: str) -> None:
    match = RELATION_LINE.fullmatch(line.strip())
    if match is None:
        raise ValueError(f"a Relations line must read '- Label: [text](/asset/id)': {line.strip()}")
    field, target = _field(match.group(1)), match.group(3)
    if field in LIST_RELATIONS:
        relations.setdefault(field, []).append(target)
    else:
        relations[field] = target


def _document(front: dict[str, Any], body: str) -> str:
    text = "---\n" + "\n".join(dump_yaml(front)) + "\n---\n"
    return text + (body + "\n" if body else "")


def _scalar(value: Any) -> str:
    if value is None or isinstance(value, (bool, int, float)):
        return json.dumps(value)
    if value == []:
        return "[]"
    if value == {}:
        return "{}"
    if PLAIN_SCALAR.fullmatch(value) and not value.endswith(" ") and value.lower() not in YAML_WORDS:
        return value
    return json.dumps(value, ensure_ascii=False)


def _is_scalar(value: Any) -> bool:
    return not isinstance(value, (dict, list)) or value in ([], {})


def dump_yaml(mapping: dict[str, Any], indent: int = 0) -> list[str]:
    pad = " " * indent
    lines = []
    for key, value in mapping.items():
        if not YAML_KEY.fullmatch(key):
            raise ValueError(f"the concept format cannot write the key {key!r}")
        if _is_scalar(value):
            lines.append(f"{pad}{key}: {_scalar(value)}")
        elif isinstance(value, dict):
            lines += [f"{pad}{key}:", *dump_yaml(value, indent + 2)]
        else:
            lines += [f"{pad}{key}:", *_dump_sequence(value, indent + 2)]
    return lines


def _dump_sequence(items: list[Any], indent: int) -> list[str]:
    pad = " " * indent
    lines = []
    for item in items:
        if _is_scalar(item):
            lines.append(f"{pad}- {_scalar(item)}")
        elif isinstance(item, dict):
            nested = dump_yaml(item, indent + 2)
            lines += [f"{pad}- {nested[0].lstrip()}", *nested[1:]]
        else:
            lines += [f"{pad}-", *_dump_sequence(item, indent + 2)]
    return lines


def _parse_scalar(text: str, number: int) -> Any:
    if text.startswith('"'):
        try:
            return json.loads(text)
        except ValueError as error:
            raise ValueError(f"frontmatter line {number}: unreadable quoted text") from error
    if text in ("[]", "{}", "null", "true", "false") or INTEGER.fullmatch(text):
        return json.loads(text)
    if text[:1] in ("'", "[", "{", "&", "*", "!", "|", ">", "%", "@", "`"):
        raise ValueError(f"frontmatter line {number}: write this value plainly or in double quotes: {text}")
    return text


class _Lines:
    def __init__(self, lines: list[str]) -> None:
        self.rows = [(len(line) - len(line.lstrip(" ")), line.strip(), number)
                     for number, line in enumerate(lines, start=2)
                     if line.strip() and not line.lstrip().startswith("#")]
        self.at = 0

    def peek(self) -> tuple[int, str, int] | None:
        return self.rows[self.at] if self.at < len(self.rows) else None


def load_yaml(lines: list[str]) -> Any:
    source = _Lines(lines)
    if source.peek() is None:
        return {}
    value = _parse_block(source, source.peek()[0])
    if source.peek() is not None:
        raise ValueError(f"frontmatter line {source.peek()[2]}: unexpected indentation")
    return value


def _parse_block(source: _Lines, indent: int) -> Any:
    row = source.peek()
    if row[1] == "-" or row[1].startswith("- "):
        return _parse_sequence(source, indent)
    return _parse_mapping(source, indent)


def _key_value(text: str, number: int) -> tuple[str, str]:
    key, separator, rest = text.partition(":")
    if not separator or not YAML_KEY.fullmatch(key) or (rest and not rest.startswith(" ")):
        raise ValueError(f"frontmatter line {number}: expected 'key: value', got {text}")
    return key, rest.strip()


def _parse_mapping(source: _Lines, indent: int) -> dict[str, Any]:
    mapping: dict[str, Any] = {}
    while (row := source.peek()) is not None and row[0] == indent and not row[1].startswith("-"):
        key, rest = _key_value(row[1], row[2])
        source.at += 1
        if key in mapping:
            raise ValueError(f"frontmatter line {row[2]}: {key} appears twice")
        if rest:
            mapping[key] = _parse_scalar(rest, row[2])
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
            source.rows[source.at] = (indent + 2, content, row[2])
            items.append(_parse_mapping(source, indent + 2))
        else:
            source.at += 1
            items.append(_parse_scalar(content, row[2]))
    return items
