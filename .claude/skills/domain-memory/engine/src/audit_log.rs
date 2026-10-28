use std::cmp::Ordering;
use std::path::{Path, PathBuf};

use crate::audit_append::{LOG_LINE, append, write_manifest};
use crate::files::{MAX_JSON_BYTES, load_json};
use crate::git;
use crate::json::{self, Object, Style, Value};
use crate::okf::truthy;
use crate::proposals::readable_packages;
use crate::py;
use crate::pyvalue::{equal, splitlines, str_of};
use crate::sha256::hex_digest;
use crate::store::{has_registry, missing_registry, posix};
use crate::transaction::with_writer_lock;

pub const MANIFEST_FORMAT: &str = "domain-memory-audit/v1";
pub const APPLY_OPERATION: &str = "apply-approved-updates";
const CONFLICT_PREFIXES: [&str; 3] = ["<<<<<<< ", ">>>>>>> ", "||||||| "];

pub const CANONICAL: Style = Style {
    indent: None,
    sort_keys: true,
    ensure_ascii: false,
    compact: true,
};

enum Halt {
    Invalid(String),
    Crash(String),
}

impl From<String> for Halt {
    fn from(reason: String) -> Self {
        Halt::Invalid(reason)
    }
}

struct Seen {
    digest: String,
    position: usize,
    sequence: Value,
}

pub fn events_path(root: &Path) -> PathBuf {
    root.join("audit").join("events.jsonl")
}

pub fn audit_manifest_path(root: &Path) -> PathBuf {
    root.join("audit").join("manifest.json")
}

pub fn event_digest(event: &Object) -> String {
    let canonical = json::dumps(&Value::Object(event.clone()), CANONICAL);
    format!("sha256:{}", hex_digest(canonical.as_bytes()))
}

pub fn read_events(root: &Path) -> Result<Vec<Object>, String> {
    Ok(parsed_events(&event_lines(root)?, root)?
        .into_iter()
        .map(|(_, event)| event)
        .collect())
}

pub fn event_lines(root: &Path) -> Result<Vec<String>, String> {
    let path = events_path(root);
    if !path.is_file() {
        return Ok(Vec::new());
    }
    let size = path.metadata().map(|meta| meta.len()).unwrap_or(0);
    if size > MAX_JSON_BYTES {
        return Err(format!(
            "audit log exceeds {MAX_JSON_BYTES} bytes: {}",
            py::shown(&path)
        ));
    }
    let bytes = std::fs::read(&path).map_err(|error| py::os_error(&error, &path))?;
    let text = py::utf8_text(&bytes).map_err(|error| format!("audit/events.jsonl: {error}"))?;
    Ok(splitlines(&text).into_iter().map(str::to_string).collect())
}

pub fn conflict_marker(line: &str) -> bool {
    CONFLICT_PREFIXES
        .iter()
        .any(|prefix| line.starts_with(prefix))
        || line == "======="
}

pub fn parsed_events(lines: &[String], root: &Path) -> Result<Vec<(usize, Object)>, String> {
    let mut events = Vec::new();
    for (offset, line) in lines.iter().enumerate() {
        let index = offset + 1;
        if conflict_marker(line) {
            return Err(format!(
                "conflict markers at event {index}; run reconcile-audit --registry-root {}",
                posix(&py::resolve_path(root))
            ));
        }
        match json::parse(line) {
            Ok(Value::Object(event)) => events.push((index, event)),
            Ok(_) => return Err(format!("audit event {index} must be an object")),
            Err(()) => return Err(format!("audit event {index} is invalid JSON")),
        }
    }
    Ok(events)
}

pub fn parents_of(event: &Object) -> Vec<Value> {
    match event.get("previous_event_sha256") {
        None | Some(Value::Null) => Vec::new(),
        Some(Value::Array(items)) => items.clone(),
        Some(other) => vec![other.clone()],
    }
}

fn reject_unhashable(value: &Value) -> Result<(), Halt> {
    let kind = match value {
        Value::Array(_) => "list",
        Value::Object(_) => "dict",
        _ => return Ok(()),
    };
    Err(Halt::Crash(format!("unhashable type: '{kind}'")))
}

fn seen_by<'a>(seen: &'a [Seen], key: &Value) -> Option<&'a Seen> {
    let Value::Str(text) = key else {
        return None;
    };
    seen.iter().find(|entry| entry.digest == *text)
}

fn number_of(value: &Value) -> f64 {
    match value {
        Value::Bool(flag) => f64::from(u8::from(*flag)),
        Value::Int(digits) => digits.parse().unwrap_or(f64::INFINITY),
        Value::Float(number) => *number,
        _ => 0.0,
    }
}

fn highest(sequences: impl Iterator<Item = Value>) -> Value {
    let mut best: Option<Value> = None;
    for candidate in sequences {
        let replaces = best
            .as_ref()
            .is_none_or(|kept| number_of(&candidate) > number_of(kept));
        if replaces {
            best = Some(candidate);
        }
    }
    best.unwrap_or_else(|| Value::int(0))
}

pub fn successor(value: &Value) -> Value {
    match value {
        Value::Float(number) => Value::Float(number + 1.0),
        Value::Bool(flag) => Value::Int((1 + i128::from(*flag)).to_string()),
        Value::Int(digits) => match digits.parse::<i128>() {
            Ok(whole) => Value::Int((whole + 1).to_string()),
            Err(_) => Value::Float(number_of(value) + 1.0),
        },
        other => other.clone(),
    }
}

fn dag_heads(events: &[Object]) -> Result<(Vec<String>, Value), Halt> {
    let mut seen: Vec<Seen> = Vec::new();
    for (offset, event) in events.iter().enumerate() {
        let position = offset + 1;
        let mut unsigned = Object::new();
        for (key, value) in event.iter().filter(|(key, _)| key != "event_sha256") {
            unsigned.insert(key.clone(), value.clone());
        }
        let digest = event.get("event_sha256").cloned().unwrap_or(Value::Null);
        reject_unhashable(&digest)?;
        if let Some(prior) = seen_by(&seen, &digest) {
            return Err(Halt::Invalid(format!(
                "audit event {position} repeats event {}",
                prior.position
            )));
        }
        let listed = parents_of(event);
        let mut parent_sequences = Vec::new();
        for parent in &listed {
            reject_unhashable(parent)?;
            if let Some(found) = seen_by(&seen, parent) {
                parent_sequences.push(found.sequence.clone());
            }
        }
        if parent_sequences.len() != listed.len() {
            return Err(Halt::Invalid(format!(
                "audit chain is broken at event {position}"
            )));
        }
        let expected = successor(&highest(parent_sequences.into_iter()));
        let sequence = event.get("sequence").cloned().unwrap_or(Value::Null);
        if !equal(&sequence, &expected) {
            return Err(Halt::Invalid(format!(
                "audit sequence is invalid at event {position}"
            )));
        }
        let computed = event_digest(&unsigned);
        let Value::Str(stored) = digest else {
            return Err(Halt::Invalid(format!(
                "audit digest is invalid at event {position}"
            )));
        };
        if stored != computed {
            return Err(Halt::Invalid(format!(
                "audit digest is invalid at event {position}"
            )));
        }
        seen.push(Seen {
            digest: stored,
            position,
            sequence,
        });
    }
    let referenced: Vec<String> = events
        .iter()
        .flat_map(parents_of)
        .filter_map(|parent| parent.as_str().map(str::to_string))
        .collect();
    let mut heads: Vec<String> = seen
        .iter()
        .filter(|entry| !referenced.contains(&entry.digest))
        .map(|entry| entry.digest.clone())
        .collect();
    heads.sort();
    let height = highest(seen.iter().map(|entry| entry.sequence.clone()));
    Ok((heads, height))
}

fn single_head(heads: &[String]) -> Value {
    match heads {
        [only] => Value::Str(only.clone()),
        _ => Value::Null,
    }
}

fn head_strings(heads: &[String]) -> Value {
    Value::Array(heads.iter().cloned().map(Value::Str).collect())
}

fn manifest_error(root: &Path, events: usize, heads: &[String]) -> Result<Option<String>, String> {
    let path = audit_manifest_path(root);
    if !path.is_file() {
        return Ok(None);
    }
    let manifest = load_json(&path)?;
    let field = |key: &str| manifest.get(key).cloned().unwrap_or(Value::Null);
    let matches_log = field("format") == Value::Str(MANIFEST_FORMAT.to_string())
        && equal(&field("events"), &Value::int(events))
        && equal(&field("head_sha256"), &single_head(heads))
        && manifest
            .get("heads")
            .is_none_or(|listed| equal(listed, &head_strings(heads)));
    Ok((!matches_log).then(|| "audit manifest does not match the event log".to_string()))
}

fn invalid(reason: String) -> Object {
    let mut result = Object::new();
    result.insert("status", Value::Str("invalid".to_string()));
    result.insert("reason", Value::Str(reason));
    result
}

pub fn is_valid(result: &Object) -> bool {
    result.get("status").and_then(Value::as_str) == Some("valid")
}

pub fn verify(root: &Path) -> Result<Object, String> {
    if !has_registry(root) {
        return Ok(invalid(missing_registry(root)?));
    }
    let events = match read_events(root) {
        Ok(events) => events,
        Err(reason) => return Ok(invalid(reason)),
    };
    let (heads, height) = match dag_heads(&events) {
        Ok(found) => found,
        Err(Halt::Invalid(reason)) => return Ok(invalid(reason)),
        Err(Halt::Crash(reason)) => return Err(reason),
    };
    match manifest_error(root, events.len(), &heads) {
        Ok(None) => {}
        Ok(Some(reason)) | Err(reason) => return Ok(invalid(reason)),
    }
    let mut result = Object::new();
    result.insert("status", Value::Str("valid".to_string()));
    result.insert("events", Value::int(events.len()));
    result.insert("height", height);
    result.insert("head_sha256", single_head(&heads));
    result.insert("heads", head_strings(&heads));
    Ok(result)
}

pub fn anchor_error(
    root: &Path,
    changes_root: Option<&Path>,
    against: Option<&str>,
) -> Result<Option<String>, String> {
    if let Some(changes_root) = changes_root {
        let missing = applied_without_event(root, changes_root)?;
        if !missing.is_empty() {
            return Ok(Some(format!(
                "applied proposals have no {APPLY_OPERATION} event in the audit log: {}",
                missing.join(", ")
            )));
        }
    }
    match against {
        Some(against) => committed_prefix_error(root, against),
        None => Ok(None),
    }
}

fn hashable(value: &Value) -> Result<(), String> {
    reject_unhashable(value).map_err(|halt| match halt {
        Halt::Invalid(reason) | Halt::Crash(reason) => reason,
    })
}

fn applied_without_event(root: &Path, changes_root: &Path) -> Result<Vec<String>, String> {
    let mut applied = Vec::new();
    for event in read_events(root)? {
        if event.get("operation").and_then(Value::as_str) == Some(APPLY_OPERATION) {
            let id = event.get("proposal_id").cloned().unwrap_or(Value::Null);
            hashable(&id)?;
            applied.push(id);
        }
    }
    let mut missing = Vec::new();
    for (package, proposal) in readable_packages(changes_root)? {
        let was_applied = ["status", "superseded_from_status"]
            .iter()
            .any(|key| proposal.get(key).and_then(Value::as_str) == Some("applied"));
        let id = proposal.get("proposal_id").cloned().unwrap_or(Value::Null);
        if was_applied {
            hashable(&id)?;
            if !applied.iter().any(|known| equal(known, &id)) {
                let name = package
                    .file_name()
                    .map(|name| name.to_string_lossy().into_owned())
                    .unwrap_or_default();
                missing.push(format!("{name} ({})", str_of(&id)));
            }
        }
    }
    Ok(missing)
}

pub fn type_name(value: &Value) -> &'static str {
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

fn committed_prefix_error(root: &Path, against: &str) -> Result<Option<String>, String> {
    let top = git::toplevel(root)?;
    let unresolved = || format!("git cannot resolve {against} from {}", py::shown(root));
    let Some(top) = top else {
        return Err(unresolved());
    };
    if git::resolve_commit(root, against)?.is_none() {
        return Err(unresolved());
    }
    let manifest_path = py::relative_to(&py::resolve_path(&audit_manifest_path(root)), &top)?;
    let committed = git::run_git_text(&top, &["show", &format!("{against}:{manifest_path}")])?;
    if committed.code != 0 {
        return Ok(Some(format!(
            "no audit manifest committed at {against}:{manifest_path}, so nothing anchors the log"
        )));
    }
    let Ok(parsed) = json::parse_keeping_last_duplicate(&committed.stdout) else {
        return Ok(Some(format!(
            "the audit manifest committed at {against} is not valid JSON"
        )));
    };
    let Value::Object(manifest) = parsed else {
        return Err(format!(
            "'{}' object has no attribute 'get'",
            type_name(&parsed)
        ));
    };
    let count = manifest.get("events").cloned().unwrap_or(Value::Null);
    let head = manifest.get("head_sha256").cloned().unwrap_or(Value::Null);
    let heads = match manifest.get("heads") {
        Some(Value::Array(listed)) => listed.clone(),
        _ if truthy(&head) => vec![head.clone()],
        _ => Vec::new(),
    };
    let mut present = Vec::new();
    for event in read_events(root)? {
        let digest = event.get("event_sha256").cloned().unwrap_or(Value::Null);
        hashable(&digest)?;
        present.push(digest);
    }
    if !extends_log(&count, &heads, &present)? {
        return Ok(Some(format!(
            "the audit log does not extend the one committed at {against} ({} events, head {}); \
             events were removed or rewritten since",
            str_of(&count),
            str_of(&head)
        )));
    }
    Ok(None)
}

fn extends_log(count: &Value, heads: &[Value], present: &[Value]) -> Result<bool, String> {
    let Some(claimed) = whole_number(count) else {
        return Ok(false);
    };
    let distinct = present
        .iter()
        .enumerate()
        .filter(|(at, digest)| !present[..*at].iter().any(|earlier| equal(earlier, digest)))
        .count();
    if claimed > distinct as f64 {
        return Ok(false);
    }
    for digest in heads {
        hashable(digest)?;
        if !present.iter().any(|known| equal(known, digest)) {
            return Ok(false);
        }
    }
    Ok(true)
}

fn whole_number(value: &Value) -> Option<f64> {
    match value {
        Value::Int(_) => Some(number_of(value)),
        _ => None,
    }
}

fn compare_whole(left: &str, right: &str) -> Ordering {
    let magnitude = |text: &str| -> (bool, String) {
        let digits = text.trim_start_matches('-').trim_start_matches('0');
        (
            text.starts_with('-') && !digits.is_empty(),
            digits.to_string(),
        )
    };
    let (left_negative, left_digits) = magnitude(left);
    let (right_negative, right_digits) = magnitude(right);
    let by_size = left_digits
        .len()
        .cmp(&right_digits.len())
        .then_with(|| left_digits.cmp(&right_digits));
    match (left_negative, right_negative) {
        (false, false) => by_size,
        (true, true) => by_size.reverse(),
        (true, false) => Ordering::Less,
        (false, true) => Ordering::Greater,
    }
}

fn sequence_key(event: &Object) -> &str {
    match event.get("sequence") {
        Some(Value::Int(digits)) => digits,
        _ => "0",
    }
}

fn reconciliation_summary(
    events: usize,
    markers: usize,
    duplicates: usize,
    joined: &[String],
    written: bool,
) -> Object {
    let mut summary = Object::new();
    summary.insert("events", Value::int(events));
    summary.insert("conflict_markers_removed", Value::int(markers));
    summary.insert("duplicates_dropped", Value::int(duplicates));
    summary.insert("joined", head_strings(joined));
    summary.insert("written", Value::Bool(written));
    summary
}

fn without_repeats(parsed: Vec<(usize, Object)>) -> Vec<Object> {
    let mut seen = std::collections::HashSet::new();
    let mut unique = Vec::new();
    for (_, event) in parsed {
        let key = event
            .get("event_sha256")
            .map_or_else(|| "None".to_string(), str_of);
        if seen.insert(key) {
            unique.push(event);
        }
    }
    unique
}

fn reconcile_locked(root: &Path) -> Result<Object, String> {
    let lines = event_lines(root)?;
    if lines.is_empty() {
        return Ok(reconciliation_summary(0, 0, 0, &[], false));
    }
    let kept: Vec<String> = lines
        .iter()
        .filter(|line| !conflict_marker(line))
        .cloned()
        .collect();
    let mut ordered = without_repeats(parsed_events(&kept, root)?);
    ordered.sort_by(|left, right| compare_whole(sequence_key(left), sequence_key(right)));
    let heads = match dag_heads(&ordered) {
        Ok((heads, _)) => heads,
        Err(Halt::Invalid(reason) | Halt::Crash(reason)) => return Err(reason),
    };
    let text: String = ordered
        .iter()
        .map(|event| format!("{}\n", json::dumps(&Value::Object(event.clone()), LOG_LINE)))
        .collect();
    let previous: String = lines.iter().map(|line| format!("{line}\n")).collect();
    let rewritten = text != previous;
    if rewritten {
        let path = events_path(root);
        std::fs::write(&path, &text).map_err(|error| py::open_error(&error, &path))?;
    }
    let manifest = audit_manifest_path(root);
    let before = manifest
        .is_file()
        .then(|| std::fs::read(&manifest))
        .transpose()
        .map_err(|error| py::open_error(&error, &manifest))?;
    write_manifest(root, Value::int(ordered.len()), &heads)?;
    let after = std::fs::read(&manifest).map_err(|error| py::open_error(&error, &manifest))?;
    let manifest_rewritten = before.as_deref() != Some(after.as_slice());
    let joined = if heads.len() > 1 { heads } else { Vec::new() };
    if !joined.is_empty() {
        let mut event = Object::new();
        event.insert("operation", Value::Str("reconcile-audit".to_string()));
        event.insert("joined", head_strings(&joined));
        append(root, &event)?;
    }
    Ok(reconciliation_summary(
        ordered.len() + usize::from(!joined.is_empty()),
        lines.len() - kept.len(),
        kept.len() - ordered.len(),
        &joined,
        rewritten || manifest_rewritten || !joined.is_empty(),
    ))
}

pub fn reconcile(root: &Path) -> Result<Object, String> {
    if !has_registry(root) {
        return Err(missing_registry(root)?);
    }
    with_writer_lock(root, || reconcile_locked(root))
}
