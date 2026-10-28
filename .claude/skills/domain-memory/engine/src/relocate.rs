use std::path::Path;

use crate::audit_log::ReviewingProposals;
use crate::candidate_evidence::source_map_for;
use crate::cite::{citation, excerpt_starts, line_range};
use crate::coverage::{SourceKinds, all_references, with_references};
use crate::evidence::{Verifier, WIDEN_TO_UNIQUE};
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::pyvalue::{equal, str_of};
use crate::registry_write::{is_reviewed, without};
use crate::store::{ASSETS, Registry};

pub struct Recitation<'a> {
    pub asset: &'a str,
    pub record_id: &'a str,
    pub path: &'a str,
    pub lines: &'a str,
    pub replace: Option<&'a str>,
    pub onto: Option<Object>,
}

struct Relocation<'a> {
    repo_root: &'a Path,
    old: &'a str,
    new: &'a str,
    kinds: SourceKinds,
    unmatched: Vec<Object>,
    in_place: usize,
}

struct RecordUpdate {
    registry_updates: Vec<Value>,
    supersedes: Vec<Value>,
    candidates: Vec<Value>,
}

fn posix_parts(text: &str) -> Vec<String> {
    let slashes = text.len() - text.trim_start_matches('/').len();
    let mut parts = Vec::new();
    match slashes {
        0 => {}
        2 => parts.push("//".to_string()),
        _ => parts.push("/".to_string()),
    }
    parts.extend(
        text.split('/')
            .filter(|part| !part.is_empty() && *part != ".")
            .map(str::to_string),
    );
    parts
}

fn whole(value: Option<&Value>) -> Result<i128, String> {
    match value {
        None => Ok(0),
        Some(Value::Int(digits)) => Ok(digits.parse().unwrap_or(if digits.starts_with('-') {
            i128::MIN / 4
        } else {
            i128::MAX / 4
        })),
        Some(Value::Bool(flag)) => Ok(i128::from(*flag)),
        Some(other) => Err(format!(
            "unsupported operand type(s) for -: {}",
            crate::pyvalue::repr_of(other)
        )),
    }
}

fn text_of(value: Option<&Value>) -> String {
    value.map_or_else(|| "None".to_string(), str_of)
}

impl Relocation<'_> {
    fn target(&self, path: &str) -> Option<String> {
        let old = self.old.trim_end_matches('/');
        let new = self.new.trim_end_matches('/');
        if matches!(old, "" | ".") && matches!(new, "" | ".") {
            return Some(path.to_string());
        }
        if path == old {
            return Some(new.to_string());
        }
        path.strip_prefix(old)
            .and_then(|rest| rest.strip_prefix('/'))
            .map(|rest| format!("{new}/{rest}"))
    }

    fn placement(&self, implementation_path: &str) -> Option<String> {
        if let Some(target) = self.target(implementation_path) {
            return Some(target);
        }
        let owned = posix_parts(implementation_path.trim_matches('/'));
        let old = posix_parts(self.old.trim_end_matches('/'));
        let new = posix_parts(self.new.trim_end_matches('/'));
        if owned.is_empty()
            || old.len() < owned.len()
            || old[old.len() - owned.len()..] != owned[..]
        {
            return None;
        }
        let from = new.len().saturating_sub(owned.len());
        Some(new[from..].join("/"))
    }

    fn placed(&self, asset: &str, record: Object) -> Object {
        let path = record.get("implementation_path").and_then(Value::as_str);
        let target = match path {
            Some(path) if asset == "contexts" => self.placement(path),
            _ => None,
        };
        match target {
            Some(target) if Some(target.as_str()) != path => {
                let mut placed = record;
                placed.insert("implementation_path", Value::Str(target));
                placed
            }
            _ => record,
        }
    }

    fn moved(&mut self, value: &Value) -> Result<Value, String> {
        with_references(value, &mut |reference| self.reference(reference), 0)
    }

    fn reference(&mut self, reference: &Value) -> Result<Value, String> {
        let Value::Object(object) = reference else {
            return Ok(reference.clone());
        };
        let path = match object.get("path") {
            None => "",
            Some(Value::Str(path)) => path,
            Some(other) => {
                return Err(format!(
                    "'{}' object has no attribute 'startswith'",
                    crate::audit_log::type_name(other)
                ));
            }
        };
        let Some(target) = self.target(path) else {
            return Ok(reference.clone());
        };
        let verdict = Verifier::new(self.repo_root, false).check(reference)?;
        let status = verdict.get("status").and_then(Value::as_str);
        let moved_path = object
            .get("path")
            .is_none_or(|held| held.as_str() != Some(&target));
        if status == Some("current") || (status == Some("shifted") && moved_path) {
            self.in_place += 1;
            return Ok(reference.clone());
        }
        let lines = match object.get("lines") {
            Some(Value::Object(found)) => found.clone(),
            Some(held) if truthy(held) => {
                return Err(format!(
                    "'{}' object has no attribute 'get'",
                    crate::audit_log::type_name(held)
                ));
            }
            _ => Object::new(),
        };
        let length = whole(lines.get("end"))? - whole(lines.get("start"))? + 1;
        let excerpt = object.get("excerpt_sha256");
        let digest = excerpt.filter(|held| truthy(held));
        let starts = match digest {
            Some(Value::Str(digest)) if length > 0 => excerpt_starts(
                self.repo_root,
                &target,
                usize::try_from(length).unwrap_or(usize::MAX),
                digest,
            ),
            _ => Vec::new(),
        };
        if starts.len() != 1 {
            let mut problem = Object::new();
            problem.insert("path", object.get("path").cloned().unwrap_or(Value::Null));
            problem.insert("lines", Value::Object(lines));
            problem.insert("target", Value::Str(target.clone()));
            problem.insert("found", Value::int(starts.len()));
            problem.insert(
                "reason",
                Value::Str(unmatched_reason(
                    &target,
                    starts.len(),
                    digest.is_some(),
                    length,
                )),
            );
            self.unmatched.push(problem);
            return Ok(reference.clone());
        }
        let first = starts[0];
        let last = first + usize::try_from(length).unwrap_or(0) - 1;
        let recited = citation(
            self.repo_root,
            &target,
            &first.to_string(),
            &last.to_string(),
        )?;
        let mut merged = without(object, &["source_kind"]);
        for (key, value) in recited.iter() {
            merged.insert(key.clone(), value.clone());
        }
        Ok(self.kinds.classify(&Value::Object(merged)))
    }
}

fn unmatched_reason(target: &str, found: usize, has_excerpt: bool, length: i128) -> String {
    if !has_excerpt {
        return "the citation has no excerpt digest, so nothing can find its lines; cite it again with cite".to_string();
    }
    if length <= 0 {
        return "the cited line range is empty; cite it again with cite".to_string();
    }
    if found == 0 {
        return format!(
            "the cited excerpt occurs 0 times in {target}; its content changed, so check the fact and recite it"
        );
    }
    format!(
        "the cited excerpt occurs {found} times in {target}; recite it with the lines that hold the fact. {WIDEN_TO_UNIQUE}"
    )
}

fn record_update(
    asset: &str,
    entry: &Object,
    changed: &Object,
    reviewing: &ReviewingProposals,
) -> RecordUpdate {
    let record = Value::Object(without(changed, &["verified"]));
    if is_reviewed(asset, entry) {
        let key = format!("{asset}/{}", text_of(entry.get("id")));
        let mut update = Object::new();
        update.insert("operation", Value::Str("upsert".to_string()));
        update.insert("asset", Value::Str(asset.to_string()));
        update.insert("record", record);
        return RecordUpdate {
            registry_updates: vec![Value::Object(update)],
            supersedes: reviewing
                .proposal(&key)
                .into_iter()
                .filter(truthy)
                .collect(),
            candidates: Vec::new(),
        };
    }
    let mut candidate = Object::new();
    candidate.insert("asset", Value::Str(asset.to_string()));
    candidate.insert("record", record);
    RecordUpdate {
        registry_updates: Vec::new(),
        supersedes: Vec::new(),
        candidates: vec![Value::Object(candidate)],
    }
}

fn proposals_in(onto: &Object, key: &str) -> Result<Vec<Value>, String> {
    match onto.get(key) {
        None => Ok(Vec::new()),
        Some(Value::Array(items)) => Ok(items.clone()),
        Some(other) => Err(format!(
            "'{}' object is not iterable",
            crate::audit_log::type_name(other)
        )),
    }
}

fn updates_record(update: &Value, asset: &str, record_id: &str) -> Result<bool, String> {
    let Value::Object(update) = update else {
        return Err(format!(
            "'{}' object has no attribute 'get'",
            crate::audit_log::type_name(update)
        ));
    };
    if update.get("asset").and_then(Value::as_str) != Some(asset) {
        return Ok(false);
    }
    Ok(match update.get("record") {
        Some(Value::Object(record)) => record.get("id").and_then(Value::as_str) == Some(record_id),
        _ => false,
    })
}

fn earlier_record(
    onto: Option<&Object>,
    asset: &str,
    record_id: &str,
) -> Result<Option<Object>, String> {
    let Some(onto) = onto else {
        return Ok(None);
    };
    let mut proposed = proposals_in(onto, "registry_updates")?;
    proposed.extend(proposals_in(onto, "candidates")?);
    for item in &proposed {
        if updates_record(item, asset, record_id)?
            && let Some(Value::Object(record)) = item.as_object().and_then(|it| it.get("record"))
        {
            return Ok(Some(record.clone()));
        }
    }
    Ok(None)
}

fn others(items: Vec<Value>, asset: &str, record_id: &str) -> Result<Vec<Value>, String> {
    let mut kept = Vec::new();
    for item in items {
        if !updates_record(&item, asset, record_id)? {
            kept.push(item);
        }
    }
    Ok(kept)
}

fn sorted_supersedes(onto: &Object, update: &RecordUpdate) -> Result<Vec<Value>, String> {
    let mut named: Vec<String> = Vec::new();
    for value in proposals_in(onto, "supersedes")?
        .iter()
        .chain(&update.supersedes)
    {
        let Value::Str(text) = value else {
            return Err("'<' not supported between instances of 'str' and 'NoneType'".to_string());
        };
        if !named.contains(text) {
            named.push(text.clone());
        }
    }
    named.sort();
    Ok(named.into_iter().map(Value::Str).collect())
}

fn merged(
    onto: &Object,
    update: RecordUpdate,
    asset: &str,
    record_id: &str,
    resolved: &Object,
) -> Result<Object, String> {
    let mut merged = onto.clone();
    let mut registry_updates = others(proposals_in(onto, "registry_updates")?, asset, record_id)?;
    registry_updates.extend(update.registry_updates.iter().cloned());
    let supersedes = sorted_supersedes(onto, &update)?;
    let mut candidates = others(proposals_in(onto, "candidates")?, asset, record_id)?;
    candidates.extend(update.candidates.iter().cloned());
    merged.insert("registry_updates", Value::Array(registry_updates));
    merged.insert("supersedes", Value::Array(supersedes));
    merged.insert("candidates", Value::Array(candidates));
    if onto.get("unmatched").is_some() {
        let null = Value::Null;
        let wanted = [
            Value::Str(asset.to_string()),
            Value::Str(record_id.to_string()),
            resolved.get("path").cloned().unwrap_or(Value::Null),
            resolved.get("lines").cloned().unwrap_or(Value::Null),
        ];
        let mut kept = Vec::new();
        for problem in proposals_in(onto, "unmatched")? {
            let Value::Object(held) = &problem else {
                return Err(format!(
                    "'{}' object has no attribute 'get'",
                    crate::audit_log::type_name(&problem)
                ));
            };
            let same = ["asset", "id", "path", "lines"]
                .iter()
                .zip(&wanted)
                .all(|(key, value)| equal(held.get(key).unwrap_or(&null), value));
            if !same {
                kept.push(problem);
            }
        }
        merged.insert("unmatched", Value::Array(kept));
    }
    Ok(merged)
}

pub fn recite(root: &Path, repo_root: &Path, request: Recitation) -> Result<Object, String> {
    let Recitation {
        asset,
        record_id,
        path,
        lines,
        replace,
        onto,
    } = request;
    let registry = Registry::open(root)?;
    let entry = registry
        .records(asset)?
        .into_iter()
        .filter_map(|item| item.as_object().cloned())
        .find(|item| item.get("id").and_then(Value::as_str) == Some(record_id))
        .ok_or_else(|| format!("no {asset} record {record_id}"))?;
    let working = earlier_record(onto.as_ref(), asset, record_id)?.unwrap_or_else(|| entry.clone());
    let replaced_text = replace.filter(|given| !given.is_empty()).unwrap_or(path);
    let (old_path, old_lines) = replaced_text.split_once(':').unwrap_or((replaced_text, ""));
    let wanted = if old_lines.is_empty() {
        None
    } else {
        let (start, end) = line_range(old_lines)?;
        Some((Value::Int(start), Value::Int(end)))
    };
    let replaced = |reference: &Value| -> Result<bool, String> {
        let Value::Object(held) = reference else {
            return Ok(false);
        };
        if held.get("path").and_then(Value::as_str) != Some(old_path) {
            return Ok(false);
        }
        let Some((start, end)) = &wanted else {
            return Ok(true);
        };
        let spans = match held.get("lines") {
            Some(Value::Object(found)) => found.clone(),
            Some(other) if truthy(other) => {
                return Err(format!(
                    "'{}' object has no attribute 'get'",
                    crate::audit_log::type_name(other)
                ));
            }
            _ => Object::new(),
        };
        let null = Value::Null;
        Ok(equal(spans.get("start").unwrap_or(&null), start)
            && equal(spans.get("end").unwrap_or(&null), end))
    };
    let working_value = Value::Object(working);
    let mut matches = 0;
    for reference in all_references(&working_value, 0)? {
        if replaced(&reference)? {
            matches += 1;
        }
    }
    if matches != 1 {
        let mut cited: Vec<String> = Vec::new();
        for reference in all_references(&working_value, 0)? {
            let Value::Object(held) = reference else {
                continue;
            };
            let spans = held.get("lines").and_then(Value::as_object);
            let bound = |key: &str| text_of(spans.and_then(|found| found.get(key)));
            cited.push(format!(
                "{}:{}-{}",
                text_of(held.get("path")),
                bound("start"),
                bound("end")
            ));
        }
        cited.sort();
        let shown = if cited.is_empty() {
            "nothing".to_string()
        } else {
            cited.join(", ")
        };
        return Err(format!(
            "{asset}/{record_id} has {matches} citations of {replaced_text}; name the one to replace with --replace PATH:START-END. It cites: {shown}"
        ));
    }
    let (start, end) = line_range(lines)?;
    let cited = citation(repo_root, path, &start, &end)?;
    let source_map = source_map_for(root)?;
    let recited = SourceKinds::of(&source_map).classify(&Value::Object(cited));
    let mut resolved: Option<Object> = None;
    let changed = with_references(
        &working_value,
        &mut |reference| {
            if replaced(reference)? {
                resolved = reference.as_object().cloned();
                return Ok(recited.clone());
            }
            Ok(reference.clone())
        },
        0,
    )?;
    let reviewing = ReviewingProposals::of(root)?;
    let Value::Object(changed) = changed else {
        return Err("a record is an object".to_string());
    };
    let update = record_update(asset, &entry, &changed, &reviewing);
    let Some(onto) = onto else {
        return Ok(update_object(update));
    };
    let resolved = resolved.unwrap_or_default();
    merged(&onto, update, asset, record_id, &resolved)
}

fn update_object(update: RecordUpdate) -> Object {
    let mut object = Object::new();
    object.insert("registry_updates", Value::Array(update.registry_updates));
    object.insert("supersedes", Value::Array(update.supersedes));
    object.insert("candidates", Value::Array(update.candidates));
    object
}

pub fn relocate_evidence(
    root: &Path,
    repo_root: &Path,
    old: &str,
    new: &str,
) -> Result<Object, String> {
    let source_map = source_map_for(root)?;
    let mut relocation = Relocation {
        repo_root,
        old,
        new,
        kinds: SourceKinds::of(&source_map),
        unmatched: Vec::new(),
        in_place: 0,
    };
    let reviewing = ReviewingProposals::of(root)?;
    let mut updates = Vec::new();
    let mut candidates = Vec::new();
    let mut supersedes: Vec<String> = Vec::new();
    let registry = Registry::open(root)?;
    for (asset, _, _) in ASSETS {
        for item in registry.records(asset)? {
            let Value::Object(entry) = item else {
                continue;
            };
            let unmatched_before = relocation.unmatched.len();
            let Value::Object(moved) = relocation.moved(&Value::Object(entry.clone()))? else {
                continue;
            };
            let moved = relocation.placed(asset, moved);
            for problem in &mut relocation.unmatched[unmatched_before..] {
                problem.insert("asset", Value::Str((*asset).to_string()));
                problem.insert("id", entry.get("id").cloned().unwrap_or(Value::Null));
            }
            if equal(&Value::Object(moved.clone()), &Value::Object(entry.clone())) {
                continue;
            }
            let update = record_update(asset, &entry, &moved, &reviewing);
            updates.extend(update.registry_updates);
            for proposal in update.supersedes {
                let Value::Str(proposal) = proposal else {
                    return Err(
                        "'<' not supported between instances of 'int' and 'str'".to_string()
                    );
                };
                if !supersedes.contains(&proposal) {
                    supersedes.push(proposal);
                }
            }
            candidates.extend(update.candidates);
        }
    }
    supersedes.sort();
    let mut report = Object::new();
    report.insert("from", Value::Str(old.to_string()));
    report.insert("to", Value::Str(new.to_string()));
    report.insert("registry_updates", Value::Array(updates));
    report.insert(
        "supersedes",
        Value::Array(supersedes.into_iter().map(Value::Str).collect()),
    );
    report.insert("candidates", Value::Array(candidates));
    report.insert(
        "unmatched",
        Value::Array(
            relocation
                .unmatched
                .into_iter()
                .map(Value::Object)
                .collect(),
        ),
    );
    report.insert("citations_in_place", Value::int(relocation.in_place));
    Ok(report)
}

#[cfg(test)]
mod tests {
    use super::posix_parts;

    #[test]
    fn a_posix_path_keeps_its_root_as_a_part_and_drops_dots() {
        assert_eq!(posix_parts("/a/./b"), ["/", "a", "b"]);
        assert_eq!(posix_parts("//a"), ["//", "a"]);
        assert_eq!(posix_parts("///a"), ["/", "a"]);
        assert_eq!(posix_parts("a//b/"), ["a", "b"]);
        assert!(posix_parts("").is_empty());
    }
}
