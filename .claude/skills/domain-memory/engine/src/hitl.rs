use std::path::{Path, PathBuf};

use crate::attestations::{commit_carries_proposal, verify_attested_commit};
use crate::audit_append::utc_now;
use crate::changes::validate_change_package;
use crate::common::EVIDENCE_FILE;
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::git::resolve_commit;
use crate::json::{Object, Value};
use crate::policy::review_governance;
use crate::pyvalue::str_of;
use crate::revision::{Refusal, base_registry_revision, require_current_registry_revision};
use crate::store::shown_path;

const REBASABLE_STATUSES: [&str; 3] = ["submitted", "verified", "approved"];
const AFTER_REBASE: &str = "record-approval for each required role, verify-proposal, sign a commit holding the package, \
    then ship-package --commit <that commit>";
const SIGNED_COMMIT: &str = "git-signed-commit";
const SIGNED_COMMIT_STEPS: &str = "after the signed commit that adds this Change Package and changes the Domain Memory files \
    exists, run ship-package --commit <that commit>; it checks the signature and the \
    package, and writes scm_attestation with provider \"git-signed-commit\"";

pub fn proposal_path(root: &Path) -> PathBuf {
    root.join(PROPOSAL_FILE)
}

fn field(object: &Object, key: &str) -> Value {
    object.get(key).cloned().unwrap_or(Value::Null)
}

fn not_rebased_reason(status: &Value) -> &'static str {
    match status.as_str() {
        Some("draft") => "submit-proposal stamps its base",
        Some("applied") => "it is applied; supersede it and open a new draft",
        Some("superseded") => "it is retired; redraft-proposal opens a new draft from it",
        _ => "only a submitted, verified or approved proposal is",
    }
}

fn next_when_current(status: &str) -> &'static str {
    match status {
        "submitted" => "record-approval for each required role, then verify-proposal",
        "verified" => "sign a commit holding the package, then ship-package --commit <that commit>",
        _ => "run ship-package to apply it",
    }
}

fn article(word: &str) -> &'static str {
    match word.chars().next() {
        None | Some('a' | 'e' | 'i' | 'o' | 'u') => "an",
        Some(_) => "a",
    }
}

fn stale(proposal: &Object, registry_root: &Path, repo_root: &Path) -> Result<bool, String> {
    match require_current_registry_revision(proposal, registry_root, repo_root) {
        Ok(_) => Ok(false),
        Err(Refusal::Stale(_)) => Ok(true),
        Err(Refusal::Other(message)) => Err(message),
    }
}

fn next_revision(current: &Value) -> Result<Value, String> {
    match current {
        Value::Int(digits) => digits
            .parse::<i128>()
            .map(|number| Value::Int((number + 1).to_string()))
            .map_err(|_| "proposal_revision is too large to increase".to_string()),
        Value::Float(number) => Ok(Value::Float(number + 1.0)),
        Value::Bool(flag) => Ok(Value::Int((i32::from(*flag) + 1).to_string())),
        _ => Err("unsupported operand type(s) for +: proposal_revision and 'int'".to_string()),
    }
}

pub fn rebase_proposal(
    root: &Path,
    registry_root: &Path,
    repo_root: &Path,
) -> Result<Object, String> {
    let (root, registry_root, repo_root) = (
        crate::py::resolve_path(root),
        crate::py::resolve_path(registry_root),
        crate::py::resolve_path(repo_root),
    );
    let mut proposal = load_json(&proposal_path(&root))?;
    let status = field(&proposal, "status");
    let Some(held) = status
        .as_str()
        .filter(|held| REBASABLE_STATUSES.contains(held))
    else {
        let shown = str_of(&status);
        return Err(format!(
            "{} {shown} proposal is not rebased; {}",
            article(&shown),
            not_rebased_reason(&status)
        ));
    };
    let shown = shown_path(&root, Some(&repo_root));
    let mut reply = Object::new();
    reply.insert("package", Value::Str(shown));
    if !stale(&proposal, &registry_root, &repo_root)? {
        reply.insert("status", status.clone());
        reply.insert("proposal_revision", field(&proposal, "proposal_revision"));
        reply.insert(
            "base_registry_revision",
            field(&proposal, "base_registry_revision"),
        );
        reply.insert(
            "note",
            Value::Str("the proposal base is current; nothing was written".to_string()),
        );
        reply.insert("next", Value::Str(next_when_current(held).to_string()));
        return Ok(reply);
    }
    let revision = base_registry_revision(&registry_root, &repo_root)?;
    let current_revision = field(&proposal, "proposal_revision");
    let retired = match proposal.get("approvals") {
        Some(Value::Array(approvals)) => approvals
            .iter()
            .filter_map(Value::as_object)
            .filter(|approval| {
                crate::pyvalue::equal(&field(approval, "proposal_revision"), &current_revision)
            })
            .count(),
        _ => 0,
    };
    proposal.insert("base_registry_revision", Value::Object(revision.clone()));
    proposal.insert("proposal_revision", next_revision(&current_revision)?);
    proposal.insert("status", Value::Str("submitted".to_string()));
    proposal.insert("verified_at", Value::Null);
    proposal.insert("finalized_at", Value::Null);
    let evidence_path = root.join(EVIDENCE_FILE);
    let mut evidence = load_json(&evidence_path)?;
    let attested = matches!(evidence.get("scm_attestation"), Some(Value::Object(_)));
    evidence.insert("registry_revision", Value::Object(revision.clone()));
    evidence.insert("scm_attestation", Value::Null);
    write_json(&evidence_path, &Value::Object(evidence))?;
    write_json(&proposal_path(&root), &Value::Object(proposal.clone()))?;
    let mut reply = Object::new();
    reply.insert("package", Value::Str(shown_path(&root, Some(&repo_root))));
    reply.insert("status", Value::Str("submitted".to_string()));
    reply.insert("proposal_revision", field(&proposal, "proposal_revision"));
    reply.insert("base_registry_revision", Value::Object(revision));
    reply.insert("approvals_retired", Value::int(retired));
    reply.insert("attestation_removed", Value::Bool(attested));
    reply.insert("next", Value::Str(AFTER_REBASE.to_string()));
    Ok(reply)
}

pub fn attest_signed_commit(
    root: &Path,
    registry_root: &Path,
    repo_root: &Path,
    commit: &str,
) -> Result<String, String> {
    let governance = review_governance(registry_root)?;
    let verifier = field(&governance, "verifier");
    if verifier.as_str() != Some(SIGNED_COMMIT) {
        return Err(format!(
            "--commit is for the git-signed-commit verifier; this policy uses {}",
            str_of(&verifier)
        ));
    }
    let proposal = load_json(&proposal_path(root))?;
    if matches!(
        proposal.get("base_registry_revision"),
        None | Some(Value::Null)
    ) {
        return Err(
            "submit the proposal first, so the attestation can bind to its base revision"
                .to_string(),
        );
    }
    let Some(full) = resolve_commit(repo_root, commit)? else {
        return Err(format!(
            "no commit {commit} in {}",
            crate::py::shown(repo_root)
        ));
    };
    let mut named = Object::new();
    named.insert("commit", Value::Str(full.clone()));
    let mut errors = verify_attested_commit(&named, repo_root, registry_root, &governance)?;
    if errors.is_empty() {
        errors = commit_carries_proposal(repo_root, &full, root, &proposal)?;
    }
    if !errors.is_empty() {
        return Err(format!(
            "the commit cannot attest this proposal: {}",
            errors.join("; ")
        ));
    }
    let evidence_path = root.join(EVIDENCE_FILE);
    let mut evidence = load_json(&evidence_path)?;
    let mut attestation = Object::new();
    attestation.insert("provider", Value::Str(SIGNED_COMMIT.to_string()));
    attestation.insert("commit", Value::Str(full.clone()));
    attestation.insert("status", Value::Str("approved".to_string()));
    attestation.insert("proposal_revision", field(&proposal, "proposal_revision"));
    attestation.insert(
        "base_registry_revision",
        field(&proposal, "base_registry_revision"),
    );
    evidence.insert("scm_attestation", Value::Object(attestation));
    write_json(&evidence_path, &Value::Object(evidence))?;
    Ok(full)
}

fn missing_attestation_message(verifier: &Value) -> String {
    if verifier.as_str() == Some(SIGNED_COMMIT) {
        return format!(
            "finalizing a proposal requires SCM attestation: {SIGNED_COMMIT_STEPS}, then finalize again"
        );
    }
    "finalizing a proposal requires SCM attestation, and the policy names no review verifier"
        .to_string()
}

pub fn finalize_proposal(
    root: &Path,
    registry_root: &Path,
    repo_root: &Path,
) -> Result<(), Refusal> {
    let path = proposal_path(root);
    let mut proposal = load_json(&path)?;
    if field(&proposal, "status").as_str() != Some("verified") {
        return Err(Refusal::Other(
            "only a verified proposal may be finalized".to_string(),
        ));
    }
    require_current_registry_revision(&proposal, registry_root, repo_root)?;
    let evidence = load_json(&root.join(EVIDENCE_FILE))?;
    let governance = review_governance(registry_root)?;
    let verifier = field(&governance, "verifier");
    let Some(Value::Object(attestation)) = evidence.get("scm_attestation") else {
        return Err(Refusal::Other(missing_attestation_message(&verifier)));
    };
    let mut errors = if verifier.as_str() == Some(SIGNED_COMMIT) {
        let mut found = verify_attested_commit(attestation, repo_root, registry_root, &governance)?;
        if found.is_empty() {
            let commit = str_of(&field(attestation, "commit"));
            found = commit_carries_proposal(repo_root, &commit, root, &proposal)?;
        }
        found
    } else {
        vec!["policy has no review verifier".to_string()]
    };
    if !errors.is_empty() {
        return Err(Refusal::Other(format!(
            "SCM governance is not externally verified: {}",
            errors.join("; ")
        )));
    }
    errors = validate_change_package(root, Some(registry_root), Some("approved"), None)?;
    if errors.is_empty() {
        proposal.insert("status", Value::Str("approved".to_string()));
        proposal.insert("finalized_at", Value::Str(utc_now()));
        write_json(&path, &Value::Object(proposal))?;
        return Ok(());
    }
    proposal.insert("status", Value::Str("submitted".to_string()));
    let mut kept = Object::new();
    for (key, value) in proposal.iter() {
        if key != "finalized_at" {
            kept.insert(key.clone(), value.clone());
        }
    }
    write_json(&path, &Value::Object(kept))?;
    Err(Refusal::Other(format!(
        "proposal lacks required approvals or traceability, and is submitted again so it \
         can be corrected, then verified and finalized again: {}",
        errors.join("; ")
    )))
}
