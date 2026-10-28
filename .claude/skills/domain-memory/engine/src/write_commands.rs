use std::path::PathBuf;

use crate::cite::{OUTSIDE_SOURCES_CONSEQUENCE, citation, function_span, line_range};
use crate::cli::Invocation;
use crate::json::Value;
use crate::promotion::{Span, add_promoted_evidence, drop_promoted_evidence, promote_candidate};
use crate::py;
use crate::pyvalue::str_of;
use crate::reconciliation::{reconcile_pending_update, reconciliation_path};
use crate::transaction::{recover_interrupted_update, transaction_path};
use crate::updates::{demote_local_reviews, retract_candidate, upsert_candidate};
use crate::{emit_stderr, emit_stdout};

pub fn resolved(invocation: &Invocation, dest: &str) -> PathBuf {
    py::resolve(invocation.text(dest).unwrap_or("."))
}

fn upsert_summary(count: usize, created: usize) -> String {
    if count == 1 {
        return if created > 0 {
            "Candidate record created.".to_string()
        } else {
            "Candidate record updated.".to_string()
        };
    }
    format!(
        "{count} candidate records written: {created} created, {} updated.",
        count - created
    )
}

pub fn upsert(invocation: &Invocation) -> Result<i32, String> {
    let record_files: Vec<PathBuf> = invocation
        .texts("record_file")
        .iter()
        .map(|given| py::resolve(given))
        .collect();
    let upserted = upsert_candidate(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
        invocation.text("asset").unwrap_or_default(),
        &record_files,
    )?;
    emit_stdout(&format!(
        "{}\n",
        upsert_summary(record_files.len(), upserted.created)
    ));
    if !upserted.same_evidence.is_empty() {
        emit_stderr(&format!(
            "NOTE: its cited excerpts are all among those of {}, or theirs among its; it may be a duplicate of that record.\n",
            upserted.same_evidence.join(", ")
        ));
    }
    if !upserted.outside.is_empty() {
        emit_stderr(&format!(
            "NOTE: this record rests on files the confirmed source map does not cover: {}. \
             That is reach, not an error. A Domain Memory that comes to depend on an unconfirmed file loses its \
             evidence when that file moves. {OUTSIDE_SOURCES_CONSEQUENCE}\n",
            upserted.outside.join(", ")
        ));
    }
    if !upserted.status_dropped.is_empty() {
        emit_stderr(&format!(
            "NOTE: {} carried a review status, stored as candidate: a record \
             becomes reviewed only through an approved Change Package.\n",
            upserted.status_dropped.join(", ")
        ));
    }
    Ok(0)
}

pub fn retract(invocation: &Invocation) -> Result<i32, String> {
    retract_candidate(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
        invocation.text("asset").unwrap_or_default(),
        invocation.texts("id"),
        invocation.text("reason").unwrap_or_default(),
    )?;
    emit_stdout("Candidate record retracted.\n");
    Ok(0)
}

pub fn demote(invocation: &Invocation) -> Result<i32, String> {
    demote_local_reviews(
        &resolved(invocation, "registry_root"),
        &resolved(invocation, "repo_root"),
    )?;
    emit_stdout("Local reviewed records were demoted to Working Memory candidates.\n");
    Ok(0)
}

fn cited_parts(given: &str) -> (&str, &str) {
    given.rsplit_once(':').unwrap_or(("", given))
}

fn added_citations(invocation: &Invocation) -> Result<Vec<Value>, String> {
    let (cites, functions) = (invocation.texts("cite"), invocation.texts("cite_function"));
    if cites.is_empty() && functions.is_empty() {
        return Ok(Vec::new());
    }
    let Some(repo_root) = invocation.text("repo_root") else {
        return Err("--cite and --cite-function need --repo-root".to_string());
    };
    let repo_root = py::resolve(repo_root);
    let mut spans: Vec<(&str, (String, String))> = Vec::new();
    for given in cites {
        let (path, lines) = cited_parts(given);
        spans.push((path, line_range(lines)?));
    }
    for given in functions {
        let (path, name) = cited_parts(given);
        let (start, end) = function_span(&repo_root, path, name)?;
        spans.push((path, (start.to_string(), end.to_string())));
    }
    spans
        .iter()
        .map(|(path, (start, end))| citation(&repo_root, path, start, end).map(Value::Object))
        .collect()
}

fn dropped_spans(invocation: &Invocation) -> Result<Vec<Span>, String> {
    invocation
        .texts("drop_cite")
        .iter()
        .map(|given| {
            let (path, lines) = cited_parts(given);
            let (start, end) = line_range(lines)?;
            Ok(vec![
                Value::Str(path.to_string()),
                Value::Int(start),
                Value::Int(end),
            ])
        })
        .collect()
}

pub fn promote(invocation: &Invocation) -> Result<i32, String> {
    let citations = added_citations(invocation)?;
    let dropped = dropped_spans(invocation)?;
    let package_root = resolved(invocation, "package_root");
    let asset = invocation.text("asset").unwrap_or_default();
    let record_id = invocation.text("id").unwrap_or_default();
    let superseded = promote_candidate(
        &package_root,
        &resolved(invocation, "registry_root"),
        asset,
        record_id,
        invocation.flag("amend"),
    )?;
    if !citations.is_empty() {
        add_promoted_evidence(&package_root, asset, record_id, &citations)?;
    }
    if !dropped.is_empty() {
        drop_promoted_evidence(&package_root, asset, record_id, &dropped)?;
    }
    emit_stdout(&format!(
        "{asset}/{record_id} is in registry_updates; ship-package reviews it once the proposal is approved.\n"
    ));
    if let Some(reviewer) = superseded {
        emit_stderr(&format!(
            "NOTE: the package supersedes {}, which reviewed it; change the record in registry_updates, \
             such as adding the citation of the test that proves it.\n",
            str_of(&reviewer)
        ));
    }
    Ok(0)
}

pub fn recover(invocation: &Invocation) -> Result<i32, String> {
    let root = resolved(invocation, "registry_root");
    let interrupted = transaction_path(&root).is_file();
    recover_interrupted_update(&root, invocation.flag("force"))?;
    emit_stdout(if interrupted {
        "Interrupted Registry update recovered.\n"
    } else {
        "No interrupted Registry update.\n"
    });
    if let Some(repo_root) = invocation.text("repo_root") {
        match reconcile_pending_update(&root, &py::resolve(repo_root))? {
            Some(outcome) => emit_stdout(&format!("Pending apply reconciled: {outcome}.\n")),
            None => emit_stdout("No pending apply to reconcile.\n"),
        }
        return Ok(0);
    }
    if reconciliation_path(&root).is_file() {
        emit_stderr("ERROR: a pending apply remains; rerun with --repo-root to reconcile it\n");
        return Ok(1);
    }
    Ok(0)
}
