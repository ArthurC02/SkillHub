use std::collections::{BTreeSet, HashMap, HashSet};
use std::path::{Path, PathBuf};

use crate::coverage::all_references;
use crate::evidence::Verifier;
use crate::json::{Object, Value};
use crate::okf::{prose_field, review_field, title_field, truthy};
use crate::policy::validate_policy;
use crate::py;
use crate::pyvalue::{equal, repr_of, splitlines, str_of};
use crate::source_probe::{
    Fail, Listing, Res, domain_memory_hint, existing_policy, os_failure, path_order, shown_path,
};
use crate::store::{ASSETS, Registry, has_registry, registry_dir, registry_files};
use crate::unicode::casefold;

const CONFLICT_MARKERS: [&str; 3] = ["<<<<<<< ", ">>>>>>> ", "||||||| "];
const REGISTRY_STATUSES: &str = "candidate, deprecated, reviewed, superseded";
const CONTEXT_SUBDOMAINS: [&str; 3] = ["core", "supporting", "generic"];
const ABSENCE_ASSETS: [&str; 5] = ["vocabulary", "aggregates", "rules", "contracts", "events"];
const SINGLE_CONTEXT_FIELDS: [(&str, &[&str]); 5] = [
    ("aggregates", &["context"]),
    ("interactions", &["producer_context", "consumer_context"]),
    ("events", &["owner_context"]),
    ("capabilities", &["context"]),
    ("value-objects", &["context"]),
];
const CONTEXT_RELATIONSHIPS: [&str; 7] = [
    "partnership",
    "shared-kernel",
    "customer-supplier",
    "conformist",
    "anticorruption-layer",
    "open-host-service",
    "published-language",
];
const DEPENDENCY_MODES: [&str; 4] = [
    "synchronous-query",
    "synchronous-write",
    "asynchronous",
    "none",
];
const DEPENDENCY_POLICIES: [&str; 2] = ["allowed", "forbidden"];
type ClosedField = (&'static str, &'static [&'static str]);
const CLOSED_FIELDS: [(&str, &[ClosedField]); 2] = [
    ("interactions", &[("relationship", &CONTEXT_RELATIONSHIPS)]),
    (
        "dependency-policies",
        &[
            ("mode", &DEPENDENCY_MODES),
            ("policy", &DEPENDENCY_POLICIES),
        ],
    ),
];
const SEPARATE_WAYS_HINT: &str = "; separate-ways is a decision not to collaborate, recorded as a dependency policy with policy forbidden and mode none";
const IDEMPOTENCY_FIELDS: [&str; 4] = ["key", "scope", "on_replay", "on_conflict"];
const OPTIONAL_IDEMPOTENCY_FIELDS: [&str; 1] = ["expiry"];
const INDEX: &str = "index.md";
const INDEX_DESCRIPTION_LIMIT: usize = 160;
const HEADINGS: [(&str, &str); 11] = [
    ("contexts", "Bounded Contexts"),
    ("vocabulary", "Vocabulary"),
    ("aggregates", "Aggregates"),
    ("rules", "Business Rules"),
    ("contracts", "Contracts"),
    ("interactions", "Interactions"),
    ("decisions", "Decisions"),
    ("events", "Domain Events"),
    ("capabilities", "Capabilities"),
    ("value-objects", "Value Objects"),
    ("dependency-policies", "Dependency Policies"),
];

fn review_required(asset: &str) -> &'static [&'static str] {
    match asset {
        "contexts" => &["name", "responsibility", "verified"],
        "vocabulary" => &["name", "definition", "contexts", "verified"],
        "aggregates" => &["context", "root", "invariants", "verified"],
        "rules" => &["contexts", "statement", "verified"],
        "contracts" => &[
            "kind",
            "producer_context",
            "consumer_contexts",
            "version",
            "compatibility_policy",
            "data_classification",
            "verified",
        ],
        "interactions" => &[
            "producer_context",
            "consumer_context",
            "consistency",
            "delivery",
            "verified",
        ],
        "decisions" => &["statement", "source", "verified"],
        "events" => &["owner_context", "meaning", "schema", "verified"],
        "capabilities" => &["context", "meaning", "verified"],
        "value-objects" => &["context", "meaning", "fields", "verified"],
        _ => &["from_context", "to_context", "mode", "policy", "verified"],
    }
}

struct Loaded {
    asset: &'static str,
    records: Vec<Object>,
}

type Records<'a> = &'a [Loaded];

fn held<'a>(records: Records<'a>, asset: &str) -> &'a [Object] {
    records
        .iter()
        .find(|loaded| loaded.asset == asset)
        .map_or(&[], |loaded| loaded.records.as_slice())
}

fn id_text(entry: &Object) -> String {
    entry.get("id").map_or_else(|| "None".to_string(), str_of)
}

fn field_text(entry: &Object, key: &str) -> String {
    entry.get(key).map_or_else(|| "None".to_string(), str_of)
}

fn lossy_lines(bytes: &[u8]) -> String {
    String::from_utf8_lossy(bytes)
        .replace("\r\n", "\n")
        .replace('\r', "\n")
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

fn walk_all(directory: &Path, found: &mut Vec<PathBuf>) {
    let Ok(entries) = std::fs::read_dir(directory) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        let is_directory = entry.file_type().is_ok_and(|kind| kind.is_dir());
        found.push(path.clone());
        if is_directory {
            walk_all(&path, found);
        }
    }
}

fn json_files_in(root: &Path) -> Vec<PathBuf> {
    let Ok(entries) = std::fs::read_dir(root) else {
        return Vec::new();
    };
    entries
        .flatten()
        .filter(|entry| {
            let name = entry.file_name().to_string_lossy().into_owned();
            if cfg!(windows) {
                name.to_lowercase().ends_with(".json")
            } else {
                name.ends_with(".json")
            }
        })
        .map(|entry| entry.path())
        .collect()
}

fn conflict_marker_errors(root: &Path) -> Res<Vec<String>> {
    let mut texts: Vec<(PathBuf, String)> = Vec::new();
    for (relative, data) in registry_files(root)? {
        texts.push((registry_dir(root).join(relative), lossy_lines(&data)));
    }
    let mut elsewhere = json_files_in(root);
    walk_all(&root.join("changes"), &mut elsewhere);
    for path in elsewhere.into_iter().filter(|path| path.is_file()) {
        let data = std::fs::read(&path).map_err(|error| os_failure(&error, &path))?;
        texts.push((path, lossy_lines(&data)));
    }
    texts.sort_by(|left, right| path_order(&left.0, &right.0));
    Ok(texts
        .iter()
        .filter_map(|(path, text)| {
            conflict_marked_line(text).map(|line| {
                format!(
                    "{} holds git conflict markers at line {line}; resolve the merge before validating",
                    shown_path(path, Some(root))
                )
            })
        })
        .collect())
}

fn entry_status_errors(asset: &str, entry: &Object) -> Vec<String> {
    let label = format!("{asset}/{}", id_text(entry));
    let status = entry
        .get(review_field(asset))
        .cloned()
        .unwrap_or_else(|| Value::Str("candidate".to_string()));
    let allowed = ["candidate", "reviewed", "deprecated", "superseded"];
    let Some(text) = status.as_str().filter(|text| allowed.contains(text)) else {
        return vec![format!(
            "{label} has an invalid status: {} is not one of {REGISTRY_STATUSES}",
            repr_of(&status)
        )];
    };
    let mut errors = Vec::new();
    if entry.get("review").is_some() {
        errors.push(format!(
            "{label} holds a review block; this version reads verified only"
        ));
    }
    if text == "reviewed" {
        let missing: Vec<&str> = review_required(asset)
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
    errors
}

fn unknown_message(contexts: &BTreeSet<String>) -> String {
    format!(
        "unknown context {}; add the Context first",
        contexts.iter().cloned().collect::<Vec<_>>().join(", ")
    )
}

fn texts_of(value: Option<&Value>) -> Vec<String> {
    value
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .map(str_of)
        .collect()
}

fn paths_overlap(one: &str, other: &str) -> bool {
    let first: Vec<&str> = one.trim_matches('/').split('/').collect();
    let second: Vec<&str> = other.trim_matches('/').split('/').collect();
    let shared = first.len().min(second.len());
    first[..shared] == second[..shared]
}

fn context_placement_problems(entries: &[Object]) -> Vec<String> {
    let mut errors = Vec::new();
    let mut claimed: Vec<(String, String)> = Vec::new();
    for entry in entries {
        let label = format!("contexts/{}", id_text(entry));
        if let Some(subdomain) = entry.get("subdomain")
            && !matches!(subdomain, Value::Null)
            && !subdomain
                .as_str()
                .is_some_and(|text| CONTEXT_SUBDOMAINS.contains(&text))
        {
            let hint = if subdomain.as_str() == Some("shared-kernel") {
                "; shared-kernel is a relationship between two Contexts, recorded on their Interaction"
            } else {
                ""
            };
            errors.push(format!(
                "{label} has an unknown subdomain: {} is not one of core, generic, supporting{hint}",
                repr_of(subdomain)
            ));
        }
        if let Some(prefixes) = entry.get("requirement_prefixes")
            && !matches!(prefixes, Value::Null)
        {
            let valid = prefixes.as_array().is_some_and(|items| {
                items.iter().all(|item| {
                    item.as_str()
                        .is_some_and(|text| !py::strip(text).is_empty())
                })
            });
            if !valid {
                errors.push(format!(
                    "{label} requirement_prefixes must be a list of non-empty strings"
                ));
            }
        }
        let path = match entry.get("implementation_path") {
            None | Some(Value::Null) => continue,
            Some(Value::Str(text)) if !py::strip(text).is_empty() => text.clone(),
            Some(_) => {
                errors.push(format!(
                    "{label} implementation_path must be a non-empty string"
                ));
                continue;
            }
        };
        for (other, owner) in &claimed {
            if paths_overlap(&path, other) {
                errors.push(format!(
                    "{label} claims {path}, which overlaps {other} claimed by {owner}: code belongs to one Context"
                ));
            }
        }
        match claimed.iter_mut().find(|(seen, _)| *seen == path) {
            Some(slot) => slot.1 = id_text(entry),
            None => claimed.push((path, id_text(entry))),
        }
    }
    errors
}

fn contexts_holding(records: Records) -> HashMap<&'static str, HashSet<String>> {
    let strings = |value: Option<&Value>| -> Vec<String> {
        value
            .and_then(Value::as_array)
            .unwrap_or_default()
            .iter()
            .filter_map(|item| item.as_str().map(str::to_string))
            .collect()
    };
    let text = |entry: &Object, key: &str| -> Option<String> {
        entry.get(key).and_then(Value::as_str).map(str::to_string)
    };
    let mut holding: HashMap<&'static str, HashSet<String>> = HashMap::new();
    for entry in held(records, "vocabulary") {
        holding
            .entry("vocabulary")
            .or_default()
            .extend(strings(entry.get("contexts")));
    }
    for entry in held(records, "aggregates") {
        holding
            .entry("aggregates")
            .or_default()
            .extend(text(entry, "context"));
    }
    for entry in held(records, "rules") {
        holding
            .entry("rules")
            .or_default()
            .extend(strings(entry.get("contexts")));
    }
    for entry in held(records, "contracts") {
        let slot = holding.entry("contracts").or_default();
        slot.extend(text(entry, "producer_context"));
        slot.extend(strings(entry.get("consumer_contexts")));
    }
    for entry in held(records, "events") {
        holding
            .entry("events")
            .or_default()
            .extend(text(entry, "owner_context"));
    }
    holding
}

fn confirmed_absence_errors(records: Records) -> Vec<String> {
    let holding = contexts_holding(records);
    let mut errors = Vec::new();
    for entry in held(records, "contexts") {
        let label = format!("contexts/{}", id_text(entry));
        let declarations = match entry.get("confirmed_absences") {
            None => Vec::new(),
            Some(Value::Array(items)) => items.clone(),
            Some(_) => {
                errors.push(format!("{label} confirmed_absences must be a list"));
                continue;
            }
        };
        for declaration in &declarations {
            let asset = declaration
                .as_object()
                .and_then(|found| found.get("asset"))
                .and_then(Value::as_str)
                .filter(|asset| ABSENCE_ASSETS.contains(asset));
            let Some(asset) = asset else {
                errors.push(format!(
                    "{label} confirms an absence of an unknown asset; use one of aggregates, contracts, events, rules, vocabulary"
                ));
                continue;
            };
            let reason = declaration
                .as_object()
                .and_then(|found| found.get("reason"))
                .and_then(Value::as_str);
            if reason.is_none_or(|text| py::strip(text).is_empty()) {
                errors.push(format!(
                    "{label} confirms an absence of {asset} without a reason; a confirmed absence is a finding and needs one"
                ));
            } else if entry
                .get("id")
                .and_then(Value::as_str)
                .is_some_and(|id| holding.get(asset).is_some_and(|named| named.contains(id)))
            {
                errors.push(format!(
                    "{label} confirms an absence of {asset}, but {asset} records name it"
                ));
            }
        }
    }
    errors
}

fn known_contexts(records: Records) -> HashSet<String> {
    held(records, "contexts")
        .iter()
        .filter_map(|entry| entry.get("id").and_then(Value::as_str).map(str::to_string))
        .collect()
}

fn outside(known: &HashSet<String>, value: Option<&Value>) -> bool {
    !value
        .and_then(Value::as_str)
        .is_some_and(|text| known.contains(text))
}

fn unknown_among(known: &HashSet<String>, names: &[String]) -> BTreeSet<String> {
    names
        .iter()
        .filter(|name| !known.contains(*name))
        .cloned()
        .collect()
}

fn context_list_errors(records: Records, known: &HashSet<String>) -> Vec<String> {
    let mut errors = Vec::new();
    for asset in ["vocabulary", "rules"] {
        for entry in held(records, asset) {
            let label = format!("{asset}/{}", id_text(entry));
            match entry.get("contexts") {
                Some(Value::Array(items)) if !items.is_empty() => {
                    let missing = unknown_among(known, &texts_of(entry.get("contexts")));
                    if !missing.is_empty() {
                        errors.push(format!("{label} references {}", unknown_message(&missing)));
                    }
                }
                _ => errors.push(format!("{label} requires contexts")),
            }
        }
    }
    errors
}

fn single_context_errors(records: Records, known: &HashSet<String>) -> Vec<String> {
    let mut errors = Vec::new();
    for (asset, fields) in SINGLE_CONTEXT_FIELDS {
        for entry in held(records, asset) {
            for field in fields {
                if outside(known, entry.get(field)) {
                    let shown = BTreeSet::from([field_text(entry, field)]);
                    errors.push(format!(
                        "{asset}/{} references {}",
                        id_text(entry),
                        unknown_message(&shown)
                    ));
                }
            }
        }
    }
    for entry in held(records, "dependency-policies") {
        let ends: BTreeSet<String> = ["from_context", "to_context"]
            .iter()
            .filter(|end| outside(known, entry.get(end)))
            .map(|end| field_text(entry, end))
            .collect();
        if !ends.is_empty() {
            errors.push(format!(
                "dependency-policies/{} references {}",
                id_text(entry),
                unknown_message(&ends)
            ));
        }
    }
    errors
}

fn contract_party_errors(records: Records, known: &HashSet<String>) -> Vec<String> {
    let mut errors = Vec::new();
    for entry in held(records, "contracts") {
        let label = format!("contracts/{}", id_text(entry));
        if outside(known, entry.get("producer_context")) {
            let shown = BTreeSet::from([field_text(entry, "producer_context")]);
            errors.push(format!(
                "{label} references as producer {}",
                unknown_message(&shown)
            ));
        }
        match entry.get("consumer_contexts") {
            None => {}
            Some(Value::Array(_)) => {
                let missing = unknown_among(known, &texts_of(entry.get("consumer_contexts")));
                if !missing.is_empty() {
                    errors.push(format!(
                        "{label} references as consumer {}",
                        unknown_message(&missing)
                    ));
                }
            }
            Some(_) => errors.push(format!(
                "{label} consumer_contexts must be a list of Context ids"
            )),
        }
    }
    errors
}

fn event_consumer_errors(records: Records, known: &HashSet<String>) -> Vec<String> {
    let mut errors = Vec::new();
    for entry in held(records, "events") {
        let label = format!("events/{}", id_text(entry));
        match entry.get("consumers") {
            None | Some(Value::Null) => {}
            Some(Value::Array(_)) => {
                let missing = unknown_among(known, &texts_of(entry.get("consumers")));
                if !missing.is_empty() {
                    errors.push(format!(
                        "{label} references as consumer {}",
                        unknown_message(&missing)
                    ));
                }
            }
            Some(_) => errors.push(format!("{label} consumers must be a list of Context ids")),
        }
    }
    errors
}

fn contract_reference_errors(records: Records) -> Vec<String> {
    let contract_ids: Vec<&Value> = held(records, "contracts")
        .iter()
        .filter_map(|entry| entry.get("id"))
        .collect();
    let mut errors = Vec::new();
    for asset in ["interactions", "events"] {
        for entry in held(records, asset) {
            match entry.get("contract_id") {
                None | Some(Value::Null) => {}
                Some(found) if contract_ids.iter().any(|id| equal(id, found)) => {}
                Some(_) => errors.push(format!(
                    "{asset}/{} references unknown contract",
                    id_text(entry)
                )),
            }
        }
    }
    errors
}

fn named_contexts(entry: &Object) -> Vec<Value> {
    match entry.get("contexts") {
        Some(Value::Array(items)) => items.clone(),
        _ => vec![entry.get("owner_context").cloned().unwrap_or(Value::Null)],
    }
}

fn aggregate_binding_errors(records: Records) -> Vec<String> {
    let aggregates: Vec<(&Value, Value)> = held(records, "aggregates")
        .iter()
        .map(|entry| {
            (
                entry.get("id").unwrap_or(&Value::Null),
                entry.get("context").cloned().unwrap_or(Value::Null),
            )
        })
        .collect();
    let mut errors = Vec::new();
    for asset in ["rules", "events"] {
        for entry in held(records, asset) {
            let aggregate = match entry.get("aggregate") {
                None | Some(Value::Null) => continue,
                Some(found) => found,
            };
            let label = format!("{asset}/{}", id_text(entry));
            let bound = aggregates.iter().rev().find(|(id, _)| equal(id, aggregate));
            match bound {
                None => errors.push(format!(
                    "{label} references unknown aggregate {}; add the Aggregate first",
                    str_of(aggregate)
                )),
                Some((_, context)) => {
                    if !named_contexts(entry)
                        .iter()
                        .any(|named| equal(named, context))
                    {
                        errors.push(format!(
                            "{label} aggregate {} belongs to context {}, which the record does not name",
                            str_of(aggregate),
                            str_of(context)
                        ));
                    }
                }
            }
        }
    }
    errors
}

fn closed_field_errors(records: Records) -> Vec<String> {
    let mut errors = Vec::new();
    for (asset, fields) in CLOSED_FIELDS {
        for entry in held(records, asset) {
            for (field, allowed) in fields.iter() {
                let Some(value) = entry
                    .get(field)
                    .filter(|found| !matches!(found, Value::Null))
                else {
                    continue;
                };
                if value.as_str().is_some_and(|text| allowed.contains(&text)) {
                    continue;
                }
                let hint = if *field == "relationship" && value.as_str() == Some("separate-ways") {
                    SEPARATE_WAYS_HINT
                } else {
                    ""
                };
                let mut sorted: Vec<&str> = allowed.to_vec();
                sorted.sort_unstable();
                errors.push(format!(
                    "{asset}/{} has an unknown {field}: {} is not one of {}{hint}",
                    id_text(entry),
                    repr_of(value),
                    sorted.join(", ")
                ));
            }
        }
    }
    errors
}

fn labels(entry: &Object, field: &str) -> Option<Vec<String>> {
    match entry.get(field) {
        None | Some(Value::Null) => Some(Vec::new()),
        Some(Value::Array(items)) => items
            .iter()
            .map(|item| {
                item.as_str()
                    .filter(|text| !py::strip(text).is_empty())
                    .map(str::to_string)
            })
            .collect(),
        Some(_) => None,
    }
}

fn context_set(entry: &Object) -> HashSet<String> {
    entry
        .get("contexts")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .map(str_of)
        .collect()
}

fn name_folded(entry: &Object) -> String {
    casefold(&entry.get("name").map(str_of).unwrap_or_default())
}

fn term_label_errors(terms: &[Object]) -> Vec<String> {
    let term_ids: Vec<&Value> = terms.iter().filter_map(|term| term.get("id")).collect();
    let mut errors = Vec::new();
    for (index, term) in terms.iter().enumerate() {
        let place = format!("vocabulary/{}", id_text(term));
        let (Some(synonyms), Some(look_alikes)) =
            (labels(term, "synonyms"), labels(term, "not_same_as"))
        else {
            errors.push(format!(
                "{place} synonyms and not_same_as must be lists of non-empty strings"
            ));
            continue;
        };
        let folded: Vec<String> = synonyms.iter().map(|synonym| casefold(synonym)).collect();
        let distinct: HashSet<&String> = folded.iter().collect();
        if distinct.len() != folded.len() {
            errors.push(format!("{place} repeats a synonym"));
        }
        if folded.contains(&name_folded(term)) {
            errors.push(format!("{place} lists its own name as a synonym"));
        }
        let contexts = context_set(term);
        for (other_index, other) in terms.iter().enumerate() {
            if other_index != index
                && folded.contains(&name_folded(other))
                && !contexts.is_disjoint(&context_set(other))
            {
                errors.push(format!(
                    "{place} synonym {} is the name of {} in a shared context",
                    other
                        .get("name")
                        .map_or_else(|| "None".to_string(), repr_of),
                    id_text(other)
                ));
            }
        }
        let names_unknown = look_alikes
            .iter()
            .any(|name| !term_ids.iter().any(|id| id.as_str() == Some(name)));
        let names_itself = term
            .get("id")
            .and_then(Value::as_str)
            .is_some_and(|id| look_alikes.iter().any(|name| name == id));
        if names_unknown || names_itself {
            errors.push(format!(
                "{place} not_same_as must name other vocabulary term ids"
            ));
        }
    }
    errors
}

fn idempotency_errors(contracts: &[Object]) -> Vec<String> {
    let mut errors = Vec::new();
    for contract in contracts {
        let idempotency = match contract.get("idempotency") {
            None | Some(Value::Null) => continue,
            Some(found) => found,
        };
        let place = format!("contracts/{}", id_text(contract));
        let Value::Object(fields) = idempotency else {
            errors.push(format!("{place} idempotency must be an object"));
            continue;
        };
        let missing: Vec<&str> = IDEMPOTENCY_FIELDS
            .iter()
            .copied()
            .filter(|field| fields.get(field).is_none())
            .collect();
        let known = |name: &str| {
            IDEMPOTENCY_FIELDS.contains(&name) || OPTIONAL_IDEMPOTENCY_FIELDS.contains(&name)
        };
        let mut unknown: Vec<&str> = fields
            .iter()
            .map(|(name, _)| name.as_str())
            .filter(|name| !known(name))
            .collect();
        unknown.sort_unstable();
        let mut blank: Vec<&str> = fields
            .iter()
            .filter(|(name, value)| {
                known(name)
                    && !value
                        .as_str()
                        .is_some_and(|text| !py::strip(text).is_empty())
            })
            .map(|(name, _)| name.as_str())
            .collect();
        blank.sort_unstable();
        if !missing.is_empty() {
            errors.push(format!("{place} idempotency lacks {}", missing.join(", ")));
        }
        if !unknown.is_empty() {
            errors.push(format!(
                "{place} idempotency has unknown fields {}",
                unknown.join(", ")
            ));
        }
        if !blank.is_empty() {
            errors.push(format!(
                "{place} idempotency fields must be non-empty text: {}",
                blank.join(", ")
            ));
        }
    }
    errors
}

fn cross_reference_errors(records: Records) -> Vec<String> {
    let known = known_contexts(records);
    let mut errors = context_placement_problems(held(records, "contexts"));
    errors.extend(confirmed_absence_errors(records));
    errors.extend(context_list_errors(records, &known));
    errors.extend(single_context_errors(records, &known));
    errors.extend(contract_party_errors(records, &known));
    errors.extend(event_consumer_errors(records, &known));
    errors.extend(contract_reference_errors(records));
    errors.extend(aggregate_binding_errors(records));
    errors.extend(closed_field_errors(records));
    errors.extend(term_label_errors(held(records, "vocabulary")));
    errors.extend(idempotency_errors(held(records, "contracts")));
    errors
}

fn evidence_errors(records: Records, repo_root: &Path) -> Res<Vec<String>> {
    let mut by_asset = Object::new();
    for loaded in records {
        by_asset.insert(
            loaded.asset,
            Value::Array(loaded.records.iter().cloned().map(Value::Object).collect()),
        );
    }
    let mut verifier = Verifier::new(repo_root, false);
    let mut errors = Vec::new();
    for reference in all_references(&Value::Object(by_asset), 0)? {
        let result = verifier.check(&reference)?;
        let status = result
            .get("status")
            .and_then(Value::as_str)
            .unwrap_or_default();
        if matches!(status, "invalid" | "missing" | "unhashed") {
            let detail = result.get("reason").or_else(|| result.get("path"));
            errors.push(format!(
                "evidence {status}: {}",
                detail.map_or_else(|| "None".to_string(), str_of)
            ));
        }
    }
    Ok(errors)
}

fn first_sentence(prose: Option<&Value>) -> String {
    let Some(Value::Str(prose)) = prose else {
        return String::new();
    };
    let first_line = prose.split('\n').next().unwrap_or_default();
    let sentence = first_line
        .split(". ")
        .next()
        .unwrap_or_default()
        .trim_end_matches('.');
    let sentence = if sentence.chars().count() > INDEX_DESCRIPTION_LIMIT {
        let cut: String = sentence.chars().take(INDEX_DESCRIPTION_LIMIT - 1).collect();
        format!("{}…", cut.trim_end_matches(py::is_space))
    } else {
        sentence.to_string()
    };
    format!("{sentence}.")
}

fn asset_index(asset: &str, records: &[Object]) -> String {
    let heading = HEADINGS
        .iter()
        .find(|(name, _)| *name == asset)
        .map_or("", |(_, heading)| *heading);
    let mut lines = vec![format!("# {heading}"), String::new()];
    let mut ordered: Vec<&Object> = records.iter().collect();
    ordered.sort_by_key(|entry| id_text(entry));
    for record in ordered {
        let id = id_text(record);
        let label = title_field(asset)
            .and_then(|field| record.get(field))
            .map_or_else(|| id.clone(), str_of);
        let description = first_sentence(prose_field(asset).and_then(|field| record.get(field)));
        let link = format!("- [{label}](/{asset}/{id})");
        lines.push(if description.is_empty() {
            link
        } else {
            format!("{link} — {description}")
        });
    }
    format!("{}\n", lines.join("\n"))
}

fn root_index() -> String {
    let mut lines = vec![
        "---".to_string(),
        "okf_version: \"0.2\"".to_string(),
        "---".to_string(),
        "# Domain Registry".to_string(),
        String::new(),
    ];
    for (asset, heading) in HEADINGS {
        lines.push(format!("- [{heading}](/{asset}/index.md)"));
    }
    format!("{}\n", lines.join("\n"))
}

fn index_is_stale(path: &Path, expected: &str) -> Res<bool> {
    if !path.is_file() {
        return Ok(true);
    }
    let data = std::fs::read(path).map_err(|error| os_failure(&error, path))?;
    Ok(py::utf8_text(&data)? != expected)
}

fn stale_indexes(root: &Path, records: Records) -> Res<Vec<PathBuf>> {
    let mut stale = Vec::new();
    let top = registry_dir(root).join(INDEX);
    if index_is_stale(&top, &root_index())? {
        stale.push(top);
    }
    for (asset, _, _) in ASSETS {
        let directory = registry_dir(root).join(asset);
        let Some(loaded) = records.iter().find(|loaded| loaded.asset == *asset) else {
            continue;
        };
        if !directory.is_dir() {
            continue;
        }
        let path = directory.join(INDEX);
        if index_is_stale(&path, &asset_index(asset, &loaded.records))? {
            stale.push(path);
        }
    }
    Ok(stale)
}

fn owned_directory(implementation_path: &str) -> String {
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

fn directory_of(path: &str) -> String {
    format!(
        "/{}/",
        path.rsplit_once('/').map_or("", |(directory, _)| directory)
    )
}

fn implementation_path_errors(contexts: &[Object], listing: &mut Listing) -> Res<Vec<String>> {
    let placed: Vec<(&Object, String)> = contexts
        .iter()
        .filter_map(|entry| match entry.get("implementation_path") {
            Some(Value::Str(path)) if !py::strip(path).is_empty() => Some((entry, path.clone())),
            _ => None,
        })
        .collect();
    if placed.is_empty() {
        return Ok(Vec::new());
    }
    let directories: HashSet<String> = listing
        .files()?
        .names
        .iter()
        .map(|name| directory_of(name))
        .collect();
    Ok(placed
        .iter()
        .filter(|(entry, path)| {
            let owned = owned_directory(path);
            let reviewed = entry.get("status").and_then(Value::as_str) == Some("reviewed");
            reviewed && !directories.iter().any(|directory| directory.contains(&owned))
        })
        .map(|(entry, path)| {
            format!(
                "contexts/{} implementation_path {path} lies over no file in the repository; after a move, relocate-evidence --from <old repository path> --to <new> proposes the new path, or correct it",
                id_text(entry)
            )
        })
        .collect())
}

pub struct Validated {
    pub errors: Vec<String>,
    pub counts: Option<[usize; 5]>,
}

pub fn validate(root: &Path, repo_root: &Path, listing: &mut Listing) -> Res<Validated> {
    let refused = |errors: Vec<String>| Validated {
        errors,
        counts: None,
    };
    if !has_registry(root) {
        let message = format!(
            "no Domain Registry at {}: it holds no registry/{INDEX}; {}",
            shown_path(root, Some(repo_root)),
            domain_memory_hint(root, Some(repo_root))?
        );
        return Ok(refused(vec![message]));
    }
    let errors = conflict_marker_errors(root)?;
    if !errors.is_empty() {
        return Ok(refused(errors));
    }
    let policy_errors = match existing_policy(root) {
        Err(message) => vec![message],
        Ok(None) => Vec::new(),
        Ok(Some(policy)) => validate_policy(&policy),
    };
    if !policy_errors.is_empty() {
        return Ok(refused(policy_errors));
    }
    let registry = Registry::open(root).map_err(Fail::Os)?;
    let mut errors = Vec::new();
    let mut loaded: Vec<Loaded> = Vec::new();
    for (asset, _, _) in ASSETS {
        match registry.records(asset) {
            Err(message) => errors.push(message),
            Ok(records) => loaded.push(Loaded {
                asset,
                records: records
                    .into_iter()
                    .filter_map(|record| match record {
                        Value::Object(object) => Some(object),
                        _ => None,
                    })
                    .collect(),
            }),
        }
    }
    for set in &loaded {
        for entry in &set.records {
            errors.extend(entry_status_errors(set.asset, entry));
        }
    }
    errors.extend(cross_reference_errors(&loaded));
    errors.extend(evidence_errors(&loaded, repo_root)?);
    for path in stale_indexes(root, &loaded)? {
        errors.push(format!(
            "{} does not list the records beside it; run refresh-indexes --registry-root {}",
            shown_path(&path, Some(root)),
            shown_path(root, None)
        ));
    }
    if loaded.iter().any(|set| set.asset == "contexts") {
        errors.extend(implementation_path_errors(
            held(&loaded, "contexts"),
            listing,
        )?);
    }
    let count = |asset: &str| held(&loaded, asset).len();
    Ok(Validated {
        errors,
        counts: Some([
            count("contexts"),
            count("vocabulary"),
            count("aggregates"),
            count("rules"),
            count("contracts"),
        ]),
    })
}
