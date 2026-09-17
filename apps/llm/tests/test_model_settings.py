import os
import subprocess
import sys

import pytest


def _model_when(variable: str, value: str, module: str, attribute: str) -> str:
    env = {**os.environ, variable: value}
    result = subprocess.run(
        [sys.executable, "-c", f"import {module} as m; print(m.{attribute})"],
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    return result.stdout.strip()


@pytest.mark.parametrize(
    ("variable", "module", "attribute", "default"),
    [
        ("MATCH_REASON_MODEL", "skillhub_llm.app", "MATCH_REASON_MODEL", "gpt-5.6-luna"),
        ("SUGGEST_CRITERIA_MODEL", "skillhub_llm.app", "SUGGEST_CRITERIA_MODEL", "gpt-5.4-mini"),
        ("CREATION_MODEL", "skillhub_llm.creation", "MODEL", "gpt-5.4-mini"),
        ("ENRICH_MODEL", "skillhub_llm.enrich", "ENRICH_MODEL", "gpt-5.6-sol"),
        ("JUDGE_MODEL", "skillhub_llm.evaluate", "JUDGE_MODEL", "gpt-5.6-terra"),
        ("GENERATE_SKILL_MODEL", "skillhub_llm.generate", "GENERATE_SKILL_MODEL", "gpt-5.4-mini"),
    ],
)
def test_a_blank_model_setting_from_the_template_means_the_default_and_a_value_overrides_it(
    variable: str, module: str, attribute: str, default: str
) -> None:
    assert _model_when(variable, "", module, attribute) == default
    assert _model_when(variable, "chosen-model", module, attribute) == "chosen-model"
