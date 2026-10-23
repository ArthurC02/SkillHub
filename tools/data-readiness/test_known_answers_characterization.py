#!/usr/bin/env python3
import json
import sys
import tempfile
from pathlib import Path

import known_answers as ka
import measure_readiness as mr

MIGRATION = """CREATE TYPE lane AS ENUM ('fast', 'slow');
CREATE TABLE users (id uuid PRIMARY KEY, email text NOT NULL);
COMMENT ON TABLE users IS 'one row per person';
CREATE TABLE runs (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    state text NOT NULL CHECK (state IN ('queued', 'done')),
    speed lane
);
COMMENT ON TABLE runs IS 'one execution';
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


def inputs_of(root: Path) -> mr.Inputs:
    return mr.read_inputs(root, root / "docs" / "domain-memory")


def question(kind: str, table: str, answer, column: str | None = None, **extra) -> dict:
    oracle = {"kind": kind, "table": table}
    if column:
        oracle["column"] = column
    return {"id": f"{kind}-{table}", "category": kind, "question": "?", "answer": answer, "oracle": oracle, **extra}


def test_every_oracle_kind_reads_its_answer_from_the_files():
    inputs = inputs_of(repo())
    cases = [
        (question("exists", "runs", "runs"), "runs"),
        (question("exists", "runs", "state", column="state"), "state"),
        (question("exists", "runs", "gone", column="gone"), None),
        (question("absent", "runs", "none", column="gone"), "none"),
        (question("absent", "runs", "none", column="state"), None),
        (question("allowed_values", "runs", ["queued", "done"], column="state"), ["queued", "done"]),
        (question("allowed_values", "runs", ["fast", "slow"], column="speed"), ["fast", "slow"]),
        (question("references", "runs", ["users"]), ["users"]),
        (question("referenced_by", "users", ["runs"]), ["runs"]),
        (question("owner", "runs", "run"), "run"),
        (question("immutable", "runs", "yes"), "yes"),
        (question("immutable", "users", "no"), "no"),
        (question("classification", "users", "personal", column="email"), "personal"),
        (question("classification", "users", "personal", column="id"), None),
    ]
    assert [ka.expected(q, inputs) for q, _ in cases] == [truth for _, truth in cases]


def test_check_names_every_question_whose_recorded_answer_the_files_contradict_or_no_longer_support():
    inputs = inputs_of(repo())
    questions = [
        question("allowed_values", "runs", ["done", "queued"], column="state"),
        question("owner", "runs", "identity"),
        question("exists", "runs", "gone", column="gone"),
    ]
    assert ka.check(questions, inputs) == [
        "owner-runs: recorded 'identity' but the files say 'run'",
        "exists-runs: the files no longer support the recorded answer 'gone'",
    ]


def test_grading_ignores_case_order_and_whitespace_but_not_content():
    assert ka.correct(["users", "runs"], [" Runs", "USERS "])
    assert ka.correct("Run", " run ")
    assert ka.correct("none", []) and ka.correct("none", "none") and ka.correct("none", None)
    assert not ka.correct(["users", "runs"], ["users"])
    assert not ka.correct("none", "users")
    assert not ka.correct("yes", "no")


def test_asking_sends_the_catalog_for_a_question_without_tables_and_the_described_tables_otherwise():
    root = repo()
    inputs = inputs_of(root)
    sent = []

    def post(url, headers, body):
        sent.append((url, headers["Authorization"], body["model"], body["messages"][1]["content"]))
        return {"choices": [{"message": {"content": '```json\n{"answer": "Runs"}\n```'}}]}

    report = ka.ask(
        [question("exists", "runs", "runs"), question("owner", "runs", "run", tables=["runs"])],
        inputs, root, None, ka.Gateway("http://gw/", "secret", "mini", post=post),
    )
    assert [(url, auth, model) for url, auth, model, _ in sent] == [("http://gw/v1/chat/completions", "Bearer secret", "mini")] * 2
    assert "runs: one execution" in sent[0][3] and "users: one row per person" in sent[0][3]
    described = json.loads(sent[1][3].split("Documents:\n", 1)[1].split("\n\nQuestion:", 1)[0])
    assert (described["table"], described["ownership"]["tables"], described["ownership"]["immutable"]) == ("runs", "run", "history")
    assert [r["correct"] for r in report["results"]] == [True, False]
    assert report["summary"]["all"] == {"numerator": 1, "denominator": 2, "ratio": 0.5}


def test_the_recorded_answers_of_the_repository_still_hold():
    root = Path(__file__).resolve().parents[2]
    inputs = mr.read_inputs(root, root / "docs" / "domain-memory")
    assert ka.check(ka.load_questions(), inputs) == []


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
