from __future__ import annotations

import re
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
RENAME_COLUMN = re.compile(rf"^RENAME\s+(?:COLUMN\s+)?({IDENT})\s+TO\s+({IDENT})", re.I)
ADD_CONSTRAINT = re.compile(
    rf"^ADD\s+(?:CONSTRAINT\s+({IDENT})\s+)?(CHECK|UNIQUE|FOREIGN\s+KEY|PRIMARY\s+KEY|EXCLUDE)\b", re.I)
OTHER_ALTER = re.compile(
    r"^(ALTER\s+(COLUMN\s+)?|DROP\s+CONSTRAINT|RENAME|SET|RESET|ENABLE|DISABLE|ATTACH|DETACH|VALIDATE"
    r"|OWNER|CLUSTER|REPLICA|NO\s+FORCE|FORCE)", re.I)
CREATE_INDEX = re.compile(
    rf"^\s*CREATE\s+(UNIQUE\s+)?INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?({IDENT})\s+ON\s+"
    rf"(?:ONLY\s+)?({IDENT})", re.I)
CREATE_TRIGGER = re.compile(
    rf"^\s*CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+({IDENT})\s.*?\bON\s+({IDENT})", re.I | re.S)
COMMENT_ON = re.compile(rf"^\s*COMMENT\s+ON\s+(TABLE|COLUMN|TYPE)\s+({IDENT})(?:\.({IDENT}))?\s+IS\s+", re.I)
COMMENT_TEXT = re.compile(r"\bIS\s+('(?:[^']|'')*'(?:\s*'(?:[^']|'')*')*)", re.I | re.S)
STRING_LITERAL = re.compile(r"'((?:[^']|'')*)'", re.S)
CREATE_TYPE = re.compile(rf"^\s*CREATE\s+(TYPE|DOMAIN)\s+({IDENT})", re.I)
ENUM_VALUES = re.compile(r"\bAS\s+ENUM\s*\((.*)\)", re.I | re.S)
ADD_ENUM_VALUE = re.compile(
    rf"^\s*ALTER\s+TYPE\s+({IDENT})\s+ADD\s+VALUE\s+(?:IF\s+NOT\s+EXISTS\s+)?'((?:[^']|'')*)'", re.I)
DROP_CONSTRAINT = re.compile(rf"^DROP\s+CONSTRAINT\s+(?:IF\s+EXISTS\s+)?({IDENT})", re.I)
DROP_INDEX = re.compile(rf"^\s*DROP\s+INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+EXISTS\s+)?({IDENT})", re.I)
DROP_TRIGGER = re.compile(rf"^\s*DROP\s+TRIGGER\s+(?:IF\s+EXISTS\s+)?({IDENT})\s+ON\s+({IDENT})", re.I)
IGNORED_STATEMENT = re.compile(
    r"^(CREATE\s+(OR\s+REPLACE\s+)?(FUNCTION|VIEW)|CREATE\s+(EXTENSION|SCHEMA|POLICY|SEQUENCE)"
    r"|DROP\s+(FUNCTION|TYPE|EXTENSION)|ALTER\s+(INDEX|TYPE|SEQUENCE|FUNCTION)"
    r"|LOCK\s+TABLE|GRANT|REVOKE|INSERT|UPDATE|DELETE|SET|SELECT|DO\s+\$|BEGIN|COMMIT)", re.I)
UNIQUE_INDEX = "unique-index"
REFERENCES = re.compile(rf"\bREFERENCES\s+({IDENT})", re.I)
COLUMN_LINE = re.compile(
    rf"^\s*({IDENT})\s+(.+?)(?=\s+(?:NOT|NULL|DEFAULT|CHECK|REFERENCES|PRIMARY|UNIQUE|GENERATED|COLLATE|CONSTRAINT|EXCLUDE)\b|\s*$)",
    re.I | re.S)
TABLE_LEVEL = re.compile(r"^\s*(CONSTRAINT|PRIMARY\s+KEY|UNIQUE|CHECK|FOREIGN\s+KEY|EXCLUDE|LIKE)\b", re.I)
CONSTRAINT_KIND = re.compile(r"\s*(CHECK|UNIQUE|EXCLUDE|FOREIGN|PRIMARY|LIKE)\b", re.I)
NAMED_CONSTRAINT = re.compile(rf"CONSTRAINT\s+({IDENT})\s+(.*)$", re.I | re.S)
TRAILING_COMMENT = re.compile(r"[ \t]*--[^\n]*")
COLUMN_CHECK = re.compile(r"\bCHECK\s*\(", re.I)
COLUMN_UNIQUE = re.compile(r"\bUNIQUE\b", re.I)
OPAQUE_SPANS = (("--", "\n"), ("$$", "$$"), ("'", "'"))


@dataclass
class Table:
    name: str
    file: str
    line: int
    columns: dict[str, dict[str, Any]] = field(default_factory=dict)
    described: bool = False
    partition_of: str | None = None
    comment: str | None = None


@dataclass(frozen=True)
class Span:
    file: str
    first: int
    last: int


@dataclass(frozen=True)
class Constraint:
    kind: str
    table: str
    file: str
    line: int
    end: int
    name: str
    text: str

    @classmethod
    def at(cls, kind: str, table: str, span: Span, name: str, text: str) -> Constraint:
        return cls(kind, table, span.file, span.first, span.last, name, one_line(text))


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
    enums: dict[str, list[str]] = field(default_factory=dict)
    type_comments: dict[str, str] = field(default_factory=dict)
    unparsed: list[str] = field(default_factory=list)
    migrations: list[str] = field(default_factory=list)


def _declares_the_same(earlier: Constraint, later: Constraint) -> bool:
    if UNIQUE_INDEX in (earlier.kind, later.kind):
        return False
    return (earlier.kind == "trigger") == (later.kind == "trigger") and (earlier.table, earlier.name) == (later.table, later.name)


def _declare(schema: Schema, constraint: Constraint) -> None:
    schema.constraints = [kept for kept in schema.constraints if not _declares_the_same(kept, constraint)]
    schema.constraints.append(constraint)


def _undeclare(schema: Schema, keep: Callable[[Constraint], bool]) -> None:
    schema.constraints = [constraint for constraint in schema.constraints if keep(constraint)]


def _opaque_end(text: str, index: int) -> int | None:
    for opener, closer in OPAQUE_SPANS:
        if text.startswith(opener, index):
            close = text.find(closer, index + len(opener))
            if close == -1:
                return len(text)
            return close if closer == "\n" else close + len(closer)
    return None


def statements(text: str) -> list[tuple[int, str]]:
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
            statement = _without_leading_comments(buffer)
            if statement:
                found.append((start, statement))
            buffer, start = [], 0
        else:
            if start == 0 and not char.isspace():
                start = line
            buffer.append(char)
            line += char == "\n"
        index += 1
    tail = _without_leading_comments(buffer)
    if tail:
        found.append((start, tail))
    return found


def _without_leading_comments(buffer: list[str]) -> str:
    statement = "".join(buffer).strip()
    while statement.startswith("--"):
        _, _, statement = statement.partition("\n")
        statement = statement.strip()
    return statement


def strip_comments(statement: str) -> str:
    return "\n".join(line.split("--", 1)[0] for line in statement.split("\n"))


def one_line(text: str) -> str:
    return " ".join(strip_comments(text).split())


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
    parts, depth, current, index = [], 0, [], 0
    while index < len(body):
        end = _opaque_end(body, index)
        if end is not None:
            current.append(body[index:end])
            index = end
            continue
        char = body[index]
        depth += (char == "(") - (char == ")")
        if char == "," and depth == 0:
            parts.append("".join(current))
            current = []
        else:
            current.append(char)
        index += 1
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


def _table_parts(statement: str, line: int) -> list[tuple[int, int, str]]:
    offset = statement[: statement.index("(")].count("\n")
    parts = []
    for part in _with_trailing_comments_kept_on_their_line(_split_top_level(_table_body(statement))):
        stripped = strip_comments(part)
        leading = stripped[: len(stripped) - len(stripped.lstrip())]
        first = line + offset + leading.count("\n")
        parts.append((first, first + stripped.strip().count("\n"), part))
        offset += part.count("\n")
    return parts


def _read_table(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    name = match.group(1)
    table = Table(name, file, line, partition_of=match.group(3))
    schema.tables[name] = table
    if table.partition_of:
        return
    for first, last, part in _table_parts(statement, line):
        text = strip_comments(part).strip()
        if not text:
            continue
        span = Span(file, first, last)
        if TABLE_LEVEL.match(text):
            _read_table_constraint(schema, name, span, text)
            continue
        column = COLUMN_LINE.match(text)
        if column is None:
            schema.unparsed.append(f"{file}:{first}: {text[:60]}")
            continue
        table.columns[column.group(1)] = _new_column(first, column.group(2))
        _read_column_rules(schema, name, span, text)


def _new_column(line: int, declared_type: str) -> dict[str, Any]:
    return {"line": line, "type": one_line(declared_type), "described": False, "comment": None}


def _read_column_rules(schema: Schema, table: str, span: Span, text: str) -> None:
    column = f"{table}.{text.split()[0]}"
    if COLUMN_CHECK.search(text):
        _declare(schema, Constraint.at("check", table, span, column, text))
    if COLUMN_UNIQUE.search(text):
        _declare(schema, Constraint.at("unique", table, span, column, text))
    referenced = REFERENCES.search(text)
    if referenced:
        schema.foreign_keys.append(ForeignKey(table, referenced.group(1), span.file, span.first))


def _read_table_constraint(schema: Schema, table: str, span: Span, text: str) -> None:
    named = NAMED_CONSTRAINT.match(text)
    label = named.group(1) if named else f"{table}:{span.first}"
    clause = named.group(2) if named else text
    leading = CONSTRAINT_KIND.match(clause)
    kind = leading.group(1).upper() if leading else ""
    if kind in {"CHECK", "UNIQUE", "EXCLUDE"}:
        _declare(schema, Constraint.at(kind.lower(), table, span, label, text))
    elif kind == "FOREIGN":
        referenced = REFERENCES.search(clause)
        if referenced:
            schema.foreign_keys.append(ForeignKey(table, referenced.group(1), span.file, span.first))
    elif kind not in {"PRIMARY", "LIKE"}:
        schema.unparsed.append(f"{span.file}:{span.first}: {text[:60]}")


def _read_alter(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    table = match.group(1)
    span = Span(file, line, line + statement.count("\n"))
    if table not in schema.tables:
        schema.unparsed.append(f"{file}:{line}: ALTER TABLE {table} before its CREATE")
        return
    for action in _split_top_level(strip_comments(match.group(2))):
        text = action.strip()
        if not text:
            continue
        added = ADD_COLUMN.match(text)
        dropped = DROP_COLUMN.match(text)
        renamed = RENAME_COLUMN.match(text)
        dropped_constraint = DROP_CONSTRAINT.match(text)
        if dropped_constraint:
            _drop_constraint(schema, table, dropped_constraint.group(1))
        elif ADD_CONSTRAINT.match(text):
            _read_table_constraint(schema, table, span, text[len("ADD "):].strip())
        elif added:
            declaration = text[added.start(1):]
            schema.tables[table].columns[added.group(1)] = _new_column(line, COLUMN_LINE.match(declaration).group(2))
            _read_column_rules(schema, table, span, declaration)
        elif dropped:
            schema.tables[table].columns.pop(dropped.group(1), None)
        elif renamed:
            _rename_column(schema.tables[table], renamed.group(1), renamed.group(2))
        elif not OTHER_ALTER.match(text):
            schema.unparsed.append(f"{file}:{line}: ALTER TABLE {table} {text[:50]}")


def _rename_column(table: Table, old: str, new: str) -> None:
    if old in table.columns:
        table.columns[new] = table.columns.pop(old)


def _drop_constraint(schema: Schema, table: str, name: str) -> None:
    _undeclare(schema, lambda c: c.kind in ("trigger", UNIQUE_INDEX) or c.table != table or name not in _names_of(c))


def _names_of(constraint: Constraint) -> set[str]:
    table, dot, column = constraint.name.partition(".")
    if not dot or table != constraint.table:
        return {constraint.name}
    return {constraint.name, f"{table}_{column}_{'key' if constraint.kind == 'unique' else constraint.kind}"}


def _read_drop(schema: Schema, _file: str, _line: int, _statement: str, match: re.Match[str]) -> None:
    schema.tables.pop(match.group(1), None)


def _read_index(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    unique, name, table = match.group(1), match.group(2), match.group(3)
    _undeclare(schema, lambda c: c.kind != UNIQUE_INDEX or c.name != name)
    if unique:
        _declare(schema, Constraint.at(UNIQUE_INDEX, table, Span(file, line, line + statement.count("\n")), name, statement))


def _read_trigger(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    _declare(schema, Constraint.at(
        "trigger", match.group(2), Span(file, line, line + statement.count("\n")), match.group(1), statement))


def _read_drop_index(schema: Schema, _file: str, _line: int, _statement: str, match: re.Match[str]) -> None:
    name = match.group(1)
    _undeclare(schema, lambda c: c.kind != UNIQUE_INDEX or c.name != name)


def _read_drop_trigger(schema: Schema, _file: str, _line: int, _statement: str, match: re.Match[str]) -> None:
    name, table = match.group(1), match.group(2)
    _undeclare(schema, lambda c: c.kind != "trigger" or (c.table, c.name) != (table, name))


def _read_comment(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    kind, name = match.group(1).upper(), match.group(2)
    table = schema.tables.get(name)
    quoted = COMMENT_TEXT.search(strip_comments(statement))
    text = one_line("".join(STRING_LITERAL.findall(quoted.group(1))).replace("''", "'")) if quoted else ""
    if kind == "TYPE":
        if name in schema.types:
            schema.type_comments[name] = text
        else:
            schema.unparsed.append(f"{file}:{line}: COMMENT ON unknown type {name}")
    elif table is None:
        schema.unparsed.append(f"{file}:{line}: COMMENT ON unknown table {name}")
    elif kind == "TABLE":
        table.described = True
        table.comment = text
    elif match.group(3) in table.columns:
        table.columns[match.group(3)].update(described=True, comment=text)


def _read_type(schema: Schema, _file: str, _line: int, statement: str, match: re.Match[str]) -> None:
    schema.types.append(match.group(2))
    values = ENUM_VALUES.search(strip_comments(statement))
    if values:
        schema.enums[match.group(2)] = [_unquote(value) for value in STRING_LITERAL.findall(values.group(1))]


def _read_enum_value(schema: Schema, _file: str, _line: int, _statement: str, match: re.Match[str]) -> None:
    schema.enums.setdefault(match.group(1), []).append(_unquote(match.group(2)))


def _unquote(literal: str) -> str:
    return literal.replace("''", "'")


Reader = Callable[[Schema, str, int, str, "re.Match[str]"], None]
READERS: list[tuple[re.Pattern[str], Reader]] = [
    (CREATE_TABLE, _read_table), (DROP_TABLE, _read_drop), (ALTER_TABLE, _read_alter), (CREATE_INDEX, _read_index),
    (CREATE_TRIGGER, _read_trigger), (COMMENT_ON, _read_comment), (CREATE_TYPE, _read_type),
    (DROP_INDEX, _read_drop_index), (DROP_TRIGGER, _read_drop_trigger), (ADD_ENUM_VALUE, _read_enum_value),
]


def read_statement(schema: Schema, file: str, line: int, statement: str) -> None:
    head = strip_comments(statement).strip()
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
        file = path.resolve().relative_to(repo_root.resolve()).as_posix()
        schema.migrations.append(file)
        for line, statement in statements(path.read_text(encoding="utf-8")):
            read_statement(schema, file, line, statement)
    return schema


def live_tables(schema: Schema) -> dict[str, Table]:
    return {name: table for name, table in schema.tables.items() if table.partition_of is None}
