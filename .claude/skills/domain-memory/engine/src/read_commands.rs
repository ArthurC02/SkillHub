use std::path::Path;

use crate::cli::Invocation;
use crate::compare;
use crate::coupling::{self, Limits, Reading};
use crate::files::load_json;
use crate::json::{Object, Value};
use crate::py;
use crate::readiness;
use crate::source_probe::{self, Fail};
use crate::store::{self, Registry};

pub fn change_coupling(invocation: &Invocation) -> Result<i32, String> {
    let depth = invocation.int("depth").unwrap_or(2);
    if depth < 1 {
        return Err("change-coupling --depth must be at least 1".to_string());
    }
    let limits = Limits {
        min_revisions: invocation.int("min_revisions").unwrap_or(5),
        min_shared: invocation.int("min_shared").unwrap_or(5),
        min_degree: invocation.int("min_degree").unwrap_or(30),
        max_changeset: invocation.int("max_changeset").unwrap_or(30),
        max_modules: invocation.int("max_modules"),
    };
    let mut reading = Reading {
        since: invocation.text("since").map(str::to_string),
        exclude_subjects: invocation.texts("exclude_subject").to_vec(),
        ..Reading::default()
    };
    if let Some(root) = invocation.text("registry_root") {
        let registry = Registry::open(&py::resolve(root))?;
        reading.context_paths = coupling::contexts_with_paths(&registry)?;
        reading.collaborations = coupling::registered_collaborations(&registry)?;
    }
    let roots = match invocation.texts("root") {
        [] => vec![".".to_string()],
        given => given.to_vec(),
    };
    let repo_root = py::resolve(invocation.text("repo_root").unwrap_or("."));
    let report = coupling::change_coupling(&repo_root, &roots, depth, &limits, &reading)?;
    crate::print_json(&report);
    Ok(0)
}

fn exported(
    root: &Path,
    commit: Option<&str>,
    repo_root: Option<&str>,
    option: &str,
) -> Result<Object, String> {
    let Some(commit) = commit else {
        return store::read_documents(root);
    };
    let repo_root = repo_root.ok_or_else(|| format!("{option} needs --repo-root"))?;
    store::committed_documents(&py::resolve(repo_root), commit, root)?
        .ok_or_else(|| format!("no Domain Memory Registry at {commit}"))
}

pub fn compare_registry(invocation: &Invocation) -> Result<i32, String> {
    let gold = exported(
        &py::resolve(invocation.text("gold_root").unwrap_or(".")),
        invocation.text("gold_commit"),
        invocation.text("repo_root"),
        "compare-registry --gold-commit",
    )?;
    let candidate = exported(
        &py::resolve(invocation.text("candidate_root").unwrap_or(".")),
        None,
        None,
        "",
    )?;
    let comparison = compare::compare_registries(&gold, &candidate, invocation.texts("context"))?;
    crate::print_json(&Value::Object(comparison.report));
    if comparison.copied_matches * 2 > comparison.attempted_matches {
        crate::emit_stderr(&format!(
            "NOTE: {} of {} matches cite exactly the gold evidence; read overall_independent, which leaves them out, as the extraction's own score.\n",
            comparison.copied_matches, comparison.attempted_matches
        ));
    }
    if !comparison.without_candidate.is_empty() {
        crate::emit_stderr(&format!(
            "NOTE: no candidate Context pairs with {}, so its records cannot pair; check that the candidate root holds an extraction of that Context.\n",
            comparison.without_candidate.join(", ")
        ));
    }
    Ok(0)
}

pub fn probe(invocation: &Invocation) -> Result<i32, String> {
    let given_registry = invocation.text("registry_root").unwrap_or(".");
    let given_repo = invocation.text("repo_root").unwrap_or(".");
    let registry_root = py::resolve(given_registry);
    let repo_root = py::resolve(given_repo);
    let policy_file = match invocation.text("policy") {
        Some(given) => py::resolve(given),
        None => source_probe::policy_path(&registry_root),
    };
    let policy = if policy_file.exists() {
        Some(load_json(&policy_file)?)
    } else {
        None
    };
    let result = source_probe::probe_sources(
        &repo_root,
        &registry_root.join("source-map.json"),
        policy.as_ref(),
    )
    .map_err(Fail::text)?;
    let summary = source_probe::probe_summary(
        &result,
        &py::resolve(given_registry),
        &py::resolve(given_repo),
        given_registry,
        given_repo,
    )
    .map_err(Fail::text)?;
    crate::emit_stdout(&format!("{summary}\n"));
    Ok(match result.status.as_str() {
        "current" | "discovery_drifted" => 0,
        "absent" => 2,
        _ => 1,
    })
}

pub fn readiness(invocation: &Invocation) -> Result<i32, String> {
    let given_repo = invocation.text("repo_root").unwrap_or(".");
    let given_registry = invocation.text("registry_root");
    let registry_root = given_registry.map(py::resolve);
    let result = readiness::readiness_report(&py::resolve(given_repo), registry_root.as_deref())
        .map_err(Fail::text)?;
    crate::print_json(&Value::Object(result.report.clone()));
    if let Some(located) = &result.located {
        crate::emit_stderr(&format!("{located}\n"));
    }
    if let Some(note) = readiness::drift_note(&result.report, given_registry, given_repo) {
        crate::emit_stderr(&format!("{note}\n"));
    }
    Ok(0)
}
