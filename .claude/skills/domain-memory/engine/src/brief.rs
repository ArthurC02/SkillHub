use std::collections::{BTreeMap, BTreeSet};
use std::path::{Path, PathBuf};

use crate::context::{cited_paths, context_model, context_summary, summarized};
use crate::dependents::DependencyGraph;
use crate::evidence::{self, FAILING, Verifier};
use crate::files::load_json;
use crate::git;
use crate::impact::{self, Library};
use crate::json::{self, Object, Value};
use crate::okf::truthy;
use crate::py;
use crate::pyvalue::{equal, repr_of, str_of};
use crate::records::{audit_events, obligations_proving, read_record, readable_packages};
use crate::store::{self, ASSETS, DEFAULT_STATUS, Registry};

const FORMAT: &str = "domain-memory-brief/v1";
const PROOF_KIND: &str = "tests";
const CONFIRMED: &str = "developer-confirmed";
const CHANGED_CITATION: [&str; 3] = ["stale", "missing", "invalid"];
const APPLY_OPERATION: &str = "apply-approved-updates";
const MIGRATE_OPERATION: &str = "migrate-registry";
const CONTEXT_FIELDS: [&str; 5] = [
    "owner_context",
    "producer_context",
    "consumer_context",
    "from_context",
    "to_context",
];
const CONTEXT_LISTS: [&str; 3] = ["contexts", "consumer_contexts", "consumers"];
const VERDICTS: [(&str, &str); 5] = [
    (
        "unregistered-co-change",
        "Contexts changed together with no registered collaboration between them; register the interaction, contract or dependency policy, or split the change.",
    ),
    (
        "boundary-evidence-changed",
        "A collaboration's cited lines changed; re-read each record below, and recite only what the code still states.",
    ),
    (
        "registered-co-change",
        "Contexts changed together through registered collaborations; check that each one below still holds.",
    ),
    (
        "no-cross-context-change",
        "The change stays inside one Context.",
    ),
    (
        "unknown",
        "No Context records an implementation_path, so the changed files cannot be mapped to Contexts.",
    ),
];

type History = BTreeMap<String, Vec<Value>>;
type Packages = Option<Vec<(PathBuf, Object)>>;

fn text_of(object: &Object, key: &str) -> String {
    object
        .get(key)
        .map(str_of)
        .unwrap_or_else(|| "None".to_string())
}

fn value_of(object: &Object, key: &str) -> Value {
    object.get(key).cloned().unwrap_or(Value::Null)
}

fn strings<I: IntoIterator<Item = String>>(items: I) -> Value {
    Value::Array(items.into_iter().map(Value::Str).collect())
}

fn objects<I: IntoIterator<Item = Object>>(items: I) -> Value {
    Value::Array(items.into_iter().map(Value::Object).collect())
}

fn list_of<'a>(object: &'a Object, key: &str) -> &'a [Value] {
    object
        .get(key)
        .and_then(Value::as_array)
        .unwrap_or_default()
}

fn object_list<'a>(object: &'a Object, key: &str) -> impl Iterator<Item = &'a Object> {
    list_of(object, key).iter().filter_map(Value::as_object)
}

fn string_list(value: Option<&Value>) -> Vec<String> {
    value
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .map(str_of)
        .collect()
}

fn counted(count: usize, noun: &str) -> String {
    if count == 1 {
        format!("{count} {noun}")
    } else {
        format!("{count} {noun}s")
    }
}

fn reviewed_through(proposal: &Value) -> String {
    if truthy(proposal) {
        format!("through {}", str_of(proposal))
    } else {
        "without a reviewing proposal".to_string()
    }
}

fn apply_records(event: &Object) -> Vec<String> {
    let mut found: Vec<String> = Vec::new();
    let named: Vec<&str> = match event.get("records") {
        Some(Value::Array(items)) => items.iter().filter_map(Value::as_str).collect(),
        Some(Value::Object(records)) => records.iter().map(|(key, _)| key.as_str()).collect(),
        _ => Vec::new(),
    };
    for record in named {
        if !found.iter().any(|seen| seen == record) {
            found.push(record.to_string());
        }
    }
    found
}

fn review_history(root: &Path) -> Result<History, String> {
    let mut history = History::new();
    let Some(events) = audit_events(root)? else {
        return Ok(history);
    };
    for event in &events {
        match event.get("operation").and_then(Value::as_str) {
            Some(APPLY_OPERATION) => {
                let proposal = value_of(event, "proposal_id");
                for record in apply_records(event) {
                    history.entry(record).or_default().push(proposal.clone());
                }
            }
            Some(MIGRATE_OPERATION) => {
                if let Some(Value::Object(reviewed)) = event.get("reviewed_by") {
                    for (record, proposal) in reviewed.iter() {
                        history
                            .entry(record.clone())
                            .or_default()
                            .push(proposal.clone());
                    }
                }
            }
            _ => {}
        }
    }
    Ok(history)
}

fn summarized_with_default(record: &Object) -> Object {
    let mut shown = summarized(record);
    if matches!(shown.get("status"), Some(Value::Null)) {
        shown.insert("status", Value::Str(DEFAULT_STATUS.to_string()));
    }
    shown
}

fn with_status(reached: Vec<Object>) -> Vec<Object> {
    reached
        .into_iter()
        .map(|mut item| {
            if matches!(item.get("status"), Some(Value::Null)) {
                item.insert("status", Value::Str(DEFAULT_STATUS.to_string()));
            }
            item
        })
        .collect()
}

fn context_row(
    registry: &Registry,
    root: &Path,
    library: &Library,
    identifier: &str,
    files: &[String],
) -> Result<Object, String> {
    let found = library
        .raw("contexts")
        .iter()
        .find(|entry| entry.get("id").and_then(Value::as_str) == Some(identifier));
    let mut row = Object::new();
    row.insert("id", Value::Str(identifier.to_string()));
    row.insert("changed_files", strings(files.to_vec()));
    let Some(context) = found else {
        row.insert("model", Value::Null);
        return Ok(row);
    };
    let model = context_model(registry, root, identifier, context.clone())?;
    let mut summary = context_summary(&model);
    let shown = summarized_with_default(context);
    if let Some(Value::Object(held)) = summary.get_mut("context") {
        for (key, value) in shown.iter() {
            held.insert(key.clone(), value.clone());
        }
    }
    row.insert("model", Value::Object(summary));
    Ok(row)
}

fn proof_of(
    changes_root: &Path,
    proposals: &[Value],
    id: &str,
    packages: &mut Packages,
) -> Result<Option<Vec<Value>>, String> {
    if packages.is_none() {
        *packages = Some(readable_packages(changes_root)?);
    }
    let readable = packages.as_deref().unwrap_or_default();
    for proposal in proposals.iter().rev() {
        match obligations_proving(readable, proposal, ("rule", id))? {
            None => return Ok(None),
            Some(found) if !found.is_empty() => return Ok(Some(found)),
            Some(_) => {}
        }
    }
    Ok(Some(Vec::new()))
}

fn with_check(
    mut record: Object,
    history: &History,
    changes_root: Option<&Path>,
    packages: &mut Packages,
) -> Result<Object, String> {
    if text_of(&record, "asset") != "rules" {
        return Ok(record);
    }
    let id = text_of(&record, "id");
    let proposals = history
        .get(&format!("rules/{id}"))
        .filter(|found| !found.is_empty());
    let found = match (changes_root, proposals) {
        (Some(changes_root), Some(proposals)) => proof_of(changes_root, proposals, &id, packages)?,
        _ => None,
    };
    let first = found
        .and_then(|obligations| obligations.into_iter().next())
        .and_then(|first| first.as_object().cloned());
    match first {
        Some(first) => {
            record.insert("check", value_of(&first, "check"));
            record.insert("cwd", value_of(&first, "working_directory"));
        }
        None => {
            record.insert("check", Value::Null);
            record.insert("cwd", Value::Null);
        }
    }
    Ok(record)
}

fn dependents_of(graph: &DependencyGraph, asset: &str, id: &str) -> Vec<Object> {
    with_status(graph.reached(asset, id))
}

fn record_row(graph: &DependencyGraph, asset: &str, id: &str, notes: &Object) -> Object {
    let mut shown = Object::new();
    let record = graph.record(asset, id);
    match record {
        Some(record) => {
            for (key, value) in summarized_with_default(record).iter() {
                shown.insert(key.clone(), value.clone());
            }
        }
        None => shown.insert("id", Value::Str(id.to_string())),
    }
    shown.insert("asset", Value::Str(asset.to_string()));
    for (key, value) in notes.iter() {
        shown.insert(key.clone(), value.clone());
    }
    let dependents = record.map_or_else(Vec::new, |_| dependents_of(graph, asset, id));
    shown.insert("dependents", objects(dependents));
    shown
}

fn by_id(entries: &[Object]) -> BTreeMap<String, &Object> {
    entries
        .iter()
        .map(|entry| (str_of(&value_of(entry, "id")), entry))
        .collect()
}

fn registry_changes(
    root: &Path,
    repo_root: &Path,
    base: &str,
    library: &Library,
    history: &History,
) -> Result<Vec<Object>, String> {
    let before = store::committed_documents(repo_root, base, root)?.unwrap_or_default();
    let mut changes = Vec::new();
    for (asset, key, _) in ASSETS {
        let then_records: Vec<Object> = before
            .get(asset)
            .and_then(Value::as_object)
            .and_then(|document| document.get(key))
            .and_then(Value::as_array)
            .unwrap_or_default()
            .iter()
            .filter_map(Value::as_object)
            .cloned()
            .collect();
        let then = by_id(&then_records);
        let now = by_id(library.raw(asset));
        let mut ids: Vec<&String> = then.keys().chain(now.keys()).collect();
        ids.sort();
        ids.dedup();
        for id in ids {
            let (change, record) = match (then.get(id), now.get(id)) {
                (Some(old), None) => ("removed", *old),
                (None, Some(new)) => ("added", *new),
                (Some(old), Some(new))
                    if !equal(
                        &Value::Object((*old).clone()),
                        &Value::Object((*new).clone()),
                    ) =>
                {
                    ("amended", *new)
                }
                _ => continue,
            };
            let shown = summarized(record);
            let status = match shown.get("status") {
                Some(status) if truthy(status) => status.clone(),
                _ => Value::Str(DEFAULT_STATUS.to_string()),
            };
            let mut row = Object::new();
            row.insert("asset", Value::Str((*asset).to_string()));
            row.insert("id", Value::Str(id.clone()));
            row.insert("change", Value::Str(change.to_string()));
            row.insert("status", status);
            row.insert("summary", value_of(&shown, "summary"));
            let through = history
                .get(&format!("{asset}/{id}"))
                .and_then(|proposals| proposals.last())
                .cloned()
                .unwrap_or(Value::Null);
            row.insert("through", through);
            changes.push(row);
        }
    }
    Ok(changes)
}

fn committed_source_map(
    repo_root: &Path,
    base: &str,
    path: &Path,
) -> Result<Option<Value>, String> {
    let relative = py::relative_to(&py::resolve_path(path), &py::resolve_path(repo_root))?;
    let shown = git::run_git_bytes(repo_root, &["show", &format!("{base}:{relative}")])?;
    if shown.code != 0 {
        return Ok(None);
    }
    let invalid = || format!("{base}:{relative} is not valid JSON");
    let text = String::from_utf8(shown.stdout).map_err(|_| invalid())?;
    json::parse_keeping_last_duplicate(&text)
        .map(Some)
        .map_err(|()| invalid())
}

fn discovered(source_map: &Value) -> Result<BTreeSet<String>, String> {
    let Value::Object(map) = source_map else {
        return Err(format!(
            "'{}' object has no attribute 'get'",
            python_type(source_map)
        ));
    };
    let mut found = BTreeSet::new();
    let groups = match map.get("source_groups") {
        Some(value) if truthy(value) => value.as_array().unwrap_or_default(),
        _ => &[],
    };
    for group in groups {
        let Value::Object(group) = group else {
            return Err(format!(
                "'{}' object has no attribute 'get'",
                python_type(group)
            ));
        };
        let paths = match group.get("paths") {
            Some(value) if truthy(value) => value.as_array().unwrap_or_default(),
            _ => &[],
        };
        found.extend(paths.iter().map(repr_of));
    }
    Ok(found)
}

fn python_type(value: &Value) -> &'static str {
    match value {
        Value::Null => "NoneType",
        Value::Bool(_) => "bool",
        Value::Int(_) => "int",
        Value::Float(_) => "float",
        Value::Str(_) => "str",
        Value::Array(_) => "list",
        Value::Object(_) => "dict",
    }
}

fn source_map_change(root: &Path, repo_root: &Path, base: &str) -> Result<Object, String> {
    let path = root.join("source-map.json");
    let then = committed_source_map(repo_root, base, &path)?;
    let then = match then {
        Some(value) if truthy(&value) => value,
        _ => Value::Object(Object::new()),
    };
    let now = if path.is_file() {
        load_json(&path)?
    } else {
        Object::new()
    };
    let mut change = Object::new();
    if equal(&then, &Value::Object(now.clone())) {
        change.insert("changed", Value::Bool(false));
        return Ok(change);
    }
    let before = discovered(&then)?;
    let after = discovered(&Value::Object(now.clone()))?;
    let confirmation = if now.iter().next().is_some() {
        value_of(&now, "selection_status")
    } else {
        Value::Str("removed".to_string())
    };
    change.insert("changed", Value::Bool(true));
    change.insert("confirmed_by", value_of(&now, "confirmed_by"));
    change.insert("confirmation", confirmation);
    change.insert(
        "discovered_added",
        Value::int(after.difference(&before).count()),
    );
    change.insert(
        "discovered_removed",
        Value::int(before.difference(&after).count()),
    );
    Ok(change)
}

fn source_map_line(change: &Object) -> String {
    let added = int_of(change.get("discovered_added"));
    let removed = int_of(change.get("discovered_removed"));
    let noun = if added + removed == 1 {
        "file"
    } else {
        "files"
    };
    let counts = format!("(+{added} -{removed} discovered {noun})");
    let confirmation = value_of(change, "confirmation");
    if confirmation.as_str() == Some(CONFIRMED) {
        format!(
            "source map re-confirmed in this range by {} {counts}",
            text_of(change, "confirmed_by")
        )
    } else {
        format!(
            "source map changed in this range and is {}, not confirmed {counts}",
            str_of(&confirmation)
        )
    }
}

fn int_of(value: Option<&Value>) -> i64 {
    match value {
        Some(Value::Int(digits)) => digits.parse().unwrap_or(0),
        _ => 0,
    }
}

fn shown_evidence(report: Option<&Object>) -> Value {
    let Some(report) = report else {
        return Value::Null;
    };
    let mut summary = Object::new();
    if let Some(Value::Object(held)) = report.get("summary") {
        for (key, value) in held.iter().filter(|(key, _)| key != "by_source_kind") {
            summary.insert(key.clone(), value.clone());
        }
    }
    let mut shown = Object::new();
    shown.insert("summary", Value::Object(summary));
    shown.insert(
        "paths",
        report
            .get("paths")
            .cloned()
            .unwrap_or_else(|| Value::Object(Object::new())),
    );
    Value::Object(shown)
}

fn failing(evidence: &Value) -> String {
    let Some(summary) = evidence
        .as_object()
        .and_then(|held| held.get("summary"))
        .and_then(Value::as_object)
    else {
        return String::new();
    };
    let counts: Vec<(&str, i64)> = FAILING
        .iter()
        .map(|status| (*status, int_of(summary.get(status))))
        .filter(|(_, count)| *count != 0)
        .collect();
    if counts.is_empty() {
        return String::new();
    }
    let total: i64 = counts.iter().map(|(_, count)| count).sum();
    let noun = if total == 1 { "citation" } else { "citations" };
    let parts: Vec<String> = counts
        .iter()
        .map(|(status, count)| format!("{count} {status}"))
        .collect();
    format!("{} {noun}", parts.join(", "))
}

fn change_attention(brief: &Object) -> Vec<String> {
    let verdict = text_of(brief, "verdict");
    let sentence = VERDICTS
        .iter()
        .find(|(name, _)| *name == verdict)
        .map_or("", |(_, sentence)| *sentence);
    let mut attention = vec![sentence.to_string()];
    let records: Vec<&Object> = object_list(brief, "records").collect();
    for record in &records {
        let name = format!("{}/{}", text_of(record, "asset"), text_of(record, "id"));
        if record.get("changed_evidence").is_some() {
            attention.push(format!(
                "{name} cites {} at lines that changed; check that its statement still holds before reciting: {}",
                string_list(record.get("changed_evidence")).join(", "),
                text_of(record, "summary")
            ));
        } else if record.get("touched_evidence").is_some() {
            attention.push(format!(
                "{name} cites {}, which the change overlaps; re-read it.",
                string_list(record.get("touched_evidence")).join(", ")
            ));
        }
    }
    for entry in object_list(brief, "outside_every_context") {
        if entry.get("cited_as").and_then(Value::as_str) != Some(PROOF_KIND) {
            attention.push(format!(
                "{} lies outside every Context but is cited by {}; place it or move the citation.",
                text_of(entry, "path"),
                string_list(entry.get("cited_by")).join(", ")
            ));
        }
    }
    let rules: Vec<&&Object> = records
        .iter()
        .filter(|record| text_of(record, "asset") == "rules")
        .collect();
    if !rules.is_empty() {
        let verb = if rules.len() == 1 { "cites" } else { "cite" };
        let then = if rules
            .iter()
            .all(|rule| rule.get("check").is_some_and(truthy))
        {
            "run the checks listed on them"
        } else {
            "re-run their checks, which get-record names"
        };
        attention.push(format!(
            "{} {verb} lines this change touched; {then}.",
            counted(rules.len(), "rule")
        ));
    }
    let failed = failing(brief.get("evidence").unwrap_or(&Value::Null));
    if !failed.is_empty() {
        attention.push(format!(
            "{failed} of the changed files; recite only a record whose statement still holds."
        ));
    }
    let mut groups: Vec<(String, Value, usize)> = Vec::new();
    for change in object_list(brief, "registry_changes") {
        let asset = text_of(change, "asset");
        let through = value_of(change, "through");
        match groups
            .iter_mut()
            .find(|(seen, held, _)| *seen == asset && equal(held, &through))
        {
            Some(group) => group.2 += 1,
            None => groups.push((asset, through, 1)),
        }
    }
    for (asset, through, count) in groups {
        let reviewed = if truthy(&through) {
            "last reviewed "
        } else {
            ""
        };
        attention.push(format!(
            "{asset}: {} changed in this range, {reviewed}{}.",
            counted(count, "record"),
            reviewed_through(&through)
        ));
    }
    if let Some(Value::Object(map)) = brief.get("source_map")
        && map.get("changed") == Some(&Value::Bool(true))
    {
        attention.push(source_map_line(map));
    }
    attention
}

fn record_attention(brief: &Object) -> Vec<String> {
    let record = brief.get("record").and_then(Value::as_object);
    let mut attention = Vec::new();
    if let Some(record) = record {
        let status = value_of(record, "status");
        if status.as_str() != Some("reviewed") {
            attention.push(format!(
                "The record is {}, not a reviewed constraint.",
                str_of(&status)
            ));
        }
    }
    let reached = list_of(brief, "dependents").len();
    if reached != 0 {
        let verb = if reached == 1 { "reaches" } else { "reach" };
        attention.push(format!(
            "{} {verb} it through relations and may need the same change; they are listed.",
            counted(reached, "record")
        ));
    }
    let evidence = brief.get("evidence").unwrap_or(&Value::Null);
    if matches!(evidence, Value::Null) {
        attention
            .push("The record cites nothing; it cannot be verified against the code.".to_string());
    } else {
        let failed = failing(evidence);
        if !failed.is_empty() {
            attention.push(format!(
                "{failed}; recite them before relying on the record."
            ));
        }
    }
    if attention.is_empty() {
        attention
            .push("Nothing stands out; the record is reviewed, cited and verified.".to_string());
    }
    attention
}

fn contexts_of(record: &Object) -> Vec<String> {
    let mut found: BTreeSet<String> = CONTEXT_FIELDS
        .iter()
        .filter_map(|name| record.get(name).and_then(Value::as_str))
        .map(str::to_string)
        .collect();
    for name in CONTEXT_LISTS {
        found.extend(string_items(record.get(name)));
    }
    found.into_iter().collect()
}

fn string_items(value: Option<&Value>) -> Vec<String> {
    match value {
        Some(Value::Array(items)) => items
            .iter()
            .filter_map(Value::as_str)
            .map(str::to_string)
            .collect(),
        Some(Value::Str(text)) => text.chars().map(|c| c.to_string()).collect(),
        Some(Value::Object(object)) => object.iter().map(|(key, _)| key.clone()).collect(),
        _ => Vec::new(),
    }
}

fn dependency_graph(library: &Library) -> DependencyGraph {
    DependencyGraph::from_records(library.in_order())
}

pub fn brief_for_change(root: &Path, repo_root: &Path, base: &str) -> Result<Object, String> {
    let changes = impact::changed_files_since(repo_root, base)?;
    let registry = Registry::open(root)?;
    let library = Library::read(&registry)?;
    let found = impact::boundary_impact(&library, root, repo_root, base, &changes)?;
    let mut touched: BTreeMap<(String, String), Object> = BTreeMap::new();
    let touched_now = impact::touched_records(&library, repo_root, base, &changes)?;
    let overlapping = list_of(&found, "boundary_evidence_touched");
    for entry in touched_now
        .iter()
        .chain(overlapping)
        .filter_map(Value::as_object)
    {
        touched
            .entry(key_of(entry))
            .or_default()
            .insert("touched_evidence", value_of(entry, "touched_evidence"));
    }
    for entry in object_list(&found, "boundary_evidence_changed") {
        touched
            .entry(key_of(entry))
            .or_default()
            .insert("changed_evidence", value_of(entry, "changed_evidence"));
    }
    let evidence = changed_file_evidence(root, repo_root, &found, &mut touched)?;
    let graph = dependency_graph(&library);
    let history = review_history(root)?;
    let changes_root = Some(root.join("changes")).filter(|beside| beside.is_dir());
    let mut packages: Packages = None;
    let mut contexts = Vec::new();
    if let Some(Value::Object(held)) = found.get("contexts_touched") {
        for (id, files) in held.iter() {
            let files = string_list(Some(files));
            contexts.push(context_row(&registry, root, &library, id, &files)?);
        }
    }
    let mut records = Vec::new();
    for ((asset, id), notes) in &touched {
        let row = record_row(&graph, asset, id, notes);
        records.push(with_check(
            row,
            &history,
            changes_root.as_deref(),
            &mut packages,
        )?);
    }
    let mut subject = Object::new();
    subject.insert("base", Value::Str(base.to_string()));
    subject.insert("changed_files", value_of(&found, "changed_files"));
    let mut brief = Object::new();
    brief.insert("format", Value::Str(FORMAT.to_string()));
    brief.insert("subject", Value::Object(subject));
    brief.insert("verdict", value_of(&found, "verdict"));
    brief.insert("contexts", objects(contexts));
    brief.insert("records", objects(records));
    let changes_listed = registry_changes(root, repo_root, base, &library, &history)?;
    brief.insert("registry_changes", objects(changes_listed));
    brief.insert(
        "source_map",
        Value::Object(source_map_change(root, repo_root, base)?),
    );
    brief.insert("evidence", shown_evidence(evidence.as_ref()));
    brief.insert(
        "outside_every_context",
        value_of(&found, "cited_outside_contexts"),
    );
    let attention = change_attention(&brief);
    brief.insert("attention", strings(attention));
    Ok(brief)
}

fn key_of(entry: &Object) -> (String, String) {
    (text_of(entry, "asset"), text_of(entry, "id"))
}

fn changed_file_evidence(
    root: &Path,
    repo_root: &Path,
    found: &Object,
    touched: &mut BTreeMap<(String, String), Object>,
) -> Result<Option<Object>, String> {
    let mut changed_paths: BTreeSet<String> = BTreeSet::new();
    if let Some(Value::Object(held)) = found.get("contexts_touched") {
        for (_, files) in held.iter() {
            changed_paths.extend(string_list(Some(files)));
        }
    }
    changed_paths.extend(string_list(found.get("outside_every_context")));
    if changed_paths.is_empty() {
        return Ok(None);
    }
    let paths: Vec<String> = changed_paths.into_iter().collect();
    let mut verifier = Verifier::new(repo_root, false);
    let report = evidence::verify_evidence(root, &mut verifier, &paths, &Vec::new())?;
    for item in object_list(&report, "results") {
        let status = item
            .get("status")
            .and_then(Value::as_str)
            .unwrap_or_default();
        if !CHANGED_CITATION.contains(&status) {
            continue;
        }
        let notes = touched.entry(key_of(item)).or_default();
        let Some(path) = item.get("path") else {
            return Err("'path'".to_string());
        };
        let mut merged: BTreeSet<String> = string_list(notes.get("changed_evidence"))
            .into_iter()
            .collect();
        merged.insert(str_of(path));
        notes.insert("changed_evidence", strings(merged));
    }
    Ok(Some(report))
}

pub fn brief_for_record(
    root: &Path,
    repo_root: &Path,
    asset: &str,
    record_id: &str,
) -> Result<Object, String> {
    let Some(record) = read_record(root, asset, record_id)? else {
        return Err(format!("{asset} record not found: {record_id}"));
    };
    let paths: Vec<String> = cited_paths(&record)?.into_iter().collect();
    let registry = Registry::open(root)?;
    let library = Library::read(&registry)?;
    let graph = dependency_graph(&library);
    let mut shown = summarized_with_default(&record);
    shown.insert("asset", Value::Str(asset.to_string()));
    let mut contexts = Vec::new();
    for identifier in contexts_of(&record) {
        contexts.push(context_row(&registry, root, &library, &identifier, &[])?);
    }
    let evidence = if paths.is_empty() {
        None
    } else {
        let mut verifier = Verifier::new(repo_root, false);
        Some(evidence::verify_evidence(
            root,
            &mut verifier,
            &paths,
            &Vec::new(),
        )?)
    };
    let mut subject = Object::new();
    subject.insert("asset", Value::Str(asset.to_string()));
    subject.insert("id", Value::Str(record_id.to_string()));
    let mut brief = Object::new();
    brief.insert("format", Value::Str(FORMAT.to_string()));
    brief.insert("subject", Value::Object(subject));
    brief.insert("record", Value::Object(shown));
    brief.insert("contexts", objects(contexts));
    brief.insert(
        "dependents",
        objects(dependents_of(&graph, asset, record_id)),
    );
    brief.insert("evidence", shown_evidence(evidence.as_ref()));
    let attention = record_attention(&brief);
    brief.insert("attention", strings(attention));
    Ok(brief)
}

fn line(entry: &Object) -> String {
    let summary = match entry.get("summary") {
        Some(found) if truthy(found) => format!(": {}", str_of(found)),
        _ => String::new(),
    };
    let asset = entry.get("asset").map(str_of).unwrap_or_default();
    format!(
        "{asset}/{} ({}){summary}",
        text_of(entry, "id"),
        text_of(entry, "status")
    )
    .trim_start_matches('/')
    .to_string()
}

fn dependent_lines(reached: &[Value]) -> Vec<String> {
    let lines: Vec<String> = reached
        .iter()
        .filter_map(Value::as_object)
        .map(|item| {
            format!(
                "- {} via {} at depth {}",
                line(item),
                text_of(item, "through"),
                text_of(item, "depth")
            )
        })
        .collect();
    if lines.is_empty() {
        vec!["- none".to_string()]
    } else {
        lines
    }
}

fn context_lines(context: &Object) -> Vec<String> {
    let model = context.get("model").and_then(Value::as_object);
    let heading = match model {
        None => format!("### {}", text_of(context, "id")),
        Some(model) => format!(
            "### {}",
            line(
                model
                    .get("context")
                    .and_then(Value::as_object)
                    .unwrap_or(model)
            )
        ),
    };
    let mut lines = vec![heading, String::new()];
    let files = string_list(context.get("changed_files"));
    if !files.is_empty() {
        lines.push(format!("Changed: {}", files.join(", ")));
        lines.push(String::new());
    }
    let Some(model) = model else {
        lines.push("Not in the Registry.".to_string());
        lines.push(String::new());
        return lines;
    };
    for key in ["rules", "interactions", "contracts", "events"] {
        let entries = list_of(model, key);
        if entries.is_empty() {
            continue;
        }
        lines.push(format!("{key}:"));
        lines.extend(
            entries
                .iter()
                .filter_map(Value::as_object)
                .map(|entry| format!("- {}", line(entry))),
        );
        lines.push(String::new());
    }
    lines
}

fn record_lines(record: &Object) -> Vec<String> {
    let mut lines = vec![format!("### {}", line(record)), String::new()];
    for key in ["changed_evidence", "touched_evidence"] {
        if record.get(key).is_some() {
            lines.push(format!(
                "{}: {}",
                key.replace('_', " "),
                string_list(record.get(key)).join(", ")
            ));
            lines.push(String::new());
        }
    }
    if let Some(check) = record.get("check") {
        let shown = if truthy(check) {
            format!("`{}` in `{}`", str_of(check), text_of(record, "cwd"))
        } else {
            "none known; get-record names what proves it".to_string()
        };
        lines.push(format!("Check: {shown}"));
        lines.push(String::new());
    }
    lines.push("Dependents:".to_string());
    lines.extend(dependent_lines(list_of(record, "dependents")));
    lines.push(String::new());
    lines
}

fn registry_change_lines(changes: &[Value]) -> Vec<String> {
    let lines: Vec<String> = changes
        .iter()
        .filter_map(Value::as_object)
        .map(|change| {
            format!(
                "- {} {}: {}",
                text_of(change, "change"),
                reviewed_through(&value_of(change, "through")),
                line(change)
            )
        })
        .collect();
    if lines.is_empty() {
        vec!["- none".to_string()]
    } else {
        lines
    }
}

fn evidence_lines(evidence: &Value) -> Vec<String> {
    let Some(summary) = evidence
        .as_object()
        .and_then(|held| held.get("summary"))
        .and_then(Value::as_object)
    else {
        return vec!["No citations to verify.".to_string()];
    };
    summary
        .iter()
        .filter(|(_, count)| truthy(count))
        .map(|(status, count)| format!("- {status}: {}", str_of(count)))
        .collect()
}

pub fn to_markdown(brief: &Object) -> String {
    let subject = brief
        .get("subject")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default();
    let title = match subject.get("base") {
        Some(base) => format!("since {}", str_of(base)),
        None => format!("{}/{}", text_of(&subject, "asset"), text_of(&subject, "id")),
    };
    let mut lines = vec![format!("# Change brief: {title}"), String::new()];
    if let Some(verdict) = brief.get("verdict") {
        lines.push(format!("**Verdict:** `{}`", str_of(verdict)));
        lines.push(String::new());
    }
    if let Some(Value::Object(record)) = brief.get("record") {
        lines.push(format!("**Record:** {}", line(record)));
        lines.push(String::new());
    }
    lines.push("## Attention".to_string());
    lines.push(String::new());
    lines.extend(
        string_list(brief.get("attention"))
            .into_iter()
            .map(|item| format!("- {item}")),
    );
    lines.push(String::new());
    if !list_of(brief, "contexts").is_empty() {
        lines.push("## Contexts".to_string());
        lines.push(String::new());
        for context in object_list(brief, "contexts") {
            lines.extend(context_lines(context));
        }
    }
    if !list_of(brief, "records").is_empty() {
        lines.push("## Records to re-read".to_string());
        lines.push(String::new());
        for record in object_list(brief, "records") {
            lines.extend(record_lines(record));
        }
    }
    if brief.get("registry_changes").is_some() {
        lines.push("## Registry changes".to_string());
        lines.push(String::new());
        lines.extend(registry_change_lines(list_of(brief, "registry_changes")));
        lines.push(String::new());
    }
    if let Some(Value::Object(map)) = brief.get("source_map")
        && map.get("changed").is_some_and(truthy)
    {
        lines.push("## Source map".to_string());
        lines.push(String::new());
        lines.push(format!("- {}", source_map_line(map)));
        lines.push(String::new());
    }
    if let Some(dependents) = brief.get("dependents")
        && !matches!(dependents, Value::Null)
    {
        lines.push("## Dependents".to_string());
        lines.push(String::new());
        lines.extend(dependent_lines(list_of(brief, "dependents")));
        lines.push(String::new());
    }
    lines.push("## Evidence".to_string());
    lines.push(String::new());
    lines.extend(evidence_lines(
        brief.get("evidence").unwrap_or(&Value::Null),
    ));
    lines.push(String::new());
    lines.join("\n")
}
