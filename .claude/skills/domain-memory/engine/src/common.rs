use crate::json::Value;
use crate::proposals::completed_identifier;
use crate::pyvalue::repr_of;

pub const REQUIREMENT_FILE: &str = "requirement-normalization.json";
pub const OBLIGATIONS_FILE: &str = "test-obligations.json";
pub const EVIDENCE_FILE: &str = "evidence-bundle.json";
pub const CHANGE_PACKAGE_FILES: [&str; 5] = [
    REQUIREMENT_FILE,
    crate::files::PROPOSAL_FILE,
    OBLIGATIONS_FILE,
    EVIDENCE_FILE,
    "draft-pr.md",
];
pub const CHANGE_PACKAGE_FORMATS: [(&str, &str); 4] = [
    (REQUIREMENT_FILE, "domain-requirement/v1"),
    (crate::files::PROPOSAL_FILE, "domain-change-proposal/v1"),
    (OBLIGATIONS_FILE, "domain-test-obligations/v1"),
    (EVIDENCE_FILE, "domain-evidence-bundle/v1"),
];
pub const CHANGE_CLASSIFICATIONS: [&str; 2] = ["material", "routine"];
pub const RISK_FLAGS: [&str; 6] = [
    "cross_context",
    "financial_decision",
    "irreversible_change",
    "public_contract",
    "regulated_rule",
    "sensitive_data",
];
pub const PROPOSAL_STATUSES: [&str; 7] = [
    "applied",
    "approved",
    "draft",
    "rejected",
    "submitted",
    "superseded",
    "verified",
];
pub const OBLIGATION_SOURCE_TYPES: [&str; 4] =
    ["acceptance-criterion", "contract", "invariant", "rule"];
pub const OBLIGATION_STATUSES: [&str; 3] = ["blocked", "executed", "planned"];

pub fn invalid_choice(label: &str, field: &str, value: &Value, allowed: &[&str]) -> String {
    format!(
        "{label} has an invalid {field}: {} is not one of {}",
        repr_of(value),
        allowed.join(", ")
    )
}

pub fn is_one_of(value: Option<&Value>, allowed: &[&str]) -> bool {
    value
        .and_then(Value::as_str)
        .is_some_and(|text| allowed.contains(&text))
}

pub fn whole_number(value: Option<&Value>) -> Option<&str> {
    match value {
        Some(Value::Int(digits)) => Some(digits),
        _ => None,
    }
}

pub fn positive_whole_number(value: Option<&Value>) -> bool {
    whole_number(value).is_some_and(|digits| !digits.starts_with('-') && digits != "0")
}

pub fn iso_timestamp(value: &Value) -> bool {
    completed_identifier(value)
        && value
            .as_str()
            .is_some_and(|text| fromisoformat(&text.replace('Z', "+00:00")))
}

fn byte_at(bytes: &[u8], index: usize) -> u8 {
    bytes.get(index).copied().unwrap_or(0)
}

fn digits(bytes: &[u8], from: usize, count: usize) -> Option<(i64, usize)> {
    let mut value = 0;
    for offset in 0..count {
        let digit = byte_at(bytes, from + offset).wrapping_sub(b'0');
        if digit > 9 {
            return None;
        }
        value = value * 10 + i64::from(digit);
    }
    Some((value, from + count))
}

fn is_leap(year: i64) -> bool {
    year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
}

fn days_in_month(year: i64, month: i64) -> i64 {
    match month {
        2 if is_leap(year) => 29,
        2 => 28,
        4 | 6 | 9 | 11 => 30,
        _ => 31,
    }
}

fn days_before_year(year: i64) -> i64 {
    let previous = year - 1;
    previous * 365 + previous / 4 - previous / 100 + previous / 400
}

fn ordinal_of_new_year(year: i64) -> i64 {
    days_before_year(year) + 1
}

fn week_one_monday(year: i64) -> i64 {
    let first_day = ordinal_of_new_year(year);
    let first_weekday = (first_day + 6).rem_euclid(7);
    let monday = first_day - first_weekday;
    if first_weekday > 3 {
        monday + 7
    } else {
        monday
    }
}

fn iso_week_date(year: i64, week: i64, day: i64) -> Option<(i64, i64, i64)> {
    if year < 1 {
        return None;
    }
    if week <= 0 || week >= 53 {
        let first_weekday = (ordinal_of_new_year(year) + 6).rem_euclid(7);
        let long_year = first_weekday == 3 || (first_weekday == 2 && is_leap(year));
        if !(week == 53 && long_year) {
            return None;
        }
    }
    if !(1..=7).contains(&day) {
        return None;
    }
    let mut ordinal = week_one_monday(year) + (week - 1) * 7 + day - 1;
    if ordinal < 1 {
        return None;
    }
    ordinal -= 1;
    let (cycles, mut rest) = (ordinal / 146_097, ordinal % 146_097);
    let mut year = cycles * 400 + 1;
    let centuries = (rest / 36_524).min(3);
    rest -= centuries * 36_524;
    year += centuries * 100;
    let quads = rest / 1461;
    rest %= 1461;
    year += quads * 4;
    let single = (rest / 365).min(3);
    rest -= single * 365;
    year += single;
    let mut month = 1;
    while month < 12 && rest >= days_in_month(year, month) {
        rest -= days_in_month(year, month);
        month += 1;
    }
    Some((year, month, rest + 1))
}

fn parse_date(bytes: &[u8], length: usize) -> Option<(i64, i64, i64)> {
    let (year, mut at) = digits(bytes, 0, 4)?;
    let separated = byte_at(bytes, at) == b'-';
    if separated {
        at += 1;
    }
    if byte_at(bytes, at) == b'W' {
        at += 1;
        let (week, next) = digits(bytes, at, 2)?;
        at = next;
        let day = if at < length {
            if separated {
                let dash = byte_at(bytes, at);
                at += 1;
                if dash != b'-' {
                    return None;
                }
            }
            digits(bytes, at, 1)?.0
        } else {
            1
        };
        return iso_week_date(year, week, day);
    }
    let (month, next) = digits(bytes, at, 2)?;
    at = next;
    if separated {
        let dash = byte_at(bytes, at);
        at += 1;
        if dash != b'-' {
            return None;
        }
    }
    let (day, _) = digits(bytes, at, 2)?;
    Some((year, month, day))
}

fn date_time_separator(bytes: &[u8]) -> Option<usize> {
    let length = bytes.len();
    if length == 7 {
        return Some(7);
    }
    let is_digit = |index: usize| byte_at(bytes, index).is_ascii_digit();
    if byte_at(bytes, 4) == b'-' {
        if byte_at(bytes, 5) != b'W' {
            return Some(10);
        }
        if length < 8 {
            return None;
        }
        if length > 8 && byte_at(bytes, 8) == b'-' {
            if length == 9 {
                return None;
            }
            if length > 10 && is_digit(10) {
                return Some(8);
            }
            return Some(10);
        }
        return Some(8);
    }
    if byte_at(bytes, 4) == b'W' {
        let mut index = 7;
        while index < length && is_digit(index) {
            index += 1;
        }
        if index < 9 {
            return Some(index);
        }
        return Some(if index % 2 == 0 { 7 } else { 8 });
    }
    Some(8)
}

struct Clock {
    hour: i64,
    minute: i64,
    second: i64,
    microsecond: i64,
}

enum Parsed {
    Complete,
    Trailing,
}

fn parse_clock(bytes: &[u8], from: usize, end: usize) -> Option<(Clock, Parsed)> {
    let mut parts = [0i64; 3];
    let mut at = from;
    let mut separated = true;
    for index in 0..3 {
        let (value, next) = digits(bytes, at, 2)?;
        parts[index] = value;
        at = next;
        let after = byte_at(bytes, at);
        at += 1;
        if index == 0 {
            separated = after == b':';
        }
        if after == b'.' || after == b',' {
            break;
        } else if at >= end {
            let parsed = if after == 0 {
                Parsed::Complete
            } else {
                Parsed::Trailing
            };
            return Some((clock_of(parts, 0), parsed));
        } else if separated && after == b':' {
            continue;
        } else if !separated {
            at -= 1;
        } else {
            return None;
        }
    }
    let to_parse = end.saturating_sub(at).min(6);
    if to_parse == 0 {
        return None;
    }
    let (mut microsecond, mut next) = digits(bytes, at, to_parse)?;
    for _ in to_parse..6 {
        microsecond *= 10;
    }
    while byte_at(bytes, next).is_ascii_digit() {
        next += 1;
    }
    let parsed = if byte_at(bytes, next) == 0 {
        Parsed::Complete
    } else {
        Parsed::Trailing
    };
    Some((clock_of(parts, microsecond), parsed))
}

fn clock_of(parts: [i64; 3], microsecond: i64) -> Clock {
    Clock {
        hour: parts[0],
        minute: parts[1],
        second: parts[2],
        microsecond,
    }
}

fn parse_time(bytes: &[u8], from: usize) -> Option<Clock> {
    let end = bytes.len();
    let mut zone_at = from;
    while zone_at < end && !matches!(byte_at(bytes, zone_at), b'Z' | b'+' | b'-') {
        zone_at += 1;
    }
    let (clock, parsed) = parse_clock(bytes, from, zone_at)?;
    if zone_at == end {
        return matches!(parsed, Parsed::Complete).then_some(clock);
    }
    let sign = if byte_at(bytes, zone_at) == b'-' {
        -1
    } else {
        1
    };
    let (zone, zone_parsed) = parse_clock(bytes, zone_at + 1, end)?;
    if matches!(zone_parsed, Parsed::Trailing) {
        return None;
    }
    let microseconds = sign
        * (((zone.hour * 3600 + zone.minute * 60 + zone.second) * 1_000_000) + zone.microsecond);
    (microseconds.abs() < 86_400_000_000).then_some(clock)
}

fn fromisoformat(text: &str) -> bool {
    if text.chars().count() < 7 {
        return false;
    }
    let bytes = text.as_bytes();
    let Some(separator) = date_time_separator(bytes) else {
        return false;
    };
    let Some((year, month, day)) = parse_date(bytes, separator) else {
        return false;
    };
    let valid_date = (1..=9999).contains(&year)
        && (1..=12).contains(&month)
        && (1..=days_in_month(year, month)).contains(&day);
    if !valid_date {
        return false;
    }
    if bytes.len() <= separator {
        return true;
    }
    let mut at = separator;
    let lead = bytes[at];
    at += if lead & 0x80 == 0 {
        1
    } else if lead & 0xf0 == 0xe0 {
        3
    } else if lead & 0xf0 == 0xf0 {
        4
    } else {
        2
    };
    let Some(clock) = parse_time(bytes, at) else {
        return false;
    };
    clock.hour <= 23 && clock.minute <= 59 && clock.second <= 59 && clock.microsecond <= 999_999
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn iso_timestamps_follow_the_standard_library_parser() {
        let cases = include_str!("iso_timestamp_golden.txt");
        for line in cases.lines() {
            let (verdict, text) = line.split_once('\t').unwrap();
            assert_eq!(
                fromisoformat(text),
                verdict == "1",
                "{text:?} is {}",
                if verdict == "1" { "valid" } else { "invalid" }
            );
        }
    }
}
