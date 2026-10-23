#!/usr/bin/env python3
import json
import sys
import tempfile
from pathlib import Path

import export_contracts as ec
import measure_readiness as mr

MIGRATION = """CREATE TYPE lane AS ENUM ('fast', 'slow');
CREATE TABLE users (id uuid PRIMARY KEY, email text NOT NULL, tags text[]);
COMMENT ON TABLE users IS 'one row per person';
COMMENT ON COLUMN users.email IS 'the sign-in address';
CREATE TABLE runs (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    state text NOT NULL CHECK (state IN ('queued', 'done')),
    speed lane,
    cost numeric(12, 6),
    CONSTRAINT runs_cost_positive CHECK (cost IS NULL OR cost >= 0)
);
COMMENT ON TABLE runs IS 'one execution';
COMMENT ON TYPE lane IS 'fast on the highway, slow in town';
"""
OWNERS = """files:
queries:
allow:
read_allow:
tables:
  users: identity
  runs: run
immutable:
  runs: history
retention:
  users: keep
classification:
  users.email: personal, shown only to the account itself
"""


def repo() -> Path:
    root = Path(tempfile.mkdtemp())
    (root / "db" / "migrations").mkdir(parents=True)
    (root / "db" / "queries").mkdir()
    (root / "db" / "migrations" / "0001.sql").write_text(MIGRATION, encoding="utf-8")
    (root / "db" / "query-owners.yaml").write_text(OWNERS, encoding="utf-8")
    (root / "docs" / "domain-memory").mkdir(parents=True)
    return root


def test_a_contract_carries_meaning_types_requirements_values_classification_ownership_and_relations():
    root = repo()
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    runs = ec.contract("runs", inputs, root, None)
    users = ec.contract("users", inputs, root, None)
    assert (runs["apiVersion"], runs["kind"], runs["id"], runs["version"], runs["description"]["purpose"]) == \
        ("v3.0.2", "DataContract", "runs", "1", "one execution")
    properties = {p["name"]: p for p in runs["schema"][0]["properties"]}
    assert [(p["name"], p["logicalType"], p["required"], p["primaryKey"]) for p in runs["schema"][0]["properties"]] == [
        ("id", "string", True, True), ("user_id", "string", True, False), ("state", "string", True, False),
        ("speed", "string", False, False), ("cost", "number", False, False)]
    assert properties["state"]["quality"] == [{"type": "library", "rule": "validValues", "validValues": ["queued", "done"]}]
    assert properties["speed"]["quality"][0]["validValues"] == ["fast", "slow"]
    assert properties["speed"]["description"] == "fast on the highway, slow in town"
    assert "description" not in properties["cost"] and "classification" not in properties["cost"]
    assert [q["name"] for q in runs["schema"][0]["quality"]] == ["runs_state_check", "runs_cost_positive"]
    assert runs["customProperties"] == [
        {"property": "owner", "value": "run"}, {"property": "immutable", "value": "history"},
        {"property": "references", "value": ["users"]}, {"property": "referencedBy", "value": []}]
    assert runs["authoritativeDefinitions"] == [{"type": "implementation", "url": "db/migrations/0001.sql:5"}]
    email = {p["name"]: p for p in users["schema"][0]["properties"]}
    assert (email["email"]["classification"], email["email"]["description"], email["tags"]["logicalType"]) == \
        ("personal", "the sign-in address", "array")
    assert users["customProperties"][1] == {"property": "retention", "value": "keep"}


def test_export_writes_one_json_file_per_live_table():
    root = repo()
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    out = root / "contracts"
    assert ec.export(inputs, root, None, out) == ["runs", "users"]
    assert json.loads((out / "runs.json").read_text(encoding="utf-8"))["id"] == "runs"


def test_every_table_of_the_repository_exports_without_an_unknown_type_falling_back_silently():
    root = Path(__file__).resolve().parents[2]
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    unknown = set()
    for table in inputs.tables:
        for prop in ec.contract(table, inputs, root, root / "docs" / "domain-memory")["schema"][0]["properties"]:
            base = prop["physicalType"].split("(")[0].strip().lower()
            if prop["logicalType"] == "string" and base not in ec.LOGICAL_TYPES and base not in inputs.schema.enums:
                unknown.add(prop["physicalType"])
    assert unknown == {"vector(1536)"}


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
