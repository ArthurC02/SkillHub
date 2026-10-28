use std::path::{Path, PathBuf};

use crate::coverage::registry_coverage;
use crate::files::{load_json, one_level_too_deep_hint};
use crate::json::{Object, Value};
use crate::listing::Repository;
use crate::okf::truthy;
use crate::policy::existing_policy;
use crate::py::{self, Raised};
use crate::sources::{
    Profile, discover_sources, discovery_drift_note, listing_drift, verify_source_map,
};
use crate::store::{Listed, domain_memories_in, shown_path};
use crate::validate::validate;

const PRODUCT_GROUPS: [&str; 6] = [
    "decisions",
    "requirements",
    "contracts",
    "bounded_contexts",
    "implementation",
    "tests",
];
const COUNTED_KINDS: [&str; 5] = [
    "contexts",
    "vocabulary_terms",
    "aggregates",
    "rules",
    "contracts",
];
const UNUSABLE_REGISTRY_STATUSES: [&str; 3] = ["invalid", "unverified", "stale"];
const ERRORS_SHOWN: usize = 3;

pub struct Readiness {
    pub report: Object,
    pub located: Option<String>,
}

fn text(value: &str) -> Value {
    Value::Str(value.to_string())
}

fn names(items: &[String]) -> Value {
    Value::Array(items.iter().map(|item| text(item)).collect())
}

fn signal(kind: &str, field: &str, value: Value) -> Value {
    let mut found = Object::new();
    found.insert("kind", text(kind));
    found.insert(field, value);
    Value::Object(found)
}

fn record_count(report: &Object) -> usize {
    COUNTED_KINDS
        .iter()
        .map(|kind| match report.get(kind) {
            Some(Value::Int(digits)) => digits.parse().unwrap_or(0),
            _ => 0,
        })
        .sum()
}

fn registry_condition(
    repo_root: &Path,
    registry_path: &Path,
    listing: &mut Repository,
) -> Result<(String, usize, Vec<String>), Raised> {
    if !registry_path.exists() {
        return Ok(("absent".to_string(), 0, Vec::new()));
    }
    let source_map = registry_path.join("source-map.json");
    if !source_map.is_file() {
        let hint = one_level_too_deep_hint(&source_map);
        return Ok((
            "unverified".to_string(),
            0,
            vec![format!("Domain Memory exists without a source map{hint}.")],
        ));
    }
    let checked = existing_policy(registry_path)
        .map_err(Raised::Value)
        .and_then(|policy| verify_source_map(repo_root, &source_map, policy.as_ref()));
    let source_result = match checked {
        Err(Raised::Value(error)) => {
            return Ok((
                "invalid".to_string(),
                0,
                vec![format!("Domain Memory source map cannot be read: {error}")],
            ));
        }
        other => other?,
    };
    let status = source_result
        .get("status")
        .and_then(Value::as_str)
        .unwrap_or_default();
    if status != "current" {
        return Ok((
            status.to_string(),
            0,
            vec![
                "Domain Memory sources require review before they can guide implementation."
                    .to_string(),
            ],
        ));
    }
    let errors = validate(registry_path, Some(listing), false)?;
    if !errors.is_empty() {
        let shown: Vec<String> = errors.iter().take(ERRORS_SHOWN).cloned().collect();
        return Ok((
            "invalid".to_string(),
            0,
            vec![format!(
                "Domain Memory Registry is invalid: {}",
                shown.join("; ")
            )],
        ));
    }
    let records = record_count(&registry_coverage(registry_path)?);
    Ok(("current".to_string(), records, Vec::new()))
}

fn listing_signal(discovered: &Object, registry_root: &Path) -> Result<Value, String> {
    let source_map = load_json(&registry_root.join("source-map.json"))?;
    let (gone, unlisted) = listing_drift(discovered, &source_map)?;
    let mut found = Object::new();
    found.insert("kind", text("listing"));
    found.insert(
        "status",
        text(if gone.is_empty() && unlisted.is_empty() {
            "current"
        } else {
            "drifted"
        }),
    );
    found.insert("discovered_but_gone", names(&gone));
    found.insert("discovered_but_unlisted", names(&unlisted));
    Ok(Value::Object(found))
}

fn readable_policy(registry_root: Option<&Path>) -> Option<Object> {
    existing_policy(&py::resolve_path(registry_root?))
        .ok()
        .flatten()
}

fn governance(candidates: Option<&Value>, registry_root: Option<&Path>) -> Value {
    let policy = readable_policy(registry_root);
    let configured = policy
        .as_ref()
        .and_then(|found| found.get("review_governance"))
        .and_then(Value::as_object)
        .filter(|rules| rules.get("verifier").is_some_and(truthy));
    let (Some(rules), Some(Value::Object(candidates))) = (configured, candidates) else {
        return candidates.cloned().unwrap_or(Value::Null);
    };
    let mut found = Object::new();
    found.insert(
        "ci_tools",
        candidates.get("ci_tools").cloned().unwrap_or(Value::Null),
    );
    found.insert(
        "configured_verifier",
        rules.get("verifier").cloned().unwrap_or(Value::Null),
    );
    found.insert(
        "configured_trigger",
        rules.get("trigger").cloned().unwrap_or(Value::Null),
    );
    Value::Object(found)
}

fn group_present(discovered: &Object, kind: &str) -> bool {
    discovered
        .get("source_groups")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .any(|group| {
            group.get("kind").and_then(Value::as_str) == Some(kind)
                && group.get("paths").is_some_and(truthy)
        })
}

fn assess(repo_root: &Path, registry_root: Option<&Path>) -> Result<Object, Raised> {
    let policy = readable_policy(registry_root);
    let profile = Profile::of_policy(policy.as_ref());
    let mut listing = Repository::new(repo_root);
    let shared = (!profile.has_excluded_directories()).then_some(&mut listing);
    let discovered = discover_sources(repo_root, &profile, shared)?;
    let present = |kind: &str| group_present(&discovered, kind);
    let product_sources = PRODUCT_GROUPS.iter().any(|kind| present(kind));
    let mut signals: Vec<Value> = PRODUCT_GROUPS
        .iter()
        .map(|kind| {
            signal(
                kind,
                "status",
                text(if present(kind) { "present" } else { "missing" }),
            )
        })
        .collect();
    let mut blocks: Vec<String> = Vec::new();
    let mut registry_status = "absent".to_string();
    let mut registry_records = 0;
    if let Some(root) = registry_root {
        let resolved = py::resolve_path(root);
        (registry_status, registry_records, blocks) =
            registry_condition(repo_root, &resolved, &mut listing)?;
        if registry_status != "absent"
            && !UNUSABLE_REGISTRY_STATUSES.contains(&registry_status.as_str())
        {
            signals.push(listing_signal(&discovered, &resolved)?);
        }
        signals.push(signal("registry", "status", text(&registry_status)));
        signals.push(signal(
            "registry_records",
            "count",
            Value::int(registry_records),
        ));
    }
    let unusable = UNUSABLE_REGISTRY_STATUSES.contains(&registry_status.as_str());
    let dead = registry_root.is_some() && !product_sources && unusable;
    let (state, mut next_capability) = if dead {
        ("dead", "recover-or-reconfirm")
    } else if !product_sources && registry_records == 0 {
        ("empty", "discover")
    } else if !present("implementation") {
        ("greenfield", "design")
    } else if registry_root.is_some() && registry_records == 0 {
        ("brownfield", "extract")
    } else {
        ("brownfield", "read-and-maintain")
    };
    if unusable {
        next_capability = "recover-or-reconfirm";
    }
    let mut report = Object::new();
    report.insert("format", text("domain-memory-readiness/v1"));
    report.insert("state", text(state));
    report.insert("confidence", text(if dead { "medium" } else { "high" }));
    report.insert("signals", Value::Array(signals));
    report.insert(
        "governance_candidates",
        governance(discovered.get("governance_candidates"), registry_root),
    );
    report.insert("blocks", names(&blocks));
    report.insert("next_capability", text(next_capability));
    Ok(report)
}

pub fn readiness_report(
    repo_root: &Path,
    registry_root: Option<&Path>,
) -> Result<Readiness, Raised> {
    let repo_root = py::resolve_path(repo_root);
    if let Some(given) = registry_root {
        let mut report = assess(&repo_root, Some(given))?;
        report.insert(
            "registry_root",
            text(&shown_path(&py::resolve_path(given), Some(&repo_root))),
        );
        return Ok(Readiness {
            report,
            located: None,
        });
    }
    match domain_memories_in(&repo_root)? {
        Listed::Refused(message) => {
            let mut report = assess(&repo_root, None)?;
            report.insert("registry_root", Value::Null);
            report.insert("domain_memories", Value::Null);
            Ok(Readiness {
                report,
                located: Some(format!(
                    "NOTE: git could not list the Domain Memories this repository holds: {message}; pass --registry-root to assess one."
                )),
            })
        }
        Listed::Places(found) => {
            let shown: Vec<String> = found
                .iter()
                .map(|place| shown_path(place, Some(&repo_root)))
                .collect();
            let chosen: Option<&PathBuf> = if found.len() == 1 {
                found.first()
            } else {
                None
            };
            let mut report = assess(&repo_root, chosen.map(PathBuf::as_path))?;
            report.insert(
                "registry_root",
                chosen.map_or(Value::Null, |_| text(&shown[0])),
            );
            report.insert("domain_memories", names(&shown));
            let located = if chosen.is_some() {
                Some(format!(
                    "NOTE: assessing the Domain Memory at {}, the one this repository holds; pass --registry-root to assess another.",
                    shown[0]
                ))
            } else if !found.is_empty() {
                Some(format!(
                    "NOTE: Domain Memories in this repository: {}; pass --registry-root to assess one.",
                    shown.join(", ")
                ))
            } else {
                None
            };
            Ok(Readiness { report, located })
        }
    }
}

pub fn drift_note(
    report: &Object,
    given_registry: Option<&str>,
    given_repo: &str,
) -> Option<String> {
    let mut drifted = report
        .get("signals")?
        .as_array()?
        .iter()
        .find_map(|signal| {
            let signal = signal.as_object()?;
            (signal.get("kind")?.as_str()? == "listing"
                && signal.get("status")?.as_str()? == "drifted")
                .then(|| signal.clone())
        })?;
    drifted.insert("status", text("discovery_drifted"));
    let repo = py::PurePath::new(given_repo);
    let registry = match given_registry {
        Some(given) => py::PurePath::new(given),
        None => repo.joined(report.get("registry_root")?.as_str()?),
    };
    discovery_drift_note(&drifted, &registry, &repo)
}
