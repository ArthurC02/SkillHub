use std::path::{Path, PathBuf};

use crate::difflib::close_matches;
use crate::files::{MAX_JSON_BYTES, load_json};
use crate::json::{self, Object, Value};
use crate::proposals::iter_packages;
use crate::pyvalue::{equal, splitlines, str_of};
use crate::store::{self, posix, registry_dir};
use crate::{okf, py};

const PROVEN_ASSETS: &[&str] = &["rules", "contracts"];
const APPLY_OPERATION: &str = "apply-approved-updates";
const OBLIGATIONS_FILE: &str = "test-obligations.json";
const EVIDENCE_FILE: &str = "evidence-bundle.json";
const CONFLICT_MARKERS: &[&str] = &["<<<<<<< ", ">>>>>>> ", "||||||| "];
const DID_YOU_MEAN_LIMIT: usize = 3;

pub struct Reply {
    pub record: Value,
    pub notes: Vec<String>,
}

pub fn get_record(
    root: &Path,
    asset: &str,
    identifier: &str,
    changes_root: Option<&str>,
) -> Result<Reply, String> {
    let Some(record) = read_record(root, asset, identifier)? else {
        let known = store::asset_records(root, asset)?;
        return Err(format!(
            "{asset} record not found: {identifier}{}",
            did_you_mean(identifier, &known)
        ));
    };
    let changes_root = match changes_root {
        Some(given) => Some(py::resolve(given)),
        None => Some(root.join("changes")).filter(|beside| beside.is_dir()),
    };
    let mut notes = Vec::new();
    let record = match changes_root {
        Some(changes_root) if PROVEN_ASSETS.contains(&asset) => {
            with_proofs(root, &changes_root, asset, identifier, record, &mut notes)?
        }
        _ => record,
    };
    Ok(Reply {
        record: Value::Object(record),
        notes,
    })
}

fn is_concept_file_name(identifier: &str) -> bool {
    let mut characters = identifier.chars();
    characters.next().is_some_and(|c| c.is_ascii_alphanumeric())
        && characters.all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '_' | '-'))
        && identifier != "index"
}

pub fn read_record(root: &Path, asset: &str, identifier: &str) -> Result<Option<Object>, String> {
    if !is_concept_file_name(identifier) {
        return Ok(None);
    }
    let concept = registry_dir(root)
        .join(asset)
        .join(format!("{identifier}.md"));
    if !concept.is_file() {
        return Ok(None);
    }
    let bytes = std::fs::read(&concept).map_err(|error| py::os_error(&error, &concept))?;
    let text =
        py::utf8_text(&bytes).map_err(|error| format!("{asset}/{identifier}.md: {error}"))?;
    okf::decode(asset, identifier, &text)
        .map(Some)
        .map_err(|error| format!("{asset}/{identifier}.md: {error}"))
}

pub fn did_you_mean(identifier: &str, records: &[Value]) -> String {
    let known: Vec<&str> = records
        .iter()
        .filter_map(|record| record.as_object()?.get("id")?.as_str())
        .collect();
    let mut close: Vec<&str> = known
        .iter()
        .copied()
        .filter(|candidate| candidate.starts_with(identifier))
        .collect();
    for candidate in close_matches(identifier, &known, DID_YOU_MEAN_LIMIT) {
        if !close.contains(&candidate) {
            close.push(candidate);
        }
    }
    close.truncate(DID_YOU_MEAN_LIMIT);
    if close.is_empty() {
        String::new()
    } else {
        format!("; did you mean {}?", close.join(", "))
    }
}

fn with_proofs(
    root: &Path,
    changes_root: &Path,
    asset: &str,
    identifier: &str,
    record: Object,
    notes: &mut Vec<String>,
) -> Result<Object, String> {
    let key = format!("{asset}/{identifier}");
    let Some(proposals) = review_history(root, &key)? else {
        notes.push(format!(
            "NOTE: {key} was not reviewed through an applied Change Package, so no test obligation proves it yet."
        ));
        return Ok(record);
    };
    let source = (asset.strip_suffix('s').unwrap_or(asset), identifier);
    let packages = readable_packages(changes_root)?;
    let mut since: Vec<&Value> = Vec::new();
    for proposal_id in proposals.iter().rev() {
        match obligations_proving(&packages, proposal_id, source)? {
            None => {
                notes.push(format!(
                    "NOTE: the Change Package of {}, which reviewed {key}, is not under {}, so the tests that prove it are unknown.",
                    str_of(proposal_id),
                    py::shown(changes_root)
                ));
                return Ok(record);
            }
            Some(found) if !found.is_empty() => {
                if !since.is_empty() {
                    let proven_by = found[0]
                        .as_object()
                        .and_then(|entry| entry.get("proposal_id"));
                    notes.push(format!(
                        "NOTE: {key} is proven by {}; {}, applied since, changed it without a test obligation, as a re-citation does.",
                        proven_by.map(str_of).unwrap_or_default(),
                        joined(&since)?
                    ));
                }
                return Ok(proven(record, found));
            }
            Some(_) => since.push(proposal_id),
        }
    }
    Ok(proven(record, Vec::new()))
}

fn joined(items: &[&Value]) -> Result<String, String> {
    items
        .iter()
        .enumerate()
        .map(|(index, item)| {
            item.as_str()
                .map(str::to_string)
                .ok_or_else(|| format!("sequence item {index}: expected str instance"))
        })
        .collect::<Result<Vec<_>, _>>()
        .map(|parts| parts.join(", "))
}

fn proven(mut record: Object, obligations: Vec<Value>) -> Object {
    record.insert("proven_by", Value::Array(obligations));
    record
}

fn review_history(root: &Path, key: &str) -> Result<Option<Vec<Value>>, String> {
    let Some(events) = audit_events(root)? else {
        return Ok(None);
    };
    let history: Vec<Value> = events
        .iter()
        .flat_map(|event| reviewed_by(event, key))
        .collect();
    Ok((!history.is_empty()).then_some(history))
}

pub fn audit_events(root: &Path) -> Result<Option<Vec<Object>>, String> {
    let path = root.join("audit").join("events.jsonl");
    if !path.is_file() {
        return Ok(None);
    }
    if path.metadata().map(|meta| meta.len()).unwrap_or(0) > MAX_JSON_BYTES {
        return Err(format!(
            "audit log exceeds {MAX_JSON_BYTES} bytes: {}",
            py::shown(&path)
        ));
    }
    let bytes = std::fs::read(&path).map_err(|error| py::os_error(&error, &path))?;
    let text = py::utf8_text(&bytes).map_err(|error| format!("audit/events.jsonl: {error}"))?;
    splitlines(&text)
        .into_iter()
        .enumerate()
        .map(|(index, line)| parsed_event(index + 1, line, root))
        .collect::<Result<Vec<_>, _>>()
        .map(Some)
}

fn parsed_event(index: usize, line: &str, root: &Path) -> Result<Object, String> {
    if CONFLICT_MARKERS
        .iter()
        .any(|marker| line.starts_with(marker))
        || line == "======="
    {
        return Err(format!(
            "conflict markers at event {index}; run reconcile-audit --registry-root {}",
            posix(&py::resolve_path(root))
        ));
    }
    match json::parse(line) {
        Ok(Value::Object(event)) => Ok(event),
        Ok(_) => Err(format!("audit event {index} must be an object")),
        Err(()) => Err(format!("audit event {index} is invalid JSON")),
    }
}

pub fn reviewed_by(event: &Object, key: &str) -> Vec<Value> {
    match event.get("operation").and_then(Value::as_str) {
        Some(APPLY_OPERATION) => {
            let names_key = match event.get("records") {
                None => false,
                Some(Value::Array(records)) => {
                    records.iter().any(|record| record.as_str() == Some(key))
                }
                Some(Value::Object(records)) => records.get(key).is_some(),
                Some(_) => false,
            };
            let proposal_id = event.get("proposal_id").cloned().unwrap_or(Value::Null);
            if names_key {
                vec![proposal_id]
            } else {
                Vec::new()
            }
        }
        Some("migrate-registry") => event
            .get("reviewed_by")
            .and_then(Value::as_object)
            .and_then(|reviewed| reviewed.get(key))
            .cloned()
            .into_iter()
            .collect(),
        _ => Vec::new(),
    }
}

pub fn readable_packages(changes_root: &Path) -> Result<Vec<(PathBuf, Object)>, String> {
    iter_packages(changes_root)?
        .into_iter()
        .map(|(package, proposal)| {
            proposal
                .map(|proposal| (package.clone(), proposal))
                .map_err(|error| {
                    format!(
                        "Change Package {} cannot be read: {error}",
                        file_name(&package)
                    )
                })
        })
        .collect()
}

fn file_name(package: &Path) -> String {
    package
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_default()
}

pub fn obligations_proving(
    packages: &[(PathBuf, Object)],
    proposal_id: &Value,
    source: (&str, &str),
) -> Result<Option<Vec<Value>>, String> {
    let matching: Vec<&PathBuf> = packages
        .iter()
        .filter(|(_, proposal)| {
            equal(
                proposal.get("proposal_id").unwrap_or(&Value::Null),
                proposal_id,
            )
        })
        .map(|(package, _)| package)
        .collect();
    if matching.is_empty() {
        return Ok(None);
    }
    let mut found = Vec::new();
    for package in matching {
        for entry in obligations(package)? {
            let Some(entry) = entry.as_object() else {
                continue;
            };
            if !covers(entry, source) {
                continue;
            }
            let obligation_id = entry.get("id").cloned().unwrap_or(Value::Null);
            let mut row = Object::new();
            row.insert("proposal_id", proposal_id.clone());
            row.insert("package", Value::Str(file_name(package)));
            row.insert("obligation_id", obligation_id.clone());
            row.insert("check", entry.get("check").cloned().unwrap_or(Value::Null));
            row.insert(
                "working_directory",
                working_directory(package, &obligation_id)?,
            );
            found.push(Value::Object(row));
        }
    }
    Ok(Some(found))
}

pub fn obligations(package: &Path) -> Result<Vec<Value>, String> {
    let path = package.join(OBLIGATIONS_FILE);
    if !path.is_file() {
        return Ok(Vec::new());
    }
    Ok(match load_json(&path)?.get("obligations") {
        None => Vec::new(),
        Some(Value::Array(items)) => items.clone(),
        Some(Value::Str(text)) => text.chars().map(|c| Value::Str(c.to_string())).collect(),
        Some(Value::Object(object)) => object
            .iter()
            .map(|(key, _)| Value::Str(key.clone()))
            .collect(),
        Some(_) => return Err("obligations is not iterable".to_string()),
    })
}

pub fn covers(obligation: &Object, source: (&str, &str)) -> bool {
    let named = |item: &Object| {
        item.get("source_type").and_then(Value::as_str) == Some(source.0)
            && item.get("source_id").and_then(Value::as_str) == Some(source.1)
    };
    let also = match obligation.get("also_covers") {
        Some(Value::Array(items)) => items.as_slice(),
        _ => &[],
    };
    named(obligation) || also.iter().filter_map(Value::as_object).any(named)
}

fn working_directory(package: &Path, obligation_id: &Value) -> Result<Value, String> {
    let bundle = load_json(&package.join(EVIDENCE_FILE))?;
    let mut directory = Value::Str(".".to_string());
    let results: Vec<Value> = match bundle.get("test_results") {
        None => Vec::new(),
        Some(Value::Array(items)) => items.clone(),
        Some(_) => Vec::new(),
    };
    for result in results.iter().filter_map(Value::as_object) {
        let named = equal(
            result.get("obligation_id").unwrap_or(&Value::Null),
            obligation_id,
        );
        if let (true, Some(Value::Str(path))) = (named, result.get("working_directory")) {
            directory = Value::Str(path.clone());
        }
    }
    Ok(directory)
}
