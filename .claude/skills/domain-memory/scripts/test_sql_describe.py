import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.evidence import citation
from domain_registry.sql_describe import describe_table
from domain_registry.sql_schema import read_schema
from test_support import RegistryTestCase

MIGRATION = """CREATE TABLE orders (
    id uuid PRIMARY KEY,
    state text NOT NULL CHECK (state IN ('open', 'paid', 'it''s odd')),
    buyer_id uuid REFERENCES users (id),
    note text,
    kind text CHECK (kind IS NULL OR kind IN ('gift', 'plain')),
    source text,
    CONSTRAINT orders_note_when_paid CHECK ((note IS NOT NULL) = (state IN ('paid'))),
    CONSTRAINT orders_source_when_gift CHECK (kind <> 'gift' OR source IN ('shop', 'partner'))
);
CREATE TABLE users (id uuid PRIMARY KEY);
CREATE TABLE shipments (id uuid PRIMARY KEY, order_id uuid NOT NULL REFERENCES orders (id));
CREATE TABLE orders_2026 PARTITION OF orders FOR VALUES FROM ('a') TO ('b');
CREATE UNIQUE INDEX orders_note_key ON orders (note);
COMMENT ON TABLE orders IS 'One row per order; the buyer''s request as placed.';
COMMENT ON COLUMN orders.state IS
    'open until paid; '
    'paid once the charge settled.';
CREATE TABLE scratch (id uuid PRIMARY KEY, order_id uuid REFERENCES orders (id));
DROP TABLE scratch;
CREATE TYPE mood AS ENUM ('calm', 'it''s odd');
ALTER TYPE mood ADD VALUE IF NOT EXISTS 'wild';
COMMENT ON TYPE mood IS 'calm by default; it''s odd when the buyer hesitated; wild once they bought twice.';
ALTER TABLE orders ADD COLUMN mood mood, ADD COLUMN mood_check text CHECK (mood_check IN ('yes')),
    ADD COLUMN only_one text CHECK (only_one IS NULL OR only_one = 'it''s one'),
    ADD COLUMN strict text CHECK (strict IS NOT NULL AND strict IN ('a', 'b')) NOT VALID;
"""


class SqlDescribeTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.migrations = self.repo / "db" / "migrations"
        self.migrations.mkdir(parents=True)
        (self.migrations / "0001_init.sql").write_text(MIGRATION, encoding="utf-8")

    def describe(self, table: str = "orders", registry: bool = True) -> dict:
        return describe_table(self.repo / "memory" if registry else None, self.repo, self.migrations, table)

    def test_the_reader_keeps_the_comment_text_with_quotes_unescaped_and_whitespace_folded(self) -> None:
        schema = read_schema(self.migrations, self.repo)
        orders = schema.tables["orders"]
        self.assertEqual(
            ("One row per order; the buyer's request as placed.", "open until paid; paid once the charge settled.", None),
            (orders.comment, orders.columns["state"]["comment"], orders.columns["note"]["comment"]),
        )

    def test_a_description_carries_meaning_allowed_values_relations_partitions_and_the_undescribed_columns(self) -> None:
        described = self.describe(registry=False)
        self.assertEqual("One row per order; the buyer's request as placed.", described["meaning"])
        self.assertEqual("db/migrations/0001_init.sql:1", described["declared_at"])
        self.assertEqual(
            [("id", "uuid", None), ("state", "text", ["open", "paid", "it's odd"]), ("buyer_id", "uuid", None), ("note", "text", None),
             ("kind", "text", ["gift", "plain"]), ("source", "text", None), ("mood", "mood", ["calm", "it's odd", "wild"]),
             ("mood_check", "text", ["yes"]), ("only_one", "text", ["it's one"]), ("strict", "text", ["a", "b"])],
            [(column["name"], column["type"], column["allowed_values"]) for column in described["columns"]],
        )
        self.assertEqual("open until paid; paid once the charge settled.", described["columns"][1]["meaning"])
        self.assertEqual(
            [("id", True, True), ("state", True, False), ("buyer_id", False, False), ("note", False, False)],
            [(column["name"], column["required"], column["primary_key"]) for column in described["columns"][:4]],
        )
        self.assertEqual(
            [(None, "state"), ("calm by default; it's odd when the buyer hesitated; wild once they bought twice.", "mood")],
            [(column["type_meaning"], column["name"]) for column in described["columns"] if column["name"] in ("state", "mood")],
        )
        self.assertEqual(
            [("check", "orders.state"), ("check", "orders.kind"), ("check", "orders_note_when_paid"),
             ("check", "orders_source_when_gift"), ("unique-index", "orders_note_key"), ("check", "orders.mood_check"),
             ("check", "orders.only_one"), ("check", "orders.strict")],
            [(constraint["kind"], constraint["name"]) for constraint in described["constraints"]],
        )
        self.assertEqual(([{"parent": "users", "at": "db/migrations/0001_init.sql:4"}], [{"child": "shipments", "at": "db/migrations/0001_init.sql:12"}]),
                         (described["references"], described["referenced_by"]))
        self.assertEqual((["orders_2026"], ["id", "buyer_id", "note", "kind", "source", "mood", "mood_check", "only_one", "strict"], []),
                         (described["partitions"], described["undescribed_columns"], described["registry"]))

    def test_registry_records_that_name_the_table_or_cite_its_lines_are_listed_with_their_status(self) -> None:
        self.seed("contexts.json", [{"id": "sales", "name": "Sales", "responsibility": "Own orders end to end."}])
        self.seed("rules.json", [
            {"id": "paid-once", "contexts": ["sales"], "statement": "An order is paid once.", "status": "reviewed",
             "evidence": [citation(self.repo, "db/migrations/0001_init.sql", 3, 3)]},
            {"id": "elsewhere", "contexts": ["sales"], "statement": "Shipments leave the warehouse.", "status": "candidate",
             "evidence": [citation(self.repo, "db/migrations/0001_init.sql", 12, 12)]},
            {"id": "by-name", "contexts": ["sales"], "statement": "The orders table keeps one row per order.", "status": "candidate"},
        ])
        self.assertEqual(
            [("contexts", "sales", None, "Own orders end to end."),
             ("rules", "by-name", "candidate", "The orders table keeps one row per order."),
             ("rules", "paid-once", "reviewed", "An order is paid once.")],
            [(entry["asset"], entry["id"], entry["status"], entry["summary"]) for entry in self.describe()["registry"]],
        )

    def test_an_unknown_table_is_refused_with_the_closest_names(self) -> None:
        with self.assertRaisesRegex(ValueError, r"^no table order in .*; did you mean orders\?$"):
            self.describe("order")
        with self.assertRaisesRegex(ValueError, r"^no table zzzz in [^;]*$"):
            self.describe("zzzz")

    def test_the_command_prints_the_description_and_names_the_undescribed_columns_on_stderr(self) -> None:
        common = ("describe-table", "--repo-root", str(self.repo), "--migrations", str(self.migrations))
        result = self.run_cli(*common, "--table", "orders", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(
            (0, "orders", ["id", "buyer_id", "note", "kind", "source", "mood", "mood_check", "only_one", "strict"],
             "NOTE: 9 columns of orders carry no COMMENT ON, so the files do not say what they mean: "
             "id, buyer_id, note, kind, source, mood, mood_check, only_one, strict"),
            (result.returncode, json.loads(result.stdout)["table"], json.loads(result.stdout)["undescribed_columns"], result.stderr.strip()),
        )
        bare = self.run_cli(*common, "--table", "users")
        self.assertEqual(
            (0, [], "NOTE: 1 column of users carries no COMMENT ON, so the files do not say what it means: id\n"
                    "NOTE: without --registry-root the description carries no Registry records; pass it to see the rules and "
                    "aggregates that mention the table."),
            (bare.returncode, json.loads(bare.stdout)["registry"], bare.stderr.strip()),
        )


if __name__ == "__main__":
    unittest.main()
