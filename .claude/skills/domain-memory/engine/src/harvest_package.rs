use std::collections::BTreeSet;
use std::path::Path;

use crate::common::{EVIDENCE_FILE, OBLIGATIONS_FILE, REQUIREMENT_FILE};
use crate::counterfactual::OBLIGATION_PLACEHOLDER;
use crate::coverage::all_references;
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::hitl::same_command;
use crate::json::{Object, Value};
use crate::py;
use crate::pyvalue::{repr_of, str_of};

const PACKAGE_DOCUMENTS: [&str; 4] = [
    REQUIREMENT_FILE,
    OBLIGATIONS_FILE,
    PROPOSAL_FILE,
    EVIDENCE_FILE,
];
const REQUIREMENT_FIELDS: [&str; 3] = ["intent", "actor", "observable_outcome"];
const DESIGN_FIELDS: [&str; 4] = [
    "domain_forces",
    "decision",
    "invariants_preserved",
    "rejected_alternatives",
];
const LIST_COUNTERFACTUAL: &str = "Each rule's protection is broken in turn and the rule's own test fails; the evidence bundle's counterfactual_check list records every run.";

pub type Documents = Vec<(&'static str, Object)>;

pub fn criterion_of(rule_id: &str) -> String {
    format!("enforced-{rule_id}")
}

pub fn obligation_of(rule_id: &str) -> String {
    format!("rule-{rule_id}")
}

fn text(value: &Value) -> String {
    str_of(value)
}

fn field<'a>(object: &'a Object, key: &str) -> &'a Value {
    object.get(key).unwrap_or(&Value::Null)
}

pub fn harvest_package(package_root: &Path, spec: &Object) -> Result<Documents, String> {
    let mut documents: Documents = Vec::new();
    for name in PACKAGE_DOCUMENTS {
        documents.push((name, load_json(&package_root.join(name))?));
    }
    let proposal = &documents[2].1;
    if proposal.get("status").and_then(Value::as_str) != Some("draft") {
        return Err(format!(
            "the package is {}; harvest --spec fills a draft",
            text(field(proposal, "status"))
        ));
    }
    let rules = promoted_rules(proposal);
    if rules.is_empty() {
        return Err(
            "the package promotes no rule; run promote-candidate for each harvested rule first"
                .to_string(),
        );
    }
    let empty = Object::new();
    let entries = match spec.get("rules") {
        Some(Value::Object(entries)) => entries,
        _ => &empty,
    };
    let errors = spec_errors(&rules, entries);
    if !errors.is_empty() {
        return Err(format!(
            "the harvest spec cannot fill this package: {}",
            errors.join("; ")
        ));
    }
    fill(&mut documents, &rules, entries, spec);
    Ok(documents)
}

pub fn write_harvest_package(package_root: &Path, documents: &Documents) -> Result<(), String> {
    for (name, document) in documents {
        write_json(&package_root.join(name), &Value::Object(document.clone()))?;
    }
    Ok(())
}

fn promoted_rules(proposal: &Object) -> Vec<Object> {
    let Some(Value::Array(updates)) = proposal.get("registry_updates") else {
        return Vec::new();
    };
    updates
        .iter()
        .filter_map(Value::as_object)
        .filter(|update| {
            update.get("asset").and_then(Value::as_str) == Some("rules")
                && update.get("operation").and_then(Value::as_str) == Some("upsert")
        })
        .filter_map(|update| update.get("record").and_then(Value::as_object))
        .filter(|record| record.get("id").and_then(Value::as_str).is_some())
        .cloned()
        .collect()
}

fn rule_id(rule: &Object) -> &str {
    rule.get("id").and_then(Value::as_str).unwrap_or_default()
}

fn spec_errors(rules: &[Object], entries: &Object) -> Vec<String> {
    let promoted: Vec<&str> = rules.iter().map(rule_id).collect();
    let mut errors: Vec<String> = promoted
        .iter()
        .filter(|id| entries.get(id).is_none())
        .map(|id| format!("rules.{id} is missing"))
        .collect();
    errors.extend(
        entries
            .iter()
            .filter(|(id, _)| !promoted.contains(&id.as_str()))
            .map(|(id, _)| format!("rules.{id} names a rule the package does not promote")),
    );
    for id in &promoted {
        if let Some(entry) = entries.get(id) {
            errors.extend(
                entry_errors(id, entry)
                    .into_iter()
                    .map(|error| format!("rules.{id}: {error}")),
            );
        }
    }
    errors
}

fn entry_errors(rule_id: &str, entry: &Value) -> Vec<String> {
    let Value::Object(entry) = entry else {
        return vec!["give the check that runs the rule's test".to_string()];
    };
    if !entry
        .get("check")
        .and_then(Value::as_str)
        .is_some_and(|check| !py::strip(check).is_empty())
    {
        return vec!["give the check that runs the rule's test".to_string()];
    }
    match entry.get("counterfactual") {
        Some(Value::Object(run)) if run.get("verdict").is_some() => {
            run_errors(rule_id, entry, run)
        }
        _ => vec![
            "counterfactual must be the whole output of counterfactual, so its verdict and test_command can be read"
                .to_string(),
        ],
    }
}

fn run_errors(rule_id: &str, entry: &Object, run: &Object) -> Vec<String> {
    if run.get("verdict").and_then(Value::as_str) != Some("killed") {
        return vec![format!(
            "the counterfactual is {}, not killed; the rule is not shown to fail without its protection",
            text(field(run, "verdict"))
        )];
    }
    let Some(Value::Object(check)) = run.get("counterfactual_check") else {
        return vec![
            "the counterfactual output has no counterfactual_check; give the whole output of counterfactual"
                .to_string(),
        ];
    };
    let given = field(entry, "check");
    if !same_command(&text(field(run, "test_command")), given) {
        return vec![format!(
            "the counterfactual ran {}, not the check {}",
            repr_of(field(run, "test_command")),
            repr_of(given)
        )];
    }
    let named = field(check, "obligation_id");
    let wanted = obligation_of(rule_id);
    if named.as_str() != Some(OBLIGATION_PLACEHOLDER) && named.as_str() != Some(wanted.as_str()) {
        return vec![format!(
            "the counterfactual names obligation {}, not {wanted}",
            text(named)
        )];
    }
    Vec::new()
}

fn chosen(object: &mut Object, value: Option<&Value>, fields: &[&str]) {
    if let Some(Value::Object(given)) = value {
        for name in fields {
            if let Some(found) = given.get(name) {
                object.insert(*name, found.clone());
            }
        }
    }
}

fn strings(items: impl IntoIterator<Item = String>) -> Value {
    Value::Array(items.into_iter().map(Value::Str).collect())
}

fn fill(documents: &mut Documents, rules: &[Object], entries: &Object, spec: &Object) {
    let sources = cited_paths(rules);
    let requirement = &mut documents[0].1;
    chosen(requirement, spec.get("requirement"), &REQUIREMENT_FIELDS);
    requirement.insert(
        "acceptance_criteria",
        Value::Array(
            rules
                .iter()
                .map(|rule| {
                    let mut criterion = Object::new();
                    criterion.insert("id", Value::Str(criterion_of(rule_id(rule))));
                    criterion.insert("statement", field(rule, "statement").clone());
                    Value::Object(criterion)
                })
                .collect(),
        ),
    );
    requirement.insert("sources", strings(sources.clone()));
    documents[1].1.insert(
        "obligations",
        Value::Array(
            rules
                .iter()
                .map(|rule| {
                    let entry = entries
                        .get(rule_id(rule))
                        .and_then(Value::as_object)
                        .cloned()
                        .unwrap_or_default();
                    Value::Object(obligation(rule, &entry))
                })
                .collect(),
        ),
    );
    let proposal = &mut documents[2].1;
    let contexts: BTreeSet<String> = rules
        .iter()
        .filter_map(|rule| rule.get("contexts").and_then(Value::as_array))
        .flatten()
        .filter_map(Value::as_str)
        .map(str::to_string)
        .collect();
    proposal.insert("affected_contexts", strings(contexts));
    proposal.insert(
        "rule_ids",
        strings(rules.iter().map(|r| rule_id(r).to_string())),
    );
    for decision in ["ownership_decision", "boundary_decision"] {
        if let Some(given @ Value::Object(_)) = spec.get(decision) {
            proposal.insert(decision, given.clone());
        }
    }
    let mut design = match proposal.get("implementation_design") {
        Some(Value::Object(design)) => design.clone(),
        _ => Object::new(),
    };
    chosen(
        &mut design,
        spec.get("implementation_design"),
        &DESIGN_FIELDS,
    );
    design.insert(
        "proof_obligations",
        strings(rules.iter().map(|rule| obligation_of(rule_id(rule)))),
    );
    design.insert(
        "counterfactual_check",
        Value::Str(LIST_COUNTERFACTUAL.to_string()),
    );
    proposal.insert("implementation_design", Value::Object(design));
    let evidence = &mut documents[3].1;
    evidence.insert("sources", strings(sources));
    evidence.insert(
        "counterfactual_check",
        Value::Array(
            rules
                .iter()
                .map(|rule| {
                    let mut run = entries
                        .get(rule_id(rule))
                        .and_then(Value::as_object)
                        .and_then(|entry| entry.get("counterfactual"))
                        .and_then(Value::as_object)
                        .and_then(|run| run.get("counterfactual_check"))
                        .and_then(Value::as_object)
                        .cloned()
                        .unwrap_or_default();
                    run.insert("obligation_id", Value::Str(obligation_of(rule_id(rule))));
                    Value::Object(run)
                })
                .collect(),
        ),
    );
}

fn obligation(rule: &Object, entry: &Object) -> Object {
    let id = rule_id(rule);
    let mut covered = Object::new();
    covered.insert("source_type", Value::Str("rule".to_string()));
    covered.insert("source_id", Value::Str(id.to_string()));
    let mut built = Object::new();
    built.insert("id", Value::Str(obligation_of(id)));
    built.insert(
        "source_type",
        Value::Str("acceptance-criterion".to_string()),
    );
    built.insert("source_id", Value::Str(criterion_of(id)));
    built.insert("also_covers", Value::Array(vec![Value::Object(covered)]));
    built.insert("expected_outcome", field(rule, "statement").clone());
    built.insert(
        "level",
        entry
            .get("level")
            .cloned()
            .unwrap_or_else(|| Value::Str("unit".to_string())),
    );
    built.insert("status", Value::Str("planned".to_string()));
    built.insert(
        "check",
        Value::Str(
            py::strip(
                entry
                    .get("check")
                    .and_then(Value::as_str)
                    .unwrap_or_default(),
            )
            .to_string(),
        ),
    );
    built
}

fn cited_paths(rules: &[Object]) -> BTreeSet<String> {
    let all = Value::Array(rules.iter().cloned().map(Value::Object).collect());
    all_references(&all, 0)
        .unwrap_or_default()
        .iter()
        .filter_map(|reference| match reference {
            Value::Object(held) => held.get("path").and_then(Value::as_str).map(str::to_string),
            Value::Str(cited) => Some(cited.split(':').next().unwrap_or_default().to_string()),
            _ => None,
        })
        .collect()
}
