use std::collections::BTreeSet;
use std::path::Path;

use crate::json::{Object, Value};
use crate::policy::review_mode;
use crate::pyvalue::str_of;
use crate::records::{did_you_mean, read_record};
use crate::store::asset_records;

const SUMMARY_FIELDS: &[&str] = &[
    "statement",
    "definition",
    "responsibility",
    "description",
    "name",
];
const BOUND_RULES: &str = "rules_bound_through_collaborations";
const MAX_NESTING_DEPTH: usize = 100;

pub fn get_context(root: &Path, identifier: &str, summary: bool) -> Result<Value, String> {
    let Some(context) = read_record(root, "contexts", identifier)? else {
        let known = asset_records(root, "contexts")?;
        return Err(format!(
            "context not found: {identifier}{}",
            did_you_mean(identifier, &known)
        ));
    };
    let model = context_model(root, identifier, context)?;
    Ok(Value::Object(if summary {
        context_summary(&model)
    } else {
        model
    }))
}

fn records(root: &Path, asset: &str) -> Result<Vec<Object>, String> {
    Ok(asset_records(root, asset)?
        .into_iter()
        .filter_map(|record| match record {
            Value::Object(record) => Some(record),
            _ => None,
        })
        .collect())
}

fn field<'a>(record: &'a Object, key: &str) -> &'a Value {
    record.get(key).unwrap_or(&Value::Null)
}

fn is(record: &Object, key: &str, identifier: &str) -> bool {
    record.get(key).and_then(Value::as_str) == Some(identifier)
}

fn names(record: &Object, key: &str, identifier: &str) -> bool {
    record
        .get(key)
        .and_then(Value::as_array)
        .is_some_and(|items| items.iter().any(|item| item.as_str() == Some(identifier)))
}

fn keep(entries: Vec<Object>, wanted: impl Fn(&Object) -> bool) -> Vec<Object> {
    entries.into_iter().filter(|entry| wanted(entry)).collect()
}

fn objects(entries: Vec<Object>) -> Value {
    Value::Array(entries.into_iter().map(Value::Object).collect())
}

fn context_model(root: &Path, identifier: &str, context: Object) -> Result<Object, String> {
    let contracts = keep(records(root, "contracts")?, |entry| {
        is(entry, "producer_context", identifier) || names(entry, "consumer_contexts", identifier)
    });
    let interactions = keep(records(root, "interactions")?, |entry| {
        is(entry, "producer_context", identifier) || is(entry, "consumer_context", identifier)
    });
    let rules = records(root, "rules")?;
    let mode = review_mode(root)?;
    let vocabulary = keep(records(root, "vocabulary")?, |entry| {
        names(entry, "contexts", identifier)
    });
    let aggregates = keep(records(root, "aggregates")?, |entry| {
        is(entry, "context", identifier)
    });
    let own_rules = keep(rules.clone(), |entry| names(entry, "contexts", identifier));
    let events = keep(records(root, "events")?, |entry| {
        is(entry, "owner_context", identifier) || names(entry, "consumers", identifier)
    });
    let capabilities = keep(records(root, "capabilities")?, |entry| {
        is(entry, "context", identifier)
    });
    let value_objects = keep(records(root, "value-objects")?, |entry| {
        is(entry, "context", identifier)
    });
    let bound = bound_rules(identifier, &contracts, &interactions, &rules)?;

    let mut model = Object::new();
    let usage = if mode == "scm-verified" {
        "constraint"
    } else {
        "working-memory"
    };
    model.insert("usage", Value::Str(usage.to_string()));
    model.insert("review_mode", Value::Str(mode));
    model.insert("context", Value::Object(context));
    model.insert("vocabulary", objects(vocabulary));
    model.insert("aggregates", objects(aggregates));
    model.insert("rules", objects(own_rules));
    model.insert("contracts", objects(contracts));
    model.insert("interactions", objects(interactions));
    model.insert("events", objects(events));
    model.insert("capabilities", objects(capabilities));
    model.insert("value_objects", objects(value_objects));
    model.insert(BOUND_RULES, Value::Array(bound));
    Ok(model)
}

fn bound_rules(
    identifier: &str,
    contracts: &[Object],
    interactions: &[Object],
    rules: &[Object],
) -> Result<Vec<Value>, String> {
    let mut collaborations = Vec::new();
    for (asset, entries) in [("contracts", contracts), ("interactions", interactions)] {
        for entry in entries {
            collaborations.push((
                format!("{asset}/{}", str_of(field(entry, "id"))),
                cited_paths(entry)?,
            ));
        }
    }
    let mut bound = Vec::new();
    for rule in rules {
        if names(rule, "contexts", identifier) {
            continue;
        }
        let cited = cited_paths(rule)?;
        let via: Vec<Value> = collaborations
            .iter()
            .filter(|(_, paths)| !cited.is_disjoint(paths))
            .map(|(name, _)| Value::Str(name.clone()))
            .collect();
        if via.is_empty() {
            continue;
        }
        let mut entry = Object::new();
        entry.insert("rule", field(rule, "id").clone());
        entry.insert(
            "contexts",
            rule.get("contexts")
                .cloned()
                .unwrap_or(Value::Array(Vec::new())),
        );
        entry.insert("statement", field(rule, "statement").clone());
        entry.insert("via", Value::Array(via));
        bound.push(Value::Object(entry));
    }
    Ok(bound)
}

fn cited_paths(record: &Object) -> Result<BTreeSet<String>, String> {
    let mut references = Vec::new();
    all_references(&Value::Object(record.clone()), 0, &mut references)?;
    Ok(references
        .iter()
        .filter_map(|reference| {
            reference
                .as_object()?
                .get("path")?
                .as_str()
                .map(str::to_string)
        })
        .collect())
}

fn all_references(value: &Value, depth: usize, found: &mut Vec<Value>) -> Result<(), String> {
    if depth > MAX_NESTING_DEPTH {
        return Err(format!(
            "evidence document exceeds nesting depth {MAX_NESTING_DEPTH}"
        ));
    }
    match value {
        Value::Object(object) => {
            for (key, child) in object.iter() {
                if let (true, Value::Array(items)) = (key == "evidence", child) {
                    found.extend(items.iter().cloned());
                }
                all_references(child, depth + 1, found)?;
            }
        }
        Value::Array(items) => {
            for child in items {
                all_references(child, depth + 1, found)?;
            }
        }
        _ => {}
    }
    Ok(())
}

pub fn summarized(entry: &Object) -> Object {
    let summary = SUMMARY_FIELDS
        .iter()
        .find_map(|key| entry.get(key).and_then(Value::as_str))
        .map_or(Value::Null, |text| Value::Str(text.to_string()));
    let status = entry
        .get("status")
        .or_else(|| entry.get("review_status"))
        .cloned()
        .unwrap_or(Value::Null);
    let mut out = Object::new();
    out.insert("id", field(entry, "id").clone());
    out.insert("status", status);
    out.insert("summary", summary);
    out
}

fn context_summary(model: &Object) -> Object {
    let mut out = Object::new();
    for (key, value) in model.iter() {
        let shown = match value {
            Value::Array(items) if key != BOUND_RULES => Value::Array(
                items
                    .iter()
                    .filter_map(Value::as_object)
                    .map(|entry| Value::Object(summarized(entry)))
                    .collect(),
            ),
            Value::Object(context) if key == "context" => {
                let mut entry = summarized(context);
                entry.insert(
                    "implementation_path",
                    field(context, "implementation_path").clone(),
                );
                Value::Object(entry)
            }
            other => other.clone(),
        };
        out.insert(key.clone(), shown);
    }
    out
}

pub fn boundary_analysis(root: &Path, source: &str, target: &str) -> Result<Value, String> {
    let interactions = records(root, "interactions")?;
    let contracts = records(root, "contracts")?;
    let dependencies = records(root, "dependency-policies")?;
    let events = records(root, "events")?;
    let held = [&interactions, &contracts, &dependencies, &events];
    let forward = dependence(held, source, target);
    let backward = dependence(held, target, source);
    let collaborates = |kinds: &[usize]| {
        [&forward, &backward]
            .iter()
            .any(|direction| kinds.iter().any(|kind| !direction[*kind].is_empty()))
    };
    let status = if collaborates(&[0, 1, 3]) {
        "known"
    } else if collaborates(&[2]) {
        "policy_only"
    } else {
        "no_registered_collaboration"
    };
    let mut reply = Object::new();
    reply.insert("source_context", Value::Str(source.to_string()));
    reply.insert("target_context", Value::Str(target.to_string()));
    reply.insert("source_depends_on_target", direction(forward));
    reply.insert("target_depends_on_source", direction(backward));
    reply.insert("status", Value::Str(status.to_string()));
    Ok(Value::Object(reply))
}

fn dependence(held: [&Vec<Object>; 4], dependent: &str, provider: &str) -> [Vec<Object>; 4] {
    let [interactions, contracts, dependencies, events] = held;
    let chosen =
        |entries: &Vec<Object>, wanted: &dyn Fn(&Object) -> bool| keep(entries.clone(), wanted);
    [
        chosen(interactions, &|entry| {
            is(entry, "producer_context", provider) && is(entry, "consumer_context", dependent)
        }),
        chosen(contracts, &|entry| {
            is(entry, "producer_context", provider) && names(entry, "consumer_contexts", dependent)
        }),
        chosen(dependencies, &|entry| {
            is(entry, "from_context", dependent) && is(entry, "to_context", provider)
        }),
        chosen(events, &|entry| {
            is(entry, "owner_context", provider) && names(entry, "consumers", dependent)
        }),
    ]
}

fn direction(lists: [Vec<Object>; 4]) -> Value {
    let mut out = Object::new();
    for (name, entries) in ["interactions", "contracts", "dependencies", "events"]
        .into_iter()
        .zip(lists)
    {
        out.insert(name, objects(entries));
    }
    Value::Object(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_listed_id_names_a_context() {
        let mut record = Object::new();
        record.insert("contexts", Value::Array(vec![Value::Str("catalog".into())]));
        record.insert("consumers", Value::Null);
        assert!(names(&record, "contexts", "catalog"));
        assert!(!names(&record, "contexts", "cat"));
        assert!(!names(&record, "consumers", "catalog"));
    }
}
