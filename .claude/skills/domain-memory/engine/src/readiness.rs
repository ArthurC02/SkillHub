use std::path::{Path, PathBuf};

use crate::files::{load_json, one_level_too_deep_hint};
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::py;
use crate::registry_check::validate;
use crate::source_probe::{
    Discovered, Fail, Listed, Listing, Res, counted, discover_sources, domain_memories_in,
    existing_policy, joined_given, listing_drift, project_profile, refresh_listing_advice,
    shown_path, verify_source_map,
};

const PRODUCT_GROUPS: [&str; 6] = [
    "decisions",
    "requirements",
    "contracts",
    "bounded_contexts",
    "implementation",
    "tests",
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

fn registry_condition(
    repo_root: &Path,
    registry_path: &Path,
    listing: &mut Listing,
) -> Res<(String, usize, Vec<String>)> {
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
        .map_err(Fail::Value)
        .and_then(|policy| verify_source_map(repo_root, &source_map, policy.as_ref()));
    let source_result = match checked {
        Err(Fail::Value(error)) => {
            return Ok((
                "invalid".to_string(),
                0,
                vec![format!("Domain Memory source map cannot be read: {error}")],
            ));
        }
        other => other?,
    };
    if source_result.status != "current" {
        return Ok((
            source_result.status,
            0,
            vec![
                "Domain Memory sources require review before they can guide implementation."
                    .to_string(),
            ],
        ));
    }
    let checked = validate(registry_path, repo_root, listing)?;
    if !checked.errors.is_empty() {
        let shown: Vec<String> = checked.errors.iter().take(ERRORS_SHOWN).cloned().collect();
        return Ok((
            "invalid".to_string(),
            0,
            vec![format!(
                "Domain Memory Registry is invalid: {}",
                shown.join("; ")
            )],
        ));
    }
    let records = checked.counts.map_or(0, |counts| counts.iter().sum());
    Ok(("current".to_string(), records, Vec::new()))
}

fn listing_signal(discovered: &Discovered, registry_root: &Path) -> Res<Value> {
    let source_map = load_json(&registry_root.join("source-map.json"))?;
    let (gone, unlisted) = listing_drift(discovered, &source_map);
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

fn governance(ci_tools: &[String], registry_root: Option<&Path>) -> Value {
    let policy = readable_policy(registry_root);
    let configured = policy
        .as_ref()
        .and_then(|found| found.get("review_governance"))
        .and_then(Value::as_object)
        .filter(|rules| rules.get("verifier").is_some_and(truthy));
    let mut found = Object::new();
    found.insert("ci_tools", names(ci_tools));
    match configured {
        Some(rules) => {
            found.insert(
                "configured_verifier",
                rules.get("verifier").cloned().unwrap_or(Value::Null),
            );
            found.insert(
                "configured_trigger",
                rules.get("trigger").cloned().unwrap_or(Value::Null),
            );
        }
        None => {
            found.insert("recommended_verifier", text("git-signed-commit"));
            found.insert("recommended_trigger", text("git-push"));
        }
    }
    Value::Object(found)
}

fn assess(repo_root: &Path, registry_root: Option<&Path>) -> Res<Object> {
    let policy = readable_policy(registry_root);
    let profile = project_profile(policy.as_ref());
    let mut listing = Listing::default_for(repo_root);
    let discovered = if profile.excluded_directories.is_empty() {
        discover_sources(repo_root, &profile, Some(&mut listing))?
    } else {
        discover_sources(repo_root, &profile, None)?
    };
    let present = |kind: &str| {
        discovered
            .groups
            .iter()
            .any(|(group, paths)| *group == kind && !paths.is_empty())
    };
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
        governance(&discovered.ci_tools, registry_root),
    );
    report.insert("blocks", names(&blocks));
    report.insert("next_capability", text(next_capability));
    Ok(report)
}

pub fn readiness_report(repo_root: &Path, registry_root: Option<&Path>) -> Res<Readiness> {
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
    let drifted = report
        .get("signals")?
        .as_array()?
        .iter()
        .find_map(|signal| {
            let signal = signal.as_object()?;
            (signal.get("kind")?.as_str()? == "listing"
                && signal.get("status")?.as_str()? == "drifted")
                .then_some(signal)
        })?;
    let count = |key: &str| {
        drifted
            .get(key)
            .and_then(Value::as_array)
            .map_or(0, <[Value]>::len)
    };
    let (gone, unlisted) = (
        count("discovered_but_gone"),
        count("discovered_but_unlisted"),
    );
    if gone == 0 && unlisted == 0 {
        return None;
    }
    let registry = match given_registry {
        Some(given) => given.to_string(),
        None => joined_given(given_repo, report.get("registry_root")?.as_str()?),
    };
    Some(format!(
        "NOTE: discovery drifted: {} gone, {} unlisted; {}",
        counted(gone, "listed path"),
        counted(unlisted, "matching path"),
        refresh_listing_advice(&registry, given_repo)
    ))
}
