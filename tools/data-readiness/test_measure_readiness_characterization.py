#!/usr/bin/env python3
import contextlib
import io
import json
import sys
import tempfile
from pathlib import Path

import measure_readiness as mr

MIGRATION_0001 = """CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE, -- the sign-in address; never shown to other members
    -- the workspace the user signed up from
    home_workspace uuid,
    plain text
);
COMMENT ON TABLE users IS 'one row per person';
COMMENT ON COLUMN users.plain IS 'said explicitly';
COMMENT ON TABLE nobody IS 'lost';
CREATE TABLE runs (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    state text NOT NULL CHECK (state IN ('queued', 'done')),
    note text,
    CONSTRAINT runs_done_has_note CHECK (state <> 'done' OR note IS NOT NULL),
    PRIMARY KEY (id, state),
    FOREIGN KEY (user_id, note) REFERENCES users(id, plain)
);
CREATE TABLE runs_2026 PARTITION OF runs FOR VALUES FROM ('a') TO ('b');
CREATE TABLE scratch (id int);
DROP TABLE scratch;
CREATE UNIQUE INDEX runs_note_key ON runs (note);
CREATE INDEX runs_live ON runs (state) WHERE state = 'queued';
CREATE INDEX runs_plain ON runs (state);
CREATE FUNCTION guard() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'immutable; stays';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER runs_guard BEFORE UPDATE ON runs FOR EACH ROW EXECUTE FUNCTION guard();
CREATE MATERIALIZED VIEW oddity AS SELECT 1
"""

MIGRATION_0002 = """ALTER TABLE runs ADD COLUMN token_hash text REFERENCES users(email), DROP COLUMN note;
ALTER TABLE runs ADD CONSTRAINT runs_token_key UNIQUE (token_hash);
ALTER TABLE runs ALTER COLUMN state SET DEFAULT 'queued';
ALTER TABLE ghosts ADD COLUMN x int;
ALTER TABLE runs FROB everything;
"""

QUERIES_RUN = """-- name: ListRuns :many
WITH recent AS (SELECT id FROM runs)
SELECT * FROM recent JOIN users ON users.id = recent.id JOIN nowhere ON true;

-- name: MarkDone :exec
UPDATE runs SET state = 'done' WHERE id = $1;
"""

QUERIES_IDENTITY = """-- name: CreateUser :one
INSERT INTO users (id) VALUES ($1) RETURNING *;

-- name: TouchRun :exec
UPDATE runs SET state = $2 WHERE id = $1;
"""

QUERIES_ORPHAN = """-- name: Orphan :exec
UPDATE runs SET state = 'lost';
"""

OWNERS = """# owners
files:
  run.sql: run
  identity.sql: identity
queries:
  TouchRun: run
tables:
  users: identity
  runs: run, trial
immutable:
  runs: history stays
retention:
  users: kept while the account lives
  runs:
"""

RULE = """---
type: Rule
status: reviewed
sources:
  - resource: "db/migrations/0001_init.sql#L15-16"
    excerpt_sha256: "sha256:x"
  - resource: "docs/adr/README.md#L3"
    excerpt_sha256: "sha256:y"
---
A run that is done carries a note; the runs table keeps it.
"""

POLICY = """---
type: Dependency Policy
mode: synchronous-query
policy: allowed
---
Scopes resolve here.

# Relations
- From context: [run](/contexts/run)
- To context: [identity](/contexts/identity)
"""

INTERACTION = """---
type: Interaction
consistency: "eventual"
---
- [run](/contexts/run) tells [identity](/contexts/identity)
"""

FREE_TEXT_INTERACTION = """---
type: Interaction
consistency: whenever the outbox drains
---
free text
"""


GENERIC_MIGRATION = """CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    user_id uuid REFERENCES users(id),
    action text NOT NULL CHECK (action <> '')
);
"""

IDENTITY = """packages:
  - id: skillpkg
    kind: Shared Kernel
    path: shared/skillpkg
  - id: audit
    kind: Generic
    path: foundation/observability/audit
  - id: billing
    kind: Bounded Context
    path: billing
"""


def with_a_generic_table(root: Path) -> mr.Inputs:
    fixture(root)
    (root / "db" / "migrations" / "0003_audit.sql").write_text(GENERIC_MIGRATION, encoding="utf-8")
    (root / "apps" / "platform").mkdir(parents=True)
    (root / "apps" / "platform" / "architecture-identity.yaml").write_text(IDENTITY, encoding="utf-8")
    (root / "db" / "query-owners.yaml").write_text(OWNERS.replace("  runs: run, trial\n", "  runs: run, trial\n  audit_events: audit\n"), encoding="utf-8")
    return mr.read_inputs(root, root / "docs" / "domain-memory")


def fixture(root: Path) -> mr.Inputs:
    (root / "db" / "migrations").mkdir(parents=True)
    (root / "db" / "queries").mkdir()
    (root / "db" / "migrations" / "0001_init.sql").write_text(MIGRATION_0001, encoding="utf-8")
    (root / "db" / "migrations" / "0002_more.sql").write_text(MIGRATION_0002, encoding="utf-8")
    (root / "db" / "queries" / "run.sql").write_text(QUERIES_RUN, encoding="utf-8")
    (root / "db" / "queries" / "identity.sql").write_text(QUERIES_IDENTITY, encoding="utf-8")
    (root / "db" / "queries" / "orphan.sql").write_text(QUERIES_ORPHAN, encoding="utf-8")
    (root / "db" / "query-owners.yaml").write_text(OWNERS, encoding="utf-8")
    registry = root / "docs" / "domain-memory" / "registry"
    for asset, name, text in [
        ("rules", "run-done-has-note", RULE), ("dependency-policies", "run-to-identity", POLICY),
        ("interactions", "run-tells-identity", INTERACTION), ("interactions", "free", FREE_TEXT_INTERACTION),
        ("contexts", "run", "---\ntype: Context\n---\n"), ("contexts", "identity", "---\ntype: Context\n---\n"),
        ("rules", "index", "ignored listing"),
    ]:
        (registry / asset).mkdir(parents=True, exist_ok=True)
        (registry / asset / f"{name}.md").write_text(text, encoding="utf-8")
    return mr.read_inputs(root, root / "docs" / "domain-memory")


def measured():
    with tempfile.TemporaryDirectory() as root:
        inputs = fixture(Path(root))
        return inputs, mr.measure(inputs)


def ratio(report, name):
    value = report["metrics"][name]
    return value["numerator"], value["denominator"]


def test_a_semicolon_inside_a_comment_a_quote_a_dollar_body_or_parentheses_does_not_split():
    text = "CREATE TABLE a (x text -- one; two\n, y text DEFAULT 'a;b', z int CHECK (f(1;2)));\nDO $$ x; y $$;\n\n  DROP TABLE a"
    assert [(line, s[:14]) for line, s in mr._statements(text)] == \
        [(1, "CREATE TABLE a"), (3, "DO $$ x; y $$"), (5, "DROP TABLE a")]


def test_the_statement_line_is_where_its_first_word_starts_after_leading_comments():
    assert mr._statements("-- header\n\n-- more\nCREATE TABLE a (x int);")[0][0] == 4


def test_tables_columns_and_partitions_are_counted_from_create_alter_and_drop():
    inputs, report = measured()
    assert sorted(inputs.tables) == ["runs", "users"]
    assert sorted(inputs.tables["runs"].columns) == ["id", "state", "token_hash", "user_id"]
    assert report["inventory"] == {
        "migrations": 2, "tables": 2, "partitions": 1, "columns": 8, "queries": 5, "constraints": 6,
        "foreign_keys": 3, "types": 0, "registry_records": 6, "unparsed_statements": 4,
    }


def test_every_unread_statement_is_named_with_its_file_and_line():
    _, report = measured()
    assert report["details"]["unparsed_statements"] == [
        "db/migrations/0001_init.sql:10: COMMENT ON unknown table nobody",
        "db/migrations/0001_init.sql:32: CREATE MATERIALIZED VIEW oddity AS SELECT 1",
        "db/migrations/0002_more.sql:4: ALTER TABLE ghosts before its CREATE",
        "db/migrations/0002_more.sql:5: ALTER TABLE runs FROB everything",
    ]


def test_a_column_is_described_only_by_comment_on_not_by_a_comment_beside_or_above_it():
    inputs, report = measured()
    users = inputs.tables["users"].columns
    assert [(name, info["line"], info["described"]) for name, info in users.items()] == \
        [("id", 2, False), ("email", 3, False), ("home_workspace", 5, False), ("plain", 6, True)]
    assert (inputs.tables["users"].described, inputs.tables["runs"].described) == (True, False)
    assert ratio(report, "meaning.tables_described") == (1, 2)
    assert ratio(report, "meaning.columns_described") == (1, 8)


def test_column_and_table_level_rules_are_counted_and_a_primary_key_is_not():
    inputs, _ = measured()
    assert sorted((c.kind, c.name, c.file[-8:], c.line) for c in inputs.schema.constraints) == [
        ("check", "runs.state", "init.sql", 14), ("check", "runs_done_has_note", "init.sql", 16),
        ("trigger", "runs_guard", "init.sql", 31),
        ("unique", "runs_token_key", "more.sql", 2), ("unique", "users.email", "init.sql", 3),
        ("unique-index", "runs_note_key", "init.sql", 23),
    ]


def test_foreign_keys_come_from_column_references_table_constraints_and_added_columns():
    inputs, _ = measured()
    assert sorted((key.child, key.parent, key.line) for key in inputs.schema.foreign_keys) == \
        [("runs", "users", 1), ("runs", "users", 13), ("runs", "users", 18)]


def test_a_constraint_counts_as_cited_only_when_a_record_cites_its_line():
    _, report = measured()
    assert ratio(report, "rules.constraints_cited_by_a_record") == (1, 6)
    assert "db/migrations/0001_init.sql:16 check runs_done_has_note" not in report["details"]["uncited_constraints"]
    assert "db/migrations/0001_init.sql:14 check runs.state" in report["details"]["uncited_constraints"]


DRAFT_RULE = """---
type: Rule
status: draft
sources:
  - resource: "db/migrations/0001_init.sql#L14"
    excerpt_sha256: "sha256:z"
---
A run state is queued or done.

# Relations
- Context: [run](/contexts/run)
"""


def test_a_draft_record_cites_nothing_the_registry_knows_until_it_is_reviewed():
    with tempfile.TemporaryDirectory() as root:
        inputs = fixture(Path(root))
        rules = Path(root) / "docs/domain-memory/registry/rules"
        (rules / "run-state").with_suffix(".md").write_text(DRAFT_RULE, encoding="utf-8")
        draft = mr.measure(mr.read_inputs(Path(root), Path(root) / "docs/domain-memory"))
        (rules / "run-state.md").write_text(DRAFT_RULE.replace("status: draft", "status: stable"), encoding="utf-8")
        reviewed = mr.measure(mr.read_inputs(Path(root), Path(root) / "docs/domain-memory"))
    assert [record.status for record in inputs.records if record.asset == "rules"] == ["reviewed"]
    assert (ratio(draft, "rules.constraints_cited_by_a_record"), draft["details"]["constraints_cited_only_by_a_candidate"]) == ((1, 6), 1)
    assert "db/migrations/0001_init.sql:14 check runs.state" in draft["details"]["uncited_constraints"]
    assert (ratio(reviewed, "rules.constraints_cited_by_a_record"), reviewed["details"]["constraints_cited_only_by_a_candidate"]) == ((2, 6), 0)


def test_a_draft_context_or_policy_does_not_count_as_the_registry_knowing_it():
    with tempfile.TemporaryDirectory() as root:
        fixture(Path(root))
        registry = Path(root) / "docs/domain-memory/registry"
        (registry / "contexts" / "run.md").write_text("---\ntype: Context\nstatus: draft\n---\n", encoding="utf-8")
        (registry / "dependency-policies" / "run-to-identity.md").write_text(POLICY.replace("---\ntype", "---\nstatus: candidate\ntype", 1), encoding="utf-8")
        report = mr.measure(mr.read_inputs(Path(root), Path(root) / "docs/domain-memory"))
    assert ratio(report, "ownership.tables_with_a_known_owner") == (1, 2)
    assert report["details"]["tables_without_a_known_owner"] == {"runs": ["run", "trial"]}
    assert ratio(report, "rules.cross_context_foreign_keys_with_a_dependency_policy") == (0, 3)


def test_a_cross_context_foreign_key_is_covered_by_a_dependency_policy_from_child_to_parent():
    _, report = measured()
    assert ratio(report, "rules.foreign_keys_inside_one_context") == (0, 3)
    assert ratio(report, "rules.cross_context_foreign_keys_with_a_dependency_policy") == (3, 3)
    assert report["details"]["cross_context_foreign_keys_without_a_policy"] == []


def test_a_policy_in_the_other_direction_does_not_cover_the_foreign_key():
    with tempfile.TemporaryDirectory() as root:
        inputs = fixture(Path(root))
        policy = Path(root) / "docs/domain-memory/registry/dependency-policies/run-to-identity.md"
        policy.write_text(POLICY.replace("[run](/contexts/run)", "[x](/contexts/identity)", 1)
                          .replace("[identity](/contexts/identity)\n", "[run](/contexts/run)\n"), encoding="utf-8")
        report = mr.measure(mr.read_inputs(Path(root), Path(root) / "docs/domain-memory"))
    assert ratio(report, "rules.cross_context_foreign_keys_with_a_dependency_policy") == (0, 3)
    assert report["details"]["cross_context_foreign_keys_without_a_policy"] == ["runs->users (run->identity)"]


def test_queries_name_only_known_tables_and_not_their_ctes():
    inputs, _ = measured()
    assert [(q.name, q.tables, q.writes) for q in inputs.queries] == [
        ("CreateUser", ["users"], True), ("TouchRun", ["runs"], True), ("Orphan", ["runs"], True),
        ("ListRuns", ["runs", "users"], False), ("MarkDone", ["runs"], True),
    ]


def test_a_query_owner_is_declared_per_query_before_per_file_and_an_unowned_query_has_none():
    inputs, report = measured()
    assert [inputs.query_owner(q) for q in inputs.queries] == ["identity", "run", "", "run", "run"]
    assert ratio(report, "access.queries_with_an_owner") == (4, 5)


def test_an_unowned_writer_is_not_a_second_owner_of_the_table_it_writes():
    _, report = measured()
    assert ratio(report, "ownership.tables_written_by_at_most_one_owner") == (2, 2)
    assert report["details"]["multi_writer_tables"] == {}


def test_ownership_counts_declared_owners_writers_aggregates_and_touch():
    _, report = measured()
    assert ratio(report, "ownership.tables_with_exactly_one_declared_owner") == (1, 2)
    assert report["details"]["tables_without_one_owner"] == {"runs": ["run", "trial"]}
    assert ratio(report, "ownership.tables_with_a_known_owner") == (1, 2)
    assert report["details"]["tables_without_a_known_owner"] == {"runs": ["trial"]}
    assert ratio(report, "ownership.tables_written_by_at_most_one_owner") == (2, 2)
    assert ratio(report, "ownership.tables_claimed_by_an_aggregate") == (0, 2)
    assert ratio(report, "ownership.tables_touched_by_a_query") == (2, 2)


def test_a_generic_package_from_the_identity_file_is_a_known_owner_but_a_bounded_context_entry_is_not():
    with tempfile.TemporaryDirectory() as root:
        inputs = with_a_generic_table(Path(root))
        report = mr.measure(inputs)
    assert inputs.generic == {"skillpkg", "audit"}
    assert ratio(report, "ownership.tables_with_a_known_owner") == (2, 3)
    assert report["details"]["tables_without_a_known_owner"] == {"runs": ["trial"]}


def test_a_table_without_any_owner_is_not_known_even_when_nothing_is_unknown_about_it():
    with tempfile.TemporaryDirectory() as root:
        with_a_generic_table(Path(root))
        owners = Path(root) / "db/query-owners.yaml"
        owners.write_text(owners.read_text(encoding="utf-8").replace("  audit_events: audit\n", ""), encoding="utf-8")
        report = mr.measure(mr.read_inputs(Path(root), Path(root) / "docs/domain-memory"))
    assert ratio(report, "ownership.tables_with_a_known_owner") == (1, 3)
    assert report["details"]["tables_without_a_known_owner"] == {"runs": ["trial"]}


def test_a_generic_table_states_no_rule_and_its_foreign_keys_need_no_dependency_policy():
    with tempfile.TemporaryDirectory() as root:
        report = mr.measure(with_a_generic_table(Path(root)))
    assert report["inventory"]["constraints"] == 7
    assert ratio(report, "rules.constraints_cited_by_a_record") == (1, 6)
    assert ratio(report, "rules.foreign_keys_inside_one_context") == (0, 3)
    assert ratio(report, "rules.cross_context_foreign_keys_with_a_dependency_policy") == (3, 3)
    assert report["details"]["foreign_keys_from_generic_tables"] == ["audit_events->users"]
    assert "db/migrations/0003_audit.sql:4 check audit_events.action" not in report["details"]["uncited_constraints"]


def test_without_an_identity_file_no_owner_is_generic():
    with tempfile.TemporaryDirectory() as root:
        inputs = fixture(Path(root))
    assert inputs.generic == set()


def test_two_contexts_writing_one_table_are_listed_as_its_writers():
    with tempfile.TemporaryDirectory() as root:
        fixture(Path(root))
        owners = Path(root) / "db/query-owners.yaml"
        owners.write_text(OWNERS.replace("  TouchRun: run\n", ""), encoding="utf-8")
        report = mr.measure(mr.read_inputs(Path(root), Path(root) / "docs/domain-memory"))
    assert ratio(report, "ownership.tables_written_by_at_most_one_owner") == (1, 2)
    assert report["details"]["multi_writer_tables"] == {"runs": ["identity", "run"]}


def with_extra_migration(root: Path, sql: str, owners: str | None = None) -> dict:
    fixture(root)
    (root / "db" / "migrations" / "0003_extra.sql").write_text(sql, encoding="utf-8")
    if owners is not None:
        (root / "db" / "query-owners.yaml").write_text(owners, encoding="utf-8")
    return mr.measure(mr.read_inputs(root, root / "docs" / "domain-memory"))


def test_an_enum_column_counts_as_explained_only_when_its_comment_names_every_allowed_value():
    _, report = measured()
    assert ratio(report, "meaning.enum_columns_whose_comment_names_every_value") == (0, 1)
    assert report["details"]["enum_comments_missing_values"] == {"runs.state": ["queued", "done"]}
    with tempfile.TemporaryDirectory() as root:
        partial = with_extra_migration(Path(root), "COMMENT ON COLUMN runs.state IS 'queued until the worker picks it up';\n")
    with tempfile.TemporaryDirectory() as root:
        complete = with_extra_migration(Path(root), "COMMENT ON COLUMN runs.state IS 'queued until picked up, done once it finished';\n")
    assert (ratio(partial, "meaning.enum_columns_whose_comment_names_every_value"), partial["details"]["enum_comments_missing_values"]) == ((0, 1), {"runs.state": ["done"]})
    assert (ratio(complete, "meaning.enum_columns_whose_comment_names_every_value"), complete["details"]["enum_comments_missing_values"]) == ((1, 1), {})


def test_a_column_typed_with_an_enum_is_explained_by_the_type_comment_when_its_own_comment_is_silent():
    enum = "CREATE TYPE lane AS ENUM ('fast', 'slow');\nALTER TABLE runs ADD COLUMN lane lane;\n"
    with tempfile.TemporaryDirectory() as root:
        silent = with_extra_migration(Path(root), enum)
    with tempfile.TemporaryDirectory() as root:
        explained = with_extra_migration(Path(root), enum + "COMMENT ON TYPE lane IS 'fast on the highway, slow in town';\n")
    assert silent["details"]["enum_comments_missing_values"] == {"runs.lane": ["fast", "slow"], "runs.state": ["queued", "done"]}
    assert explained["details"]["enum_comments_missing_values"] == {"runs.state": ["queued", "done"]}


def test_a_list_longer_than_twelve_values_is_a_vocabulary_the_check_itself_states_not_a_comment_to_write():
    twelve = ", ".join(f"'v{index}'" for index in range(12))
    thirteen = twelve + ", 'v12'"
    with tempfile.TemporaryDirectory() as root:
        at_limit = with_extra_migration(Path(root), f"CREATE TABLE codes (code text CHECK (code IN ({twelve})));\n")
    with tempfile.TemporaryDirectory() as root:
        past_limit = with_extra_migration(Path(root), f"CREATE TABLE codes (code text CHECK (code IN ({thirteen})));\n")
    assert ratio(at_limit, "meaning.enum_columns_whose_comment_names_every_value") == (0, 2)
    assert ratio(past_limit, "meaning.enum_columns_whose_comment_names_every_value") == (0, 1)


def test_a_status_like_column_needs_a_check_that_is_exactly_its_value_list():
    _, report = measured()
    assert ratio(report, "meaning.status_like_columns_with_a_check_list") == (1, 1)
    with tempfile.TemporaryDirectory() as root:
        report = with_extra_migration(
            Path(root),
            "CREATE TYPE lane AS ENUM ('a');\n"
            "ALTER TABLE runs ADD COLUMN verdict text, ADD COLUMN provider text, ADD COLUMN mode text CHECK (mode IS NULL OR mode IN ('fast', 'slow')), "
            "ADD COLUMN decision text CHECK (decision <> 'x' OR mode = 'fast'), ADD COLUMN tier lane;\n",
        )
    assert ratio(report, "meaning.status_like_columns_with_a_check_list") == (3, 5)
    assert report["details"]["status_like_columns_without_a_check_list"] == ["runs.decision", "runs.verdict"]


def test_a_sensitive_column_is_classified_only_by_its_exact_table_dot_column_key():
    _, report = measured()
    assert ratio(report, "sensitivity.candidate_columns_classified") == (0, 2)
    assert report["details"]["unclassified_sensitive_columns"] == ["runs.token_hash", "users.email"]
    owners = OWNERS + "classification:\n  users.email: personal, anonymized at purge\n  token_hash: secret\n"
    with tempfile.TemporaryDirectory() as root:
        report = with_extra_migration(Path(root), "", owners)
    assert ratio(report, "sensitivity.candidate_columns_classified") == (1, 2)
    assert report["details"]["unclassified_sensitive_columns"] == ["runs.token_hash"]


def test_a_table_the_registry_mentions_by_name_is_known_to_it():
    _, report = measured()
    assert ratio(report, "meaning.tables_known_to_registry") == (1, 2)


def test_sensitive_candidates_are_named_columns_and_cited_only_by_line():
    _, report = measured()
    assert report["details"]["sensitive_candidates"] == ["runs.token_hash", "users.email"]
    assert ratio(report, "sensitivity.candidate_columns_cited_by_a_record") == (0, 2)


def test_a_record_citing_a_sensitive_column_line_registers_it():
    with tempfile.TemporaryDirectory() as root:
        fixture(Path(root))
        rule = Path(root) / "docs/domain-memory/registry/rules/run-done-has-note.md"
        rule.write_text(RULE.replace("#L15-16", "#L3"), encoding="utf-8")
        report = mr.measure(mr.read_inputs(Path(root), Path(root) / "docs/domain-memory"))
    assert ratio(report, "sensitivity.candidate_columns_cited_by_a_record") == (1, 2)


def test_consistency_and_provenance_read_the_registry_and_the_owners_file():
    _, report = measured()
    assert ratio(report, "consistency.interactions_with_a_known_consistency") == (1, 2)
    assert ratio(report, "provenance.migrations_cited_by_a_record") == (1, 2)
    assert ratio(report, "provenance.immutable_tables_known_to_registry") == (1, 1)
    assert ratio(report, "provenance.tables_with_a_retention_policy") == (1, 2)


def test_owner_values_split_on_commas_and_skip_comments_and_blank_lines():
    with tempfile.TemporaryDirectory() as root:
        path = Path(root) / "o.yaml"
        path.write_text("# head\n\ntables:\n  a: x, y\n  # inner\n  b: z\nscope:\n  q: platform-wide\n", encoding="utf-8")
        assert mr.read_owners(path) == {"tables": {"a": ["x", "y"], "b": ["z"]}, "scope": {"q": ["platform-wide"]}}


def test_a_ratio_with_nothing_to_count_is_none_not_zero():
    assert mr._ratio(0, 0) == {"numerator": 0, "denominator": 0, "ratio": None}
    assert mr._ratio(1, 3) == {"numerator": 1, "denominator": 3, "ratio": 0.333}


def test_render_shows_the_delta_against_a_baseline_and_na_for_empty_denominators():
    report = {"inventory": {"tables": 2}, "metrics": {"m": mr._ratio(1, 2), "e": mr._ratio(0, 0)}}
    baseline = {"metrics": {"m": mr._ratio(1, 4)}}
    assert mr.render(report, baseline).splitlines()[4:] == [
        "| m | 1/2 | 50.0% | 25.0% | +0.250 |  |",
        "| e | 0/0 | n/a |  |  |  |",
    ]
    assert mr.render(report, None).splitlines()[4] == "| m | 1/2 | 50.0% |  |  |  |"


def test_render_marks_a_target_met_on_the_target_and_below_under_it_or_with_nothing_to_count():
    report = {"inventory": {}, "metrics": {"m": mr._ratio(1, 2), "n": mr._ratio(1, 2), "e": mr._ratio(0, 0)}}
    assert mr.render(report, None, {"m": 0.5, "n": 0.501, "e": 1.0}).splitlines()[4:] == [
        "| m | 1/2 | 50.0% |  |  | 50.0% met |",
        "| n | 1/2 | 50.0% |  |  | 50.1% below |",
        "| e | 0/0 | n/a |  |  | 100.0% below |",
    ]


def test_a_regression_is_a_ratio_below_its_baseline_and_not_one_equal_higher_or_unmeasured():
    report = {"metrics": {"down": mr._ratio(1, 4), "same": mr._ratio(1, 2), "up": mr._ratio(3, 4),
                          "empty": mr._ratio(0, 0), "new": mr._ratio(1, 2)}}
    baseline = {"metrics": {"down": mr._ratio(1, 2), "same": mr._ratio(1, 2), "up": mr._ratio(1, 2),
                            "empty": mr._ratio(1, 2), "gone": mr._ratio(1, 2)}}
    assert mr.regressions(report, baseline) == ["down fell from 50.0% to 25.0%"]


def test_main_fails_only_when_asked_and_a_ratio_fell_below_the_baseline():
    with tempfile.TemporaryDirectory() as root:
        fixture(Path(root))
        higher = Path(root) / "higher.json"
        higher.write_text(json.dumps({"metrics": {"meaning.tables_described": mr._ratio(2, 2)}}), encoding="utf-8")
        same = Path(root) / "same.json"
        same.write_text(json.dumps({"metrics": {"meaning.tables_described": mr._ratio(1, 2)}}), encoding="utf-8")
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()) as errors:
            codes = (mr.main(["--repo-root", root, "--baseline", str(higher)]),
                     mr.main(["--repo-root", root, "--baseline", str(same), "--fail-on-regression"]),
                     mr.main(["--repo-root", root, "--baseline", str(higher), "--fail-on-regression"]))
    assert codes == (0, 0, 1)
    assert "ERROR: meaning.tables_described fell from 100.0% to 50.0%" in errors.getvalue()


def test_main_writes_the_json_report_and_exits_zero_even_with_unread_statements():
    with tempfile.TemporaryDirectory() as root:
        fixture(Path(root))
        out = Path(root) / "report.json"
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            code = mr.main(["--repo-root", root, "--json", str(out)])
        written = json.loads(out.read_text(encoding="utf-8"))
    assert code == 0
    assert written["inventory"]["unparsed_statements"] == 4
    assert set(written) == {"inventory", "metrics", "details"}


if __name__ == "__main__":
    failed = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"ok   {name}")
            except AssertionError as exc:
                failed += 1
                print(f"FAIL {name}: {exc}")
    sys.exit(1 if failed else 0)
