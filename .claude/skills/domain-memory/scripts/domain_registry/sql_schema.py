from __future__ import annotations

import itertools
import re
from collections.abc import Callable
from dataclasses import dataclass, field, replace
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
DYNAMIC_DDL = re.compile(r"\bEXECUTE\s+(?!FUNCTION\b|PROCEDURE\b)", re.I)
TEMPLATED_TRIGGER = re.compile(rf"\bCREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+({IDENT})\b", re.I)
DYNAMIC_EXCERPT = 160
PARENT = re.compile(rf"\bREFERENCES\s+({IDENT})\s*(?:\(([^)]*)\))?", re.I)
REFERENTIAL_ACTION = r"(CASCADE|RESTRICT|NO\s+ACTION|SET\s+NULL|SET\s+DEFAULT)"
ON_DELETE = re.compile(rf"\bON\s+DELETE\s+{REFERENTIAL_ACTION}", re.I)
ON_UPDATE = re.compile(rf"\bON\s+UPDATE\s+{REFERENTIAL_ACTION}", re.I)
COLUMN_LIST = re.compile(r"\(([^)]*)\)")
COLUMN_LINE = re.compile(
    rf"^\s*({IDENT})\s+(.+?)(?=\s+(?:NOT|NULL|DEFAULT|CHECK|REFERENCES|PRIMARY|UNIQUE|GENERATED|COLLATE|CONSTRAINT|EXCLUDE)\b|\s*$)",
    re.I | re.S)
TABLE_LEVEL = re.compile(r"^\s*(CONSTRAINT|PRIMARY\s+KEY|UNIQUE|CHECK|FOREIGN\s+KEY|EXCLUDE|LIKE)\b", re.I)
CONSTRAINT_KIND = re.compile(r"\s*(CHECK|UNIQUE|EXCLUDE|FOREIGN|PRIMARY|LIKE)\b", re.I)
NAMED_CONSTRAINT = re.compile(rf"CONSTRAINT\s+({IDENT})\s+(.*)$", re.I | re.S)
TRAILING_COMMENT = re.compile(r"[ \t]*--[^\n]*")
NOT_NULL = re.compile(r"\bNOT\s+NULL\b", re.I)
PRIMARY_KEY = re.compile(r"\bPRIMARY\s+KEY\b", re.I)
SET_NOT_NULL = re.compile(rf"^ALTER\s+(?:COLUMN\s+)?({IDENT})\s+(SET|DROP)\s+NOT\s+NULL\b", re.I)
SET_DEFAULT = re.compile(rf"^ALTER\s+(?:COLUMN\s+)?({IDENT})\s+(?:SET\s+DEFAULT\s+(.*)|DROP\s+DEFAULT)$", re.I | re.S)
CLAUSE_STARTERS = {"DEFAULT", "CHECK", "REFERENCES", "PRIMARY", "UNIQUE", "CONSTRAINT", "GENERATED", "NOT", "COLLATE",
                   "DEFERRABLE", "INITIALLY"}
OPAQUE_SPANS = (("--", "\n"), ("$$", "$$"), ("'", "'"))
NAME_LIMIT = 63


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
class Declared:
    table: str
    span: Span
    text: str


@dataclass(frozen=True)
class Constraint:
    kind: str
    table: str
    file: str
    line: int
    end: int
    name: str
    text: str
    columns: tuple[str, ...] = ()
    column: str | None = None
    clause: str = ""

    @classmethod
    def at(cls, kind: str, declared: Declared, name: str, columns: tuple[str, ...] = (), column: str | None = None) -> Constraint:
        span = declared.span
        return cls(kind, declared.table, span.file, span.first, span.last, name, one_line(declared.text), columns, column)


@dataclass(frozen=True)
class ForeignKey:
    child: str
    parent: str
    file: str
    line: int
    name: str = ""
    columns: tuple[str, ...] = ()
    parent_columns: tuple[str, ...] = ()
    on_delete: str | None = None
    on_update: str | None = None
    text: str = ""


@dataclass(frozen=True)
class Index:
    name: str
    table: str
    file: str
    line: int
    unique: bool
    definition: str
    columns: tuple[str, ...]


@dataclass(frozen=True)
class Retired:
    kind: str
    table: str
    name: str
    file: str
    line: int
    end: int
    replaced_at: str


@dataclass(frozen=True)
class Dynamic:
    file: str
    line: int
    tables: tuple[str, ...]
    text: str
    triggers: tuple[str, ...] = ()


@dataclass
class Schema:
    tables: dict[str, Table] = field(default_factory=dict)
    constraints: list[Constraint] = field(default_factory=list)
    foreign_keys: list[ForeignKey] = field(default_factory=list)
    primary_keys: dict[str, Constraint] = field(default_factory=dict)
    indexes: list[Index] = field(default_factory=list)
    retired: list[Retired] = field(default_factory=list)
    dynamic: list[Dynamic] = field(default_factory=list)
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
    _undeclare(schema, lambda kept: not _declares_the_same(kept, constraint), Span(constraint.file, constraint.line, constraint.end))
    schema.constraints.append(constraint)


def _undeclare(schema: Schema, keep: Callable[[Constraint], bool], at: Span) -> None:
    _retire(schema, [constraint for constraint in schema.constraints if not keep(constraint)], at)
    schema.constraints = [constraint for constraint in schema.constraints if keep(constraint)]


def _retire(schema: Schema, gone: list[Constraint], at: Span) -> None:
    schema.retired.extend(Retired(c.kind, c.table, c.name, c.file, c.line, c.end, f"{at.file}:{at.first}") for c in gone)


def _drop_keys(schema: Schema, keep: Callable[[ForeignKey], bool], at: Span) -> None:
    schema.retired.extend(Retired("foreign", key.child, key.name, key.file, key.line, key.line, f"{at.file}:{at.first}")
                          for key in schema.foreign_keys if not keep(key))
    schema.foreign_keys = [key for key in schema.foreign_keys if keep(key)]


def _drop_primary_key(schema: Schema, table: str, at: Span) -> None:
    if table in schema.primary_keys:
        _retire(schema, [schema.primary_keys.pop(table)], at)


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


def _top_level_words(text: str) -> list[tuple[int, str]]:
    found, depth, index = [], 0, 0
    while index < len(text):
        end = _opaque_end(text, index)
        if end is not None:
            index = end
            continue
        char = text[index]
        depth += (char == "(") - (char == ")")
        if depth == 0 and (char.isalpha() or char == "_"):
            start = index
            while index < len(text) and (text[index].isalnum() or text[index] == "_"):
                index += 1
            found.append((start, text[start:index].upper()))
            continue
        index += 1
    return found


def _clauses(declaration: str, start: int) -> list[tuple[str, str, str | None]]:
    starters = [(at, word) for at, word in _top_level_words(declaration) if at >= start and word in CLAUSE_STARTERS]
    clauses: list[tuple[str, str, str | None]] = []
    given = None
    for position, (at, word) in enumerate(starters):
        stop = starters[position + 1][0] if position + 1 < len(starters) else len(declaration)
        text = declaration[at:stop].strip()
        if word == "CONSTRAINT":
            given = text.split()[1] if len(text.split()) > 1 else None
            continue
        clauses.append((word, text, given))
        given = None
    return clauses


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
        declared = Declared(name, Span(file, first, last), text)
        if TABLE_LEVEL.match(text):
            _read_table_constraint(schema, declared)
            continue
        if not _read_column(schema, declared):
            schema.unparsed.append(f"{file}:{first}: {text[:60]}")


def _read_column(schema: Schema, declared: Declared) -> bool:
    column = COLUMN_LINE.match(declared.text)
    if column is None:
        return False
    span = declared.span
    schema.tables[declared.table].columns[column.group(1)] = _new_column(span.file, span.first, column.group(2), declared.text)
    _read_column_rules(schema, declared, column.group(1), column.end(2))
    return True


def _new_column(file: str, line: int, declared_type: str, declaration: str) -> dict[str, Any]:
    primary = bool(PRIMARY_KEY.search(declaration))
    return {"file": file, "line": line, "type": one_line(declared_type), "not_null": primary or bool(NOT_NULL.search(declaration)),
            "primary_key": primary, "described": False, "comment": None, "default": None}


def _read_column_rules(schema: Schema, declared: Declared, column: str, start: int) -> None:
    for keyword, clause, given in _clauses(declared.text, start):
        if keyword == "DEFAULT":
            schema.tables[declared.table].columns[column]["default"] = one_line(clause[len("DEFAULT"):])
        elif keyword == "CHECK":
            _add_check(schema, declared, clause, given, column)
        elif keyword == "UNIQUE":
            _add_unique(schema, declared, (column,), given, column)
        elif keyword == "PRIMARY":
            _add_primary_key(schema, declared, (column,), given)
        elif keyword == "REFERENCES":
            _add_foreign_key(schema, declared, (column,), clause, given)


def _read_table_constraint(schema: Schema, declared: Declared) -> None:
    named = NAMED_CONSTRAINT.match(declared.text)
    given = named.group(1) if named else None
    clause = named.group(2) if named else declared.text
    leading = CONSTRAINT_KIND.match(clause)
    kind = leading.group(1).upper() if leading else ""
    listed = _listed_columns(clause)
    if kind == "CHECK":
        _add_check(schema, declared, clause, given, None)
    elif kind == "UNIQUE":
        _add_unique(schema, declared, listed, given, None)
    elif kind == "EXCLUDE":
        involved = _columns_named_in(clause, schema.tables[declared.table].columns)
        name = given or _default_name(schema, declared.table, "_".join(involved), "excl")
        _declare(schema, Constraint.at("exclude", declared, name, involved))
    elif kind == "FOREIGN":
        _add_foreign_key(schema, declared, listed, clause, given)
    elif kind == "PRIMARY":
        _add_primary_key(schema, declared, listed, given)
    elif kind != "LIKE":
        schema.unparsed.append(f"{declared.span.file}:{declared.span.first}: {declared.text[:60]}")


def _listed_columns(clause: str) -> tuple[str, ...]:
    listed = COLUMN_LIST.search(clause)
    return tuple(column.strip() for column in listed.group(1).split(",") if column.strip()) if listed else ()


def _columns_named_in(text: str, columns: dict[str, Any]) -> tuple[str, ...]:
    words = re.findall(IDENT, STRING_LITERAL.sub("''", text))
    return tuple(dict.fromkeys(word for word in words if word in columns))


def _object_name(first: str, second: str, label: str) -> str:
    first, second = first[:NAME_LIMIT], second[:NAME_LIMIT]
    kept_first, kept_second = len(first), len(second)
    overhead = len(label) + 1 + (1 if second else 0)
    while kept_first + kept_second + overhead > NAME_LIMIT:
        if kept_first > kept_second:
            kept_first -= 1
        else:
            kept_second -= 1
    return "_".join(part for part in (first[:kept_first], second[:kept_second], label) if part)


def _used_names(schema: Schema, table: str) -> set[str]:
    return ({c.name for c in schema.constraints if c.table == table and c.kind != "trigger"}
            | {key.name for key in schema.foreign_keys if key.child == table}
            | {primary.name for primary in schema.primary_keys.values()} | {index.name for index in schema.indexes}
            | set(schema.tables))


def _default_name(schema: Schema, table: str, middle: str, label: str) -> str:
    used = _used_names(schema, table)
    for attempt in itertools.count():
        name = _object_name(table, middle, f"{label}{attempt}" if attempt else label)
        if name not in used:
            return name
    raise AssertionError


def _add_check(schema: Schema, declared: Declared, clause: str, given: str | None, column: str | None) -> None:
    involved = _columns_named_in(clause[len("CHECK"):], schema.tables[declared.table].columns)
    name = given or _default_name(schema, declared.table, involved[0] if len(involved) == 1 else "", "check")
    _declare(schema, replace(Constraint.at("check", declared, name, involved, column), clause=one_line(clause)))


def _add_unique(schema: Schema, declared: Declared, listed: tuple[str, ...], given: str | None, column: str | None) -> None:
    name = given or _default_name(schema, declared.table, "_".join(listed), "key")
    _declare(schema, Constraint.at("unique", declared, name, listed, column))


def _add_primary_key(schema: Schema, declared: Declared, listed: tuple[str, ...], given: str | None) -> None:
    columns = schema.tables[declared.table].columns
    for column in listed:
        if column in columns:
            columns[column].update(not_null=True, primary_key=True)
    name = given or _default_name(schema, declared.table, "", "pkey")
    _drop_primary_key(schema, declared.table, declared.span)
    schema.primary_keys[declared.table] = Constraint.at("primary", declared, name, listed)


def _add_foreign_key(schema: Schema, declared: Declared, listed: tuple[str, ...], clause: str, given: str | None) -> None:
    parent = PARENT.search(clause)
    span = declared.span
    if parent is None:
        schema.unparsed.append(f"{span.file}:{span.first}: {declared.text[:60]}")
        return
    deletion, update = ON_DELETE.search(clause), ON_UPDATE.search(clause)
    schema.foreign_keys.append(ForeignKey(
        declared.table, parent.group(1), span.file, span.first,
        given or _default_name(schema, declared.table, "_".join(listed), "fkey"), listed,
        _listed_columns(parent.group(0)) if parent.group(2) else (),
        " ".join(deletion.group(1).upper().split()) if deletion else None,
        " ".join(update.group(1).upper().split()) if update else None, one_line(declared.text)))


def _read_alter(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    table = match.group(1)
    span = Span(file, line, line + statement.count("\n"))
    if table not in schema.tables:
        schema.unparsed.append(f"{file}:{line}: ALTER TABLE {table} before its CREATE")
        return
    for action in _split_top_level(strip_comments(match.group(2))):
        text = action.strip()
        if text and not _read_alter_action(schema, table, span, text):
            schema.unparsed.append(f"{file}:{line}: ALTER TABLE {table} {text[:50]}")


def _read_alter_action(schema: Schema, table: str, span: Span, text: str) -> bool:
    columns = schema.tables[table].columns
    added = ADD_COLUMN.match(text)
    dropped = DROP_COLUMN.match(text)
    renamed = RENAME_COLUMN.match(text)
    dropped_constraint = DROP_CONSTRAINT.match(text)
    nullability = SET_NOT_NULL.match(text)
    default = SET_DEFAULT.match(text)
    if nullability and nullability.group(1) in columns:
        columns[nullability.group(1)]["not_null"] = nullability.group(2).upper() == "SET"
    elif default and default.group(1) in columns:
        columns[default.group(1)]["default"] = one_line(default.group(2)) if default.group(2) else None
    elif dropped_constraint:
        _drop_constraint(schema, table, dropped_constraint.group(1), span)
    elif ADD_CONSTRAINT.match(text):
        _read_table_constraint(schema, Declared(table, span, text[len("ADD "):].strip()))
    elif added:
        return _read_column(schema, Declared(table, span, text[added.start(1):]))
    elif dropped:
        _drop_column(schema, table, dropped.group(1), span)
    elif renamed:
        _rename_column(schema, table, renamed.group(1), renamed.group(2))
    else:
        return bool(OTHER_ALTER.match(text))
    return True


def _rename_column(schema: Schema, table: str, old: str, new: str) -> None:
    columns = schema.tables[table].columns
    if old in columns:
        columns[new] = columns.pop(old)

    def renamed(listed: tuple[str, ...]) -> tuple[str, ...]:
        return tuple(new if column == old else column for column in listed)

    schema.constraints = [replace(c, columns=renamed(c.columns)) if c.table == table else c for c in schema.constraints]
    schema.foreign_keys = [replace(k, columns=renamed(k.columns)) if k.child == table else k for k in schema.foreign_keys]
    schema.indexes = [replace(i, columns=renamed(i.columns)) if i.table == table else i for i in schema.indexes]
    if table in schema.primary_keys:
        schema.primary_keys[table] = replace(schema.primary_keys[table], columns=renamed(schema.primary_keys[table].columns))


def _drop_constraint(schema: Schema, table: str, name: str, at: Span) -> None:
    _undeclare(schema, lambda c: c.kind in ("trigger", UNIQUE_INDEX) or (c.table, c.name) != (table, name), at)
    _drop_keys(schema, lambda key: (key.child, key.name) != (table, name), at)
    if table in schema.primary_keys and schema.primary_keys[table].name == name:
        _drop_primary_key(schema, table, at)


def _drop_column(schema: Schema, table: str, column: str, at: Span) -> None:
    schema.tables[table].columns.pop(column, None)
    _undeclare(schema, lambda c: c.kind == "trigger" or c.table != table or column not in c.columns, at)
    _drop_keys(schema, lambda key: key.child != table or column not in key.columns, at)
    schema.indexes = [index for index in schema.indexes if index.table != table or column not in index.columns]
    if table in schema.primary_keys and column in schema.primary_keys[table].columns:
        _drop_primary_key(schema, table, at)


def _read_drop(schema: Schema, file: str, line: int, _statement: str, match: re.Match[str]) -> None:
    table = match.group(1)
    at = Span(file, line, line)
    schema.tables.pop(table, None)
    _drop_primary_key(schema, table, at)
    _undeclare(schema, lambda c: c.table != table, at)
    _drop_keys(schema, lambda key: table not in (key.child, key.parent), at)
    schema.indexes = [index for index in schema.indexes if index.table != table]


def _read_index(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    unique, name, table = bool(match.group(1)), match.group(2), match.group(3)
    definition = one_line(strip_comments(statement).strip()[match.end():])
    columns = _columns_named_in(definition, schema.tables[table].columns if table in schema.tables else {})
    schema.indexes = [index for index in schema.indexes if index.name != name]
    schema.indexes.append(Index(name, table, file, line, unique, definition, columns))
    span = Span(file, line, line + statement.count("\n"))
    _undeclare(schema, lambda c: c.kind != UNIQUE_INDEX or c.name != name, span)
    if unique:
        _declare(schema, Constraint.at(UNIQUE_INDEX, Declared(table, span, statement), name, columns))


def _read_trigger(schema: Schema, file: str, line: int, statement: str, match: re.Match[str]) -> None:
    declared = Declared(match.group(2), Span(file, line, line + statement.count("\n")), statement)
    _declare(schema, Constraint.at("trigger", declared, match.group(1)))


def _read_drop_index(schema: Schema, file: str, line: int, _statement: str, match: re.Match[str]) -> None:
    name = match.group(1)
    schema.indexes = [index for index in schema.indexes if index.name != name]
    _undeclare(schema, lambda c: c.kind != UNIQUE_INDEX or c.name != name, Span(file, line, line))


def _read_drop_trigger(schema: Schema, file: str, line: int, _statement: str, match: re.Match[str]) -> None:
    name, table = match.group(1), match.group(2)
    _undeclare(schema, lambda c: c.kind != "trigger" or (c.table, c.name) != (table, name), Span(file, line, line))


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
    if DYNAMIC_DDL.search(head):
        _read_dynamic(schema, file, line, head)
    if not IGNORED_STATEMENT.match(head):
        schema.unparsed.append(f"{file}:{line}: {head[:60]}")


def _read_dynamic(schema: Schema, file: str, line: int, head: str) -> None:
    named = tuple(dict.fromkeys(literal for literal in STRING_LITERAL.findall(head) if literal in schema.tables))
    text = one_line(head)
    start = DYNAMIC_DDL.search(text).start()
    triggers = tuple(dict.fromkeys(TEMPLATED_TRIGGER.findall(head)))
    schema.dynamic.append(Dynamic(file, line, named, text[start:start + DYNAMIC_EXCERPT], triggers))


def read_schema(migrations_dir: Path, repo_root: Path) -> Schema:
    schema = Schema()
    for path in sorted(migrations_dir.glob("*.sql")):
        file = path.resolve().relative_to(repo_root.resolve()).as_posix()
        schema.migrations.append(file)
        for line, statement in statements(path.read_text(encoding="utf-8")):
            read_statement(schema, file, line, statement)
    return schema


def migrations_inside(repo_root: Path, migrations: Path) -> Path:
    root = repo_root.resolve()
    located = (migrations if migrations.is_absolute() else root / migrations).resolve()
    if not located.is_relative_to(root):
        raise ValueError(f"--migrations resolves outside --repo-root ({located}); pass it relative to the repo root or as an "
                         "absolute path inside it")
    if not located.is_dir():
        raise ValueError(f"--migrations directory {located} does not exist")
    if not any(located.glob("*.sql")):
        raise ValueError(f"--migrations directory {located} holds no .sql files")
    return located


def live_tables(schema: Schema) -> dict[str, Table]:
    return {name: table for name, table in schema.tables.items() if table.partition_of is None}
