use std::path::PathBuf;

use crate::cli::Invocation;
use crate::emit_stdout;
use crate::files::{load_json, write_source_file};
use crate::json::{Object, Value};
use crate::policy::{self, NewPolicy, existing_policy, policy_path};
use crate::py::{self, Raised};
use crate::registry_write::init_registry;
use crate::revision::inside;
use crate::sources::{self, Profile, selected_source_map, under};
use crate::transaction::remove_tree;

const PROFILE_FLAGS: [(&str, &str); 4] = [
    ("implementation_root", "implementation_roots"),
    ("excluded_directory", "excluded_directories"),
    ("decision_root", "decision_roots"),
    ("boundary_marker", "boundary_markers"),
];

fn resolved(invocation: &Invocation, dest: &str) -> PathBuf {
    py::resolve(invocation.text(dest).unwrap_or("."))
}

fn sources_under(repo_root: &std::path::Path, given: &[String]) -> Vec<PathBuf> {
    given
        .iter()
        .map(|source| under(repo_root, source))
        .collect()
}

fn strings(items: &[String]) -> Value {
    Value::Array(items.iter().map(|item| Value::Str(item.clone())).collect())
}

fn given_profile(invocation: &Invocation) -> Result<Value, String> {
    let mut given = Object::new();
    for (flag, key) in PROFILE_FLAGS {
        let values = invocation.texts(flag);
        if !values.is_empty() {
            given.insert(key, strings(values));
        }
    }
    let given = Value::Object(given);
    let errors = policy::profile_errors(Some(&given));
    if errors.is_empty() {
        Ok(given)
    } else {
        Err(format!("invalid project profile: {}", errors.join("; ")))
    }
}

fn governance_of(invocation: &Invocation) -> Object {
    let named = |dest: &str| {
        Value::Str(
            invocation
                .text(dest)
                .filter(|given| !given.is_empty())
                .unwrap_or("none")
                .to_string(),
        )
    };
    let mut governance = Object::new();
    governance.insert("verifier", named("review_verifier"));
    governance.insert("trigger", named("review_trigger"));
    governance.insert(
        "ci_requirement",
        Value::Str(
            invocation
                .text("ci_requirement")
                .unwrap_or("none")
                .to_string(),
        ),
    );
    governance.insert(
        "authorized_signers",
        strings(invocation.texts("authorized_signer")),
    );
    governance.insert(
        "distinct_signer",
        Value::Bool(invocation.flag("distinct_signer")),
    );
    governance
}

fn refused_governance(invocation: &Invocation) -> Option<&'static str> {
    let verifier = invocation.text("review_verifier").unwrap_or_default();
    if invocation.text("review_mode") == Some("scm-verified") && verifier.is_empty() {
        return Some("scm-verified Domain Memory requires --review-verifier");
    }
    if verifier != "git-signed-commit" {
        return None;
    }
    if invocation.texts("authorized_signer").is_empty() {
        return Some("git-signed-commit requires at least one --authorized-signer");
    }
    (invocation.text("review_trigger") != Some("git-push"))
        .then_some("git-signed-commit requires --review-trigger git-push")
}

fn list_or(items: &[String], default: &[&str]) -> Vec<String> {
    if items.is_empty() {
        default.iter().map(|item| (*item).to_string()).collect()
    } else {
        items.to_vec()
    }
}

pub fn init_domain_memory(invocation: &Invocation) -> Result<i32, String> {
    let repo_root = resolved(invocation, "repo_root");
    let output = resolved(invocation, "output");
    if let Some(refusal) = refused_governance(invocation) {
        return Err(refusal.to_string());
    }
    if inside(&repo_root, &output).is_none() {
        return Err("output must be inside the repository".to_string());
    }
    let selected = sources_under(&repo_root, invocation.texts("source"));
    let profile = given_profile(invocation)?;
    let section = Profile::from_section(Some(&profile));
    let preliminary = selected_source_map(&repo_root, &selected, None, Some(&section))?;
    init_registry(&output)?;
    let authority = invocation.text("source_authority").unwrap_or_default();
    let chosen = preliminary
        .get("selected_paths")
        .cloned()
        .unwrap_or(Value::Null);
    let attempt = (|| -> Result<Object, Raised> {
        policy::write_policy(
            &output,
            &chosen,
            NewPolicy {
                storage_mode: invocation.text("storage_mode").unwrap_or_default(),
                data_classification: invocation.text("data_classification").unwrap_or_default(),
                review_mode: invocation.text("review_mode").unwrap_or_default(),
                authority,
                include: &list_or(invocation.texts("include"), &["**"]),
                exclude: &list_or(invocation.texts("exclude"), &[]),
                governance: governance_of(invocation),
                profile: section.to_value(),
            },
        )?;
        let written = load_json(&policy_path(&output))?;
        selected_source_map(&repo_root, &selected, Some(&written), None)
    })();
    let source_map = match attempt {
        Ok(map) => map,
        Err(Raised::Value(message)) => {
            remove_tree(&output)?;
            return Err(message);
        }
        Err(other) => return Err(other.message()),
    };
    write_source_file(&output.join("source-map.json"), &Value::Object(source_map))?;
    emit_stdout(&format!(
        "Domain Memory initialized. The sources are agent-asserted: you selected them, and {authority}, \
         the source authority, has not said they are the right corpus. Show them the selected paths and ask. When \
         they answer, record it with \
         confirm-sources --confirmed-by <the identity they give you>; until then verify-sources reports the map \
         as unverified, which is the truth and not a problem to work around.\n"
    ));
    Ok(0)
}

fn selected_of(source_map: &Object) -> Vec<String> {
    source_map
        .get("selected_paths")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(|item| item.as_str().map(str::to_string))
        .collect()
}

pub fn confirm_sources(invocation: &Invocation) -> Result<i32, String> {
    let registry_root = resolved(invocation, "registry_root");
    let repo_root = resolved(invocation, "repo_root");
    let confirmed_by = invocation.text("confirmed_by").unwrap_or_default();
    let policy = existing_policy(&registry_root)?;
    let source_map =
        sources::confirm_sources(&registry_root, &repo_root, confirmed_by, policy.as_ref())?;
    emit_stdout(&format!(
        "Sources confirmed by {confirmed_by}: {}\n",
        selected_of(&source_map).join(", ")
    ));
    Ok(0)
}

pub fn refresh_sources(invocation: &Invocation) -> Result<i32, String> {
    let registry_root = resolved(invocation, "registry_root");
    let repo_root = resolved(invocation, "repo_root");
    let policy = existing_policy(&registry_root)?;
    let source_map = sources::refresh_sources(&registry_root, &repo_root, policy.as_ref())?;
    emit_stdout(&format!(
        "{}\n",
        sources::refresh_confirmation_notice(&selected_of(&source_map))
    ));
    Ok(0)
}

pub fn refresh_listing(invocation: &Invocation) -> Result<i32, String> {
    let registry_root = resolved(invocation, "registry_root");
    let repo_root = resolved(invocation, "repo_root");
    let policy = existing_policy(&registry_root)?;
    let change = sources::refresh_listing(&registry_root, &repo_root, policy.as_ref())?;
    let (added, removed) = (change.added.len(), change.removed.len());
    if added + removed == 0 {
        emit_stdout("Listing already current\n");
        return Ok(0);
    }
    let noun = if added + removed == 1 {
        "path"
    } else {
        "paths"
    };
    emit_stdout(&format!(
        "Listing refreshed: +{added} -{removed} discovered {noun}\n"
    ));
    for (name, items) in [("added", &change.added), ("removed", &change.removed)] {
        if !items.is_empty() {
            emit_stdout(&format!("{name}: {}\n", items.join(", ")));
        }
    }
    Ok(0)
}

pub fn refine_sources(invocation: &Invocation) -> Result<i32, String> {
    let registry_root = resolved(invocation, "registry_root");
    let repo_root = resolved(invocation, "repo_root");
    let changes = !invocation.texts("add").is_empty() || !invocation.texts("remove").is_empty();
    if !invocation.texts("source").is_empty() == changes {
        return Err(
            "give the whole selection with --source, or change it with --add and --remove, not both"
                .to_string(),
        );
    }
    let selection = if changes {
        sources::adjusted_selection(
            &registry_root,
            &repo_root,
            sources_under(&repo_root, invocation.texts("add")),
            &sources_under(&repo_root, invocation.texts("remove")),
        )?
    } else {
        sources_under(&repo_root, invocation.texts("source"))
    };
    let policy = load_json(&policy_path(&registry_root))?;
    let source_map = sources::refine_sources(&registry_root, &repo_root, &selection, &policy)?;
    emit_stdout(&format!(
        "Focused sources selected and require developer confirmation: {}\n",
        selected_of(&source_map).join(", ")
    ));
    Ok(0)
}
