from __future__ import annotations

import argparse
import json
import re
import sys
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

IDENT = r"[a-zA-Z_][a-zA-Z0-9_]*"
CREATE_TABLE = re.compile(
    rf"^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?({IDENT})\s*(\(|PARTITION\s+OF\s+({IDENT}))", re.I)
DROP_TABLE = re.compile(rf"^\s*DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?({IDENT})", re.I)
ALTER_TABLE = re.compile(rf"^\s*ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?({IDENT})\s+(.*)$", re.I | re.S)
ADD_COLUMN = re.compile(rf"^ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?({IDENT})\s+", re.I)
DROP_COLUMN = re.compile(rf"^DROP\s+(?:COLUMN\s+)?(?:IF\s+EXISTS\s+)?({IDENT})", re.I)
ADD_CONSTRAINT = re.compile(
    rf"^ADD\s+(?:CONSTRAINT\s+({IDENT})\s+)?(CHECK|UNIQUE|FOREIGN\s+KEY|PRIMARY\s+KEY|EXCLUDE)\b", re.I)
OTHER_ALTER = re.compile(
    r"^(ALTER\s+(COLUMN\s+)?|DROP\s+CONSTRAINT|RENAME|SET|RESET|ENABLE|DISABLE|ATTACH|DETACH|VALIDATE"
    r"|OWNER|CLUSTER|REPLICA|NO\s+FORCE|FORCE)", re.I)
CREATE_INDEX = re.compile(
    rf"^\s*CREATE\s+(UNIQUE\s+)?INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?({IDENT})\s+ON\s+(?:ONLY\s+)?({IDENT})",
    re.I)
PARTIAL_INDEX = re.compile(r"\)\s*WHERE\b", re.I)
CREATE_TRIGGER = re.compile(rf"^\s*CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+({IDENT})\s.*?\bON\s+({IDENT})", re.I | re.S)
COMMENT_ON = re.compile(rf"^\s*COMMENT\s+ON\s+(TABLE|COLUMN)\s+({IDENT})(?:\.({IDENT}))?\s+IS\s+", re.I)
CREATE_TYPE = re.compile(rf"^\s*CREATE\s+(TYPE|DOMAIN)\s+({IDENT})", re.I)
IGNORED_STATEMENT = re.compile(
    r"^(CREATE\s+(OR\s+REPLACE\s+)?(FUNCTION|VIEW)|CREATE\s+(EXTENSION|SCHEMA|POLICY|SEQUENCE)"
    r"|DROP\s+(INDEX|TRIGGER|FUNCTION|TYPE|EXTENSION)|ALTER\s+(INDEX|TYPE|SEQUENCE|FUNCTION)"
    r"|LOCK\s+TABLE|GRANT|REVOKE|INSERT|UPDATE|DELETE|SET|SELECT|DO\s+\$|BEGIN|COMMIT)", re.I)
REFERENCES = re.compile(rf"\bREFERENCES\s+({IDENT})", re.I)
COLUMN_LINE = re.compile(rf"^\s*({IDENT})\s+\S")
TABLE_LEVEL = re.compile(r"^\s*(CONSTRAINT|PRIMARY\s+KEY|UNIQUE|CHECK|FOREIGN\s+KEY|EXCLUDE|LIKE)\b", re.I)
CONSTRAINT_KIND = re.compile(r"\s*(CHECK|UNIQUE|EXCLUDE|FOREIGN|PRIMARY|LIKE)\b", re.I)
NAMED_CONSTRAINT = re.compile(rf"CONSTRAINT\s+({IDENT})\s+(.*)$", re.I | re.S)
TRAILING_COMMENT = re.compile(r"[ \t]*--[^\n]*")
COLUMN_CHECK = re.compile(r"\bCHECK\s*\(", re.I)
COLUMN_UNIQUE = re.compile(r"\bUNIQUE\b", re.I)
QUERY_NAME = re.compile(r"^--\s*name:\s*(\S+)\s+:(\w+)", re.M)
QUERY_TABLES = re.compile(rf"\b(?:FROM|JOIN|INTO|UPDATE|DELETE\s+FROM)\s+(?:ONLY\s+)?({IDENT})", re.I)
WRITES = re.compile(r"\b(INSERT\s+INTO|UPDATE|DELETE\s+FROM)\b", re.I)
CITATION = re.compile(r"^(?P<path>[^#]+)#L(?P<start>\d+)(?:-(?P<end>\d+))?$")
CITED_RESOURCE = re.compile(r'resource:\s*"([^"]+)"')
CONTEXT_LINK = re.compile(r"\(/contexts/([a-z0-9-]+)\)")
RECORD_TYPE = re.compile(r"^type:\s*(.+)$", re.M)
RECORD_CONSISTENCY = re.compile(r"^consistency:\s*(.+)$", re.M)
SENSITIVE = re.compile(
    r"(^|_)(email|token|secret|password|passwd|phone|dsn|credential|ssn|api_key|ip_address|client_ip)(_|$)", re.I)
KNOWN_CONSISTENCIES = {"synchronous", "eventual"}
OPAQUE_SPANS = (("--", "\n"), ("$$", "$$"), ("'", "'"))


@dataclass
class Table:
    name: str
    file: str
    line: int
    columns: dict[str, dict[str, Any]] = field(default_factory=dict)
    described: bool = False
    partition_of: str | None = None


@dataclass
class Constraint:
    kind: str
    table: str
    file: str
    line: int
    name: str


@dataclass(frozen=True)
class ForeignKey:
    child: str
    parent: str
    file: str
    line: int


@dataclass
class Schema:
    tables: dict[str, Table] = field(default_factory=dict)
    constraints: list[Constraint] = field(default_factory=list)
    foreign_keys: list[ForeignKey] = field(default_factory=list)
    types: list[str] = field(default_factory=list)
    unparsed: list[str] = field(default_factory=list)
    migrations: list[str] = field(default_factory=list)


def _opaque_end(text: str, index: int) -> int | None:
    for opener, closer in OPAQUE_SPANS:
        if text.startswith(opener, index):
            close = text.find(closer, index + len(opener))
            if close == -1:
                return len(text)
            return close if closer == "\n" else close + len(closer)
    return None


def _statements(text: str) -> list[tuple[int, str]]:
    found: list[tuple[int, str]] = []
    buffer: list[str] = []
    line, start, depth, index = 1, 0, 0, 0
    while index < len(text):
        end = _opaque_end(text, index)
        if end is not None:
            if start == 0 and not text.startswith("--", index):
                start = line
            buffer.append(text[index:end])
            line += text.count("\n", index, end)
            index = end
            continue
        char = text[index]
        depth += (char == "(") - (char == ")")
        if char == ";" and depth <= 0:
            statement = "".join(buffer).strip()
            if statement:
                found.append((start, statement))
            buffer, start = [], 0
        else:
            if start == 0 and not char.isspace():
                start = line
            buffer.append(char)
            line += char == "\n"
        index += 1
    tail = "".join(buffer).strip()
    if tail:
        found.append((start, tail))
    return found


def _strip_comments(statement: str) -> str:
    return "\n".join(line.split("--", 1)[0] for line in statement.split("\n"))


def _table_body(statement: str) -> str:
    open_at = statement.index("(")
    depth = 0
    for index in range(open_at, len(statement)):
        if statement[index] == "(":
            depth += 1
        elif statement[index] == ")":
            depth -= 1
            if depth == 0:
                return statement[open_at + 1:index]
    return statement[open_at + 1:]


def _split_top_level(body: str) -> list[str]:
    parts, depth, current = [], 0, []
    for char in body:
        if char == "(":
            depth += 1
        elif char == ")":
            depth -= 1
        if char == "," and depth == 0:
            parts.append("".join(current))
            current = []
        else:
            current.append(char)
    parts.append("".join(current))
    return parts


def _with_trailing_comments_kept_on_their_line(parts: list[str]) -> list[str]:
    kept = list(parts)
    for index in range(1, len(kept)):
        comment = TRAILING_COMMENT.match(kept[index])
        if comment:
            kept[index - 1] += comment.group(0)
            kept[index] = kept[index][comment.end():]
    return kept


def _table_parts(statement: str, line: int) -> list[tuple[int, str]]:
    offset = statement[: statement.index("(")].count("\n")
    parts = []
    for part in _with_trailing_comments_kept_on_their_line(_split_top_level(_table_body(statement))):
        stripped = _strip_comments(part)
        leading = stripped[: len(stripped) - len(stripped.lstrip())]
        parts.append((line + offset + leading.count("\n"), part))
        offset += part.count("\n")
    return parts


def _read_table(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    name = match.group(1)
    table = Table(name, file, line, partition_of=match.group(3))
    schema.tables[name] = table
    if table.partition_of:
        return
    for part_line, part in _table_parts(statement, line):
        text = _strip_comments(part).strip()
        if not text:
            continue
        if TABLE_LEVEL.match(text):
            _read_table_constraint(schema, name, file, part_line, text)
            continue
        column = COLUMN_LINE.match(text)
        if column is None:
            schema.unparsed.append(f"{file}:{part_line}: {text[:60]}")
            continue
        table.columns[column.group(1)] = {"line": part_line, "described": "--" in part}
        _read_column_rules(schema, name, file, part_line, text)


def _read_column_rules(schema: Schema, table: str, file: str, line: int, text: str) -> None:
    column = f"{table}.{text.split()[0]}"
    if COLUMN_CHECK.search(text):
        schema.constraints.append(Constraint("check", table, file, line, column))
    if COLUMN_UNIQUE.search(text):
        schema.constraints.append(Constraint("unique", table, file, line, column))
    referenced = REFERENCES.search(text)
    if referenced:
        schema.foreign_keys.append(ForeignKey(table, referenced.group(1), file, line))


def _read_table_constraint(schema: Schema, table: str, file: str, line: int, text: str) -> None:
    named = NAMED_CONSTRAINT.match(text)
    label = named.group(1) if named else f"{table}:{line}"
    clause = named.group(2) if named else text
    leading = CONSTRAINT_KIND.match(clause)
    kind = leading.group(1).upper() if leading else ""
    if kind in {"CHECK", "UNIQUE", "EXCLUDE"}:
        schema.constraints.append(Constraint(kind.lower(), table, file, line, label))
    elif kind == "FOREIGN":
        referenced = REFERENCES.search(clause)
        if referenced:
            schema.foreign_keys.append(ForeignKey(table, referenced.group(1), file, line))
    elif kind not in {"PRIMARY", "LIKE"}:
        schema.unparsed.append(f"{file}:{line}: {text[:60]}")


def _read_alter(schema: Schema, file: str, line: int, _statement: str, match: re.Match[str]) -> None:
    table = match.group(1)
    if table not in schema.tables:
        schema.unparsed.append(f"{file}:{line}: ALTER TABLE {table} before its CREATE")
        return
    for action in _split_top_level(_strip_comments(match.group(2))):
        text = action.strip()
        if not text:
            continue
        added = ADD_COLUMN.match(text)
        dropped = DROP_COLUMN.match(text)
        if ADD_CONSTRAINT.match(text):
            _read_table_constraint(schema, table, file, line, text[len("ADD "):].strip())
        elif added:
            schema.tables[table].columns[added.group(1)] = {"line": line, "described": False}
            _read_column_rules(schema, table, file, line, text[len("ADD "):].replace("COLUMN ", "", 1).strip())
        elif dropped:
            schema.tables[table].columns.pop(dropped.group(1), None)
        elif not OTHER_ALTER.match(text):
            schema.unparsed.append(f"{file}:{line}: ALTER TABLE {table} {text[:50]}")


def _read_drop(schema: Schema, _file: str, _line: int, _statement: str, match: re.Match[str]) -> None:
    schema.tables.pop(match.group(1), None)


def _read_index(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    unique, name, table = match.group(1), match.group(2), match.group(3)
    partial = PARTIAL_INDEX.search(_strip_comments(statement)) is not None
    if unique or partial:
        schema.constraints.append(Constraint("unique-index" if unique else "partial-index", table, file, line, name))


def _read_trigger(schema: Schema, file: str, line: int, _statement: str, match: re.Match[str]) -> None:
    schema.constraints.append(Constraint("trigger", match.group(2), file, line, match.group(1)))


def _read_comment(schema: Schema, file: str, line: int, _statement: str, match: re.Match[str]) -> None:
    table = schema.tables.get(match.group(2))
    if table is None:
        schema.unparsed.append(f"{file}:{line}: COMMENT ON unknown table {match.group(2)}")
    elif match.group(1).upper() == "TABLE":
        table.described = True
    elif match.group(3) in table.columns:
        table.columns[match.group(3)]["described"] = True


def _read_type(schema: Schema, _file: str, _line: int, _statement: str, match: re.Match[str]) -> None:
    schema.types.append(match.group(2))


Reader = Callable[[Schema, str, int, str, "re.Match[str]"], None]
READERS: list[tuple[re.Pattern[str], Reader]] = [
    (CREATE_TABLE, _read_table), (DROP_TABLE, _read_drop), (ALTER_TABLE, _read_alter), (CREATE_INDEX, _read_index),
    (CREATE_TRIGGER, _read_trigger), (COMMENT_ON, _read_comment), (CREATE_TYPE, _read_type),
]


def _read_statement(schema: Schema, file: str, line: int, statement: str) -> None:
    head = _strip_comments(statement).strip()
    for pattern, reader in READERS:
        match = pattern.match(head)
        if match:
            reader(schema, file, line, statement, match)
            return
    if not IGNORED_STATEMENT.match(head):
        schema.unparsed.append(f"{file}:{line}: {head[:60]}")


def read_schema(migrations_dir: Path, repo_root: Path) -> Schema:
    schema = Schema()
    for path in sorted(migrations_dir.glob("*.sql")):
        file = path.relative_to(repo_root).as_posix()
        schema.migrations.append(file)
        for line, statement in _statements(path.read_text(encoding="utf-8")):
            _read_statement(schema, file, line, statement)
    return schema


@dataclass
class Query:
    name: str
    file: str
    tables: list[str]
    writes: bool


def _query_tables(body: str, known_tables: set[str]) -> list[str]:
    tables: list[str] = []
    for name in QUERY_TABLES.findall(body):
        lowered = name.lower()
        if lowered in known_tables and lowered not in tables:
            tables.append(lowered)
    return tables


def read_queries(queries_dir: Path, known_tables: set[str]) -> list[Query]:
    queries: list[Query] = []
    for path in sorted(queries_dir.glob("*.sql")):
        text = path.read_text(encoding="utf-8")
        marks = list(QUERY_NAME.finditer(text))
        for index, mark in enumerate(marks):
            end = marks[index + 1].start() if index + 1 < len(marks) else len(text)
            body = _strip_comments(text[mark.end():end])
            queries.append(Query(mark.group(1), path.name, _query_tables(body, known_tables), WRITES.search(body) is not None))
    return queries


def read_owners(path: Path) -> dict[str, dict[str, list[str]]]:
    sections: dict[str, dict[str, list[str]]] = {}
    current: dict[str, list[str]] | None = None
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.rstrip()
        if not line or line.lstrip().startswith("#"):
            continue
        if not line.startswith(" ") and line.endswith(":"):
            current = sections.setdefault(line[:-1], {})
        elif current is not None and line.startswith("  ") and ":" in line:
            key, _, value = line.strip().partition(":")
            current[key.strip()] = [item.strip() for item in value.split(",") if item.strip()]
    return sections


@dataclass
class Record:
    asset: str
    name: str
    kind: str
    citations: list[tuple[str, int, int]]
    contexts: list[str]
    consistency: str | None
    text: str

    def cites(self, file: str, line: int) -> bool:
        return any(path == file and start <= line <= end for path, start, end in self.citations)


def _frontmatter(text: str) -> tuple[str, str]:
    if not text.startswith("---"):
        return "", text
    end = text.find("\n---", 3)
    return (text[3:end], text[end + 4:]) if end != -1 else ("", text)


def _read_record(path: Path) -> Record:
    front, body = _frontmatter(path.read_text(encoding="utf-8"))
    citations = []
    for resource in CITED_RESOURCE.findall(front):
        cited = CITATION.match(resource)
        if cited:
            start = int(cited.group("start"))
            citations.append((cited.group("path"), start, int(cited.group("end") or start)))
    kind = RECORD_TYPE.search(front)
    consistency = RECORD_CONSISTENCY.search(front)
    return Record(
        path.parent.name, path.stem, kind.group(1).strip() if kind else "", citations, CONTEXT_LINK.findall(body),
        consistency.group(1).strip().strip('"') if consistency else None, body,
    )


def read_registry(registry_root: Path) -> list[Record]:
    registry = registry_root / "registry"
    return [_read_record(path) for path in sorted(registry.glob("*/*.md")) if path.name != "index.md"]


def _ratio(numerator: int, denominator: int) -> dict[str, Any]:
    return {"numerator": numerator, "denominator": denominator,
            "ratio": round(numerator / denominator, 3) if denominator else None}


def _mentions(records: list[Record], name: str) -> bool:
    pattern = re.compile(rf"\b{re.escape(name)}\b")
    return any(pattern.search(record.text) for record in records)


@dataclass
class Inputs:
    schema: Schema
    queries: list[Query]
    owners: dict[str, dict[str, list[str]]]
    records: list[Record]

    @property
    def tables(self) -> dict[str, Table]:
        return {name: table for name, table in self.schema.tables.items() if table.partition_of is None}

    def table_owners(self, table: str) -> list[str]:
        return self.owners.get("tables", {}).get(table, [])

    def query_owner(self, query: Query) -> str:
        declared = self.owners.get("queries", {}).get(query.name) or self.owners.get("files", {}).get(query.file) or [""]
        return declared[0]

    def cited(self, file: str, line: int) -> bool:
        return any(record.cites(file, line) for record in self.records)

    @property
    def contexts(self) -> set[str]:
        return {record.name for record in self.records if record.asset == "contexts"}


def _meaning(inputs: Inputs) -> dict[str, Any]:
    tables = inputs.tables
    columns = [info for table in tables.values() for info in table.columns.values()]
    return {
        "meaning.tables_described": _ratio(sum(table.described for table in tables.values()), len(tables)),
        "meaning.columns_described": _ratio(sum(info["described"] for info in columns), len(columns)),
        "meaning.tables_known_to_registry": _ratio(sum(_mentions(inputs.records, name) for name in tables), len(tables)),
    }


def _writers(inputs: Inputs) -> dict[str, set[str]]:
    writers: dict[str, set[str]] = {}
    for query in inputs.queries:
        if query.writes:
            for table in query.tables:
                writers.setdefault(table, set()).add(inputs.query_owner(query))
    return {table: owners - {""} for table, owners in writers.items()}


def _ownership(inputs: Inputs) -> tuple[dict[str, Any], dict[str, Any]]:
    tables = inputs.tables
    writers = _writers(inputs)
    touched = {table for query in inputs.queries for table in query.tables}
    claimed = {
        name for record in inputs.records if record.asset == "aggregates"
        for name in tables if re.search(rf"\b{name}\b", record.text)
    }
    contexts = inputs.contexts
    foreign = {name: [owner for owner in inputs.table_owners(name) if owner not in contexts] for name in tables}
    metrics = {
        "ownership.tables_with_exactly_one_declared_owner": _ratio(
            sum(len(inputs.table_owners(name)) == 1 for name in tables), len(tables)),
        "ownership.tables_owned_by_registry_contexts_only": _ratio(
            sum(not owners for owners in foreign.values()), len(tables)),
        "ownership.tables_written_by_at_most_one_owner": _ratio(
            sum(len(writers.get(name, set())) <= 1 for name in tables), len(tables)),
        "ownership.tables_claimed_by_an_aggregate": _ratio(len(claimed), len(tables)),
        "ownership.tables_touched_by_a_query": _ratio(sum(name in touched for name in tables), len(tables)),
    }
    details = {
        "tables_without_one_owner": {name: inputs.table_owners(name) for name in sorted(tables)
                                     if len(inputs.table_owners(name)) != 1},
        "tables_owned_outside_the_registry": {name: owners for name, owners in sorted(foreign.items()) if owners},
        "tables_untouched": sorted(name for name in tables if name not in touched),
        "multi_writer_tables": {name: sorted(owners) for name, owners in sorted(writers.items()) if len(owners) > 1},
    }
    return metrics, details


def _cross_context(inputs: Inputs, key: ForeignKey) -> tuple[str, str] | None:
    child, parent = inputs.table_owners(key.child), inputs.table_owners(key.parent)
    if child and parent and not set(child) & set(parent):
        return child[0], parent[0]
    return None


def _rules(inputs: Inputs) -> tuple[dict[str, Any], dict[str, Any]]:
    tables = inputs.tables
    constraints = [constraint for constraint in inputs.schema.constraints if constraint.table in tables]
    cited = [constraint for constraint in constraints if inputs.cited(constraint.file, constraint.line)]
    crossing = {key: pair for key in inputs.schema.foreign_keys if (pair := _cross_context(inputs, key))}
    policies = {
        (record.contexts[0], record.contexts[1]) for record in inputs.records
        if record.asset == "dependency-policies" and len(record.contexts) > 1
    }
    covered = [key for key, pair in crossing.items() if pair in policies]
    metrics = {
        "rules.constraints_cited_by_a_record": _ratio(len(cited), len(constraints)),
        "rules.foreign_keys_inside_one_context": _ratio(
            len(inputs.schema.foreign_keys) - len(crossing), len(inputs.schema.foreign_keys)),
        "rules.cross_context_foreign_keys_with_a_dependency_policy": _ratio(len(covered), len(crossing)),
    }
    details = {
        "uncited_constraints": sorted(f"{c.file}:{c.line} {c.kind} {c.name}" for c in constraints if c not in cited),
        "cross_context_foreign_keys_without_a_policy": sorted({
            f"{key.child}->{key.parent} ({pair[0]}->{pair[1]})" for key, pair in crossing.items() if pair not in policies}),
    }
    return metrics, details


def _access_and_sensitivity(inputs: Inputs) -> tuple[dict[str, Any], dict[str, Any]]:
    tables = inputs.tables
    sensitive = [(name, column) for name, table in tables.items() for column in table.columns if SENSITIVE.search(column)]
    registered = [(name, column) for name, column in sensitive
                  if inputs.cited(tables[name].file, tables[name].columns[column]["line"])]
    metrics = {
        "access.queries_with_an_owner": _ratio(
            sum(bool(inputs.query_owner(query)) for query in inputs.queries), len(inputs.queries)),
        "sensitivity.candidate_columns_cited_by_a_record": _ratio(len(registered), len(sensitive)),
    }
    details = {"sensitive_candidates": sorted(f"{name}.{column}" for name, column in sensitive)}
    return metrics, details


def _consistency_and_provenance(inputs: Inputs) -> dict[str, Any]:
    tables = inputs.tables
    interactions = [record for record in inputs.records if record.asset == "interactions"]
    cited_migrations = {
        path for record in inputs.records for path, _, _ in record.citations if path in inputs.schema.migrations}
    immutable = list(inputs.owners.get("immutable", {}))
    retention = inputs.owners.get("retention", {})
    return {
        "consistency.interactions_with_a_known_consistency": _ratio(
            sum(record.consistency in KNOWN_CONSISTENCIES for record in interactions), len(interactions)),
        "provenance.migrations_cited_by_a_record": _ratio(len(cited_migrations), len(inputs.schema.migrations)),
        "provenance.immutable_tables_known_to_registry": _ratio(
            sum(_mentions(inputs.records, name) for name in immutable), len(immutable)),
        "provenance.tables_with_a_retention_policy": _ratio(sum(bool(retention.get(name)) for name in tables), len(tables)),
    }


def read_inputs(repo_root: Path, registry_root: Path) -> Inputs:
    schema = read_schema(repo_root / "db" / "migrations", repo_root)
    return Inputs(
        schema, read_queries(repo_root / "db" / "queries", set(schema.tables)),
        read_owners(repo_root / "db" / "query-owners.yaml"), read_registry(registry_root),
    )


def measure(inputs: Inputs) -> dict[str, Any]:
    tables = inputs.tables
    ownership, ownership_details = _ownership(inputs)
    rules, rule_details = _rules(inputs)
    access, access_details = _access_and_sensitivity(inputs)
    return {
        "inventory": {
            "migrations": len(inputs.schema.migrations), "tables": len(tables),
            "partitions": len(inputs.schema.tables) - len(tables),
            "columns": sum(len(table.columns) for table in tables.values()), "queries": len(inputs.queries),
            "constraints": sum(constraint.table in tables for constraint in inputs.schema.constraints),
            "foreign_keys": len(inputs.schema.foreign_keys), "types": len(inputs.schema.types),
            "registry_records": len(inputs.records), "unparsed_statements": len(inputs.schema.unparsed),
        },
        "metrics": {**_meaning(inputs), **ownership, **rules, **access, **_consistency_and_provenance(inputs)},
        "details": {"unparsed_statements": inputs.schema.unparsed, **ownership_details, **rule_details, **access_details},
    }


def render(report: dict[str, Any], baseline: dict[str, Any] | None) -> str:
    lines = ["| metric | now | ratio | baseline | delta |", "| --- | --- | --- | --- | --- |"]
    base_metrics = (baseline or {}).get("metrics", {})
    for name, value in report["metrics"].items():
        ratio = value["ratio"]
        base = base_metrics.get(name, {}).get("ratio")
        delta = "" if ratio is None or base is None else f"{ratio - base:+.3f}"
        shown = "n/a" if ratio is None else f"{ratio:.1%}"
        shown_base = "" if base is None else f"{base:.1%}"
        lines.append(f"| {name} | {value['numerator']}/{value['denominator']} | {shown} | {shown_base} | {delta} |")
    inventory = ", ".join(f"{key} {value}" for key, value in report["inventory"].items())
    return f"{inventory}\n\n" + "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Measure how AI-ready the data layer is, from db/ and the Domain Memory, without a database.")
    parser.add_argument("--repo-root", type=Path, default=Path.cwd())
    parser.add_argument("--registry-root", type=Path, default=None)
    parser.add_argument("--baseline", type=Path, default=None)
    parser.add_argument("--json", type=Path, default=None, help="write the full report here")
    args = parser.parse_args(argv)
    repo_root = args.repo_root.resolve()
    registry_root = (args.registry_root or repo_root / "docs" / "domain-memory").resolve()
    report = measure(read_inputs(repo_root, registry_root))
    baseline = json.loads(args.baseline.read_text(encoding="utf-8")) if args.baseline else None
    if args.json:
        args.json.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8", newline="\n")
    print(render(report, baseline))
    unparsed = report["inventory"]["unparsed_statements"]
    if unparsed:
        print(f"\n{unparsed} statement(s) not read; see details.unparsed_statements", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
