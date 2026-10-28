use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

use crate::changes::{
    init_change_package as create_package, missing_approval_roles, redraft_change_package,
    shifted_citation_notes, validate_change_package,
};
use crate::cli::Invocation;
use crate::common::{EVIDENCE_FILE, OBLIGATIONS_FILE, REQUIREMENT_FILE};
use crate::coverage::all_references;
use crate::files::{PROPOSAL_FILE, load_json};
use crate::hitl::rebase_proposal;
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::py::{self, PurePath};
use crate::pyregex;
use crate::pyvalue::{equal, str_of};
use crate::recitation_package::recitation_package;
use crate::records::read_record;
use crate::ship::ship_package;
use crate::{emit_stderr, emit_stdout, print_json};

const KNOWN_CONSISTENCIES: [&str; 2] = ["synchronous", "eventual"];

fn resolved(invocation: &Invocation, dest: &str) -> PathBuf {
    py::resolve(invocation.text(dest).unwrap_or("."))
}

fn optional(invocation: &Invocation, dest: &str) -> Option<PathBuf> {
    invocation.text(dest).map(py::resolve)
}

fn joined(given: &str, name: &str) -> PathBuf {
    PathBuf::from(PurePath::new(given).joined(name).native())
}

pub fn init_change_package(invocation: &Invocation) -> Result<i32, String> {
    if invocation.text("recitation").is_some() {
        return init_recitation_package(invocation);
    }
    let given = invocation.text("output").unwrap_or(".");
    let (requirement_id, proposal_id, proposer) = (
        invocation.text("requirement_id"),
        invocation.text("proposal_id"),
        invocation.text("proposer"),
    );
    create_package(&py::resolve(given), requirement_id, proposal_id, proposer)?;
    let missing: Vec<&str> = [
        ("--requirement-id", requirement_id),
        ("--proposal-id", proposal_id),
        ("--proposer", proposer),
    ]
    .iter()
    .filter(|(_, value)| value.is_none())
    .map(|(flag, _)| *flag)
    .collect();
    if !missing.is_empty() {
        emit_stdout(&format!(
            "Created {}. Still blank: {}; validate-change-package refuses the package until they are filled, so pass them here next time or fill them now.\n",
            PurePath::new(given).as_posix(),
            missing.join(", ")
        ));
    }
    Ok(0)
}

fn init_recitation_package(invocation: &Invocation) -> Result<i32, String> {
    let (requirement_id, proposal_id, proposer) = (
        invocation.text("requirement_id"),
        invocation.text("proposal_id"),
        invocation.text("proposer"),
    );
    let (Some(requirement_id), Some(proposal_id), Some(proposer), Some(registry_root)) = (
        requirement_id,
        proposal_id,
        proposer,
        optional(invocation, "registry_root"),
    ) else {
        return Err(
            "--recitation needs --registry-root, --requirement-id, --proposal-id and --proposer, \
             so the package is complete when it is written"
                .to_string(),
        );
    };
    let given = invocation.text("output").unwrap_or(".");
    let recitation = load_json(&resolved(invocation, "recitation"))?;
    let blank = recitation_package(
        &py::resolve(given),
        &registry_root,
        &recitation,
        [requirement_id, proposal_id, proposer],
    )?;
    emit_stdout(&format!(
        "Created {}: a routine re-citation package; submit it, record its one test result, then review it.\n",
        PurePath::new(given).as_posix()
    ));
    if !blank.is_empty() {
        emit_stdout(&format!(
            "Still blank: {}; fill it before submit-proposal.\n",
            blank.join("; ")
        ));
    }
    Ok(0)
}

pub fn redraft_proposal(invocation: &Invocation) -> Result<i32, String> {
    redraft_change_package(
        &resolved(invocation, "package_root"),
        &resolved(invocation, "output"),
        invocation.text("proposal_id").unwrap_or_default(),
    )?;
    Ok(0)
}

pub fn rebase(invocation: &Invocation) -> Result<i32, String> {
    let reply = rebase_proposal(
        &resolved(invocation, "package_root"),
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
    )?;
    print_json(&Value::Object(reply));
    Ok(0)
}

pub fn ship(invocation: &Invocation) -> Result<i32, String> {
    let report = ship_package(
        &resolved(invocation, "package_root"),
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
        invocation.text("commit"),
    )?;
    let failed = report
        .get("errors")
        .and_then(Value::as_array)
        .is_some_and(|errors| !errors.is_empty());
    print_json(&Value::Object(report));
    Ok(i32::from(failed))
}

fn report_errors(errors: &[String], success: &str) -> i32 {
    if errors.is_empty() {
        emit_stdout(&format!("{success}\n"));
        return 0;
    }
    let lines: Vec<String> = errors
        .iter()
        .map(|error| format!("ERROR: {error}"))
        .collect();
    emit_stderr(&format!("{}\n", lines.join("\n")));
    1
}

fn print_notes(notes: &[String]) {
    for note in notes {
        emit_stderr(&format!("{note}\n"));
    }
}

fn unchecked_at(status: &str, package_root: &str) -> Result<&'static str, String> {
    if !matches!(status, "submitted" | "verified") {
        return Ok("");
    }
    let requirement = load_json(&joined(package_root, REQUIREMENT_FILE))?;
    let proposal = load_json(&joined(package_root, PROPOSAL_FILE))?;
    if missing_approval_roles(&requirement, &proposal).is_empty() {
        return Ok("");
    }
    Ok(", apart from the approvals, which are given only once it is submitted")
}

pub fn validate_package(invocation: &Invocation) -> Result<i32, String> {
    let given = invocation.text("package_root").unwrap_or(".");
    let registry_root = optional(invocation, "registry_root");
    let repo_root = optional(invocation, "repo_root");
    let as_status = invocation.text("as_status");
    let errors = validate_change_package(
        &py::resolve(given),
        registry_root.as_deref(),
        as_status,
        repo_root.as_deref(),
    )?;
    if let Some(repo_root) = &repo_root {
        let proposal = load_json(&joined(given, PROPOSAL_FILE))?;
        print_notes(&shifted_citation_notes(&proposal, repo_root)?);
    }
    let Some(status) = as_status else {
        let code = report_errors(&errors, "Change package is valid.");
        if code != 0 {
            return Ok(code);
        }
        let proposal = load_json(&joined(given, PROPOSAL_FILE))?;
        if !proposal.get("registry_updates").is_some_and(truthy) {
            emit_stdout(
                "NOTE: this package changes no Registry record; add the updates it is for, or supersede it.\n",
            );
        }
        note_counterfactuals_off_cited_files(given, registry_root.as_deref())?;
        print_notes(&package_consistency_notes(given)?);
        return Ok(0);
    };
    let unchecked = if errors.is_empty() {
        unchecked_at(status, given)?
    } else {
        ""
    };
    Ok(report_errors(
        &errors,
        &format!("Change package has everything {status} requires{unchecked}."),
    ))
}

fn consistency_notes(holders: Vec<(String, Value)>) -> Vec<String> {
    holders
        .iter()
        .filter_map(|(place, holder)| {
            let consistency = holder.as_object()?.get("consistency")?.as_str()?;
            (!KNOWN_CONSISTENCIES.contains(&consistency)).then(|| {
                format!(
                    "NOTE: {place} consistency \"{consistency}\" is not one of the known values {}; it is accepted as written, but a reader looking for those values misses it.",
                    KNOWN_CONSISTENCIES.join(", ")
                )
            })
        })
        .collect()
}

fn package_consistency_notes(package_root: &str) -> Result<Vec<String>, String> {
    let proposal = load_json(&joined(package_root, PROPOSAL_FILE))?;
    let name = PurePath::new(package_root).name();
    let mut holders = vec![(
        format!("{name} boundary_decision"),
        proposal
            .get("boundary_decision")
            .cloned()
            .unwrap_or(Value::Null),
    )];
    if let Some(Value::Array(updates)) = proposal.get("registry_updates") {
        for update in updates.iter().filter_map(Value::as_object) {
            if update.get("asset").and_then(Value::as_str) != Some("interactions") {
                continue;
            }
            let record = update.get("record").cloned().unwrap_or(Value::Null);
            if let Value::Object(held) = &record {
                let id = str_of(held.get("id").unwrap_or(&Value::Null));
                holders.push((format!("{name} interactions/{id}"), record.clone()));
            }
        }
    }
    Ok(consistency_notes(holders))
}

fn cited_paths(record: &Object) -> Result<BTreeSet<String>, String> {
    Ok(all_references(&Value::Object(record.clone()), 0)?
        .iter()
        .filter_map(Value::as_object)
        .filter_map(|reference| reference.get("path").and_then(Value::as_str))
        .map(str::to_string)
        .collect())
}

fn mutated_path(protection: &str) -> Option<String> {
    let pattern = pyregex::compile(r"^(?P<path>[^:\s]+):\d+: ").ok()?;
    let characters: Vec<char> = protection.chars().collect();
    let matched = pattern.match_at(&characters, 0)?;
    let (start, end) = matched.named("path")?;
    Some(characters[start..end].iter().collect())
}

fn note_counterfactuals_off_cited_files(
    package_root: &str,
    registry_root: Option<&Path>,
) -> Result<(), String> {
    let evidence = load_json(&joined(package_root, EVIDENCE_FILE))?;
    let checks: Vec<Object> = match evidence.get("counterfactual_check") {
        Some(Value::Array(items)) => items.iter().filter_map(Value::as_object).cloned().collect(),
        Some(Value::Object(only)) => vec![only.clone()],
        _ => Vec::new(),
    };
    let obligations = load_json(&joined(package_root, OBLIGATIONS_FILE))?;
    let obligations: Vec<&Object> = match obligations.get("obligations") {
        None => Vec::new(),
        Some(Value::Array(items)) => items.iter().filter_map(Value::as_object).collect(),
        Some(_) => Vec::new(),
    };
    let proposal = load_json(&joined(package_root, PROPOSAL_FILE))?;
    let updated: Vec<&Object> = match proposal.get("registry_updates") {
        Some(Value::Array(items)) => items
            .iter()
            .filter_map(Value::as_object)
            .filter(|update| update.get("asset").and_then(Value::as_str) == Some("rules"))
            .filter_map(|update| update.get("record")?.as_object())
            .collect(),
        _ => Vec::new(),
    };
    for check in &checks {
        let protection = match check.get("mutated_protection") {
            None => String::new(),
            Some(value) => str_of(value),
        };
        let wanted = check.get("obligation_id").unwrap_or(&Value::Null);
        let obligation = obligations
            .iter()
            .rev()
            .find(|entry| equal(entry.get("id").unwrap_or(&Value::Null), wanted));
        let (Some(path), Some(obligation)) = (mutated_path(&protection), obligation) else {
            continue;
        };
        let also: &[Value] = match obligation.get("also_covers") {
            Some(Value::Array(items)) => items,
            _ => &[],
        };
        let rule_ids: Vec<&str> = std::iter::once(*obligation)
            .chain(also.iter().filter_map(Value::as_object))
            .filter(|source| source.get("source_type").and_then(Value::as_str) == Some("rule"))
            .filter_map(|source| source.get("source_id")?.as_str())
            .collect();
        let mut cited: BTreeSet<String> = BTreeSet::new();
        for rule_id in &rule_ids {
            let in_package = updated
                .iter()
                .rev()
                .find(|record| record.get("id").and_then(Value::as_str) == Some(*rule_id))
                .filter(|record| truthy(&Value::Object((**record).clone())));
            let rule = match (in_package, registry_root) {
                (Some(record), _) => Some((*record).clone()),
                (None, Some(root)) => read_record(root, "rules", rule_id)?,
                (None, None) => None,
            };
            if let Some(rule) = rule.filter(|rule| truthy(&Value::Object(rule.clone()))) {
                cited.extend(cited_paths(&rule)?);
            }
        }
        if !rule_ids.is_empty() && !cited.contains(&path) {
            emit_stderr(&format!(
                "NOTE: the counterfactual proving {} mutates {path}, which the rule \
                 does not cite. That proves the rule only when the mutated line lies on the \
                 execution path of a cited line, as the query a cited call runs does; otherwise mutate a cited line.\n",
                rule_ids.join(", ")
            ));
        }
    }
    Ok(())
}
