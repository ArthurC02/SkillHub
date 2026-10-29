use crate::pyvalue::{message_repr, message_text, splitlines, str_of};
use std::path::{Path, PathBuf};

use crate::coverage::all_references;
use crate::crossrefs::{cross_reference_errors, identifier, records_of};
use crate::evidence::Verifier;
use crate::indexes::{self, Loaded};
use crate::json::{Object, Value};
use crate::listing::Repository;
use crate::okf::{review_field, truthy};
use crate::policy::{existing_policy, review_mode, validate_policy};
use crate::proposals::list_proposals;
use crate::py;
use crate::store::{
    ASSETS, Registry, has_registry, missing_registry_for, registry_dir, shown_path,
};

const REGISTRY_STATUSES: [&str; 4] = ["candidate", "deprecated", "reviewed", "superseded"];
const CONFLICT_MARKERS: [&str; 3] = ["<<<<<<< ", ">>>>>>> ", "||||||| "];
const KNOWN_CONSISTENCIES: [&str; 2] = ["synchronous", "eventual"];
const REVIEW_REQUIRED_FIELDS: &[(&str, &[&str])] = &[
    ("contexts", &["name", "responsibility", "verified"]),
    (
        "vocabulary",
        &["name", "definition", "contexts", "verified"],
    ),
    ("aggregates", &["context", "root", "invariants", "verified"]),
    ("rules", &["contexts", "statement", "verified"]),
    (
        "contracts",
        &[
            "kind",
            "producer_context",
            "consumer_contexts",
            "version",
            "compatibility_policy",
            "data_classification",
            "verified",
        ],
    ),
    (
        "interactions",
        &[
            "producer_context",
            "consumer_context",
            "consistency",
            "delivery",
            "verified",
        ],
    ),
    ("decisions", &["statement", "source", "verified"]),
    (
        "events",
        &["owner_context", "meaning", "schema", "verified"],
    ),
    ("capabilities", &["context", "meaning", "verified"]),
    (
        "value-objects",
        &["context", "meaning", "fields", "verified"],
    ),
    (
        "dependency-policies",
        &["from_context", "to_context", "mode", "policy", "verified"],
    ),
];

pub fn validate(
    root: &Path,
    repository: Option<&mut Repository>,
    require_reviewed: bool,
) -> Result<Vec<String>, String> {
    let repo_root = repository.as_ref().map(|found| found.root.clone());
    if !has_registry(root) {
        return Ok(vec![missing_registry_for(root, repo_root.as_deref())?]);
    }
    let registry = Registry::open(root)?;
    let mut errors = conflict_marker_errors(root, &registry)?;
    if !errors.is_empty() {
        return Ok(errors);
    }
    errors.extend(policy_errors(root));
    if !errors.is_empty() {
        return Ok(errors);
    }
    if require_reviewed && review_mode(root)? != "scm-verified" {
        errors.push("local-draft-only Domain Memory cannot satisfy --require-reviewed".to_string());
    }
    let (loaded, load_errors) = load_assets(&registry);
    errors.extend(load_errors);
    for (asset, entries) in &loaded {
        for entry in entries {
            entry_status_errors(asset, entry, require_reviewed, &mut errors);
        }
    }
    errors.extend(cross_reference_errors(&loaded));
    if let Some(repo_root) = &repo_root {
        errors.extend(evidence_errors(&loaded, repo_root)?);
    }
    for path in indexes::stale_indexes(root, &loaded)? {
        errors.push(format!(
            "{} does not list the records beside it; run refresh-indexes --registry-root {}",
            shown_path(&path, Some(root)),
            shown_path(root, None)
        ));
    }
    if let Some(repository) = repository {
        let contexts = records_of(&loaded, "contexts");
        if loaded.iter().any(|(asset, _)| *asset == "contexts") {
            errors.extend(implementation_path_errors(contexts, repository)?);
        }
    }
    Ok(errors)
}

fn policy_errors(root: &Path) -> Vec<String> {
    match existing_policy(root) {
        Ok(Some(policy)) => validate_policy(&policy),
        Ok(None) => Vec::new(),
        Err(error) => vec![error],
    }
}

fn load_assets(registry: &Registry) -> (Vec<(&'static str, Vec<Object>)>, Vec<String>) {
    let mut loaded = Vec::new();
    let mut errors = Vec::new();
    for (asset, _, _) in ASSETS {
        match registry.records(asset) {
            Ok(records) => loaded.push((
                *asset,
                records
                    .into_iter()
                    .filter_map(|record| record.as_object().cloned())
                    .collect(),
            )),
            Err(error) => errors.push(error),
        }
    }
    (loaded, errors)
}

fn required_fields(asset: &str) -> &'static [&'static str] {
    REVIEW_REQUIRED_FIELDS
        .iter()
        .find(|(name, _)| *name == asset)
        .map_or(&[], |(_, fields)| fields)
}

fn entry_status_errors(
    asset: &str,
    entry: &Object,
    require_reviewed: bool,
    errors: &mut Vec<String>,
) {
    let label = format!("{asset}/{}", identifier(entry));
    let status = entry
        .get(review_field(asset))
        .cloned()
        .unwrap_or_else(|| Value::Str("candidate".to_string()));
    let Some(name) = status
        .as_str()
        .filter(|name| REGISTRY_STATUSES.contains(name))
    else {
        errors.push(format!(
            "{label} has an invalid status: {} is not one of {}",
            message_repr(&status),
            REGISTRY_STATUSES.join(", ")
        ));
        return;
    };
    if entry.get("review").is_some() {
        errors.push(format!(
            "{label} holds a review block; this version reads verified only"
        ));
    }
    if name == "reviewed" {
        let missing: Vec<&str> = required_fields(asset)
            .iter()
            .copied()
            .filter(|field| !entry.get(field).is_some_and(truthy))
            .collect();
        if !missing.is_empty() {
            errors.push(format!(
                "{label} is reviewed but missing {}",
                missing.join(", ")
            ));
        }
        if !entry.get("evidence").is_some_and(truthy) {
            errors.push(format!("{label} is reviewed but lacks evidence"));
        }
    }
    if require_reviewed && name != "reviewed" {
        errors.push(format!("{label} is not reviewed"));
    }
}

fn evidence_errors(loaded: &Loaded, repo_root: &Path) -> Result<Vec<String>, String> {
    let mut held = Object::new();
    for (asset, records) in loaded {
        held.insert(
            format!("{asset}.json"),
            Value::Array(records.iter().cloned().map(Value::Object).collect()),
        );
    }
    let mut verifier = Verifier::new(repo_root, false);
    let mut errors = Vec::new();
    for reference in all_references(&Value::Object(held), 0)? {
        let result = verifier.check(&reference)?;
        let status = result.get("status").map(str_of).unwrap_or_default();
        if ["invalid", "missing", "unhashed"].contains(&status.as_str()) {
            let shown = result.get("reason").or_else(|| result.get("path"));
            errors.push(format!(
                "evidence {status}: {}",
                shown.map_or_else(|| "null".to_string(), message_text)
            ));
        }
    }
    Ok(errors)
}

fn lossy_text(data: &[u8]) -> String {
    String::from_utf8_lossy(data)
        .replace("\r\n", "\n")
        .replace('\r', "\n")
}

fn json_beside(root: &Path) -> Vec<PathBuf> {
    let Ok(entries) = std::fs::read_dir(root) else {
        return Vec::new();
    };
    entries
        .flatten()
        .filter(|entry| {
            let name = entry.file_name().to_string_lossy().into_owned();
            let name = if cfg!(windows) {
                name.to_lowercase()
            } else {
                name
            };
            name.ends_with(".json") && entry.path().is_file()
        })
        .map(|entry| entry.path())
        .collect()
}

fn files_under(directory: &Path, found: &mut Vec<PathBuf>) {
    let Ok(entries) = std::fs::read_dir(directory) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if entry.file_type().is_ok_and(|kind| kind.is_dir()) {
            files_under(&path, found);
        } else if path.is_file() {
            found.push(path);
        }
    }
}

fn conflict_marker_errors(root: &Path, registry: &Registry) -> Result<Vec<String>, String> {
    let mut texts: Vec<(PathBuf, String)> = registry
        .files()
        .iter()
        .map(|(relative, data)| (registry_dir(root).join(relative), lossy_text(data)))
        .collect();
    let mut elsewhere = json_beside(root);
    files_under(&root.join("changes"), &mut elsewhere);
    for path in elsewhere {
        let data = std::fs::read(&path).map_err(|error| py::os_error(&error, &path))?;
        texts.push((path, lossy_text(&data)));
    }
    texts.sort_by(|left, right| py::compare_paths(&left.0, &right.0));
    Ok(texts
        .iter()
        .filter_map(|(path, text)| Some((path, conflict_marked_line(text)?)))
        .map(|(path, line)| {
            format!(
                "{} holds git conflict markers at line {line}; resolve the merge before validating",
                shown_path(path, Some(root))
            )
        })
        .collect())
}

fn conflict_marked_line(text: &str) -> Option<usize> {
    splitlines(text)
        .iter()
        .position(|line| {
            CONFLICT_MARKERS
                .iter()
                .any(|marker| line.starts_with(marker))
        })
        .map(|index| index + 1)
}

pub struct Unplaced {
    pub id: String,
    pub path: String,
    pub reviewed: bool,
}

pub fn owned_directory(implementation_path: &str) -> String {
    let parts: Vec<&str> = implementation_path
        .trim_matches('/')
        .split('/')
        .filter(|part| !part.is_empty() && *part != ".")
        .collect();
    if parts.is_empty() {
        "/".to_string()
    } else {
        format!("/{}/", parts.join("/"))
    }
}

pub fn unplaced_implementation_paths(
    contexts: &[Object],
    repository: &mut Repository,
) -> Result<Vec<Unplaced>, String> {
    let placed: Vec<Unplaced> = contexts
        .iter()
        .filter_map(|entry| {
            let path = entry.get("implementation_path")?.as_str()?;
            (!py::strip(path).is_empty()).then(|| Unplaced {
                id: identifier(entry),
                path: path.to_string(),
                reviewed: entry
                    .get("status")
                    .is_some_and(|status| status.as_str() == Some("reviewed")),
            })
        })
        .collect();
    if placed.is_empty() {
        return Ok(placed);
    }
    let directories = repository.directories()?;
    Ok(placed
        .into_iter()
        .filter(|found| {
            let owned = owned_directory(&found.path);
            !directories
                .iter()
                .any(|directory| directory.contains(&owned))
        })
        .collect())
}

fn implementation_path_errors(
    contexts: &[Object],
    repository: &mut Repository,
) -> Result<Vec<String>, String> {
    Ok(unplaced_implementation_paths(contexts, repository)?
        .into_iter()
        .filter(|found| found.reviewed)
        .map(|found| {
            format!(
                "contexts/{} implementation_path {} lies over no file in the repository; after a move, relocate-evidence --from <old repository path> --to <new> proposes the new path, or correct it",
                found.id, found.path
            )
        })
        .collect())
}

pub fn unplaced_note(
    registry_root: &Path,
    repository: &mut Repository,
) -> Result<Option<String>, String> {
    let contexts = Registry::open(registry_root)?.records("contexts")?;
    let contexts: Vec<Object> = contexts
        .into_iter()
        .filter_map(|record| record.as_object().cloned())
        .collect();
    let unplaced: Vec<String> = unplaced_implementation_paths(&contexts, repository)?
        .into_iter()
        .filter(|found| !found.reviewed)
        .map(|found| format!("contexts/{} implementation_path {}", found.id, found.path))
        .collect();
    if unplaced.is_empty() {
        return Ok(None);
    }
    let verb = if unplaced.len() == 1 { "lies" } else { "lie" };
    Ok(Some(format!(
        "NOTE: {} {verb} over no file in the repository; a candidate may name code to come, and ship-package refuses to review it until code is there or the path is corrected.",
        unplaced.join(", ")
    )))
}

pub fn consistency_notes(registry_root: &Path) -> Result<Vec<String>, String> {
    let interactions = Registry::open(registry_root)?.records("interactions")?;
    Ok(interactions
        .iter()
        .filter_map(Value::as_object)
        .filter_map(|record| {
            let consistency = record.get("consistency")?.as_str()?;
            (!KNOWN_CONSISTENCIES.contains(&consistency)).then(|| {
                format!(
                    "NOTE: interactions/{} consistency \"{consistency}\" is not one of the known values {}; it is accepted as written, but a reader looking for those values misses it.",
                    identifier(record),
                    KNOWN_CONSISTENCIES.join(", ")
                )
            })
        })
        .collect())
}

pub fn unsettled_notes(registry_root: &Path) -> Result<Vec<String>, String> {
    let beside = registry_root.join("changes");
    if !beside.is_dir() {
        return Ok(Vec::new());
    }
    let rows = list_proposals(&beside)?;
    let rows: Vec<&Object> = rows.iter().filter_map(Value::as_object).collect();
    let proposal = |row: &Object| {
        row.get("proposal_id")
            .map_or_else(|| "null".to_string(), message_text)
    };
    let pending: Vec<String> = rows
        .iter()
        .filter(|row| row.get("status").and_then(Value::as_str) == Some("approved"))
        .map(|row| proposal(row))
        .collect();
    let dangling: Vec<String> = rows
        .iter()
        .filter(|row| row.get("superseded_by_unknown").is_some_and(truthy))
        .map(|row| {
            let by = row
                .get("superseded_by")
                .map_or_else(|| "null".to_string(), message_text);
            format!("{} (by {by})", proposal(row))
        })
        .collect();
    let mut notes = Vec::new();
    if !dangling.is_empty() {
        let names = if dangling.len() == 1 {
            " names"
        } else {
            "s name"
        };
        notes.push(format!(
            "NOTE: {} superseded Change Package{names} a replacement no package carries: {}. Create it with redraft-proposal --proposal-id <id>.",
            dangling.len(),
            dangling.join(", ")
        ));
    }
    if !pending.is_empty() {
        let plural = if pending.len() == 1 { "" } else { "s" };
        notes.push(format!(
            "NOTE: {} approved Change Package{plural} not applied: {}. Apply each with ship-package; retire one that cannot be applied with supersede-proposal, and redraft-proposal carries its work into a new draft.",
            pending.len(),
            pending.join(", ")
        ));
    }
    Ok(notes)
}
