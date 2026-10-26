use crate::digits::decimal_value;
use crate::json::{self, Object, Value};
use crate::py;

pub const INT_MAX_STR_DIGITS: usize = 4300;

const TITLE_FIELDS: &[(&str, &str)] = &[("contexts", "name"), ("vocabulary", "name")];

const PROSE_FIELDS: &[(&str, &str)] = &[
    ("contexts", "responsibility"),
    ("vocabulary", "definition"),
    ("rules", "statement"),
    ("decisions", "statement"),
    ("events", "meaning"),
    ("capabilities", "meaning"),
    ("value-objects", "meaning"),
    ("dependency-policies", "rationale"),
];

const LIST_RELATIONS: &[&str] = &[
    "contexts",
    "consumer_contexts",
    "consumers",
    "not_same_as",
    "supersedes",
];
const RESERVED_KEYS: &[&str] = &[
    "type",
    "title",
    "status",
    "registry_status",
    "decision_status",
    "sources",
];
const RELATIONS_HEADING: &str = "Relations";
const QUOTE_FIRST: &[char] = &['\'', '[', '{', '&', '*', '!', '|', '>', '%', '@', '`'];

pub fn prose_field(asset: &str) -> Option<&'static str> {
    lookup(PROSE_FIELDS, asset)
}

pub fn title_field(asset: &str) -> Option<&'static str> {
    lookup(TITLE_FIELDS, asset)
}

fn lookup(table: &[(&str, &'static str)], asset: &str) -> Option<&'static str> {
    table
        .iter()
        .find(|(name, _)| *name == asset)
        .map(|(_, field)| *field)
}

pub fn review_field(asset: &str) -> &'static str {
    if asset == "decisions" {
        "review_status"
    } else {
        "status"
    }
}

pub fn decode(asset: &str, record_id: &str, text: &str) -> Result<Object, String> {
    let (front, body) = split_concept(text)?;
    let mut record = Object::new();
    record.insert("id", Value::Str(record_id.to_string()));
    if let (Some(field), Some(title)) = (title_field(asset), front.get("title")) {
        record.insert(field, title.clone());
    }
    let (prose, relations, sections) = parse_body(&body)?;
    if !prose.is_empty() {
        let field = prose_field(asset).ok_or_else(|| {
            format!("a {asset} concept has no prose field; move its text into a frontmatter field")
        })?;
        record.insert(field, Value::Str(prose));
    }
    for (key, value) in front.iter() {
        if !RESERVED_KEYS.contains(&key.as_str()) {
            record.insert(key.clone(), value.clone());
        }
    }
    for (key, value) in relations.iter().chain(sections.iter()) {
        record.insert(key.clone(), value.clone());
    }
    if let Some(sources) = front.get("sources") {
        let sources = sources
            .as_array()
            .ok_or("sources must be a list of cited sources")?;
        let evidence = sources
            .iter()
            .map(evidence_of)
            .collect::<Result<Vec<_>, _>>()?;
        record.insert("evidence", Value::Array(evidence));
    }
    for (key, value) in registry_status(asset, &front)? {
        record.insert(key, value);
    }
    Ok(record)
}

fn registry_status(asset: &str, front: &Object) -> Result<Vec<(String, Value)>, String> {
    let mut keys = Vec::new();
    if asset == "decisions"
        && let Some(status) = front.get("decision_status")
    {
        keys.push(("status".to_string(), status.clone()));
    }
    let field = review_field(asset).to_string();
    if let Some(status) = front.get("registry_status") {
        keys.push((field, status.clone()));
    } else if let Some(status) = front.get("status") {
        let mapped = match status.as_str().ok_or("status must be text")? {
            "draft" => Value::Str("candidate".to_string()),
            "stable" => Value::Str("reviewed".to_string()),
            _ => status.clone(),
        };
        keys.push((field, mapped));
    }
    Ok(keys)
}

pub fn split_concept(text: &str) -> Result<(Object, String), String> {
    let normalized = text.replace("\r\n", "\n");
    let lines: Vec<&str> = normalized.split('\n').collect();
    if lines[0] != "---" {
        return Err("a concept document starts with a --- frontmatter line".to_string());
    }
    let end = lines
        .iter()
        .skip(1)
        .position(|line| *line == "---")
        .map(|offset| offset + 1)
        .ok_or("a concept document's frontmatter has no closing --- line")?;
    let front = match load_yaml(&lines[1..end])? {
        Value::Object(front) if front.get("type").is_some_and(truthy) => front,
        _ => return Err("a concept document's frontmatter needs a non-empty type".to_string()),
    };
    Ok((front, lines[end + 1..].join("\n")))
}

pub fn truthy(value: &Value) -> bool {
    match value {
        Value::Null => false,
        Value::Bool(flag) => *flag,
        Value::Int(digits) => digits != "0",
        Value::Float(number) => *number != 0.0,
        Value::Str(text) => !text.is_empty(),
        Value::Array(items) => !items.is_empty(),
        Value::Object(object) => object.iter().next().is_some(),
    }
}

fn evidence_of(source: &Value) -> Result<Value, String> {
    let Some(object) = source.as_object() else {
        return Ok(source.clone());
    };
    let Some(resource) = object.get("resource").and_then(Value::as_str) else {
        return Ok(source.clone());
    };
    let mut out = Object::new();
    match cited_range(resource)? {
        None => out.insert("path", Value::Str(resource.to_string())),
        Some((path, start, end)) => {
            out.insert("path", Value::Str(path));
            let mut lines = Object::new();
            lines.insert("start", start);
            lines.insert("end", end);
            out.insert("lines", Value::Object(lines));
        }
    }
    for (key, value) in object.iter() {
        if key != "resource" {
            out.insert(key.clone(), value.clone());
        }
    }
    Ok(Value::Object(out))
}

fn cited_range(resource: &str) -> Result<Option<(String, Value, Value)>, String> {
    if resource.contains('\n') {
        return Ok(None);
    }
    for (at, _) in resource.rmatch_indices("#L") {
        if at == 0 {
            continue;
        }
        let Some((start, end)) = resource[at + 2..].split_once('-') else {
            continue;
        };
        if is_decimal(start) && is_decimal(end) {
            return Ok(Some((
                resource[..at].to_string(),
                python_int(start)?,
                python_int(end)?,
            )));
        }
    }
    Ok(None)
}

fn is_decimal(text: &str) -> bool {
    !text.is_empty() && text.chars().all(|c| decimal_value(c).is_some())
}

fn python_int(text: &str) -> Result<Value, String> {
    let count = text.chars().count();
    if count > INT_MAX_STR_DIGITS {
        return Err(digit_limit(count));
    }
    let ascii: String = text
        .chars()
        .filter_map(decimal_value)
        .map(|digit| char::from_digit(digit, 10).unwrap_or('0'))
        .collect();
    let trimmed = ascii.trim_start_matches('0');
    Ok(Value::Int(if trimmed.is_empty() {
        "0".to_string()
    } else {
        trimmed.to_string()
    }))
}

pub fn digit_limit(count: usize) -> String {
    format!(
        "Exceeds the limit ({INT_MAX_STR_DIGITS} digits) for integer string conversion: value has {count} digits; use sys.set_int_max_str_digits() to increase the limit"
    )
}

type Body = (String, Object, Object);

fn parse_body(body: &str) -> Result<Body, String> {
    let mut prose_lines = Vec::new();
    let mut relations = Object::new();
    let mut sections = Object::new();
    let mut heading: Option<String> = None;
    for line in body.split('\n') {
        if let Some(rest) = line.strip_prefix("# ") {
            heading = Some(py::strip(rest).to_string());
            continue;
        }
        match &heading {
            None => prose_lines.push(line),
            Some(name) if name == RELATIONS_HEADING && !py::strip(line).is_empty() => {
                add_relation(&mut relations, line)?;
            }
            Some(name) => {
                if let Some(item) = line.strip_prefix("- ") {
                    push_item(&mut sections, &field_of(name), item);
                } else if !py::strip(line).is_empty() {
                    return Err(format!(
                        "a line under {name} must start with '- ': {}",
                        py::strip(line)
                    ));
                }
            }
        }
    }
    Ok((
        py::strip(&prose_lines.join("\n")).to_string(),
        relations,
        sections,
    ))
}

fn push_item(object: &mut Object, field: &str, item: &str) {
    let item = Value::Str(item.to_string());
    match object.get_mut(field) {
        Some(Value::Array(items)) => items.push(item),
        _ => object.insert(field, Value::Array(vec![item])),
    }
}

fn field_of(label: &str) -> String {
    label.to_lowercase().replace(' ', "_")
}

fn add_relation(relations: &mut Object, line: &str) -> Result<(), String> {
    let stripped = py::strip(line);
    let (label, target) = relation_line(stripped).ok_or_else(|| {
        format!("a Relations line must read '- Label: [text](/asset/id)': {stripped}")
    })?;
    let field = field_of(label);
    if LIST_RELATIONS.contains(&field.as_str()) {
        push_item(relations, &field, target);
    } else {
        relations.insert(field, Value::Str(target.to_string()));
    }
    Ok(())
}

fn relation_line(line: &str) -> Option<(&str, &str)> {
    let body = line.strip_prefix("- ")?;
    body.char_indices()
        .skip(1)
        .filter(|(at, _)| body[*at..].starts_with(": ["))
        .find_map(|(at, _)| {
            let target = relation_target(&body[at + 3..])?;
            let label = &body[..at];
            (!label.contains('\n')).then_some((label, target))
        })
}

fn relation_target(rest: &str) -> Option<&str> {
    let (_, after) = rest.split_once(']')?;
    let link = after.strip_prefix("(/")?.strip_suffix(')')?;
    let (asset, target) = link.split_once('/')?;
    let asset_ok = !asset.is_empty() && asset.chars().all(|c| c.is_ascii_lowercase() || c == '-');
    let target_ok = !target.is_empty() && !target.chars().any(|c| c == ')' || py::is_space(c));
    (asset_ok && target_ok).then_some(target)
}

struct Row {
    indent: usize,
    text: String,
    number: usize,
}

struct Lines {
    rows: Vec<Row>,
    at: usize,
}

impl Lines {
    fn peek(&self) -> Option<&Row> {
        self.rows.get(self.at)
    }
}

pub fn load_yaml(lines: &[&str]) -> Result<Value, String> {
    let rows = lines
        .iter()
        .enumerate()
        .filter(|(_, line)| {
            let stripped = py::strip(line);
            !stripped.is_empty() && !stripped.starts_with('#')
        })
        .map(|(index, line)| Row {
            indent: line.chars().take_while(|c| *c == ' ').count(),
            text: py::strip(line).to_string(),
            number: index + 2,
        })
        .collect();
    let mut source = Lines { rows, at: 0 };
    let Some(indent) = source.peek().map(|row| row.indent) else {
        return Ok(Value::Object(Object::new()));
    };
    let value = parse_block(&mut source, indent)?;
    if let Some(row) = source.peek() {
        return Err(format!(
            "frontmatter line {}: unexpected indentation",
            row.number
        ));
    }
    Ok(value)
}

fn is_dash(text: &str) -> bool {
    text == "-" || text.starts_with("- ")
}

fn parse_block(source: &mut Lines, indent: usize) -> Result<Value, String> {
    if source.peek().is_some_and(|row| is_dash(&row.text)) {
        Ok(Value::Array(parse_sequence(source, indent)?))
    } else {
        Ok(Value::Object(parse_mapping(source, indent)?))
    }
}

fn parse_mapping(source: &mut Lines, indent: usize) -> Result<Object, String> {
    let mut mapping = Object::new();
    while let Some(row) = source.peek() {
        if row.indent != indent || row.text.starts_with('-') {
            break;
        }
        let number = row.number;
        let (key, rest) = key_value(&row.text, number)?;
        source.at += 1;
        if mapping.get(&key).is_some() {
            return Err(format!("frontmatter line {number}: {key} appears twice"));
        }
        if !rest.is_empty() {
            let value = parse_scalar(&rest, number)?;
            mapping.insert(key, value);
            continue;
        }
        let nested = source
            .peek()
            .filter(|next| {
                next.indent > indent || (next.indent == indent && next.text.starts_with('-'))
            })
            .map(|next| next.indent);
        let value = match nested {
            None => Value::Null,
            Some(nested) => parse_block(source, nested)?,
        };
        mapping.insert(key, value);
    }
    Ok(mapping)
}

fn parse_sequence(source: &mut Lines, indent: usize) -> Result<Vec<Value>, String> {
    let mut items = Vec::new();
    while let Some(row) = source.peek() {
        if row.indent != indent || !is_dash(&row.text) {
            break;
        }
        let number = row.number;
        let content = py::strip(&row.text[1..]).to_string();
        if content.is_empty() {
            source.at += 1;
            let nested = source
                .peek()
                .map(|next| next.indent)
                .ok_or_else(|| format!("frontmatter line {number}: a '-' entry has no value"))?;
            items.push(parse_block(source, nested)?);
        } else if starts_with_key(&content) {
            source.rows[source.at] = Row {
                indent: indent + 2,
                text: content,
                number,
            };
            items.push(Value::Object(parse_mapping(source, indent + 2)?));
        } else {
            source.at += 1;
            items.push(parse_scalar(&content, number)?);
        }
    }
    Ok(items)
}

fn is_key(text: &str) -> bool {
    let mut chars = text.chars();
    chars
        .next()
        .is_some_and(|c| c.is_ascii_alphabetic() || c == '_')
        && chars.all(|c| c.is_ascii_alphanumeric() || c == '_')
}

fn starts_with_key(content: &str) -> bool {
    content
        .split_once(':')
        .is_some_and(|(key, rest)| is_key(key) && (rest.is_empty() || rest.starts_with(' ')))
}

fn key_value(text: &str, number: usize) -> Result<(String, String), String> {
    match text.split_once(':') {
        Some((key, rest)) if is_key(key) && (rest.is_empty() || rest.starts_with(' ')) => {
            Ok((key.to_string(), py::strip(rest).to_string()))
        }
        _ => Err(format!(
            "frontmatter line {number}: expected 'key: value', got {text}"
        )),
    }
}

fn parse_scalar(text: &str, number: usize) -> Result<Value, String> {
    if text.starts_with('"') {
        return json::parse(text)
            .map_err(|()| format!("frontmatter line {number}: unreadable quoted text"));
    }
    if ["[]", "{}", "null", "true", "false"].contains(&text)
        || is_integer(text)
        || is_decimal_number(text)
    {
        return json_literal(text);
    }
    if text.starts_with(QUOTE_FIRST) {
        return Err(format!(
            "frontmatter line {number}: write this value plainly or in double quotes: {text}"
        ));
    }
    Ok(Value::Str(text.to_string()))
}

fn json_literal(text: &str) -> Result<Value, String> {
    if let Ok(value) = json::parse(text) {
        return Ok(value);
    }
    match json::number_prefix(text) {
        None => Err("Expecting value: line 1 column 1 (char 0)".to_string()),
        Some(end) if end < text.len() => Err(format!(
            "Extra data: line 1 column {} (char {end})",
            end + 1
        )),
        Some(_) => Err(digit_limit(text.trim_start_matches('-').len())),
    }
}

fn digit_run(text: &str) -> usize {
    text.char_indices()
        .find(|(_, c)| decimal_value(*c).is_none())
        .map_or(text.len(), |(at, _)| at)
}

fn signed_digits(text: &str) -> Option<&str> {
    let unsigned = text.strip_prefix('-').unwrap_or(text);
    let run = digit_run(unsigned);
    (run > 0).then(|| &unsigned[run..])
}

fn is_integer(text: &str) -> bool {
    signed_digits(text).is_some_and(str::is_empty)
}

fn is_decimal_number(text: &str) -> bool {
    let Some(rest) = signed_digits(text) else {
        return false;
    };
    let fraction = rest
        .strip_prefix('.')
        .filter(|after| digit_run(after) > 0)
        .map(|after| &after[digit_run(after)..]);
    if fraction.is_some_and(str::is_empty) {
        return true;
    }
    let before_exponent = fraction.unwrap_or(rest);
    before_exponent
        .strip_prefix(['e', 'E'])
        .map(|after| after.strip_prefix(['+', '-']).unwrap_or(after))
        .is_some_and(|digits| !digits.is_empty() && digit_run(digits) == digits.len())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn decoded(asset: &str, text: &str) -> String {
        match decode(asset, "x", text) {
            Ok(record) => json::dumps(&Value::Object(record), json::Style::PRINTED),
            Err(error) => format!("ERROR {error}"),
        }
    }

    #[test]
    fn numbers_follow_python_regexes_and_json_loads() {
        assert!(is_integer("-12") && is_integer("\u{663}") && !is_integer("1a"));
        assert!(
            is_decimal_number("1.5") && is_decimal_number("1e5") && is_decimal_number("1.5E-3")
        );
        assert!(!is_decimal_number("1.") && !is_decimal_number("1.5e") && !is_decimal_number(".5"));
        assert_eq!(
            json_literal("01"),
            Err("Extra data: line 1 column 2 (char 1)".to_string())
        );
        assert_eq!(
            json_literal("\u{663}"),
            Err("Expecting value: line 1 column 1 (char 0)".to_string())
        );
    }

    #[test]
    fn a_relation_label_is_the_shortest_that_still_matches() {
        assert_eq!(
            relation_line("- Owner context: [a](/contexts/a)"),
            Some(("Owner context", "a"))
        );
        assert_eq!(relation_line("- A: [b: [c](/x/y)"), Some(("A", "y")));
        assert_eq!(relation_line("- A: [b](/x/y z)"), None);
    }

    #[test]
    fn evidence_lines_split_at_the_last_cited_range() {
        let decoded = decoded(
            "rules",
            "---\ntype: Business Rule\nsources:\n  - resource: a#L1-2#L3-4\n    hash: h\n---\n",
        );
        assert!(decoded.contains("\"path\": \"a#L1-2\""), "{decoded}");
        assert!(decoded.contains("\"start\": 3"), "{decoded}");
    }
}
