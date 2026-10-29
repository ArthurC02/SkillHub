use crate::pyvalue::{equal, message_repr, message_text, str_of};
use std::collections::BTreeSet;
use std::path::Path;

use crate::attestations::verify_scm_value;
use crate::audit_append::utc_now;
use crate::candidate_evidence::{classify_all, source_map_for, unclassified_paths};
use crate::common::{
    CHANGE_CLASSIFICATIONS, CHANGE_PACKAGE_FILES, CHANGE_PACKAGE_FORMATS, EVIDENCE_FILE,
    OBLIGATION_SOURCE_TYPES, OBLIGATION_STATUSES, OBLIGATIONS_FILE, PROPOSAL_STATUSES,
    REQUIREMENT_FILE, RISK_FLAGS, invalid_choice, is_one_of, iso_timestamp, positive_whole_number,
};
use crate::coverage::{SourceKinds, all_references};
use crate::crossrefs::cross_reference_errors;
use crate::evidence::{self, Verifier, fields_beyond_evidence};
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::policy::approved_command_profiles;
use crate::proposals::{completed_identifier, completed_identifiers, superseded_ids};
use crate::py;
use crate::pyregex;
use crate::registry_write::without;
use crate::revision::{DIGEST_HEX_LENGTH, DIGEST_PREFIX, valid_digest, valid_registry_revision};
use crate::store::{ASSETS, Registry};

const SUBMITTED_OR_LATER: [&str; 4] = ["submitted", "verified", "approved", "applied"];
const VERIFIED_OR_LATER: [&str; 3] = ["verified", "approved", "applied"];
const APPROVED_OR_LATER: [&str; 2] = ["approved", "applied"];
const SETTLED_STATUSES: [&str; 2] = ["applied", "superseded"];
const BEHAVIOR_ASSETS: [&str; 3] = ["rules", "contracts", "aggregates"];
const DECISIONS_WITH_EVIDENCE: [&str; 2] = ["ownership_decision", "boundary_decision"];
const STAND_IN_TIME: &str = "1970-01-01T00:00:00Z";
const REVIEW_STATE: [(&str, &[&str]); 2] = [
    (
        PROPOSAL_FILE,
        &[
            "status",
            "proposal_revision",
            "submitted_at",
            "verified_at",
            "finalized_at",
            "applied_at",
            "base_registry_revision",
            "approvals",
        ],
    ),
    (
        EVIDENCE_FILE,
        &[
            "registry_revision",
            "approvals",
            "scm_attestation",
            "counterfactual_check",
            "test_results",
            "contract_checks",
            "architecture_checks",
            "security_checks",
        ],
    ),
];
const STATE_AFTER_SUBMISSION: [&str; 5] = [
    "applied_registry_revision",
    "superseded_from_status",
    "superseded_at",
    "superseded_reason",
    "superseded_by",
];
const TEMPLATES: [(&str, &str); 5] = [
    (
        REQUIREMENT_FILE,
        include_str!("../../templates/requirement-normalization.json"),
    ),
    (
        PROPOSAL_FILE,
        include_str!("../../templates/domain-change-proposal.json"),
    ),
    (
        OBLIGATIONS_FILE,
        include_str!("../../templates/test-obligations.json"),
    ),
    (
        EVIDENCE_FILE,
        include_str!("../../templates/evidence-bundle.json"),
    ),
    ("draft-pr.md", include_str!("../../templates/draft-pr.md")),
];

fn get<'a>(object: &'a Object, key: &str) -> &'a Value {
    object.get(key).unwrap_or(&Value::Null)
}

fn status_in(status: &Value, among: &[&str]) -> bool {
    status.as_str().is_some_and(|text| among.contains(&text))
}

pub struct ParsedUpdate {
    pub asset: String,
    pub record_id: String,
    pub record: Option<Object>,
}

pub fn parsed_update(update: &Value) -> Result<ParsedUpdate, String> {
    let (asset, record_id, record) = evidence::parsed_update(update)?;
    Ok(ParsedUpdate {
        asset,
        record_id,
        record,
    })
}

fn parsed_updates(proposal: &Object) -> Vec<ParsedUpdate> {
    match proposal.get("registry_updates") {
        Some(Value::Array(items)) => items
            .iter()
            .filter_map(|raw| parsed_update(raw).ok())
            .collect(),
        _ => Vec::new(),
    }
}

fn registry_update_errors(proposal: &Object) -> Vec<String> {
    match proposal.get("registry_updates") {
        None | Some(Value::Null) => Vec::new(),
        Some(Value::Array(items)) => items
            .iter()
            .filter_map(|update| parsed_update(update).err())
            .collect(),
        Some(_) => vec!["domain-change-proposal.json registry_updates must be a list".to_string()],
    }
}

pub struct Documents {
    pub requirement: Object,
    pub proposal: Object,
    pub obligations: Object,
    pub evidence: Object,
}

impl Documents {
    fn load(root: &Path) -> Result<Self, String> {
        Ok(Self {
            requirement: load_json(&root.join(REQUIREMENT_FILE))?,
            proposal: load_json(&root.join(PROPOSAL_FILE))?,
            obligations: load_json(&root.join(OBLIGATIONS_FILE))?,
            evidence: load_json(&root.join(EVIDENCE_FILE))?,
        })
    }

    fn named(&self, name: &str) -> &Object {
        match name {
            REQUIREMENT_FILE => &self.requirement,
            PROPOSAL_FILE => &self.proposal,
            OBLIGATIONS_FILE => &self.obligations,
            _ => &self.evidence,
        }
    }
}

fn stand_in_revision() -> Value {
    let mut revision = Object::new();
    revision.insert("observed_commit", Value::Null);
    revision.insert(
        "registry_digest",
        Value::Str(format!("{DIGEST_PREFIX}{}", "0".repeat(DIGEST_HEX_LENGTH))),
    );
    Value::Object(revision)
}

fn as_if_moved_to(status: &str, mut documents: Documents) -> Documents {
    documents
        .proposal
        .insert("status", Value::Str(status.to_string()));
    if SUBMITTED_OR_LATER.contains(&status)
        && !valid_registry_revision(documents.proposal.get("base_registry_revision"))
    {
        documents
            .proposal
            .insert("base_registry_revision", stand_in_revision());
        documents
            .evidence
            .insert("registry_revision", stand_in_revision());
    }
    if status == "verified" && !iso_timestamp(get(&documents.proposal, "verified_at")) {
        documents
            .proposal
            .insert("verified_at", Value::Str(STAND_IN_TIME.to_string()));
    }
    if status == "applied" {
        if !iso_timestamp(get(&documents.proposal, "applied_at")) {
            documents
                .proposal
                .insert("applied_at", Value::Str(STAND_IN_TIME.to_string()));
        }
        if !valid_registry_revision(documents.proposal.get("applied_registry_revision")) {
            documents
                .proposal
                .insert("applied_registry_revision", stand_in_revision());
        }
    }
    documents
}

pub fn test_attestation_errors(
    evidence: &Object,
    obligation_ids: &BTreeSet<String>,
    registry_root: Option<&Path>,
) -> Result<Vec<String>, String> {
    let results: &[Value] = match evidence.get("test_results") {
        None => &[],
        Some(Value::Array(items)) => items,
        Some(_) => {
            return Ok(vec![
                "evidence-bundle.json has invalid test_results".to_string(),
            ]);
        }
    };
    let mut errors = Vec::new();
    let profiles = approved_command_profiles(registry_root)?;
    let mut completed: BTreeSet<String> = BTreeSet::new();
    for result in results {
        let passed = result.as_object().filter(|result| {
            get(result, "status").as_str() == Some("passed")
                && completed_identifier(get(result, "obligation_id"))
        });
        let Some(result) = passed else {
            errors.push("each verified test result requires a passed obligation_id".to_string());
            continue;
        };
        let obligation_id = str_of(get(result, "obligation_id"));
        if !obligation_ids.contains(&obligation_id) {
            errors.push("a test result references an unknown obligation".to_string());
            continue;
        }
        if completed.contains(&obligation_id) {
            errors.push("a test obligation may have only one passing attestation".to_string());
        }
        completed.insert(obligation_id);
        if !completed_identifier(get(result, "evidence"))
            || !completed_identifier(get(result, "command_profile"))
        {
            errors.push("a verified test result requires evidence and command_profile".to_string());
        }
        if !profiles.is_empty()
            && !get(result, "command_profile")
                .as_str()
                .is_some_and(|profile| profiles.iter().any(|known| known == profile))
        {
            errors.push(
                "a test result uses a command profile not approved by Domain Memory policy"
                    .to_string(),
            );
        }
        if !equal(get(result, "exit_code"), &Value::Int("0".to_string()))
            || !valid_digest(get(result, "output_sha256"))
            || !iso_timestamp(get(result, "finished_at"))
        {
            errors.push(
                "a verified test result requires exit_code 0, output_sha256, and finished_at"
                    .to_string(),
            );
        }
    }
    if obligation_ids.iter().any(|id| !completed.contains(id)) {
        errors.push("every test obligation requires a passing attestation".to_string());
    }
    Ok(errors)
}

fn directory_has_entries(output: &Path) -> Result<bool, String> {
    if !output.exists() {
        return Ok(false);
    }
    let mut entries = std::fs::read_dir(output).map_err(|error| py::os_error(&error, output))?;
    Ok(entries.next().is_some())
}

fn refuse_used_proposal_id(output: &Path, proposal_id: &str) -> Result<(), String> {
    let resolved = py::resolve_path(output);
    let siblings = resolved.parent().unwrap_or(&resolved).to_path_buf();
    if !siblings.is_dir() {
        return Ok(());
    }
    let entries = std::fs::read_dir(&siblings).map_err(|error| py::os_error(&error, &siblings))?;
    for entry in entries {
        let name = entry
            .map_err(|error| py::os_error(&error, &siblings))?
            .file_name();
        let package = siblings.join(&name);
        if !package.is_dir() {
            continue;
        }
        let Ok(proposal) = load_json(&package.join(PROPOSAL_FILE)) else {
            continue;
        };
        let used = proposal.get("proposal_id").and_then(Value::as_str) == Some(proposal_id);
        if used && py::compare_paths(&py::resolve_path(&package), &resolved).is_ne() {
            return Err(format!(
                "proposal_id {proposal_id} is already used by {}; choose another",
                name.to_string_lossy()
            ));
        }
    }
    Ok(())
}

fn with_whole_package(
    output: &Path,
    build: impl FnOnce() -> Result<(), String>,
) -> Result<(), String> {
    let existed = output.exists();
    let outcome = build();
    if outcome.is_err() {
        let _ = std::fs::remove_dir_all(output);
        if existed {
            let _ = std::fs::create_dir(output);
        }
    }
    outcome
}

fn fill_identifiers(output: &Path, given: &[(&str, &str)]) -> Result<(), String> {
    for (name, _) in CHANGE_PACKAGE_FORMATS {
        let mut document = load_json(&output.join(name))?;
        for (key, value) in given {
            if document.get(key).is_some() {
                document.insert(*key, Value::Str((*value).to_string()));
            }
        }
        write_json(&output.join(name), &Value::Object(document))?;
    }
    Ok(())
}

pub fn init_change_package(
    output: &Path,
    requirement_id: Option<&str>,
    proposal_id: Option<&str>,
    proposer: Option<&str>,
) -> Result<(), String> {
    let identifiers = [
        ("requirement_id", requirement_id),
        ("proposal_id", proposal_id),
        ("proposer", proposer),
    ];
    let blank: Vec<&str> = identifiers
        .iter()
        .filter(|(_, value)| {
            value.is_some_and(|text| !completed_identifier(&Value::Str(text.to_string())))
        })
        .map(|(key, _)| *key)
        .collect();
    if !blank.is_empty() {
        return Err(format!("{} must not be blank", blank.join(", ")));
    }
    if directory_has_entries(output)? {
        return Err(format!(
            "output directory is not empty: {}",
            py::shown(output)
        ));
    }
    if let Some(proposal_id) = proposal_id {
        refuse_used_proposal_id(output, proposal_id)?;
    }
    with_whole_package(output, || {
        std::fs::create_dir_all(output).map_err(|error| py::os_error(&error, output))?;
        for (name, text) in TEMPLATES {
            let target = output.join(name);
            std::fs::write(&target, text).map_err(|error| py::open_error(&error, &target))?;
        }
        let given: Vec<(&str, &str)> = identifiers
            .iter()
            .filter_map(|(key, value)| value.map(|text| (*key, text)))
            .collect();
        if given.is_empty() {
            return Ok(());
        }
        fill_identifiers(output, &given)
    })
}

fn copy_tree(source: &Path, target: &Path) -> Result<(), String> {
    std::fs::create_dir_all(target).map_err(|error| py::os_error(&error, target))?;
    let entries = std::fs::read_dir(source).map_err(|error| py::os_error(&error, source))?;
    for entry in entries {
        let entry = entry.map_err(|error| py::os_error(&error, source))?;
        let (from, to) = (entry.path(), target.join(entry.file_name()));
        if from.is_dir() {
            copy_tree(&from, &to)?;
        } else {
            std::fs::copy(&from, &to).map_err(|error| py::os_error(&error, &from))?;
        }
    }
    Ok(())
}

fn template_document(name: &str) -> Result<Object, String> {
    let text = TEMPLATES
        .iter()
        .find(|(known, _)| *known == name)
        .map_or("", |(_, text)| text);
    match crate::json::parse(text) {
        Ok(Value::Object(document)) => Ok(document),
        _ => Err(format!("invalid JSON: {name}")),
    }
}

fn reset_review_state(output: &Path, replaced: &Value) -> Result<(), String> {
    for (name, fields) in REVIEW_STATE {
        let mut document = load_json(&output.join(name))?;
        let template = template_document(name)?;
        for field in fields {
            document.insert(*field, get(&template, field).clone());
        }
        let mut document = without(&document, &STATE_AFTER_SUBMISSION);
        if name == PROPOSAL_FILE {
            let mut named: BTreeSet<String> = superseded_ids(&document).unwrap_or_default();
            if let Value::Str(text) = replaced {
                named.insert(text.clone());
            }
            let mut listed: Vec<Value> = named.into_iter().map(Value::Str).collect();
            if !matches!(replaced, Value::Str(_) | Value::Null) && listed.is_empty() {
                listed.push(replaced.clone());
            }
            document.insert("supersedes", Value::Array(listed));
            document.insert("redrafted_at", Value::Str(utc_now()));
        }
        write_json(&output.join(name), &Value::Object(document))?;
    }
    Ok(())
}

pub fn redraft_change_package(
    source: &Path,
    output: &Path,
    proposal_id: &str,
) -> Result<(), String> {
    let proposal = load_json(&source.join(PROPOSAL_FILE))?;
    if get(&proposal, "status").as_str() != Some("superseded") {
        return Err(
            "only a superseded proposal is redrafted; supersede it first with its reason, \
             so its own record says why it was replaced"
                .to_string(),
        );
    }
    if !completed_identifier(&Value::Str(proposal_id.to_string())) {
        return Err("proposal_id must not be blank".to_string());
    }
    if directory_has_entries(output)? {
        return Err(format!(
            "output directory is not empty: {}",
            py::shown(output)
        ));
    }
    refuse_used_proposal_id(output, proposal_id)?;
    let replaced = get(&load_json(&source.join(PROPOSAL_FILE))?, "proposal_id").clone();
    with_whole_package(output, || {
        copy_tree(source, output)?;
        fill_identifiers(output, &[("proposal_id", proposal_id)])?;
        reset_review_state(output, &replaced)
    })
}

fn design_field_errors(
    design: &Object,
    obligation_ids: &BTreeSet<String>,
    classification: &str,
) -> Vec<String> {
    let mut errors = Vec::new();
    for field in [
        "domain_forces",
        "invariants_preserved",
        "rejected_alternatives",
    ] {
        let values = get(design, field);
        if !completed_identifiers(values) {
            errors.push(format!(
                "implementation_design requires a valid {field} list"
            ));
        } else if classification == "material" && !truthy(values) {
            errors.push(format!(
                "a material proposal requires at least one entry in {field}"
            ));
        }
    }
    if !completed_identifier(get(design, "decision")) {
        errors.push("implementation_design requires a completed decision".to_string());
    }
    let proof_obligations = get(design, "proof_obligations");
    if !completed_identifiers(proof_obligations) {
        errors.push("implementation_design requires proof obligation IDs".to_string());
    } else if proof_obligations
        .as_array()
        .unwrap_or_default()
        .iter()
        .any(|id| !obligation_ids.contains(&str_of(id)))
    {
        errors.push("implementation_design references an unknown proof obligation".to_string());
    }
    if !completed_identifier(get(design, "counterfactual_check")) {
        errors.push("implementation_design requires a counterfactual check".to_string());
    }
    if classification == "material" && !truthy(proof_obligations) {
        errors.push("a material proposal requires at least one proof obligation".to_string());
    }
    errors
}

fn implementation_design_errors(
    proposal: &Object,
    obligation_ids: &BTreeSet<String>,
) -> Vec<String> {
    let classification = get(proposal, "change_classification");
    let Some(kind) = classification
        .as_str()
        .filter(|kind| CHANGE_CLASSIFICATIONS.contains(kind))
    else {
        return vec![invalid_choice(
            PROPOSAL_FILE,
            "change_classification",
            classification,
            &CHANGE_CLASSIFICATIONS,
        )];
    };
    if let Value::Object(design) = get(proposal, "implementation_design") {
        return design_field_errors(design, obligation_ids, kind);
    }
    if kind == "material" {
        return vec!["a material proposal requires implementation_design with domain forces, decision, invariants, alternatives, proof obligations, and counterfactual check".to_string()];
    }
    Vec::new()
}

fn changes_behavior(proposal: &Object) -> bool {
    truthy(get(proposal, "rule_ids"))
        || truthy(get(proposal, "contract_ids"))
        || match proposal.get("registry_updates") {
            Some(Value::Array(updates)) => updates.iter().any(|update| {
                update
                    .as_object()
                    .and_then(|update| update.get("asset"))
                    .and_then(Value::as_str)
                    .is_some_and(|asset| BEHAVIOR_ASSETS.contains(&asset))
            }),
            _ => false,
        }
}

fn inapplicable_counterfactual_errors(check: &Object, proposal: &Object) -> Vec<String> {
    if changes_behavior(proposal) {
        return vec![
            "counterfactual_check may be not_applicable only when the proposal changes no rule, \
             contract, or aggregate; a changed rule needs a test shown to fail without it"
                .to_string(),
        ];
    }
    if !completed_identifier(get(check, "reason")) {
        return vec!["a not_applicable counterfactual_check requires a reason".to_string()];
    }
    Vec::new()
}

pub fn counterfactual_errors(
    evidence: &Object,
    obligation_ids: &BTreeSet<String>,
    proposal: &Object,
) -> Vec<String> {
    let check = get(evidence, "counterfactual_check");
    if let Value::Object(only) = check
        && get(only, "status").as_str() == Some("not_applicable")
    {
        return inapplicable_counterfactual_errors(only, proposal);
    }
    let checks: Vec<&Value> = match check {
        Value::Array(items) => items.iter().collect(),
        other => vec![other],
    };
    let all_objects = checks.iter().all(|entry| matches!(entry, Value::Object(_)));
    if checks.is_empty() || !all_objects {
        return vec![
            "a material verified proposal requires counterfactual_check evidence".to_string(),
        ];
    }
    let checks: Vec<&Object> = checks.into_iter().filter_map(Value::as_object).collect();
    let targets: Vec<String> = checks
        .iter()
        .map(|entry| str_of(get(entry, "obligation_id")))
        .collect();
    if targets.iter().collect::<BTreeSet<_>>().len() < targets.len() {
        return vec![
            "each counterfactual_check in a list targets a different obligation".to_string(),
        ];
    }
    checks
        .iter()
        .flat_map(|entry| killed_counterfactual_errors(entry, obligation_ids, proposal))
        .collect()
}

fn killed_counterfactual_errors(
    check: &Object,
    obligation_ids: &BTreeSet<String>,
    proposal: &Object,
) -> Vec<String> {
    let planned = proposal
        .get("implementation_design")
        .and_then(Value::as_object)
        .and_then(|design| design.get("proof_obligations"))
        .and_then(Value::as_array);
    let target = get(check, "obligation_id");
    if !status_in(get(proposal, "status"), &SETTLED_STATUSES)
        && let Some(planned) = planned
        && !planned.iter().any(|id| equal(id, target))
    {
        return vec![
            "counterfactual_check must target one of implementation_design's proof_obligations"
                .to_string(),
        ];
    }
    let known = target
        .as_str()
        .is_some_and(|id| obligation_ids.contains(id));
    if get(check, "status").as_str() != Some("passed")
        || get(check, "restored") != &Value::Bool(true)
        || !known
        || !completed_identifier(get(check, "mutated_protection"))
        || !completed_identifier(get(check, "failure_evidence"))
    {
        return vec!["counterfactual_check requires passed status, a known obligation_id, mutated_protection, failure_evidence, and restored=true".to_string()];
    }
    Vec::new()
}

fn obligation_source_error(obligation_id: &str, source: &Object) -> Option<String> {
    let source_type = get(source, "source_type");
    if !is_one_of(Some(source_type), &OBLIGATION_SOURCE_TYPES) {
        return Some(format!(
            "test obligation {obligation_id} has source_type {}; a source_type names what the \
             obligation is derived from, not the kind of check, and must be one of {}",
            message_repr(source_type),
            OBLIGATION_SOURCE_TYPES.join(", ")
        ));
    }
    if !completed_identifier(get(source, "source_id")) {
        return Some(format!(
            "test obligation {obligation_id} has an incomplete source_id: {}",
            message_repr(get(source, "source_id"))
        ));
    }
    None
}

type Source = (String, String);

fn named_sources(
    criterion_ids: &[String],
    rule_ids: &[Value],
    contract_ids: &[Value],
) -> Vec<(String, Value)> {
    let mut named: Vec<(String, Value)> = Vec::new();
    for id in criterion_ids {
        named.push(("acceptance-criterion".to_string(), Value::Str(id.clone())));
    }
    for id in rule_ids {
        named.push(("rule".to_string(), id.clone()));
    }
    for id in contract_ids {
        named.push(("contract".to_string(), id.clone()));
    }
    named
}

fn has_source(sources: &[(String, Value)], wanted: &Source) -> bool {
    sources
        .iter()
        .any(|(kind, id)| *kind == wanted.0 && id.as_str() == Some(wanted.1.as_str()))
}

fn unnamed_also_covered(
    obligation_id: &str,
    also_covers: &[&Object],
    named: &[(String, Value)],
) -> Vec<String> {
    also_covers
        .iter()
        .filter(|source| {
            get(source, "source_type").as_str() != Some("invariant")
                && obligation_source_error(obligation_id, source).is_none()
                && !has_source(
                    named,
                    &(
                        str_of(get(source, "source_type")),
                        str_of(get(source, "source_id")),
                    ),
                )
        })
        .map(|source| {
            format!(
                "test obligation {obligation_id} lists {} {} in also_covers, but the \
                 proposal names no such {}; also_covers counts only toward the acceptance criteria and \
                 the rule_ids and contract_ids the proposal names",
                str_of(get(source, "source_type")),
                str_of(get(source, "source_id")),
                str_of(get(source, "source_type"))
            )
        })
        .collect()
}

fn obligation_errors(
    obligations: &Object,
    criterion_ids: &[String],
    rule_ids: &[Value],
    contract_ids: &[Value],
) -> (Vec<String>, BTreeSet<String>) {
    let records: &[Value] = match obligations.get("obligations") {
        Some(Value::Array(items)) if !items.is_empty() => items,
        _ => {
            return (
                vec!["test-obligations.json requires obligations".to_string()],
                BTreeSet::new(),
            );
        }
    };
    let mut errors = Vec::new();
    let mut obligation_ids: BTreeSet<String> = BTreeSet::new();
    let mut obligation_sources: Vec<(String, Value)> = Vec::new();
    let named = named_sources(criterion_ids, rule_ids, contract_ids);
    for record in records {
        let Some(obligation) = record
            .as_object()
            .filter(|obligation| completed_identifier(get(obligation, "id")))
        else {
            errors.push("each test obligation requires a completed id".to_string());
            continue;
        };
        let obligation_id = str_of(get(obligation, "id"));
        if obligation_ids.contains(&obligation_id) {
            errors.push("test-obligations.json contains duplicate obligation id".to_string());
        }
        obligation_ids.insert(obligation_id.clone());
        let mut also_covers: Vec<&Object> = Vec::new();
        match obligation.get("also_covers") {
            None => {}
            Some(Value::Array(items))
                if items.iter().all(|item| matches!(item, Value::Object(_))) =>
            {
                also_covers = items.iter().filter_map(Value::as_object).collect();
            }
            Some(_) => errors.push(format!(
                "test obligation {obligation_id} has also_covers that is not a list of sources"
            )),
        }
        for source in std::iter::once(obligation).chain(also_covers.iter().copied()) {
            match obligation_source_error(&obligation_id, source) {
                Some(error) => errors.push(error),
                None => obligation_sources.push((
                    str_of(get(source, "source_type")),
                    get(source, "source_id").clone(),
                )),
            }
        }
        errors.extend(unnamed_also_covered(&obligation_id, &also_covers, &named));
        if !completed_identifier(get(obligation, "expected_outcome"))
            || !completed_identifier(get(obligation, "check"))
        {
            errors.push(format!(
                "test obligation {obligation_id} requires expected_outcome and check"
            ));
        }
        if !is_one_of(obligation.get("status"), &OBLIGATION_STATUSES) {
            errors.push(invalid_choice(
                &format!("test obligation {obligation_id}"),
                "status",
                get(obligation, "status"),
                &OBLIGATION_STATUSES,
            ));
        }
    }
    for criterion_id in criterion_ids {
        if !has_source(
            &obligation_sources,
            &("acceptance-criterion".to_string(), criterion_id.clone()),
        ) {
            errors.push(format!(
                "acceptance criterion {criterion_id} lacks a test obligation: \
                 add one whose source_type is acceptance-criterion and whose source_id is {criterion_id}"
            ));
        }
    }
    for (source_type, source_ids) in [("rule", rule_ids), ("contract", contract_ids)] {
        for source_id in source_ids {
            let held = obligation_sources
                .iter()
                .any(|(kind, id)| kind == source_type && equal(id, source_id));
            if !held {
                errors.push(format!(
                    "{source_type} {} lacks a test obligation",
                    str_of(source_id)
                ));
            }
        }
    }
    (errors, obligation_ids)
}

fn counts_as_approval(approval: &Value, proposal: &Object) -> bool {
    approval.as_object().is_some_and(|approval| {
        get(approval, "decision").as_str() == Some("approved")
            && equal(
                get(approval, "proposal_revision"),
                get(proposal, "proposal_revision"),
            )
            && equal(
                get(approval, "base_registry_revision"),
                get(proposal, "base_registry_revision"),
            )
            && signed_in_full(approval)
    })
}

fn signed_in_full(approval: &Object) -> bool {
    ["role", "reviewer", "scope"]
        .iter()
        .all(|field| completed_identifier(get(approval, field)))
        && iso_timestamp(get(approval, "approved_at"))
}

pub fn missing_approval_roles(requirement: &Object, proposal: &Object) -> Vec<String> {
    let approved: BTreeSet<String> = match get(proposal, "approvals") {
        Value::Array(approvals) => approvals
            .iter()
            .filter(|approval| counts_as_approval(approval, proposal))
            .filter_map(|approval| approval.as_object())
            .map(|approval| str_of(get(approval, "role")))
            .collect(),
        _ => BTreeSet::new(),
    };
    let required: BTreeSet<String> = match requirement.get("required_approval_roles") {
        None => BTreeSet::new(),
        Some(Value::Array(roles)) => roles.iter().map(str_of).collect(),
        Some(_) => BTreeSet::new(),
    };
    required.difference(&approved).cloned().collect()
}

fn approval_entry_error(
    approval: &Value,
    revision: &Value,
    base_revision: &Value,
) -> Option<String> {
    let Value::Object(approval) = approval else {
        return Some("domain-change-proposal.json has invalid approval entry".to_string());
    };
    if get(approval, "decision").as_str() != Some("approved")
        || !equal(get(approval, "proposal_revision"), revision)
    {
        return None;
    }
    if !signed_in_full(approval) {
        return Some("an approval requires role, reviewer, approved_at, and scope".to_string());
    }
    if !equal(get(approval, "base_registry_revision"), base_revision) {
        return Some("an approval must bind to the proposal base_registry_revision".to_string());
    }
    None
}

fn approval_list_errors(proposal: &Object, revision: &Value) -> Vec<String> {
    let approvals: &[Value] = match proposal.get("approvals") {
        None => &[],
        Some(Value::Array(items)) => items,
        Some(_) => return vec!["domain-change-proposal.json has invalid approvals".to_string()],
    };
    let base_revision = get(proposal, "base_registry_revision");
    approvals
        .iter()
        .filter_map(|approval| approval_entry_error(approval, revision, base_revision))
        .collect()
}

fn lifecycle_field_errors(
    requirement: &Object,
    proposal: &Object,
    evidence: &Object,
    status: &Value,
) -> Vec<String> {
    let mut errors = Vec::new();
    let base_revision = get(proposal, "base_registry_revision");
    let missing = missing_approval_roles(requirement, proposal);
    if status_in(status, &APPROVED_OR_LATER) && !missing.is_empty() {
        errors.push(format!(
            "an approved or applied proposal lacks required approvals: {}",
            missing.join(", ")
        ));
    }
    if status_in(status, &SUBMITTED_OR_LATER)
        && !valid_registry_revision(proposal.get("base_registry_revision"))
    {
        errors.push(
            "a submitted, approved, or applied proposal requires a Registry digest base revision"
                .to_string(),
        );
    }
    if status_in(status, &SUBMITTED_OR_LATER)
        && !equal(get(evidence, "registry_revision"), base_revision)
    {
        errors.push(
            "evidence-bundle.json must retain the proposal base_registry_revision".to_string(),
        );
    }
    if status.as_str() == Some("verified") && !iso_timestamp(get(proposal, "verified_at")) {
        errors.push("a verified proposal requires verified_at".to_string());
    }
    if status.as_str() == Some("applied") {
        if !iso_timestamp(get(proposal, "applied_at")) {
            errors.push("an applied proposal requires applied_at".to_string());
        }
        if !valid_registry_revision(proposal.get("applied_registry_revision")) {
            errors.push("an applied proposal requires applied_registry_revision".to_string());
        }
    }
    errors
}

fn approval_and_revision_errors(
    documents: &Documents,
    status: &Value,
    revision: &Value,
) -> Vec<String> {
    let mut errors = Vec::new();
    let roles_ok = match documents.requirement.get("required_approval_roles") {
        None => true,
        Some(roles) => completed_identifiers(roles),
    };
    if !roles_ok {
        errors
            .push("requirement-normalization.json has invalid required_approval_roles".to_string());
    }
    errors.extend(approval_list_errors(&documents.proposal, revision));
    errors.extend(lifecycle_field_errors(
        &documents.requirement,
        &documents.proposal,
        &documents.evidence,
        status,
    ));
    errors
}

fn evidence_result_errors(evidence: &Object, obligation_ids: &BTreeSet<String>) -> Vec<String> {
    let results: &[Value] = match evidence.get("test_results") {
        None => &[],
        Some(Value::Array(items)) => items,
        Some(_) => return vec!["evidence-bundle.json has invalid test_results".to_string()],
    };
    let mut errors = Vec::new();
    for result in results {
        let complete = result.as_object().filter(|result| {
            get(result, "status").as_str() == Some("passed")
                && completed_identifier(get(result, "obligation_id"))
                && completed_identifier(get(result, "evidence"))
        });
        let Some(result) = complete else {
            errors
                .push("each test result requires a passed obligation_id and evidence".to_string());
            continue;
        };
        if !obligation_ids.contains(&str_of(get(result, "obligation_id"))) {
            errors.push("a test result references an unknown obligation".to_string());
        }
    }
    errors
}

fn registry_reference_errors(staged: &Staged, proposal: &Object) -> Vec<String> {
    let mut errors = Vec::new();
    for (asset, kind, referenced) in [
        ("contexts", "context", proposal.get("affected_contexts")),
        ("rules", "rule", proposal.get("rule_ids")),
        ("contracts", "contract", proposal.get("contract_ids")),
    ] {
        let known: Vec<&Value> = records_in(staged, asset)
            .iter()
            .map(|entry| get(entry, "id"))
            .collect();
        if let Some(Value::Array(referenced)) = referenced
            && referenced
                .iter()
                .any(|name| !known.iter().any(|id| equal(id, name)))
        {
            errors.push(format!(
                "domain-change-proposal.json references an unknown registry {kind}"
            ));
        }
    }
    errors
}

fn identity_errors(documents: &Documents) -> Vec<String> {
    let requirement_id = get(&documents.requirement, "requirement_id");
    let proposal_id = get(&documents.proposal, "proposal_id");
    let mut errors: Vec<String> = CHANGE_PACKAGE_FORMATS
        .iter()
        .filter(|(name, format)| get(documents.named(name), "format").as_str() != Some(*format))
        .map(|(name, _)| format!("{name} has an invalid format"))
        .collect();
    if !completed_identifier(requirement_id) {
        errors
            .push("requirement-normalization.json requires a completed requirement_id".to_string());
    }
    if !completed_identifier(proposal_id) {
        errors.push("domain-change-proposal.json requires a completed proposal_id".to_string());
    }
    if !completed_identifier(get(&documents.proposal, "proposer")) {
        errors.push("domain-change-proposal.json requires a completed proposer".to_string());
    }
    for name in [PROPOSAL_FILE, OBLIGATIONS_FILE, EVIDENCE_FILE] {
        if !equal(get(documents.named(name), "requirement_id"), requirement_id) {
            errors.push(format!("{name} must reference the requirement_id"));
        }
    }
    for name in [OBLIGATIONS_FILE, EVIDENCE_FILE] {
        if !equal(get(documents.named(name), "proposal_id"), proposal_id) {
            errors.push(format!("{name} must reference the proposal_id"));
        }
    }
    errors
}

fn acceptance_criteria(requirement: &Object) -> (Vec<String>, Vec<String>) {
    let criteria: &[Value] = match requirement.get("acceptance_criteria") {
        Some(Value::Array(items)) if !items.is_empty() => items,
        _ => {
            return (
                vec!["requirement-normalization.json requires acceptance_criteria".to_string()],
                Vec::new(),
            );
        }
    };
    let mut errors = Vec::new();
    let mut ids: Vec<String> = Vec::new();
    for criterion in criteria {
        let complete = criterion.as_object().filter(|criterion| {
            completed_identifier(get(criterion, "id"))
                && completed_identifier(get(criterion, "statement"))
        });
        let Some(criterion) = complete else {
            errors.push(
                "each acceptance criterion requires a completed id and statement".to_string(),
            );
            continue;
        };
        let id = str_of(get(criterion, "id"));
        if !ids.contains(&id) {
            ids.push(id);
        }
    }
    (errors, ids)
}

fn unknown_flags(flags: &Value) -> bool {
    match flags {
        Value::Array(items) => items
            .iter()
            .any(|flag| !flag.as_str().is_some_and(|name| RISK_FLAGS.contains(&name))),
        _ => true,
    }
}

fn risk_flag_errors(requirement: &Object, proposal: &Object) -> Vec<String> {
    let mut errors = Vec::new();
    let no_flags = Value::Array(Vec::new());
    let requirement_flags = requirement.get("risk_flags").unwrap_or(&no_flags);
    let proposal_flags = proposal.get("risk_flags").unwrap_or(&no_flags);
    if unknown_flags(requirement_flags) {
        errors.push("requirement-normalization.json has an unknown risk flag".to_string());
    }
    if unknown_flags(proposal_flags) {
        errors.push("domain-change-proposal.json has an unknown risk flag".to_string());
    } else if let (Value::Array(wanted), Value::Array(kept)) = (requirement_flags, proposal_flags)
        && wanted
            .iter()
            .any(|flag| !kept.iter().any(|other| equal(other, flag)))
    {
        errors.push("domain-change-proposal.json must retain requirement risk flags".to_string());
    }
    errors
}

fn identifier_list_ok(proposal: &Object, key: &str) -> bool {
    proposal.get(key).is_none_or(completed_identifiers)
}

fn proposal_field_errors(proposal: &Object) -> Vec<String> {
    let mut errors = Vec::new();
    if !is_one_of(proposal.get("status"), &PROPOSAL_STATUSES) {
        errors.push(invalid_choice(
            PROPOSAL_FILE,
            "status",
            get(proposal, "status"),
            &PROPOSAL_STATUSES,
        ));
    }
    if !positive_whole_number(proposal.get("proposal_revision")) {
        errors
            .push("domain-change-proposal.json requires a positive proposal_revision".to_string());
    }
    let contexts = get(proposal, "affected_contexts");
    if !truthy(contexts) || !completed_identifiers(contexts) {
        errors.push("domain-change-proposal.json requires affected_contexts".to_string());
    }
    if !identifier_list_ok(proposal, "rule_ids") {
        errors.push("domain-change-proposal.json has invalid rule_ids".to_string());
    }
    if !identifier_list_ok(proposal, "contract_ids") {
        errors.push("domain-change-proposal.json has invalid contract_ids".to_string());
    }
    if superseded_ids(proposal).is_none() {
        errors.push("domain-change-proposal.json supersedes must be a proposal id or a list of distinct proposal ids".to_string());
    }
    errors
}

type Staged = Vec<(&'static str, Vec<Object>)>;

fn records_in<'a>(staged: &'a Staged, asset: &str) -> &'a [Object] {
    staged
        .iter()
        .find(|(name, _)| *name == asset)
        .map_or(&[], |(_, records)| records)
}

fn current_records(registry_root: &Path) -> Result<Staged, String> {
    let registry = Registry::open(registry_root)?;
    let mut current = Vec::new();
    for (asset, _, _) in ASSETS {
        let held = registry
            .records(asset)?
            .into_iter()
            .filter_map(|record| record.as_object().cloned())
            .collect();
        current.push((*asset, held));
    }
    Ok(current)
}

fn staged_with(current: &Staged, proposal: &Object) -> Staged {
    let mut records = current.clone();
    for update in parsed_updates(proposal) {
        let Some((_, entries)) = records.iter_mut().find(|(name, _)| *name == update.asset) else {
            continue;
        };
        entries.retain(|entry| get(entry, "id").as_str() != Some(update.record_id.as_str()));
        if let Some(record) = update.record {
            entries.push(record);
        }
    }
    records
}

fn updated_evidence_errors(proposal: &Object, repo_root: &Path) -> Result<Vec<String>, String> {
    let mut errors = Vec::new();
    let mut check = Verifier::new(repo_root, false);
    let Some(Value::Array(updates)) = proposal.get("registry_updates") else {
        return Ok(errors);
    };
    for raw in updates {
        let Some((raw, record)) = raw
            .as_object()
            .and_then(|raw| Some((raw, raw.get("record")?.as_object()?)))
        else {
            continue;
        };
        for reference in all_references(&Value::Object(record.clone()), 0)? {
            let result = check.check(&reference)?;
            let status = get(&result, "status");
            if !status_in(status, &["current", "shifted"]) {
                let shown = ["reason", "path", "reference"]
                    .iter()
                    .map(|key| get(&result, key))
                    .find(|value| truthy(value))
                    .map_or_else(|| "null".to_string(), message_text);
                errors.push(format!(
                    "registry update {}/{} cites evidence that is {}: {shown}",
                    str_of(get(raw, "asset")),
                    str_of(get(record, "id")),
                    message_text(status)
                ));
            }
        }
    }
    Ok(errors)
}

fn cited_file(item: &Value) -> Option<String> {
    match item {
        Value::Object(object) => get(object, "path").as_str().map(str::to_string),
        Value::Str(text) => {
            let pattern = pyregex::compile(r"^(?P<path>[^\s:]+)(?::\d+(?:-\d+)?)?$").ok()?;
            let characters: Vec<char> = text.chars().collect();
            let matched = pattern.match_at(&characters, 0)?;
            let (start, end) = matched.named("path")?;
            let path: String = characters[start..end].iter().collect();
            path.contains('/').then_some(path)
        }
        _ => None,
    }
}

fn decision_evidence_errors(proposal: &Object, repo_root: &Path) -> Vec<String> {
    let mut errors = Vec::new();
    for decision in DECISIONS_WITH_EVIDENCE {
        let Some(Value::Array(evidence)) = proposal
            .get(decision)
            .and_then(Value::as_object)
            .and_then(|value| value.get("evidence"))
        else {
            continue;
        };
        for item in evidence {
            if let Some(path) = cited_file(item).filter(|path| !path.is_empty())
                && !repo_root.join(&path).is_file()
            {
                errors.push(format!(
                    "{decision} evidence names {path}, which is not a file in the repository"
                ));
            }
        }
    }
    errors
}

fn unverified_update_errors(current: &Staged, proposal: &Object) -> Result<Vec<String>, String> {
    let approved = match proposal.get("approvals") {
        Some(Value::Array(approvals)) => approvals.iter().any(|entry| {
            entry
                .as_object()
                .is_some_and(|entry| get(entry, "decision").as_str() == Some("approved"))
        }),
        _ => false,
    };
    if approved {
        return Ok(Vec::new());
    }
    let mut errors = Vec::new();
    for update in parsed_updates(proposal) {
        let Some(record) = &update.record else {
            continue;
        };
        let label = format!("{}/{}", update.asset, update.record_id);
        let existing = records_in(current, &update.asset)
            .iter()
            .find(|entry| get(entry, "id").as_str() == Some(update.record_id.as_str()));
        let Some(existing) = existing.filter(|entry| truthy(get(entry, "verified"))) else {
            errors.push(format!(
                "once applied: {label} would be reviewed by nobody; the proposal records no approval and the \
                 Registry holds no verified {label}, so record-approval first"
            ));
            continue;
        };
        let fields = fields_beyond_evidence(&update.asset, existing, record)?;
        if !fields.is_empty() {
            errors.push(format!(
                "once applied: {label} would be reviewed by nobody; the proposal records no approval and \
                 changes more than its citations ({}), so record-approval first",
                fields.join(", ")
            ));
        }
    }
    Ok(errors)
}

fn staged_registry_errors(
    current: &Staged,
    staged: &Staged,
    proposal: &Object,
    repo_root: Option<&Path>,
) -> Result<Vec<String>, String> {
    let before: BTreeSet<String> = cross_reference_errors(current).into_iter().collect();
    let mut errors: Vec<String> = cross_reference_errors(staged)
        .into_iter()
        .filter(|error| !before.contains(error))
        .map(|error| format!("once applied: {error}"))
        .collect();
    if let Some(repo_root) = repo_root {
        errors.extend(updated_evidence_errors(proposal, repo_root)?);
        errors.extend(decision_evidence_errors(proposal, repo_root));
    }
    if status_in(get(proposal, "status"), &VERIFIED_OR_LATER) {
        errors.extend(unverified_update_errors(current, proposal)?);
    }
    Ok(errors)
}

pub fn unconfirmed_evidence_errors(
    proposal: &Object,
    registry_root: &Path,
) -> Result<Vec<String>, String> {
    let kinds = SourceKinds::of(&source_map_for(registry_root)?);
    let mut errors = Vec::new();
    for update in parsed_updates(proposal) {
        let record = Value::Object(update.record.clone().unwrap_or_default());
        let outside = unclassified_paths(&classify_all(&record, &kinds)?)?;
        if !outside.is_empty() {
            errors.push(format!(
                "{}/{} cites {} outside the confirmed sources, so \
                 ship-package would refuse to review it; cite a confirmed source, or have the developer \
                 select that path",
                update.asset,
                update.record_id,
                outside.join(", ")
            ));
        }
    }
    Ok(errors)
}

pub fn validate_change_package(
    root: &Path,
    registry_root: Option<&Path>,
    as_status: Option<&str>,
    repo_root: Option<&Path>,
) -> Result<Vec<String>, String> {
    let missing: Vec<String> = CHANGE_PACKAGE_FILES
        .iter()
        .filter(|name| !root.join(name).is_file())
        .map(|name| {
            format!(
                "missing change package file: {}",
                py::shown(&root.join(name))
            )
        })
        .collect();
    if !missing.is_empty() {
        return Ok(missing);
    }
    let mut documents = Documents::load(root)?;
    if let Some(status) = as_status {
        documents = as_if_moved_to(status, documents);
    }
    let (requirement, proposal, obligations, evidence) = (
        &documents.requirement,
        &documents.proposal,
        &documents.obligations,
        &documents.evidence,
    );
    let mut errors = identity_errors(&documents);
    let (criterion_errors, criterion_ids) = acceptance_criteria(requirement);
    errors.extend(criterion_errors);
    errors.extend(risk_flag_errors(requirement, proposal));
    errors.extend(proposal_field_errors(proposal));
    errors.extend(registry_update_errors(proposal));
    let status = get(proposal, "status");
    let revision = get(proposal, "proposal_revision");
    let list_of = |key: &str| -> Vec<Value> {
        match get(proposal, key) {
            Value::Array(items) => items.clone(),
            _ => Vec::new(),
        }
    };
    let (found, obligation_ids) = obligation_errors(
        obligations,
        &criterion_ids,
        &list_of("rule_ids"),
        &list_of("contract_ids"),
    );
    errors.extend(found);
    errors.extend(implementation_design_errors(proposal, &obligation_ids));
    errors.extend(approval_and_revision_errors(&documents, status, revision));
    errors.extend(evidence_result_errors(evidence, &obligation_ids));
    if status_in(status, &VERIFIED_OR_LATER) {
        errors.extend(test_attestation_errors(
            evidence,
            &obligation_ids,
            registry_root,
        )?);
        if get(proposal, "change_classification").as_str() == Some("material") {
            errors.extend(counterfactual_errors(evidence, &obligation_ids, proposal));
        }
    }
    if status_in(status, &APPROVED_OR_LATER) {
        match get(evidence, "scm_attestation") {
            Value::Object(attestation) => errors.extend(verify_scm_value(attestation, proposal)),
            _ => {
                errors.push("an approved or applied proposal requires SCM attestation".to_string())
            }
        }
    }
    if let Some(registry_root) = registry_root
        && !status_in(status, &SETTLED_STATUSES)
    {
        let current = current_records(registry_root)?;
        let staged = staged_with(&current, proposal);
        errors.extend(registry_reference_errors(&staged, proposal));
        errors.extend(staged_registry_errors(
            &current, &staged, proposal, repo_root,
        )?);
        errors.extend(unconfirmed_evidence_errors(proposal, registry_root)?);
    }
    Ok(errors)
}

pub fn shifted_citation_notes(proposal: &Object, repo_root: &Path) -> Result<Vec<String>, String> {
    let mut notes = Vec::new();
    let mut check = Verifier::new(repo_root, false);
    let Some(Value::Array(updates)) = proposal.get("registry_updates") else {
        return Ok(notes);
    };
    for raw in updates {
        let Some((raw, record)) = raw
            .as_object()
            .and_then(|raw| Some((raw, raw.get("record")?.as_object()?)))
        else {
            continue;
        };
        let mut paths: BTreeSet<String> = BTreeSet::new();
        let mut count = 0;
        for reference in all_references(&Value::Object(record.clone()), 0)? {
            let result = check.check(&reference)?;
            if get(&result, "status").as_str() == Some("shifted") {
                count += 1;
                paths.insert(str_of(get(&result, "path")));
            }
        }
        if count > 0 {
            let many = count > 1;
            notes.push(format!(
                "NOTE: registry update {}/{} cites {count} citation{} \
                 whose lines are unchanged but now sit elsewhere in {}; this passes every check and \
                 needs no Change Package of its own; when the record is next touched, relocate-evidence re-cites {}",
                str_of(get(raw, "asset")),
                str_of(get(record, "id")),
                if many { "s" } else { "" },
                paths.into_iter().collect::<Vec<_>>().join(", "),
                if many { "them" } else { "it" }
            ));
        }
    }
    Ok(notes)
}
