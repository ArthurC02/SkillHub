use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

use crate::audit_log::CANONICAL;
use crate::candidate_evidence::without_derived_keys;
use crate::git::{self, Failure};
use crate::json::{self, Object, Value};
use crate::okf::{review_field, truthy};
use crate::policy::{existing_policy, policy_path};
use crate::py;
use crate::pyvalue::{equal, str_of};
use crate::sha256::hex_digest;
use crate::store::{ASSETS, Registry, committed_documents, registry_dir, shown_path};

const COMMIT_LENGTH: usize = 40;
const MANIFEST_FORMAT: &str = "domain-registry/v1";

fn canonical(value: &Value) -> String {
    json::dumps(value, CANONICAL)
}

fn manifest_document() -> Value {
    let mut manifest = Object::new();
    manifest.insert("format", Value::Str(MANIFEST_FORMAT.to_string()));
    manifest.insert("status", Value::Str("candidate".to_string()));
    let artifacts = ASSETS
        .iter()
        .map(|(asset, _, _)| Value::Str(format!("{asset}.json")))
        .collect();
    manifest.insert("artifacts", Value::Array(artifacts));
    Value::Object(manifest)
}

fn feed(digest: &mut Vec<u8>, name: &str, content: &[u8]) {
    digest.extend_from_slice(name.as_bytes());
    digest.push(0);
    digest.extend_from_slice(content);
    digest.push(0);
}

fn in_identifier_order(mut records: Vec<Value>) -> Vec<Value> {
    records.sort_by_key(|record| {
        record
            .as_object()
            .map(|object| object.get("id").map_or_else(|| "None".to_string(), str_of))
            .unwrap_or_default()
    });
    records
}

struct Snapshot {
    records: Vec<(&'static str, Vec<Value>)>,
    policy: Option<Value>,
}

fn present(policy: Value) -> Option<Value> {
    (policy != Value::Null).then_some(policy)
}

fn current_snapshot(root: &Path) -> Result<Snapshot, String> {
    let registry = Registry::open(root)?;
    let mut records = Vec::new();
    for (asset, _, _) in ASSETS {
        records.push((*asset, registry.records(asset)?));
    }
    let policy = existing_policy(root)?.map(Value::Object);
    Ok(Snapshot { records, policy })
}

fn policy_file_name(root: &Path) -> String {
    policy_path(root)
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_default()
}

fn digest_of(snapshot: &Snapshot, policy_name: &str) -> String {
    let mut by_file_name: Vec<&(&str, &str, &str)> = ASSETS.iter().collect();
    by_file_name.sort_by_key(|(asset, _, _)| format!("{asset}.json"));
    let mut fed = Vec::new();
    feed(
        &mut fed,
        "manifest.json",
        canonical(&manifest_document()).as_bytes(),
    );
    for (asset, key, format) in by_file_name {
        let records = snapshot
            .records
            .iter()
            .find(|(name, _)| name == asset)
            .map(|(_, records)| records.clone())
            .unwrap_or_default();
        let mut document = Object::new();
        document.insert("format", Value::Str((*format).to_string()));
        document.insert("status", Value::Str("candidate".to_string()));
        document.insert(*key, Value::Array(in_identifier_order(records)));
        feed(
            &mut fed,
            &format!("{asset}.json"),
            canonical(&Value::Object(document)).as_bytes(),
        );
    }
    let policy_bytes = snapshot
        .policy
        .as_ref()
        .map_or_else(|| b"absent".to_vec(), |found| canonical(found).into_bytes());
    feed(&mut fed, policy_name, &policy_bytes);
    format!("sha256:{}", hex_digest(&fed))
}

pub fn registry_digest(root: &Path) -> Result<String, String> {
    Ok(digest_of(&current_snapshot(root)?, &policy_file_name(root)))
}

fn is_commit(text: &str) -> bool {
    text.len() == COMMIT_LENGTH
        && text
            .chars()
            .all(|character| matches!(character, '0'..='9' | 'a'..='f'))
}

fn git_commit(repo_root: &Path) -> Result<Option<String>, String> {
    let done = match git::run_git(repo_root, &["rev-parse", "HEAD"], None) {
        Ok(done) => done,
        Err(error) if error.kind == Failure::Spawn => return Ok(None),
        Err(error) => return Err(error.message),
    };
    if done.code != 0 {
        return Ok(None);
    }
    let printed = String::from_utf8_lossy(&done.stdout).replace("\r\n", "\n");
    let commit = py::strip(&printed);
    Ok(is_commit(commit).then(|| commit.to_string()))
}

pub fn inside(top: &Path, path: &Path) -> Option<PathBuf> {
    let top = py::resolve_path(top);
    let located = py::resolve_path(&top.join(path));
    py::relative_to(&located, &top).is_ok().then_some(located)
}

pub fn current_registry_revision(root: &Path, repo_root: &Path) -> Result<Object, String> {
    let commit = git_commit(&py::resolve_path(repo_root))?;
    let resolved_root =
        inside(repo_root, root).ok_or("registry root must be inside repo root".to_string())?;
    let mut revision = Object::new();
    revision.insert("observed_commit", commit.map_or(Value::Null, Value::Str));
    revision.insert(
        "registry_digest",
        Value::Str(registry_digest(&resolved_root)?),
    );
    Ok(revision)
}

pub const DIGEST_PREFIX: &str = "sha256:";
pub const DIGEST_HEX_LENGTH: usize = 64;
const STALE_BASE: &str =
    "proposal base revision is stale; rebase-proposal, then obtain fresh approval";
const CANNOT_COMPARE: &str = "so the records this proposal touches cannot be compared; rebase-proposal restamps the proposal on the current Registry";

pub enum Refusal {
    Stale(String),
    Other(String),
}

impl From<String> for Refusal {
    fn from(message: String) -> Self {
        Refusal::Other(message)
    }
}

impl From<Refusal> for String {
    fn from(refusal: Refusal) -> Self {
        match refusal {
            Refusal::Stale(message) | Refusal::Other(message) => message,
        }
    }
}

fn is_lowercase_hex(text: &str, length: usize) -> bool {
    text.len() == length
        && text
            .chars()
            .all(|character| matches!(character, '0'..='9' | 'a'..='f'))
}

pub fn valid_digest(value: &Value) -> bool {
    value
        .as_str()
        .and_then(|text| text.strip_prefix(DIGEST_PREFIX))
        .is_some_and(|hex| is_lowercase_hex(hex, DIGEST_HEX_LENGTH))
}

pub fn valid_commit(value: &Value) -> bool {
    value
        .as_str()
        .is_some_and(|text| is_lowercase_hex(text, COMMIT_LENGTH))
}

pub fn valid_registry_revision(value: Option<&Value>) -> bool {
    let Some(Value::Object(revision)) = value else {
        return false;
    };
    let commit = revision.get("observed_commit").unwrap_or(&Value::Null);
    (*commit == Value::Null || valid_commit(commit))
        && valid_digest(revision.get("registry_digest").unwrap_or(&Value::Null))
}

fn committed_json(repo_root: &Path, commit: &str, path: &Path) -> Result<Option<Value>, String> {
    let relative = py::relative_to(&py::resolve_path(path), &py::resolve_path(repo_root))?;
    let shown = git::run_git_bytes(repo_root, &["show", &format!("{commit}:{relative}")])?;
    if shown.code != 0 {
        return Ok(None);
    }
    let invalid = || format!("{commit}:{relative} is not valid JSON");
    let text = String::from_utf8(shown.stdout).map_err(|_| invalid())?;
    json::parse_keeping_last_duplicate(&text)
        .map(Some)
        .map_err(|()| invalid())
}

fn committed_snapshot(
    root: &Path,
    repo_root: &Path,
    commit: &str,
) -> Result<Option<Snapshot>, String> {
    let Some(documents) = committed_documents(repo_root, commit, root)? else {
        return Ok(None);
    };
    let policy = committed_json(repo_root, commit, &policy_path(root))?.and_then(present);
    let mut records = Vec::new();
    for (asset, key, _) in ASSETS {
        let held = documents
            .get(asset)
            .and_then(Value::as_object)
            .and_then(|document| document.get(key))
            .and_then(Value::as_array)
            .map(<[Value]>::to_vec)
            .unwrap_or_default();
        records.push((*asset, held));
    }
    Ok(Some(Snapshot { records, policy }))
}

pub fn base_registry_revision(root: &Path, repo_root: &Path) -> Result<Object, String> {
    let mut revision = current_registry_revision(root, repo_root)?;
    let commit = match revision.get("observed_commit") {
        Some(Value::Str(commit)) => commit.clone(),
        _ => return Ok(revision),
    };
    let Some(committed) = committed_snapshot(root, repo_root, &commit)? else {
        return Ok(revision);
    };
    revision.insert(
        "committed_digest",
        Value::Str(digest_of(&committed, &policy_file_name(root))),
    );
    Ok(revision)
}

fn strings_in(value: &Value, found: &mut BTreeSet<String>) {
    match value {
        Value::Str(text) => {
            found.insert(text.clone());
        }
        Value::Object(object) => object.iter().for_each(|(_, item)| strings_in(item, found)),
        Value::Array(items) => items.iter().for_each(|item| strings_in(item, found)),
        _ => {}
    }
}

fn updates_of(proposal: &Object) -> Vec<&Object> {
    match proposal.get("registry_updates") {
        Some(Value::Array(items)) => items.iter().filter_map(Value::as_object).collect(),
        _ => Vec::new(),
    }
}

fn touched_records(proposal: &Object, snapshot: &Snapshot) -> BTreeSet<(String, String)> {
    let mut touched = BTreeSet::new();
    let mut mentioned = BTreeSet::new();
    for update in updates_of(proposal) {
        let record = update.get("record").unwrap_or(&Value::Null);
        let named = update.get("id").filter(|id| truthy(id));
        let held = record
            .as_object()
            .and_then(|record| record.get("id"))
            .unwrap_or(&Value::Null);
        let record_id = named.unwrap_or(held);
        let asset = update.get("asset").unwrap_or(&Value::Null);
        touched.insert((format!("{}.json", str_of(asset)), str_of(record_id)));
        strings_in(record, &mut mentioned);
    }
    for (asset, held) in &snapshot.records {
        for record in held.iter().filter_map(Value::as_object) {
            if let Some(id) = record.get("id").and_then(Value::as_str)
                && mentioned.contains(id)
            {
                touched.insert((format!("{asset}.json"), id.to_string()));
            }
        }
    }
    touched
}

fn written_records(proposal: &Object) -> Vec<((String, String), &Object)> {
    updates_of(proposal)
        .into_iter()
        .filter(|update| update.get("operation").and_then(Value::as_str) == Some("upsert"))
        .filter_map(|update| {
            let record = update.get("record")?.as_object()?;
            let key = (
                format!(
                    "{}.json",
                    str_of(update.get("asset").unwrap_or(&Value::Null))
                ),
                str_of(record.get("id").unwrap_or(&Value::Null)),
            );
            Some((key, record))
        })
        .collect()
}

fn record_in<'a>(snapshot: &'a Snapshot, asset: &str, record_id: &str) -> Option<&'a Object> {
    snapshot
        .records
        .iter()
        .find(|(name, _)| *name == asset)?
        .1
        .iter()
        .filter_map(Value::as_object)
        .find(|record| record.get("id").and_then(Value::as_str) == Some(record_id))
}

pub fn as_stored(asset: &str, record: &Object) -> Result<Value, String> {
    let mut kept = Object::new();
    for (key, value) in record.iter() {
        if key != review_field(asset) && key != "verified" {
            kept.insert(key.clone(), value.clone());
        }
    }
    without_derived_keys(&Value::Object(kept))
}

fn changed_record(
    (name, record_id): &(String, String),
    (then, now): (&Snapshot, &Snapshot),
    written: &[((String, String), &Object)],
) -> Result<Option<String>, String> {
    let asset = name.strip_suffix(".json").unwrap_or(name);
    let shown = format!("{asset}/{record_id}");
    if !ASSETS.iter().any(|(known, _, _)| *known == asset) {
        return Ok(Some(format!(
            "{shown}, which this proposal touches, changed"
        )));
    }
    let (before, after) = (
        record_in(then, asset, record_id),
        record_in(now, asset, record_id),
    );
    let same = match (before, after) {
        (None, None) => true,
        (Some(one), Some(other)) => {
            equal(&Value::Object(one.clone()), &Value::Object(other.clone()))
        }
        _ => false,
    };
    if same {
        return Ok(None);
    }
    let Some((_, proposed)) = written
        .iter()
        .find(|(key, _)| key.0 == *name && key.1 == *record_id)
    else {
        return Ok(Some(format!(
            "{shown}, which this proposal touches, changed"
        )));
    };
    if let Some(now) = after
        && equal(&as_stored(asset, now)?, &as_stored(asset, proposed)?)
    {
        return Ok(None);
    }
    Ok(Some(format!(
        "{shown}, which this proposal writes, changed since it was reviewed"
    )))
}

fn base_snapshot(
    root: &Path,
    repo_root: &Path,
    base: &Object,
) -> Result<Result<Snapshot, String>, String> {
    let Some(commit) = base.get("observed_commit").and_then(Value::as_str) else {
        return Ok(Err(format!(
            "the base revision names no commit, {CANNOT_COMPARE}"
        )));
    };
    let short: String = commit.chars().take(12).collect();
    let Some(rebuilt) = committed_snapshot(root, repo_root, commit)? else {
        return Ok(Err(format!(
            "commit {short} does not hold the Registry at {}, {CANNOT_COMPARE}",
            shown_path(&registry_dir(root), Some(repo_root))
        )));
    };
    let recorded = base
        .get("committed_digest")
        .or_else(|| base.get("registry_digest"))
        .unwrap_or(&Value::Null);
    let rebuilt_digest = Value::Str(digest_of(&rebuilt, &policy_file_name(root)));
    if !equal(&rebuilt_digest, recorded) {
        return Ok(Err(format!(
            "the Registry committed at {short} does not reproduce the recorded base digest, {CANNOT_COMPARE}"
        )));
    }
    Ok(Ok(rebuilt))
}

fn changed_since_base(
    proposal: &Object,
    root: &Path,
    repo_root: &Path,
    base: &Object,
) -> Result<Option<String>, String> {
    let then = match base_snapshot(root, repo_root, base)? {
        Ok(rebuilt) => rebuilt,
        Err(reason) => return Ok(Some(reason)),
    };
    let now = current_snapshot(root)?;
    let same_policy = match (&then.policy, &now.policy) {
        (None, None) => true,
        (Some(one), Some(other)) => equal(one, other),
        _ => false,
    };
    if !same_policy {
        return Ok(Some("the Domain Memory policy changed".to_string()));
    }
    let written = written_records(proposal);
    let touched: BTreeSet<(String, String)> = touched_records(proposal, &then)
        .union(&touched_records(proposal, &now))
        .cloned()
        .collect();
    for entry in &touched {
        if let Some(changed) = changed_record(entry, (&then, &now), &written)? {
            return Ok(Some(changed));
        }
    }
    Ok(None)
}

pub fn require_current_registry_revision(
    proposal: &Object,
    root: &Path,
    repo_root: &Path,
) -> Result<String, Refusal> {
    let expected = proposal.get("base_registry_revision");
    let Some(Value::Object(base)) = expected.filter(|_| valid_registry_revision(expected)) else {
        return Err(Refusal::Other(
            "proposal requires a Git commit and Registry digest base revision".to_string(),
        ));
    };
    let resolved_root =
        inside(repo_root, root).ok_or("registry root must be inside repo root".to_string())?;
    let actual = registry_digest(&resolved_root)?;
    let recorded = base.get("registry_digest").unwrap_or(&Value::Null);
    if recorded.as_str() == Some(actual.as_str()) {
        return Ok(actual);
    }
    match changed_since_base(proposal, root, repo_root, base)? {
        Some(changed) => Err(Refusal::Stale(format!("{STALE_BASE}: {changed}"))),
        None => Ok(actual),
    }
}
