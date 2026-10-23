#!/usr/bin/env python3
import json
import sys
import tempfile
from pathlib import Path

import export_contracts as ec
import jsonschema
import measure_readiness as mr

MIGRATION = """CREATE TYPE lane AS ENUM ('fast', 'slow');
CREATE TABLE users (id uuid PRIMARY KEY, email text NOT NULL, tags text[]);
COMMENT ON TABLE users IS 'one row per person';
COMMENT ON COLUMN users.email IS 'the sign-in address';
CREATE TABLE runs (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('queued', 'done')),
    speed lane,
    cost numeric(12, 6),
    CONSTRAINT runs_cost_positive CHECK (cost IS NULL OR cost >= 0)
);
COMMENT ON TABLE runs IS 'one execution';
COMMENT ON TYPE lane IS 'fast on the highway, slow in town';
CREATE TABLE steps (id uuid PRIMARY KEY, run_id uuid, ws uuid, FOREIGN KEY (run_id, ws) REFERENCES runs (id, state));
"""
OWNERS = """files:
queries:
allow:
read_allow:
tables:
  users: identity
  runs: run
  steps: run
immutable:
  runs: history
retention:
  users: keep
classification:
  users.email: personal, shown only to the account itself
"""
ODCS = json.loads(ec.SCHEMA_FILE.read_text(encoding="utf-8"))


def repo() -> Path:
    root = Path(tempfile.mkdtemp())
    (root / "db" / "migrations").mkdir(parents=True)
    (root / "db" / "queries").mkdir()
    (root / "db" / "migrations" / "0001.sql").write_text(MIGRATION, encoding="utf-8")
    (root / "db" / "query-owners.yaml").write_text(OWNERS, encoding="utf-8")
    (root / "docs" / "domain-memory").mkdir(parents=True)
    return root


def violations(contract: dict) -> list[str]:
    validator = jsonschema.Draft201909Validator(ODCS)
    return sorted(f"{'/'.join(str(part) for part in error.absolute_path)}: {error.message[:80]}" for error in validator.iter_errors(contract))


def test_a_contract_carries_meaning_types_requirements_values_classification_ownership_and_relations():
    root = repo()
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    runs = ec.contract("runs", inputs, root, None)
    users = ec.contract("users", inputs, root, None)
    assert (runs["apiVersion"], runs["kind"], runs["id"], runs["version"], runs["description"]["purpose"]) == \
        ("v3.1.0", "DataContract", "runs", "1", "one execution")
    properties = {p["name"]: p for p in runs["schema"][0]["properties"]}
    assert [(p["name"], p["logicalType"], p["required"], p["primaryKey"]) for p in runs["schema"][0]["properties"]] == [
        ("id", "string", True, True), ("user_id", "string", True, False), ("state", "string", True, False),
        ("speed", "string", False, False), ("cost", "number", False, False)]
    assert properties["state"]["quality"] == [{
        "type": "library", "metric": "invalidValues", "arguments": {"validValues": ["queued", "done"]}, "mustBe": 0,
        "name": "state_valid_values", "description": "state holds one of its listed values"}]
    assert properties["speed"]["quality"][0]["arguments"]["validValues"] == ["fast", "slow"]
    assert properties["speed"]["description"] == "fast on the highway, slow in town"
    assert "description" not in properties["cost"] and "classification" not in properties["cost"]
    assert [(q["name"], q["query"], q["mustBe"]) for q in runs["schema"][0]["quality"]] == [
        ("runs_state_check", "SELECT COUNT(*) FROM runs WHERE NOT (state IN ('queued', 'done'))", 0),
        ("runs_cost_positive", "SELECT COUNT(*) FROM runs WHERE NOT (cost IS NULL OR cost >= 0)", 0)]
    assert runs["schema"][0]["relationships"] == [{
        "type": "foreignKey", "from": "runs.user_id", "to": "users.id",
        "customProperties": [{"property": "name", "value": "runs_user_id_fkey"}, {"property": "onDelete", "value": "CASCADE"},
                             {"property": "onUpdate", "value": "NO ACTION"}]}]
    assert runs["customProperties"] == [
        {"property": "owner", "value": "run"}, {"property": "immutable", "value": "history"},
        {"property": "references", "value": ["users"]}, {"property": "referencedBy", "value": ["steps"]}]
    assert runs["authoritativeDefinitions"] == [{"type": "implementation", "url": "db/migrations/0001.sql:5"}]
    email = {p["name"]: p for p in users["schema"][0]["properties"]}
    assert (email["email"]["classification"], email["email"]["description"], email["tags"]["logicalType"]) == \
        ("personal", "the sign-in address", "array")
    assert users["customProperties"][1] == {"property": "retention", "value": "keep"}


def test_a_composite_foreign_key_is_a_relationship_over_two_column_lists():
    root = repo()
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    steps = ec.contract("steps", inputs, root, None)
    assert [(r["from"], r["to"]) for r in steps["schema"][0]["relationships"]] == [(["steps.run_id", "steps.ws"], ["runs.id", "runs.state"])]


def test_every_fixture_contract_is_valid_against_the_vendored_odcs_schema_and_an_invalid_one_is_refused():
    root = repo()
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    contracts = {table: ec.contract(table, inputs, root, None) for table in inputs.tables}
    assert {table: violations(contract) for table, contract in contracts.items()} == {"runs": [], "steps": [], "users": []}
    broken = json.loads(json.dumps(contracts["runs"]))
    broken["schema"][0]["quality"][0].pop("query")
    broken["schema"][0]["properties"][2]["quality"][0].pop("metric")
    broken["schema"][0]["properties"][0]["logicalType"] = "uuid"
    assert sorted({line.split(":")[0] for line in violations(broken)}) == \
        ["schema/0/properties/0/logicalType", "schema/0/properties/2/quality/0", "schema/0/quality/0"]
    assert ("description" not in ec.contract("steps", inputs, root, None)["schema"][0], contracts["steps"]["description"]) == (True, {})


def test_export_writes_one_json_file_per_live_table():
    root = repo()
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    out = root / "contracts"
    assert ec.export(inputs, root, None, out) == ["runs", "steps", "users"]
    assert json.loads((out / "runs.json").read_text(encoding="utf-8"))["id"] == "runs"


def test_every_table_of_the_repository_exports_a_valid_contract_without_an_unknown_type_falling_back_silently():
    root = Path(__file__).resolve().parents[2]
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    unknown, invalid = set(), {}
    for table in inputs.tables:
        contract = ec.contract(table, inputs, root, root / "docs" / "domain-memory")
        for prop in contract["schema"][0]["properties"]:
            base = prop["physicalType"].split("(")[0].strip().lower()
            if prop["logicalType"] == "string" and base not in ec.LOGICAL_TYPES and base not in inputs.schema.enums:
                unknown.add(prop["physicalType"])
        if violations(contract):
            invalid[table] = violations(contract)[:3]
    assert (unknown, invalid) == ({"vector(1536)"}, {})


if __name__ == "__main__":
    failures = 0
    for name, test in sorted(globals().items()):
        if name.startswith("test_") and callable(test):
            try:
                test()
                print("ok  ", name)
            except Exception as error:
                failures += 1
                print("FAIL", name, "->", repr(error)[:300])
    sys.exit(1 if failures else 0)
