use std::path::PathBuf;

use crate::files::load_json;
use crate::harvest_run::{discard_harvest, harvest, package_root_of};

use crate::cite::OUTSIDE_SOURCES_CONSEQUENCE;
use crate::cli::Invocation;
use crate::harvest::harvest_candidates;
use crate::json::{Object, Value};
use crate::py;
use crate::{emit_stderr, print_json};

fn resolved(invocation: &Invocation, dest: &str) -> PathBuf {
    py::resolve(invocation.text(dest).unwrap_or("."))
}

fn pairs(entries: &[String], option: &str, shape: &str) -> Result<Vec<(String, String)>, String> {
    let mut found: Vec<(String, String)> = Vec::new();
    for entry in entries {
        let (left, right) = entry.split_once('=').unwrap_or((entry, ""));
        let (left, right) = (py::strip(left), py::strip(right));
        if !entry.contains('=') || left.is_empty() || right.is_empty() {
            return Err(format!("{option} takes {shape}, got {entry}"));
        }
        match found.iter_mut().find(|(seen, _)| seen == left) {
            Some(slot) => slot.1 = right.to_string(),
            None => found.push((left.to_string(), right.to_string())),
        }
    }
    Ok(found)
}

fn listed(report: &Object, key: &str) -> Vec<String> {
    report
        .get(key)
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_str)
        .map(str::to_string)
        .collect()
}

pub fn harvest_candidates_command(invocation: &Invocation) -> Result<i32, String> {
    let context_map = pairs(
        invocation.texts("context_map"),
        "--context-map",
        "CANDIDATE=GOLD",
    )?;
    let report = harvest_candidates(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
        &resolved(invocation, "from_root"),
        &context_map,
        invocation.flag("keep_duplicates"),
    )?;
    print_json(&Value::Object(report.clone()));
    let collisions = listed(&report, "context_id_collisions");
    if !collisions.is_empty() {
        emit_stderr(&format!(
            "NOTE: {} pair with the Context of the same id here only by that id, not by implementation_path, so their records attach to it; pass --context-map if they are another.\n",
            collisions.join(", ")
        ));
    }
    let duplicates = listed(&report, "possible_duplicates");
    if !duplicates.is_empty() {
        emit_stderr(&format!(
            "NOTE: some harvested records cite within the evidence of {}; check that they are not duplicates.\n",
            duplicates.join(", ")
        ));
    }
    let outside = listed(&report, "outside_sources");
    if !outside.is_empty() {
        emit_stderr(&format!(
            "NOTE: harvested records rest on files the confirmed source map does not cover: {}. {OUTSIDE_SOURCES_CONSEQUENCE}\n",
            outside.join(", ")
        ));
    }
    Ok(0)
}

fn given(text: &str) -> PathBuf {
    PathBuf::from(py::PurePath::new(text).native())
}

pub fn harvest_command(invocation: &Invocation) -> Result<i32, String> {
    let spec = load_json(&given(invocation.text("spec").unwrap_or_default()))?;
    let changes_root = invocation.text("changes_root").map(py::resolve);
    let report = harvest(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
        &spec,
        changes_root.as_deref(),
        invocation.text("through").unwrap_or("draft"),
    )?;
    let failed = report
        .get("errors")
        .and_then(Value::as_array)
        .is_some_and(|errors| !errors.is_empty());
    print_json(&Value::Object(report));
    Ok(i32::from(failed))
}

pub fn discard_harvest_command(invocation: &Invocation) -> Result<i32, String> {
    let (package, spec) = (invocation.text("package_root"), invocation.text("spec"));
    if package.is_none() == spec.is_none() {
        return Err("discard-harvest takes --package-root or --spec, one of them".to_string());
    }
    let registry_root = given(invocation.text("registry_root").unwrap_or_default());
    let package_root = match package {
        Some(text) => given(text),
        None => package_root_of(
            &load_json(&given(spec.unwrap_or_default()))?,
            &registry_root,
            invocation.text("changes_root").map(given).as_deref(),
        )?,
    };
    let event = discard_harvest(
        &package_root,
        &registry_root,
        &given(invocation.text("repo_root").unwrap_or_default()),
        invocation.text("reason").unwrap_or_default(),
    )?;
    print_json(&Value::Object(event));
    Ok(0)
}
