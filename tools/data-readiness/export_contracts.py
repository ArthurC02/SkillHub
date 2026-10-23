from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import measure_readiness as mr  # noqa: E402
from domain_registry.sql_describe import describe_table  # noqa: E402

API_VERSION = "v3.0.2"
LOGICAL_TYPES = {
    "text": "string", "uuid": "string", "bytea": "string", "tsvector": "string", "date": "date",
    "timestamptz": "timestamp", "timestamp": "timestamp", "boolean": "boolean", "jsonb": "object", "json": "object",
    "bigint": "integer", "integer": "integer", "int": "integer", "smallint": "integer",
    "numeric": "number", "double precision": "number", "real": "number",
}


def logical_type(physical: str, enums: dict[str, list[str]]) -> str:
    if physical.endswith("[]"):
        return "array"
    if physical in enums:
        return "string"
    base = physical.split("(")[0].strip().lower()
    return LOGICAL_TYPES.get(base, "string")


def _property(column: dict[str, Any], table: str, inputs: mr.Inputs) -> dict[str, Any]:
    declared = {
        "name": column["name"],
        "physicalType": column["type"],
        "logicalType": logical_type(column["type"], inputs.schema.enums),
        "required": column["required"],
        "primaryKey": column["primary_key"],
    }
    if column["meaning"] or column["type_meaning"]:
        declared["description"] = " ".join(text for text in (column["meaning"], column["type_meaning"]) if text)
    classification = inputs.owners.get("classification", {}).get(f"{table}.{column['name']}")
    if classification:
        declared["classification"] = classification[0]
    if column["allowed_values"]:
        declared["quality"] = [{"type": "library", "rule": "validValues", "validValues": column["allowed_values"]}]
    return declared


def _custom(name: str, value: Any) -> dict[str, Any]:
    return {"property": name, "value": value}


def contract(table: str, inputs: mr.Inputs, repo_root: Path, registry_root: Path | None) -> dict[str, Any]:
    described = describe_table(registry_root, repo_root, repo_root / "db" / "migrations", table)
    owners = inputs.owners
    custom = [_custom("owner", ", ".join(inputs.table_owners(table)) or None)]
    custom += [_custom(section, ", ".join(owners[section][table]))
               for section in ("retention", "immutable") if table in owners.get(section, {})]
    custom += [_custom("references", sorted({edge["parent"] for edge in described["references"]})),
               _custom("referencedBy", sorted({edge["child"] for edge in described["referenced_by"]}))]
    definitions = [{"type": "implementation", "url": described["declared_at"]}]
    definitions += [
        {"type": "businessDefinition", "url": f"{mention['asset']}/{mention['id']}", "description": mention["summary"]}
        for mention in described["registry"]
    ]
    return {
        "apiVersion": API_VERSION,
        "kind": "DataContract",
        "id": table,
        "name": table,
        "version": str(len(inputs.schema.migrations)),
        "status": "active",
        "description": {"purpose": described["meaning"]},
        "schema": [{
            "name": table,
            "physicalType": "table",
            "description": described["meaning"],
            "properties": [_property(column, table, inputs) for column in described["columns"]],
            "quality": [{"type": "sql", "description": constraint["text"], "name": constraint["name"]}
                        for constraint in described["constraints"] if constraint["kind"] == "check"],
        }],
        "customProperties": custom,
        "authoritativeDefinitions": definitions,
    }


def export(inputs: mr.Inputs, repo_root: Path, registry_root: Path | None, out: Path) -> list[str]:
    out.mkdir(parents=True, exist_ok=True)
    written = []
    for table in sorted(inputs.tables):
        (out / f"{table}.json").write_text(json.dumps(contract(table, inputs, repo_root, registry_root), indent=2) + "\n",
                                           encoding="utf-8")
        written.append(table)
    return written


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-root", type=Path, default=Path.cwd())
    parser.add_argument("--registry-root", type=Path, default=None)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args(argv)
    registry_root = args.registry_root or args.repo_root / "docs" / "domain-memory"
    inputs = mr.read_inputs(args.repo_root, registry_root)
    written = export(inputs, args.repo_root, registry_root, args.out)
    print(f"{len(written)} contracts written to {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
