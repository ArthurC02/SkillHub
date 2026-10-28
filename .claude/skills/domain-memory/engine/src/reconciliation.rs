use std::path::{Path, PathBuf};

use crate::audit_append::utc_now;
use crate::audit_log::{APPLY_OPERATION, read_events};
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::json::{Object, Value};
use crate::proposals::completed_identifier;
use crate::py;
use crate::pyvalue::{equal, str_of};
use crate::revision::{current_registry_revision, inside};

pub fn reconciliation_path(root: &Path) -> PathBuf {
    root.join(".domain-registry-reconciliation.json")
}

fn was_applied(registry_root: &Path, proposal_id: &Value) -> Result<bool, String> {
    Ok(read_events(registry_root)?.iter().any(|event| {
        event.get("operation").and_then(Value::as_str) == Some(APPLY_OPERATION)
            && equal(
                event.get("proposal_id").unwrap_or(&Value::Null),
                proposal_id,
            )
    }))
}

fn mark_applied(
    proposal_path: &Path,
    proposal: &mut Object,
    registry_root: &Path,
    repo_root: &Path,
) -> Result<(), String> {
    proposal.insert("status", Value::Str("applied".to_string()));
    proposal.insert("applied_at", Value::Str(utc_now()));
    proposal.insert(
        "applied_registry_revision",
        Value::Object(current_registry_revision(registry_root, repo_root)?),
    );
    write_json(proposal_path, &Value::Object(proposal.clone()))
}

fn reconciled(
    proposal_path: &Path,
    proposal: &mut Object,
    registry_root: &Path,
    repo_root: &Path,
) -> Result<String, String> {
    let proposal_id = proposal.get("proposal_id").cloned().unwrap_or(Value::Null);
    let shown_id = str_of(&proposal_id);
    let status = proposal.get("status").cloned().unwrap_or(Value::Null);
    if status.as_str() == Some("applied") {
        return Ok(format!("{shown_id} was already marked applied"));
    }
    if !was_applied(registry_root, &proposal_id)? {
        return Ok(format!(
            "{shown_id} was never applied: the audit log holds no apply for it, so the Registry is unchanged"
        ));
    }
    if status.as_str() != Some("approved") {
        return Err(format!(
            "the audit log records an apply of {shown_id}, but its proposal is {}; reconcile the package with the Registry by hand",
            str_of(&status)
        ));
    }
    mark_applied(proposal_path, proposal, registry_root, repo_root)?;
    Ok(format!(
        "{shown_id} was applied before an interruption and is now marked applied"
    ))
}

pub fn reconcile_pending_update(
    registry_root: &Path,
    repo_root: &Path,
) -> Result<Option<String>, String> {
    let marker_path = reconciliation_path(registry_root);
    if !marker_path.is_file() {
        return Ok(None);
    }
    let marker = load_json(&marker_path)?;
    let proposal_id = marker.get("proposal_id").unwrap_or(&Value::Null);
    let Some(Value::Str(relative)) = marker.get("package") else {
        return Err("registry reconciliation marker is malformed".to_string());
    };
    if !completed_identifier(proposal_id) {
        return Err("registry reconciliation marker is malformed".to_string());
    }
    let package_root = inside(repo_root, Path::new(relative))
        .ok_or("registry reconciliation marker points outside the repository".to_string())?;
    let proposal_path = package_root.join(PROPOSAL_FILE);
    let mut proposal = load_json(&proposal_path)?;
    if !equal(
        proposal.get("proposal_id").unwrap_or(&Value::Null),
        proposal_id,
    ) {
        return Err("registry reconciliation marker does not match its proposal".to_string());
    }
    let outcome = reconciled(&proposal_path, &mut proposal, registry_root, repo_root)?;
    std::fs::remove_file(&marker_path).map_err(|error| py::os_error(&error, &marker_path))?;
    Ok(Some(outcome))
}
