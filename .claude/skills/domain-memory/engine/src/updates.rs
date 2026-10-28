use std::path::{Path, PathBuf};

use crate::candidate_evidence::{
    classify_all, hash_all, one_cites_within_the_other, source_map_for, unclassified_paths,
};
use crate::coverage::SourceKinds;
use crate::json::{self, Object, Value};
use crate::okf::{review_field, truthy};
use crate::policy::review_mode;
use crate::proposals::completed_identifier;
use crate::py;
use crate::registry_write::{
    asset_names, is_reviewed, position_of, read_records, without, write_records,
};
use crate::transaction::mutate_registry;

#[derive(Default)]
pub struct Upserted {
    pub outside: Vec<String>,
    pub created: usize,
    pub same_evidence: Vec<String>,
    pub status_dropped: Vec<String>,
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

fn stage_candidates(
    asset: &str,
    records: &mut Vec<Object>,
    batch: &[Object],
    kinds: &SourceKinds,
    result: &mut Upserted,
) -> Result<(), String> {
    for record in batch {
        let id = id_of(record);
        let existing = position_of(records, &id);
        if existing.is_some_and(|at| is_reviewed(asset, &records[at])) {
            return Err(format!("cannot overwrite reviewed record: {id}"));
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
            stage_candidates(asset, &mut records, &batch, &kinds, &mut result)?;
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
