use crate::pyvalue::{equal, message_text, str_of};
use std::path::Path;

use crate::audit_log::APPLY_OPERATION;
use crate::common::EVIDENCE_FILE;
use crate::files::load_json;
use crate::git::resolve_commit;
use crate::hitl::{attest_signed_commit, finalize_proposal, proposal_path};
use crate::json::{Object, Value};
use crate::py;
use crate::revision::{Refusal, require_current_registry_revision};
use crate::store::shown_path;
use crate::updates::apply_approved_updates;

const FORMAT: &str = "domain-memory-ship/v1";
const ATTEST: &str = "attest-signed-commit";
const FINALIZE: &str = "finalize-proposal";
const APPLY: &str = APPLY_OPERATION;
const ALREADY_APPLIED: &str = "nothing to do here; if the applied Registry change is not committed yet, sign a commit holding it, then push";
const STALE_NEXT: &str = "the proposal base is stale; rebase-proposal, then record-approval, verify-proposal, sign a commit holding the \
    package, and ship-package --commit <that commit>";

struct Report {
    package: String,
    completed: Vec<String>,
    errors: Vec<String>,
    note: Option<String>,
    commit: Option<String>,
}

impl Report {
    fn into_object(self, status: Value, next: String) -> Object {
        let strings =
            |items: Vec<String>| Value::Array(items.into_iter().map(Value::Str).collect());
        let mut report = Object::new();
        report.insert("format", Value::Str(FORMAT.to_string()));
        report.insert("package", Value::Str(self.package));
        report.insert("completed", strings(self.completed));
        report.insert("errors", strings(self.errors));
        if let Some(note) = self.note {
            report.insert("note", Value::Str(note));
        }
        if let Some(commit) = self.commit {
            report.insert("commit", Value::Str(commit));
        }
        report.insert("status", status);
        report.insert("next", Value::Str(next));
        report
    }
}

fn next_for(status: &Value) -> String {
    match status.as_str() {
        Some("applied") => "the Registry is updated; sign a commit holding the package and the Registry, then push".to_string(),
        Some("approved") => "the proposal is approved but not applied; run ship-package again".to_string(),
        Some("verified") => "the proposal is verified; ship-package --commit <the signed commit that carries the package>".to_string(),
        Some("submitted") => "the proposal is submitted again; correct what finalize named, verify-proposal, then ship-package".to_string(),
        _ => format!(
            "the proposal is {}; ship-package works from a verified proposal",
            message_text(status)
        ),
    }
}

fn steps_from(status: &Value, commit: Option<&str>) -> Vec<&'static str> {
    match status.as_str() {
        Some("approved") => vec![APPLY],
        Some("applied") => Vec::new(),
        _ if commit.is_some() => vec![ATTEST, FINALIZE, APPLY],
        _ => vec![FINALIZE, APPLY],
    }
}

fn another_commit(
    package_root: &Path,
    repo_root: &Path,
    status: &Value,
    commit: Option<&str>,
) -> Result<Vec<String>, String> {
    let evidence = load_json(&package_root.join(EVIDENCE_FILE))?;
    let attested = match evidence.get("scm_attestation") {
        Some(Value::Object(attestation)) => attestation.get("commit").cloned(),
        _ => None,
    };
    let (Some(commit), Some(attested)) = (commit, attested.filter(|held| *held != Value::Null))
    else {
        return Ok(Vec::new());
    };
    let given = resolve_commit(repo_root, commit)?.unwrap_or_else(|| commit.to_string());
    if equal(&Value::Str(given.clone()), &attested) {
        return Ok(Vec::new());
    }
    Ok(vec![format!(
        "{ATTEST}: the proposal is {} with commit {}; {given} is another commit, so nothing was written",
        str_of(status),
        str_of(&attested)
    )])
}

pub fn ship_package(
    package_root: &Path,
    registry_root: &Path,
    repo_root: &Path,
    commit: Option<&str>,
) -> Result<Object, String> {
    let (package_root, registry_root, repo_root) = (
        py::resolve_path(package_root),
        py::resolve_path(registry_root),
        py::resolve_path(repo_root),
    );
    let mut report = Report {
        package: shown_path(&package_root, Some(&repo_root)),
        completed: Vec::new(),
        errors: Vec::new(),
        note: None,
        commit: None,
    };
    let proposal = load_json(&proposal_path(&package_root))?;
    let status = proposal.get("status").cloned().unwrap_or(Value::Null);
    let names = steps_from(&status, commit);
    if matches!(status.as_str(), Some("approved" | "applied")) {
        report
            .errors
            .extend(another_commit(&package_root, &repo_root, &status, commit)?);
        if names.is_empty() && report.errors.is_empty() {
            report.note = Some(format!(
                "already {}; nothing was written",
                message_text(&status)
            ));
        }
    }
    let mut stale_steps = false;
    if status.as_str() == Some("verified")
        && let Err(refusal) =
            require_current_registry_revision(&proposal, &registry_root, &repo_root)
    {
        stale_steps |= matches!(refusal, Refusal::Stale(_));
        report
            .errors
            .push(format!("{FINALIZE}: {}", String::from(refusal)));
    }
    for name in names {
        if !report.errors.is_empty() {
            break;
        }
        let outcome: Result<Option<String>, Refusal> = match name {
            ATTEST => attest_signed_commit(
                &package_root,
                &registry_root,
                &repo_root,
                commit.unwrap_or_default(),
            )
            .map(Some)
            .map_err(Refusal::Other),
            FINALIZE => finalize_proposal(&package_root, &registry_root, &repo_root).map(|()| None),
            _ => apply_approved_updates(&package_root, &registry_root, &repo_root).map(|()| None),
        };
        match outcome {
            Ok(attested) => {
                report.completed.push(name.to_string());
                if name == ATTEST {
                    report.commit = attested;
                }
            }
            Err(refusal) => {
                stale_steps |= matches!(refusal, Refusal::Stale(_));
                report
                    .errors
                    .push(format!("{name}: {}", String::from(refusal)));
                break;
            }
        }
    }
    let status = load_json(&proposal_path(&package_root))?
        .get("status")
        .cloned()
        .unwrap_or(Value::Null);
    let next = if report.note.is_some() {
        ALREADY_APPLIED.to_string()
    } else if stale_steps {
        STALE_NEXT.to_string()
    } else {
        next_for(&status)
    };
    Ok(report.into_object(status, next))
}
