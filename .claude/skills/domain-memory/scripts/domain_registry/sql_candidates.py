from __future__ import annotations

import re
from pathlib import Path
from typing import Any

from .evidence import citation, classified, source_map_for
from .sql_schema import Constraint, live_tables, read_schema
from .store import read_document, write_document
from .transaction import mutate_registry
from .updates import Staging, Upserted, stage_candidates

RULES = "rules.json"
ID_SEPARATORS = re.compile(r"[^A-Za-z0-9_-]+")
STATEMENT_LIMIT = 240


def label_of(constraint: Constraint) -> str:
    return f"{constraint.table}.{constraint.column}" if constraint.column else constraint.name


def rule_id(constraint: Constraint) -> str:
    if constraint.column:
        return ID_SEPARATORS.sub("-", f"{label_of(constraint)}-{constraint.kind}").lower()
    label = ID_SEPARATORS.sub("-", constraint.name).lower()
    return label if label.startswith(constraint.table.lower()) else f"{constraint.table.lower()}-{label}"


def statement_of(constraint: Constraint) -> str:
    text = constraint.text
    if len(text) > STATEMENT_LIMIT:
        text = text[: STATEMENT_LIMIT - 1] + "…"
    return f"{constraint.table} keeps {constraint.kind} {label_of(constraint)}: {text}"


def extract_sql_rules(
    registry_root: Path, repo_root: Path, migrations_dir: Path, table_contexts: dict[str, str],
    default_context: str | None,
) -> dict[str, Any]:
    schema = read_schema(migrations_dir, repo_root)
    tables = live_tables(schema)
    source_map = source_map_for(registry_root)
    records: list[dict[str, Any]] = []
    unplaced: set[str] = set()
    live = [constraint for constraint in schema.constraints if constraint.table in tables]
    for constraint in live:
        context = table_contexts.get(constraint.table, default_context)
        if not context:
            unplaced.add(constraint.table)
            continue
        records.append({
            "id": rule_id(constraint), "contexts": [context], "statement": statement_of(constraint),
            "status": "candidate",
            "evidence": [classified(
                citation(repo_root, constraint.file, constraint.line, constraint.end), source_map)],
        })
    staging = Staging(source_map, skip_reviewed=True, skip_duplicates=True)
    result = Upserted()

    def mutate(staged: Path) -> None:
        document = read_document(staged, RULES)
        stage_candidates(document, "rules", records, staging, result)
        write_document(staged, RULES, document)

    def written() -> list[str]:
        skipped = {*result.skipped_reviewed, *(entry["record"] for entry in result.skipped_duplicate)}
        return [f"rules/{record['id']}" for record in records if f"rules/{record['id']}" not in skipped]

    mutate_registry(
        registry_root, repo_root, mutate, audit_event=lambda: {"operation": "extract-sql-schema", "records": written()})
    return {
        "migrations": len(schema.migrations), "tables": len(tables), "constraints": len(live),
        "extracted": written(), "created": result.created, "updated": len(written()) - result.created,
        "skipped_reviewed": result.skipped_reviewed, "skipped_duplicate": result.skipped_duplicate,
        "unplaced_tables": sorted(unplaced), "unread_statements": schema.unparsed,
        "outside_sources": sorted(set(result.outside)),
    }
