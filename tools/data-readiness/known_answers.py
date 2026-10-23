from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.request
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import measure_readiness as mr  # noqa: E402
from domain_registry.sql_describe import allowed_values, describe_table  # noqa: E402

QUESTIONS = HERE / "known-answers.jsonl"
SYSTEM_PROMPT = (
    "You answer questions about a database schema using only the documents provided. "
    'Reply with one JSON object {"answer": ...} and nothing else: a single table, column, context or class name as a string; '
    'a JSON array of names when the question asks for several, even if only one applies; "yes" or "no"; '
    'or "none" when the documents show no such thing.'
)
Post = Callable[[str, dict[str, str], dict[str, Any]], dict[str, Any]]


def load_questions(path: Path = QUESTIONS) -> list[dict[str, Any]]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


Oracle = dict[str, Any]


def _present(inputs: mr.Inputs, table: str, column: str | None) -> bool:
    return table in inputs.tables and (column is None or column in inputs.tables[table].columns)


def _allowed(inputs: mr.Inputs, oracle: Oracle) -> Any:
    table, column = oracle["table"], oracle.get("column")
    if not _present(inputs, table, column):
        return None
    constraints = [c for c in inputs.schema.constraints if c.table == table]
    declared = inputs.tables[table].columns[column].get("type", "")
    return allowed_values(column, constraints, inputs.schema.enums.get(declared))


def _owner(inputs: mr.Inputs, oracle: Oracle) -> Any:
    owners = inputs.table_owners(oracle["table"])
    return owners[0] if len(owners) == 1 else None


def _classification(inputs: mr.Inputs, oracle: Oracle) -> Any:
    declared = inputs.owners.get("classification", {}).get(f"{oracle['table']}.{oracle.get('column')}")
    return declared[0] if declared else None


def _keys_to(inputs: mr.Inputs, oracle: Oracle) -> list:
    return [key for key in inputs.schema.foreign_keys if key.child == oracle["table"] and key.parent == oracle["parent"]]


def _insert_must_supply(inputs: mr.Inputs, oracle: Oracle) -> Any:
    if not _present(inputs, oracle["table"], oracle["column"]):
        return None
    info = inputs.tables[oracle["table"]].columns[oracle["column"]]
    return _yes(info.get("not_null") and info.get("default") is None)


def _checks_of(inputs: mr.Inputs, table: str) -> list:
    return [c for c in inputs.schema.constraints if c.table == table and c.kind == "check"]


def _unique_over(inputs: mr.Inputs, oracle: Oracle) -> Any:
    wanted = set(oracle["columns"])
    constraints = [c for c in inputs.schema.constraints if c.table == oracle["table"] and c.kind in ("unique", "unique-index")]
    return _yes(not any(set(c.columns) == wanted for c in constraints))


def _yes(condition: Any) -> str:
    return "yes" if condition else "no"


ORACLES: dict[str, Callable[[mr.Inputs, Oracle], Any]] = {
    "absent": lambda inputs, oracle: None if _present(inputs, oracle["table"], oracle.get("column")) else "none",
    "allowed_values": _allowed,
    "references": lambda inputs, oracle: sorted(
        {key.parent for key in inputs.schema.foreign_keys if key.child == oracle["table"] and key.parent in inputs.tables}),
    "referenced_by": lambda inputs, oracle: sorted(
        {key.child for key in inputs.schema.foreign_keys if key.parent == oracle["table"] and key.child in inputs.tables}),
    "owner": _owner,
    "immutable": lambda inputs, oracle: _yes(oracle["table"] in inputs.owners.get("immutable", {})),
    "classification": _classification,
    "deleted_with_parent": lambda inputs, oracle: _yes(any(key.on_delete == "CASCADE" for key in _keys_to(inputs, oracle))),
    "fk_columns": lambda inputs, oracle: sorted({column for key in _keys_to(inputs, oracle) for column in key.columns}),
    "insert_must_supply": _insert_must_supply,
    "unique_over": _unique_over,
    "check_over": lambda inputs, oracle: _yes(
        not any(set(oracle["columns"]) <= set(c.columns) for c in _checks_of(inputs, oracle["table"]))),
    "check_on_column": lambda inputs, oracle: _yes(any(oracle["column"] in c.columns for c in _checks_of(inputs, oracle["table"]))),
}


def expected(question: dict[str, Any], inputs: mr.Inputs) -> Any:
    oracle = question["oracle"]
    if oracle["kind"] == "exists":
        return question["answer"] if _present(inputs, oracle["table"], oracle.get("column")) else None
    return ORACLES[oracle["kind"]](inputs, oracle)


def normalized(answer: Any) -> Any:
    if isinstance(answer, list):
        return sorted(str(item).strip().lower() for item in answer)
    return str(answer).strip().lower()


def as_list(given: Any) -> list:
    if isinstance(given, list):
        return given
    return [part for part in str(given).split(",") if part.strip()]


def correct(recorded: Any, given: Any) -> bool:
    if recorded == "none":
        return given in ("none", "", None, [])
    if isinstance(recorded, list):
        return normalized(recorded) == normalized(as_list(given))
    return normalized(recorded) == normalized(given)


def check(questions: list[dict[str, Any]], inputs: mr.Inputs) -> list[str]:
    problems = []
    for question in questions:
        truth = expected(question, inputs)
        if truth is None:
            problems.append(f"{question['id']}: the files no longer support the recorded answer {question['answer']!r}")
        elif not correct(question["answer"], truth):
            problems.append(f"{question['id']}: recorded {question['answer']!r} but the files say {truth!r}")
    return problems


def catalog(inputs: mr.Inputs) -> str:
    return "\n".join(f"{name}: {table.comment or '(no description)'}" for name, table in sorted(inputs.tables.items()))


def owner_rows(inputs: mr.Inputs, table: str) -> dict[str, Any]:
    rows: dict[str, Any] = {}
    for section in ("tables", "retention", "immutable"):
        if table in inputs.owners.get(section, {}):
            rows[section] = ", ".join(inputs.owners[section][table])
    rows["classification"] = {
        key: ", ".join(value) for key, value in inputs.owners.get("classification", {}).items() if key.startswith(table + ".")
    }
    return rows


def context_for(question: dict[str, Any], inputs: mr.Inputs, repo_root: Path, registry_root: Path | None) -> str:
    tables = question.get("tables") or []
    if not tables:
        return "Tables of the database, one per line as name: description\n" + catalog(inputs)
    parts = []
    for table in tables:
        described = describe_table(registry_root, repo_root, repo_root / "db" / "migrations", table)
        described["ownership"] = owner_rows(inputs, table)
        parts.append(json.dumps(described, indent=1))
    return "\n".join(parts)


def http_post(url: str, headers: dict[str, str], body: dict[str, Any]) -> dict[str, Any]:
    request = urllib.request.Request(url, data=json.dumps(body).encode("utf-8"), method="POST",
                                     headers={**headers, "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=120) as response:
        return json.loads(response.read().decode("utf-8"))


@dataclass
class Gateway:
    url: str
    key: str
    model: str
    post: Post = http_post

    def ask(self, context: str, question: str) -> Any:
        body = {
            "model": self.model,
            "temperature": 0,
            "messages": [
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": f"Documents:\n{context}\n\nQuestion: {question}"},
            ],
        }
        reply = self.post(self.url.rstrip("/") + "/v1/chat/completions", {"Authorization": f"Bearer {self.key}"}, body)
        text = reply["choices"][0]["message"]["content"].strip()
        if text.startswith("```"):
            text = text.strip("`").removeprefix("json").strip()
        try:
            return json.loads(text).get("answer")
        except (ValueError, AttributeError):
            return text


def ask(questions: list[dict[str, Any]], inputs: mr.Inputs, repo_root: Path, registry_root: Path | None,
        gateway: Gateway) -> dict[str, Any]:
    results = []
    for question in questions:
        context = context_for(question, inputs, repo_root, registry_root)
        given = gateway.ask(context, question["question"])
        results.append({"id": question["id"], "category": question["category"], "expected": question["answer"],
                        "given": given, "correct": correct(question["answer"], given)})
    by_category: dict[str, list[bool]] = defaultdict(list)
    for result in results:
        by_category[result["category"]].append(result["correct"])
    summary = {name: mr._ratio(sum(flags), len(flags)) for name, flags in sorted(by_category.items())}
    summary["all"] = mr._ratio(sum(r["correct"] for r in results), len(results))
    return {"model": gateway.model, "summary": summary, "results": results}


def render(report: dict[str, Any]) -> str:
    lines = [f"| category | correct | ratio |", "| --- | --- | --- |"]
    for name, ratio in report["summary"].items():
        lines.append(f"| {name} | {ratio['numerator']}/{ratio['denominator']} | {ratio['ratio']} |")
    wrong = [r for r in report["results"] if not r["correct"]]
    lines += [f"wrong {r['id']}: expected {r['expected']!r}, given {r['given']!r}" for r in wrong]
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=["check", "ask"])
    parser.add_argument("--repo-root", type=Path, default=Path.cwd())
    parser.add_argument("--registry-root", type=Path, default=None)
    parser.add_argument("--questions", type=Path, default=QUESTIONS)
    parser.add_argument("--gateway", default=os.environ.get("LITELLM_BASE_URL", ""))
    parser.add_argument("--key-env", default="LITELLM_API_KEY")
    parser.add_argument("--model", default="gpt-5.4-mini")
    parser.add_argument("--json", type=Path, default=None)
    args = parser.parse_args(argv)
    registry_root = args.registry_root or args.repo_root / "docs" / "domain-memory"
    inputs = mr.read_inputs(args.repo_root, registry_root)
    questions = load_questions(args.questions)
    if args.command == "check":
        problems = check(questions, inputs)
        print("\n".join(problems) if problems else f"{len(questions)} known answers still hold")
        return 1 if problems else 0
    key = os.environ.get(args.key_env, "")
    if not args.gateway or not key:
        print(f"ask needs --gateway (or LITELLM_BASE_URL) and the key in ${args.key_env}", file=sys.stderr)
        return 2
    report = ask(questions, inputs, args.repo_root, registry_root, Gateway(args.gateway, key, args.model))
    if args.json:
        args.json.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(render(report))
    return 0


if __name__ == "__main__":
    sys.exit(main())
