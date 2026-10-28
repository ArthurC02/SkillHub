use std::path::Path;

use crate::audit_log::{read_events, type_name};
use crate::compare::contexts_named_by;
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::json::{Object, Value};
use crate::okf::{review_field, truthy};
use crate::pyvalue::{equal, str_of};
use crate::records::reviewed_by;
use crate::registry_write::{is_reviewed, position_of, read_records, without};

pub type Span = Vec<Value>;

fn no_attribute_get(value: &Value) -> String {
    format!("'{}' object has no attribute 'get'", type_name(value))
}

fn unhashable(value: &Value) -> Result<(), String> {
    match value {
        Value::Array(_) | Value::Object(_) => {
            Err(format!("unhashable type: '{}'", type_name(value)))
        }
        _ => Ok(()),
    }
}

fn registry_updates(proposal: &Object) -> Result<Vec<Value>, String> {
    let blank_text = || no_attribute_get(&Value::Str(String::new()));
    match proposal.get("registry_updates") {
        None => Ok(Vec::new()),
        Some(Value::Array(items)) => Ok(items.clone()),
        Some(Value::Object(keys)) if keys.iter().next().is_none() => Ok(Vec::new()),
        Some(Value::Str(text)) if text.is_empty() => Ok(Vec::new()),
        Some(Value::Object(_) | Value::Str(_)) => Err(blank_text()),
        Some(other) => Err(format!("'{}' object is not iterable", type_name(other))),
    }
}

fn updates_record(update: &Value, asset: &str, record_id: &str) -> Result<bool, String> {
    let Value::Object(update) = update else {
        return Err(no_attribute_get(update));
    };
    if update.get("asset").and_then(Value::as_str) != Some(asset) {
        return Ok(false);
    }
    match update.get("record").filter(|record| truthy(record)) {
        None => Ok(false),
        Some(Value::Object(record)) => {
            Ok(record.get("id").and_then(Value::as_str) == Some(record_id))
        }
        Some(other) => Err(no_attribute_get(other)),
    }
}

fn cited_span(reference: &Value) -> Result<Span, String> {
    let span = match reference {
        Value::Object(object) => {
            let lines = object.get("lines").and_then(Value::as_object);
            let line = |key: &str| {
                lines
                    .and_then(|lines| lines.get(key))
                    .cloned()
                    .unwrap_or(Value::Null)
            };
            vec![
                object.get("path").cloned().unwrap_or(Value::Null),
                line("start"),
                line("end"),
            ]
        }
        other => vec![other.clone()],
    };
    span.iter().try_for_each(unhashable)?;
    Ok(span)
}

fn same_span(left: &[Value], right: &[Value]) -> bool {
    left.len() == right.len() && left.iter().zip(right).all(|(a, b)| equal(a, b))
}

fn with_citations(evidence: Option<&Value>, citations: &[Value]) -> Result<Vec<Value>, String> {
    let mut merged: Vec<Value> = match evidence {
        Some(Value::Array(items)) => items.clone(),
        _ => Vec::new(),
    };
    let mut at: Vec<(Span, usize)> = Vec::new();
    for (index, reference) in merged.iter().enumerate() {
        let span = cited_span(reference)?;
        match at.iter_mut().find(|(known, _)| same_span(known, &span)) {
            Some(slot) => slot.1 = index,
            None => at.push((span, index)),
        }
    }
    for citation in citations {
        let span = cited_span(citation)?;
        match at.iter().find(|(known, _)| same_span(known, &span)) {
            Some((_, index)) => merged[*index] = citation.clone(),
            None => {
                at.push((span, merged.len()));
                merged.push(citation.clone());
            }
        }
    }
    Ok(merged)
}

fn with_superseded(supersedes: Option<&Value>, proposal_id: &Value) -> Value {
    let mut named: Vec<Value> = match supersedes {
        Some(Value::Str(only)) => vec![Value::Str(only.clone())],
        Some(Value::Array(items)) => items.clone(),
        _ => Vec::new(),
    };
    if !named.iter().any(|known| equal(known, proposal_id)) {
        named.push(proposal_id.clone());
    }
    Value::Array(named)
}

fn reviewing_proposal(registry_root: &Path, key: &str) -> Result<Option<Value>, String> {
    let events = read_events(registry_root)?;
    Ok(events
        .iter()
        .flat_map(|event| reviewed_by(event, key))
        .last()
        .filter(|proposal_id| *proposal_id != Value::Null))
}

fn earlier_update(
    proposal: &Object,
    asset: &str,
    record_id: &str,
) -> Result<Option<Value>, String> {
    for entry in registry_updates(proposal)? {
        if updates_record(&entry, asset, record_id)? {
            return Ok(Some(entry));
        }
    }
    Ok(None)
}

fn earlier_evidence(earlier: &Value) -> Option<&Vec<Value>> {
    let Value::Object(update) = earlier else {
        return None;
    };
    match update.get("record")?.as_object()?.get("evidence")? {
        Value::Array(items) => Some(items),
        _ => None,
    }
}

pub fn promote_candidate(
    package_root: &Path,
    registry_root: &Path,
    asset: &str,
    record_id: &str,
    amend: bool,
) -> Result<Option<Value>, String> {
    let path = package_root.join(PROPOSAL_FILE);
    let mut proposal = load_json(&path)?;
    if proposal.get("status").and_then(Value::as_str) != Some("draft") {
        return Err(
            "registry updates are added while the proposal is a draft; a submitted proposal's \
             content is what its approvals cover. To change it, run supersede-proposal, then \
             redraft-proposal into a new package, and promote into that draft"
                .to_string(),
        );
    }
    let records = read_records(registry_root, asset)?;
    let Some(at) = position_of(&records, record_id) else {
        return Err(format!(
            "no {asset}/{record_id} in the Registry; record it with upsert-candidate first"
        ));
    };
    let record = &records[at];
    let mut superseded = None;
    if is_reviewed(asset, record) {
        if !amend {
            return Err(format!(
                "{asset}/{record_id} is already reviewed; record the change as a candidate with \
                 upsert-candidate, then promote that, or pass --amend to change the reviewed record as it stands"
            ));
        }
        let reviewer = reviewing_proposal(registry_root, &format!("{asset}/{record_id}"))?;
        let Some(reviewer) = reviewer else {
            return Err(format!(
                "no applied Change Package reviewed {asset}/{record_id} in the audit log, so the package \
                 cannot name the proposal it supersedes"
            ));
        };
        proposal.insert(
            "supersedes",
            with_superseded(proposal.get("supersedes"), &reviewer),
        );
        superseded = Some(reviewer);
    }
    let mut promoted = without(record, &[review_field(asset), "verified"]);
    let earlier = earlier_update(&proposal, asset, record_id)?;
    if let Some(carried) = earlier.as_ref().and_then(earlier_evidence) {
        let merged = with_citations(promoted.get("evidence"), carried)?;
        promoted.insert("evidence", Value::Array(merged));
    }
    let named = contexts_named_by(asset, &promoted);
    let mut update = Object::new();
    update.insert("operation", Value::Str("upsert".to_string()));
    update.insert("asset", Value::Str(asset.to_string()));
    update.insert("record", Value::Object(promoted));
    let mut updates = Vec::new();
    for entry in registry_updates(&proposal)? {
        if !updates_record(&entry, asset, record_id)? {
            updates.push(entry);
        }
    }
    updates.push(Value::Object(update));
    proposal.insert("registry_updates", Value::Array(updates));
    let mut contexts: Vec<String> = match proposal.get("affected_contexts") {
        Some(Value::Array(listed)) => listed
            .iter()
            .filter_map(Value::as_str)
            .map(str::to_string)
            .collect(),
        _ => Vec::new(),
    };
    contexts.extend(named);
    contexts.sort();
    contexts.dedup();
    proposal.insert(
        "affected_contexts",
        Value::Array(contexts.into_iter().map(Value::Str).collect()),
    );
    write_json(&path, &Value::Object(proposal))?;
    Ok(superseded)
}

fn rewrite_promoted_records(
    package_root: &Path,
    asset: &str,
    record_id: &str,
    mut rewrite: impl FnMut(&mut Object) -> Result<(), String>,
) -> Result<(), String> {
    let path = package_root.join(PROPOSAL_FILE);
    let mut proposal = load_json(&path)?;
    registry_updates(&proposal)?;
    if let Some(Value::Array(updates)) = proposal.get_mut("registry_updates") {
        for entry in updates.iter_mut() {
            if !updates_record(entry, asset, record_id)? {
                continue;
            }
            if let Value::Object(update) = entry
                && let Some(Value::Object(record)) = update.get_mut("record")
            {
                rewrite(record)?;
            }
        }
    }
    write_json(&path, &Value::Object(proposal))
}

pub fn add_promoted_evidence(
    package_root: &Path,
    asset: &str,
    record_id: &str,
    citations: &[Value],
) -> Result<(), String> {
    rewrite_promoted_records(package_root, asset, record_id, |record| {
        let merged = with_citations(record.get("evidence"), citations)?;
        record.insert("evidence", Value::Array(merged));
        Ok(())
    })
}

pub fn drop_promoted_evidence(
    package_root: &Path,
    asset: &str,
    record_id: &str,
    spans: &[Span],
) -> Result<(), String> {
    rewrite_promoted_records(package_root, asset, record_id, |record| {
        let cited: Vec<Value> = match record.get("evidence") {
            Some(Value::Array(items)) => items.clone(),
            _ => Vec::new(),
        };
        let held = cited
            .iter()
            .map(cited_span)
            .collect::<Result<Vec<_>, _>>()?;
        for span in spans {
            if !held.iter().any(|known| same_span(known, span)) {
                return Err(format!(
                    "{asset}/{record_id} does not cite {}:{}-{}, so there is nothing to drop",
                    str_of(&span[0]),
                    str_of(&span[1]),
                    str_of(&span[2])
                ));
            }
        }
        let kept = cited
            .iter()
            .zip(&held)
            .filter(|(_, held)| !spans.iter().any(|span| same_span(held, span)))
            .map(|(reference, _)| reference.clone())
            .collect();
        record.insert("evidence", Value::Array(kept));
        Ok(())
    })
}
