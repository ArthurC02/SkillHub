use std::path::{Path, PathBuf};

use crate::audit_log::{APPLY_OPERATION, read_events};
use crate::candidate_evidence::{
    cited_excerpts, classify_all, hash_all, one_cites_within_the_other, source_map_for,
    unclassified_paths,
};
use crate::changes::{ParsedUpdate, parsed_update, validate_change_package};
use crate::common::iso_timestamp;
use crate::coverage::SourceKinds;
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::json::{self, Object, Value};
use crate::okf::{review_field, truthy};
use crate::policy::review_mode;
use crate::proposals::{completed_identifier, superseded_ids};
use crate::py;
use crate::pyvalue::str_of;
use crate::reconciliation::{
    mark_applied, reconcile_pending_update, reconciliation_path, was_applied,
};
use crate::registry_write::{
    asset_names, is_reviewed, position_of, read_records, without, write_records,
};
use crate::revision::{Refusal, current_registry_revision, require_current_registry_revision};
use crate::transaction::{mutate_registry, mutate_registry_expecting};

#[derive(Default)]
pub struct Upserted {
    pub outside: Vec<String>,
    pub created: usize,
    pub same_evidence: Vec<String>,
    pub skipped_reviewed: Vec<String>,
    pub skipped_duplicate: Vec<(String, String)>,
    pub status_dropped: Vec<String>,
}

pub struct Staging<'a> {
    pub kinds: &'a SourceKinds,
    pub skip_reviewed: bool,
    pub skip_duplicates: bool,
}

fn id_of(record: &Object) -> String {
    record
        .get("id")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

fn read_update(path: &Path) -> Result<Object, String> {
    let invalid = || format!("invalid JSON: {}", py::shown(path));
    let bytes = match std::fs::read(path) {
        Ok(bytes) => bytes,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Err(format!("no such record file: {}", py::shown(path)));
        }
        Err(_) => return Err(invalid()),
    };
    let text = py::utf8_text(&bytes).map_err(|_| invalid())?;
    let value = json::parse(&text).map_err(|()| invalid())?;
    match value {
        Value::Object(record) if record.get("id").is_some_and(completed_identifier) => Ok(record),
        _ => Err(format!(
            "{} must hold one record object with a completed id, such as \
             {{\"id\": \"orders\", \"name\": \"Orders\", \"responsibility\": \"...\"}} \u{2014} not a list of records, \
             not a Change Package, and not the whole asset file. One record per file; \
             give --record-file once for each record that belongs to the same change.",
            py::shown(path)
        )),
    }
}

fn batch_records(record_paths: &[PathBuf]) -> Result<Vec<Object>, String> {
    if record_paths.is_empty() {
        return Err("upserting a candidate requires at least one record file".to_string());
    }
    let records = record_paths
        .iter()
        .map(|path| read_update(path))
        .collect::<Result<Vec<_>, _>>()?;
    let ids: Vec<String> = records.iter().map(id_of).collect();
    let mut repeated: Vec<&String> = ids
        .iter()
        .filter(|id| ids.iter().filter(|other| other == id).count() > 1)
        .collect();
    repeated.sort();
    repeated.dedup();
    if !repeated.is_empty() {
        let names: Vec<&str> = repeated.iter().map(|id| id.as_str()).collect();
        return Err(format!(
            "one batch names a record more than once: {}",
            names.join(", ")
        ));
    }
    Ok(records)
}

fn as_object(value: Value) -> Object {
    match value {
        Value::Object(object) => object,
        _ => Object::new(),
    }
}

fn reviewed_copy(
    asset: &str,
    records: &[Object],
    record: &Object,
) -> Result<Option<String>, String> {
    let excerpts = cited_excerpts(record)?;
    if excerpts.is_empty() {
        return Ok(None);
    }
    for other in records {
        if !is_reviewed(asset, other) {
            continue;
        }
        let held = cited_excerpts(other)?;
        let same = held.len() == excerpts.len() && held.iter().all(|pair| excerpts.contains(pair));
        if same {
            let named = other.get("id").map_or_else(|| "None".to_string(), str_of);
            return Ok(Some(format!("{asset}/{named}")));
        }
    }
    Ok(None)
}

pub fn stage_candidates(
    asset: &str,
    records: &mut Vec<Object>,
    batch: &[Object],
    staging: &Staging,
    result: &mut Upserted,
) -> Result<(), String> {
    let kinds = staging.kinds;
    for record in batch {
        let id = id_of(record);
        let existing = position_of(records, &id);
        if existing.is_some_and(|at| is_reviewed(asset, &records[at])) {
            if !staging.skip_reviewed {
                return Err(format!("cannot overwrite reviewed record: {id}"));
            }
            result.skipped_reviewed.push(format!("{asset}/{id}"));
            continue;
        }
        if staging.skip_duplicates
            && let Some(duplicates) = reviewed_copy(asset, records, record)?
        {
            result
                .skipped_duplicate
                .push((format!("{asset}/{id}"), duplicates));
            continue;
        }
        result.created += usize::from(existing.is_none());
        for other in records.iter() {
            if id_of(other) != id && one_cites_within_the_other(record, other)? {
                result
                    .same_evidence
                    .push(format!("{asset}/{}", id_of(other)));
            }
        }
        let candidate = classify_all(&Value::Object(record.clone()), kinds)?;
        result.outside.extend(unclassified_paths(&candidate)?);
        let candidate = as_object(candidate);
        let status = candidate.get(review_field(asset));
        let carried_status = status.is_some_and(|status| status.as_str() != Some("candidate"));
        if carried_status || candidate.get("verified").is_some_and(truthy) {
            result.status_dropped.push(format!("{asset}/{id}"));
        }
        let mut candidate = without(&candidate, &["verified"]);
        candidate.insert(review_field(asset), Value::Str("candidate".to_string()));
        match existing {
            Some(at) => records[at] = candidate,
            None => records.push(candidate),
        }
    }
    Ok(())
}

fn operation_event(operation: &str, asset: Option<&str>) -> Object {
    let mut event = Object::new();
    event.insert("operation", Value::Str(operation.to_string()));
    if let Some(asset) = asset {
        event.insert("asset", Value::Str(asset.to_string()));
    }
    event
}

pub fn upsert_candidate(
    root: &Path,
    repo_root: &Path,
    asset: &str,
    record_paths: &[PathBuf],
) -> Result<Upserted, String> {
    let mut batch = Vec::new();
    for record in batch_records(record_paths)? {
        batch.push(as_object(hash_all(&Value::Object(record), repo_root)?));
    }
    let source_map = source_map_for(root)?;
    let kinds = SourceKinds::of(&source_map);
    let mut result = Upserted::default();
    let mut event = operation_event("upsert-candidate", Some(asset));
    match batch.as_slice() {
        [only] => event.insert("record_id", Value::Str(id_of(only))),
        many => event.insert(
            "record_ids",
            Value::Array(
                many.iter()
                    .map(|record| Value::Str(id_of(record)))
                    .collect(),
            ),
        ),
    }
    mutate_registry(
        root,
        repo_root,
        |staging| {
            let mut records = read_records(staging, asset)?;
            let policy = Staging {
                kinds: &kinds,
                skip_reviewed: false,
                skip_duplicates: false,
            };
            stage_candidates(asset, &mut records, &batch, &policy, &mut result)?;
            write_records(staging, asset, &records)
        },
        || event,
    )?;
    result.outside.sort();
    result.outside.dedup();
    Ok(result)
}

pub fn retract_candidate(
    root: &Path,
    repo_root: &Path,
    asset: &str,
    record_ids: &[String],
    reason: &str,
) -> Result<(), String> {
    if py::strip(reason).is_empty() {
        return Err("retracting a candidate requires the reason it does not belong".to_string());
    }
    if record_ids.is_empty() {
        return Err("retracting a candidate requires at least one --id".to_string());
    }
    let mut event = operation_event("retract-candidate", Some(asset));
    event.insert(
        "record_ids",
        Value::Array(record_ids.iter().cloned().map(Value::Str).collect()),
    );
    event.insert("reason", Value::Str(py::strip(reason).to_string()));
    mutate_registry(
        root,
        repo_root,
        |staging| {
            let mut records = read_records(staging, asset)?;
            for record_id in record_ids {
                let Some(at) = position_of(&records, record_id) else {
                    return Err(format!("no such record: {asset}/{record_id}"));
                };
                if is_reviewed(asset, &records[at]) {
                    return Err(format!(
                        "cannot retract reviewed record: {record_id}; a reviewed record \
                         is withdrawn through a superseding proposal"
                    ));
                }
                records.remove(at);
            }
            write_records(staging, asset, &records)
        },
        || event,
    )
}

pub fn demote_local_reviews(root: &Path, repo_root: &Path) -> Result<(), String> {
    if review_mode(root)? != "local-draft-only" {
        return Err("review demotion is only for local-draft-only Domain Memory".to_string());
    }
    mutate_registry(
        root,
        repo_root,
        |staging| {
            for asset in asset_names() {
                let records: Vec<Object> = read_records(staging, asset)?
                    .iter()
                    .map(|record| {
                        let mut demoted = without(record, &["verified"]);
                        demoted.insert(review_field(asset), Value::Str("candidate".to_string()));
                        demoted
                    })
                    .collect();
                write_records(staging, asset, &records)?;
            }
            Ok(())
        },
        || operation_event("demote-local-reviews", None),
    )
}

fn reviewing_proposals(registry_root: &Path) -> Result<Vec<(String, Value)>, String> {
    let mut latest: Vec<(String, Value)> = Vec::new();
    let mut note = |record: &str, proposal_id: &Value| match latest
        .iter_mut()
        .find(|(known, _)| known == record)
    {
        Some(slot) => slot.1 = proposal_id.clone(),
        None => latest.push((record.to_string(), proposal_id.clone())),
    };
    for event in read_events(registry_root)? {
        match event.get("operation").and_then(Value::as_str) {
            Some(APPLY_OPERATION) => {
                let proposal_id = event.get("proposal_id").unwrap_or(&Value::Null);
                match event.get("records") {
                    Some(Value::Array(records)) => records
                        .iter()
                        .filter_map(Value::as_str)
                        .for_each(|record| note(record, proposal_id)),
                    Some(Value::Object(records)) => records
                        .iter()
                        .for_each(|(record, _)| note(record, proposal_id)),
                    _ => {}
                }
            }
            Some("migrate-registry") => {
                if let Some(Value::Object(reviewed)) = event.get("reviewed_by") {
                    reviewed.iter().for_each(|(record, id)| note(record, id));
                }
            }
            _ => {}
        }
    }
    Ok(latest)
}

fn verified_by(approvals: &Value) -> Vec<Value> {
    approvals
        .as_array()
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .filter(|approval| approval.get("decision").and_then(Value::as_str) == Some("approved"))
        .map(|approval| {
            let held = |key: &str| approval.get(key).cloned().unwrap_or(Value::Null);
            let mut entry = Object::new();
            entry.insert(
                "by",
                Value::Str(format!("human:{}", str_of(&held("reviewer")))),
            );
            entry.insert("at", held("approved_at"));
            Value::Object(entry)
        })
        .collect()
}

fn refuse_unsuperseded_overwrite(
    update: &ParsedUpdate,
    existing: Option<&Object>,
    proposal: &Object,
    reviewing: &[(String, Value)],
) -> Result<(), String> {
    if !existing.is_some_and(|record| is_reviewed(&update.asset, record)) {
        return Ok(());
    }
    let key = format!("{}/{}", update.asset, update.record_id);
    let reviewed_by = reviewing
        .iter()
        .find(|(known, _)| *known == key)
        .map(|(_, proposal_id)| proposal_id)
        .filter(|proposal_id| truthy(proposal_id));
    let listed = reviewed_by.is_some_and(|proposal_id| {
        proposal_id
            .as_str()
            .is_some_and(|id| superseded_ids(proposal).unwrap_or_default().contains(id))
    });
    if listed {
        return Ok(());
    }
    Err(format!(
        "cannot overwrite reviewed record: {}; a proposal that \
         replaces it must list {} in supersedes",
        update.record_id,
        reviewed_by.map_or_else(|| "the proposal that reviewed it".to_string(), str_of)
    ))
}

fn reviewed_record(
    update: &ParsedUpdate,
    record: &Object,
    proposal: &Object,
    kinds: &SourceKinds,
    existing: Option<&Object>,
) -> Result<Object, String> {
    let promoted = classify_all(&Value::Object(record.clone()), kinds)?;
    let mut promoted = as_object(promoted);
    let approvals = proposal.get("approvals").unwrap_or(&Value::Null);
    let mut verified = verified_by(approvals);
    if verified.is_empty() {
        verified = match existing.and_then(|held| held.get("verified")) {
            Some(Value::Array(items)) => items.clone(),
            _ => Vec::new(),
        };
    }
    promoted.insert("verified", Value::Array(verified));
    promoted.insert(
        review_field(&update.asset),
        Value::Str("reviewed".to_string()),
    );
    Ok(promoted)
}

fn apply_update(
    update: &ParsedUpdate,
    records: &mut Vec<Object>,
    proposal: &Object,
    kinds: &SourceKinds,
    reviewing: &[(String, Value)],
) -> Result<(), String> {
    let existing = position_of(records, &update.record_id);
    refuse_unsuperseded_overwrite(update, existing.map(|at| &records[at]), proposal, reviewing)?;
    if let Some(record) = &update.record {
        let reviewed = reviewed_record(
            update,
            record,
            proposal,
            kinds,
            existing.map(|at| &records[at]),
        )?;
        match existing {
            Some(at) => records[at] = reviewed,
            None => records.push(reviewed),
        }
        return Ok(());
    }
    let Some(at) = existing else {
        return Err(format!(
            "cannot remove absent record: {}/{}; a removal \
             names a record the Registry holds",
            update.asset, update.record_id
        ));
    };
    records.remove(at);
    Ok(())
}

fn finalized_proposal(package_root: &Path, registry_root: &Path) -> Result<Object, String> {
    if review_mode(registry_root)? != "scm-verified" {
        return Err("reviewed updates require an scm-verified Domain Memory policy".to_string());
    }
    let errors = validate_change_package(package_root, Some(registry_root), None, None)?;
    if !errors.is_empty() {
        return Err(format!("change package is invalid: {}", errors.join("; ")));
    }
    let proposal = load_json(&package_root.join(PROPOSAL_FILE))?;
    let finalized = proposal.get("finalized_at").unwrap_or(&Value::Null);
    if proposal.get("status").and_then(Value::as_str) != Some("approved")
        || !iso_timestamp(finalized)
    {
        return Err("only a finalized approved proposal may apply registry updates".to_string());
    }
    Ok(proposal)
}

fn rebased_revisions(
    base: &Value,
    accepted_digest: &str,
    registry_root: &Path,
    repo_root: &Path,
) -> Result<Vec<(&'static str, Value)>, String> {
    let recorded = base
        .as_object()
        .and_then(|base| base.get("registry_digest"))
        .and_then(Value::as_str);
    if recorded == Some(accepted_digest) {
        return Ok(vec![("from_revision", base.clone())]);
    }
    let mut from = current_registry_revision(registry_root, repo_root)?;
    from.insert("registry_digest", Value::Str(accepted_digest.to_string()));
    Ok(vec![
        ("from_revision", Value::Object(from)),
        ("proposal_base_revision", base.clone()),
    ])
}

fn unlink_marker(registry_root: &Path) -> Result<(), String> {
    let marker = reconciliation_path(registry_root);
    match std::fs::remove_file(&marker) {
        Err(error) if error.kind() != std::io::ErrorKind::NotFound => {
            Err(py::os_error(&error, &marker))
        }
        _ => Ok(()),
    }
}

pub fn apply_approved_updates(
    package_root: &Path,
    registry_root: &Path,
    repo_root: &Path,
) -> Result<(), Refusal> {
    reconcile_pending_update(registry_root, repo_root)?;
    let proposal = finalized_proposal(package_root, registry_root)?;
    let accepted_digest = require_current_registry_revision(&proposal, registry_root, repo_root)?;
    let updates = match proposal.get("registry_updates") {
        Some(Value::Array(items)) if !items.is_empty() => items.clone(),
        _ => {
            return Err(Refusal::Other(
                "approved proposal has no registry_updates".to_string(),
            ));
        }
    };
    let kinds = SourceKinds::of(&source_map_for(registry_root)?);
    let reviewing = reviewing_proposals(registry_root)?;
    let parsed = updates
        .iter()
        .map(parsed_update)
        .collect::<Result<Vec<_>, _>>()?;
    let mut touched: Vec<String> = parsed
        .iter()
        .map(|update| format!("{}/{}", update.asset, update.record_id))
        .collect();
    touched.sort();
    touched.dedup();
    let base_revision = proposal
        .get("base_registry_revision")
        .cloned()
        .unwrap_or(Value::Null);
    let proposal_id = proposal.get("proposal_id").cloned().unwrap_or(Value::Null);
    let relative_package = py::relative_to(
        &py::resolve_path(package_root),
        &py::resolve_path(repo_root),
    )?;
    let mut marker = Object::new();
    marker.insert(
        "format",
        Value::Str("domain-registry-reconciliation/v1".to_string()),
    );
    marker.insert("package", Value::Str(relative_package));
    marker.insert("proposal_id", proposal_id.clone());
    write_json(&reconciliation_path(registry_root), &Value::Object(marker))?;
    let outcome = mutate_registry_expecting(
        registry_root,
        repo_root,
        Some(&accepted_digest),
        |staging| {
            let mut documents: Vec<(String, Vec<Object>)> = Vec::new();
            for update in &parsed {
                let at = match documents
                    .iter()
                    .position(|(asset, _)| *asset == update.asset)
                {
                    Some(at) => at,
                    None => {
                        documents
                            .push((update.asset.clone(), read_records(staging, &update.asset)?));
                        documents.len() - 1
                    }
                };
                apply_update(update, &mut documents[at].1, &proposal, &kinds, &reviewing)?;
            }
            for (asset, records) in &documents {
                write_records(staging, asset, records)?;
            }
            Ok(())
        },
        || {
            let mut event = operation_event(APPLY_OPERATION, None);
            event.insert("proposal_id", proposal_id.clone());
            event.insert(
                "records",
                Value::Array(touched.iter().cloned().map(Value::Str).collect()),
            );
            for (key, value) in
                rebased_revisions(&base_revision, &accepted_digest, registry_root, repo_root)?
            {
                event.insert(key, value);
            }
            event.insert(
                "to_revision",
                Value::Object(current_registry_revision(registry_root, repo_root)?),
            );
            Ok(event)
        },
    );
    if let Err(failure) = outcome {
        if !was_applied(registry_root, &proposal_id)? {
            unlink_marker(registry_root)?;
        }
        return Err(Refusal::Other(failure));
    }
    let mut proposal = proposal;
    mark_applied(
        &package_root.join(PROPOSAL_FILE),
        &mut proposal,
        registry_root,
        repo_root,
    )?;
    unlink_marker(registry_root)?;
    Ok(())
}
