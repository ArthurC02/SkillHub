from __future__ import annotations

import difflib
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS
from .sql_schema import (
    IDENT,
    Constraint,
    Dynamic,
    ForeignKey,
    Index,
    Retired,
    Schema,
    Table,
    live_tables,
    one_line,
    read_schema,
    strip_comments,
)
from .store import read_document

CHECK_BODY = re.compile(r"\bCHECK\s*\((.*)\)", re.I | re.S)
ONLY_AN_IN_LIST = re.compile(
    rf"^\s*(?:({IDENT})\s+IS\s+(?:NULL\s+OR|NOT\s+NULL\s+AND)\s+)?({IDENT})\s+(?:IN\s*\(([^)]*)\)|=\s*('(?:[^']|'')*'))\s*$",
    re.I | re.S)
QUOTED = re.compile(r"'((?:[^']|'')*)'")
SUMMARY_FIELDS = ("statement", "definition", "responsibility", "description", "name")
SUMMARY_LENGTH = 160
DEFAULT_ACTION = "NO ACTION"
STATUS_LIKE = re.compile(r"(^|_)(status|state|kind|mode|tier|role|decision|verdict|outcome)$")
VALUE_COMMENT_LIMIT = 12
CONVENTIONAL_COLUMNS = frozenset({"id", "created_at", "updated_at"})


@dataclass(frozen=True)
class ValueList:
    values: list[str]
    null_decided: bool


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
    retired = [gone for gone in schema.retired if gone.table == name]
    dynamic = [statement for statement in schema.dynamic if name in statement.tables]
    spans = [(table.file, table.line, table.line), *((c.file, c.line, c.end) for c in primary + constraints),
             *((key.file, key.line, key.line) for key in outgoing)]
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
        "dynamic_ddl": [{"at": f"{statement.file}:{statement.line}", "statement": statement.text} for statement in dynamic],
        "registry": _mentions(registry_root, name, spans, retired) if registry_root is not None else [],
        "undescribed_columns": [column for column, info in table.columns.items() if info.get("comment") is None],
        "gaps": _gaps(table, constraints, outgoing, dynamic, schema),
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


def value_list(column: str, constraints: list[Constraint], enum_values: list[str] | None = None) -> ValueList | None:
    for constraint in constraints:
        body = CHECK_BODY.search(strip_comments(constraint.text)) if constraint.kind == "check" else None
        listed = ONLY_AN_IN_LIST.match(body.group(1)) if body else None
        if listed and listed.group(2) == column and listed.group(1) in (None, column):
            values = [value.replace("''", "'") for value in QUOTED.findall(listed.group(3) or listed.group(4))]
            return ValueList(values, null_decided=listed.group(1) is not None)
    return ValueList(list(enum_values), null_decided=False) if enum_values else None


def allowed_values(column: str, constraints: list[Constraint], enum_values: list[str] | None = None) -> list[str] | None:
    listed = value_list(column, constraints, enum_values)
    return listed.values if listed else None


def _gaps(table: Table, constraints: list[Constraint], keys: list[ForeignKey], dynamic: list[Dynamic], schema: Schema) -> list[str]:
    gaps = [] if table.comment is not None else ["the table has no COMMENT ON saying what one row is"]
    for column, info in table.columns.items():
        gaps.extend(_column_gaps(column, info, value_list(column, constraints, schema.enums.get(info["type"])), schema))
    gaps.extend(f"foreign key {key.name} to {key.parent} does not say ON DELETE; choose CASCADE, RESTRICT, SET NULL or NO ACTION"
                for key in keys if key.on_delete is None)
    gaps.extend(f"the statement at {statement.file}:{statement.line} builds DDL at run time and names {table.name}; "
                "the files cannot show what it creates, so write a static statement per table" for statement in dynamic)
    return gaps


def _column_gaps(column: str, info: dict[str, Any], listed: ValueList | None, schema: Schema) -> list[str]:
    gaps = []
    explained_in = " ".join(text for text in (info.get("comment"), schema.type_comments.get(info["type"])) if text)
    if info.get("comment") is None and column not in CONVENTIONAL_COLUMNS:
        gaps.append(f"column {column} has no COMMENT ON saying what it means")
    if listed is None and STATUS_LIKE.search(column):
        gaps.append(f"status-like column {column} has no value list; add CHECK ({column} IN (...)) or an enum type")
    if listed and not info.get("not_null") and not listed.null_decided:
        gaps.append(f"column {column} lists its values without deciding NULL; make it NOT NULL, or write "
                    f"CHECK ({column} IS NULL OR {column} IN (...)) or CHECK ({column} IS NOT NULL AND {column} IN (...))")
    unexplained = [value for value in listed.values if value not in explained_in] if listed and explained_in else []
    if unexplained and len(listed.values) <= VALUE_COMMENT_LIMIT:
        gaps.append(f"the comment on {column} does not say what these values mean: {', '.join(unexplained)}")
    return gaps


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


def _mentions(registry_root: Path, name: str, spans: list[tuple[str, int, int]], retired: list[Retired]) -> list[dict[str, Any]]:
    word = re.compile(rf"\b{re.escape(name)}\b")
    found = []
    for document_name, key in ASSET_KEYS.items():
        for record in read_document(registry_root, document_name).get(key, []):
            if not isinstance(record, dict):
                continue
            named = any(word.search(str(record.get(field, ""))) for field in SUMMARY_FIELDS)
            replaced = next((gone for gone in retired if _cites_any(record.get("evidence"), [(gone.file, gone.line, gone.end)])), None)
            if named or replaced or _cites_any(record.get("evidence"), spans):
                found.append(_mention(key, record, replaced))
    return found


def _cites_any(evidence: Any, spans: list[tuple[str, int, int]]) -> bool:
    for reference in evidence if isinstance(evidence, list) else []:
        if not isinstance(reference, dict) or not isinstance(reference.get("lines"), dict):
            continue
        start, end = reference["lines"].get("start"), reference["lines"].get("end")
        if any(reference.get("path") == file and start <= last and end >= first for file, first, last in spans):
            return True
    return False


def _mention(asset: str, record: dict[str, Any], replaced: Retired | None) -> dict[str, Any]:
    summary = next((str(record[field]) for field in SUMMARY_FIELDS if record.get(field)), "")
    return {
        "asset": asset,
        "id": record.get("id"),
        "status": record.get("status"),
        "summary": summary if len(summary) <= SUMMARY_LENGTH else summary[: SUMMARY_LENGTH - 1] + "…",
        "cites_replaced": {"kind": replaced.kind, "name": replaced.name, "at": f"{replaced.file}:{replaced.line}",
                           "replaced_at": replaced.replaced_at} if replaced else None,
    }
