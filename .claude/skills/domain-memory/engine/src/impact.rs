use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::path::Path;

use crate::coverage::{SourceKinds, all_references};
use crate::evidence::Verifier;
use crate::files::load_json;
use crate::git;
use crate::json::{Object, Value};
use crate::pyvalue::{splitlines, str_of};
use crate::store::{ASSETS, DEFAULT_STATUS, Registry};
use crate::validate::owned_directory;

pub const COLLABORATION_ASSETS: [&str; 4] =
    ["interactions", "dependency-policies", "contracts", "events"];
const UNCHANGED_EXCERPT: [&str; 2] = ["current", "shifted"];
const HUNK_START: &str = "@@ -";

pub type Records = Vec<(&'static str, Vec<Object>)>;
type Hunks = HashMap<String, Vec<(i128, i128)>>;

pub struct Changes {
    pub tracked: BTreeSet<String>,
    pub untracked: BTreeSet<String>,
}

impl Changes {
    pub fn paths(&self) -> Vec<String> {
        self.tracked.union(&self.untracked).cloned().collect()
    }
}

fn git_text(root: &Path, arguments: &[&str], failure: &str) -> Result<String, String> {
    let done = git::checked_git_bytes(root, arguments, failure)?;
    Ok(git::universal_newlines(&done.stdout))
}

fn nul_separated(text: &str) -> BTreeSet<String> {
    text.split('\0')
        .filter(|path| !path.is_empty())
        .map(str::to_string)
        .collect()
}

pub fn changed_files_since(repo_root: &Path, base: &str) -> Result<Changes, String> {
    let failure = format!("git could not list the changes since {base}");
    let changed = git_text(
        repo_root,
        &["diff", "--name-only", "-z", base, "--"],
        &failure,
    )?;
    let untracked = git_text(
        repo_root,
        &["ls-files", "--others", "--exclude-standard", "-z"],
        &failure,
    )?;
    Ok(Changes {
        tracked: nul_separated(&changed),
        untracked: nul_separated(&untracked),
    })
}

fn digits_at(text: &str) -> (&str, &str) {
    let end = text
        .find(|c: char| !c.is_ascii_digit())
        .unwrap_or(text.len());
    text.split_at(end)
}

fn hunk_range(line: &str) -> Option<(i128, i128)> {
    let rest = line.strip_prefix(HUNK_START)?;
    let old_end = rest.find(|c: char| crate::py::is_space(c))?;
    if old_end == 0 {
        return None;
    }
    let rest = rest[old_end..].strip_prefix(" +")?;
    let (start, rest) = digits_at(rest);
    if start.is_empty() {
        return None;
    }
    let (count, rest) = match rest.strip_prefix(',') {
        Some(after) => {
            let (count, rest) = digits_at(after);
            if count.is_empty() {
                return None;
            }
            (Some(count), rest)
        }
        None => (None, rest),
    };
    if !rest.starts_with(" @@") {
        return None;
    }
    let start: i128 = start.parse().ok()?;
    let count: i128 = count.map_or(Some(1), |text| text.parse().ok())?;
    Some((start, start + count - 1))
}

fn hunks(repo_root: &Path, base: &str, paths: &[String]) -> Result<Hunks, String> {
    let mut found = Hunks::new();
    if paths.is_empty() {
        return Ok(found);
    }
    let mut arguments = vec![
        "--literal-pathspecs",
        "-c",
        "core.quotepath=false",
        "diff",
        "-U0",
        "--no-color",
        "--no-ext-diff",
        base,
        "--",
    ];
    arguments.extend(paths.iter().map(String::as_str));
    let failure = format!("git could not read the changes since {base}");
    let output = git_text(repo_root, &arguments, &failure)?;
    let mut current: Option<&str> = None;
    for line in splitlines(&output) {
        if line.starts_with("+++ ") {
            current = line.strip_prefix("+++ b/");
        } else if let (Some(path), Some(range)) = (current, hunk_range(line)) {
            found.entry(path.to_string()).or_default().push(range);
        }
    }
    Ok(found)
}

fn whole_number(value: Option<&Value>) -> Option<i128> {
    match value {
        Some(Value::Int(digits)) => digits.parse().ok(),
        _ => None,
    }
}

pub fn cited_ranges(entry: &Object) -> Result<Vec<(String, i128, i128)>, String> {
    let mut found = Vec::new();
    for reference in all_references(&Value::Object(entry.clone()), 0)? {
        let Some(reference) = reference.as_object() else {
            continue;
        };
        let (Some(Value::Str(path)), Some(Value::Object(lines))) =
            (reference.get("path"), reference.get("lines"))
        else {
            continue;
        };
        if let (Some(start), Some(end)) = (
            whole_number(lines.get("start")),
            whole_number(lines.get("end")),
        ) {
            found.push((path.clone(), start, end));
        }
    }
    Ok(found)
}

pub struct Library {
    records: Vec<(&'static str, Vec<Object>)>,
}

impl Library {
    pub fn read(registry: &Registry) -> Result<Self, String> {
        let mut records: Vec<(&'static str, Vec<Object>)> = Vec::new();
        let first = ["contexts"].into_iter().chain(COLLABORATION_ASSETS);
        for asset in first.chain(ASSETS.iter().map(|(asset, _, _)| *asset)) {
            if records.iter().any(|(held, _)| *held == asset) {
                continue;
            }
            let entries = registry
                .records(asset)?
                .into_iter()
                .filter_map(|record| match record {
                    Value::Object(entry) => Some(entry),
                    _ => None,
                })
                .collect();
            records.push((asset, entries));
        }
        records.sort_by_key(|(held, _)| ASSETS.iter().position(|(name, _, _)| name == held));
        Ok(Self { records })
    }

    pub fn raw(&self, asset: &str) -> &[Object] {
        self.records
            .iter()
            .find(|(held, _)| *held == asset)
            .map_or(&[], |(_, entries)| entries)
    }

    pub fn in_order(&self) -> &[(&'static str, Vec<Object>)] {
        &self.records
    }

    pub fn with_status(&self, asset: &str) -> Vec<Object> {
        self.raw(asset)
            .iter()
            .map(|entry| {
                let mut entry = entry.clone();
                if entry.get("status").is_none() {
                    entry.insert("status", Value::Str(DEFAULT_STATUS.to_string()));
                }
                entry
            })
            .collect()
    }

    fn every_with_status(&self) -> Records {
        self.records
            .iter()
            .map(|(asset, _)| (*asset, self.with_status(asset)))
            .collect()
    }

    fn collaborations_with_status(&self) -> Records {
        COLLABORATION_ASSETS
            .iter()
            .map(|asset| (*asset, self.with_status(asset)))
            .collect()
    }
}

fn field(entry: &Object, key: &str) -> Value {
    entry.get(key).cloned().unwrap_or(Value::Null)
}

fn strings(entries: impl IntoIterator<Item = String>) -> Value {
    Value::Array(entries.into_iter().map(Value::Str).collect())
}

fn cited_paths_of(records: &Records) -> Result<BTreeSet<String>, String> {
    let mut cited = BTreeSet::new();
    for (_, entries) in records {
        for entry in entries {
            cited.extend(cited_ranges(entry)?.into_iter().map(|(path, _, _)| path));
        }
    }
    Ok(cited)
}

fn hunks_in(
    repo_root: &Path,
    base: &str,
    cited: &BTreeSet<String>,
    changes: &Changes,
) -> Result<Hunks, String> {
    let tracked: Vec<String> = cited.intersection(&changes.tracked).cloned().collect();
    let mut found = hunks(repo_root, base, &tracked)?;
    for path in cited.intersection(&changes.untracked) {
        found.insert(path.clone(), vec![(1, i128::MAX)]);
    }
    Ok(found)
}

fn evidence_overlapping(records: &Records, hunks: &Hunks) -> Result<Vec<Value>, String> {
    let mut touched = Vec::new();
    for (asset, entries) in records {
        for entry in entries {
            let mut overlapping = BTreeSet::new();
            for (path, start, end) in cited_ranges(entry)? {
                let touches = hunks.get(&path).is_some_and(|ranges| {
                    ranges
                        .iter()
                        .any(|(first, last)| start <= *last && *first <= end)
                });
                if touches {
                    overlapping.insert(format!("{path}:{start}-{end}"));
                }
            }
            if !overlapping.is_empty() {
                let mut row = Object::new();
                row.insert("asset", Value::Str((*asset).to_string()));
                row.insert("id", field(entry, "id"));
                row.insert("status", field(entry, "status"));
                row.insert("touched_evidence", strings(overlapping));
                touched.push(Value::Object(row));
            }
        }
    }
    Ok(touched)
}

pub fn touched_records(
    library: &Library,
    repo_root: &Path,
    base: &str,
    changes: &Changes,
) -> Result<Vec<Value>, String> {
    let records = library.every_with_status();
    let cited = cited_paths_of(&records)?;
    let found = hunks_in(repo_root, base, &cited, changes)?;
    evidence_overlapping(&records, &found)
}

pub fn directory_of(path: &str) -> String {
    format!(
        "/{}/",
        path.rsplit_once('/').map_or("", |(parent, _)| parent)
    )
}

fn touched_contexts(contexts: &[Object], paths: &[String]) -> Vec<(String, Vec<String>)> {
    let directories: Vec<String> = paths.iter().map(|path| directory_of(path)).collect();
    contexts
        .iter()
        .filter_map(|context| {
            let Some(Value::Str(placed)) = context.get("implementation_path") else {
                return None;
            };
            let owned = owned_directory(placed);
            let files: Vec<String> = paths
                .iter()
                .zip(&directories)
                .filter(|(_, directory)| directory.contains(&owned))
                .map(|(path, _)| path.clone())
                .collect();
            (!files.is_empty()).then(|| (str_of(&field(context, "id")), files))
        })
        .collect()
}

fn text_ends(entry: &Object, key: &str) -> Vec<Value> {
    entry
        .get(key)
        .and_then(Value::as_array)
        .map(<[Value]>::to_vec)
        .unwrap_or_default()
}

fn ends_of(asset: &str, entry: &Object) -> Vec<(Value, Value)> {
    match asset {
        "interactions" => vec![(
            field(entry, "producer_context"),
            field(entry, "consumer_context"),
        )],
        "dependency-policies" => vec![(field(entry, "from_context"), field(entry, "to_context"))],
        "contracts" => text_ends(entry, "consumer_contexts")
            .into_iter()
            .map(|consumer| (field(entry, "producer_context"), consumer))
            .collect(),
        _ => text_ends(entry, "consumers")
            .into_iter()
            .map(|consumer| (field(entry, "owner_context"), consumer))
            .collect(),
    }
}

type Collaborations = BTreeMap<BTreeSet<String>, Vec<Value>>;

fn collaborations_of(records: &Records) -> Collaborations {
    let mut found = Collaborations::new();
    for (asset, entries) in records {
        for entry in entries {
            for (one, other) in ends_of(asset, entry) {
                let (Value::Str(one), Value::Str(other)) = (one, other) else {
                    continue;
                };
                let mut row = Object::new();
                row.insert("asset", Value::Str((*asset).to_string()));
                row.insert("id", field(entry, "id"));
                row.insert("status", field(entry, "status"));
                found
                    .entry(BTreeSet::from([one, other]))
                    .or_default()
                    .push(Value::Object(row));
            }
        }
    }
    found
}

fn changed_evidence(
    records: &Records,
    paths: &BTreeSet<String>,
    verifier: &mut Verifier,
) -> Result<Vec<Value>, String> {
    let mut touched = Vec::new();
    for (asset, entries) in records {
        for entry in entries {
            let mut changed = BTreeSet::new();
            for reference in all_references(&Value::Object(entry.clone()), 0)? {
                let cited = match &reference {
                    Value::Object(object) => object.get("path").cloned(),
                    other => Some(other.clone()),
                };
                let Some(Value::Str(cited)) = cited else {
                    continue;
                };
                if !paths.contains(&cited) {
                    continue;
                }
                if reference.as_object().is_some() {
                    let verdict = verifier.check(&reference)?;
                    let status = verdict.get("status").and_then(Value::as_str);
                    if status.is_some_and(|status| UNCHANGED_EXCERPT.contains(&status)) {
                        continue;
                    }
                }
                changed.insert(cited);
            }
            if !changed.is_empty() {
                let mut row = Object::new();
                row.insert("asset", Value::Str((*asset).to_string()));
                row.insert("id", field(entry, "id"));
                row.insert("status", field(entry, "status"));
                row.insert("changed_evidence", strings(changed));
                touched.push(Value::Object(row));
            }
        }
    }
    Ok(touched)
}

fn verdict_of(pairs: &[Value], evidence: &[Value], mapped: bool) -> &'static str {
    let unregistered = pairs.iter().any(|pair| {
        pair.as_object()
            .and_then(|pair| pair.get("collaborations"))
            .and_then(Value::as_array)
            .is_some_and(<[Value]>::is_empty)
    });
    if unregistered {
        "unregistered-co-change"
    } else if !evidence.is_empty() {
        "boundary-evidence-changed"
    } else if !pairs.is_empty() {
        "registered-co-change"
    } else if mapped {
        "no-cross-context-change"
    } else {
        "unknown"
    }
}

fn cited_outside(
    library: &Library,
    root: &Path,
    paths: &BTreeSet<String>,
) -> Result<Value, String> {
    let mut citing: BTreeMap<String, BTreeSet<String>> = BTreeMap::new();
    for (asset, entries) in library.every_with_status() {
        for entry in &entries {
            for reference in all_references(&Value::Object(entry.clone()), 0)? {
                let cited = match &reference {
                    Value::Object(object) => object.get("path").cloned(),
                    other => Some(other.clone()),
                };
                if let Some(Value::Str(cited)) = cited
                    && paths.contains(&cited)
                {
                    citing
                        .entry(cited)
                        .or_default()
                        .insert(format!("{asset}/{}", str_of(&field(entry, "id"))));
                }
            }
        }
    }
    if citing.is_empty() {
        return Ok(Value::Array(Vec::new()));
    }
    let source_map_path = root.join("source-map.json");
    let source_map = if source_map_path.is_file() {
        load_json(&source_map_path)?
    } else {
        Object::new()
    };
    let kinds = SourceKinds::of(&source_map);
    Ok(Value::Array(
        citing
            .into_iter()
            .map(|(path, records)| {
                let mut probe = Object::new();
                probe.insert("path", Value::Str(path.clone()));
                let classified = kinds.classify(&Value::Object(probe));
                let mut row = Object::new();
                row.insert("path", Value::Str(path));
                row.insert("cited_by", strings(records));
                row.insert(
                    "cited_as",
                    classified
                        .as_object()
                        .and_then(|found| found.get("source_kind"))
                        .cloned()
                        .unwrap_or(Value::Null),
                );
                Value::Object(row)
            })
            .collect(),
    ))
}

fn pair_row(one: &str, other: &str, collaborations: &Collaborations) -> Value {
    let key = BTreeSet::from([one.to_string(), other.to_string()]);
    let mut row = Object::new();
    row.insert("contexts", strings([one.to_string(), other.to_string()]));
    row.insert(
        "collaborations",
        Value::Array(collaborations.get(&key).cloned().unwrap_or_default()),
    );
    Value::Object(row)
}

pub fn boundary_impact(
    library: &Library,
    root: &Path,
    repo_root: &Path,
    base: &str,
    changes: &Changes,
) -> Result<Object, String> {
    let paths = changes.paths();
    let contexts = library.with_status("contexts");
    let mapped = contexts
        .iter()
        .any(|context| matches!(context.get("implementation_path"), Some(Value::Str(_))));
    let touched = touched_contexts(&contexts, &paths);
    let records = library.collaborations_with_status();
    let collaborations = collaborations_of(&records);
    let mut names: Vec<&str> = touched.iter().map(|(id, _)| id.as_str()).collect();
    names.sort_unstable();
    let mut pairs = Vec::new();
    for (index, one) in names.iter().enumerate() {
        for other in &names[index + 1..] {
            pairs.push(pair_row(one, other, &collaborations));
        }
    }
    let mut verifier = Verifier::new(repo_root, false);
    let changed: BTreeSet<String> = paths.iter().cloned().collect();
    let evidence = changed_evidence(&records, &changed, &mut verifier)?;
    let cited = cited_paths_of(&records)?;
    let found = hunks_in(repo_root, base, &cited, changes)?;
    let owned: BTreeSet<&String> = touched.iter().flat_map(|(_, files)| files).collect();
    let outside: Vec<String> = paths
        .iter()
        .filter(|path| !owned.contains(path))
        .cloned()
        .collect();
    let mut contexts_touched = Object::new();
    for (id, files) in &touched {
        contexts_touched.insert(id.clone(), strings(files.clone()));
    }
    let verdict = verdict_of(&pairs, &evidence, mapped);
    let mut impact = Object::new();
    impact.insert("base", Value::Str(base.to_string()));
    impact.insert("changed_files", Value::int(paths.len()));
    impact.insert("mapped_files", Value::int(owned.len()));
    impact.insert("verdict", Value::Str(verdict.to_string()));
    impact.insert("contexts_touched", Value::Object(contexts_touched));
    impact.insert("co_changed_pairs", Value::Array(pairs));
    impact.insert("boundary_evidence_changed", Value::Array(evidence));
    impact.insert(
        "boundary_evidence_touched",
        Value::Array(evidence_overlapping(&records, &found)?),
    );
    impact.insert("outside_every_context", strings(outside.clone()));
    impact.insert(
        "cited_outside_contexts",
        cited_outside(library, root, &outside.into_iter().collect())?,
    );
    Ok(impact)
}
