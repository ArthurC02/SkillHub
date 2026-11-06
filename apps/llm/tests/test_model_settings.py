import json
import os
import subprocess
import sys

import pytest

SETTINGS = [
    ("MATCH_REASON_MODEL", "skillhub_llm.app", "MATCH_REASON_MODEL", "skillhub-match-reason"),
    (
        "SUGGEST_CRITERIA_MODEL",
        "skillhub_llm.app",
        "SUGGEST_CRITERIA_MODEL",
        "skillhub-suggest-criteria",
    ),
    (
        "SUGGEST_TEST_CASES_MODEL",
        "skillhub_llm.app",
        "SUGGEST_TEST_CASES_MODEL",
        "skillhub-suggest-test-cases",
    ),
    ("CREATION_MODEL", "skillhub_llm.creation", "MODEL", "skillhub-creation"),
    ("ENRICH_MODEL", "skillhub_llm.enrich", "ENRICH_MODEL", "skillhub-enrich"),
    ("JUDGE_MODEL", "skillhub_llm.evaluate", "JUDGE_MODEL", "skillhub-judge"),
    ("GENERATE_SKILL_MODEL", "skillhub_llm.generate", "GENERATE_SKILL_MODEL", "skillhub-generate"),
]

READ_EVERY_SETTING = (
    "import importlib, json, sys\n"
    "print(json.dumps({f'{m}.{a}': getattr(importlib.import_module(m), a)"
    " for _, m, a, _ in json.loads(sys.argv[1])}))"
)


def _models_when_every_variable_is(value: str) -> dict[str, str]:
    env = {**os.environ, **{variable: value for variable, _, _, _ in SETTINGS}}
    result = subprocess.run(
        [sys.executable, "-c", READ_EVERY_SETTING, json.dumps(SETTINGS)],
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    return json.loads(result.stdout)


@pytest.fixture(scope="module")
def models_from_a_blank_template() -> dict[str, str]:
    return _models_when_every_variable_is("")


@pytest.fixture(scope="module")
def models_from_a_chosen_value() -> dict[str, str]:
    return _models_when_every_variable_is("chosen-model")


@pytest.mark.parametrize(("variable", "module", "attribute", "default"), SETTINGS)
def test_a_blank_model_setting_from_the_template_means_the_default_and_a_value_overrides_it(
    variable: str,
    module: str,
    attribute: str,
    default: str,
    models_from_a_blank_template: dict[str, str],
    models_from_a_chosen_value: dict[str, str],
) -> None:
    assert models_from_a_blank_template[f"{module}.{attribute}"] == default
    assert models_from_a_chosen_value[f"{module}.{attribute}"] == "chosen-model"
