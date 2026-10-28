use std::fmt::Write as _;

#[derive(Clone, Debug, PartialEq)]
pub enum Value {
    Null,
    Bool(bool),
    Int(String),
    Float(f64),
    Str(String),
    Array(Vec<Value>),
    Object(Object),
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct Object(Vec<(String, Value)>);

impl Object {
    pub fn new() -> Self {
        Self(Vec::new())
    }

    pub fn get(&self, key: &str) -> Option<&Value> {
        self.0
            .iter()
            .find(|(name, _)| name == key)
            .map(|(_, value)| value)
    }

    pub fn insert(&mut self, key: impl Into<String>, value: Value) {
        let key = key.into();
        match self.0.iter_mut().find(|(name, _)| *name == key) {
            Some(slot) => slot.1 = value,
            None => self.0.push((key, value)),
        }
    }

    pub fn get_mut(&mut self, key: &str) -> Option<&mut Value> {
        self.0
            .iter_mut()
            .find(|(name, _)| name == key)
            .map(|(_, value)| value)
    }

    pub fn iter(&self) -> impl Iterator<Item = &(String, Value)> {
        self.0.iter()
    }
}

impl Value {
    pub fn as_str(&self) -> Option<&str> {
        match self {
            Value::Str(text) => Some(text),
            _ => None,
        }
    }

    pub fn as_object(&self) -> Option<&Object> {
        match self {
            Value::Object(object) => Some(object),
            _ => None,
        }
    }

    pub fn as_array(&self) -> Option<&[Value]> {
        match self {
            Value::Array(items) => Some(items),
            _ => None,
        }
    }

    pub fn int(number: usize) -> Value {
        Value::Int(number.to_string())
    }
}

const MAX_DEPTH: usize = 980;

pub fn parse(text: &str) -> Result<Value, ()> {
    parsed(text, false)
}

pub fn parse_keeping_last_duplicate(text: &str) -> Result<Value, ()> {
    parsed(text, true)
}

pub enum Fault {
    Malformed,
    DuplicateKey(String),
}

pub fn parse_naming_duplicate(text: &str) -> Result<Value, Fault> {
    let mut parser = Parser::over(text, false);
    parser.document().map_err(|()| {
        parser
            .duplicate
            .take()
            .map_or(Fault::Malformed, Fault::DuplicateKey)
    })
}

fn parsed(text: &str, last_duplicate_wins: bool) -> Result<Value, ()> {
    Parser::over(text, last_duplicate_wins).document()
}

pub fn loads(text: &str) -> Result<Value, String> {
    if text.starts_with('\u{feff}') {
        return Err(decode_error(
            text,
            "Unexpected UTF-8 BOM (decode using utf-8-sig)",
            0,
        ));
    }
    let mut parser = Parser::over(text, true);
    parser.document().map_err(|()| {
        let (message, at) = parser.fault.unwrap_or(("Expecting value", parser.at));
        decode_error(text, message, at)
    })
}

fn decode_error(text: &str, message: &str, at: usize) -> String {
    let before = &text[..at];
    let position = before.chars().count();
    let line = before.matches('\n').count() + 1;
    let column = before.rfind('\n').map_or(position + 1, |newline| {
        before[newline + 1..].chars().count() + 1
    });
    format!("{message}: line {line} column {column} (char {position})")
}

pub fn number_prefix(text: &str) -> Option<usize> {
    let mut parser = Parser {
        bytes: text.as_bytes(),
        text,
        at: 0,
        last_duplicate_wins: false,
        duplicate: None,
        fault: None,
    };
    parser.scan_number().ok()?;
    Some(parser.at)
}

struct Parser<'a> {
    bytes: &'a [u8],
    text: &'a str,
    at: usize,
    last_duplicate_wins: bool,
    duplicate: Option<String>,
    fault: Option<(&'static str, usize)>,
}

impl<'a> Parser<'a> {
    fn over(text: &'a str, last_duplicate_wins: bool) -> Self {
        Self {
            bytes: text.as_bytes(),
            text,
            at: 0,
            last_duplicate_wins,
            duplicate: None,
            fault: None,
        }
    }

    fn fail<T>(&mut self, message: &'static str, at: usize) -> Result<T, ()> {
        self.fault.get_or_insert((message, at));
        Err(())
    }

    fn document(&mut self) -> Result<Value, ()> {
        self.skip_whitespace();
        let value = self.value(0)?;
        self.skip_whitespace();
        if self.at == self.bytes.len() {
            Ok(value)
        } else {
            self.fail("Extra data", self.at)
        }
    }

    fn skip_whitespace(&mut self) {
        while matches!(self.bytes.get(self.at), Some(b' ' | b'\t' | b'\n' | b'\r')) {
            self.at += 1;
        }
    }

    fn eat(&mut self, word: &str) -> bool {
        if self.text[self.at..].starts_with(word) {
            self.at += word.len();
            true
        } else {
            false
        }
    }

    fn value(&mut self, depth: usize) -> Result<Value, ()> {
        if depth > MAX_DEPTH {
            return self.fail("maximum recursion depth exceeded", self.at);
        }
        let Some(first) = self.bytes.get(self.at) else {
            return self.fail("Expecting value", self.at);
        };
        match first {
            b'{' => self.object(depth),
            b'[' => self.array(depth),
            b'"' => self.string().map(Value::Str),
            b'n' if self.eat("null") => Ok(Value::Null),
            b't' if self.eat("true") => Ok(Value::Bool(true)),
            b'f' if self.eat("false") => Ok(Value::Bool(false)),
            b'N' if self.eat("NaN") => Ok(Value::Float(f64::NAN)),
            b'I' if self.eat("Infinity") => Ok(Value::Float(f64::INFINITY)),
            b'-' if self.eat("-Infinity") => Ok(Value::Float(f64::NEG_INFINITY)),
            b'-' | b'0'..=b'9' => {
                let start = self.at;
                self.number()
                    .or_else(|()| self.fail("Expecting value", start))
            }
            _ => self.fail("Expecting value", self.at),
        }
    }

    fn object(&mut self, depth: usize) -> Result<Value, ()> {
        self.at += 1;
        let mut object = Object::new();
        self.skip_whitespace();
        if self.bytes.get(self.at) == Some(&b'}') {
            self.at += 1;
            return Ok(Value::Object(object));
        }
        let mut repeated: Option<String> = None;
        let mut comma: Option<usize> = None;
        loop {
            self.skip_whitespace();
            if self.bytes.get(self.at) != Some(&b'"') {
                return match comma {
                    Some(at) if self.bytes.get(self.at) == Some(&b'}') => {
                        self.fail("Illegal trailing comma before end of object", at)
                    }
                    _ => self.fail("Expecting property name enclosed in double quotes", self.at),
                };
            }
            let key = self.string()?;
            self.skip_whitespace();
            if self.bytes.get(self.at) != Some(&b':') {
                return self.fail("Expecting ':' delimiter", self.at);
            }
            self.at += 1;
            self.skip_whitespace();
            let value = self.value(depth + 1)?;
            if object.get(&key).is_some() && !self.last_duplicate_wins && repeated.is_none() {
                repeated = Some(key.clone());
            }
            object.insert(key, value);
            self.skip_whitespace();
            match self.bytes.get(self.at) {
                Some(b',') => {
                    comma = Some(self.at);
                    self.at += 1;
                }
                Some(b'}') => {
                    self.at += 1;
                    if repeated.is_some() {
                        self.duplicate = repeated;
                        return Err(());
                    }
                    return Ok(Value::Object(object));
                }
                _ => return self.fail("Expecting ',' delimiter", self.at),
            }
        }
    }

    fn array(&mut self, depth: usize) -> Result<Value, ()> {
        self.at += 1;
        let mut items = Vec::new();
        self.skip_whitespace();
        if self.bytes.get(self.at) == Some(&b']') {
            self.at += 1;
            return Ok(Value::Array(items));
        }
        loop {
            self.skip_whitespace();
            items.push(self.value(depth + 1)?);
            self.skip_whitespace();
            match self.bytes.get(self.at) {
                Some(b',') => {
                    let comma = self.at;
                    self.at += 1;
                    self.skip_whitespace();
                    if self.bytes.get(self.at) == Some(&b']') {
                        return self.fail("Illegal trailing comma before end of array", comma);
                    }
                }
                Some(b']') => {
                    self.at += 1;
                    return Ok(Value::Array(items));
                }
                _ => return self.fail("Expecting ',' delimiter", self.at),
            }
        }
    }

    fn string(&mut self) -> Result<String, ()> {
        let start = self.at;
        self.at += 1;
        let mut out = String::new();
        loop {
            let rest = &self.text[self.at..];
            let mut chars = rest.chars();
            let Some(next) = chars.next() else {
                return self.fail("Unterminated string starting at", start);
            };
            let here = self.at;
            self.at += next.len_utf8();
            match next {
                '"' => return Ok(out),
                '\\' => self.escape(&mut out, start)?,
                control if (control as u32) < 0x20 => {
                    return self.fail("Invalid control character at", here);
                }
                other => out.push(other),
            }
        }
    }

    fn escape(&mut self, out: &mut String, start: usize) -> Result<(), ()> {
        let Some(&code) = self.bytes.get(self.at) else {
            return self.fail("Unterminated string starting at", start);
        };
        self.at += 1;
        match code {
            b'"' => out.push('"'),
            b'\\' => out.push('\\'),
            b'/' => out.push('/'),
            b'b' => out.push('\u{8}'),
            b'f' => out.push('\u{c}'),
            b'n' => out.push('\n'),
            b'r' => out.push('\r'),
            b't' => out.push('\t'),
            b'u' => {
                let first = self.hex4()?;
                if (0xD800..0xDC00).contains(&first) && self.text[self.at..].starts_with("\\u") {
                    let saved = self.at;
                    self.at += 2;
                    let second = self.hex4()?;
                    if (0xDC00..0xE000).contains(&second) {
                        let combined = 0x10000 + ((first - 0xD800) << 10) + (second - 0xDC00);
                        out.push(char::from_u32(combined).ok_or(())?);
                        return Ok(());
                    }
                    self.at = saved;
                }
                out.push(char::from_u32(first).unwrap_or('\u{FFFD}'));
            }
            _ => return self.fail("Invalid \\escape", self.at - 2),
        }
        Ok(())
    }

    fn hex4(&mut self) -> Result<u32, ()> {
        let digits = match self.text.get(self.at..self.at + 4) {
            Some(digits) if digits.bytes().all(|byte| byte.is_ascii_hexdigit()) => digits,
            _ => return self.fail("Invalid \\uXXXX escape", self.at - 1),
        };
        self.at += 4;
        u32::from_str_radix(digits, 16).map_err(|_| ())
    }

    fn number(&mut self) -> Result<Value, ()> {
        let start = self.at;
        let is_float = self.scan_number()?;
        let literal = &self.text[start..self.at];
        if is_float {
            return literal.parse::<f64>().map(Value::Float).map_err(|_| ());
        }
        if literal.trim_start_matches('-').len() > crate::okf::INT_MAX_STR_DIGITS {
            return Err(());
        }
        Ok(Value::Int(if literal == "-0" {
            "0".to_string()
        } else {
            literal.to_string()
        }))
    }

    fn scan_number(&mut self) -> Result<bool, ()> {
        if self.bytes.get(self.at) == Some(&b'-') {
            self.at += 1;
        }
        match self.bytes.get(self.at) {
            Some(b'0') => self.at += 1,
            Some(b'1'..=b'9') => self.digits(),
            _ => return Err(()),
        }
        let mut is_float = false;
        if self.bytes.get(self.at) == Some(&b'.')
            && self.bytes.get(self.at + 1).is_some_and(u8::is_ascii_digit)
        {
            self.at += 1;
            self.digits();
            is_float = true;
        }
        if matches!(self.bytes.get(self.at), Some(b'e' | b'E')) {
            let mark = self.at;
            self.at += 1;
            if matches!(self.bytes.get(self.at), Some(b'+' | b'-')) {
                self.at += 1;
            }
            if self.bytes.get(self.at).is_some_and(u8::is_ascii_digit) {
                self.digits();
                is_float = true;
            } else {
                self.at = mark;
            }
        }
        Ok(is_float)
    }

    fn digits(&mut self) {
        while self.bytes.get(self.at).is_some_and(u8::is_ascii_digit) {
            self.at += 1;
        }
    }
}

#[derive(Clone, Copy)]
pub struct Style {
    pub indent: Option<usize>,
    pub sort_keys: bool,
    pub ensure_ascii: bool,
    pub compact: bool,
}

impl Style {
    pub const PRINTED: Style = Style {
        indent: Some(2),
        sort_keys: false,
        ensure_ascii: false,
        compact: false,
    };
    fn separators(self) -> (&'static str, &'static str) {
        match (self.compact, self.indent) {
            (true, _) => (",", ":"),
            (false, Some(_)) => (",", ": "),
            (false, None) => (", ", ": "),
        }
    }
}

pub fn dumps(value: &Value, style: Style) -> String {
    let mut out = String::new();
    write_value(&mut out, value, style, 0);
    out
}

fn write_value(out: &mut String, value: &Value, style: Style, level: usize) {
    match value {
        Value::Null => out.push_str("null"),
        Value::Bool(true) => out.push_str("true"),
        Value::Bool(false) => out.push_str("false"),
        Value::Int(digits) => out.push_str(digits),
        Value::Float(number) => out.push_str(&float_repr(*number)),
        Value::Str(text) => write_string(out, text, style.ensure_ascii),
        Value::Array(items) => {
            let rendered: Vec<&Value> = items.iter().collect();
            write_container(out, '[', ']', rendered.len(), style, level, |out, index| {
                write_value(out, rendered[index], style, level + 1);
            });
        }
        Value::Object(object) => {
            let mut entries: Vec<&(String, Value)> = object.iter().collect();
            if style.sort_keys {
                entries.sort_by(|left, right| left.0.cmp(&right.0));
            }
            let (_, key_separator) = style.separators();
            write_container(out, '{', '}', entries.len(), style, level, |out, index| {
                write_string(out, &entries[index].0, style.ensure_ascii);
                out.push_str(key_separator);
                write_value(out, &entries[index].1, style, level + 1);
            });
        }
    }
}

fn write_container(
    out: &mut String,
    open: char,
    close: char,
    count: usize,
    style: Style,
    level: usize,
    mut item: impl FnMut(&mut String, usize),
) {
    out.push(open);
    if count == 0 {
        out.push(close);
        return;
    }
    let (item_separator, _) = style.separators();
    let newline = |out: &mut String, depth: usize| {
        if let Some(width) = style.indent {
            out.push('\n');
            out.push_str(&" ".repeat(width * depth));
        }
    };
    for index in 0..count {
        if index > 0 {
            out.push_str(item_separator);
        }
        newline(out, level + 1);
        item(out, index);
    }
    newline(out, level);
    out.push(close);
}

fn write_string(out: &mut String, text: &str, ensure_ascii: bool) {
    out.push('"');
    for character in text.chars() {
        match character {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            '\u{8}' => out.push_str("\\b"),
            '\u{c}' => out.push_str("\\f"),
            control if (control as u32) < 0x20 => {
                let _ = write!(out, "\\u{:04x}", control as u32);
            }
            plain if !ensure_ascii || (' '..='~').contains(&plain) => out.push(plain),
            wide => {
                let mut units = [0u16; 2];
                for unit in wide.encode_utf16(&mut units) {
                    let _ = write!(out, "\\u{unit:04x}");
                }
            }
        }
    }
    out.push('"');
}

pub fn float_repr(number: f64) -> String {
    if number.is_nan() {
        return "NaN".to_string();
    }
    if number.is_infinite() {
        return if number > 0.0 {
            "Infinity"
        } else {
            "-Infinity"
        }
        .to_string();
    }
    if number == 0.0 {
        return if number.is_sign_negative() {
            "-0.0"
        } else {
            "0.0"
        }
        .to_string();
    }
    let scientific = format!("{:e}", number.abs());
    let (mantissa, exponent) = scientific.split_once('e').unwrap_or((&scientific, "0"));
    let digits: String = mantissa.chars().filter(char::is_ascii_digit).collect();
    let point = exponent.parse::<i32>().unwrap_or(0) + 1;
    let sign = if number < 0.0 { "-" } else { "" };
    let body = if !(-3..=16).contains(&point) {
        let (head, tail) = digits.split_at(1);
        let fraction = if tail.is_empty() {
            String::new()
        } else {
            format!(".{tail}")
        };
        let shown = point - 1;
        format!(
            "{head}{fraction}e{}{:02}",
            if shown < 0 { '-' } else { '+' },
            shown.abs()
        )
    } else if point <= 0 {
        format!("0.{}{digits}", "0".repeat(point.unsigned_abs() as usize))
    } else if point as usize >= digits.len() {
        format!("{digits}{}.0", "0".repeat(point as usize - digits.len()))
    } else {
        let (whole, fraction) = digits.split_at(point as usize);
        format!("{whole}.{fraction}")
    };
    format!("{sign}{body}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn floats_print_as_python_repr_prints_them() {
        let cases = [
            (1.0, "1.0"),
            (0.1, "0.1"),
            (1e16, "1e+16"),
            (1e15, "1000000000000000.0"),
            (123456789012345680.0, "1.2345678901234568e+17"),
            (0.0001, "0.0001"),
            (0.00001, "1e-05"),
            (1.5e300, "1.5e+300"),
            (-2.5e-7, "-2.5e-07"),
            (-0.0, "-0.0"),
            (12.5, "12.5"),
        ];
        for (number, expected) in cases {
            assert_eq!(float_repr(number), expected, "{number}");
        }
    }

    #[test]
    fn printed_json_matches_python_indent_two_without_ascii_escaping() {
        let value =
            parse(r#"{"b": [1, 2.5, {}], "a": [], "é": "é\n\u0001\"", "n": null}"#).unwrap();
        assert_eq!(
            dumps(&value, Style::PRINTED),
            "{\n  \"b\": [\n    1,\n    2.5,\n    {}\n  ],\n  \"a\": [],\n  \"é\": \"é\\n\\u0001\\\"\",\n  \"n\": null\n}"
        );
    }

    #[test]
    fn canonical_json_sorts_keys_and_uses_tight_separators() {
        let value = parse(r#"{"b": 1, "a": [true, "x"]}"#).unwrap();
        assert_eq!(
            dumps(
                &value,
                Style {
                    indent: None,
                    sort_keys: true,
                    ensure_ascii: false,
                    compact: true
                }
            ),
            r#"{"a":[true,"x"],"b":1}"#
        );
    }

    #[test]
    fn ascii_escaping_writes_surrogate_pairs_like_python() {
        let style = Style {
            indent: None,
            sort_keys: false,
            ensure_ascii: true,
            compact: false,
        };
        assert_eq!(
            dumps(&Value::Str("é😀\u{7f}".into()), style),
            "\"\\u00e9\\ud83d\\ude00\\u007f\""
        );
    }

    #[test]
    fn python_rejects_what_this_parser_rejects() {
        for text in [
            "{\"a\": 1, \"a\": 2}",
            "[1,]",
            "01",
            "\"tab\there\"",
            "{} x",
            "\u{feff}{}",
            "[1.]",
            "-",
        ] {
            assert!(parse(text).is_err(), "{text}");
        }
    }

    #[test]
    fn python_accepts_what_this_parser_accepts() {
        let parsed = parse("[NaN, Infinity, -Infinity, -0, 1E2, 12345678901234567890123]").unwrap();
        let printed = dumps(
            &parsed,
            Style {
                indent: None,
                sort_keys: false,
                ensure_ascii: false,
                compact: false,
            },
        );
        assert_eq!(
            printed,
            "[NaN, Infinity, -Infinity, 0, 100.0, 12345678901234567890123]"
        );
    }
}
