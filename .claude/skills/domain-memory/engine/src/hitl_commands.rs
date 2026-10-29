use crate::cli::Invocation;
use crate::files::{PROPOSAL_FILE, load_json};
use crate::hitl::{
    Approval, ObligationRun, record_approval, record_test_result, submit_proposal,
    supersede_proposal, verify_proposal,
};
use crate::json::Value;
use crate::proposals::list_proposals;
use crate::py::{self, PurePath};
use crate::pyvalue::message_text;
use crate::write_commands::resolved;
use crate::{emit_stderr, emit_stdout};

const PAST_VERIFICATION: [&str; 3] = ["verified", "approved", "applied"];
const DEFAULT_TIMEOUT_SECONDS: i64 = 600;

fn required<'a>(invocation: &'a Invocation, dest: &str) -> &'a str {
    invocation.text(dest).unwrap_or_default()
}

pub fn submit(invocation: &Invocation) -> Result<i32, String> {
    submit_proposal(
        &resolved(invocation, "package_root"),
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
    )?;
    emit_stdout("Proposal submitted for human review.\n");
    Ok(0)
}

pub fn approve(invocation: &Invocation) -> Result<i32, String> {
    record_approval(
        &resolved(invocation, "package_root"),
        &Approval {
            role: required(invocation, "role"),
            reviewer: required(invocation, "reviewer"),
            scope: required(invocation, "scope"),
            approved_at: invocation.text("approved_at"),
        },
    )?;
    emit_stdout("Human approval recorded.\n");
    Ok(0)
}

pub fn supersede(invocation: &Invocation) -> Result<i32, String> {
    let given = required(invocation, "package_root");
    let package_root = py::resolve(given);
    let superseded_by = invocation.text("superseded_by");
    supersede_proposal(&package_root, required(invocation, "reason"), superseded_by)?;
    if let Some(replacement) = superseded_by {
        let beside = package_root.parent().unwrap_or(&package_root);
        let carried = list_proposals(beside)?.iter().any(|row| {
            row.as_object()
                .and_then(|row| row.get("proposal_id"))
                .and_then(Value::as_str)
                == Some(replacement)
        });
        if !carried {
            let shown = PurePath::new(given).native();
            emit_stderr(&format!(
                "NOTE: no package beside {shown} carries {replacement} yet; create it with redraft-proposal --package-root {shown} --output <dir> --proposal-id {replacement}, and validate names this proposal until a package carries it.\n"
            ));
        }
        emit_stdout(&format!(
            "Proposal superseded by {replacement}. Its record stays.\n"
        ));
    } else {
        emit_stdout(
            "Proposal superseded. Its record stays; create a new draft for the replacement.\n",
        );
    }
    Ok(0)
}

pub fn verify(invocation: &Invocation) -> Result<i32, String> {
    let package_root = resolved(invocation, "package_root");
    let proposal = load_json(&package_root.join(PROPOSAL_FILE))?;
    let status = proposal.get("status").and_then(Value::as_str);
    if let Some(status) = status.filter(|status| PAST_VERIFICATION.contains(status)) {
        let revision = proposal
            .get("proposal_revision")
            .map_or_else(|| "null".to_string(), message_text);
        emit_stdout(&format!(
            "already {status} at revision {revision}; nothing was written\n"
        ));
        return Ok(0);
    }
    verify_proposal(
        &package_root,
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
    )?;
    emit_stdout("Proposal test evidence verified.\n");
    Ok(0)
}

pub fn record_result(invocation: &Invocation) -> Result<i32, String> {
    let obligation_id = required(invocation, "obligation");
    record_test_result(
        &resolved(invocation, "package_root"),
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
        &ObligationRun {
            obligation_id,
            command_profile: required(invocation, "command_profile"),
            cwd: invocation.text("cwd"),
        },
        invocation.int("timeout").unwrap_or(DEFAULT_TIMEOUT_SECONDS),
    )?;
    emit_stdout(&format!("Recorded a passing result for {obligation_id}.\n"));
    Ok(0)
}
