from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

PLUGIN_SCRIPTS = Path(__file__).resolve().parents[2] / ".claude" / "skills" / "domain-memory" / "scripts"
sys.path.insert(0, str(PLUGIN_SCRIPTS))

from domain_registry.sql_schema import IDENT, ForeignKey, Schema, Table, read_schema, statements, strip_comments  # noqa: E402

QUERY_NAME = re.compile(r"^--\s*name:\s*(\S+)\s+:(\w+)", re.M)
QUERY_TABLES = re.compile(rf"\b(?:FROM|JOIN|INTO|UPDATE|DELETE\s+FROM)\s+(?:ONLY\s+)?({IDENT})", re.I)
WRITES = re.compile(r"\b(INSERT\s+INTO|UPDATE|DELETE\s+FROM)\b", re.I)
CITATION = re.compile(r"^(?P<path>[^#]+)#L(?P<start>\d+)(?:-(?P<end>\d+))?$")
CITED_RESOURCE = re.compile(r'resource:\s*"([^"]+)"')
CONTEXT_LINK = re.compile(r"\(/contexts/([a-z0-9-]+)\)")
RECORD_TYPE = re.compile(r"^type:\s*(.+)$", re.M)
RECORD_CONSISTENCY = re.compile(r"^consistency:\s*(.+)$", re.M)
RECORD_STATUS = re.compile(r"^status:\s*(\S+)", re.M)
UNREVIEWED_STATUSES = {"draft", "candidate"}
SENSITIVE = re.compile(
    r"(^|_)(email|token|secret|password|passwd|phone|dsn|credential|ssn|api_key|ip_address|client_ip)(_|$)", re.I)
KNOWN_CONSISTENCIES = {"synchronous", "eventual"}
_statements = statements
_strip_comments = strip_comments


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
    status: str

    @property
    def reviewed(self) -> bool:
        return self.status not in UNREVIEWED_STATUSES

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
    status = RECORD_STATUS.search(front)
    return Record(
        path.parent.name, path.stem, kind.group(1).strip() if kind else "", citations, CONTEXT_LINK.findall(body),
        consistency.group(1).strip().strip('"') if consistency else None, body, status.group(1) if status else "",
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


GENERIC_KINDS = {"Generic", "Shared Kernel"}
IDENTITY_ID = re.compile(r"^\s*-\s*id:\s*(\S+)")
IDENTITY_KIND = re.compile(r"^\s*kind:\s*(.+?)\s*$")


def read_identity(path: Path) -> set[str]:
    if not path.is_file():
        return set()
    generic, current = set(), None
    for line in path.read_text(encoding="utf-8").splitlines():
        named = IDENTITY_ID.match(line)
        kind = IDENTITY_KIND.match(line)
        if named:
            current = named.group(1)
        elif kind and current and kind.group(1) in GENERIC_KINDS:
            generic.add(current)
    return generic


@dataclass
class Inputs:
    schema: Schema
    queries: list[Query]
    owners: dict[str, dict[str, list[str]]]
    records: list[Record]
    generic: set[str]

    @property
    def tables(self) -> dict[str, Table]:
        return {name: table for name, table in self.schema.tables.items() if table.partition_of is None}

    def table_owners(self, table: str) -> list[str]:
        return self.owners.get("tables", {}).get(table, [])

    def query_owner(self, query: Query) -> str:
        declared = self.owners.get("queries", {}).get(query.name) or self.owners.get("files", {}).get(query.file) or [""]
        return declared[0]

    @property
    def reviewed(self) -> list[Record]:
        return [record for record in self.records if record.reviewed]

    def cited(self, file: str, line: int) -> bool:
        return any(record.cites(file, line) for record in self.reviewed)

    def cited_by_a_candidate(self, file: str, line: int) -> bool:
        return any(record.cites(file, line) for record in self.records if not record.reviewed)

    @property
    def contexts(self) -> set[str]:
        return {record.name for record in self.reviewed if record.asset == "contexts"}

    def is_generic(self, table: str) -> bool:
        return any(owner in self.generic for owner in self.table_owners(table))


def _meaning(inputs: Inputs) -> dict[str, Any]:
    tables = inputs.tables
    columns = [info for table in tables.values() for info in table.columns.values()]
    return {
        "meaning.tables_described": _ratio(sum(table.described for table in tables.values()), len(tables)),
        "meaning.columns_described": _ratio(sum(info["described"] for info in columns), len(columns)),
        "meaning.tables_known_to_registry": _ratio(sum(_mentions(inputs.reviewed, name) for name in tables), len(tables)),
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
        name for record in inputs.reviewed if record.asset == "aggregates"
        for name in tables if re.search(rf"\b{name}\b", record.text)
    }
    known = inputs.contexts | inputs.generic
    unknown = {name: [owner for owner in inputs.table_owners(name) if owner not in known] for name in tables}
    metrics = {
        "ownership.tables_with_exactly_one_declared_owner": _ratio(
            sum(len(inputs.table_owners(name)) == 1 for name in tables), len(tables)),
        "ownership.tables_with_a_known_owner": _ratio(
            sum(bool(inputs.table_owners(name)) and not unknown[name] for name in tables), len(tables)),
        "ownership.tables_written_by_at_most_one_owner": _ratio(
            sum(len(writers.get(name, set())) <= 1 for name in tables), len(tables)),
        "ownership.tables_claimed_by_an_aggregate": _ratio(len(claimed), len(tables)),
        "ownership.tables_touched_by_a_query": _ratio(sum(name in touched for name in tables), len(tables)),
    }
    details = {
        "tables_without_one_owner": {name: inputs.table_owners(name) for name in sorted(tables)
                                     if len(inputs.table_owners(name)) != 1},
        "tables_without_a_known_owner": {name: owners for name, owners in sorted(unknown.items()) if owners},
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
    constraints = [constraint for constraint in inputs.schema.constraints
                   if constraint.table in tables and not inputs.is_generic(constraint.table)]
    cited = [constraint for constraint in constraints if inputs.cited(constraint.file, constraint.line)]
    generic_keys = [key for key in inputs.schema.foreign_keys if inputs.is_generic(key.child) or inputs.is_generic(key.parent)]
    governed = [key for key in inputs.schema.foreign_keys if key not in generic_keys]
    crossing = {key: pair for key in governed if (pair := _cross_context(inputs, key))}
    policies = {
        (record.contexts[0], record.contexts[1]) for record in inputs.reviewed
        if record.asset == "dependency-policies" and len(record.contexts) > 1
    }
    awaiting_review = [c for c in constraints if c not in cited and inputs.cited_by_a_candidate(c.file, c.line)]
    covered = [key for key, pair in crossing.items() if pair in policies]
    metrics = {
        "rules.constraints_cited_by_a_record": _ratio(len(cited), len(constraints)),
        "rules.foreign_keys_inside_one_context": _ratio(len(governed) - len(crossing), len(governed)),
        "rules.cross_context_foreign_keys_with_a_dependency_policy": _ratio(len(covered), len(crossing)),
    }
    details = {
        "uncited_constraints": sorted(f"{c.file}:{c.line} {c.kind} {c.name}" for c in constraints if c not in cited),
        "constraints_cited_only_by_a_candidate": len(awaiting_review),
        "cross_context_foreign_keys_without_a_policy": sorted({
            f"{key.child}->{key.parent} ({pair[0]}->{pair[1]})" for key, pair in crossing.items() if pair not in policies}),
        "foreign_keys_from_generic_tables": sorted({f"{key.child}->{key.parent}" for key in generic_keys}),
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
        path for record in inputs.reviewed for path, _, _ in record.citations if path in inputs.schema.migrations}
    immutable = list(inputs.owners.get("immutable", {}))
    retention = inputs.owners.get("retention", {})
    return {
        "consistency.interactions_with_a_known_consistency": _ratio(
            sum(record.consistency in KNOWN_CONSISTENCIES for record in interactions), len(interactions)),
        "provenance.migrations_cited_by_a_record": _ratio(len(cited_migrations), len(inputs.schema.migrations)),
        "provenance.immutable_tables_known_to_registry": _ratio(
            sum(_mentions(inputs.reviewed, name) for name in immutable), len(immutable)),
        "provenance.tables_with_a_retention_policy": _ratio(sum(bool(retention.get(name)) for name in tables), len(tables)),
    }


def read_inputs(repo_root: Path, registry_root: Path) -> Inputs:
    schema = read_schema(repo_root / "db" / "migrations", repo_root)
    return Inputs(
        schema, read_queries(repo_root / "db" / "queries", set(schema.tables)),
        read_owners(repo_root / "db" / "query-owners.yaml"), read_registry(registry_root),
        read_identity(repo_root / "apps" / "platform" / "architecture-identity.yaml"),
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


def _percent(ratio: float | None) -> str:
    return "" if ratio is None else f"{ratio:.1%}"


def render(report: dict[str, Any], baseline: dict[str, Any] | None, targets: dict[str, float] | None = None) -> str:
    lines = ["| metric | now | ratio | baseline | delta | target |", "| --- | --- | --- | --- | --- | --- |"]
    base_metrics = (baseline or {}).get("metrics", {})
    for name, value in report["metrics"].items():
        ratio = value["ratio"]
        base = base_metrics.get(name, {}).get("ratio")
        target = (targets or {}).get(name)
        delta = "" if ratio is None or base is None else f"{ratio - base:+.3f}"
        shown = "n/a" if ratio is None else f"{ratio:.1%}"
        met = "" if target is None else f"{_percent(target)} {'met' if ratio is not None and ratio >= target else 'below'}"
        lines.append(f"| {name} | {value['numerator']}/{value['denominator']} | {shown} | {_percent(base)} | {delta} | {met} |")
    inventory = ", ".join(f"{key} {value}" for key, value in report["inventory"].items())
    return f"{inventory}\n\n" + "\n".join(lines)


def regressions(report: dict[str, Any], baseline: dict[str, Any]) -> list[str]:
    found = []
    for name, value in report["metrics"].items():
        base = baseline.get("metrics", {}).get(name, {}).get("ratio")
        if base is not None and value["ratio"] is not None and value["ratio"] < base:
            found.append(f"{name} fell from {_percent(base)} to {_percent(value['ratio'])}")
    return found


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Measure how AI-ready the data layer is, from db/ and the Domain Memory, without a database.")
    parser.add_argument("--repo-root", type=Path, default=Path.cwd())
    parser.add_argument("--registry-root", type=Path, default=None)
    parser.add_argument("--baseline", type=Path, default=None)
    parser.add_argument("--targets", type=Path, default=None)
    parser.add_argument("--fail-on-regression", action="store_true", help="exit 1 when a ratio is below its baseline")
    parser.add_argument("--json", type=Path, default=None, help="write the full report here")
    args = parser.parse_args(argv)
    repo_root = args.repo_root.resolve()
    registry_root = (args.registry_root or repo_root / "docs" / "domain-memory").resolve()
    report = measure(read_inputs(repo_root, registry_root))
    baseline = json.loads(args.baseline.read_text(encoding="utf-8")) if args.baseline else None
    targets = json.loads(args.targets.read_text(encoding="utf-8")) if args.targets else None
    if args.json:
        args.json.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8", newline="\n")
    print(render(report, baseline, targets))
    unparsed = report["inventory"]["unparsed_statements"]
    if unparsed:
        print(f"\n{unparsed} statement(s) not read; see details.unparsed_statements", file=sys.stderr)
    fallen = regressions(report, baseline) if args.fail_on_regression and baseline else []
    for line in fallen:
        print(f"ERROR: {line}", file=sys.stderr)
    return 1 if fallen else 0


if __name__ == "__main__":
    sys.exit(main())
