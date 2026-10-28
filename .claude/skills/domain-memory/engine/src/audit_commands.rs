use std::io::Read;
use std::path::Path;

use crate::attestations;
use crate::audit_log;
use crate::cli::Invocation;
use crate::files::{PROPOSAL_FILE, load_json};
use crate::git_governance;
use crate::json::{self, Object, Style, Value};
use crate::proposals;
use crate::py;
use crate::pyvalue::str_of;
use crate::{emit_stderr, emit_stdout, print_json};

const INLINE: Style = Style {
    indent: None,
    sort_keys: false,
    ensure_ascii: false,
    compact: false,
};

pub fn print_inline(value: &Value) {
    emit_stdout(&format!("{}\n", json::dumps(value, INLINE)));
}

fn report_errors(errors: &[String], success_message: &str) -> i32 {
    if errors.is_empty() {
        emit_stdout(&format!("{success_message}\n"));
        return 0;
    }
    let lines: Vec<String> = errors
        .iter()
        .map(|error| format!("ERROR: {error}"))
        .collect();
    emit_stderr(&format!("{}\n", lines.join("\n")));
    1
}

fn resolved(invocation: &Invocation, dest: &str) -> std::path::PathBuf {
    py::resolve(invocation.text(dest).unwrap_or("."))
}

pub fn verify_audit(invocation: &Invocation) -> Result<i32, String> {
    let root = resolved(invocation, "registry_root");
    let mut result = audit_log::verify(&root)?;
    if audit_log::is_valid(&result) {
        let changes_root = invocation.text("changes_root").map(py::resolve);
        if let Some(reason) =
            audit_log::anchor_error(&root, changes_root.as_deref(), invocation.text("against"))?
        {
            result = Object::new();
            result.insert("status", Value::Str("invalid".to_string()));
            result.insert("reason", Value::Str(reason));
        }
    }
    let valid = audit_log::is_valid(&result);
    print_inline(&Value::Object(result));
    Ok(if valid { 0 } else { 1 })
}

pub fn audit_attestations(invocation: &Invocation) -> Result<i32, String> {
    let findings = attestations::audit_attestations(
        &resolved(invocation, "repo_root"),
        &resolved(invocation, "changes_root"),
    )?;
    let all_carried = findings.iter().all(|finding| {
        finding
            .as_object()
            .and_then(|object| object.get("verdict"))
            .and_then(Value::as_str)
            == Some("carried")
    });
    print_json(&Value::Array(findings));
    Ok(if all_carried { 0 } else { 1 })
}

pub fn verify_scm_attestation(invocation: &Invocation) -> Result<i32, String> {
    let package_root = resolved(invocation, "package_root");
    let proposal = load_json(&package_root.join(PROPOSAL_FILE))?;
    let attestation = load_json(&resolved(invocation, "attestation"))?;
    let errors = attestations::verify_scm_value(&attestation, &proposal);
    Ok(report_errors(&errors, "SCM attestation is valid."))
}

fn commits_to_verify(invocation: &Invocation) -> Result<Vec<String>, String> {
    if let Some(commit) = invocation
        .text("commit")
        .filter(|commit| !commit.is_empty())
    {
        return Ok(vec![commit.to_string()]);
    }
    let mut bytes = Vec::new();
    let _ = std::io::stdin().read_to_end(&mut bytes);
    Ok(py::utf8_strict(&bytes)?
        .split(py::is_space)
        .filter(|word| !word.is_empty())
        .map(str::to_string)
        .collect())
}

pub fn verify_git_governance(invocation: &Invocation) -> Result<i32, String> {
    let commits = commits_to_verify(invocation)?;
    let registry_root = resolved(invocation, "registry_root");
    let errors = git_governance::verify_git_governance(
        &registry_root,
        &resolved(invocation, "repo_root"),
        &commits,
    )?;
    let plural = if commits.len() == 1 { "" } else { "s" };
    let code = report_errors(
        &errors,
        &format!(
            "Git governance is valid for {} commit{plural}.",
            commits.len()
        ),
    );
    note_unsettled(&registry_root)?;
    Ok(code)
}

pub fn governance_readiness(invocation: &Invocation) -> Result<i32, String> {
    let readiness = git_governance::governance_readiness(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
    )?;
    let mut result = Object::new();
    result.insert("status", Value::Str(readiness.status.to_string()));
    result.insert(
        "blocks",
        Value::Array(readiness.blocks.into_iter().map(Value::Str).collect()),
    );
    print_inline(&Value::Object(result));
    Ok(if readiness.status == "needs-setup" {
        1
    } else {
        0
    })
}

fn note_unsettled(registry_root: &Path) -> Result<(), String> {
    let beside = registry_root.join("changes");
    let rows = if beside.is_dir() {
        proposals::list_proposals(&beside)?
    } else {
        Vec::new()
    };
    let field = |row: &Object, key: &str| row.get(key).cloned().unwrap_or(Value::Null);
    let rows: Vec<&Object> = rows.iter().filter_map(Value::as_object).collect();
    let pending: Vec<String> = rows
        .iter()
        .filter(|row| field(row, "status").as_str() == Some("approved"))
        .map(|row| str_of(&field(row, "proposal_id")))
        .collect();
    let dangling: Vec<String> = rows
        .iter()
        .filter(|row| field(row, "superseded_by_unknown") == Value::Bool(true))
        .map(|row| {
            format!(
                "{} (by {})",
                str_of(&field(row, "proposal_id")),
                str_of(&field(row, "superseded_by"))
            )
        })
        .collect();
    if !dangling.is_empty() {
        let names = if dangling.len() == 1 {
            " names"
        } else {
            "s name"
        };
        emit_stderr(&format!(
            "NOTE: {} superseded Change Package{names} a replacement no package carries: {}. \
             Create it with redraft-proposal --proposal-id <id>.\n",
            dangling.len(),
            dangling.join(", ")
        ));
    }
    if !pending.is_empty() {
        let plural = if pending.len() == 1 { "" } else { "s" };
        emit_stderr(&format!(
            "NOTE: {} approved Change Package{plural} not applied: {}. Apply each with ship-package; \
             retire one that cannot be applied with supersede-proposal, and redraft-proposal carries \
             its work into a new draft.\n",
            pending.len(),
            pending.join(", ")
        ));
    }
    Ok(())
}
