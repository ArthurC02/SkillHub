from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.audit import read_events
from domain_registry.registry import record_by_id, validate
from domain_registry.sql_candidates import extract_sql_rules
from domain_registry.sql_schema import read_schema, statements
from test_support import VERIFIED, RegistryTestCase, applied_by

ORDERS = {"id": "orders", "name": "Orders", "status": "reviewed", "verified": VERIFIED, "implementation_path": "src/orders"}

MIGRATION = """CREATE TABLE orders (
    id uuid PRIMARY KEY,
    total integer NOT NULL CHECK (total >= 0), -- never negative
    code text UNIQUE,
    CONSTRAINT orders_code_shape CHECK (
        code ~ '^[A-Z]{3}$'
    )
);
CREATE TABLE orders_2026 PARTITION OF orders FOR VALUES FROM ('a') TO ('b');
CREATE TABLE scratch (n int CHECK (n > 0));
DROP TABLE scratch;
CREATE UNIQUE INDEX orders_code_key ON orders (code) WHERE code IS NOT NULL;
CREATE TABLE shipments (id uuid PRIMARY KEY, weight integer CHECK (weight > 0));
CREATE MATERIALIZED VIEW oddity AS SELECT 1;
CREATE INDEX orders_total_idx ON orders (total);
"""


class SqlExtractionTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.migrations = self.repo / "db" / "migrations"
        self.migrations.mkdir(parents=True)
        (self.migrations / "0001_init.sql").write_text(MIGRATION, encoding="utf-8")
        self.seed("contexts.json", [ORDERS])
        self.root = self.repo / "memory"

    def extract(self, table_contexts: dict[str, str] | None = None, default: str | None = None) -> dict:
        contexts = {"orders": "orders"} if table_contexts is None else table_contexts
        return extract_sql_rules(self.root, self.repo, self.migrations, contexts, default)

    def test_each_constraint_of_a_mapped_table_becomes_a_candidate_rule_citing_its_lines(self) -> None:
        report = self.extract()
        self.assertEqual(report["extracted"], [
            "rules/orders-total-check", "rules/orders-code-unique", "rules/orders_code_shape", "rules/orders_code_key",
        ])
        self.assertEqual((report["created"], report["updated"], report["tables"], report["constraints"]), (4, 0, 2, 5))
        rule = record_by_id(self.root, "rules", "orders_code_shape")
        self.assertEqual(rule["status"], "candidate")
        self.assertEqual(rule["contexts"], ["orders"])
        self.assertEqual(rule["statement"],
                         "orders keeps check orders_code_shape: CONSTRAINT orders_code_shape CHECK ( code ~ '^[A-Z]{3}$' )")
        reference = rule["evidence"][0]
        self.assertEqual((reference["path"], reference["lines"]), ("db/migrations/0001_init.sql", {"start": 5, "end": 7}))
        self.assertTrue(reference["excerpt_sha256"].startswith("sha256:"))
        self.assertEqual([error for error in validate(self.root, self.repo, require_reviewed=False) if "rules/" in error], [])

    def test_a_column_rule_cites_the_column_line_and_keeps_the_trailing_comment_out_of_the_statement(self) -> None:
        self.extract()
        rule = record_by_id(self.root, "rules", "orders-total-check")
        self.assertEqual(rule["evidence"][0]["lines"], {"start": 3, "end": 3})
        self.assertEqual(rule["statement"], "orders keeps check orders.total: total integer NOT NULL CHECK (total >= 0)")

    def test_a_table_without_a_context_is_left_out_and_named_and_the_default_context_places_it(self) -> None:
        report = self.extract()
        self.assertEqual(report["unplaced_tables"], ["shipments"])
        self.assertIsNone(record_by_id(self.root, "rules", "shipments-weight-check"))
        placed = self.extract(default="orders")
        self.assertEqual(placed["unplaced_tables"], [])
        self.assertEqual(record_by_id(self.root, "rules", "shipments-weight-check")["contexts"], ["orders"])

    def test_a_dropped_table_and_a_partition_contribute_no_rules_and_the_unread_statement_is_named(self) -> None:
        report = self.extract(default="orders")
        self.assertNotIn("rules/scratch-n-check", report["extracted"])
        self.assertEqual(report["unread_statements"], ["db/migrations/0001_init.sql:14: CREATE MATERIALIZED VIEW oddity AS SELECT 1"])

    def test_extracting_again_updates_the_candidates_and_leaves_a_reviewed_rule_alone(self) -> None:
        self.seed("rules.json", [{
            "id": "orders_code_shape", "contexts": ["orders"], "statement": "Codes are three capitals.",
            "status": "reviewed", "verified": VERIFIED,
            "evidence": [{"path": "db/migrations/0001_init.sql", "lines": {"start": 5, "end": 7}}],
        }])
        applied_by(self.root, "PRO-1", "rules/orders_code_shape")
        first = self.extract()
        self.assertEqual((first["created"], first["updated"], first["skipped_reviewed"]), (3, 0, ["rules/orders_code_shape"]))
        (self.migrations / "0001_init.sql").write_text(MIGRATION.replace("total >= 0", "total > 0"), encoding="utf-8")
        report = self.extract()
        self.assertEqual(report["skipped_reviewed"], ["rules/orders_code_shape"])
        self.assertEqual((report["created"], report["updated"]), (0, 3))
        self.assertEqual(record_by_id(self.root, "rules", "orders_code_shape")["statement"], "Codes are three capitals.")
        self.assertIn("total > 0", record_by_id(self.root, "rules", "orders-total-check")["statement"])
        self.assertEqual([event["records"] for event in read_events(self.root) if event.get("operation") == "extract-sql-schema"][-1],
                         ["rules/orders-total-check", "rules/orders-code-unique", "rules/orders_code_key"])

    def test_a_statement_longer_than_the_limit_is_cut_with_an_ellipsis(self) -> None:
        long = "CREATE TABLE wide (id int, CONSTRAINT wide_long CHECK (" + " OR ".join(f"id = {n}" for n in range(80)) + "));\n"
        (self.migrations / "0002_wide.sql").write_text(long, encoding="utf-8")
        self.extract({"wide": "orders"})
        statement = record_by_id(self.root, "rules", "wide_long")["statement"]
        self.assertEqual(len(statement) - len("wide keeps check wide_long: "), 240)
        self.assertTrue(statement.endswith("…"))

    def test_the_cli_prints_the_report_and_refuses_a_malformed_table_context(self) -> None:
        env = {**os.environ, "PYTHONPATH": str(Path(__file__).parent)}
        script = Path(__file__).parent / "registry_tools.py"
        base = [sys.executable, str(script), "extract-sql-schema", "--registry-root", str(self.root),
                "--repo-root", str(self.repo), "--migrations", str(self.migrations)]
        done = subprocess.run([*base, "--table-context", "orders=orders"], capture_output=True, text=True, env=env, check=False)
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(json.loads(done.stdout)["unplaced_tables"], ["shipments"])
        self.assertIn("NOTE: no Context for the constraints of shipments", done.stderr)
        self.assertIn("NOTE: 1 statement(s) were not read", done.stderr)
        refused = subprocess.run([*base, "--table-context", "orders"], capture_output=True, text=True, env=env, check=False)
        self.assertEqual(refused.returncode, 1)
        self.assertIn("ERROR: --table-context takes TABLE=CONTEXT, got orders", refused.stderr)


class SqlSchemaTest(unittest.TestCase):
    def test_a_semicolon_inside_a_comment_a_quote_a_dollar_body_or_parentheses_does_not_split(self) -> None:
        text = ("-- heading; not a statement\nCREATE TABLE a (x text -- one; two\n, y text DEFAULT 'a;b', "
                "z int CHECK (f(1;2)));\nDO $$ x; y $$;\n\n  DROP TABLE a")
        self.assertEqual([(line, s[:14]) for line, s in statements(text)],
                         [(2, "CREATE TABLE a"), (4, "DO $$ x; y $$"), (6, "DROP TABLE a")])

    def test_a_comment_line_before_create_table_does_not_shift_the_column_lines(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text("-- head\n-- more\nCREATE TABLE a (\n  x int CHECK (x > 0)\n);\n", encoding="utf-8")
        schema = read_schema(root / "m", root)
        self.assertEqual([(c.name, c.line, c.end) for c in schema.constraints], [("a.x", 4, 4)])
        self.assertEqual(schema.tables["a"].columns["x"]["line"], 4)

    def test_a_later_declaration_of_the_same_name_replaces_the_earlier_and_a_drop_removes_it(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text(
            "CREATE TABLE a (x int, y int, CONSTRAINT a_x CHECK (x > 0), CONSTRAINT a_y CHECK (y > 0));\n"
            "CREATE UNIQUE INDEX a_x_key ON a (x);\nCREATE INDEX a_y_live ON a (y) WHERE y > 0;\n"
            "CREATE TRIGGER a_guard BEFORE UPDATE ON a FOR EACH ROW EXECUTE FUNCTION guard();\n"
            "CREATE TRIGGER a_other BEFORE DELETE ON a FOR EACH ROW EXECUTE FUNCTION guard();\n", encoding="utf-8")
        (root / "m" / "0002.sql").write_text(
            "ALTER TABLE a DROP CONSTRAINT a_x, ADD CONSTRAINT a_x CHECK (x > 1);\n"
            "ALTER TABLE a DROP CONSTRAINT a_y;\n"
            "DROP INDEX a_x_key;\nCREATE UNIQUE INDEX a_x_key ON a (x, y);\n"
            "DROP INDEX IF EXISTS a_y_live;\n"
            "CREATE OR REPLACE TRIGGER a_guard BEFORE UPDATE ON a FOR EACH ROW EXECUTE FUNCTION guard2();\n"
            "DROP TRIGGER a_other ON a;\n", encoding="utf-8")
        schema = read_schema(root / "m", root)
        self.assertEqual(sorted((c.kind, c.name, c.file[-8:], c.line) for c in schema.constraints), [
            ("check", "a_x", "0002.sql", 1), ("trigger", "a_guard", "0002.sql", 6), ("unique-index", "a_x_key", "0002.sql", 4),
        ])

    def test_constraints_carry_their_first_and_last_line(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text(MIGRATION, encoding="utf-8")
        schema = read_schema(root / "m", root)
        self.assertEqual(sorted((c.kind, c.name, c.line, c.end) for c in schema.constraints), [
            ("check", "orders.total", 3, 3), ("check", "orders_code_shape", 5, 7), ("check", "scratch.n", 10, 10),
            ("check", "shipments.weight", 13, 13), ("unique", "orders.code", 4, 4), ("unique-index", "orders_code_key", 12, 12),
        ])

    def test_only_comment_on_describes_a_column_and_a_plain_or_partial_index_is_no_rule(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text(MIGRATION, encoding="utf-8")
        (root / "m" / "0002.sql").write_text(
            "CREATE INDEX orders_live ON orders (code) WHERE code IS NOT NULL;\n"
            "COMMENT ON COLUMN orders.code IS 'the public code';\n", encoding="utf-8")
        schema = read_schema(root / "m", root)
        columns = schema.tables["orders"].columns
        self.assertEqual((columns["total"]["described"], columns["code"]["described"]), (False, True))
        self.assertEqual(["orders_code_key"], [c.name for c in schema.constraints if c.kind == "unique-index"])
        self.assertEqual([], [c.name for c in schema.constraints if c.name in {"orders_total_idx", "orders_live"}])

    def test_dropping_a_column_constraint_by_the_name_the_database_gives_it_removes_the_inline_declaration(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text(
            "CREATE TABLE a (x text CHECK (x IN ('p')), y text UNIQUE, z int CHECK (z > 0));\n", encoding="utf-8")
        (root / "m" / "0002.sql").write_text(
            "ALTER TABLE a DROP CONSTRAINT IF EXISTS a_x_check;\n"
            "ALTER TABLE a ADD CONSTRAINT a_x_check CHECK (x IN ('p', 'q'));\n"
            "ALTER TABLE a DROP CONSTRAINT a_y_key;\n"
            "ALTER TABLE a DROP CONSTRAINT b_z_check;\n", encoding="utf-8")
        schema = read_schema(root / "m", root)
        self.assertEqual(
            [("check", "a.z", 1), ("check", "a_x_check", 2)],
            sorted((c.kind, c.name, c.line) for c in schema.constraints),
        )

    def test_a_comma_inside_a_comment_or_a_string_starts_no_column_and_a_type_keeps_all_its_words(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text(
            "CREATE TABLE a (\n"
            "    seq bigint NOT NULL, -- producer sequence, lets gaps be detected\n"
            "    tags text NOT NULL DEFAULT 'x,y',\n"
            "    cost numeric(12, 6) CHECK (cost >= 0),\n"
            "    weight double precision\n"
            ");\n"
            "ALTER TABLE a ADD COLUMN IF NOT EXISTS extra text CHECK (extra <> '');\n", encoding="utf-8")
        schema = read_schema(root / "m", root)
        self.assertEqual(
            ([("seq", "bigint"), ("tags", "text"), ("cost", "numeric(12, 6)"), ("weight", "double precision"), ("extra", "text")],
             [("check", "a.cost"), ("check", "a.extra")], []),
            ([(name, info["type"]) for name, info in schema.tables["a"].columns.items()],
             sorted((c.kind, c.name) for c in schema.constraints), schema.unparsed),
        )

    def test_an_enum_type_keeps_its_values_in_order_including_those_added_later(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text(
            "CREATE TYPE lane AS ENUM ('fast', 'it''s slow');\nCREATE DOMAIN money AS numeric(12, 2);\n"
            "CREATE TABLE a (x lane NOT NULL, y money, z numeric(12,2));\n", encoding="utf-8")
        (root / "m" / "0002.sql").write_text(
            "ALTER TYPE lane ADD VALUE 'stopped';\nALTER TYPE lane ADD VALUE IF NOT EXISTS 'it''s odd';\n"
            "ALTER TABLE a ADD COLUMN w lane;\n"
            "COMMENT ON TYPE lane IS 'fast when the road is clear; it''s slow otherwise.';\n"
            "COMMENT ON TYPE gone IS 'never created';\n", encoding="utf-8")
        schema = read_schema(root / "m", root)
        self.assertEqual(
            ({"lane": ["fast", "it's slow", "stopped", "it's odd"]}, ["lane", "money"], ["lane", "money", "numeric(12,2)", "lane"],
             {"lane": "fast when the road is clear; it's slow otherwise."}, ["m/0002.sql:5: COMMENT ON unknown type gone"]),
            (schema.enums, schema.types, [info["type"] for info in schema.tables["a"].columns.values()], schema.type_comments,
             schema.unparsed),
        )

    def test_a_renamed_column_keeps_its_line_and_takes_the_comment_under_its_new_name(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "m").mkdir()
        (root / "m" / "0001.sql").write_text(MIGRATION, encoding="utf-8")
        (root / "m" / "0002.sql").write_text(
            "ALTER TABLE orders RENAME COLUMN total TO amount;\n"
            "ALTER TABLE orders RENAME code TO reference;\n"
            "COMMENT ON COLUMN orders.amount IS 'what the buyer pays';\n", encoding="utf-8")
        schema = read_schema(root / "m", root)
        columns = schema.tables["orders"].columns
        self.assertEqual(["amount", "id", "reference"], sorted(columns))
        self.assertEqual(({"line": 3, "type": "integer", "described": True, "comment": "what the buyer pays"}, []),
                         (columns["amount"], [u for u in schema.unparsed if "RENAME" in u]))


if __name__ == "__main__":
    unittest.main()
