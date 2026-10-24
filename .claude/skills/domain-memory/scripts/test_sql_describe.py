import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.evidence import citation
from domain_registry.sql_describe import describe_table, gaps_since
from test_support import stored_policy_of
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
            [("primary", "orders_pkey"), ("check", "orders_state_check"), ("check", "orders_kind_check"), ("check", "orders_note_when_paid"),
             ("check", "orders_source_when_gift"), ("unique-index", "orders_note_key"), ("check", "orders_mood_check_check"),
             ("check", "orders_only_one_check"), ("check", "orders_strict_check"), ("foreign", "orders_buyer_id_fkey")],
            [(constraint["kind"], constraint["name"]) for constraint in described["constraints"]],
        )
        self.assertEqual(
            ({"kind": "foreign", "name": "orders_buyer_id_fkey", "text": "buyer_id uuid REFERENCES users (id)", "at": "db/migrations/0001_init.sql:4"},
             [{"parent": "users", "name": "orders_buyer_id_fkey", "columns": ["buyer_id"], "parent_columns": ["id"], "on_delete": "NO ACTION",
               "on_update": "NO ACTION", "at": "db/migrations/0001_init.sql:4"}],
             [{"child": "shipments", "name": "shipments_order_id_fkey", "columns": ["order_id"], "parent_columns": ["id"], "on_delete": "NO ACTION",
               "on_update": "NO ACTION", "at": "db/migrations/0001_init.sql:12"}]),
            (described["constraints"][-1], described["references"], described["referenced_by"]))
        self.assertEqual(
            ([{"name": "orders_note_key", "unique": True, "definition": "(note)", "at": "db/migrations/0001_init.sql:14"}],
             "db/migrations/0001_init.sql:2", "db/migrations/0001_init.sql:24"),
            (described["indexes"], described["columns"][0]["at"], described["columns"][6]["at"]))
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

    def test_statements_under_queries_that_name_the_table_are_listed_with_their_verb_and_the_columns_they_use(self) -> None:
        queries = self.repo / "db" / "queries"
        queries.mkdir()
        (queries / "orders.sql").write_text(
            "-- name: GetOrder :one\nSELECT id, state FROM orders WHERE id = $1;\n\n"
            "UPDATE orders SET note = $2\nWHERE id = $1;\n"
            "WITH recent AS (SELECT order_id FROM shipments) DELETE FROM orders WHERE id IN (SELECT order_id FROM recent);\n"
            "SELECT order_id -- FROM orders\nFROM shipments WHERE label = 'FROM orders';\n"
            "INSERT INTO public.orders (id, state) VALUES ($1, 'open');\n"
            "SELECT count(*) AS orders FROM shipments;\n", encoding="utf-8")
        common = ("describe-table", "--repo-root", str(self.repo), "--migrations", str(self.migrations), "--queries", "db/queries")
        result = self.run_cli(*common, "--table", "orders")
        self.assertEqual(
            [{"at": "db/queries/orders.sql:2", "verb": "SELECT", "columns": ["id", "state"]},
             {"at": "db/queries/orders.sql:4", "verb": "UPDATE", "columns": ["id", "note"]},
             {"at": "db/queries/orders.sql:6", "verb": "DELETE", "columns": ["id"]},
             {"at": "db/queries/orders.sql:9", "verb": "INSERT", "columns": ["id", "state"]}],
            json.loads(result.stdout)["queries"])
        none = self.run_cli(*common, "--table", "users")
        self.assertEqual(([], "NOTE: no statement under db/queries names users"),
                         (json.loads(none.stdout)["queries"], none.stderr.strip().splitlines()[0]))
        outside = self.run_cli(*common[:-1], "../elsewhere", "--table", "orders")
        self.assertEqual((1, True), (outside.returncode, outside.stderr.startswith("ERROR: --queries resolves outside --repo-root")))
        self.assertNotIn("queries", self.describe("orders"))

    def test_an_unknown_table_is_refused_with_the_closest_names(self) -> None:
        with self.assertRaisesRegex(ValueError, r"^no table 'order' in .*; did you mean orders\?$"):
            self.describe("order")
        with self.assertRaisesRegex(ValueError, r"^no table 'zzzz' in [^;]*$"):
            self.describe("zzzz")
        with self.assertRaisesRegex(ValueError, r"^no table 'orders ' in .*; did you mean orders, users\?$"):
            self.describe("orders ")
        padded = self.run_cli("describe-table", "--repo-root", str(self.repo), "--migrations", str(self.migrations), "--table", " orders ")
        self.assertEqual((0, "orders"), (padded.returncode, json.loads(padded.stdout)["table"]))

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


PARENT_AND_CHILD = """CREATE TABLE parent (id uuid PRIMARY KEY, ws uuid NOT NULL, UNIQUE (id, ws));
CREATE TABLE child (
    id uuid PRIMARY KEY,
    ws uuid NOT NULL,
    parent_id uuid NOT NULL REFERENCES parent (id),
    legacy text CHECK (legacy <> ''),
    name text NOT NULL CHECK (name ~ '^[a-z]+$') UNIQUE,
    qty int CHECK (qty > 0) CHECK (qty < 9),
    owner uuid,
    enabled boolean NOT NULL DEFAULT false,
    both_cols text DEFAULT 'a,b'::text,
    UNIQUE (ws, name),
    CHECK (NOT enabled OR owner IS NOT NULL),
    CHECK (legacy IS NULL OR both_cols IS NULL),
    CHECK (qty IS NULL OR qty <> 5)
);
CREATE UNIQUE INDEX child_legacy_key ON child (legacy);
CREATE INDEX child_legacy_both ON child (legacy, both_cols);
CREATE INDEX child_owner_idx ON child (owner) WHERE enabled;
CREATE TABLE note (id uuid PRIMARY KEY, child_id uuid REFERENCES child ON DELETE SET NULL ON UPDATE CASCADE);
"""
REWRITE = """ALTER TABLE child DROP CONSTRAINT child_parent_id_fkey,
    ADD FOREIGN KEY (parent_id, ws) REFERENCES parent (id, ws) ON DELETE CASCADE,
    DROP COLUMN legacy,
    DROP CONSTRAINT child_ws_name_key,
    ALTER COLUMN enabled SET DEFAULT true,
    ALTER COLUMN both_cols DROP DEFAULT,
    ADD CHECK (qty > 1),
    ADD COLUMN extra text DEFAULT 'x';
"""


class RewrittenTableDescribeTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.migrations = self.repo / "db" / "migrations"
        self.migrations.mkdir(parents=True)
        (self.migrations / "0001_init.sql").write_text(PARENT_AND_CHILD, encoding="utf-8")

    def describe(self, table: str) -> dict:
        return describe_table(None, self.repo, self.migrations, table)

    def test_before_the_rewrite_every_constraint_carries_the_name_the_database_gives_it(self) -> None:
        described = self.describe("child")
        self.assertEqual(
            [("primary", "child_pkey"), ("check", "child_legacy_check"), ("check", "child_name_check"), ("unique", "child_name_key"),
             ("check", "child_qty_check"), ("check", "child_qty_check1"), ("unique", "child_ws_name_key"), ("check", "child_check"),
             ("check", "child_check1"), ("check", "child_qty_check2"), ("unique-index", "child_legacy_key"),
             ("foreign", "child_parent_id_fkey")],
            [(constraint["kind"], constraint["name"]) for constraint in described["constraints"]],
        )
        self.assertEqual(
            (["child_legacy_key", "child_legacy_both", "child_owner_idx"], ["false", "'a,b'::text"],
             [{"child": "note", "name": "note_child_id_fkey", "columns": ["child_id"], "parent_columns": ["id"], "on_delete": "SET NULL",
               "on_update": "CASCADE", "at": "db/migrations/0001_init.sql:20"}]),
            ([index["name"] for index in described["indexes"]],
             [column["default"] for column in described["columns"] if column["name"] in ("enabled", "both_cols")],
             described["referenced_by"]),
        )
        self.assertEqual([("primary", "parent_pkey"), ("unique", "parent_id_ws_key")],
                         [(constraint["kind"], constraint["name"]) for constraint in self.describe("parent")["constraints"]])

    def test_after_the_rewrite_only_what_the_database_would_still_hold_is_described(self) -> None:
        (self.migrations / "0002_rewrite.sql").write_text(REWRITE, encoding="utf-8")
        described = self.describe("child")
        self.assertEqual(
            [("primary", "child_pkey"), ("check", "child_name_check"), ("unique", "child_name_key"), ("check", "child_qty_check"),
             ("check", "child_qty_check1"), ("check", "child_check"), ("check", "child_qty_check2"), ("check", "child_qty_check3"),
             ("foreign", "child_parent_id_ws_fkey")],
            [(constraint["kind"], constraint["name"]) for constraint in described["constraints"]],
        )
        self.assertEqual(
            ([{"parent": "parent", "name": "child_parent_id_ws_fkey", "columns": ["parent_id", "ws"], "parent_columns": ["id", "ws"],
               "on_delete": "CASCADE", "on_update": "NO ACTION", "at": "db/migrations/0002_rewrite.sql:1"}],
             [{"name": "child_owner_idx", "unique": False, "definition": "(owner) WHERE enabled", "at": "db/migrations/0001_init.sql:19"}]),
            (described["references"], described["indexes"]),
        )
        self.assertEqual(
            [("enabled", "true", "db/migrations/0001_init.sql:10"), ("both_cols", None, "db/migrations/0001_init.sql:11"),
             ("extra", "'x'", "db/migrations/0002_rewrite.sql:1")],
            [(column["name"], column["default"], column["at"]) for column in described["columns"] if column["name"] in ("enabled", "both_cols", "extra")],
        )
        self.assertNotIn("legacy", [column["name"] for column in described["columns"]])
        self.assertEqual(
            [("foreign", "child_parent_id_fkey"), ("check", "child_legacy_check"), ("check", "child_check1"),
             ("unique-index", "child_legacy_key"), ("unique", "child_ws_name_key")],
            [(gone.kind, gone.name) for gone in read_schema(self.migrations, self.repo).retired if gone.table == "child"],
        )
        self.assertEqual({"db/migrations/0002_rewrite.sql:1"}, {gone.replaced_at for gone in read_schema(self.migrations, self.repo).retired})

WIDENED = """ALTER TABLE orders DROP CONSTRAINT orders_state_check,
    ADD CONSTRAINT orders_state_check CHECK (state IN ('open', 'paid', 'it''s odd', 'void'));
"""
FENCE = """DO $$ BEGIN
    EXECUTE format('CREATE TRIGGER fence BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard()', 'users');
END $$;
CREATE TABLE loose (
    id uuid PRIMARY KEY, mode text, updated_at timestamptz,
    step text NOT NULL CHECK (step IN ('run', 'running')),
    role text,
    level text CHECK (level <> ''),
    category text,
    kind text,
    grade text CHECK (grade IN ('a', 'b')),
    rank text CHECK (rank IN ('x', 'y')) CHECK (rank <> 'z'),
    CHECK ((grade IS NULL) = (updated_at IS NULL))
);
COMMENT ON COLUMN loose.step IS 'running: the step is executing; run: it finished.';
COMMENT ON COLUMN loose.role IS 'free text job title';
COMMENT ON COLUMN loose.category IS 'Not free text: one of the vendor labels';
COMMENT ON COLUMN loose.kind IS 'Free-text label';
"""
TIDY = """CREATE TABLE tidy (id uuid PRIMARY KEY, state text NOT NULL CHECK (state IN ('on', 'off')), owner uuid REFERENCES users (id) ON DELETE CASCADE);
COMMENT ON TABLE tidy IS 'one switch per owner';
COMMENT ON COLUMN tidy.id IS 'the switch';
COMMENT ON COLUMN tidy.state IS 'on while lit; off otherwise';
COMMENT ON COLUMN tidy.owner IS 'who flips it';
"""
RESTATED_FENCE = """CREATE TRIGGER fence BEFORE INSERT ON orders FOR EACH ROW EXECUTE FUNCTION guard();
DO $$ BEGIN
    EXECUTE format('CREATE TRIGGER fence BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard()', 'users', 'orders');
END $$;
DO $$ BEGIN
    EXECUTE format('CREATE TRIGGER %I BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard()', 'fence', 'orders');
END $$;
"""
RESTATEMENT = """DROP TRIGGER fence ON users;
CREATE TRIGGER fence BEFORE INSERT ON users FOR EACH ROW EXECUTE FUNCTION guard();
CREATE TRIGGER other BEFORE INSERT ON orders FOR EACH ROW EXECUTE FUNCTION guard();
"""
ORDERS_GAPS = [
    "the comment on state does not say what these values mean: it's odd",
    "column buyer_id has no COMMENT ON saying what it means",
    "column note has no COMMENT ON saying what it means",
    "column kind has no COMMENT ON saying what it means",
    "column source has no COMMENT ON saying what it means",
    "column mood has no COMMENT ON saying what it means",
    "column mood_check has no COMMENT ON saying what it means",
    "column mood_check lists its values without deciding NULL; make it NOT NULL, or write CHECK (mood_check IS NULL OR "
    "mood_check IN (...)) or CHECK (mood_check IS NOT NULL AND mood_check IN (...))",
    "column only_one has no COMMENT ON saying what it means",
    "column strict has no COMMENT ON saying what it means",
    "foreign key orders_buyer_id_fkey to users does not say ON DELETE; choose CASCADE, RESTRICT, SET NULL or NO ACTION",
]


class CompletenessDescribeTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.migrations = self.repo / "db" / "migrations"
        self.migrations.mkdir(parents=True)
        (self.migrations / "0001_init.sql").write_text(MIGRATION, encoding="utf-8")
        self.common = ("describe-table", "--repo-root", str(self.repo), "--migrations", str(self.migrations))

    def describe(self, table: str, registry: bool = False) -> dict:
        return describe_table(self.repo / "memory" if registry else None, self.repo, self.migrations, table)

    def test_a_record_citing_a_constraint_a_later_migration_replaced_is_flagged_and_noted(self) -> None:
        (self.migrations / "0002_widen.sql").write_text(WIDENED, encoding="utf-8")
        self.seed("rules.json", [
            {"id": "old-list", "contexts": ["sales"], "statement": "A state is open or paid.", "status": "draft",
             "evidence": [citation(self.repo, "db/migrations/0001_init.sql", 3, 3)]},
            {"id": "new-list", "contexts": ["sales"], "statement": "A state may be void.", "status": "draft",
             "evidence": [citation(self.repo, "db/migrations/0002_widen.sql", 1, 2)]},
        ])
        self.assertEqual(
            [("new-list", None), ("old-list", {"kind": "check", "name": "orders_state_check", "at": "db/migrations/0001_init.sql:3",
                                               "replaced_at": "db/migrations/0002_widen.sql:1"})],
            [(mention["id"], mention["cites_replaced"]) for mention in self.describe("orders", registry=True)["registry"]],
        )
        result = self.run_cli(*self.common, "--table", "orders", "--registry-root", str(self.repo / "memory"))
        self.assertIn("NOTE: 1 Registry record cites a constraint a later migration replaced; the constraints listed above are the "
                      "ones in force: old-list", result.stderr)

    def test_gaps_name_missing_meanings_undecided_nulls_unexplained_values_silent_deletes_and_run_time_ddl(self) -> None:
        (self.migrations / "0002_fence.sql").write_text(FENCE, encoding="utf-8")
        orders, users = self.describe("orders"), self.describe("users")
        self.assertEqual((ORDERS_GAPS, []), (orders["gaps"], orders["dynamic_ddl"]))
        self.assertEqual(
            (["the table has no COMMENT ON saying what one row is",
              "the statement at db/migrations/0002_fence.sql:1 builds DDL at run time and names users; the files cannot show what it "
              "creates, so restate it on this table with a static CREATE TRIGGER of the trigger it names"],
             [{"at": "db/migrations/0002_fence.sql:1", "statement": "EXECUTE format('CREATE TRIGGER fence BEFORE INSERT ON %I FOR EACH "
                                                                      "ROW EXECUTE FUNCTION guard()', 'users'); END $$"}]),
            (users["gaps"], users["dynamic_ddl"]),
        )
        self.assertEqual(
            ["the table has no COMMENT ON saying what one row is", "column mode has no COMMENT ON saying what it means",
             "status-like column mode has no value list; add CHECK (mode IN (...)) or an enum type, or begin its comment with "
             "'free text' or constrain it with another CHECK",
             "column level has no COMMENT ON saying what it means",
             "status-like column category has no value list; add CHECK (category IN (...)) or an enum type, or begin its comment "
             "with 'free text' or constrain it with another CHECK",
             "column grade has no COMMENT ON saying what it means",
             "column rank has no COMMENT ON saying what it means",
             "column rank lists its values without deciding NULL; make it NOT NULL, or write CHECK (rank IS NULL OR rank IN (...)) "
             "or CHECK (rank IS NOT NULL AND rank IN (...))"],
            self.describe("loose")["gaps"],
        )
        with_run = FENCE.replace("running: the step is executing; run: it finished.", "running: the step is executing.")
        (self.migrations / "0002_fence.sql").write_text(with_run, encoding="utf-8")
        self.assertIn("the comment on step does not say what these values mean: run", self.describe("loose")["gaps"])
        result = self.run_cli(*self.common, "--table", "users")
        self.assertIn("NOTE: 1 statement builds DDL at run time naming users, which the description cannot show: "
                      "db/migrations/0002_fence.sql:1", result.stderr)

    def test_require_complete_fails_on_a_table_with_gaps_and_passes_a_fully_described_one(self) -> None:
        (self.migrations / "0002_tidy.sql").write_text(TIDY, encoding="utf-8")
        refused = self.run_cli(*self.common, "--table", "orders", "--require-complete")
        lenient = self.run_cli(*self.common, "--table", "orders")
        tidy = self.run_cli(*self.common, "--table", "tidy", "--require-complete")
        self.assertEqual((1, 11, 0, 0, []), (refused.returncode, len(json.loads(refused.stdout)["gaps"]), lenient.returncode,
                                             tidy.returncode, json.loads(tidy.stdout)["gaps"]))
        self.assertIn("NOTE: orders is not complete: 11 gaps listed under gaps; the change is not done until they are closed",
                      refused.stderr)
        self.assertNotIn("not complete", lenient.stderr + tidy.stderr)

    def test_a_later_static_trigger_of_the_templated_name_supersedes_the_run_time_ddl_for_that_table_only(self) -> None:
        (self.migrations / "0002_fence.sql").write_text(RESTATED_FENCE, encoding="utf-8")
        self.commit_all("fence")
        (self.migrations / "0003_restate.sql").write_text(RESTATEMENT, encoding="utf-8")
        users, orders = self.describe("users"), self.describe("orders")
        self.assertEqual(
            ([], ["the table has no COMMENT ON saying what one row is"],
             ["db/migrations/0002_fence.sql:2", "db/migrations/0002_fence.sql:5"],
             ["fence", "other"]),
            (users["dynamic_ddl"], users["gaps"], [statement["at"] for statement in orders["dynamic_ddl"]],
             [c["name"] for c in orders["constraints"] if c["kind"] == "trigger"]),
        )
        since = gaps_since(self.repo, self.migrations, "HEAD")
        self.assertEqual(
            {"users": {"new_gaps": [], "closed_gaps": [
                "the statement at db/migrations/0002_fence.sql:2 builds DDL at run time and names users; the files cannot show what "
                "it creates, so restate it on this table with a static CREATE TRIGGER of the trigger it names"]}},
            since["tables"],
        )
        quiet = self.run_cli(*self.common, "--table", "users")
        self.assertNotIn("builds DDL", quiet.stderr)

    def test_a_relative_migrations_path_resolves_against_the_repo_root_and_one_outside_it_is_refused(self) -> None:
        self.commit_all("migrations")
        relative = self.run_cli("describe-table", "--repo-root", str(self.repo), "--migrations", "db/migrations", "--table", "orders")
        since = self.run_cli("describe-table", "--repo-root", str(self.repo), "--migrations", "db/migrations", "--base", "HEAD")
        extracted = self.run_cli("extract-sql-schema", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                                 "--migrations", "db/migrations")
        self.assertEqual((0, "orders", 0, {}, 0, 1),
                         (relative.returncode, json.loads(relative.stdout)["table"], since.returncode, json.loads(since.stdout)["tables"],
                          extracted.returncode, json.loads(extracted.stdout)["migrations"]))
        outside = self.run_cli("describe-table", "--repo-root", str(self.repo), "--migrations", "../elsewhere", "--table", "orders")
        self.assertEqual(
            (1, f"ERROR: --migrations resolves outside --repo-root ({(self.repo / '..' / 'elsewhere').resolve()}); pass it relative "
                "to the repo root or as an absolute path inside it"),
            (outside.returncode, outside.stderr.strip()),
        )
        missing = self.run_cli("describe-table", "--repo-root", str(self.repo), "--migrations", "db/nope", "--table", "orders")
        (self.repo / "db" / "empty").mkdir()
        empty = self.run_cli("extract-sql-schema", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                             "--migrations", "db/empty")
        self.assertEqual(
            (1, f"ERROR: --migrations directory {(self.repo / 'db' / 'nope').resolve()} does not exist",
             1, f"ERROR: --migrations directory {(self.repo / 'db' / 'empty').resolve()} holds no .sql files"),
            (missing.returncode, missing.stderr.strip(), empty.returncode, empty.stderr.strip()),
        )

    def test_the_policy_schema_conventions_decide_which_columns_are_exempt_status_like_open_and_explained(self) -> None:
        self.commit_all("migrations")
        with_run = FENCE.replace("running: the step is executing; run: it finished.", "running: the step is executing.")
        (self.migrations / "0002_fence.sql").write_text(with_run, encoding="utf-8")
        policy = stored_policy_of(self.repo)
        policy["schema_conventions"] = {"exempt_columns": ["id", "mode"], "status_like_suffixes": ["role"],
                                        "free_text_markers": ["open text"], "value_comment_limit": 1}
        (self.repo / "memory" / "domain-memory-policy.json").write_text(json.dumps(policy), encoding="utf-8")
        tailored = [
            "the table has no COMMENT ON saying what one row is",
            "column updated_at has no COMMENT ON saying what it means",
            "status-like column role has no value list; add CHECK (role IN (...)) or an enum type, or begin its comment with "
            "'open text' or constrain it with another CHECK",
            "column level has no COMMENT ON saying what it means",
            "column grade has no COMMENT ON saying what it means",
            "column rank has no COMMENT ON saying what it means",
            "column rank lists its values without deciding NULL; make it NOT NULL, or write CHECK (rank IS NULL OR rank IN (...)) "
            "or CHECK (rank IS NOT NULL AND rank IN (...))",
        ]
        with_policy = self.run_cli(*self.common, "--table", "loose", "--registry-root", str(self.repo / "memory"))
        without = self.run_cli(*self.common, "--table", "loose")
        since = self.run_cli(*self.common, "--base", "HEAD", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(
            (tailored, 9, tailored),
            (json.loads(with_policy.stdout)["gaps"], len(json.loads(without.stdout)["gaps"]),
             json.loads(since.stdout)["tables"]["loose"]["new_gaps"]),
        )
        self.assertIn("the comment on step does not say what these values mean: run", json.loads(without.stdout)["gaps"])

    def test_without_a_table_the_catalog_lists_every_live_table_and_require_complete_fails_while_one_has_gaps(self) -> None:
        (self.migrations / "0002_tidy.sql").write_text(TIDY, encoding="utf-8")
        listed = self.run_cli(*self.common)
        tables = json.loads(listed.stdout)["tables"]
        self.assertEqual(
            (0, ["orders", "shipments", "tidy", "users"],
             {"table": "orders", "declared_at": "db/migrations/0001_init.sql:1",
              "meaning": "One row per order; the buyer's request as placed.", "columns": 10, "undescribed_columns": 9,
              "gaps": len(ORDERS_GAPS)},
             {"table": "tidy", "declared_at": "db/migrations/0002_tidy.sql:1", "meaning": "one switch per owner", "columns": 3,
              "undescribed_columns": 0, "gaps": 0},
             {"table": "users", "declared_at": "db/migrations/0001_init.sql:11", "meaning": None, "columns": 1,
              "undescribed_columns": 1, "gaps": 1},
             "NOTE: 4 tables, 3 with gaps: orders, shipments, users; --table <name> describes one"),
            (listed.returncode, [entry["table"] for entry in tables], tables[0], tables[2], tables[3], listed.stderr.strip()),
        )
        refused = self.run_cli(*self.common, "--require-complete")
        self.assertEqual((1, tables), (refused.returncode, json.loads(refused.stdout)["tables"]))

    def test_history_lists_in_order_the_migrations_that_declared_altered_indexed_triggered_or_commented_the_table(self) -> None:
        (self.migrations / "0002_change.sql").write_text(
            "ALTER TABLE orders ADD COLUMN memo text;\n"
            "CREATE TRIGGER fence BEFORE INSERT ON shipments FOR EACH ROW EXECUTE FUNCTION guard();\n" + TIDY, encoding="utf-8")
        (self.migrations / "0003_users.sql").write_text(
            "COMMENT ON TABLE users IS 'one account';\nCOMMENT ON COLUMN orders.note IS 'what the buyer wrote';\n"
            "CREATE INDEX shipments_order_idx ON shipments (order_id);\nDROP TABLE tidy;\nCREATE TABLE tidy (id uuid PRIMARY KEY);\n",
            encoding="utf-8")
        self.assertEqual(
            (["db/migrations/0001_init.sql", "db/migrations/0002_change.sql", "db/migrations/0003_users.sql"],
             ["db/migrations/0001_init.sql", "db/migrations/0003_users.sql"],
             ["db/migrations/0001_init.sql", "db/migrations/0002_change.sql", "db/migrations/0003_users.sql"],
             ["db/migrations/0003_users.sql"]),
            tuple(self.describe(table)["history"] for table in ("orders", "users", "shipments", "tidy")),
        )

    def test_gaps_since_a_ref_lists_only_what_the_change_opened_or_closed_per_table(self) -> None:
        self.commit_all("migrations")
        (self.migrations / "0002_change.sql").write_text(
            "ALTER TABLE orders ADD COLUMN memo text;\nCOMMENT ON COLUMN orders.note IS 'what the buyer wrote';\n" + TIDY
            + "CREATE TABLE loose (id uuid PRIMARY KEY, mode text);\n", encoding="utf-8")
        since = gaps_since(self.repo, self.migrations, "HEAD")
        self.assertEqual(
            (["loose", "orders"], 4, ["column memo has no COMMENT ON saying what it means"],
             ["column note has no COMMENT ON saying what it means"],
             ["the table has no COMMENT ON saying what one row is", "column mode has no COMMENT ON saying what it means",
              "status-like column mode has no value list; add CHECK (mode IN (...)) or an enum type, or begin its comment with "
              "'free text' or constrain it with another CHECK"], []),
            (sorted(since["tables"]), since["new_gaps"], since["tables"]["orders"]["new_gaps"], since["tables"]["orders"]["closed_gaps"],
             since["tables"]["loose"]["new_gaps"], since["tables"]["loose"]["closed_gaps"]),
        )
        refused = self.run_cli(*self.common, "--base", "HEAD", "--require-complete")
        self.assertEqual((1, "NOTE: 4 new gaps since HEAD in loose (3), orders (1); the change is not done until they are closed"),
                         (refused.returncode, refused.stderr.strip()))
        scoped = self.run_cli(*self.common, "--table", "orders", "--base", "HEAD", "--require-complete")
        self.assertEqual(
            (1, ["orders"], 1, "NOTE: 1 new gap since HEAD in orders (1); the change is not done until they are closed"),
            (scoped.returncode, sorted(json.loads(scoped.stdout)["tables"]), json.loads(scoped.stdout)["new_gaps"], scoped.stderr.strip()),
        )
        settled_table = self.run_cli(*self.common, "--table", "tidy", "--base", "HEAD", "--require-complete")
        self.assertEqual((0, {}, "NOTE: no new gaps since HEAD"),
                         (settled_table.returncode, json.loads(settled_table.stdout)["tables"], settled_table.stderr.strip()))
        unknown = self.run_cli(*self.common, "--table", "order", "--base", "HEAD")
        self.assertEqual((1, True), (unknown.returncode, unknown.stderr.startswith("ERROR: no table 'order' in ")))
        self.commit_all("change")
        settled = self.run_cli(*self.common, "--base", "HEAD", "--require-complete")
        self.assertEqual((0, {}, "NOTE: no new gaps since HEAD"), (settled.returncode, json.loads(settled.stdout)["tables"], settled.stderr.strip()))
        listed = self.run_cli(*self.common)
        self.assertEqual((0, ["loose", "orders", "shipments", "tidy", "users"]),
                         (listed.returncode, [entry["table"] for entry in json.loads(listed.stdout)["tables"]]))
        with self.assertRaisesRegex(ValueError, r"^could not read db/migrations at nowhere: "):
            gaps_since(self.repo, self.migrations, "nowhere")


if __name__ == "__main__":
    unittest.main()
