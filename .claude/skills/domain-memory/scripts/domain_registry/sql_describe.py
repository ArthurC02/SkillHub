from __future__ import annotations

import difflib
import io
import re
import tarfile
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS
from .policy import schema_conventions
from .git import checked_git_bytes
from .sql_schema import (
    IDENT,
    STRING_LITERAL,
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
    statements,
    strip_comments,
    top_level_words,
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
VERBS = ("SELECT", "INSERT", "UPDATE", "DELETE")


@dataclass(frozen=True)
class Conventions:
    exempt_columns: frozenset[str]
    status_like: re.Pattern[str]
    free_text: re.Pattern[str]
    marker: str
    value_comment_limit: int

    @classmethod
    def from_policy(cls, policy: dict[str, Any] | None) -> Conventions:
        given = schema_conventions(policy)
        suffixes = "|".join(re.escape(word) for word in given["status_like_suffixes"])
        markers = "|".join(re.escape(marker) for marker in given["free_text_markers"])
        return cls(frozenset(given["exempt_columns"]), re.compile(rf"(^|_)(?:{suffixes})$"),
                   re.compile(rf"^\s*(?:{markers})\b", re.I), given["free_text_markers"][0], given["value_comment_limit"])


DEFAULT_CONVENTIONS = Conventions.from_policy(None)


@dataclass(frozen=True)
class Statement:
    at: str
    text: str


@dataclass(frozen=True)
class ValueList:
    values: list[str]
    null_decided: bool


def describe_table(registry_root: Path | None, repo_root: Path, migrations_dir: Path, name: str,
                   conventions: Conventions = DEFAULT_CONVENTIONS) -> dict[str, Any]:
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
    dynamic = dynamic_for(schema, name, constraints)
    spans = [(table.file, table.line, table.line), *((c.file, c.line, c.end) for c in primary + constraints),
             *((key.file, key.line, key.line) for key in outgoing)]
    return {
        "table": name,
        "declared_at": f"{table.file}:{table.line}",
        "history": schema.history.get(name, []),
        "meaning": table.comment,
        "columns": [_column(column, info, constraints, schema) for column, info in table.columns.items()],
        "constraints": [*(_constraint(c) for c in primary + constraints), *(_foreign_constraint(key) for key in outgoing)],
        "indexes": [_index(index) for index in schema.indexes if index.table == name],
        "references": [_edge("parent", key.parent, key, tables) for key in outgoing],
        "referenced_by": [_edge("child", key.child, key, tables) for key in incoming],
        "partitions": sorted(partition for partition, declared in schema.tables.items() if declared.partition_of == name),
        "dynamic_ddl": [{"at": f"{statement.file}:{statement.line}", "statement": statement.text} for statement in dynamic],
        "registry": _mentions(registry_root, name, spans, retired) if registry_root is not None else [],
        "undescribed_columns": _undescribed(table),
        "gaps": _gaps(table, schema, conventions),
    }


def catalog(repo_root: Path, migrations_dir: Path, conventions: Conventions = DEFAULT_CONVENTIONS) -> dict[str, Any]:
    schema = read_schema(migrations_dir, repo_root)
    return {"tables": [_catalog_entry(table, schema, conventions) for _, table in sorted(live_tables(schema).items())]}


def _catalog_entry(table: Table, schema: Schema, conventions: Conventions) -> dict[str, Any]:
    return {"table": table.name, "declared_at": f"{table.file}:{table.line}", "meaning": table.comment,
            "columns": len(table.columns), "undescribed_columns": len(_undescribed(table)),
            "gaps": len(_gaps(table, schema, conventions))}


def _undescribed(table: Table) -> list[str]:
    return [column for column, info in table.columns.items() if info.get("comment") is None]


def queries_naming(queries_dir: Path, repo_root: Path, table: str, columns: list[str]) -> list[dict[str, Any]]:
    return statements_naming(sql_statements_under(queries_dir, repo_root), table, columns)


def sql_statements_under(queries_dir: Path, repo_root: Path) -> list[Statement]:
    found = []
    for path in sorted(queries_dir.rglob("*.sql")):
        file = path.resolve().relative_to(repo_root.resolve()).as_posix()
        for line, statement in statements(path.read_text(encoding="utf-8")):
            found.append(Statement(f"{file}:{line}", STRING_LITERAL.sub("''", strip_comments(statement))))
    return found


def statements_naming(read: list[Statement], table: str, columns: list[str]) -> list[dict[str, Any]]:
    names = re.compile(rf"\b(?:FROM|JOIN|INTO|UPDATE)\s+(?:ONLY\s+)?(?:{IDENT}\.)?{re.escape(table)}\b", re.I)
    patterns = {column: re.compile(rf"\b{re.escape(column)}\b") for column in columns}
    return [{"at": statement.at, "verb": next((word for _, word in top_level_words(statement.text) if word in VERBS), None),
             "columns": [column for column, pattern in patterns.items() if pattern.search(statement.text)]}
            for statement in read if names.search(statement.text)]


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
        body = CHECK_BODY.search(strip_comments(constraint.clause or constraint.text)) if constraint.kind == "check" else None
        listed = ONLY_AN_IN_LIST.match(body.group(1)) if body else None
        if listed and listed.group(2) == column and listed.group(1) in (None, column):
            values = [value.replace("''", "'") for value in QUOTED.findall(listed.group(3) or listed.group(4))]
            decided = listed.group(1) is not None or _null_decided_elsewhere(column, constraints)
            return ValueList(values, null_decided=decided)
    return ValueList(list(enum_values), null_decided=False) if enum_values else None


def _null_decided_elsewhere(column: str, constraints: list[Constraint]) -> bool:
    names_null = re.compile(rf"\b{re.escape(column)}\s+IS\s+(?:NOT\s+)?NULL\b", re.I)
    return any(c.kind == "check" and names_null.search(strip_comments(c.clause or c.text)) for c in constraints)


def allowed_values(column: str, constraints: list[Constraint], enum_values: list[str] | None = None) -> list[str] | None:
    listed = value_list(column, constraints, enum_values)
    return listed.values if listed else None


def gaps_since(repo_root: Path, migrations_dir: Path, base: str, only: str | None = None,
               conventions: Conventions = DEFAULT_CONVENTIONS) -> dict[str, Any]:
    relative = migrations_dir.resolve().relative_to(repo_root.resolve()).as_posix()
    now = _gaps_by_table(migrations_dir, repo_root, conventions)
    if only is not None:
        if only not in now:
            raise ValueError(f"no table {only!r} in {migrations_dir}{_did_you_mean(only, now)}")
        now = {only: now[only]}
    with tempfile.TemporaryDirectory() as temporary:
        before = _gaps_by_table(_migrations_at(repo_root, base, relative, Path(temporary)), Path(temporary), conventions)
    if only is not None:
        before = {only: before.get(only, [])}
    tables = {}
    for table in sorted(set(now) | set(before)):
        opened = [gap for gap in now.get(table, []) if gap not in before.get(table, [])]
        closed = [gap for gap in before.get(table, []) if gap not in now.get(table, [])]
        if opened or closed:
            tables[table] = {"new_gaps": opened, "closed_gaps": closed}
    return {"base": base, "tables": tables, "new_gaps": sum(len(entry["new_gaps"]) for entry in tables.values())}


def _migrations_at(repo_root: Path, base: str, relative: str, into: Path) -> Path:
    failure = f"could not read {relative} at {base}"
    listed = checked_git_bytes(repo_root, "ls-tree", "-d", base, "--", relative, failure=failure)
    target = into / relative
    if not listed.stdout.strip():
        target.mkdir(parents=True)
        return target
    archive = checked_git_bytes(repo_root, "archive", "--format=tar", base, "--", relative, failure=failure)
    with tarfile.open(fileobj=io.BytesIO(archive.stdout)) as tar:
        members = [member for member in tar.getmembers() if member.isfile() and ".." not in Path(member.name).parts]
        tar.extractall(into, members=members)
    return target


def _gaps_by_table(migrations_dir: Path, repo_root: Path, conventions: Conventions) -> dict[str, list[str]]:
    schema = read_schema(migrations_dir, repo_root)
    return {name: _gaps(table, schema, conventions) for name, table in live_tables(schema).items()}


def dynamic_for(schema: Schema, table: str, constraints: list[Constraint]) -> list[Dynamic]:
    return [statement for statement in schema.dynamic if table in statement.tables and not _restated(statement, constraints)]


def _restated(statement: Dynamic, constraints: list[Constraint]) -> bool:
    return any(c.kind == "trigger" and c.name in statement.triggers and (c.file, c.line) > (statement.file, statement.line)
               for c in constraints)


def _gaps(table: Table, schema: Schema, conventions: Conventions) -> list[str]:
    constraints = [c for c in schema.constraints if c.table == table.name]
    keys = [k for k in schema.foreign_keys if k.child == table.name]
    dynamic = dynamic_for(schema, table.name, constraints)
    gaps = [] if table.comment is not None else ["the table has no COMMENT ON saying what one row is"]
    for column, info in table.columns.items():
        gaps.extend(_column_gaps(column, info, schema, constraints, conventions))
    gaps.extend(f"foreign key {key.name} to {key.parent} does not say ON DELETE; choose CASCADE, RESTRICT, SET NULL or NO ACTION"
                for key in keys if key.on_delete is None)
    gaps.extend(f"the statement at {statement.file}:{statement.line} builds DDL at run time and names {table.name}; "
                "the files cannot show what it creates, so restate it on this table with a static CREATE TRIGGER of the trigger it names" for statement in dynamic)
    return gaps


def _column_gaps(column: str, info: dict[str, Any], schema: Schema, constraints: list[Constraint],
                 conventions: Conventions) -> list[str]:
    gaps = []
    listed = value_list(column, constraints, schema.enums.get(info["type"]))
    comment = info.get("comment")
    explained_in = " ".join(text for text in (comment, schema.type_comments.get(info["type"])) if text)
    if comment is None and column not in conventions.exempt_columns:
        gaps.append(f"column {column} has no COMMENT ON saying what it means")
    if listed is None and conventions.status_like.search(column) and not _declared_open(column, comment, constraints, conventions):
        gaps.append(f"status-like column {column} has no value list; add CHECK ({column} IN (...)) or an enum type, "
                    f"or begin its comment with '{conventions.marker}' or constrain it with another CHECK")
    if listed and not info.get("not_null") and not listed.null_decided and info["type"] not in schema.enums:
        gaps.append(f"column {column} lists its values without deciding NULL; make it NOT NULL, or write "
                    f"CHECK ({column} IS NULL OR {column} IN (...)) or CHECK ({column} IS NOT NULL AND {column} IN (...))")
    unexplained = [value for value in listed.values if not _mentions_value(explained_in, value)] if listed and explained_in else []
    if unexplained and len(listed.values) <= conventions.value_comment_limit:
        gaps.append(f"the comment on {column} does not say what these values mean: {', '.join(unexplained)}")
    return gaps


def _declared_open(column: str, comment: str | None, constraints: list[Constraint], conventions: Conventions) -> bool:
    return bool(comment and conventions.free_text.search(comment)) or any(c.kind == "check" and column in c.columns for c in constraints)


def _mentions_value(text: str, value: str) -> bool:
    return re.search(rf"(?<![\w-]){re.escape(value)}(?![\w-])", text) is not None


def _constraint(constraint: Constraint) -> dict[str, Any]:
    return {"kind": constraint.kind, "name": constraint.name, "text": one_line(constraint.text),
            "at": f"{constraint.file}:{constraint.line}", **({"clause": constraint.clause} if constraint.clause else {})}


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
