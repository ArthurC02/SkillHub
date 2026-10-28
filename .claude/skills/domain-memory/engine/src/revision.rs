use std::path::{Path, PathBuf};

use crate::audit_log::CANONICAL;
use crate::git::{self, Failure};
use crate::json::{self, Object, Value};
use crate::policy::{existing_policy, policy_path};
use crate::py;
use crate::pyvalue::str_of;
use crate::sha256::hex_digest;
use crate::store::{ASSETS, Registry};

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

pub fn registry_digest(root: &Path) -> Result<String, String> {
    let registry = Registry::open(root)?;
    let mut by_file_name: Vec<&(&str, &str, &str)> = ASSETS.iter().collect();
    by_file_name.sort_by_key(|(asset, _, _)| format!("{asset}.json"));
    let mut fed = Vec::new();
    feed(
        &mut fed,
        "manifest.json",
        canonical(&manifest_document()).as_bytes(),
    );
    for (asset, key, format) in by_file_name {
        let mut document = Object::new();
        document.insert("format", Value::Str((*format).to_string()));
        document.insert("status", Value::Str("candidate".to_string()));
        document.insert(
            *key,
            Value::Array(in_identifier_order(registry.records(asset)?)),
        );
        feed(
            &mut fed,
            &format!("{asset}.json"),
            canonical(&Value::Object(document)).as_bytes(),
        );
    }
    let policy = existing_policy(root)?;
    let policy_name = policy_path(root)
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_default();
    let policy_bytes = policy.map_or_else(
        || b"absent".to_vec(),
        |found| canonical(&Value::Object(found)).into_bytes(),
    );
    feed(&mut fed, &policy_name, &policy_bytes);
    Ok(format!("sha256:{}", hex_digest(&fed)))
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
