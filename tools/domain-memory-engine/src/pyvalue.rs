use crate::json::{Value, float_repr};
use crate::py;

const LINE_BREAKS: &[char] = &[
    '\n', '\r', '\u{b}', '\u{c}', '\u{1c}', '\u{1d}', '\u{1e}', '\u{85}', '\u{2028}', '\u{2029}',
];

pub fn splitlines(text: &str) -> Vec<&str> {
    let mut lines = Vec::new();
    let mut rest = text;
    while !rest.is_empty() {
        match rest.find(LINE_BREAKS) {
            None => {
                lines.push(rest);
                break;
            }
            Some(at) => {
                lines.push(&rest[..at]);
                let width = if rest[at..].starts_with("\r\n") {
                    2
                } else {
                    rest[at..].chars().next().map_or(1, char::len_utf8)
                };
                rest = &rest[at + width..];
            }
        }
    }
    lines
}

fn number(value: &Value) -> Option<f64> {
    match value {
        Value::Bool(flag) => Some(f64::from(u8::from(*flag))),
        Value::Int(digits) => digits.parse().ok(),
        Value::Float(number) => Some(*number),
        _ => None,
    }
}

pub fn equal(left: &Value, right: &Value) -> bool {
    match (left, right) {
        (Value::Int(one), Value::Int(other)) => one == other,
        (Value::Str(one), Value::Str(other)) => one == other,
        (Value::Null, Value::Null) => true,
        (Value::Array(one), Value::Array(other)) => {
            one.len() == other.len() && one.iter().zip(other).all(|(a, b)| equal(a, b))
        }
        (Value::Object(one), Value::Object(other)) => {
            let keys = one.iter().count();
            keys == other.iter().count()
                && one
                    .iter()
                    .all(|(key, value)| other.get(key).is_some_and(|found| equal(value, found)))
        }
        _ => match (number(left), number(right)) {
            (Some(one), Some(other)) => one == other,
            _ => false,
        },
    }
}

pub fn str_of(value: &Value) -> String {
    match value {
        Value::Str(text) => text.clone(),
        other => repr_of(other),
    }
}

pub fn repr_of(value: &Value) -> String {
    match value {
        Value::Null => "None".to_string(),
        Value::Bool(true) => "True".to_string(),
        Value::Bool(false) => "False".to_string(),
        Value::Int(digits) => digits.clone(),
        Value::Float(number) => float_repr(*number)
            .replace("NaN", "nan")
            .replace("Infinity", "inf"),
        Value::Str(text) => py::repr(text),
        Value::Array(items) => format!(
            "[{}]",
            items.iter().map(repr_of).collect::<Vec<_>>().join(", ")
        ),
        Value::Object(object) => format!(
            "{{{}}}",
            object
                .iter()
                .map(|(key, value)| format!("{}: {}", py::repr(key), repr_of(value)))
                .collect::<Vec<_>>()
                .join(", ")
        ),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn splitlines_breaks_where_python_does_and_drops_the_last_empty_line() {
        assert_eq!(splitlines("a\r\nb\u{85}c\u{1e}\n"), vec!["a", "b", "c", ""]);
        assert_eq!(splitlines("a\n"), vec!["a"]);
        assert!(splitlines("").is_empty());
    }

    #[test]
    fn numbers_compare_across_python_types() {
        assert!(equal(&Value::Int("1".into()), &Value::Float(1.0)));
        assert!(equal(&Value::Bool(true), &Value::Int("1".into())));
        assert!(!equal(&Value::Str("1".into()), &Value::Int("1".into())));
    }
}
