use crate::json::{self, Object, Style, Value};
use crate::okf::{LIST_RELATIONS, prose_field, review_field, title_field};
use crate::py;
use crate::pyvalue::{repr_of, str_of};

const CONCEPT_TYPES: &[(&str, &str)] = &[
    ("contexts", "Bounded Context"),
    ("vocabulary", "Term"),
    ("aggregates", "Aggregate"),
    ("rules", "Business Rule"),
    ("contracts", "Contract"),
    ("interactions", "Interaction"),
    ("decisions", "Decision"),
    ("events", "Domain Event"),
    ("capabilities", "Capability"),
    ("value-objects", "Value Object"),
    ("dependency-policies", "Dependency Policy"),
];
const RELATION_ASSETS: &[(&str, &str)] = &[
    ("context", "contexts"),
    ("contexts", "contexts"),
    ("owner_context", "contexts"),
    ("producer_context", "contexts"),
    ("consumer_context", "contexts"),
    ("consumer_contexts", "contexts"),
    ("from_context", "contexts"),
    ("to_context", "contexts"),
    ("consumers", "contexts"),
    ("aggregate", "aggregates"),
    ("contract_id", "contracts"),
    ("not_same_as", "vocabulary"),
    ("supersedes", "decisions"),
];
const OKF_STATUSES: &[(&str, &str)] = &[
    ("candidate", "draft"),
    ("reviewed", "stable"),
    ("deprecated", "deprecated"),
];
const RESERVED_KEYS: &[&str] = &[
    "type",
    "title",
    "status",
    "registry_status",
    "decision_status",
    "sources",
];
const YAML_WORDS: &[&str] = &[
    "true", "false", "null", "yes", "no", "on", "off", "y", "n", "~",
];
const RELATIONS_HEADING: &str = "Relations";

pub fn concept_type(asset: &str) -> &'static str {
    CONCEPT_TYPES
        .iter()
        .find(|(name, _)| *name == asset)
        .map_or("", |(_, kind)| kind)
}

pub fn encode(asset: &str, record: &Object) -> Result<String, String> {
    let mut remaining = Object::new();
    for (key, value) in record.iter().filter(|(key, _)| key != "id") {
        remaining.insert(key.clone(), value.clone());
    }
    let mut front = Object::new();
    front.insert("type", Value::Str(concept_type(asset).to_string()));
    if let Some(field) = title_field(asset)
        && remaining
            .get(field)
            .and_then(Value::as_str)
            .is_some_and(|title| is_text(title) && !title.contains('\n'))
    {
        front.insert("title", take(&mut remaining, field));
    }
    for (key, value) in status_keys(asset, &mut remaining)? {
        front.insert(key, value);
    }
    let prose = take_prose(asset, &mut remaining);
    let relations = take_relations(&mut remaining);
    let sections = take_sections(&mut remaining);
    let sources = take_sources(&mut remaining);
    if let Some((key, _)) = remaining
        .iter()
        .find(|(key, _)| RESERVED_KEYS.contains(&key.as_str()))
    {
        return Err(format!(
            "{asset}/{} has a field named {key}, which the concept format reserves",
            record.get("id").map_or_else(|| "None".to_string(), str_of)
        ));
    }
    for (key, value) in remaining.iter() {
        front.insert(key.clone(), value.clone());
    }
    if let Some(sources) = sources {
        front.insert("sources", Value::Array(sources));
    }
    let mut text = format!("---\n{}\n---\n", dump_yaml(&front, 0)?.join("\n"));
    let body = body(prose, &relations, &sections);
    if !body.is_empty() {
        text.push_str(&body);
        text.push('\n');
    }
    Ok(text)
}

fn take(remaining: &mut Object, key: &str) -> Value {
    let value = remaining.get(key).cloned().unwrap_or(Value::Null);
    let kept: Vec<(String, Value)> = remaining
        .iter()
        .filter(|(name, _)| name != key)
        .cloned()
        .collect();
    let mut rebuilt = Object::new();
    for (name, value) in kept {
        rebuilt.insert(name, value);
    }
    *remaining = rebuilt;
    value
}

fn is_text(value: &str) -> bool {
    !value.is_empty() && py::strip(value) == value
}

fn unhashable(value: &Value) -> Result<(), String> {
    match value {
        Value::Array(_) => Err("unhashable type: 'list'".to_string()),
        Value::Object(_) => Err("unhashable type: 'dict'".to_string()),
        _ => Ok(()),
    }
}

fn status_keys(asset: &str, remaining: &mut Object) -> Result<Vec<(&'static str, Value)>, String> {
    let mut keys = Vec::new();
    let field = review_field(asset);
    if remaining.get(field).is_some() {
        let status = take(remaining, field);
        unhashable(&status)?;
        match OKF_STATUSES
            .iter()
            .find(|(registry, _)| status.as_str() == Some(registry))
        {
            Some((_, okf)) => keys.push(("status", Value::Str((*okf).to_string()))),
            None => {
                if status.as_str() == Some("superseded") {
                    keys.push(("status", Value::Str("deprecated".to_string())));
                }
                keys.push(("registry_status", status));
            }
        }
    }
    if asset == "decisions" && remaining.get("status").is_some() {
        keys.push(("decision_status", take(remaining, "status")));
    }
    Ok(keys)
}

fn take_prose(asset: &str, remaining: &mut Object) -> Option<String> {
    let field = prose_field(asset)?;
    let value = remaining.get(field)?.as_str()?;
    if !is_text(value) || value.split('\n').any(|line| line.starts_with('#')) {
        return None;
    }
    take(remaining, field).as_str().map(str::to_string)
}

fn is_concept_id(text: &str) -> bool {
    !text.is_empty()
        && text
            .chars()
            .all(|c| !py::is_space(c) && !matches!(c, '(' | ')' | '[' | ']'))
}

fn relation_ids(field: &str, value: &Value) -> Option<Vec<String>> {
    let values: Vec<&Value> = if LIST_RELATIONS.contains(&field) {
        match value {
            Value::Array(items) if !items.is_empty() => items.iter().collect(),
            _ => return None,
        }
    } else {
        vec![value]
    };
    values
        .into_iter()
        .map(|item| {
            item.as_str()
                .filter(|id| is_concept_id(id))
                .map(str::to_string)
        })
        .collect()
}

fn take_relations(remaining: &mut Object) -> Vec<(String, &'static str, String)> {
    let fields: Vec<String> = remaining
        .iter()
        .map(|(key, _)| key.clone())
        .filter(|key| RELATION_ASSETS.iter().any(|(field, _)| field == key))
        .collect();
    let mut relations = Vec::new();
    for field in fields {
        let Some(ids) = remaining
            .get(&field)
            .and_then(|value| relation_ids(&field, value))
        else {
            continue;
        };
        take(remaining, &field);
        let target = RELATION_ASSETS
            .iter()
            .find(|(name, _)| *name == field)
            .map_or("", |(_, asset)| asset);
        relations.extend(ids.into_iter().map(|id| (field.clone(), target, id)));
    }
    relations
}

fn capitalize(text: &str) -> String {
    let mut characters = text.chars();
    match characters.next() {
        Some(first) => first
            .to_uppercase()
            .chain(characters.as_str().to_lowercase().chars())
            .collect(),
        None => String::new(),
    }
}

fn label(field: &str) -> String {
    capitalize(&field.replace('_', " "))
}

fn field_of(label: &str) -> String {
    label.to_lowercase().replace(' ', "_")
}

fn is_section(field: &str, value: &Value) -> bool {
    let Value::Array(items) = value else {
        return false;
    };
    field_of(&label(field)) == field
        && label(field) != RELATIONS_HEADING
        && !items.is_empty()
        && items.iter().all(|item| {
            item.as_str()
                .is_some_and(|text| is_text(text) && !text.contains('\n'))
        })
}

fn take_sections(remaining: &mut Object) -> Vec<(String, Vec<String>)> {
    let fields: Vec<String> = remaining
        .iter()
        .filter(|(key, value)| is_section(key, value))
        .map(|(key, _)| key.clone())
        .collect();
    fields
        .into_iter()
        .map(|field| {
            let items = take(remaining, &field)
                .as_array()
                .unwrap_or_default()
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_string)
                .collect();
            (field, items)
        })
        .collect()
}

fn cited_source(reference: &Value) -> Option<Value> {
    let reference = reference.as_object()?;
    let path = reference.get("path")?.as_str()?;
    let lines = reference.get("lines")?.as_object()?;
    let keys: Vec<&str> = lines.iter().map(|(key, _)| key.as_str()).collect();
    if keys.len() != 2 || !keys.contains(&"start") || !keys.contains(&"end") {
        return None;
    }
    let (Some(Value::Int(start)), Some(Value::Int(end))) = (lines.get("start"), lines.get("end"))
    else {
        return None;
    };
    if reference.get("resource").is_some() {
        return None;
    }
    let mut source = Object::new();
    source.insert("resource", Value::Str(format!("{path}#L{start}-{end}")));
    for (key, value) in reference.iter() {
        if key != "path" && key != "lines" {
            source.insert(key.clone(), value.clone());
        }
    }
    Some(Value::Object(source))
}

fn take_sources(remaining: &mut Object) -> Option<Vec<Value>> {
    let evidence = remaining.get("evidence")?.as_array()?;
    if evidence.is_empty() {
        return None;
    }
    let sources: Option<Vec<Value>> = evidence.iter().map(cited_source).collect();
    let sources = sources?;
    take(remaining, "evidence");
    Some(sources)
}

fn body(
    prose: Option<String>,
    relations: &[(String, &str, String)],
    sections: &[(String, Vec<String>)],
) -> String {
    let mut parts: Vec<String> = prose.into_iter().collect();
    if !relations.is_empty() {
        let mut lines = vec![format!("# {RELATIONS_HEADING}")];
        lines.extend(relations.iter().map(|(field, asset, target)| {
            format!("- {}: [{target}](/{asset}/{target})", label(field))
        }));
        parts.push(lines.join("\n"));
    }
    for (field, items) in sections {
        let mut lines = vec![format!("# {}", label(field))];
        lines.extend(items.iter().map(|item| format!("- {item}")));
        parts.push(lines.join("\n"));
    }
    parts.join("\n\n")
}

fn is_yaml_key(key: &str) -> bool {
    let mut characters = key.chars();
    characters
        .next()
        .is_some_and(|first| first.is_ascii_alphabetic() || first == '_')
        && characters.all(|c| c.is_ascii_alphanumeric() || c == '_')
}

fn is_plain(text: &str) -> bool {
    let mut characters = text.chars();
    characters
        .next()
        .is_some_and(|first| first.is_ascii_alphabetic())
        && characters.all(|c| c.is_ascii_alphanumeric() || " _./@+()-".contains(c))
        && !text.ends_with(' ')
        && !YAML_WORDS.contains(&text.to_lowercase().as_str())
}

fn is_scalar(value: &Value) -> bool {
    match value {
        Value::Array(items) => items.is_empty(),
        Value::Object(object) => object.iter().next().is_none(),
        _ => true,
    }
}

fn scalar(value: &Value) -> Result<String, String> {
    Ok(match value {
        Value::Float(number) if !number.is_finite() => {
            return Err("the concept format cannot write a number that is not finite".to_string());
        }
        Value::Array(_) => "[]".to_string(),
        Value::Object(_) => "{}".to_string(),
        Value::Str(text) if is_plain(text) => text.clone(),
        other => json::dumps(other, Style::PRINTED),
    })
}

pub fn dump_yaml(mapping: &Object, indent: usize) -> Result<Vec<String>, String> {
    let pad = " ".repeat(indent);
    let mut lines = Vec::new();
    for (key, value) in mapping.iter() {
        if !is_yaml_key(key) {
            return Err(format!(
                "the concept format cannot write the key {}",
                repr_of(&Value::Str(key.clone()))
            ));
        }
        match value {
            _ if is_scalar(value) => lines.push(format!("{pad}{key}: {}", scalar(value)?)),
            Value::Object(nested) => {
                lines.push(format!("{pad}{key}:"));
                lines.extend(dump_yaml(nested, indent + 2)?);
            }
            Value::Array(items) => {
                lines.push(format!("{pad}{key}:"));
                lines.extend(dump_sequence(items, indent + 2)?);
            }
            _ => {}
        }
    }
    Ok(lines)
}

fn dump_sequence(items: &[Value], indent: usize) -> Result<Vec<String>, String> {
    let pad = " ".repeat(indent);
    let mut lines = Vec::new();
    for item in items {
        match item {
            _ if is_scalar(item) => lines.push(format!("{pad}- {}", scalar(item)?)),
            Value::Object(nested) => {
                let nested = dump_yaml(nested, indent + 2)?;
                lines.push(format!("{pad}- {}", nested[0].trim_start()));
                lines.extend(nested[1..].iter().cloned());
            }
            Value::Array(inner) => {
                lines.push(format!("{pad}-"));
                lines.extend(dump_sequence(inner, indent + 2)?);
            }
            _ => {}
        }
    }
    Ok(lines)
}
