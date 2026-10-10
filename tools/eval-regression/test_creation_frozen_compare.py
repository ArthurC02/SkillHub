"""Pure-function tests for creation_frozen_compare.py; no model, DB or network."""

import json
import pathlib
import sys
import tempfile

sys.path.insert(0, str(pathlib.Path(__file__).parent))

import creation_frozen_compare as cfc  # noqa: E402


def test_wilson_interval_matches_known_values():
    low, high = cfc.wilson(20, 30)
    assert (round(low, 3), round(high, 3)) == (0.488, 0.808)
    assert cfc.wilson(0, 0) == (0.0, 0.0)
    assert cfc.wilson(30, 30)[1] == 1.0


def test_a_number_is_found_written_plain_or_grouped_but_not_inside_another_number():
    assert cfc.mentions("應退 31,650 元", "31650")
    assert cfc.mentions("應退31650元", "31650")
    assert not cfc.mentions("應退 131650 元", "31650")
    assert not cfc.mentions("應退 31650.5 元", "31650")
    assert not cfc.mentions("應退 316 元", "31650")


def test_a_date_is_found_in_its_common_written_forms():
    for written in ["2026-12-01", "2026/12/01", "2026年12月1日", "12月1日", "12/1"]:
        assert cfc.mentions(f"最晚 {written} 前通知", "2026-12-01"), written
    assert not cfc.mentions("最晚 12月11日 前通知", "2026-12-01")


def test_a_duration_is_found_as_a_clock_or_in_hours_and_minutes():
    assert cfc.mentions("共 41:45", "41:45")
    assert cfc.mentions("共 41 小時 45 分", "41:45")
    assert not cfc.mentions("共 41 小時", "41:45")
    assert cfc.mentions("共 40 小時", "40:00")


def write_record(run_dir, task, n, overall, output):
    record = {"final_output": output, "evaluation": {"overall": overall}}
    (run_dir / f"{task}-holdout-{n}-trial-r0.json").write_text(json.dumps(record), "utf-8")


def test_a_case_outcome_reads_the_judge_and_checks_every_expected_number():
    with tempfile.TemporaryDirectory() as tmp:
        run = pathlib.Path(tmp)
        write_record(run, "FM01", 1, "met", "應退 31,650 元")
        write_record(run, "FM01", 2, "partially_met", "應退 17000 元")
        both = {"task": "FM01", "index": 0, "expected": ["31650", "9000"]}
        one = {"task": "FM01", "index": 1, "expected": ["17000"]}
        none = {"task": "FM01", "index": 1, "expected": []}
        missing = {"task": "FM01", "index": 2, "expected": ["1"]}

        assert cfc.case_outcome(run, both) == {"ran": True, "judge": True, "code": False}
        assert cfc.case_outcome(run, one) == {"ran": True, "judge": False, "code": True}
        assert cfc.case_outcome(run, none)["code"] is None
        assert cfc.case_outcome(run, missing) == {"ran": False, "judge": False, "code": None}


def test_the_paired_difference_counts_discordant_cases_and_brackets_the_estimate():
    cases = [{"task": f"T{i // 3}", "index": i % 3} for i in range(30)]
    base = [i % 2 == 0 for i in range(30)]
    cand = [True] * 30

    paired = cfc.paired_bootstrap(cases, base, cand, reps=2000)

    assert paired["difference"] == 0.5
    assert paired["only_candidate_passed"] == 15
    assert paired["only_baseline_passed"] == 0
    assert paired["low"] <= 0.5 <= paired["high"]
    assert paired["low"] > 0


def test_identical_runs_have_no_difference():
    cases = [{"task": f"T{i // 3}", "index": i % 3} for i in range(12)]
    same = [i % 3 == 0 for i in range(12)]

    paired = cfc.paired_bootstrap(cases, same, same, reps=500)

    assert (paired["difference"], paired["low"], paired["high"]) == (0.0, 0.0, 0.0)


if __name__ == "__main__":
    for name, test in list(globals().items()):
        if name.startswith("test_"):
            test()
    print("ok")
