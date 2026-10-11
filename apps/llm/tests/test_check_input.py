import json
from datetime import date

import pytest

from skillhub_llm import check_input
from skillhub_llm.check_input import UnusableValueError, normalize

TODAY = date(2026, 10, 10)


@pytest.mark.parametrize(
    "raw,units,value",
    [
        ("85萬", {}, 850_000),
        ("1.5萬", {}, 15_000),
        ("三千五百", {}, 3_500),
        ("兩萬三千", {}, 23_000),
        ("十五", {}, 15),
        ("一百零五", {}, 105),
        ("1,234", {}, 1_234),
        ("25000公升", {"公升": 0.001, "度": 1}, 25),
        ("31度", {"公升": 0.001, "度": 1}, 31),
    ],
)
def test_a_number_given_in_words_units_or_separators_is_read_as_its_value(raw, units, value):
    assert normalize({"type": "number", "units": units}, raw, TODAY)[0] == value


def test_a_reading_that_changes_the_number_is_reported_and_a_plain_one_is_not():
    assert normalize({"type": "number"}, "85萬", TODAY)[1] == ["萬 = 10,000"]
    assert normalize({"type": "number"}, "850000", TODAY)[1] == []


@pytest.mark.parametrize(
    "raw,hours,readings",
    [
        ("3小時30分", 3.5, ["60 分鐘 = 1 小時"]),
        ("90分鐘", 1.5, ["60 分鐘 = 1 小時"]),
        ("2", 2, []),
    ],
)
def test_hours_and_minutes_are_read_as_hours(raw, hours, readings):
    assert normalize({"type": "hours"}, raw, TODAY) == (hours, readings)


@pytest.mark.parametrize(
    "raw,value",
    [
        ("後天", "2026-10-12"),
        ("前天", "2026-10-08"),
        ("3天後", "2026-10-13"),
        ("2026/2/28", "2026-02-28"),
        ("9月底", "2026-09-30"),
        ("2028年2月底", "2028-02-29"),
        ("10月初", "2026-10-01"),
        ("10/12", "2026-10-12"),
    ],
)
def test_relative_partial_and_full_dates_resolve_against_today(raw, value):
    assert normalize({"type": "date"}, raw, TODAY)[0] == value


@pytest.mark.parametrize("raw", ["2026-02-30", "10月32日", "13月底", "2/29"])
def test_a_date_that_does_not_exist_is_unusable(raw):
    with pytest.raises(UnusableValueError, match="不是存在的"):
        normalize({"type": "date"}, raw, TODAY)


@pytest.mark.parametrize(
    "field,raw,usable",
    [
        ({"type": "number", "min": 0}, "0", True),
        ({"type": "number", "min": 0}, "-1", False),
        ({"type": "hours", "max": 24}, "24", True),
        ({"type": "hours", "max": 24}, "24小時30分", False),
        ({"type": "number", "max": 120, "integer": True}, "120", True),
        ({"type": "number", "max": 120, "integer": True}, "121", False),
        ({"type": "number", "integer": True}, "2.5", False),
        ({"type": "date", "not_after_today": True}, "2026-10-10", True),
        ({"type": "date", "not_after_today": True}, "2026-10-11", False),
        ({"type": "date", "not_before_today": True}, "2026-10-10", True),
        ({"type": "date", "not_before_today": True}, "2026-10-09", False),
    ],
)
def test_each_bound_accepts_its_limit_and_refuses_one_past_it(field, raw, usable):
    if usable:
        normalize(field, raw, TODAY)
        return
    with pytest.raises(UnusableValueError):
        normalize(field, raw, TODAY)


def test_a_score_out_of_another_total_is_scaled_and_one_above_its_total_is_unusable():
    field = {"type": "number", "out_of": 100}
    assert normalize(field, "45/50", TODAY) == (90, ["45/50 換算成滿分 100"])
    assert normalize({"type": "number"}, "45/50", TODAY)[0] == 45
    with pytest.raises(UnusableValueError, match="分子大於分母"):
        normalize(field, "55/50", TODAY)


def test_an_unreadable_value_is_refused_by_type():
    with pytest.raises(ValueError):
        normalize({"type": "number"}, "很多", TODAY)


def test_a_record_keeps_good_values_and_names_the_rest():
    schema = [
        {"name": "開始", "type": "date"},
        {"name": "結束", "type": "date", "not_before": "開始"},
        {"name": "金額", "type": "number", "min": 0},
        {"name": "備註", "type": "text"},
    ]
    result = check_input.check_record(
        schema, {"開始": "2026-10-05", "結束": "2026-10-01", "金額": "85萬"}, TODAY
    )
    assert result["values"] == {"開始": "2026-10-05", "金額": 850_000}
    assert result["rejected"] == {"結束": "「2026-10-01」早於「開始」2026-10-05"}
    assert result["missing"] == ["備註"]
    assert result["assumptions"] == ["「金額」85萬 → 850000（萬 = 10,000）"]


def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False), encoding="utf-8")
    return str(path)


def test_a_run_writes_checked_values_and_prints_every_assumption_and_refusal(tmp_path, capsys):
    schema = write(
        tmp_path / "schema.json",
        {
            "fields": [
                {"name": "日期", "type": "date", "not_after_today": True},
                {"name": "時數", "type": "hours", "min": 0, "max": 24},
            ]
        },
    )
    given = write(
        tmp_path / "input.json",
        {
            "records": [
                {"日期": "前天", "時數": "90分鐘"},
                {"日期": "2026-10-09", "時數": "30"},
            ]
        },
    )
    out = tmp_path / "checked.json"

    code = check_input.main([given, "--schema", schema, "--out", str(out), "--today", "2026-10-10"])

    printed = capsys.readouterr().out.splitlines()
    assert code == 0
    assert printed == [
        "假設：第 1 筆「日期」前天 → 2026-10-08（以今天 2026-10-10 推算）",
        "假設：第 1 筆「時數」90分鐘 → 1.5（60 分鐘 = 1 小時）",
        "無法採用：第 2 筆「時數」「30」大於可能的最大值 24，不拿它計算",
        f"OK：1/2 筆可直接計算，結果在 {out}",
    ]
    checked = json.loads(out.read_text(encoding="utf-8"))
    assert checked["records"][0]["values"] == {"日期": "2026-10-08", "時數": 1.5}
    assert checked["records"][1]["values"] == {"日期": "2026-10-09"}


def test_without_a_stated_today_the_system_date_is_named_as_an_assumption(tmp_path, capsys):
    schema = write(tmp_path / "schema.json", {"fields": [{"name": "x", "type": "text"}]})
    given = write(tmp_path / "input.json", {"records": [{"x": "a"}]})

    check_input.main([given, "--schema", schema, "--out", str(tmp_path / "c.json")])

    assert capsys.readouterr().out.startswith(f"假設：今天以系統日期 {date.today()} 計")


@pytest.mark.parametrize(
    "schema,given,problem",
    [
        ({"fields": [{"name": "x", "type": "money"}]}, {"records": []}, "type must be one of"),
        ({"field": []}, {"records": []}, '"fields" list'),
        ({"fields": []}, {"rows": []}, 'input must be {"records"'),
    ],
)
def test_a_malformed_schema_or_input_stops_with_exit_2(tmp_path, capsys, schema, given, problem):
    code = check_input.main(
        [
            write(tmp_path / "input.json", given),
            "--schema",
            write(tmp_path / "schema.json", schema),
            "--out",
            str(tmp_path / "c.json"),
        ]
    )
    assert code == 2
    assert problem in capsys.readouterr().out
