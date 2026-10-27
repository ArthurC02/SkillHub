use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

use crate::files::{PROPOSAL_FILE, load_json};
use crate::json::{Object, Value};
use crate::py;
use crate::pyvalue::equal;

const PROPOSAL_MOMENTS: [&str; 6] = [
    "redrafted_at",
    "submitted_at",
    "verified_at",
    "finalized_at",
    "applied_at",
    "superseded_at",
];

pub fn completed_identifier(value: &Value) -> bool {
    value
        .as_str()
        .is_some_and(|text| !py::strip(text).is_empty() && !text.starts_with('<'))
}

fn completed_identifiers(value: &Value) -> bool {
    value
        .as_array()
        .is_some_and(|items| items.iter().all(completed_identifier))
}

pub type Package = (PathBuf, Result<Object, String>);

pub fn iter_packages(changes_root: &Path) -> Result<Vec<Package>, String> {
    let entries =
        std::fs::read_dir(changes_root).map_err(|error| py::os_error(&error, changes_root))?;
    let mut packages: Vec<PathBuf> = entries
        .filter_map(Result::ok)
        .map(|entry| changes_root.join(entry.file_name()))
        .filter(|package| package.join(PROPOSAL_FILE).is_file())
        .collect();
    packages.sort_by(|left, right| py::compare_paths(left, right));
    Ok(packages
        .into_iter()
        .map(|package| {
            let proposal = load_json(&package.join(PROPOSAL_FILE));
            (package, proposal)
        })
        .collect())
}

fn superseded_ids(row: &Object) -> Option<BTreeSet<String>> {
    match row.get("supersedes") {
        None | Some(Value::Null) => Some(BTreeSet::new()),
        Some(value) if completed_identifier(value) => value
            .as_str()
            .map(|text| BTreeSet::from([text.to_string()])),
        Some(value) if completed_identifiers(value) => {
            let items: Vec<String> = value
                .as_array()
                .unwrap_or_default()
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_string)
                .collect();
            let unique: BTreeSet<String> = items.iter().cloned().collect();
            (!items.is_empty() && unique.len() == items.len()).then_some(unique)
        }
        Some(_) => None,
    }
}

fn package_name(package: &Path) -> Value {
    Value::Str(
        package
            .file_name()
            .map(|name| name.to_string_lossy().into_owned())
            .unwrap_or_default(),
    )
}

pub fn list_proposals(changes_root: &Path) -> Result<Vec<Value>, String> {
    let mut rows: Vec<Object> = Vec::new();
    let mut replacements: Vec<(usize, Value)> = Vec::new();
    for (package, proposal) in iter_packages(changes_root)? {
        let proposal = match proposal {
            Ok(proposal) => proposal,
            Err(error) => {
                let mut row = Object::new();
                row.insert("package", package_name(&package));
                row.insert("error", Value::Str(error));
                rows.push(row);
                continue;
            }
        };
        let field = |key: &str| proposal.get(key).cloned().unwrap_or(Value::Null);
        let last_changed = PROPOSAL_MOMENTS
            .iter()
            .filter_map(|key| proposal.get(key).and_then(Value::as_str))
            .max();
        let updates = match proposal.get("registry_updates") {
            Some(Value::Array(items)) => items.len(),
            _ => 0,
        };
        let mut row = Object::new();
        row.insert("package", package_name(&package));
        row.insert("proposal_id", field("proposal_id"));
        row.insert("status", field("status"));
        row.insert(
            "last_changed_at",
            last_changed.map_or(Value::Null, |moment| Value::Str(moment.to_string())),
        );
        row.insert("registry_updates", Value::int(updates));
        row.insert(
            "supersedes",
            proposal
                .get("supersedes")
                .cloned()
                .unwrap_or(Value::Array(Vec::new())),
        );
        rows.push(row);
        if let Some(replacement) = proposal
            .get("superseded_by")
            .filter(|value| completed_identifier(value))
        {
            replacements.push((rows.len() - 1, replacement.clone()));
        }
    }
    let known: Vec<Value> = rows
        .iter()
        .map(|row| row.get("proposal_id").cloned().unwrap_or(Value::Null))
        .collect();
    let count = |id: &Value| known.iter().filter(|other| equal(other, id)).count();
    for row in &mut rows {
        let id = row.get("proposal_id").cloned().unwrap_or(Value::Null);
        if id != Value::Null && count(&id) > 1 {
            row.insert("duplicate_proposal_id", Value::Bool(true));
        }
    }
    for (index, replacement) in replacements {
        let unknown = !known.iter().any(|other| equal(other, &replacement));
        rows[index].insert("superseded_by", replacement);
        if unknown {
            rows[index].insert("superseded_by_unknown", Value::Bool(true));
        }
    }
    for row in &mut rows {
        let unknown: Vec<Value> = superseded_ids(row)
            .unwrap_or_default()
            .into_iter()
            .filter(|id| {
                !known
                    .iter()
                    .any(|other| equal(other, &Value::Str(id.clone())))
            })
            .map(Value::Str)
            .collect();
        if !unknown.is_empty() {
            row.insert("supersedes_unknown", Value::Array(unknown));
        }
    }
    Ok(rows.into_iter().map(Value::Object).collect())
}
