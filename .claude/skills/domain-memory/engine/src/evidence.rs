use std::cmp::Ordering;
use std::collections::HashMap;
use std::path::{Path, PathBuf};

use crate::coverage::{SourceKinds, all_references, with_references};
use crate::files::{PROPOSAL_FILE, load_json};
use crate::git;
use crate::json::{Object, Value};
use crate::okf::{review_field, truthy};
use crate::py;
use crate::pyvalue::{equal, repr_of};
use crate::registry_write::without;
use crate::sha256::hex_digest;
use crate::store::{ASSETS, Registry};

pub const MAX_EVIDENCE_BYTES: u64 = 16 * 1024 * 1024;
pub const STATUSES: [&str; 6] = [
    "current", "shifted", "stale", "unhashed", "missing", "invalid",
];
pub const FAILING: [&str; 4] = ["stale", "unhashed", "missing", "invalid"];
const POSIX_NUL_IN_PATH: &str = "lstat: embedded null character in path";
pub const WIDEN_TO_UNIQUE: &str =
    "Widen the citation to the enclosing block, so its lines occur once in the file.";

pub type Overlay = Vec<(String, Vec<(String, Option<Object>)>)>;

enum Source {
    Verdict(Object),
    Rows(Vec<String>),
}

pub struct Verifier {
    root: PathBuf,
    staged: bool,
    sources: HashMap<String, Source>,
    windows: HashMap<(String, usize), HashMap<String, Vec<usize>>>,
}

impl Verifier {
    pub fn new(repo_root: &Path, staged: bool) -> Self {
        Self {
            root: py::resolve_path(repo_root),
            staged,
            sources: HashMap::new(),
            windows: HashMap::new(),
        }
    }

    pub fn check(&mut self, reference: &Value) -> Result<Object, String> {
        let (path_text, lines) = match shape(reference) {
            Ok(found) => found,
            Err(reason) => return Ok(verdict(&[("status", "invalid"), ("reason", reason)])),
        };
        if !self.sources.contains_key(path_text) {
            let source = self.source(path_text)?;
            self.sources.insert(path_text.to_string(), source);
        }
        let rows = match &self.sources[path_text] {
            Source::Verdict(found) => return Ok(found.clone()),
            Source::Rows(rows) => rows.clone(),
        };
        let mut result = Object::new();
        result.insert("cited_lines", Value::Object(lines.clone()));
        let found = self.cited_lines_verdict(reference, path_text, lines, &rows);
        for (key, value) in found.iter() {
            result.insert(key.clone(), value.clone());
        }
        Ok(result)
    }

    pub fn exists(&self, path_text: &str) -> Result<bool, String> {
        if self.staged {
            return Ok(staged_content(&self.root, path_text)?.is_some());
        }
        Ok(self.root.join(path_text).is_file())
    }

    fn source(&self, path_text: &str) -> Result<Source, String> {
        if !cfg!(windows) && path_text.contains('\0') {
            return Err(POSIX_NUL_IN_PATH.to_string());
        }
        let path = py::resolve_path(&self.root.join(path_text));
        if !path.starts_with(&self.root) {
            return Ok(Source::Verdict(verdict(&[
                ("status", "invalid"),
                ("reason", "evidence path escapes repository"),
            ])));
        }
        let content = if self.staged {
            staged_content(&self.root, path_text)?
        } else if path.is_file() {
            if path.metadata().map(|meta| meta.len()).unwrap_or(0) > MAX_EVIDENCE_BYTES {
                return Ok(Source::Verdict(too_large(path_text)));
            }
            Some(std::fs::read(&path).map_err(|error| py::os_error(&error, &path))?)
        } else {
            None
        };
        let Some(content) = content else {
            return Ok(Source::Verdict(verdict(&[
                ("status", "missing"),
                ("path", path_text),
            ])));
        };
        if content.len() as u64 > MAX_EVIDENCE_BYTES {
            return Ok(Source::Verdict(too_large(path_text)));
        }
        Ok(Source::Rows(rows(&content)))
    }

    fn cited_lines_verdict(
        &mut self,
        reference: &Value,
        path_text: &str,
        lines: &Object,
        rows: &[String],
    ) -> Object {
        let reference = reference.as_object().expect("shape checked");
        let start = digits(lines.get("start"));
        let end = digits(lines.get("end"));
        let past_the_end = compare(end, &rows.len().to_string()) == Ordering::Greater;
        let excerpt = reference.get("excerpt_sha256");
        if past_the_end {
            return match excerpt {
                Some(excerpt) => self.moved_or_stale(excerpt, path_text, rows, span(start, end)),
                None => verdict(&[
                    ("status", "invalid"),
                    ("reason", "evidence line range exceeds source"),
                    ("path", path_text),
                ]),
            };
        }
        let Some(excerpt) = excerpt else {
            let reason = format!(
                "{path_text} was cited without an excerpt hash; regenerate the citation with cite"
            );
            return verdict(&[
                ("status", "unhashed"),
                ("path", path_text),
                ("reason", &reason),
            ]);
        };
        let (first, last) = (to_index(start), to_index(end));
        if excerpt.as_str() != Some(sha256_of(&rows[first - 1..last]).as_str()) {
            return self.moved_or_stale(excerpt, path_text, rows, span(start, end));
        }
        let mut current = verdict(&[("status", "current"), ("path", path_text)]);
        current.insert(
            "source_kind",
            reference
                .get("source_kind")
                .cloned()
                .unwrap_or_else(|| Value::Str("unclassified".to_string())),
        );
        current
    }

    fn moved_or_stale(
        &mut self,
        excerpt: &Value,
        path_text: &str,
        rows: &[String],
        length: Option<usize>,
    ) -> Object {
        let starts = match (excerpt.as_str(), length) {
            (Some(excerpt), Some(length)) => self.starts_of(path_text, rows, length, excerpt),
            _ => Vec::new(),
        };
        if starts.len() != 1 {
            let place = if starts.is_empty() {
                "no longer"
            } else {
                "repeated"
            };
            let widen = if starts.is_empty() {
                String::new()
            } else {
                format!(". {WIDEN_TO_UNIQUE}")
            };
            let reason = format!(
                "the cited excerpt is {place} in {path_text}; check that the fact still holds, then recite it{widen}"
            );
            return verdict(&[
                ("status", "stale"),
                ("path", path_text),
                ("reason", &reason),
            ]);
        }
        let length = length.expect("a found window has a length");
        let mut shifted = verdict(&[("status", "shifted"), ("path", path_text)]);
        let mut moved = Object::new();
        moved.insert("start", Value::int(starts[0]));
        moved.insert("end", Value::int(starts[0] + length - 1));
        shifted.insert("lines", Value::Object(moved));
        shifted.insert(
            "reason",
            Value::Str(format!(
                "the cited lines are unchanged but now sit elsewhere in {path_text}; this passes \
                 every check and needs no Change Package of its own; when one next touches the record, \
                 relocate-evidence --from {path_text} --to {path_text} re-cites them and prints the supersedes"
            )),
        );
        shifted
    }

    fn starts_of(
        &mut self,
        path_text: &str,
        rows: &[String],
        length: usize,
        excerpt: &str,
    ) -> Vec<usize> {
        let key = (path_text.to_string(), length);
        let windows = self.windows.entry(key).or_insert_with(|| {
            let mut found: HashMap<String, Vec<usize>> = HashMap::new();
            if length <= rows.len() {
                for start in 1..=rows.len() + 1 - length {
                    found
                        .entry(sha256_of(&rows[start - 1..start - 1 + length]))
                        .or_default()
                        .push(start);
                }
            }
            found
        });
        windows.get(excerpt).cloned().unwrap_or_default()
    }
}

pub fn verify_evidence(
    root: &Path,
    verifier: &mut Verifier,
    paths: &[String],
    overlay: &Overlay,
) -> Result<Object, String> {
    let source_map_path = root.join("source-map.json");
    let source_map = if source_map_path.is_file() {
        load_json(&source_map_path)?
    } else {
        Object::new()
    };
    let paths: Vec<String> = paths.iter().map(|path| posix_path(path)).collect();
    let kinds = SourceKinds::of(&source_map);
    let registry = Registry::open(root)?;
    let mut results: Vec<Object> = Vec::new();
    for (asset, _, _) in ASSETS {
        let changes = overlay
            .iter()
            .find(|(name, _)| name == asset)
            .map(|(_, changes)| changes.as_slice())
            .unwrap_or_default();
        for entry in overlaid(registry.records(asset)?, changes) {
            for reference in all_references(&entry, 0)? {
                let cited = reference.as_object().and_then(|it| it.get("path"));
                if !paths.is_empty()
                    && !cited
                        .and_then(Value::as_str)
                        .is_some_and(|it| paths.iter().any(|wanted| wanted == it))
                {
                    continue;
                }
                let mut item = Object::new();
                item.insert("asset", Value::Str((*asset).to_string()));
                item.insert(
                    "id",
                    entry
                        .as_object()
                        .and_then(|it| it.get("id"))
                        .cloned()
                        .unwrap_or(Value::Null),
                );
                for (key, value) in verifier.check(&kinds.classify(&reference))?.iter() {
                    item.insert(key.clone(), value.clone());
                }
                results.push(item);
            }
        }
    }
    let mut summary = Object::new();
    for status in STATUSES {
        let count = results
            .iter()
            .filter(|item| item.get("status").and_then(Value::as_str) == Some(status))
            .count();
        summary.insert(status, Value::int(count));
    }
    let mut kinds_seen: Vec<(Value, usize)> = Vec::new();
    for kind in results.iter().filter_map(|item| item.get("source_kind")) {
        match kinds_seen.iter_mut().find(|(seen, _)| equal(seen, kind)) {
            Some(slot) => slot.1 += 1,
            None => kinds_seen.push((kind.clone(), 1)),
        }
    }
    kinds_seen.sort_by(|(left, _), (right, _)| left.as_str().cmp(&right.as_str()));
    let mut by_kind = Object::new();
    for (kind, count) in kinds_seen {
        by_kind.insert(kind.as_str().unwrap_or_default(), Value::int(count));
    }
    summary.insert("by_source_kind", Value::Object(by_kind));
    let mut report = Object::new();
    if !paths.is_empty() {
        let mut shown = Object::new();
        for path in &paths {
            if shown.get(path).is_some() {
                continue;
            }
            let citations = results
                .iter()
                .filter(|item| item.get("path").and_then(Value::as_str) == Some(path))
                .count();
            let mut row = Object::new();
            row.insert("citations", Value::int(citations));
            row.insert("exists", Value::Bool(verifier.exists(path)?));
            shown.insert(path.clone(), Value::Object(row));
        }
        report.insert("paths", Value::Object(shown));
    }
    let mut ordered = Object::new();
    ordered.insert(
        "results",
        Value::Array(results.into_iter().map(Value::Object).collect()),
    );
    ordered.insert("summary", Value::Object(summary));
    for (key, value) in report.iter() {
        ordered.insert(key.clone(), value.clone());
    }
    Ok(ordered)
}

pub fn failure_message(report: &Object, failing: &[&str], staged: bool) -> Option<String> {
    let summary = report.get("summary").and_then(Value::as_object)?;
    let count_of = |status: &str| -> usize {
        match summary.get(status) {
            Some(Value::Int(count)) => count.parse().unwrap_or(0),
            _ => 0,
        }
    };
    let found: Vec<String> = failing
        .iter()
        .filter(|status| count_of(status) > 0)
        .map(|status| format!("{} {status}", count_of(status)))
        .collect();
    if found.is_empty() {
        return None;
    }
    let total: usize = failing.iter().map(|status| count_of(status)).sum();
    let counted = format!(
        "verify-evidence failed: {} citation{}",
        found.join(", "),
        if total == 1 { "" } else { "s" }
    );
    if !staged {
        return Some(format!(
            "{counted}; run recite on each to cite its current lines"
        ));
    }
    let mut lines: Vec<String> = report
        .get("results")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .filter(|item| {
            item.get("status")
                .and_then(Value::as_str)
                .is_some_and(|status| failing.contains(&status))
        })
        .map(failing_citation_line)
        .collect();
    lines.push(counted);
    Some(lines.join("\n"))
}

fn failing_citation_line(item: &Object) -> String {
    let text = |key: &str| {
        item.get(key)
            .map(crate::pyvalue::str_of)
            .unwrap_or_default()
    };
    let cited = item
        .get("path")
        .map_or_else(|| "its evidence".to_string(), crate::pyvalue::str_of);
    let span = match item.get("cited_lines").and_then(Value::as_object) {
        Some(lines) if lines.iter().next().is_some() => format!(
            "{cited}:{}-{}",
            lines
                .get("start")
                .map(crate::pyvalue::str_of)
                .unwrap_or_default(),
            lines
                .get("end")
                .map(crate::pyvalue::str_of)
                .unwrap_or_default()
        ),
        _ => cited,
    };
    let record = format!("{}/{}", text("asset"), text("id"));
    match text("status").as_str() {
        "stale" => format!(
            "{record}: {span} changed; check that its statement still holds; recite only if it does, otherwise amend it"
        ),
        "missing" => format!(
            "{record}: {} is not in this commit; relocate-evidence if it moved, otherwise amend or retire the record",
            text("path")
        ),
        status => {
            let reason = item
                .get("reason")
                .map(|reason| format!("; {}", crate::pyvalue::str_of(reason)))
                .unwrap_or_default();
            format!("{record}: {span} is {status}{reason}")
        }
    }
}

pub fn package_overlay(package_root: &Path) -> Result<Overlay, String> {
    let proposal = load_json(&package_root.join(PROPOSAL_FILE))?;
    let updates: Vec<Value> = match proposal.get("registry_updates") {
        Some(Value::Array(items)) => items.clone(),
        Some(other) if truthy(other) => vec![Value::Null],
        _ => Vec::new(),
    };
    let mut overlay: Overlay = Vec::new();
    for raw in &updates {
        let (asset, record_id, record) = parsed_update(raw)?;
        let slot = match overlay.iter().position(|(name, _)| *name == asset) {
            Some(at) => at,
            None => {
                overlay.push((asset, Vec::new()));
                overlay.len() - 1
            }
        };
        let changes = &mut overlay[slot].1;
        match changes.iter_mut().find(|(seen, _)| *seen == record_id) {
            Some(change) => change.1 = record,
            None => changes.push((record_id, record)),
        }
    }
    Ok(overlay)
}

pub fn parsed_update(update: &Value) -> Result<(String, String, Option<Object>), String> {
    let operation = update
        .as_object()
        .and_then(|it| it.get("operation"))
        .and_then(Value::as_str);
    let (Some(update), Some("upsert" | "remove")) = (update.as_object(), operation) else {
        return Err("each registry update must be an upsert or a remove".to_string());
    };
    let removing = operation == Some("remove");
    let asset = match update.get("asset") {
        Some(Value::Str(asset)) if ASSETS.iter().any(|(name, _, _)| name == asset) => asset.clone(),
        other => return Err(unknown_asset_message(other.unwrap_or(&Value::Null))),
    };
    let record = if removing { None } else { update.get("record") };
    let record_id = if removing {
        update.get("id")
    } else {
        record
            .and_then(Value::as_object)
            .and_then(|it| it.get("id"))
    };
    let completed = record_id
        .and_then(Value::as_str)
        .is_some_and(|it| !py::strip(it).is_empty() && !it.starts_with('<'));
    let shaped = removing || record.is_some_and(|it| matches!(it, Value::Object(_)));
    let (true, true, Some(Value::Str(record_id))) = (shaped, completed, record_id) else {
        return Err(format!(
            "registry update for {asset} needs a record with an id, or a remove with an id"
        ));
    };
    Ok((
        asset,
        record_id.clone(),
        record.and_then(Value::as_object).cloned(),
    ))
}

fn unknown_asset_message(asset: &Value) -> String {
    let mut valid: Vec<&str> = ASSETS.iter().map(|(name, _, _)| *name).collect();
    valid.sort_unstable();
    let mut message = format!(
        "registry update names unknown asset {}; valid assets: {}",
        repr_of(asset),
        valid.join(", ")
    );
    if let Some((holder, _, _)) = ASSETS
        .iter()
        .find(|(_, key, _)| Some(*key) == asset.as_str())
    {
        message.push_str(&format!(
            "; {} is the list inside {holder}.json, whose asset is {}",
            repr_of(asset),
            py::repr(holder)
        ));
    }
    message
}

fn overlaid(records: Vec<Value>, changes: &[(String, Option<Object>)]) -> Vec<Value> {
    if changes.is_empty() {
        return records;
    }
    let id_of = |entry: &Value| entry.as_object().and_then(|it| it.get("id")).cloned();
    let change_for = |entry: &Value| {
        let id = id_of(entry)?;
        changes
            .iter()
            .find(|(changed, _)| id.as_str() == Some(changed.as_str()))
    };
    let mut out: Vec<Option<Value>> = records
        .iter()
        .map(|entry| match change_for(entry) {
            Some((_, record)) => record.clone().map(Value::Object),
            None => Some(entry.clone()),
        })
        .collect();
    for (record_id, record) in changes {
        let present = records
            .iter()
            .any(|entry| id_of(entry).is_some_and(|id| id.as_str() == Some(record_id)));
        if !present {
            out.push(record.clone().map(Value::Object));
        }
    }
    out.into_iter().flatten().collect()
}

fn posix_path(given: &str) -> String {
    let text = if cfg!(windows) {
        given.replace('\\', "/")
    } else {
        given.to_string()
    };
    let root = if text.starts_with("//") && !text.starts_with("///") {
        "//"
    } else if text.starts_with('/') {
        "/"
    } else {
        ""
    };
    let parts: Vec<&str> = text
        .split('/')
        .filter(|part| !part.is_empty() && *part != ".")
        .collect();
    let joined = format!("{root}{}", parts.join("/"));
    if joined.is_empty() {
        ".".to_string()
    } else {
        joined
    }
}

fn staged_content(root: &Path, path_text: &str) -> Result<Option<Vec<u8>>, String> {
    let failure = format!("could not read the Git index of {}", py::shown(root));
    let checked = |arguments: &[&str]| -> Result<Vec<u8>, String> {
        let done =
            git::run_git_bytes(root, arguments).map_err(|error| format!("{failure}: {error}"))?;
        if done.code != 0 {
            let stderr = String::from_utf8_lossy(&done.stderr);
            return Err(format!("{failure}: {}", py::strip(&stderr)));
        }
        Ok(done.stdout)
    };
    let literal = format!(":(literal){path_text}");
    let listing = checked(&["ls-files", "--stage", "-z", "--", &literal])?;
    for entry in listing.split(|byte| *byte == 0) {
        let (meta, listed) = partition(entry, b'\t');
        let (mode, rest) = partition(meta, b' ');
        let (blob, stage) = partition(rest, b' ');
        if listed == path_text.as_bytes() && stage == b"0" && mode.starts_with(b"100") {
            return checked(&["cat-file", "blob", &String::from_utf8_lossy(blob)]).map(Some);
        }
    }
    Ok(None)
}

fn partition(bytes: &[u8], separator: u8) -> (&[u8], &[u8]) {
    match bytes.iter().position(|byte| *byte == separator) {
        Some(at) => (&bytes[..at], &bytes[at + 1..]),
        None => (bytes, &[]),
    }
}

fn too_large(path_text: &str) -> Object {
    let reason = format!("evidence source exceeds {MAX_EVIDENCE_BYTES} bytes");
    verdict(&[
        ("status", "invalid"),
        ("reason", &reason),
        ("path", path_text),
    ])
}

fn verdict(fields: &[(&str, &str)]) -> Object {
    let mut out = Object::new();
    for (key, value) in fields {
        out.insert(*key, Value::Str((*value).to_string()));
    }
    out
}

pub fn shape(reference: &Value) -> Result<(&str, &Object), &'static str> {
    let Value::Object(reference) = reference else {
        return Err("evidence must be an object");
    };
    let (Some(Value::Str(path_text)), Some(Value::Object(lines))) =
        (reference.get("path"), reference.get("lines"))
    else {
        return Err("evidence requires path and lines");
    };
    let start = digits(lines.get("start"));
    let end = digits(lines.get("end"));
    let whole = |value: Option<&Value>| matches!(value, Some(Value::Int(_)));
    if !whole(lines.get("start"))
        || !whole(lines.get("end"))
        || compare(start, "1") == Ordering::Less
        || compare(end, start) == Ordering::Less
    {
        return Err("evidence line range is invalid");
    }
    Ok((path_text, lines))
}

fn digits(value: Option<&Value>) -> &str {
    match value {
        Some(Value::Int(digits)) => digits,
        _ => "0",
    }
}

fn compare(left: &str, right: &str) -> Ordering {
    let negative = |text: &str| text.starts_with('-');
    match (negative(left), negative(right)) {
        (true, false) => Ordering::Less,
        (false, true) => Ordering::Greater,
        (true, true) => magnitude(&right[1..], &left[1..]),
        (false, false) => magnitude(left, right),
    }
}

fn magnitude(left: &str, right: &str) -> Ordering {
    let left = left.trim_start_matches('0');
    let right = right.trim_start_matches('0');
    left.len().cmp(&right.len()).then_with(|| left.cmp(right))
}

fn span(start: &str, end: &str) -> Option<usize> {
    let difference = subtract(end, start)?;
    difference.checked_add(1)
}

fn subtract(larger: &str, smaller: &str) -> Option<usize> {
    let mut result: Vec<u8> = Vec::new();
    let mut borrow = 0;
    let mut left = larger.bytes().rev();
    let mut right = smaller.bytes().rev();
    loop {
        let (a, b) = (left.next(), right.next());
        if a.is_none() && b.is_none() {
            break;
        }
        let a = i32::from(a.map_or(0, |digit| digit - b'0'));
        let b = i32::from(b.map_or(0, |digit| digit - b'0'));
        let mut digit = a - b - borrow;
        borrow = i32::from(digit < 0);
        if digit < 0 {
            digit += 10;
        }
        result.push(u8::try_from(digit).ok()? + b'0');
    }
    result.reverse();
    let trimmed = String::from_utf8(result).ok()?;
    let trimmed = trimmed.trim_start_matches('0');
    if trimmed.is_empty() {
        return Some(0);
    }
    trimmed.parse().ok()
}

fn to_index(digits: &str) -> usize {
    digits.parse().expect("checked against the row count")
}

fn sha256_of(rows: &[String]) -> String {
    format!("sha256:{}", hex_digest(rows.concat().as_bytes()))
}

pub fn rows(content: &[u8]) -> Vec<String> {
    String::from_utf8_lossy(content)
        .split_inclusive('\n')
        .map(str::to_string)
        .collect()
}

pub fn fields_beyond_evidence(
    asset: &str,
    before: &Object,
    after: &Object,
) -> Result<Vec<String>, String> {
    let settled = ["verified", review_field(asset)];
    let stripped = |record: &Object| -> Result<Object, String> {
        let kept = Value::Object(without(record, &settled));
        match with_references(&kept, &mut |_| Ok(Value::Null), 0)? {
            Value::Object(object) => Ok(object),
            _ => Ok(Object::new()),
        }
    };
    let (before, after) = (stripped(before)?, stripped(after)?);
    let mut keys: Vec<String> = Vec::new();
    for (key, _) in before.iter().chain(after.iter()) {
        if !keys.contains(key) {
            keys.push(key.clone());
        }
    }
    keys.retain(|key| match (before.get(key), after.get(key)) {
        (Some(one), Some(other)) => !equal(one, other),
        _ => true,
    });
    keys.sort();
    Ok(keys)
}

#[cfg(test)]
mod tests {
    use super::{MAX_EVIDENCE_BYTES, Verifier, compare, posix_path, span};
    use crate::cite::citation;
    use crate::json::{Object, Value};
    use crate::py;
    use std::cmp::Ordering;

    #[test]
    fn line_numbers_compare_as_whole_numbers_of_any_size() {
        let cases = [
            ("1", "1", Ordering::Equal),
            ("9", "10", Ordering::Less),
            ("-5", "1", Ordering::Less),
            ("-2", "-10", Ordering::Greater),
            (
                "100000000000000000000000000000000000000000",
                "99",
                Ordering::Greater,
            ),
        ];
        for (left, right, expected) in cases {
            assert_eq!(expected, compare(left, right), "{left} vs {right}");
        }
    }

    #[test]
    fn a_span_counts_both_ends_even_beyond_machine_integers() {
        let huge = "100000000000000000000000000000000000000000";
        let next = "100000000000000000000000000000000000000001";
        assert_eq!(
            (Some(1), Some(2), Some(10), None),
            (
                span("7", "7"),
                span(huge, next),
                span("1", "10"),
                span("1", huge)
            )
        );
    }

    #[test]
    fn a_wanted_path_is_normalized_like_a_pure_posix_path() {
        let cases = [
            ("a/b", "a/b"),
            ("./a//b/", "a/b"),
            ("a/./b", "a/b"),
            ("", "."),
            ("/x", "/x"),
            ("//x", "//x"),
            ("///x", "/x"),
        ];
        for (given, expected) in cases {
            assert_eq!(expected, posix_path(given), "{given:?}");
        }
    }

    fn source_of_size(label: &str, size: u64) -> std::path::PathBuf {
        let root = std::env::temp_dir().join(format!("dm-evidence-{label}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(&root).unwrap();
        let mut content = b"one
"
        .to_vec();
        content.resize(size as usize, b'x');
        std::fs::write(root.join("rule.md"), content).unwrap();
        py::resolve_path(&root)
    }

    fn field(verdict: &Object, key: &str) -> Option<String> {
        match verdict.get(key) {
            Some(Value::Str(text)) => Some(text.clone()),
            _ => None,
        }
    }

    fn reference_to_first_line() -> Value {
        let mut lines = Object::new();
        lines.insert("start", Value::Int("1".to_string()));
        lines.insert("end", Value::Int("1".to_string()));
        let mut reference = Object::new();
        reference.insert("path", Value::Str("rule.md".to_string()));
        reference.insert("lines", Value::Object(lines));
        Value::Object(reference)
    }

    #[test]
    fn a_source_of_exactly_the_evidence_size_limit_is_accepted() {
        let root = source_of_size("at", MAX_EVIDENCE_BYTES);
        let reference = citation(&root, "rule.md", "1", "1").unwrap();
        let verdict = Verifier::new(&root, false).check(&Value::Object(reference));
        let _ = std::fs::remove_dir_all(&root);
        assert_eq!(
            field(&verdict.unwrap(), "status"),
            Some("current".to_string())
        );
    }

    #[test]
    fn a_source_one_byte_over_the_evidence_size_limit_is_refused() {
        let root = source_of_size("over", MAX_EVIDENCE_BYTES + 1);
        let verdict = Verifier::new(&root, false)
            .check(&reference_to_first_line())
            .unwrap();
        let _ = std::fs::remove_dir_all(&root);
        assert_eq!(
            (
                field(&verdict, "status"),
                field(&verdict, "reason"),
                field(&verdict, "path")
            ),
            (
                Some("invalid".to_string()),
                Some("evidence source exceeds 16777216 bytes".to_string()),
                Some("rule.md".to_string())
            )
        );
    }
}
