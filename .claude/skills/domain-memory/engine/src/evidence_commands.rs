use std::path::PathBuf;

use crate::cite::{citation, line_range, repeated_note};
use crate::cli::Invocation;
use crate::files::load_json;
use crate::json::Value;
use crate::py;
use crate::relocate::{Recitation, recite, relocate_evidence};
use crate::{emit_stderr, print_json};

fn resolved(invocation: &Invocation, dest: &str) -> PathBuf {
    py::resolve(invocation.text(dest).unwrap_or("."))
}

fn replaced_lines_in_place<'a>(path: &str, replace: Option<&'a str>) -> Result<&'a str, String> {
    let (replaced_path, replaced_lines) = replace
        .unwrap_or_default()
        .split_once(':')
        .unwrap_or((replace.unwrap_or_default(), ""));
    if replaced_path != path || replaced_lines.is_empty() {
        return Err("recite needs --lines, unless --replace names lines of the same file as --path; then those lines are re-cited".to_string());
    }
    Ok(replaced_lines)
}

pub fn recite_command(invocation: &Invocation) -> Result<i32, String> {
    let repo_root = resolved(invocation, "repo_root");
    let path = invocation.text("path").unwrap_or_default();
    let replace = invocation.text("replace");
    let lines = match invocation.text("lines").filter(|given| !given.is_empty()) {
        Some(given) => given,
        None => replaced_lines_in_place(path, replace)?,
    };
    let onto = match invocation.text("onto") {
        Some(given) => Some(load_json(&resolved_given(given))?),
        None => None,
    };
    let reply = recite(
        &resolved(invocation, "registry_root"),
        &repo_root,
        Recitation {
            asset: invocation.text("asset").unwrap_or_default(),
            record_id: invocation.text("id").unwrap_or_default(),
            path,
            lines,
            replace,
            onto,
        },
    )?;
    print_json(&Value::Object(reply));
    let (start, end) = line_range(lines)?;
    let cited = citation(&repo_root, path, &start, &end)?;
    if let Some(note) = repeated_note(&repo_root, &cited) {
        emit_stderr(&format!("{note}\n"));
    }
    Ok(0)
}

fn resolved_given(given: &str) -> PathBuf {
    py::resolve(given)
}

pub fn relocate_evidence_command(invocation: &Invocation) -> Result<i32, String> {
    let repo_root = resolved(invocation, "repo_root");
    let moved_from = invocation.text("moved_from").unwrap_or_default();
    let moved_to = invocation.text("moved_to").unwrap_or_default();
    let relocated = relocate_evidence(
        &resolved(invocation, "registry_root"),
        &repo_root,
        moved_from,
        moved_to,
    )?;
    print_json(&Value::Object(relocated.clone()));
    note_contexts_placed_over_nothing(&repo_root, moved_to, &relocated);
    let held = |key: &str| {
        relocated
            .get(key)
            .and_then(Value::as_array)
            .is_some_and(|items| !items.is_empty())
    };
    if !["registry_updates", "candidates", "unmatched"]
        .iter()
        .any(|key| held(key))
    {
        let in_place: usize = match relocated.get("citations_in_place") {
            Some(Value::Int(digits)) => digits.parse().unwrap_or(0),
            _ => 0,
        };
        let plural = if in_place == 1 { "" } else { "s" };
        let verb = if in_place == 1 { "is" } else { "are" };
        let note = if in_place > 0 {
            format!(
                "NOTE: {in_place} citation{plural} under {moved_from} {verb} still in place and no Context ends with it; nothing to move."
            )
        } else {
            format!(
                "NOTE: no record cites a path under {moved_from} and no Context ends with it; nothing to move. Pass the path as the repository holds it, such as src/billing/orders for a Context at billing/orders."
            )
        };
        emit_stderr(&format!("{note}\n"));
    }
    Ok(0)
}

fn note_contexts_placed_over_nothing(
    repo_root: &std::path::Path,
    moved_to: &str,
    relocated: &crate::json::Object,
) {
    let mut placed: Vec<String> = Vec::new();
    for key in ["registry_updates", "candidates"] {
        for item in relocated
            .get(key)
            .and_then(Value::as_array)
            .unwrap_or_default()
        {
            let Some(item) = item.as_object() else {
                continue;
            };
            if item.get("asset").and_then(Value::as_str) != Some("contexts") {
                continue;
            }
            if let Some(id) = item
                .get("record")
                .and_then(Value::as_object)
                .and_then(|record| record.get("id"))
            {
                placed.push(crate::pyvalue::str_of(id));
            }
        }
    }
    if !placed.is_empty() && !repo_root.join(moved_to).exists() {
        emit_stderr(&format!(
            "NOTE: --to {moved_to} lies over no file in the repository; the Context path proposed for {} would be refused at review. Check --to for a typo.\n",
            placed.join(", ")
        ));
    }
}
