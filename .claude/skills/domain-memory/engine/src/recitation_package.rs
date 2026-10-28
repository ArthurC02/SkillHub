use std::collections::BTreeSet;
use std::path::Path;

use crate::changes::{ParsedUpdate, init_change_package, parsed_update};
use crate::common::{OBLIGATIONS_FILE, REQUIREMENT_FILE};
use crate::compare::contexts_named_by;
use crate::coverage::all_references;
use crate::evidence::fields_beyond_evidence;
use crate::files::{PROPOSAL_FILE, load_json, write_json};
use crate::git::{Failure, toplevel_found};
use crate::json::{Object, Value};
use crate::shlex::join;
use crate::store::{asset_records, shown_path};

const CITATIONS_CURRENT: &str = "citations-current";

fn text(value: &str) -> Value {
    Value::Str(value.to_string())
}

fn strings(items: impl IntoIterator<Item = String>) -> Value {
    Value::Array(items.into_iter().map(Value::Str).collect())
}

fn object(pairs: Vec<(&str, Value)>) -> Value {
    let mut built = Object::new();
    for (key, value) in pairs {
        built.insert(key, value);
    }
    Value::Object(built)
}

fn fill(path: &Path, values: Vec<(&str, Value)>) -> Result<(), String> {
    let mut document = load_json(path)?;
    for (key, value) in values {
        document.insert(key, value);
    }
    write_json(path, &Value::Object(document))
}

fn beyond_recitation(fields: &[String], added: i64) -> String {
    if fields == ["evidence"] && added != 0 {
        let how = if added > 0 {
            format!("adds {added}")
        } else {
            format!("drops {}", -added)
        };
        return format!(
            "{how} citation(s) and changes nothing else; a re-citation package only moves the citations a record \
             already has, so adding or dropping one is a change of claim"
        );
    }
    format!("changes more than its citations: {}", fields.join(", "))
}

fn reference_count(record: &Object) -> Result<i64, String> {
    Ok(all_references(&Value::Object(record.clone()), 0)?.len() as i64)
}

fn evidence_only_updates(
    registry_root: &Path,
    recitation: &Object,
) -> Result<Vec<(String, Object)>, String> {
    let held = |key: &str| recitation.get(key).is_some_and(crate::okf::truthy);
    if held("candidates") {
        return Err(
            "the recitation carries candidates; they go to upsert-candidate, so leave them out"
                .to_string(),
        );
    }
    if held("unmatched") {
        return Err(
            "the recitation still has unmatched citations; re-cite them with recite --onto before packaging it"
                .to_string(),
        );
    }
    let raw_updates = match recitation.get("registry_updates") {
        Some(Value::Array(items)) if !items.is_empty() => items,
        _ => return Err("the recitation has no registry_updates to package".to_string()),
    };
    let mut updates = Vec::new();
    for raw in raw_updates {
        let ParsedUpdate {
            asset,
            record_id,
            record,
        } = parsed_update(raw)?;
        let Some(record) = record else {
            return Err(format!(
                "{asset}/{record_id} is removed, which is more than a re-citation"
            ));
        };
        let current = asset_records(registry_root, &asset)?
            .into_iter()
            .filter_map(|entry| entry.as_object().cloned())
            .find(|entry| entry.get("id").and_then(Value::as_str) == Some(record_id.as_str()))
            .unwrap_or_default();
        let fields = fields_beyond_evidence(&asset, &current, &record)?;
        if !fields.is_empty() {
            let added = reference_count(&record)? - reference_count(&current)?;
            return Err(format!(
                "{asset}/{record_id} {}; write that Change Package with init-change-package instead",
                beyond_recitation(&fields, added)
            ));
        }
        updates.push((asset, record));
    }
    Ok(updates)
}

fn repository_top(registry_root: &Path) -> Result<Option<std::path::PathBuf>, String> {
    match toplevel_found(registry_root) {
        Ok(top) => Ok(top),
        Err(error) if error.kind == Failure::Spawn => Ok(None),
        Err(error) => Err(error.message),
    }
}

fn verify_command(
    registry_root: &Path,
    package_root: &Path,
    cited: &[String],
) -> Result<String, String> {
    let top = repository_top(registry_root)?;
    let top = top.as_deref();
    let executable = std::env::current_exe().unwrap_or_default();
    let mut arguments = vec![
        shown_path(&executable, top),
        "verify-evidence".to_string(),
        "--registry-root".to_string(),
        shown_path(registry_root, top),
        "--repo-root".to_string(),
        ".".to_string(),
        "--package-root".to_string(),
        shown_path(package_root, top),
    ];
    for path in cited {
        arguments.push("--path".to_string());
        arguments.push(path.clone());
    }
    Ok(join(&arguments))
}

pub fn recitation_package(
    output: &Path,
    registry_root: &Path,
    recitation: &Object,
    identifiers: [&str; 3],
) -> Result<Vec<String>, String> {
    let updates = evidence_only_updates(registry_root, recitation)?;
    let contexts: BTreeSet<String> = updates
        .iter()
        .flat_map(|(asset, record)| contexts_named_by(asset, record))
        .collect();
    let mut cited: BTreeSet<String> = BTreeSet::new();
    for (_, record) in &updates {
        for reference in all_references(&Value::Object(record.clone()), 0)? {
            if let Some(path) = reference
                .as_object()
                .and_then(|reference| reference.get("path"))
                .and_then(Value::as_str)
            {
                cited.insert(path.to_string());
            }
        }
    }
    let cited: Vec<String> = cited.into_iter().collect();
    let check = verify_command(registry_root, output, &cited)?;
    let [requirement_id, proposal_id, proposer] = identifiers;
    let names: Vec<String> = updates
        .iter()
        .map(|(asset, record)| {
            format!(
                "{asset}/{}",
                crate::pyvalue::str_of(record.get("id").unwrap_or(&Value::Null))
            )
        })
        .collect();
    init_change_package(
        output,
        Some(requirement_id),
        Some(proposal_id),
        Some(proposer),
    )?;
    fill(
        &output.join(REQUIREMENT_FILE),
        vec![
            (
                "intent",
                Value::Str(format!(
                    "Re-cite {} after the code they cite moved or changed; the facts they state are unchanged.",
                    names.join(", ")
                )),
            ),
            ("actor", text(proposer)),
            (
                "observable_outcome",
                text("verify-evidence reports every citation of the updated records current."),
            ),
            (
                "acceptance_criteria",
                Value::Array(vec![object(vec![
                    ("id", text(CITATIONS_CURRENT)),
                    (
                        "statement",
                        text("Every citation in the updated records matches the lines it cites."),
                    ),
                ])]),
            ),
            ("sources", strings(cited.clone())),
        ],
    )?;
    fill(
        &output.join(OBLIGATIONS_FILE),
        vec![(
            "obligations",
            Value::Array(vec![object(vec![
                ("id", text("evidence-current")),
                ("source_type", text("acceptance-criterion")),
                ("source_id", text(CITATIONS_CURRENT)),
                (
                    "expected_outcome",
                    text(
                        "verify-evidence reports current for every citation of the updated records.",
                    ),
                ),
                ("level", text("architecture")),
                ("status", text("planned")),
                ("check", Value::Str(check)),
            ])]),
        )],
    )?;
    fill(
        &output.join(PROPOSAL_FILE),
        vec![
            ("change_classification", text("routine")),
            ("affected_contexts", strings(contexts.iter().cloned())),
            ("implementation_design", Value::Null),
            (
                "registry_updates",
                recitation
                    .get("registry_updates")
                    .cloned()
                    .unwrap_or(Value::Null),
            ),
            (
                "supersedes",
                recitation
                    .get("supersedes")
                    .cloned()
                    .unwrap_or(Value::Array(Vec::new())),
            ),
        ],
    )?;
    Ok(if contexts.is_empty() {
        vec!["affected_contexts: none of the updated records names a Context".to_string()]
    } else {
        Vec::new()
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::json::parse;

    const CITED: &str = "---\ntype: Business Rule\nstatus: stable\nevidence:\n  - path: src/a.txt\n    lines:\n      start: 1\n      end: 1\n    excerpt_sha256: sha256:b6a98d9ce9a2d9149288fa3df42d377c3e42737afdcdaf714e33c0a100b51060\n---\nRule r1.\n";

    fn scratch_root(label: &str) -> std::path::PathBuf {
        let root = std::env::temp_dir().join(format!("dm-recite-{label}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(root.join("registry/rules")).unwrap();
        std::fs::write(
            root.join("registry/index.md"),
            "---\nokf_version: \"0.2\"\n---\n# Domain Registry\n",
        )
        .unwrap();
        std::fs::write(root.join("registry/rules/r1.md"), CITED).unwrap();
        root
    }

    fn recitation_moving_the_citation(registry: &Path) -> Object {
        let mut record = asset_records(registry, "rules").unwrap()[0]
            .as_object()
            .unwrap()
            .clone();
        let moved = parse("[{\"path\": \"src/b.txt\", \"lines\": {\"start\": 2, \"end\": 2}, \"excerpt_sha256\": \"sha256:b6a98d9ce9a2d9149288fa3df42d377c3e42737afdcdaf714e33c0a100b51060\"}]").unwrap();
        record.insert("evidence", moved);
        let update = object(vec![
            ("operation", text("upsert")),
            ("asset", text("rules")),
            ("record", Value::Object(record)),
        ]);
        let mut recitation = Object::new();
        recitation.insert("registry_updates", Value::Array(vec![update]));
        recitation
    }

    #[test]
    fn a_recitation_moving_one_citation_becomes_a_routine_package_citing_the_new_path() {
        let root = scratch_root("moves");
        let output = root.join("package");
        let blank = recitation_package(
            &output,
            &root,
            &recitation_moving_the_citation(&root),
            ["REQ-1", "P-1", "dev"],
        )
        .unwrap();
        assert_eq!(
            blank,
            vec!["affected_contexts: none of the updated records names a Context".to_string()]
        );
        let requirement = load_json(&output.join(REQUIREMENT_FILE)).unwrap();
        assert_eq!(
            requirement.get("sources"),
            Some(&strings(["src/b.txt".to_string()]))
        );
        assert_eq!(
            requirement.get("actor").and_then(Value::as_str),
            Some("dev")
        );
        let obligations = load_json(&output.join(OBLIGATIONS_FILE)).unwrap();
        let check = obligations
            .get("obligations")
            .and_then(Value::as_array)
            .unwrap()[0]
            .as_object()
            .and_then(|entry| entry.get("check"))
            .and_then(Value::as_str)
            .unwrap()
            .to_string();
        assert!(
            check.contains(" verify-evidence --registry-root "),
            "{check}"
        );
        assert!(check.ends_with(" --path src/b.txt"), "{check}");
        let proposal = load_json(&output.join(PROPOSAL_FILE)).unwrap();
        assert_eq!(
            proposal
                .get("change_classification")
                .and_then(Value::as_str),
            Some("routine")
        );
        assert_eq!(proposal.get("supersedes"), Some(&Value::Array(Vec::new())));
        std::fs::remove_dir_all(&root).unwrap();
    }

    #[test]
    fn a_recitation_adding_a_citation_is_refused_with_the_count() {
        let root = scratch_root("adds");
        let mut recitation = recitation_moving_the_citation(&root);
        let Some(Value::Array(updates)) = recitation.get_mut("registry_updates") else {
            panic!()
        };
        let Value::Object(update) = &mut updates[0] else {
            panic!()
        };
        let Some(Value::Object(record)) = update.get_mut("record") else {
            panic!()
        };
        let Some(Value::Array(cited)) = record.get_mut("evidence") else {
            panic!()
        };
        cited.push(cited[0].clone());
        let refusal = recitation_package(
            &root.join("package"),
            &root,
            &recitation,
            ["REQ-1", "P-1", "dev"],
        )
        .unwrap_err();
        assert!(
            refusal.starts_with("rules/r1 adds 1 citation(s) and changes nothing else;"),
            "{refusal}"
        );
        assert!(!root.join("package").exists());
        std::fs::remove_dir_all(&root).unwrap();
    }
}
