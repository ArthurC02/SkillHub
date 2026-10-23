from __future__ import annotations

import difflib
import re
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS
from .sql_schema import IDENT, Constraint, ForeignKey, Index, Schema, Table, live_tables, one_line, read_schema, strip_comments
from .store import read_document

CHECK_BODY = re.compile(r"\bCHECK\s*\((.*)\)", re.I | re.S)
ONLY_AN_IN_LIST = re.compile(
    rf"^\s*(?:({IDENT})\s+IS\s+(?:NULL\s+OR|NOT\s+NULL\s+AND)\s+)?({IDENT})\s+(?:IN\s*\(([^)]*)\)|=\s*('(?:[^']|'')*'))\s*$",
    re.I | re.S)
QUOTED = re.compile(r"'((?:[^']|'')*)'")
SUMMARY_FIELDS = ("statement", "definition", "responsibility", "description", "name")
SUMMARY_LENGTH = 160
DEFAULT_ACTION = "NO ACTION"


def describe_table(registry_root: Path | None, repo_root: Path, migrations_dir: Path, name: str) -> dict[str, Any]:
    schema = read_schema(migrations_dir, repo_root)
    tables = live_tables(schema)
    table = tables.get(name)
    if table is None:
        raise ValueError(f"no table {name!r} in {migrations_dir}{_did_you_mean(name, tables)}")
    constraints = [constraint for constraint in schema.constraints if constraint.table == name]
    primary = [schema.primary_keys[name]] if name in schema.primary_keys else []
    outgoing = [key for key in schema.foreign_keys if key.child == name]
    incoming = [key for key in schema.foreign_keys if key.parent == name and key.child in tables]
    return {
        "table": name,
        "declared_at": f"{table.file}:{table.line}",
        "meaning": table.comment,
        "columns": [_column(column, info, constraints, schema) for column, info in table.columns.items()],
        "constraints": [*(_constraint(c) for c in primary + constraints), *(_foreign_constraint(key) for key in outgoing)],
        "indexes": [_index(index) for index in schema.indexes if index.table == name],
        "references": [_edge("parent", key.parent, key, tables) for key in outgoing],
        "referenced_by": [_edge("child", key.child, key, tables) for key in incoming],
        "partitions": sorted(partition for partition, declared in schema.tables.items() if declared.partition_of == name),
        "registry": _mentions(registry_root, name, table, primary + constraints, outgoing) if registry_root is not None else [],
        "undescribed_columns": [column for column, info in table.columns.items() if info.get("comment") is None],
    }


def _did_you_mean(name: str, tables: dict[str, Table]) -> str:
    close = difflib.get_close_matches(name, sorted(tables), n=3, cutoff=0.5)
    return f"; did you mean {', '.join(close)}?" if close else ""


def _column(column: str, info: dict[str, Any], constraints: list[Constraint], schema: Schema) -> dict[str, Any]:
    declared_type = info.get("type", "")
    return {
        "name": column,
        "line": info["line"],
        "at": f"{info['file']}:{info['line']}",
        "type": declared_type,
        "type_meaning": schema.type_comments.get(declared_type),
        "required": info.get("not_null", False),
        "primary_key": info.get("primary_key", False),
        "default": info.get("default"),
        "meaning": info.get("comment"),
        "allowed_values": allowed_values(column, constraints, schema.enums.get(declared_type)),
    }


def allowed_values(column: str, constraints: list[Constraint], enum_values: list[str] | None = None) -> list[str] | None:
    for constraint in constraints:
        body = CHECK_BODY.search(strip_comments(constraint.text)) if constraint.kind == "check" else None
        listed = ONLY_AN_IN_LIST.match(body.group(1)) if body else None
        if listed and listed.group(2) == column and listed.group(1) in (None, column):
            return [value.replace("''", "'") for value in QUOTED.findall(listed.group(3) or listed.group(4))]
    return list(enum_values) if enum_values else None


def _constraint(constraint: Constraint) -> dict[str, Any]:
    return {"kind": constraint.kind, "name": constraint.name, "text": one_line(constraint.text),
            "at": f"{constraint.file}:{constraint.line}"}


def _foreign_constraint(key: ForeignKey) -> dict[str, Any]:
    return {"kind": "foreign", "name": key.name, "text": key.text, "at": f"{key.file}:{key.line}"}


def _index(index: Index) -> dict[str, Any]:
    return {"name": index.name, "unique": index.unique, "definition": index.definition, "at": f"{index.file}:{index.line}"}


def _edge(role: str, other: str, key: ForeignKey, tables: dict[str, Table]) -> dict[str, Any]:
    parent_columns = list(key.parent_columns) or _primary_columns(tables.get(key.parent))
    return {role: other, "name": key.name, "columns": list(key.columns), "parent_columns": parent_columns,
            "on_delete": key.on_delete or DEFAULT_ACTION, "on_update": key.on_update or DEFAULT_ACTION, "at": f"{key.file}:{key.line}"}


def _primary_columns(table: Table | None) -> list[str]:
    return [column for column, info in table.columns.items() if info.get("primary_key")] if table else []


def _mentions(registry_root: Path, name: str, table: Table, constraints: list[Constraint], keys: list[ForeignKey]) -> list[dict[str, Any]]:
    word = re.compile(rf"\b{re.escape(name)}\b")
    spans = [(table.file, table.line, table.line), *((c.file, c.line, c.end) for c in constraints),
             *((key.file, key.line, key.line) for key in keys)]
    found = []
    for document_name, key in ASSET_KEYS.items():
        for record in read_document(registry_root, document_name).get(key, []):
            if not isinstance(record, dict):
                continue
            named = any(word.search(str(record.get(field, ""))) for field in SUMMARY_FIELDS)
            if named or _cites_any(record.get("evidence"), spans):
                found.append(_mention(key, record))
    return found


def _cites_any(evidence: Any, spans: list[tuple[str, int, int]]) -> bool:
    for reference in evidence if isinstance(evidence, list) else []:
        if not isinstance(reference, dict) or not isinstance(reference.get("lines"), dict):
            continue
        start, end = reference["lines"].get("start"), reference["lines"].get("end")
        if any(reference.get("path") == file and start <= last and end >= first for file, first, last in spans):
            return True
    return False


def _mention(asset: str, record: dict[str, Any]) -> dict[str, Any]:
    summary = next((str(record[field]) for field in SUMMARY_FIELDS if record.get(field)), "")
    return {
        "asset": asset,
        "id": record.get("id"),
        "status": record.get("status"),
        "summary": summary if len(summary) <= SUMMARY_LENGTH else summary[: SUMMARY_LENGTH - 1] + "…",
    }
