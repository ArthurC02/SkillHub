import json
import pathlib

import pytest
from jsonschema import Draft202012Validator

from skillhub_llm.agents import INSTRUCTIONS

EVALS_DIR = pathlib.Path(__file__).resolve().parents[3] / "contracts" / "agents"


def evals_of(agent: str) -> dict:
    return json.loads((EVALS_DIR / f"{agent}.evals.json").read_text(encoding="utf-8"))


def test_every_evals_file_belongs_to_an_agent_with_instructions():
    files = {path.name.removesuffix(".evals.json") for path in EVALS_DIR.glob("*.evals.json")}
    assert files, "no agent evals found"
    assert files == set(INSTRUCTIONS)


@pytest.mark.parametrize("agent", sorted(INSTRUCTIONS))
def test_every_report_the_evals_accept_fits_the_agents_result_schema(agent):
    validator = Draft202012Validator(INSTRUCTIONS[agent].result_schema)
    accepted = [case for case in evals_of(agent)["cases"] if case["passes"]]
    assert accepted, f"{agent} evals accept no report"
    for case in accepted:
        errors = [error.message for error in validator.iter_errors(case["report"])]
        assert not errors, f"{case['name']}: {errors}"
