"""Pure-function tests for import_test_cases.py; no platform, no network."""

import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).parent))

import import_test_cases as itc  # noqa: E402

ASKS_THEN_JUDGE = {"kind": "sequence", "children": [{"kind": "asks"}, {"kind": "judge"}]}


def numbers_then_judge(*values):
    return {"kind": "sequence", "children": [{"kind": "contains_values", "values": list(values)}, {"kind": "judge"}]}


def test_an_ask_case_carries_the_persons_facts_and_checks_the_question_first():
    cases = itc.test_cases_of(itc.find_task("SC01"))
    ask = next(c for c in cases if c["name"] == "SC01 沒給度數")

    assert ask["user_prompt"] == "幫我算這期水費。"
    assert ask["simulated_user_facts"] == ["本期用水 45 度"]
    assert [c.get("check") for c in ask["criteria"]] == [ASKS_THEN_JUDGE, numbers_then_judge("610")]


def test_a_compute_case_checks_its_number_only_on_the_criterion_that_names_it():
    compute = itc.test_cases_of(itc.find_task("SC01"))[0]

    assert compute["simulated_user_facts"] == []
    assert compute["criteria"] == [
        {"text": "回覆的應繳水費為 370 元", "check": numbers_then_judge("370")},
        {"text": "30 度整落在第二級距內，沒有把任何用水量按第三級距費率計"},
    ]


def test_a_criterion_naming_only_some_expected_numbers_is_left_to_the_judge():
    case = {"criteria": ["總價 120 元"], "expected_numbers": ["120", "30"], "enrichment": "compute"}

    assert itc.criteria_of(case) == [{"text": "總價 120 元"}]


def test_a_name_over_the_platform_limit_is_cut_to_fit_without_breaking_a_character():
    task = {"id": "SC99", "holdout": [{"name": "長" * 80, "prompt": "p", "criteria": []}]}

    name = itc.test_cases_of(task)[0]["name"]

    assert len(name.encode()) <= itc.MAX_NAME_BYTES < len(name.encode()) + len("長".encode())
    assert name.startswith("SC99 長")


def test_every_task_in_the_set_imports_within_the_platform_limits():
    for task in itc.json.loads(itc.TASKS.read_text(encoding="utf-8"))["reference"]:
        for case in itc.test_cases_of(task):
            facts = case["simulated_user_facts"]
            assert len(facts) <= 10 and sum(len(f.encode()) for f in facts) <= 1500, case["name"]
            assert case["criteria"], case["name"]


if __name__ == "__main__":
    for name, test in list(globals().items()):
        if name.startswith("test_"):
            test()
    print("ok")
