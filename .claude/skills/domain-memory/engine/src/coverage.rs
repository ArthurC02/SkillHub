use std::path::{Path, PathBuf};

use crate::files::load_json;
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::pyvalue::{equal, str_of};
use crate::records::{audit_events, covers, obligations, readable_packages, reviewed_by};
use crate::store::{ASSETS, read_documents};

const ABSENCE_ASSETS: &[(&str, &str)] = &[
    ("vocabulary", "without_vocabulary"),
    ("aggregates", "without_aggregate"),
    ("rules", "without_rule"),
    ("contracts", "without_contract"),
    ("events", "without_event"),
];
const COUNTED: &[(&str, &str)] = &[
    ("vocabulary", "vocabulary_terms"),
    ("aggregates", "aggregates"),
    ("rules", "rules"),
    ("contracts", "contracts"),
    ("events", "events"),
];
const UNCLASSIFIED: &str = "unclassified";
const SELECTED_KIND: &str = "selected";
const MAX_NESTING_DEPTH: usize = 100;
const PROOF_ROWS: [&str; 3] = ["proven", "unproven", "unknown"];

pub struct Report {
    pub coverage: Value,
    pub missing_changes: Option<PathBuf>,
}

struct Held {
    status: Value,
    records: Vec<Object>,
}

pub fn coverage(root: &Path, changes_root: Option<PathBuf>) -> Result<Report, String> {
    let held = held_assets(root)?;
    let mut report = model_report(root, &held)?;
    rule_coverage(
        &mut report,
        root,
        of(&held, "rules"),
        &of(&held, "contexts").records,
        changes_root.as_deref(),
    )?;
    Ok(Report {
        coverage: Value::Object(report),
        missing_changes: changes_root.is_none().then(|| root.join("changes")),
    })
}

pub fn registry_coverage(root: &Path) -> Result<Object, String> {
    model_report(root, &held_assets(root)?)
}

fn held_assets(root: &Path) -> Result<Vec<(&'static str, Held)>, String> {
    let documents = read_documents(root)?;
    Ok(ASSETS
        .iter()
        .map(|(asset, key, _)| {
            let document = documents.get(asset).and_then(Value::as_object);
            let field = |name: &str| document.and_then(|document| document.get(name));
            let records = field(key)
                .and_then(Value::as_array)
                .unwrap_or_default()
                .iter()
                .filter_map(|record| record.as_object().cloned())
                .collect();
            let status = field("status").cloned().unwrap_or(Value::Null);
            (*asset, Held { status, records })
        })
        .collect())
}

fn of<'a>(held: &'a [(&str, Held)], asset: &str) -> &'a Held {
    &held
        .iter()
        .find(|(name, _)| *name == asset)
        .expect("asset")
        .1
}

fn model_report(root: &Path, held: &[(&str, Held)]) -> Result<Object, String> {
    let mut report = Object::new();
    model_coverage(&mut report, &of(held, "contexts").records, &|asset| {
        of(held, asset)
    });
    report.insert("evidence_gaps", evidence_gaps(root, held)?);
    Ok(report)
}

fn model_coverage<'a>(report: &mut Object, contexts: &[Object], of: &dyn Fn(&str) -> &'a Held) {
    let known: Vec<&str> = contexts
        .iter()
        .filter_map(|entry| text(entry, "id"))
        .collect();
    report.insert("contexts", Value::int(known.len()));
    for (asset, label) in COUNTED {
        report.insert(*label, Value::int(of(asset).records.len()));
    }
    let mut gaps = Object::new();
    let mut confirmed = Object::new();
    for (asset, gap) in ABSENCE_ASSETS {
        let holding = holding(asset, &of(asset).records);
        let absent = declared_absent(contexts, asset);
        let missing = known.iter().filter(|context| !holding.contains(context));
        let (declared, open): (Vec<&&str>, Vec<&&str>) =
            missing.partition(|context| absent.contains(context));
        gaps.insert(*gap, names(open));
        confirmed.insert(*gap, names(declared));
    }
    report.insert("gaps", Value::Object(gaps));
    report.insert("confirmed_absent", Value::Object(confirmed));
}

fn names(contexts: Vec<&&str>) -> Value {
    Value::Array(
        contexts
            .into_iter()
            .map(|context| Value::Str((*context).to_string()))
            .collect(),
    )
}

fn text<'a>(entry: &'a Object, key: &str) -> Option<&'a str> {
    entry.get(key).and_then(Value::as_str)
}

fn listed<'a>(entry: &'a Object, key: &str) -> Vec<&'a str> {
    entry
        .get(key)
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_str)
        .collect()
}

pub fn holding<'a>(asset: &str, records: &'a [Object]) -> Vec<&'a str> {
    records
        .iter()
        .flat_map(|entry| match asset {
            "vocabulary" | "rules" => listed(entry, "contexts"),
            "aggregates" => text(entry, "context").into_iter().collect(),
            "contracts" => text(entry, "producer_context")
                .into_iter()
                .chain(listed(entry, "consumer_contexts"))
                .collect(),
            _ => text(entry, "owner_context").into_iter().collect(),
        })
        .collect()
}

fn declared_absent<'a>(contexts: &'a [Object], asset: &str) -> Vec<&'a str> {
    contexts
        .iter()
        .filter(|entry| {
            entry
                .get("confirmed_absences")
                .and_then(Value::as_array)
                .unwrap_or_default()
                .iter()
                .filter_map(Value::as_object)
                .any(|declaration| text(declaration, "asset") == Some(asset))
        })
        .filter_map(|entry| text(entry, "id"))
        .collect()
}

fn is_reviewed(asset: &str, held: &Held, record: &Object) -> bool {
    let field = if asset == "decisions" {
        "review_status"
    } else {
        "status"
    };
    record.get(field).unwrap_or(&held.status).as_str() == Some("reviewed")
}

fn evidence_gaps(root: &Path, held: &[(&str, Held)]) -> Result<Value, String> {
    let path = root.join("source-map.json");
    let source_map = if path.is_file() {
        load_json(&path)?
    } else {
        Object::new()
    };
    let kinds = SourceKinds::of(&source_map);
    let mut by_name: Vec<&(&str, Held)> = held.iter().collect();
    by_name.sort_by_key(|(asset, _)| *asset);
    let mut gaps = Vec::new();
    for (asset, document) in by_name {
        for record in &document.records {
            if !is_reviewed(asset, document, record) {
                continue;
            }
            let mut entry = Object::new();
            entry.insert("asset", Value::Str((*asset).to_string()));
            entry.insert("id", record.get("id").cloned().unwrap_or(Value::Null));
            if !record.get("evidence").is_some_and(truthy) {
                entry.insert("reason", Value::Str("no evidence".to_string()));
                gaps.push(Value::Object(entry));
                continue;
            }
            let classified = with_references(
                &Value::Object(record.clone()),
                &mut |reference| Ok(kinds.classify(reference)),
                0,
            )?;
            let mut outside = Vec::new();
            for reference in all_references(&classified, 0)? {
                let Value::Object(reference) = reference else {
                    continue;
                };
                if reference.get("source_kind").and_then(Value::as_str) == Some(UNCLASSIFIED)
                    && let Some(cited) = text(&reference, "path")
                    && !outside.contains(&cited.to_string())
                {
                    outside.push(cited.to_string());
                }
            }
            if !outside.is_empty() {
                outside.sort();
                entry.insert(
                    "reason",
                    Value::Str("outside the selected sources".to_string()),
                );
                entry.insert(
                    "paths",
                    Value::Array(outside.into_iter().map(Value::Str).collect()),
                );
                gaps.push(Value::Object(entry));
            }
        }
    }
    Ok(Value::Array(gaps))
}

fn too_deep() -> String {
    format!("evidence document exceeds nesting depth {MAX_NESTING_DEPTH}")
}

pub fn with_references(
    value: &Value,
    change: &mut dyn FnMut(&Value) -> Result<Value, String>,
    depth: usize,
) -> Result<Value, String> {
    if depth > MAX_NESTING_DEPTH {
        return Err(too_deep());
    }
    Ok(match value {
        Value::Object(object) => {
            let mut out = Object::new();
            for (key, child) in object.iter() {
                let changed = match child {
                    Value::Array(items) if key == "evidence" => {
                        Value::Array(items.iter().map(&mut *change).collect::<Result<_, _>>()?)
                    }
                    _ => with_references(child, change, depth + 1)?,
                };
                out.insert(key.clone(), changed);
            }
            Value::Object(out)
        }
        Value::Array(items) => Value::Array(
            items
                .iter()
                .map(|child| with_references(child, change, depth + 1))
                .collect::<Result<_, _>>()?,
        ),
        other => other.clone(),
    })
}

pub fn all_references(value: &Value, depth: usize) -> Result<Vec<Value>, String> {
    if depth > MAX_NESTING_DEPTH {
        return Err(too_deep());
    }
    let mut found = Vec::new();
    match value {
        Value::Object(object) => {
            for (key, child) in object.iter() {
                if let (true, Value::Array(items)) = (key == "evidence", child) {
                    found.extend(items.iter().cloned());
                }
                found.extend(all_references(child, depth + 1)?);
            }
        }
        Value::Array(items) => {
            for child in items {
                found.extend(all_references(child, depth + 1)?);
            }
        }
        _ => {}
    }
    Ok(found)
}

pub struct SourceKinds {
    active: bool,
    kinds: Vec<(String, Value)>,
    group_kinds: Vec<(String, Value)>,
}

impl SourceKinds {
    pub fn of(source_map: &Object) -> Self {
        let groups: Vec<&Object> = source_map
            .get("source_groups")
            .and_then(Value::as_array)
            .unwrap_or_default()
            .iter()
            .filter_map(Value::as_object)
            .collect();
        let group_paths = |group: &Object| -> Vec<String> {
            group
                .get("paths")
                .and_then(Value::as_array)
                .unwrap_or_default()
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_string)
                .collect()
        };
        let group_kind = |group: &Object| {
            group
                .get("kind")
                .cloned()
                .unwrap_or_else(|| Value::Str(UNCLASSIFIED.to_string()))
        };
        let mut group_kinds: Vec<(String, Value)> = Vec::new();
        for group in &groups {
            let kind = group_kind(group);
            for path in group_paths(group) {
                if !group_kinds.iter().any(|(seen, _)| *seen == path) {
                    group_kinds.push((path, kind.clone()));
                }
            }
        }
        let kinds = match source_map.get("source_kinds") {
            Some(Value::Object(given)) if given.iter().next().is_some() => {
                given.iter().cloned().collect()
            }
            _ => {
                let mut last_kind: Vec<(String, Value)> = Vec::new();
                for group in &groups {
                    let kind = group_kind(group);
                    for path in group_paths(group) {
                        match last_kind.iter_mut().find(|(seen, _)| *seen == path) {
                            Some(slot) => slot.1 = kind.clone(),
                            None => last_kind.push((path, kind.clone())),
                        }
                    }
                }
                source_map
                    .get("selected_paths")
                    .and_then(Value::as_array)
                    .unwrap_or_default()
                    .iter()
                    .filter_map(Value::as_str)
                    .map(|path| {
                        let kind = last_kind.iter().find(|(seen, _)| seen == path).map_or_else(
                            || Value::Str(UNCLASSIFIED.to_string()),
                            |(_, kind)| kind.clone(),
                        );
                        (path.to_string(), kind)
                    })
                    .collect()
            }
        };
        Self {
            active: source_map.iter().next().is_some(),
            kinds,
            group_kinds,
        }
    }

    pub fn classify(&self, reference: &Value) -> Value {
        let Value::Object(reference) = reference else {
            return reference.clone();
        };
        let (true, Some(cited)) = (self.active, text(reference, "path")) else {
            return Value::Object(reference.clone());
        };
        let mut out = reference.clone();
        out.insert("source_kind", self.kind_of(cited));
        Value::Object(out)
    }

    fn kind_of(&self, cited: &str) -> Value {
        let unclassified = Value::Str(UNCLASSIFIED.to_string());
        let owners = containing_paths(cited);
        let owner = owners
            .iter()
            .find_map(|owner| self.kinds.iter().find(|(path, _)| path == owner));
        if let Some((_, kind)) = owner
            && !equal(kind, &unclassified)
        {
            return kind.clone();
        }
        let kind = owners
            .iter()
            .find_map(|owner| self.group_kinds.iter().find(|(path, _)| path == owner))
            .map_or_else(|| unclassified.clone(), |(_, kind)| kind.clone());
        if equal(&kind, &unclassified) && owner.is_some() {
            return Value::Str(SELECTED_KIND.to_string());
        }
        match kind {
            Value::Str(_) => kind,
            _ => unclassified,
        }
    }
}

fn containing_paths(path: &str) -> Vec<String> {
    let parts: Vec<&str> = path.split('/').collect();
    (1..=parts.len())
        .rev()
        .map(|size| parts[..size].join("/"))
        .filter(|owner| !owner.is_empty())
        .collect()
}

struct Row {
    context: String,
    reviewed: usize,
    candidate: usize,
    proofs: [Vec<Value>; 3],
}

impl Row {
    fn new(context: &str) -> Self {
        Self {
            context: context.to_string(),
            reviewed: 0,
            candidate: 0,
            proofs: Default::default(),
        }
    }

    fn shown(self, with_proofs: bool) -> Value {
        let mut row = Object::new();
        row.insert("context", Value::Str(self.context));
        row.insert("reviewed_rules", Value::int(self.reviewed));
        row.insert("candidate_rules", Value::int(self.candidate));
        if with_proofs {
            for (name, ids) in PROOF_ROWS.into_iter().zip(self.proofs) {
                row.insert(name, Value::Array(ids));
            }
        }
        Value::Object(row)
    }
}

fn rule_coverage(
    report: &mut Object,
    root: &Path,
    rules: &Held,
    contexts: &[Object],
    changes_root: Option<&Path>,
) -> Result<(), String> {
    let events = audit_events(root)?.unwrap_or_default();
    let proven = match changes_root {
        Some(changes_root) => Some(proven_sources(changes_root)?),
        None => None,
    };
    let mut rows: Vec<Row> = contexts
        .iter()
        .filter_map(|context| text(context, "id"))
        .map(Row::new)
        .collect();
    let (mut reviewed_rules, mut candidate_rules) = (0, 0);
    let mut proof_totals = [0; 3];
    for rule in &rules.records {
        let reviewed = is_reviewed("rules", rules, rule);
        if reviewed {
            reviewed_rules += 1;
        } else {
            candidate_rules += 1;
        }
        let id = rule.get("id").cloned().unwrap_or(Value::Null);
        let proof = match (&proven, reviewed) {
            (Some(proven), true) => {
                let key = format!("rules/{}", str_of(&id));
                let history: Vec<Value> = events
                    .iter()
                    .flat_map(|event| reviewed_by(event, &key))
                    .collect();
                let found = proven_in(proven, &history, id.as_str().unwrap_or_default());
                proof_totals[found] += 1;
                Some(found)
            }
            _ => None,
        };
        for context in listed(rule, "contexts") {
            let at = match rows.iter().position(|row| row.context == context) {
                Some(at) => at,
                None => {
                    rows.push(Row::new(context));
                    rows.len() - 1
                }
            };
            let row = &mut rows[at];
            if reviewed {
                row.reviewed += 1;
            } else {
                row.candidate += 1;
            }
            if let Some(found) = proof {
                row.proofs[found].push(id.clone());
            }
        }
    }
    let mut totals = Object::new();
    totals.insert("reviewed_rules", Value::int(reviewed_rules));
    totals.insert("candidate_rules", Value::int(candidate_rules));
    if proven.is_some() {
        for (name, count) in PROOF_ROWS.into_iter().zip(proof_totals) {
            totals.insert(name, Value::int(count));
        }
    }
    let without = rows.iter().filter(|row| row.reviewed == 0).count();
    totals.insert("contexts_without_reviewed_rules", Value::int(without));
    report.insert("rule_totals", Value::Object(totals));
    rows.sort_by(|one, other| (one.reviewed, &one.context).cmp(&(other.reviewed, &other.context)));
    report.insert(
        "rules_by_context",
        Value::Array(
            rows.into_iter()
                .map(|row| row.shown(proven.is_some()))
                .collect(),
        ),
    );
    Ok(())
}

type Proven = Vec<(Value, Vec<Object>)>;

fn proven_sources(changes_root: &Path) -> Result<Proven, String> {
    let mut proven: Proven = Vec::new();
    for (package, proposal) in readable_packages(changes_root)? {
        let proposal_id = proposal.get("proposal_id").cloned().unwrap_or(Value::Null);
        let at = match proven
            .iter()
            .position(|(seen, _)| equal(seen, &proposal_id))
        {
            Some(at) => at,
            None => {
                proven.push((proposal_id, Vec::new()));
                proven.len() - 1
            }
        };
        for entry in obligations(&package)? {
            if let Value::Object(entry) = entry {
                proven[at].1.push(entry);
            }
        }
    }
    Ok(proven)
}

const PROVEN: usize = 0;
const UNPROVEN: usize = 1;
const UNKNOWN: usize = 2;

fn proven_in(proven: &Proven, history: &[Value], rule: &str) -> usize {
    for proposal_id in history.iter().rev() {
        let Some((_, entries)) = proven.iter().find(|(seen, _)| equal(seen, proposal_id)) else {
            return UNKNOWN;
        };
        if entries.iter().any(|entry| covers(entry, ("rule", rule))) {
            return PROVEN;
        }
    }
    UNPROVEN
}
