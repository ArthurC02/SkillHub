use std::path::PathBuf;

use crate::brief;
use crate::cli::Invocation;
use crate::counterfactual::{self, Mutation, Proof, Refusal};
use crate::files::load_json;
use crate::gates;
use crate::impact::{self, Library};
use crate::json::{Object, Value};
use crate::py::{self, PurePath};
use crate::store::Registry;

const DEFAULT_TIMEOUT_SECONDS: i64 = 600;

pub fn quality_gates(invocation: &Invocation) -> Result<i32, String> {
    let report = gates::quality_gates(invocation.text("repo_root").unwrap_or("."))?;
    crate::print_json(&report);
    Ok(0)
}

pub fn boundary_impact(invocation: &Invocation) -> Result<i32, String> {
    let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
    let repo_root = py::resolve(invocation.text("repo_root").unwrap_or("."));
    let base = invocation.text("base").unwrap_or_default();
    let changes = impact::changed_files_since(&repo_root, base)?;
    let registry = Registry::open(&root)?;
    let library = Library::read(&registry)?;
    let found = impact::boundary_impact(&library, &root, &repo_root, base, &changes)?;
    crate::print_json(&Value::Object(found));
    Ok(0)
}

fn subject_refusal(
    base: Option<&str>,
    asset: Option<&str>,
    id: Option<&str>,
) -> Option<&'static str> {
    let by_record = asset.is_some() || id.is_some();
    if base.is_some() && by_record {
        return Some("brief takes --base <revision> or --asset <asset> --id <id>, not both");
    }
    if base.is_none() && !by_record {
        return Some("brief needs --base <revision>, or --asset <asset> --id <id>");
    }
    let named = |text: Option<&str>| text.is_some_and(|text| !text.is_empty());
    if by_record && !(named(asset) && named(id)) {
        return Some("brief by record needs both --asset <asset> and --id <id>");
    }
    None
}

pub fn brief(invocation: &Invocation) -> Result<i32, String> {
    let (base, asset, id) = (
        invocation.text("base"),
        invocation.text("asset"),
        invocation.text("id"),
    );
    if let Some(refusal) = subject_refusal(base, asset, id) {
        return Err(refusal.to_string());
    }
    let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
    let repo_root = py::resolve(invocation.text("repo_root").unwrap_or("."));
    let found = match base.filter(|base| !base.is_empty()) {
        Some(base) => brief::brief_for_change(&root, &repo_root, base)?,
        None if base.is_some() => return Err("None record not found: None".to_string()),
        None => brief::brief_for_record(
            &root,
            &repo_root,
            asset.unwrap_or_default(),
            id.unwrap_or_default(),
        )?,
    };
    if invocation.flag("markdown") {
        crate::emit_stdout(&format!("{}\n", brief::to_markdown(&found)));
    } else {
        crate::print_json(&Value::Object(found));
    }
    Ok(0)
}

enum Attempt {
    Suite(Object, i32),
    One(Object),
}

fn one_mutation(invocation: &Invocation) -> Result<Mutation, String> {
    let given = [
        ("--file", invocation.text("file")),
        ("--find", invocation.text("find")),
        ("--replace", invocation.text("replace")),
        ("--test-command", invocation.text("test_command")),
    ];
    let missing: Vec<&str> = given
        .iter()
        .filter(|(_, value)| value.is_none())
        .map(|(name, _)| *name)
        .collect();
    if !missing.is_empty() {
        return Err(format!(
            "counterfactual needs {}, or --suite <json file> for several mutations",
            missing.join(", ")
        ));
    }
    let text = |dest: &str| invocation.text(dest).unwrap_or_default().to_string();
    Ok(Mutation {
        file: text("file"),
        find: text("find"),
        replace: text("replace"),
    })
}

fn attempt(
    invocation: &Invocation,
    repo_root: &str,
    package: Option<&str>,
) -> Result<Attempt, Refusal> {
    if let Some(suite) = invocation.text("suite") {
        if package.is_some() {
            return Err("--package-root records one counterfactual; run each obligation's mutation on its own with --obligation-id"
                .to_string()
                .into());
        }
        let document = load_json(&PathBuf::from(PurePath::new(suite).native()))?;
        let report = counterfactual::counterfactual_suite(repo_root, &document)?;
        let code = report
            .get("results")
            .and_then(Value::as_array)
            .unwrap_or_default()
            .iter()
            .filter_map(Value::as_object)
            .filter_map(|row| row.get("verdict").and_then(Value::as_str))
            .map(counterfactual::exit_code)
            .max()
            .unwrap_or(0);
        return Ok(Attempt::Suite(report, code));
    }
    if let Some(package) = package {
        let package = PathBuf::from(PurePath::new(package).native());
        counterfactual::require_recordable(&package, invocation.text("obligation_id"))?;
    }
    let mutation = one_mutation(invocation)?;
    let proof = Proof {
        test_command: invocation
            .text("test_command")
            .unwrap_or_default()
            .to_string(),
        timeout: invocation.int("timeout").unwrap_or(DEFAULT_TIMEOUT_SECONDS),
        cwd: invocation.text("cwd").map(str::to_string),
        expect: invocation.text("expect").map(str::to_string),
        obligation_id: invocation.text("obligation_id").map(str::to_string),
    };
    Ok(Attempt::One(counterfactual::counterfactual(
        repo_root, &mutation, &proof,
    )?))
}

pub fn counterfactual(invocation: &Invocation) -> Result<i32, String> {
    let repo_root = invocation.text("repo_root").unwrap_or(".");
    let package = invocation.text("package_root");
    let mut result = match attempt(invocation, repo_root, package) {
        Ok(Attempt::Suite(report, code)) => {
            crate::print_json(&Value::Object(report));
            return Ok(code);
        }
        Ok(Attempt::One(result)) => result,
        Err(refusal) => counterfactual::error_result(refusal),
    };
    let verdict = result
        .get("verdict")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string();
    if let Some(package) = package.filter(|_| verdict != "error") {
        let package = PathBuf::from(PurePath::new(package).native());
        result = counterfactual::with_record(&package, repo_root, result);
    }
    let unrecorded = result.get("not_recorded").is_some() && verdict == "killed";
    let code = counterfactual::exit_code(if unrecorded { "error" } else { &verdict });
    crate::print_json(&Value::Object(result));
    Ok(code)
}
