use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

use crate::attestations::{commit_carries_proposal, verify_attested_commit};
use crate::audit_append::utc_now;
use crate::audit_log::type_name;
use crate::changes::{
    missing_approval_roles, test_attestation_errors, unconfirmed_evidence_errors,
    validate_change_package,
};
use crate::common::{
    EVIDENCE_FILE, OBLIGATIONS_FILE, REQUIREMENT_FILE, find_record, iso_timestamp,
};
use crate::counterfactual::{Ran, RunFault, decoded, run_captured, working_directory};
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::git::resolve_commit;
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::policy::{approved_command_profiles, review_governance};
use crate::proposals::completed_identifier;
use crate::py;
use crate::pyvalue::{equal, repr_of, str_of};
use crate::revision::{Refusal, base_registry_revision, require_current_registry_revision};
use crate::sha256::hex_digest;
use crate::shlex;
use crate::store::shown_path;
use crate::test_output::{is_go_test, no_test_passed, reports_tests};

const OUTPUT_TAIL: usize = 1200;
const CI_LINK_PREFIX: &str = "https://";
const SUPERSEDABLE_STATUSES: [&str; 5] = ["draft", "submitted", "verified", "approved", "applied"];
const RECORDABLE_STATUSES: [&str; 2] = ["draft", "submitted"];
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

static NULL: Value = Value::Null;

fn get<'a>(object: &'a Object, key: &str) -> &'a Value {
    object.get(key).unwrap_or(&NULL)
}

fn field(object: &Object, key: &str) -> Value {
    get(object, key).clone()
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

fn status_in(status: &Value, allowed: &[&str]) -> Result<bool, String> {
    crate::promotion::unhashable(status)?;
    Ok(status.as_str().is_some_and(|text| allowed.contains(&text)))
}

fn iterated(value: Option<&Value>) -> Result<Vec<Value>, String> {
    match value {
        None => Ok(Vec::new()),
        Some(Value::Array(items)) => Ok(items.clone()),
        Some(Value::Object(object)) => Ok(object
            .iter()
            .map(|(key, _)| Value::Str(key.clone()))
            .collect()),
        Some(Value::Str(text)) => Ok(text.chars().map(|c| Value::Str(c.to_string())).collect()),
        Some(other) => Err(format!("'{}' object is not iterable", type_name(other))),
    }
}

fn obligation_entries(obligations: &Object) -> Result<Vec<Value>, String> {
    iterated(obligations.get("obligations"))
}

fn silent_go_obligations(entries: &[Value]) -> Vec<String> {
    entries
        .iter()
        .filter_map(Value::as_object)
        .filter(|entry| {
            let check = entry.get("check").map_or_else(String::new, str_of);
            let argv: Vec<String> = check
                .split(py::is_space)
                .filter(|word| !word.is_empty())
                .map(str::to_string)
                .collect();
            is_go_test(&argv) && !reports_tests(&argv)
        })
        .map(|entry| entry.get("id").map_or_else(|| "None".to_string(), str_of))
        .collect()
}

pub fn submit_proposal(root: &Path, registry_root: &Path, repo_root: &Path) -> Result<(), String> {
    let path = root.join(PROPOSAL_FILE);
    let mut proposal = load_json(&path)?;
    if get(&proposal, "status").as_str() != Some("draft") {
        return Err("only a draft proposal may be submitted".to_string());
    }
    let entries = obligation_entries(&load_json(&root.join(OBLIGATIONS_FILE))?)?;
    let silent = silent_go_obligations(&entries);
    if !silent.is_empty() {
        return Err(format!(
            "obligations {} run go test without -v or -json, so record-test-result cannot tell a passing test from a skipped one; add -v to their check",
            silent.join(", ")
        ));
    }
    if !completed_identifier(get(&proposal, "proposer")) {
        return Err("a proposal requires a proposer before submission".to_string());
    }
    let revision = Value::Object(base_registry_revision(registry_root, repo_root)?);
    proposal.insert("base_registry_revision", revision.clone());
    let evidence_path = root.join(EVIDENCE_FILE);
    let mut evidence = load_json(&evidence_path)?;
    evidence.insert("registry_revision", revision);
    let mut errors = validate_change_package(root, None, None, None)?;
    errors.extend(unconfirmed_evidence_errors(&proposal, registry_root)?);
    if !errors.is_empty() {
        return Err(format!("draft proposal is invalid: {}", errors.join("; ")));
    }
    proposal.insert("status", Value::Str("submitted".to_string()));
    proposal.insert("submitted_at", Value::Str(utc_now()));
    write_json(&path, &Value::Object(proposal))?;
    write_json(&evidence_path, &Value::Object(evidence))
}

pub fn supersede_proposal(
    root: &Path,
    reason: &str,
    superseded_by: Option<&str>,
) -> Result<(), String> {
    let path = root.join(PROPOSAL_FILE);
    let mut proposal = load_json(&path)?;
    let status = get(&proposal, "status").clone();
    if !status_in(&status, &SUPERSEDABLE_STATUSES)? {
        return Err(format!(
            "a {} proposal cannot be superseded; create a new draft instead",
            str_of(&status)
        ));
    }
    if !completed_identifier(&Value::Str(reason.to_string())) {
        return Err(
            "superseding a proposal requires a reason: what its conclusion got wrong, or what replaced it"
                .to_string(),
        );
    }
    if let Some(replacement) = superseded_by
        && !completed_identifier(&Value::Str(replacement.to_string()))
    {
        return Err("superseded_by must name the proposal that replaces this one".to_string());
    }
    proposal.insert("superseded_from_status", status);
    proposal.insert("status", Value::Str("superseded".to_string()));
    proposal.insert("superseded_at", Value::Str(utc_now()));
    proposal.insert("superseded_reason", Value::Str(reason.to_string()));
    if let Some(replacement) = superseded_by {
        proposal.insert("superseded_by", Value::Str(replacement.to_string()));
    }
    write_json(&path, &Value::Object(proposal))
}

pub struct Approval<'a> {
    pub role: &'a str,
    pub reviewer: &'a str,
    pub scope: &'a str,
    pub approved_at: Option<&'a str>,
}

pub fn record_approval(root: &Path, approval: &Approval) -> Result<(), String> {
    let path = root.join(PROPOSAL_FILE);
    let mut proposal = load_json(&path)?;
    if get(&proposal, "status").as_str() != Some("submitted") {
        return Err("approvals may be recorded only for a submitted proposal".to_string());
    }
    let given = [approval.role, approval.reviewer, approval.scope];
    if !given
        .iter()
        .all(|value| completed_identifier(&Value::Str((*value).to_string())))
    {
        return Err("approval requires role, reviewer, and scope".to_string());
    }
    if get(&proposal, "proposer").as_str() == Some(approval.reviewer) {
        return Err("the proposer cannot approve the proposal".to_string());
    }
    let timestamp = match approval.approved_at {
        Some(given) if !given.is_empty() => given.to_string(),
        _ => utc_now(),
    };
    if !iso_timestamp(&Value::Str(timestamp.clone())) {
        return Err("approved_at must be an ISO-8601 timestamp".to_string());
    }
    if proposal.get("approvals").is_none() {
        proposal.insert("approvals", Value::Array(Vec::new()));
    }
    let revision = get(&proposal, "proposal_revision").clone();
    let base = get(&proposal, "base_registry_revision").clone();
    let Some(Value::Array(approvals)) = proposal.get_mut("approvals") else {
        let held = get(&proposal, "approvals");
        return Err(format!(
            "'{}' object has no attribute 'append'",
            type_name(held)
        ));
    };
    for entry in approvals.iter() {
        let Value::Object(entry) = entry else {
            return Err(format!(
                "'{}' object has no attribute 'get'",
                type_name(entry)
            ));
        };
        if get(entry, "role").as_str() == Some(approval.role)
            && equal(get(entry, "proposal_revision"), &revision)
        {
            return Err("the role already approved this proposal revision".to_string());
        }
    }
    let mut recorded = Object::new();
    recorded.insert("role", Value::Str(approval.role.to_string()));
    recorded.insert("reviewer", Value::Str(approval.reviewer.to_string()));
    recorded.insert("decision", Value::Str("approved".to_string()));
    recorded.insert("approved_at", Value::Str(timestamp));
    recorded.insert("scope", Value::Str(approval.scope.to_string()));
    recorded.insert("proposal_revision", revision);
    recorded.insert("base_registry_revision", base);
    approvals.push(Value::Object(recorded));
    write_json(&path, &Value::Object(proposal))
}

pub fn same_command(command: &str, check: &Value) -> bool {
    check.as_str().is_some_and(|text| {
        let words = |text: &str| -> Vec<String> {
            text.split(py::is_space)
                .filter(|word| !word.is_empty())
                .map(str::to_string)
                .collect()
        };
        words(command) == words(text)
    })
}

fn evidence_check_errors(evidence: &Object, entries: &[Value]) -> Result<Vec<String>, String> {
    let mut checks: Vec<(&Value, &Value)> = Vec::new();
    for entry in entries.iter().filter_map(Value::as_object) {
        let (id, check) = (get(entry, "id"), get(entry, "check"));
        crate::promotion::unhashable(id)?;
        match checks.iter_mut().find(|(known, _)| equal(known, id)) {
            Some(slot) => slot.1 = check,
            None => checks.push((id, check)),
        }
    }
    let mut errors = Vec::new();
    for result in iterated(evidence.get("test_results"))? {
        let Value::Object(result) = &result else {
            continue;
        };
        let id = get(result, "obligation_id");
        crate::promotion::unhashable(id)?;
        let Some((_, check)) = checks.iter().find(|(known, _)| equal(known, id)) else {
            continue;
        };
        let ran = str_of(get(result, "evidence"));
        if ran.starts_with(CI_LINK_PREFIX) || same_command(&ran, check) {
            continue;
        }
        errors.push(format!(
            "the test result of {} ran {}, not the obligation's check {}; record the check, or cite the CI run as an https:// link",
            str_of(id),
            repr_of(get(result, "evidence")),
            repr_of(check)
        ));
    }
    Ok(errors)
}

pub fn verify_proposal(root: &Path, registry_root: &Path, repo_root: &Path) -> Result<(), String> {
    let path = root.join(PROPOSAL_FILE);
    let mut proposal = load_json(&path)?;
    if get(&proposal, "status").as_str() != Some("submitted") {
        return Err("only a submitted proposal may be verified".to_string());
    }
    let missing = missing_approval_roles(&load_json(&root.join(REQUIREMENT_FILE))?, &proposal);
    if !missing.is_empty() {
        return Err(format!(
            "a proposal is verified after its required approvals, which can be recorded only while it is submitted; first run record-approval for each of: {}",
            missing.join(", ")
        ));
    }
    require_current_registry_revision(&proposal, registry_root, repo_root).map_err(String::from)?;
    let entries = obligation_entries(&load_json(&root.join(OBLIGATIONS_FILE))?)?;
    let obligation_ids: BTreeSet<String> = entries
        .iter()
        .filter_map(Value::as_object)
        .filter(|entry| completed_identifier(get(entry, "id")))
        .map(|entry| str_of(get(entry, "id")))
        .collect();
    let evidence = load_json(&root.join(EVIDENCE_FILE))?;
    let mut errors = test_attestation_errors(&evidence, &obligation_ids, Some(registry_root))?;
    errors.extend(evidence_check_errors(&evidence, &entries)?);
    if !errors.is_empty() {
        return Err(format!(
            "proposal lacks verified test evidence: {}",
            errors.join("; ")
        ));
    }
    let errors = validate_change_package(root, Some(registry_root), Some("verified"), None)?;
    if !errors.is_empty() {
        return Err(format!(
            "verified proposal is invalid: {}",
            errors.join("; ")
        ));
    }
    proposal.insert("status", Value::Str("verified".to_string()));
    proposal.insert("verified_at", Value::Str(utc_now()));
    write_json(&path, &Value::Object(proposal))
}

pub struct ObligationRun<'a> {
    pub obligation_id: &'a str,
    pub command_profile: &'a str,
    pub cwd: Option<&'a str>,
}

fn tail_chars(text: &str, count: usize) -> &str {
    let total = text.chars().count();
    match text.char_indices().nth(total.saturating_sub(count)) {
        Some((at, _)) => &text[at..],
        None => text,
    }
}

fn obligation_command(obligation: &Object) -> Result<String, String> {
    match get(obligation, "check") {
        Value::Str(text) => Ok(py::strip(text).to_string()),
        other if !truthy(other) => Ok(String::new()),
        other => Err(format!(
            "'{}' object has no attribute 'strip'",
            type_name(other)
        )),
    }
}

pub fn record_test_result(
    root: &Path,
    registry_root: &Path,
    repo_root: &Path,
    run: &ObligationRun,
    timeout: i64,
) -> Result<(), String> {
    let status = get(&load_json(&root.join(PROPOSAL_FILE))?, "status").clone();
    if !status_in(&status, &RECORDABLE_STATUSES)? {
        return Err(format!(
            "the proposal is {}: its test results are what verify-proposal checked, so they stay as they are; redraft it to record new ones",
            str_of(&status)
        ));
    }
    let entries = obligation_entries(&load_json(&root.join(OBLIGATIONS_FILE))?)?;
    let Some(obligation) = find_record(&entries, &Value::Str(run.obligation_id.to_string())) else {
        return Err(format!(
            "test-obligations.json has no obligation {}",
            run.obligation_id
        ));
    };
    let command = obligation_command(obligation)?;
    let directory = working_directory(repo_root, run.cwd).map_err(|refusal| refusal.reason)?;
    let profiles = approved_command_profiles(Some(registry_root))?;
    if !profiles.is_empty() && !profiles.contains(run.command_profile) {
        let approved: Vec<&str> = profiles.iter().map(String::as_str).collect();
        return Err(format!(
            "command profile {} is not approved by Domain Memory policy; approved: {}",
            run.command_profile,
            approved.join(", ")
        ));
    }
    let argv = shlex::split(&command)?;
    let (stdout, stderr, code) = match run_captured(&directory, &argv, timeout) {
        Ok(Ran::TimedOut) => {
            return Err(format!(
                "{command} gave no result within {timeout} seconds; nothing recorded"
            ));
        }
        Err(RunFault::Start(error)) => {
            return Err(format!(
                "{command} could not start: {error}. The command runs as written, with no shell and no aliases, so give the full invocation, such as <path>/domain-memory verify-audit --registry-root <root>; nothing recorded"
            ));
        }
        Err(RunFault::Other(error)) => return Err(error),
        Ok(Ran::Finished {
            code,
            stdout,
            stderr,
        }) => (stdout, stderr, code),
    };
    let mut combined = stdout;
    combined.extend(stderr);
    let digest = format!("sha256:{}", hex_digest(&combined));
    let output = decoded(combined);
    if code != 0 {
        return Err(format!(
            "{command} exited {code}; only a passing run is recorded, and nothing was written:\n{}",
            tail_chars(&output, OUTPUT_TAIL)
        ));
    }
    if let Some(reason) = no_test_passed(&argv, &output) {
        return Err(format!(
            "{command} exited 0, but {reason}; nothing recorded"
        ));
    }
    let mut result = Object::new();
    result.insert("obligation_id", Value::Str(run.obligation_id.to_string()));
    result.insert("status", Value::Str("passed".to_string()));
    result.insert("evidence", Value::Str(command));
    result.insert(
        "command_profile",
        Value::Str(run.command_profile.to_string()),
    );
    result.insert("exit_code", Value::Int("0".to_string()));
    result.insert("output_sha256", Value::Str(digest));
    result.insert("finished_at", Value::Str(utc_now()));
    if run.cwd.is_some() {
        let relative = py::relative_to(&directory, &py::resolve_path(repo_root))?;
        let shown = if relative.is_empty() {
            ".".to_string()
        } else {
            relative
        };
        result.insert("working_directory", Value::Str(shown));
    }
    let evidence_path = root.join(EVIDENCE_FILE);
    let mut evidence = load_json(&evidence_path)?;
    let mut recorded: Vec<Value> = iterated(evidence.get("test_results"))?
        .into_iter()
        .filter(|entry| {
            !entry.as_object().is_some_and(|entry| {
                equal(
                    get(entry, "obligation_id"),
                    &Value::Str(run.obligation_id.to_string()),
                )
            })
        })
        .collect();
    recorded.push(Value::Object(result));
    evidence.insert("test_results", Value::Array(recorded));
    write_json(&evidence_path, &Value::Object(evidence))
}
