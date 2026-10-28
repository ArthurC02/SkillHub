use std::cell::RefCell;
use std::path::Path;

use crate::candidate_evidence::{hash_all, source_map_for};
use crate::compare::compare_registries;
use crate::coverage::SourceKinds;
use crate::json::{Object, Value};
use crate::okf::{RELATION_TARGETS, truthy};
use crate::registry_write::{read_records, write_records};
use crate::store::{ASSETS, Registry};
use crate::transaction::mutate_registry;
use crate::updates::{Staging, Upserted, stage_candidates};

type Pairs = Vec<(String, Object)>;

fn registry(root: &Path) -> Result<Object, String> {
    let registry = Registry::open(root)?;
    let mut documents = Object::new();
    for (asset, key, _) in ASSETS {
        let kept: Vec<Value> = registry
            .records(asset)?
            .into_iter()
            .filter(|record| {
                record
                    .as_object()
                    .is_some_and(|held| held.get("id").is_some_and(truthy))
            })
            .collect();
        let mut document = Object::new();
        document.insert(*key, Value::Array(kept));
        documents.insert(*asset, Value::Object(document));
    }
    Ok(documents)
}

fn records_of<'a>(documents: &'a Object, asset: &str, key: &str) -> Vec<&'a Object> {
    documents
        .get(asset)
        .and_then(Value::as_object)
        .and_then(|document| document.get(key))
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .collect()
}

fn text_field(object: &Object, key: &str) -> String {
    object
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

fn pair_of(candidate: &str, gold: &str, by: &str) -> Object {
    let mut pair = Object::new();
    pair.insert("candidate", Value::Str(candidate.to_string()));
    pair.insert("gold", Value::Str(gold.to_string()));
    pair.insert("by", Value::Str(by.to_string()));
    pair
}

fn put(pairs: &mut Pairs, candidate: &str, pair: Object) {
    match pairs.iter_mut().find(|(seen, _)| seen == candidate) {
        Some(slot) => slot.1 = pair,
        None => pairs.push((candidate.to_string(), pair)),
    }
}

fn context_fields() -> impl Iterator<Item = &'static str> {
    RELATION_TARGETS
        .iter()
        .filter(|(_, noun)| *noun == "Context")
        .map(|(field, _)| *field)
}

fn renamed(record: &Object, paired: &[(String, String)]) -> Object {
    let rename = |name: &str| {
        paired
            .iter()
            .find(|(candidate, _)| candidate == name)
            .map_or_else(|| name.to_string(), |(_, gold)| gold.clone())
    };
    let mut out = record.clone();
    for field in context_fields() {
        match record.get(field) {
            Some(Value::Str(name)) => out.insert(field, Value::Str(rename(name))),
            Some(Value::Array(entries)) => out.insert(
                field,
                Value::Array(
                    entries
                        .iter()
                        .map(|entry| match entry {
                            Value::Str(name) => Value::Str(rename(name)),
                            other => other.clone(),
                        })
                        .collect(),
                ),
            ),
            _ => {}
        }
    }
    out
}

fn sorted_keys(pairs: &Pairs) -> Vec<&(String, Object)> {
    let mut ordered: Vec<&(String, Object)> = pairs.iter().collect();
    ordered.sort_by(|left, right| left.0.cmp(&right.0));
    ordered
}

fn names(items: Vec<String>) -> Value {
    Value::Array(items.into_iter().map(Value::Str).collect())
}

fn sorted_set(items: &[String]) -> Vec<String> {
    let mut unique: Vec<String> = items.to_vec();
    unique.sort();
    unique.dedup();
    unique
}

pub fn harvest_candidates(
    root: &Path,
    repo_root: &Path,
    from_root: &Path,
    context_map: &[(String, String)],
    keep_duplicates: bool,
) -> Result<Object, String> {
    let scratch = registry(from_root)?;
    let gold = registry(root)?;
    let comparison = compare_registries(&gold, &scratch, &[])?;
    let mut pairs: Pairs = comparison
        .report
        .get("assets")
        .and_then(Value::as_object)
        .and_then(|assets| assets.get("contexts"))
        .and_then(Value::as_object)
        .and_then(|contexts| contexts.get("matched"))
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .map(|pair| (text_field(pair, "candidate"), pair.clone()))
        .collect();
    for (candidate, given) in context_map {
        put(&mut pairs, candidate, pair_of(candidate, given, "given"));
    }
    let paired: Vec<(String, String)> = pairs
        .iter()
        .map(|(candidate, pair)| (candidate.clone(), text_field(pair, "gold")))
        .filter(|(candidate, gold)| candidate != gold)
        .collect();
    let mut batches: Vec<(&str, Vec<Object>)> = Vec::new();
    for (asset, key, _) in ASSETS {
        let mut batch = Vec::new();
        for record in records_of(&scratch, asset, key) {
            let id = text_field(record, "id");
            if *asset == "contexts" && paired.iter().any(|(candidate, _)| *candidate == id) {
                continue;
            }
            let Value::Object(hashed) =
                hash_all(&Value::Object(renamed(record, &paired)), repo_root)?
            else {
                continue;
            };
            batch.push(hashed);
        }
        batches.push((asset, batch));
    }
    let source_map = source_map_for(root)?;
    let kinds = SourceKinds::of(&source_map);
    let staging = Staging {
        kinds: &kinds,
        skip_reviewed: true,
        skip_duplicates: !keep_duplicates,
    };
    let result = RefCell::new(Upserted::default());
    let harvested = || -> Vec<String> {
        let result = result.borrow();
        let skipped: Vec<&String> = result
            .skipped_reviewed
            .iter()
            .chain(result.skipped_duplicate.iter().map(|(record, _)| record))
            .collect();
        batches
            .iter()
            .flat_map(|(asset, batch)| {
                batch
                    .iter()
                    .map(move |record| format!("{asset}/{}", text_field(record, "id")))
            })
            .filter(|key| !skipped.contains(&key))
            .collect()
    };
    mutate_registry(
        root,
        repo_root,
        |staged| {
            for (asset, batch) in &batches {
                if batch.is_empty() {
                    continue;
                }
                let mut records = read_records(staged, asset)?;
                stage_candidates(
                    asset,
                    &mut records,
                    batch,
                    &staging,
                    &mut result.borrow_mut(),
                )?;
                write_records(staged, asset, &records)?;
            }
            Ok(())
        },
        || {
            let mut event = Object::new();
            event.insert("operation", Value::Str("harvest-candidates".to_string()));
            event.insert("records", names(harvested()));
            event
        },
    )?;
    let harvested = harvested();
    let result = result.into_inner();
    let mut report = Object::new();
    let mut shown_map = Object::new();
    let mut ordered_paired = paired.clone();
    ordered_paired.sort();
    for (candidate, gold) in ordered_paired {
        shown_map.insert(candidate, Value::Str(gold));
    }
    report.insert("context_map", Value::Object(shown_map));
    report.insert(
        "context_pairs",
        Value::Array(
            sorted_keys(&pairs)
                .into_iter()
                .map(|(_, pair)| Value::Object(pair.clone()))
                .collect(),
        ),
    );
    let collisions: Vec<String> = sorted_keys(&pairs)
        .into_iter()
        .filter(|(candidate, pair)| {
            text_field(pair, "by") == "name" && *candidate == text_field(pair, "gold")
        })
        .map(|(candidate, _)| candidate.clone())
        .collect();
    report.insert("context_id_collisions", names(collisions));
    let updated = harvested.len() - result.created;
    report.insert("harvested", names(harvested));
    report.insert("created", Value::int(result.created));
    report.insert("updated", Value::int(updated));
    report.insert("skipped_reviewed", names(result.skipped_reviewed.clone()));
    report.insert(
        "skipped_duplicate",
        Value::Array(
            result
                .skipped_duplicate
                .iter()
                .map(|(record, duplicates)| {
                    let mut entry = Object::new();
                    entry.insert("record", Value::Str(record.clone()));
                    entry.insert("duplicates", Value::Str(duplicates.clone()));
                    Value::Object(entry)
                })
                .collect(),
        ),
    );
    report.insert("outside_sources", names(sorted_set(&result.outside)));
    report.insert(
        "possible_duplicates",
        names(sorted_set(&result.same_evidence)),
    );
    Ok(report)
}
