use crate::pyvalue::message_text;
use std::path::PathBuf;

use crate::audit_commands::print_inline;
use crate::audit_log;
use crate::cli::Invocation;
use crate::files::load_json;
use crate::git_governance;
use crate::json::{Object, Value};
use crate::policy;
use crate::py;
use crate::signing::{self, SigningKey};
use crate::{emit_stderr, emit_stdout, print_json};

fn resolved(invocation: &Invocation, dest: &str) -> PathBuf {
    py::resolve(invocation.text(dest).unwrap_or("."))
}

fn policy_assignments(invocation: &Invocation) -> Result<Vec<(String, String)>, String> {
    let given = invocation.texts("set");
    if given.is_empty() || given.iter().any(|entry| !entry.contains('=')) {
        return Err("each --set takes FIELD=VALUE; give at least one".to_string());
    }
    Ok(given
        .iter()
        .filter_map(|entry| entry.split_once('='))
        .map(|(field, value)| (field.to_string(), value.to_string()))
        .collect())
}

pub fn amend_policy(invocation: &Invocation) -> Result<i32, String> {
    let root = resolved(invocation, "registry_root");
    let assignments = policy_assignments(invocation)?;
    let changes = policy::amend_policy_fields(
        &root,
        &assignments,
        invocation.text("reason").unwrap_or_default(),
        invocation.text("verifier"),
    )?;
    for change in &changes {
        let rendered = |key: &str| change.get(key).map(message_text).unwrap_or_default();
        emit_stdout(&format!(
            "{}: {} -> {}\n",
            rendered("field"),
            rendered("from"),
            rendered("to")
        ));
    }
    emit_stdout(
        "The Registry revision moved once, so every captured base revision is now stale and needs fresh approval.\n",
    );
    Ok(0)
}

pub fn init_signing_key(invocation: &Invocation) -> Result<i32, String> {
    let repo_root = resolved(invocation, "repo_root");
    let result = signing::init_signing_key(&SigningKey {
        repo_root: &repo_root,
        principal: invocation.text("principal").unwrap_or_default(),
        key_file: invocation.text("key_file"),
        force: invocation.flag("force"),
        sign_every_commit: invocation.flag("sign_every_commit"),
    })?;
    print_json(&Value::Object(result));
    Ok(0)
}

fn answer(status: &str, rest: &[(&str, Value)]) -> Object {
    let mut verdict = Object::new();
    verdict.insert("status", Value::Str(status.to_string()));
    for (key, value) in rest {
        verdict.insert(*key, value.clone());
    }
    verdict
}

fn extension_of(path: &std::path::Path) -> String {
    let name = path
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_default();
    match name.rfind('.') {
        Some(dot) if dot > 0 && dot + 1 < name.len() => name[dot..].to_lowercase(),
        _ => String::new(),
    }
}

fn schema_verdict(path: &std::path::Path) -> Object {
    let text = |text: &str| Value::Str(text.to_string());
    let shown = text(&py::shown(path));
    if !path.is_file() {
        return answer("missing", &[("path", shown)]);
    }
    if extension_of(path) != ".json" {
        return answer(
            "unverified",
            &[
                ("path", shown),
                ("reason", text("only JSON schemas are parsed locally")),
            ],
        );
    }
    let Ok(value) = load_json(path) else {
        return answer(
            "invalid",
            &[("path", shown), ("reason", text("invalid JSON"))],
        );
    };
    if value.get("openapi").is_some() {
        return answer("valid", &[("kind", text("openapi")), ("path", shown)]);
    }
    if ["$schema", "type", "properties"]
        .iter()
        .any(|key| value.get(key).is_some())
    {
        return answer("valid", &[("kind", text("json-schema")), ("path", shown)]);
    }
    answer(
        "invalid",
        &[
            ("path", shown),
            ("reason", text("unrecognized JSON schema")),
        ],
    )
}

pub fn validate_contract(invocation: &Invocation) -> Result<i32, String> {
    let verdict = schema_verdict(&resolved(invocation, "schema"));
    let valid = verdict.get("status").and_then(Value::as_str) == Some("valid");
    print_inline(&Value::Object(verdict));
    Ok(if valid { 0 } else { 1 })
}

pub fn reconcile_audit(invocation: &Invocation) -> Result<i32, String> {
    let result = audit_log::reconcile(&resolved(invocation, "registry_root"))?;
    let joined = result
        .get("joined")
        .and_then(Value::as_array)
        .map_or(0, <[Value]>::len);
    print_inline(&Value::Object(result));
    if joined > 0 {
        emit_stderr(&format!(
            "NOTE: {joined} audit heads joined; commit audit/events.jsonl and audit/manifest.json\n"
        ));
    }
    Ok(0)
}

pub fn install_git_hitl_hook(invocation: &Invocation) -> Result<i32, String> {
    let hook = git_governance::install_pre_push_hook(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
    )?;
    emit_stdout(&format!(
        "Installed Git HITL pre-push hook: {}\n",
        py::shown(&hook)
    ));
    Ok(0)
}

pub fn install_pre_commit_hook(invocation: &Invocation) -> Result<i32, String> {
    let hook = git_governance::install_pre_commit_hook(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
    )?;
    emit_stdout(&format!(
        "Installed Domain Memory pre-commit hook: {}\n",
        py::shown(&hook)
    ));
    Ok(0)
}
