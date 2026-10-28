use std::path::{Path, PathBuf};

use crate::difflib::close_matches;
use crate::files::load_json;
use crate::json::{Object, Value};
use crate::py;
use crate::pyvalue::{kind_of, repr_of};

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
