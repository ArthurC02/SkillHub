use std::collections::BTreeSet;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use crate::audit_append;
use crate::candidate_evidence::{classify_all, source_map_for, unclassified_paths};
use crate::changes::{init_change_package, validate_change_package};
use crate::cite::{citation, function_span, whole_number};
use crate::common::{EVIDENCE_FILE, OBLIGATIONS_FILE};
use crate::counterfactual::counterfactual_suite;
use crate::coverage::SourceKinds;
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::harvest_package::{Documents, harvest_package, obligation_of, write_harvest_package};
use crate::hitl::{
    Approval, ObligationRun, record_approval, record_test_result, same_command, submit_proposal,
    verify_proposal,
};
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::policy::existing_policy;
use crate::promotion::{promote_candidate, replace_promoted_fields};
use crate::proposals::iter_packages;
use crate::py::{self, PurePath};
use crate::pyvalue::{equal, str_of};
use crate::records::read_record;
use crate::registry_write::is_reviewed;
use crate::revision::as_stored;
use crate::sources::{discovery_placements, refresh_listing_advice};
use crate::store::{Registry, shown_path};
use crate::transaction::{remove_tree, with_writer_lock};
use crate::updates::{retract_candidate, upsert_candidate};

const FORMAT: &str = "domain-memory-harvest/v1";
const SPEC_FIELDS: [&str; 6] = ["name", "context", "proposer", "intent", "outcome", "scope"];
const RULE_FIELDS: [&str; 5] = ["id", "statement", "test_file", "test", "check"];
const DECISIONS: [(&str, &str); 3] = [
    ("ownership", "ownership_decision"),
    ("boundary", "boundary_decision"),
    ("design", "implementation_design"),
];
const SUPERSEDING: [&str; 2] = ["amended", "reproven"];
const HARVESTED_FIELDS: [&str; 4] = ["statement", "contexts", "examples", "evidence"];
const UNFINISHED: [&str; 2] = ["draft", "submitted"];
const DISCARDABLE: [&str; 3] = ["draft", "submitted", "verified"];

fn nothing_written(stage: &str) -> Option<&'static str> {
    match stage {
        "spec" => {
            Some("no package was written; fix what the spec errors name, then run harvest again")
        }
        "citation" => Some(
            "no package was written; confirm the sources the cited paths belong to, or cite others, then run harvest again",
        ),
        "package" => Some(
            "nothing was written; the error names the way out, another spec name or discard-harvest of the package there, then run harvest again",
        ),
        "counterfactual" => {
            Some("no package was written; make each counterfactual killed, then run harvest again")
        }
        _ => None,
    }
}

fn next_of(stage: &str) -> &'static str {
    match stage {
        "draft" => {
            "the draft package passes the submitted preview; run harvest --through verified, or submit-proposal, record-test-result per obligation, record-approval and verify-proposal"
        }
        "submitted" => {
            "the proposal is submitted; run harvest --through verified to record its test results, approval and verification"
        }
        "tested" => {
            "the proposal is submitted with its test results; record-approval, then verify-proposal"
        }
        _ => "sign a commit holding the package, then ship-package --commit <that commit>",
    }
}

struct Harvest {
    registry_root: PathBuf,
    repo_root: PathBuf,
    spec: Object,
    package_root: Option<PathBuf>,
    through: String,
    rules: Vec<Object>,
    errors: Vec<String>,
    stopped_at: &'static str,
    written: bool,
    resumed_from: Option<String>,
    interrupted: bool,
    relisting: Option<String>,
}

fn field<'a>(object: &'a Object, key: &str) -> &'a Value {
    object.get(key).unwrap_or(&Value::Null)
}

fn text_of(object: &Object, key: &str) -> String {
    str_of(field(object, key))
}

fn is_text(value: Option<&Value>) -> bool {
    value
        .and_then(Value::as_str)
        .is_some_and(|text| !py::strip(text).is_empty())
}

fn spec_rules(spec: &Object) -> Vec<Object> {
    match spec.get("rules") {
        Some(Value::Array(rules)) => rules.iter().filter_map(Value::as_object).cloned().collect(),
        _ => Vec::new(),
    }
}

fn shown(path: &Path, repo_root: &Path) -> String {
    shown_path(path, Some(repo_root))
}

impl Harvest {
    fn package_root(&self) -> &Path {
        self.package_root.as_deref().unwrap_or(Path::new("."))
    }

    fn shown_package(&self) -> String {
        shown(self.package_root(), &self.repo_root)
    }

    fn report(&self) -> Object {
        let mut report = Object::new();
        report.insert("format", Value::Str(FORMAT.to_string()));
        report.insert(
            "package",
            if self.written {
                Value::Str(self.shown_package())
            } else {
                Value::Null
            },
        );
        report.insert("written", Value::Bool(self.written));
        report.insert("resumed", Value::Bool(self.resumed_from.is_some()));
        report.insert("stopped_at", Value::Str(self.stopped_at.to_string()));
        report.insert(
            "rules",
            Value::Array(self.rules.iter().cloned().map(Value::Object).collect()),
        );
        report.insert(
            "errors",
            Value::Array(self.errors.iter().cloned().map(Value::Str).collect()),
        );
        report.insert("next", Value::Str(self.next_step()));
        report
    }

    fn next_step(&self) -> String {
        if self.stopped_at == "citation"
            && let Some(relisting) = &self.relisting
        {
            return format!(
                "no package was written; discovery now places a cited path, so {relisting}, then run harvest again; otherwise confirm the sources the cited paths belong to, or cite others"
            );
        }
        if let Some(message) = nothing_written(self.stopped_at) {
            return message.to_string();
        }
        if self.interrupted {
            let package = self.shown_package();
            return format!(
                "the draft records and the package at {package} are written; run the same harvest --spec again to resume from {}, or discard-harvest --package-root {package} to undo it",
                self.stopped_at
            );
        }
        if !self.errors.is_empty() && self.stopped_at == "draft" {
            return "the draft package is written; fix what the preview names, then validate-change-package --as-status submitted".to_string();
        }
        next_of(self.stopped_at).to_string()
    }
}

pub fn harvest(
    registry_root: &Path,
    repo_root: &Path,
    spec: &Object,
    changes_root: Option<&Path>,
    through: &str,
) -> Result<Object, String> {
    let registry_root = py::resolve_path(registry_root);
    let repo_root = py::resolve_path(repo_root);
    let mut run = Harvest {
        errors: spec_errors(spec),
        registry_root,
        repo_root,
        spec: spec.clone(),
        package_root: None,
        through: through.to_string(),
        rules: Vec::new(),
        stopped_at: "spec",
        written: false,
        resumed_from: None,
        interrupted: false,
        relisting: None,
    };
    if !run.errors.is_empty() {
        return Ok(run.report());
    }
    let base = changes_root
        .map(Path::to_path_buf)
        .unwrap_or_else(|| run.registry_root.join("changes"));
    let name = text_of(spec, "name");
    run.package_root = Some(PathBuf::from(
        PurePath::new(&base.to_string_lossy())
            .joined(&name)
            .native(),
    ));
    run.stopped_at = "citation";
    let records = records(&mut run)?;
    if !run.errors.is_empty() {
        return Ok(run.report());
    }
    let records: Vec<Object> = records.into_iter().flatten().collect();
    run.stopped_at = "package";
    check_package_directory(&mut run, &records)?;
    if !run.errors.is_empty() {
        return Ok(run.report());
    }
    run.stopped_at = "counterfactual";
    let verdicts = counterfactuals(&mut run)?;
    if !run.errors.is_empty() {
        return Ok(run.report());
    }
    if let Err(error) = write(&mut run, &records, &verdicts) {
        if !run.written {
            return Err(error);
        }
        run.errors.push(error);
        run.interrupted = true;
    }
    Ok(run.report())
}

fn write(run: &mut Harvest, records: &[Object], verdicts: &Object) -> Result<(), String> {
    run.written = run.resumed_from.is_some();
    run.stopped_at = if run.resumed_from.as_deref() == Some("submitted") {
        "submitted"
    } else {
        "draft"
    };
    candidates(run, records)?;
    if run.resumed_from.as_deref() != Some("submitted") {
        package(run, records, verdicts)?;
        let preview = validate_change_package(
            run.package_root(),
            Some(&run.registry_root),
            Some("submitted"),
            Some(&run.repo_root),
        )?;
        if !preview.is_empty() {
            run.errors.extend(
                preview
                    .into_iter()
                    .map(|error| format!("once submitted: {error}")),
            );
            return Ok(());
        }
    }
    if run.through == "verified" {
        verify(run)?;
    }
    Ok(())
}

fn way_out(shown: &str, status: &Value) -> String {
    if status
        .as_str()
        .is_some_and(|text| DISCARDABLE.contains(&text))
    {
        format!(
            "to harvest again, discard-harvest --package-root {shown} --reason <why>, then run harvest again"
        )
    } else {
        "give the spec another name to harvest it as a new package".to_string()
    }
}

fn check_package_directory(run: &mut Harvest, records: &[Object]) -> Result<(), String> {
    let package_root = run.package_root().to_path_buf();
    let name = text_of(&run.spec, "name");
    if !package_root.is_dir() {
        return Ok(());
    }
    let mut entries =
        std::fs::read_dir(&package_root).map_err(|error| py::os_error(&error, &package_root))?;
    if entries.next().is_none() {
        return Ok(());
    }
    let package = run.shown_package();
    let proposal = load_json(&package_root.join(PROPOSAL_FILE)).ok();
    let Some(proposal) = proposal.filter(|proposal| {
        proposal.get("proposal_id").and_then(Value::as_str) == Some(name.as_str())
    }) else {
        run.errors.push(format!(
            "output directory is not empty: {package}; it holds no package of the harvest {name}, so give the spec another name or move what is there"
        ));
        return Ok(());
    };
    let status = field(&proposal, "status").clone();
    if !status
        .as_str()
        .is_some_and(|text| UNFINISHED.contains(&text))
    {
        run.errors.push(format!(
            "the package at {package} is {}; a finished harvest is not re-run; {}",
            str_of(&status),
            way_out(&package, &status)
        ));
        return Ok(());
    }
    let changed = if status.as_str() == Some("submitted") {
        changed_since_submitted(run, records, &proposal)?
    } else {
        Vec::new()
    };
    if !changed.is_empty() {
        run.errors.push(format!(
            "the package at {package} is submitted and {} changed since, but a submitted package is frozen; discard-harvest --package-root {package}, then run harvest again",
            changed.join(", ")
        ));
        return Ok(());
    }
    run.resumed_from = status.as_str().map(str::to_string);
    Ok(())
}

fn reviewed(registry_root: &Path, asset: &str, record: &Object) -> Result<bool, String> {
    Registry::open(registry_root)?.records(asset)?;
    Ok(is_reviewed(asset, record))
}

fn changed_since_submitted(
    run: &Harvest,
    records: &[Object],
    proposal: &Object,
) -> Result<Vec<String>, String> {
    let promoted: BTreeSet<String> = updated_records(proposal)
        .into_iter()
        .filter(|(asset, _)| asset == "rules")
        .map(|(_, id)| id)
        .collect();
    let named: BTreeSet<String> = spec_rules(&run.spec)
        .iter()
        .map(|rule| text_of(rule, "id"))
        .collect();
    let mut changed: BTreeSet<String> = named.symmetric_difference(&promoted).cloned().collect();
    for record in records {
        let id = text_of(record, "id");
        let existing = read_record(&run.registry_root, "rules", &id)?;
        let moved = match existing {
            None => true,
            Some(existing) => {
                !reviewed(&run.registry_root, "rules", &existing)?
                    && !same_draft(&existing, record)?
            }
        };
        if moved {
            changed.insert(id);
        }
    }
    Ok(changed.into_iter().collect())
}

fn updated_records(proposal: &Object) -> Vec<(String, String)> {
    let Some(Value::Array(updates)) = proposal.get("registry_updates") else {
        return Vec::new();
    };
    updates
        .iter()
        .filter_map(Value::as_object)
        .filter_map(|update| {
            let asset = update.get("asset")?.as_str()?;
            let id = update.get("record")?.as_object()?.get("id")?.as_str()?;
            Some((asset.to_string(), id.to_string()))
        })
        .collect()
}

fn spec_errors(spec: &Object) -> Vec<String> {
    let mut errors: Vec<String> = SPEC_FIELDS
        .iter()
        .filter(|name| !is_text(spec.get(name)))
        .map(|name| format!("{name} must be text"))
        .collect();
    errors.extend(
        DECISIONS
            .iter()
            .filter(|(name, _)| !matches!(spec.get(name), Some(Value::Object(_))))
            .map(|(name, _)| format!("{name} must be an object")),
    );
    if let Some(approval) = spec.get("approval")
        && !matches!(approval, Value::Null)
    {
        let valid = approval
            .as_object()
            .is_some_and(|given| is_text(given.get("role")) && is_text(given.get("reviewer")));
        if !valid {
            errors.push("approval must be an object with role and reviewer".to_string());
        }
    }
    let rules = match spec.get("rules") {
        Some(Value::Array(rules)) if !rules.is_empty() => rules,
        _ => {
            errors.push("rules must be a non-empty list".to_string());
            return errors;
        }
    };
    for (position, rule) in rules.iter().enumerate() {
        let name = match rule {
            Value::Object(held) if is_text(held.get("id")) => {
                format!("rule {}", text_of(held, "id"))
            }
            _ => format!("rule {}", position + 1),
        };
        errors.extend(
            rule_errors(rule)
                .into_iter()
                .map(|error| format!("{name}: {error}")),
        );
    }
    errors
}

fn rule_errors(rule: &Value) -> Vec<String> {
    let Value::Object(rule) = rule else {
        return vec!["must be an object".to_string()];
    };
    let mut errors: Vec<String> = RULE_FIELDS
        .iter()
        .filter(|name| !is_text(rule.get(name)))
        .map(|name| format!("{name} must be text"))
        .collect();
    let spans_valid = matches!(rule.get("enforcing"), Some(Value::Array(spans))
        if !spans.is_empty() && spans.iter().all(|span| span_of(span).is_some()));
    if !spans_valid {
        errors.push("enforcing must be a non-empty list of path:start-end".to_string());
    }
    let mutate_valid = rule
        .get("mutate")
        .and_then(Value::as_object)
        .is_some_and(|mutate| {
            ["file", "find", "replace"]
                .iter()
                .all(|name| is_text(mutate.get(name)))
        });
    if !mutate_valid {
        errors.push("mutate must be an object with file, find and replace".to_string());
    }
    for name in ["cwd", "command_profile", "level", "example"] {
        if let Some(value) = rule.get(name)
            && !matches!(value, Value::Null)
            && !is_text(Some(value))
        {
            errors.push(format!("{name} must be text"));
        }
    }
    errors
}

fn setting(spec: &Object, rule: &Object, name: &str) -> Value {
    for holder in [rule, spec] {
        if let Some(value) = holder.get(name)
            && !matches!(value, Value::Null)
        {
            return value.clone();
        }
    }
    match name {
        "command_profile" | "level" => Value::Str("unit".to_string()),
        "timeout" => Value::Int("600".to_string()),
        _ => Value::Null,
    }
}

fn span_of(value: &Value) -> Option<(String, String, String)> {
    let Value::Str(span) = value else {
        return None;
    };
    let (path, lines) = match span.rfind(':') {
        Some(at) => (&span[..at], &span[at + 1..]),
        None => ("", span.as_str()),
    };
    let (start, end) = lines.split_once('-').unwrap_or((lines, ""));
    if path.is_empty() {
        return None;
    }
    let first = whole_number(start)?;
    let last = if end.is_empty() {
        first.clone()
    } else {
        whole_number(end)?
    };
    Some((path.to_string(), first, last))
}

fn records(run: &mut Harvest) -> Result<Vec<Option<Object>>, String> {
    let source_map = source_map_for(&run.registry_root)?;
    let kinds = SourceKinds::of(&source_map);
    let mut records = Vec::new();
    for rule in spec_rules(&run.spec) {
        let id = text_of(&rule, "id");
        let mut summary = Object::new();
        summary.insert("id", field(&rule, "id").clone());
        summary.insert("obligation", Value::Str(obligation_of(&id)));
        run.rules.push(summary);
        let record = record(run, &rule, &kinds)?;
        if let Some(record) = &record
            && let Some(Value::Array(cited)) = record.get("evidence")
            && let Some(last) = cited.last()
            && let Some(summary) = run.rules.last_mut()
        {
            summary.insert("test_citation", last.clone());
        }
        records.push(record);
    }
    Ok(records)
}

fn cited_evidence(run: &Harvest, rule: &Object, kinds: &SourceKinds) -> Result<Vec<Value>, String> {
    let test_file = text_of(rule, "test_file");
    let (start, end) = function_span(&run.repo_root, &test_file, &text_of(rule, "test"))?;
    let mut evidence = Vec::new();
    let spans: &[Value] = match rule.get("enforcing") {
        Some(Value::Array(spans)) => spans,
        _ => &[],
    };
    for span in spans {
        let Some((path, first, last)) = span_of(span) else {
            continue;
        };
        let cited = citation(&run.repo_root, &path, &first, &last)?;
        evidence.push(kinds.classify(&Value::Object(cited)));
    }
    let tested = citation(
        &run.repo_root,
        &test_file,
        &start.to_string(),
        &end.to_string(),
    )?;
    evidence.push(kinds.classify(&Value::Object(tested)));
    Ok(evidence)
}

fn record(run: &mut Harvest, rule: &Object, kinds: &SourceKinds) -> Result<Option<Object>, String> {
    let id = text_of(rule, "id");
    let evidence = match cited_evidence(run, rule, kinds) {
        Ok(evidence) => evidence,
        Err(error) => {
            run.errors.push(format!("{id}: {error}"));
            return Ok(None);
        }
    };
    let mut wrapped = Object::new();
    wrapped.insert("evidence", Value::Array(evidence.clone()));
    let outside = unclassified_paths(&classify_all(&Value::Object(wrapped), kinds)?)?;
    if !outside.is_empty() {
        let hint = placement_hint(run, &outside)?;
        run.errors.push(format!(
            "{id} cites {} outside the confirmed sources; ship-package would refuse to review it, so confirm those sources first or cite others{hint}",
            outside.join(", ")
        ));
        return Ok(None);
    }
    let examples: Vec<Value> = match rule.get("examples") {
        Some(Value::Array(listed)) => listed.clone(),
        _ => match rule.get("example") {
            Some(example) if truthy(example) => vec![example.clone()],
            _ => Vec::new(),
        },
    };
    let mut record = Object::new();
    record.insert("id", field(rule, "id").clone());
    record.insert(
        "contexts",
        Value::Array(vec![field(&run.spec, "context").clone()]),
    );
    record.insert("statement", field(rule, "statement").clone());
    if !examples.is_empty() {
        record.insert("examples", Value::Array(examples));
    }
    record.insert("status", Value::Str("candidate".to_string()));
    record.insert("evidence", Value::Array(evidence));
    Ok(Some(record))
}

fn placement_hint(run: &mut Harvest, outside: &[String]) -> Result<String, String> {
    let policy = existing_policy(&run.registry_root)?;
    let placed = discovery_placements(&run.repo_root, outside, policy.as_ref())?;
    if placed.is_empty() {
        return Ok(String::new());
    }
    let listing = shown(&run.registry_root, &run.repo_root);
    let advice = refresh_listing_advice(&PurePath::new(&listing), &PurePath::new("."));
    run.relisting = Some(advice.clone());
    let places: Vec<String> = placed
        .iter()
        .map(|(path, kind)| format!("{path} under {kind}"))
        .collect();
    Ok(format!(
        "; discovery now places {}; {advice}",
        places.join(", ")
    ))
}

fn counterfactuals(run: &mut Harvest) -> Result<Object, String> {
    let rules = spec_rules(&run.spec);
    let mut mutations = Vec::new();
    for rule in &rules {
        let mut entry = rule
            .get("mutate")
            .and_then(Value::as_object)
            .cloned()
            .unwrap_or_default();
        entry.insert("test_command", field(rule, "check").clone());
        entry.insert("cwd", setting(&run.spec, rule, "cwd"));
        entry.insert("timeout", setting(&run.spec, rule, "timeout"));
        entry.insert(
            "obligation_id",
            Value::Str(obligation_of(&text_of(rule, "id"))),
        );
        mutations.push(Value::Object(entry));
    }
    let mut suite = Object::new();
    suite.insert("mutations", Value::Array(mutations));
    let report = counterfactual_suite(&run.repo_root.to_string_lossy(), &suite)
        .map_err(|refusal| refusal.reason)?;
    let results: Vec<&Object> = match report.get("results") {
        Some(Value::Array(rows)) => rows.iter().filter_map(Value::as_object).collect(),
        _ => Vec::new(),
    };
    let mut verdicts = Object::new();
    for ((rule, summary), result) in rules.iter().zip(run.rules.iter_mut()).zip(results) {
        let id = text_of(rule, "id");
        verdicts.insert(id.clone(), Value::Object(result.clone()));
        let verdict = field(result, "verdict").clone();
        summary.insert("counterfactual", verdict.clone());
        if verdict.as_str() != Some("killed") {
            run.errors.push(format!(
                "{id}: the counterfactual is {}: {}",
                str_of(&verdict),
                text_of(result, "reason")
            ));
        }
    }
    Ok(verdicts)
}

fn candidates(run: &mut Harvest, records: &[Object]) -> Result<(), String> {
    let mut pending = Vec::new();
    for (index, record) in records.iter().enumerate() {
        let id = text_of(record, "id");
        let existing = read_record(&run.registry_root, "rules", &id)?;
        let state = match &existing {
            Some(existing) if reviewed(&run.registry_root, "rules", existing)? => {
                Some(if same_content(existing, record) {
                    "reproven"
                } else {
                    "amended"
                })
            }
            Some(existing) if run.resumed_from.is_some() && same_draft(existing, record)? => {
                Some("unchanged")
            }
            _ => None,
        };
        match state {
            Some(state) => run.rules[index].insert("record", Value::Str(state.to_string())),
            None => pending.push(index),
        }
    }
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |elapsed| elapsed.as_nanos());
    let directory = std::env::temp_dir().join(format!("dm-harvest-{}-{nanos}", std::process::id()));
    std::fs::create_dir_all(&directory).map_err(|error| py::os_error(&error, &directory))?;
    let outcome = upsert_pending(run, records, &pending, &directory);
    let _ = std::fs::remove_dir_all(&directory);
    outcome
}

fn upsert_pending(
    run: &mut Harvest,
    records: &[Object],
    pending: &[usize],
    directory: &Path,
) -> Result<(), String> {
    for (position, index) in pending.iter().enumerate() {
        let record = &records[*index];
        let path = directory.join(format!("{position}-{}.json", text_of(record, "id")));
        write_json(&path, &Value::Object(record.clone()))?;
        let upserted = upsert_candidate(&run.registry_root, &run.repo_root, "rules", &[path])?;
        let state = if upserted.created > 0 {
            "created"
        } else {
            "revised"
        };
        run.rules[*index].insert("record", Value::Str(state.to_string()));
    }
    Ok(())
}

fn sorted_texts(value: Option<&Value>) -> Vec<String> {
    let mut items: Vec<String> = match value {
        Some(Value::Array(listed)) => listed.iter().map(str_of).collect(),
        _ => Vec::new(),
    };
    items.sort();
    items
}

fn listed_or_empty(value: Option<&Value>) -> Value {
    match value {
        Some(found) if truthy(found) => found.clone(),
        _ => Value::Array(Vec::new()),
    }
}

type Cited = Option<(Value, Value, Value)>;

fn cited_spans(evidence: &Value) -> Vec<Cited> {
    let Value::Array(references) = evidence else {
        return Vec::new();
    };
    let mut spans: Vec<Cited> = Vec::new();
    for reference in references {
        let cited = match reference {
            Value::Object(held) => {
                let none = Object::new();
                let lines = match held.get("lines") {
                    Some(Value::Object(lines)) => lines,
                    _ => &none,
                };
                Some((
                    field(held, "path").clone(),
                    field(lines, "start").clone(),
                    field(lines, "end").clone(),
                ))
            }
            other => span_of(other)
                .map(|(path, start, end)| (Value::Str(path), Value::Int(start), Value::Int(end))),
        };
        let known = spans.iter().any(|seen| same_cited(seen, &cited));
        if !known {
            spans.push(cited);
        }
    }
    spans
}

fn same_cited(left: &Cited, right: &Cited) -> bool {
    match (left, right) {
        (None, None) => true,
        (Some(one), Some(other)) => {
            equal(&one.0, &other.0) && equal(&one.1, &other.1) && equal(&one.2, &other.2)
        }
        _ => false,
    }
}

fn same_spans(left: &[Cited], right: &[Cited]) -> bool {
    left.len() == right.len()
        && left
            .iter()
            .all(|one| right.iter().any(|other| same_cited(one, other)))
}

fn same_content(existing: &Object, record: &Object) -> bool {
    equal(field(existing, "statement"), field(record, "statement"))
        && sorted_texts(existing.get("contexts")) == sorted_texts(record.get("contexts"))
        && equal(
            &listed_or_empty(existing.get("examples")),
            &listed_or_empty(record.get("examples")),
        )
        && same_spans(
            &cited_spans(&listed_or_empty(existing.get("evidence"))),
            &cited_spans(&listed_or_empty(record.get("evidence"))),
        )
}

fn same_draft(existing: &Object, record: &Object) -> Result<bool, String> {
    Ok(equal(
        &as_stored("rules", existing)?,
        &as_stored("rules", record)?,
    ))
}

fn package(run: &mut Harvest, records: &[Object], verdicts: &Object) -> Result<(), String> {
    let package_root = run.package_root().to_path_buf();
    let name = text_of(&run.spec, "name");
    if run.resumed_from.is_none() {
        init_change_package(
            &package_root,
            Some(&name),
            Some(&name),
            Some(&text_of(&run.spec, "proposer")),
        )?;
        run.written = true;
    }
    let promoted: Vec<String> = updated_records(&load_json(&package_root.join(PROPOSAL_FILE))?)
        .into_iter()
        .filter(|(asset, _)| asset == "rules")
        .map(|(_, id)| id)
        .collect();
    let rules = spec_rules(&run.spec);
    for ((rule, summary), record) in rules.iter().zip(&run.rules).zip(records) {
        let id = text_of(rule, "id");
        let state = text_of(summary, "record");
        let superseding = SUPERSEDING.contains(&state.as_str());
        if !promoted.contains(&id) || state == "created" || state == "revised" {
            promote_candidate(&package_root, &run.registry_root, "rules", &id, superseding)?;
        }
        if superseding {
            let mut fields = Object::new();
            for name in HARVESTED_FIELDS {
                fields.insert(name, field(record, name).clone());
            }
            replace_promoted_fields(&package_root, "rules", &id, &fields)?;
        }
    }
    let documents = harvest_package(&package_root, &fill_of(&run.spec, &rules, verdicts))?;
    if changed_documents(&package_root, &documents)? {
        write_harvest_package(&package_root, &documents)?;
    }
    Ok(())
}

fn changed_documents(package_root: &Path, documents: &Documents) -> Result<bool, String> {
    for (name, document) in documents {
        let stored = load_json(&package_root.join(name))?;
        if !equal(&Value::Object(document.clone()), &Value::Object(stored)) {
            return Ok(true);
        }
    }
    Ok(false)
}

fn fill_of(spec: &Object, rules: &[Object], verdicts: &Object) -> Object {
    let mut entries = Object::new();
    for rule in rules {
        let id = text_of(rule, "id");
        let mut entry = Object::new();
        entry.insert("check", field(rule, "check").clone());
        entry.insert("level", setting(spec, rule, "level"));
        entry.insert("counterfactual", field(verdicts, &id).clone());
        entries.insert(id, Value::Object(entry));
    }
    let mut requirement = Object::new();
    requirement.insert("intent", field(spec, "intent").clone());
    requirement.insert(
        "actor",
        spec.get("actor")
            .cloned()
            .unwrap_or_else(|| Value::Str("coding-agent".to_string())),
    );
    requirement.insert("observable_outcome", field(spec, "outcome").clone());
    let mut fill = Object::new();
    fill.insert("rules", Value::Object(entries));
    fill.insert("requirement", Value::Object(requirement));
    for (name, target) in DECISIONS {
        fill.insert(target, field(spec, name).clone());
    }
    fill
}

fn verify(run: &mut Harvest) -> Result<(), String> {
    let package_root = run.package_root().to_path_buf();
    let proposal_path = package_root.join(PROPOSAL_FILE);
    if load_json(&proposal_path)?
        .get("status")
        .and_then(Value::as_str)
        == Some("draft")
    {
        submit_proposal(&package_root, &run.registry_root, &run.repo_root)?;
    }
    run.stopped_at = "submitted";
    let tested = already_tested(&package_root)?;
    for rule in spec_rules(&run.spec) {
        let obligation = obligation_of(&text_of(&rule, "id"));
        if tested.contains(&obligation) {
            continue;
        }
        let profile = str_of(&setting(&run.spec, &rule, "command_profile"));
        let cwd = setting(&run.spec, &rule, "cwd")
            .as_str()
            .map(str::to_string);
        let timeout = match setting(&run.spec, &rule, "timeout") {
            Value::Int(digits) => digits.parse().unwrap_or(i64::MAX),
            _ => 600,
        };
        record_test_result(
            &package_root,
            &run.registry_root,
            &run.repo_root,
            &ObligationRun {
                obligation_id: &obligation,
                command_profile: &profile,
                cwd: cwd.as_deref(),
            },
            timeout,
        )?;
    }
    run.stopped_at = "tested";
    let approval = match run.spec.get("approval") {
        Some(Value::Object(approval)) => approval.clone(),
        _ => {
            run.errors
                .push("no approval in the spec; record-approval, then verify-proposal".to_string());
            return Ok(());
        }
    };
    let (role, reviewer) = (text_of(&approval, "role"), text_of(&approval, "reviewer"));
    if !already_approved(&package_root, &role, &reviewer)? {
        record_approval(
            &package_root,
            &Approval {
                role: &role,
                reviewer: &reviewer,
                scope: &text_of(&run.spec, "scope"),
                approved_at: None,
            },
        )?;
    }
    verify_proposal(&package_root, &run.registry_root, &run.repo_root)?;
    run.stopped_at = "verified";
    Ok(())
}

fn already_tested(package_root: &Path) -> Result<Vec<String>, String> {
    let obligations = load_json(&package_root.join(OBLIGATIONS_FILE))?;
    let mut checks: Vec<(&Value, &Value)> = Vec::new();
    if let Some(Value::Array(entries)) = obligations.get("obligations") {
        for entry in entries.iter().filter_map(Value::as_object) {
            let (id, check) = (field(entry, "id"), field(entry, "check"));
            match checks.iter_mut().find(|(known, _)| equal(known, id)) {
                Some(slot) => slot.1 = check,
                None => checks.push((id, check)),
            }
        }
    }
    let evidence = load_json(&package_root.join(EVIDENCE_FILE))?;
    let mut tested = Vec::new();
    if let Some(Value::Array(results)) = evidence.get("test_results") {
        for result in results.iter().filter_map(Value::as_object) {
            let id = field(result, "obligation_id");
            let Some((_, check)) = checks.iter().find(|(known, _)| equal(known, id)) else {
                continue;
            };
            if result.get("status").and_then(Value::as_str) == Some("passed")
                && same_command(&text_of(result, "evidence"), check)
                && let Value::Str(named) = id
            {
                tested.push(named.clone());
            }
        }
    }
    Ok(tested)
}

fn already_approved(package_root: &Path, role: &str, reviewer: &str) -> Result<bool, String> {
    let proposal = load_json(&package_root.join(PROPOSAL_FILE))?;
    Ok(match proposal.get("approvals") {
        Some(Value::Array(approvals)) => {
            approvals.iter().filter_map(Value::as_object).any(|entry| {
                entry.get("role").and_then(Value::as_str) == Some(role)
                    && entry.get("reviewer").and_then(Value::as_str) == Some(reviewer)
            })
        }
        _ => false,
    })
}

pub fn package_root_of(
    spec: &Object,
    registry_root: &Path,
    changes_root: Option<&Path>,
) -> Result<PathBuf, String> {
    let Some(name) = spec
        .get("name")
        .and_then(Value::as_str)
        .filter(|name| !py::strip(name).is_empty())
    else {
        return Err("the spec names no package: it has no name".to_string());
    };
    let base = changes_root
        .map(Path::to_path_buf)
        .unwrap_or_else(|| registry_root.join("changes"));
    Ok(PathBuf::from(
        PurePath::new(&base.to_string_lossy()).joined(name).native(),
    ))
}

pub fn discard_harvest(
    package_root: &Path,
    registry_root: &Path,
    repo_root: &Path,
    reason: &str,
) -> Result<Object, String> {
    let package_root = py::resolve_path(package_root);
    let registry_root = py::resolve_path(registry_root);
    let repo_root = py::resolve_path(repo_root);
    let package = shown(&package_root, &repo_root);
    if !package_root.join(PROPOSAL_FILE).is_file() {
        return Err(format!("no package at {package}"));
    }
    if py::strip(reason).is_empty() {
        return Err("discarding a package requires the reason it is undone".to_string());
    }
    let proposal = load_json(&package_root.join(PROPOSAL_FILE))?;
    let status = field(&proposal, "status");
    if !status
        .as_str()
        .is_some_and(|text| DISCARDABLE.contains(&text))
    {
        return Err(format!(
            "the package at {package} is {}; only a draft, submitted or verified package is discarded",
            str_of(status)
        ));
    }
    let elsewhere = named_by_other_packages(&package_root)?;
    let mut retracted: Vec<String> = Vec::new();
    let mut kept: Vec<String> = Vec::new();
    let mut drafts: Vec<(String, Vec<String>)> = Vec::new();
    for (asset, record_id) in updated_records(&proposal) {
        let Some(record) = read_record(&registry_root, &asset, &record_id)? else {
            continue;
        };
        if reviewed(&registry_root, &asset, &record)? {
            continue;
        }
        if elsewhere.contains(&(asset.clone(), record_id.clone())) {
            kept.push(format!("{asset}/{record_id}"));
            continue;
        }
        retracted.push(format!("{asset}/{record_id}"));
        match drafts.iter_mut().find(|(seen, _)| *seen == asset) {
            Some((_, ids)) => ids.push(record_id),
            None => drafts.push((asset, vec![record_id])),
        }
    }
    for (asset, record_ids) in &drafts {
        retract_candidate(&registry_root, &repo_root, asset, record_ids, reason)?;
    }
    let strings = |items: Vec<String>| Value::Array(items.into_iter().map(Value::Str).collect());
    let mut event = Object::new();
    event.insert("operation", Value::Str("discard-harvest".to_string()));
    event.insert("package", Value::Str(package));
    event.insert("retracted", strings(retracted));
    event.insert("kept", strings(kept));
    event.insert("reason", Value::Str(py::strip(reason).to_string()));
    with_writer_lock(&registry_root, || {
        audit_append::append(&registry_root, &event)?;
        remove_tree(&package_root)
    })?;
    Ok(event)
}

fn named_by_other_packages(package_root: &Path) -> Result<BTreeSet<(String, String)>, String> {
    let parent = package_root.parent().unwrap_or(package_root);
    let mut named = BTreeSet::new();
    for (other, proposal) in iter_packages(parent)? {
        let Ok(proposal) = proposal else {
            continue;
        };
        if other != package_root && field(&proposal, "status").as_str() != Some("superseded") {
            named.extend(updated_records(&proposal));
        }
    }
    Ok(named)
}
