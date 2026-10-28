use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

use crate::audit_append::append;
use crate::difflib::close_matches;
use crate::files::{load_json, write_json};
use crate::json::{self, Object, Value};
use crate::py::{self, Raised};
use crate::pyvalue::{kind_of, repr_of};
use crate::transaction::with_writer_lock;

pub const POLICY_FILE: &str = "domain-memory-policy.json";
const REVIEW_MODES: &[&str] = &["local-draft-only", "scm-verified"];
const ENUMERATED_FIELDS: &[(&str, &[&str])] = &[
    ("format", &["domain-memory-policy/v1"]),
    ("storage_mode", &["tracked", "ignored", "external"]),
    (
        "data_classification",
        &["public", "internal", "confidential", "restricted"],
    ),
    ("review_mode", REVIEW_MODES),
];
const GOVERNANCE_VERIFIERS: &[&str] = &["git-signed-commit", "none"];
const GOVERNANCE_TRIGGERS: &[&str] = &["git-commit", "git-push", "none"];
const CI_REQUIREMENTS: &[&str] = &["required", "optional", "none"];
const REMOVED_VERIFIER: &str = "policy names the github-pr verifier, which the plugin no longer provides: it verifies git signatures only; \
     amend-policy --set review_verifier=git-signed-commit --set review_trigger=git-push \
     --set authorized_signers=<fingerprint,...> --reason <text>";
const LIMIT_KEYS: &[&str] = &[
    "max_file_count",
    "max_file_bytes",
    "max_total_bytes",
    "max_query_results",
];
const PROFILE_KEYS: &[&str] = &[
    "implementation_roots",
    "excluded_directories",
    "decision_roots",
    "boundary_markers",
];
const BARE_NAME_KEYS: &[&str] = &["excluded_directories", "boundary_markers"];

const DEFAULT_QUERY_RESULTS: usize = 100;

pub fn policy_path(root: &Path) -> PathBuf {
    root.join(POLICY_FILE)
}

pub fn existing_policy(root: &Path) -> Result<Option<Object>, String> {
    let path = policy_path(root);
    if path.is_file() {
        return load_json(&path).map(Some);
    }
    Ok(None)
}

fn valid_policy_if_any(root: &Path) -> Result<Option<Object>, String> {
    let path = policy_path(root);
    if !path.is_file() {
        return Ok(None);
    }
    valid_policy(&path).map(Some)
}

fn valid_policy(path: &Path) -> Result<Object, String> {
    let policy = load_json(path)?;
    let errors = validate_policy(&policy);
    if !errors.is_empty() {
        return Err(format!(
            "invalid Domain Memory policy: {}",
            errors.join("; ")
        ));
    }
    Ok(policy)
}

pub fn review_governance(root: &Path) -> Result<Object, String> {
    let policy = valid_policy(&policy_path(root))?;
    Ok(policy
        .get("review_governance")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default())
}

pub fn approved_command_profiles(root: Option<&Path>) -> Result<BTreeSet<String>, String> {
    let Some(policy) = root.map(valid_policy_if_any).transpose()?.flatten() else {
        return Ok(BTreeSet::new());
    };
    let named: Vec<String> = match policy.get("approved_command_profiles") {
        None => Vec::new(),
        Some(Value::Array(profiles)) => profiles
            .iter()
            .filter_map(Value::as_str)
            .map(str::to_string)
            .collect(),
        Some(Value::Object(profiles)) => profiles.iter().map(|(key, _)| key.clone()).collect(),
        Some(Value::Str(text)) => text.chars().map(String::from).collect(),
        Some(other) => {
            return Err(format!(
                "'{}' object is not iterable",
                crate::audit_log::type_name(other)
            ));
        }
    };
    Ok(named
        .into_iter()
        .filter(|profile| !py::strip(profile).is_empty())
        .collect())
}

pub fn review_mode(root: &Path) -> Result<String, String> {
    Ok(valid_policy_if_any(root)?.map_or_else(
        || "no-policy".to_string(),
        |policy| text(&policy, "review_mode").unwrap_or_default().to_string(),
    ))
}

pub fn query_result_limit(root: &Path) -> Result<usize, String> {
    let Some(policy) = valid_policy_if_any(root)? else {
        return Ok(DEFAULT_QUERY_RESULTS);
    };
    let limit = policy
        .get("limits")
        .and_then(Value::as_object)
        .and_then(|limits| limits.get("max_query_results"));
    Ok(match limit {
        Some(Value::Int(digits)) => digits.parse().unwrap_or(usize::MAX),
        _ => DEFAULT_QUERY_RESULTS,
    })
}

const POLICY_KEYS: &[&str] = &[
    "format",
    "storage_mode",
    "data_classification",
    "review_mode",
    "review_governance",
    "source_policy",
    "limits",
    "project_profile",
    "approved_command_profiles",
];

pub fn validate_policy(policy: &Object) -> Vec<String> {
    let mut errors = Vec::new();
    for (key, allowed) in ENUMERATED_FIELDS {
        let value = policy.get(key).unwrap_or(&Value::Null);
        if !value.as_str().is_some_and(|text| allowed.contains(&text)) {
            let mut sorted = allowed.to_vec();
            sorted.sort_unstable();
            errors.push(format!(
                "policy has an invalid {key}: {} is not one of {}",
                repr_of(value),
                sorted.join(", ")
            ));
        }
    }
    let review_mode = policy.get("review_mode").unwrap_or(&Value::Null);
    errors.extend(governance_error(
        review_mode,
        policy.get("review_governance"),
    ));
    errors.extend(source_error(policy.get("source_policy")));
    errors.extend(limits_error(policy.get("limits")));
    errors.extend(profile_errors(policy.get("project_profile")));
    errors.extend(unknown_keys("policy", policy, POLICY_KEYS));
    errors
}

fn text<'a>(object: &'a Object, key: &str) -> Option<&'a str> {
    object.get(key).and_then(Value::as_str)
}

fn one_of(object: &Object, key: &str, allowed: &[&str]) -> bool {
    text(object, key).is_some_and(|value| allowed.contains(&value))
}

fn nonblank_strings(value: Option<&Value>) -> bool {
    value.and_then(Value::as_array).is_some_and(|items| {
        items.iter().all(|item| {
            item.as_str()
                .is_some_and(|text| !py::strip(text).is_empty())
        })
    })
}

fn non_empty_list(value: Option<&Value>) -> bool {
    value
        .and_then(Value::as_array)
        .is_some_and(|items| !items.is_empty())
}

fn governance_error(review_mode: &Value, governance: Option<&Value>) -> Option<String> {
    let Some(governance) = governance.and_then(Value::as_object) else {
        return Some("policy requires review_governance".to_string());
    };
    if text(governance, "verifier") == Some("github-pr")
        || text(governance, "trigger") == Some("external-scm")
    {
        return Some(REMOVED_VERIFIER.to_string());
    }
    if let Some(distinct) = governance.get("distinct_signer")
        && !matches!(distinct, Value::Bool(_))
    {
        return Some(format!(
            "review_governance.distinct_signer must be true or false, got {}",
            repr_of(distinct)
        ));
    }
    if !one_of(governance, "verifier", GOVERNANCE_VERIFIERS)
        || !one_of(governance, "trigger", GOVERNANCE_TRIGGERS)
        || !one_of(governance, "ci_requirement", CI_REQUIREMENTS)
        || !nonblank_strings(governance.get("authorized_signers"))
    {
        return Some("policy has invalid review_governance".to_string());
    }
    let mode = review_mode.as_str();
    let verifier = text(governance, "verifier");
    let rules = [
        (
            mode == Some("local-draft-only") && verifier != Some("none"),
            "local-draft-only policy must not select a review verifier",
        ),
        (
            mode == Some("scm-verified") && verifier == Some("none"),
            "scm-verified policy requires a review verifier",
        ),
        (
            verifier == Some("git-signed-commit")
                && !non_empty_list(governance.get("authorized_signers")),
            "git-signed-commit policy requires authorized_signers",
        ),
        (
            verifier == Some("git-signed-commit")
                && text(governance, "trigger") != Some("git-push"),
            "git-signed-commit policy requires git-push enforcement",
        ),
    ];
    rules
        .iter()
        .find(|(broken, _)| *broken)
        .map(|(_, message)| (*message).to_string())
}

fn source_error(source: Option<&Value>) -> Option<String> {
    let Some(source) = source.and_then(Value::as_object) else {
        return Some("policy requires selected source paths and authority".to_string());
    };
    let authority_ok =
        text(source, "authority").is_some_and(|authority| !py::strip(authority).is_empty());
    if !nonblank_strings(source.get("selected_paths"))
        || !non_empty_list(source.get("selected_paths"))
        || !authority_ok
    {
        return Some("policy requires selected source paths and authority".to_string());
    }
    if !nonblank_strings(source.get("include"))
        || !non_empty_list(source.get("include"))
        || !nonblank_strings(source.get("exclude"))
    {
        return Some("policy requires include patterns and valid exclude patterns".to_string());
    }
    None
}

fn positive_whole(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Int(digits)) => !digits.starts_with('-') && digits != "0",
        _ => false,
    }
}

fn limits_error(limits: Option<&Value>) -> Option<String> {
    let valid = limits
        .and_then(Value::as_object)
        .is_some_and(|limits| LIMIT_KEYS.iter().all(|key| positive_whole(limits.get(key))));
    (!valid).then(|| "policy has invalid limits".to_string())
}

fn closest(key: &str, known: &[&str]) -> String {
    close_matches(key, known, 1)
        .first()
        .map(|close| format!("; did you mean {close}?"))
        .unwrap_or_default()
}

fn list_errors(name: &str, items: &Value) -> Vec<String> {
    let Some(items) = items.as_array() else {
        return vec![format!("{name} must be a list, got {}", kind_of(items))];
    };
    items
        .iter()
        .enumerate()
        .filter_map(|(index, item)| match item {
            Value::Str(text) if !py::strip(text).is_empty() => None,
            Value::Str(_) => Some(format!("{name}[{index}] is blank")),
            other => Some(format!(
                "{name}[{index}] must be a string, got {}",
                kind_of(other)
            )),
        })
        .collect()
}

fn unknown_keys(section: &str, value: &Object, known: &[&str]) -> Vec<String> {
    value
        .iter()
        .filter(|(key, _)| !known.contains(&key.as_str()))
        .map(|(key, _)| format!("{section}.{key} is not a known key{}", closest(key, known)))
        .collect()
}

pub fn profile_errors(value: Option<&Value>) -> Vec<String> {
    let value = match value {
        None | Some(Value::Null) => return Vec::new(),
        Some(Value::Object(value)) => value,
        Some(other) => {
            return vec![format!(
                "project_profile must be an object, got {}",
                kind_of(other)
            )];
        }
    };
    let mut errors = unknown_keys("project_profile", value, PROFILE_KEYS);
    for key in PROFILE_KEYS {
        let Some(items) = value.get(key) else {
            continue;
        };
        let listed = list_errors(&format!("project_profile.{key}"), items);
        if listed.is_empty() {
            errors.extend(entry_errors(key, items.as_array().unwrap_or_default()));
        } else {
            errors.extend(listed);
        }
    }
    errors
}

fn entry_errors(key: &str, items: &[Value]) -> Vec<String> {
    items
        .iter()
        .enumerate()
        .filter_map(|(index, item)| {
            let item = item.as_str()?;
            if BARE_NAME_KEYS.contains(&key) {
                (item.contains('/') || item.contains('\\')).then(|| {
                    format!(
                        "project_profile.{key}[{index}] must be a bare name without a path separator, got {}",
                        py::repr(item)
                    )
                })
            } else {
                let escapes = item.starts_with(['/', '\\'])
                    || item.replace('\\', "/").split('/').any(|part| part == "..")
                    || item.contains(':');
                escapes.then(|| {
                    format!(
                        "project_profile.{key}[{index}] must be a relative path inside the repository, got {}",
                        py::repr(item)
                    )
                })
            }
        })
        .collect()
}

const POLICY_TEMPLATE: &str = include_str!("../../templates/domain-memory-policy.json");

pub struct NewPolicy<'a> {
    pub storage_mode: &'a str,
    pub data_classification: &'a str,
    pub review_mode: &'a str,
    pub authority: &'a str,
    pub include: &'a [String],
    pub exclude: &'a [String],
    pub governance: Object,
    pub profile: Value,
}

fn strings(items: &[String]) -> Value {
    Value::Array(items.iter().map(|item| Value::Str(item.clone())).collect())
}

pub fn write_policy(root: &Path, selected_paths: &Value, new: NewPolicy) -> Result<(), Raised> {
    let Ok(Value::Object(mut value)) = json::parse(POLICY_TEMPLATE) else {
        return Err(Raised::Value("the policy template is not JSON".to_string()));
    };
    let text = |item: &str| Value::Str(item.to_string());
    value.insert("storage_mode", text(new.storage_mode));
    value.insert("data_classification", text(new.data_classification));
    value.insert("review_mode", text(new.review_mode));
    value.insert("review_governance", Value::Object(new.governance));
    match value.get_mut("source_policy") {
        Some(Value::Object(section)) => {
            section.insert("selected_paths", selected_paths.clone());
            section.insert("authority", text(new.authority));
            section.insert("include", strings(new.include));
            section.insert("exclude", strings(new.exclude));
        }
        _ => {
            return Err(Raised::Value(
                "the policy template has no source_policy".to_string(),
            ));
        }
    }
    value.insert("project_profile", new.profile);
    let errors = validate_policy(&value);
    if !errors.is_empty() {
        return Err(Raised::Value(format!(
            "invalid policy: {}",
            errors.join("; ")
        )));
    }
    write_json(&policy_path(root), &Value::Object(value)).map_err(Raised::Os)
}

const AMENDABLE_FIELDS: &[(&str, &[&str])] = &[
    ("review_mode", &["review_mode"]),
    ("storage_mode", &["storage_mode"]),
    ("data_classification", &["data_classification"]),
    ("source_authority", &["source_policy", "authority"]),
    ("review_verifier", &["review_governance", "verifier"]),
    ("review_trigger", &["review_governance", "trigger"]),
    ("ci_requirement", &["review_governance", "ci_requirement"]),
    (
        "authorized_signers",
        &["review_governance", "authorized_signers"],
    ),
    ("distinct_signer", &["review_governance", "distinct_signer"]),
    (
        "implementation_roots",
        &["project_profile", "implementation_roots"],
    ),
    (
        "excluded_directories",
        &["project_profile", "excluded_directories"],
    ),
    ("decision_roots", &["project_profile", "decision_roots"]),
    ("boundary_markers", &["project_profile", "boundary_markers"]),
];
const LIST_FIELDS: &[&str] = &[
    "authorized_signers",
    "implementation_roots",
    "excluded_directories",
    "decision_roots",
    "boundary_markers",
];
const NO_VERIFIER: &str = "leaving local-draft-only requires the verifier that replaces it, one of git-signed-commit: a review mode and the authority that backs it move together";

fn field_path(field: &str) -> Option<&'static [&'static str]> {
    AMENDABLE_FIELDS
        .iter()
        .find(|(name, _)| *name == field)
        .map(|(_, path)| *path)
}

fn amended_value(field: &str, value: &str) -> Result<Value, String> {
    if LIST_FIELDS.contains(&field) {
        return Ok(Value::Array(
            value
                .split(',')
                .map(py::strip)
                .filter(|entry| !entry.is_empty())
                .map(|entry| Value::Str(entry.to_string()))
                .collect(),
        ));
    }
    if field == "distinct_signer" {
        return match py::strip(value).to_lowercase().as_str() {
            "true" => Ok(Value::Bool(true)),
            "false" => Ok(Value::Bool(false)),
            _ => Err(format!(
                "{field} takes true or false, got {}",
                py::repr(value)
            )),
        };
    }
    Ok(Value::Str(value.to_string()))
}

fn field_parent<'a>(document: &'a mut Object, field: &str) -> Result<&'a mut Object, String> {
    let path = field_path(field).unwrap_or_default();
    let mut target = document;
    for key in &path[..path.len().saturating_sub(1)] {
        if target.get(key).is_none() {
            target.insert(*key, Value::Object(Object::new()));
        }
        let Some(Value::Object(inner)) = target.get_mut(key) else {
            return Err(format!("policy {key} is not an object"));
        };
        target = inner;
    }
    Ok(target)
}

fn default_governance(review_mode: &str) -> Object {
    let local = review_mode == "local-draft-only";
    let mut governance = Object::new();
    governance.insert(
        "verifier",
        Value::Str(if local { "none" } else { "git-signed-commit" }.to_string()),
    );
    governance.insert(
        "trigger",
        Value::Str(if local { "none" } else { "git-commit" }.to_string()),
    );
    governance.insert("ci_requirement", Value::Str("none".to_string()));
    governance.insert("authorized_signers", Value::Array(Vec::new()));
    governance
}

fn apply_field(
    document: &mut Object,
    field: &str,
    value: &str,
    verifier: Option<&str>,
) -> Result<Object, String> {
    let key = field_path(field)
        .and_then(|path| path.last())
        .copied()
        .unwrap_or_default();
    let parent = field_parent(document, field)?;
    let amended = amended_value(field, value)?;
    let mut change = Object::new();
    change.insert("operation", Value::Str("amend-policy".to_string()));
    change.insert("field", Value::Str(field.to_string()));
    change.insert("to", amended.clone());
    parent.insert(key, amended);
    if field == "review_mode" && value == "local-draft-only" {
        document.insert(
            "review_governance",
            Value::Object(default_governance(value)),
        );
    } else if field == "review_mode" && value == "scm-verified" {
        let replacement = verifier
            .map(py::strip)
            .filter(|verifier| !verifier.is_empty())
            .ok_or_else(|| NO_VERIFIER.to_string())?;
        match document.get_mut("review_governance") {
            Some(Value::Object(governance)) => {
                governance.insert("verifier", Value::Str(replacement.to_string()));
            }
            _ => return Err("policy review_governance is not an object".to_string()),
        }
        change.insert("verifier", Value::Str(replacement.to_string()));
    }
    Ok(change)
}

fn refused_fields(assignments: &[(String, String)]) -> Option<String> {
    let fields: Vec<&str> = assignments
        .iter()
        .map(|(field, _)| field.as_str())
        .collect();
    let unknown: Vec<&str> = fields
        .iter()
        .copied()
        .filter(|field| field_path(field).is_none())
        .collect();
    if assignments.is_empty() || !unknown.is_empty() {
        let mut amendable: Vec<&str> = AMENDABLE_FIELDS.iter().map(|(name, _)| *name).collect();
        amendable.sort_unstable();
        let named = if unknown.is_empty() {
            "nothing".to_string()
        } else {
            unknown.join(", ")
        };
        return Some(format!(
            "{named} is not an amendable policy field; amend one of {}. \
             Selected source paths and limits are settled when a Domain Memory is initialized.",
            amendable.join(", ")
        ));
    }
    let mut repeated: Vec<&str> = fields
        .iter()
        .copied()
        .filter(|field| fields.iter().filter(|other| *other == field).count() > 1)
        .collect();
    repeated.sort_unstable();
    repeated.dedup();
    (!repeated.is_empty()).then(|| {
        format!(
            "{} is named more than once; give each field one value",
            repeated.join(", ")
        )
    })
}

pub fn amend_policy_fields(
    root: &Path,
    assignments: &[(String, String)],
    reason: &str,
    verifier: Option<&str>,
) -> Result<Vec<Object>, String> {
    if let Some(refusal) = refused_fields(assignments) {
        return Err(refusal);
    }
    let reason = py::strip(reason);
    if reason.is_empty() {
        return Err("amending a Domain Memory policy requires a reason: the policy decides what the Registry may become".to_string());
    }
    let path = policy_path(root);
    if !path.is_file() {
        return Err(format!("no Domain Memory policy at {}", py::shown(&path)));
    }
    let mut document = load_json(&path)?;
    let mut before = load_json(&path)?;
    let named_verifier = assignments
        .iter()
        .find(|(field, _)| field == "review_verifier")
        .map(|(_, value)| value.as_str());
    let verifier = verifier
        .filter(|given| !given.is_empty())
        .or(named_verifier);
    let mut ordered: Vec<&(String, String)> = assignments.iter().collect();
    ordered.sort_by_key(|(field, _)| field != "review_mode");
    let mut changes = Vec::new();
    for (field, value) in ordered {
        let mut change = apply_field(&mut document, field, value, verifier)?;
        let key = field_path(field)
            .and_then(|path| path.last())
            .copied()
            .unwrap_or_default();
        let from = field_parent(&mut before, field)?
            .get(key)
            .cloned()
            .unwrap_or(Value::Null);
        change.insert("from", from);
        change.insert("reason", Value::Str(reason.to_string()));
        changes.push(change);
    }
    let errors = validate_policy(&document);
    if !errors.is_empty() {
        return Err(format!("amended policy is invalid: {}", errors.join("; ")));
    }
    with_writer_lock(root, || {
        write_json(&path, &Value::Object(document))?;
        changes.iter().try_for_each(|change| append(root, change))
    })?;
    Ok(changes)
}
