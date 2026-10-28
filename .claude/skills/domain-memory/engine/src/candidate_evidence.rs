use std::path::Path;

use crate::cite::citation;
use crate::coverage::{SourceKinds, all_references, with_references};
use crate::evidence::shape;
use crate::files::load_json;
use crate::json::{Object, Value};
use crate::py;

const DERIVED_EVIDENCE_KEYS: [&str; 2] = ["content_sha256", "source_kind"];

pub fn source_map_for(registry_root: &Path) -> Result<Object, String> {
    let path = registry_root.join("source-map.json");
    if path.is_file() {
        load_json(&path)
    } else {
        Ok(Object::new())
    }
}

pub fn without_derived_keys(value: &Value) -> Result<Value, String> {
    with_references(
        value,
        &mut |reference| {
            Ok(match reference {
                Value::Object(object) => {
                    let mut kept = Object::new();
                    for (key, item) in object.iter() {
                        if !DERIVED_EVIDENCE_KEYS.contains(&key.as_str()) {
                            kept.insert(key.clone(), item.clone());
                        }
                    }
                    Value::Object(kept)
                }
                other => other.clone(),
            })
        },
        0,
    )
}

pub fn classify_all(value: &Value, kinds: &SourceKinds) -> Result<Value, String> {
    with_references(value, &mut |reference| Ok(kinds.classify(reference)), 0)
}

pub fn hash_all(value: &Value, repo_root: &Path) -> Result<Value, String> {
    let root = py::resolve_path(repo_root);
    with_references(value, &mut |reference| hashed(reference, &root), 0)
}

fn hashed(reference: &Value, root: &Path) -> Result<Value, String> {
    let (Ok((path, lines)), Value::Object(object)) = (shape(reference), reference) else {
        return Ok(reference.clone());
    };
    if object.get("excerpt_sha256").is_some() {
        return Ok(reference.clone());
    }
    let digits = |key: &str| match lines.get(key) {
        Some(Value::Int(digits)) => digits.as_str(),
        _ => "0",
    };
    let cited = citation(root, path, digits("start"), digits("end"))?;
    let mut merged = object.clone();
    for (key, item) in cited.iter() {
        merged.insert(key.clone(), item.clone());
    }
    Ok(Value::Object(merged))
}

pub fn unclassified_paths(value: &Value) -> Result<Vec<String>, String> {
    let mut paths: Vec<String> = all_references(value, 0)?
        .iter()
        .filter_map(Value::as_object)
        .filter(|reference| {
            reference.get("source_kind").and_then(Value::as_str) == Some("unclassified")
        })
        .filter_map(|reference| reference.get("path").and_then(Value::as_str))
        .map(str::to_string)
        .collect();
    paths.sort();
    paths.dedup();
    Ok(paths)
}

pub fn cited_excerpts(record: &Object) -> Result<Vec<(String, String)>, String> {
    let mut excerpts: Vec<(String, String)> = Vec::new();
    for reference in all_references(&Value::Object(record.clone()), 0)? {
        let Value::Object(reference) = reference else {
            continue;
        };
        if let (Some(Value::Str(path)), Some(Value::Str(digest))) =
            (reference.get("path"), reference.get("excerpt_sha256"))
        {
            let pair = (path.clone(), digest.clone());
            if !excerpts.contains(&pair) {
                excerpts.push(pair);
            }
        }
    }
    Ok(excerpts)
}

pub fn one_cites_within_the_other(one: &Object, other: &Object) -> Result<bool, String> {
    let (mine, theirs) = (cited_excerpts(one)?, cited_excerpts(other)?);
    Ok(!mine.is_empty()
        && !theirs.is_empty()
        && (mine.iter().all(|pair| theirs.contains(pair))
            || theirs.iter().all(|pair| mine.contains(pair))))
}
