"""Normalize and validate the values a person gave against a Skill's input schema."""

from __future__ import annotations

import argparse
import calendar
import json
import re
import sys
from datetime import date, timedelta

FIELD_TYPES = ("date", "number", "hours", "text")
MONTHS = 12
CHINESE_DIGITS = {
    "零": 0,
    "〇": 0,
    "一": 1,
    "二": 2,
    "兩": 2,
    "两": 2,
    "三": 3,
    "四": 4,
    "五": 5,
    "六": 6,
    "七": 7,
    "八": 8,
    "九": 9,
}
CHINESE_UNITS = {"十": 10, "百": 100, "千": 1000}
CHINESE_SECTIONS = {"萬": 10_000, "万": 10_000, "億": 100_000_000, "亿": 100_000_000}
ARABIC_WITH_SECTION = re.compile(r"^(-?\d+(?:\.\d+)?)\s*([萬万億亿千百])$")
FRACTION = re.compile(r"^(-?\d+(?:\.\d+)?)\s*/\s*(\d+(?:\.\d+)?)$")
PLAIN_NUMBER = re.compile(r"^-?\d+(?:\.\d+)?$")
HOURS_AND_MINUTES = re.compile(
    r"^(?:(-?\d+(?:\.\d+)?)\s*(?:小時|小时|鐘頭|h|hr|hrs|hours?))?\s*"
    r"(?:(-?\d+(?:\.\d+)?)\s*(?:分鐘|分钟|分|m|min|mins|minutes?))?$",
    re.IGNORECASE,
)
FULL_DATE = re.compile(r"^(\d{4})\s*[-/.年]\s*(\d{1,2})\s*[-/.月]\s*(\d{1,2})\s*日?$")
MONTH_DAY = re.compile(r"^(\d{1,2})\s*[/月]\s*(\d{1,2})\s*日?$")
MONTH_EDGE = re.compile(r"^(?:(\d{4})\s*年\s*)?(\d{1,2})\s*月\s*(底|初)$")
RELATIVE_DAYS = {
    "大前天": -3,
    "前天": -2,
    "昨天": -1,
    "今天": 0,
    "明天": 1,
    "後天": 2,
    "后天": 2,
    "大後天": 3,
    "大后天": 3,
}
DAYS_FROM_NOW = re.compile(r"^(\d+)\s*天(後|后|前)$")


class UnusableValueError(ValueError):
    """A value that cannot be right; its message names why."""


def chinese_number(text: str) -> float:
    total, section, digit = 0, 0, None
    for ch in text:
        if ch in CHINESE_DIGITS:
            digit = CHINESE_DIGITS[ch]
        elif ch in CHINESE_UNITS:
            section += (1 if digit is None else digit) * CHINESE_UNITS[ch]
            digit = None
        elif ch in CHINESE_SECTIONS:
            total += (section + (digit or 0)) * CHINESE_SECTIONS[ch]
            section, digit = 0, None
        else:
            raise ValueError(text)
    return total + section + (digit or 0)


def parse_number(raw: str, units: dict[str, float]) -> tuple[float, list[str]]:
    text = raw.strip().replace(",", "").replace("，", "")
    readings: list[str] = []
    for unit, factor in sorted(units.items(), key=lambda item: -len(item[0])):
        if unit and text.endswith(unit):
            value, inner = parse_number(text[: -len(unit)], {})
            if factor != 1:
                readings.append(f"{unit} × {factor:g}")
            return value * factor, inner + readings
    if PLAIN_NUMBER.match(text):
        return float(text), readings
    if match := ARABIC_WITH_SECTION.match(text):
        unit = match.group(2)
        factor = CHINESE_SECTIONS.get(unit) or CHINESE_UNITS[unit]
        return float(match.group(1)) * factor, [f"{unit} = {factor:,}"]
    if text and all(
        ch in CHINESE_DIGITS or ch in CHINESE_UNITS or ch in CHINESE_SECTIONS for ch in text
    ):
        return float(chinese_number(text)), ["中文數字"]
    raise ValueError(raw)


def parse_hours(raw: str) -> tuple[float, list[str]]:
    text = raw.strip()
    if PLAIN_NUMBER.match(text):
        return float(text), []
    match = HOURS_AND_MINUTES.match(text)
    if not match or not (match.group(1) or match.group(2)):
        raise ValueError(raw)
    hours = float(match.group(1) or 0)
    minutes = float(match.group(2) or 0)
    readings = ["60 分鐘 = 1 小時"] if match.group(2) else []
    return hours + minutes / 60, readings


def real_date(year: int, month: int, day: int, raw: str) -> date:
    try:
        return date(year, month, day)
    except ValueError:
        raise UnusableValueError(f"「{raw}」不是存在的日期") from None


def parse_date(raw: str, today: date) -> tuple[date, list[str]]:
    text = raw.strip()
    if text in RELATIVE_DAYS:
        return today + timedelta(days=RELATIVE_DAYS[text]), [f"以今天 {today} 推算"]
    if match := DAYS_FROM_NOW.match(text):
        days = int(match.group(1)) * (1 if match.group(2) in "後后" else -1)
        return today + timedelta(days=days), [f"以今天 {today} 推算"]
    if match := FULL_DATE.match(text):
        year, month, day = (int(g) for g in match.groups())
        return real_date(year, month, day, raw), []
    if match := MONTH_DAY.match(text):
        month, day = int(match.group(1)), int(match.group(2))
        return real_date(today.year, month, day, raw), [f"未寫年份，以 {today.year} 年計"]
    if match := MONTH_EDGE.match(text):
        year = int(match.group(1)) if match.group(1) else today.year
        month = int(match.group(2))
        if not 1 <= month <= MONTHS:
            raise UnusableValueError(f"「{raw}」不是存在的月份")
        last = calendar.monthrange(year, month)[1]
        day = last if match.group(3) == "底" else 1
        readings = [f"「{match.group(3)}」讀成 {month} 月 {day} 日"]
        if not match.group(1):
            readings.append(f"未寫年份，以 {year} 年計")
        return date(year, month, day), readings
    raise ValueError(raw)


def within_bounds(value: float, field: dict, raw: str) -> None:
    low, high = field.get("min"), field.get("max")
    if low is not None and value < low:
        raise UnusableValueError(f"「{raw}」小於可能的最小值 {low:g}")
    if high is not None and value > high:
        raise UnusableValueError(f"「{raw}」大於可能的最大值 {high:g}")


def scaled(value: float, out_of: float, field: dict, raw: str) -> tuple[float, list[str]]:
    scale = field.get("out_of")
    if out_of <= 0 or value > out_of:
        raise UnusableValueError(f"「{raw}」的分子大於分母")
    if scale is None or scale == out_of:
        return value, []
    return value / out_of * scale, [f"{value:g}/{out_of:g} 換算成滿分 {scale:g}"]


def normalize(field: dict, raw: object, today: date) -> tuple[object, list[str]]:
    kind = field["type"]
    if kind == "text":
        return str(raw), []
    text = str(raw)
    if kind == "date":
        value, readings = parse_date(text, today)
        if field.get("not_after_today") and value > today:
            raise UnusableValueError(f"「{text}」晚於今天 {today}")
        if field.get("not_before_today") and value < today:
            raise UnusableValueError(f"「{text}」早於今天 {today}")
        return value.isoformat(), readings
    if kind == "hours":
        number, readings = parse_hours(text)
    elif match := FRACTION.match(text.replace(",", "")):
        number, readings = scaled(float(match.group(1)), float(match.group(2)), field, text)
    else:
        number, readings = parse_number(text, field.get("units") or {})
    within_bounds(number, field, text)
    if field.get("integer") and number != int(number):
        raise UnusableValueError(f"「{text}」應為整數")
    return (int(number) if number == int(number) else round(number, 6)), readings


def check_order(field: dict, values: dict, rejected: dict) -> None:
    earlier = field.get("not_before")
    name = field["name"]
    if not earlier or name not in values or earlier not in values:
        return
    if values[name] < values[earlier]:
        rejected[name] = f"「{values[name]}」早於「{earlier}」{values[earlier]}"
        del values[name]


def check_record(schema: list[dict], record: dict, today: date) -> dict:
    values: dict = {}
    rejected: dict = {}
    assumptions: list[str] = []
    missing: list[str] = []
    for field in schema:
        name = field["name"]
        raw = record.get(name)
        if raw is None or str(raw).strip() == "":
            missing.append(name)
            continue
        try:
            values[name], readings = normalize(field, raw, today)
        except UnusableValueError as exc:
            rejected[name] = str(exc)
            continue
        except ValueError:
            rejected[name] = f"「{raw}」讀不出{field['type']}值"
            continue
        assumptions += [f"「{name}」{raw} → {values[name]}（{r}）" for r in readings]
    for field in schema:
        check_order(field, values, rejected)
    return {"values": values, "rejected": rejected, "missing": missing, "assumptions": assumptions}


def schema_problems(schema: object) -> list[str]:
    if not isinstance(schema, dict) or not isinstance(schema.get("fields"), list):
        return ['schema must be an object with a "fields" list']
    problems = []
    for i, field in enumerate(schema["fields"]):
        if not isinstance(field, dict) or not isinstance(field.get("name"), str):
            problems.append(f"field {i}: needs a string name")
        elif field.get("type") not in FIELD_TYPES:
            problems.append(f"field {field['name']}: type must be one of {', '.join(FIELD_TYPES)}")
    return problems


def report_lines(results: list[dict]) -> list[str]:
    lines = []
    for n, result in enumerate(results, start=1):
        prefix = f"第 {n} 筆" if len(results) > 1 else ""
        lines += [f"假設：{prefix}{a}" for a in result["assumptions"]]
        lines += [
            f"無法採用：{prefix}「{k}」{v}，不拿它計算" for k, v in result["rejected"].items()
        ]
        lines += [f"未提供：{prefix}「{k}」" for k in result["missing"]]
    return lines


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Normalize and validate input values against scripts/input_schema.json. "
            "Writes checked.json; every later step reads only that file."
        )
    )
    parser.add_argument("input", help='JSON file: {"records": [{field: value as given}, ...]}')
    parser.add_argument("--schema", default="scripts/input_schema.json")
    parser.add_argument("--out", default="checked.json")
    parser.add_argument("--today", help="YYYY-MM-DD when the message states today's date")
    return parser


def _load(path: str) -> object:
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        schema, given = _load(args.schema), _load(args.input)
        today = date.fromisoformat(args.today) if args.today else date.today()
    except (OSError, ValueError) as exc:
        print(f"cannot read input: {exc}")
        return 2
    problems = schema_problems(schema)
    records = given.get("records") if isinstance(given, dict) else None
    if not isinstance(records, list) or not all(isinstance(r, dict) for r in records):
        problems.append('input must be {"records": [ {field: value}, ... ]}')
    if problems:
        print("\n".join(problems))
        return 2
    results = [check_record(schema["fields"], record, today) for record in records]
    with open(args.out, "w", encoding="utf-8") as handle:
        json.dump(
            {"today": today.isoformat(), "records": results}, handle, ensure_ascii=False, indent=2
        )
    if not args.today:
        print(f"假設：今天以系統日期 {today} 計")
    for line in report_lines(results):
        print(line)
    usable = sum(1 for r in results if not r["rejected"])
    print(f"OK：{usable}/{len(results)} 筆可直接計算，結果在 {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
