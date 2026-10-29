use crate::pyvalue::{message_text, splitlines, str_of};
use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::path::Path;

use crate::git;
use crate::json::{Object, Value};
use crate::py;
use crate::pyregex::{self, Regex};
use crate::store::Registry;

const COMMIT_MARKER: char = '\x1e';
const SUBJECTS_SHOWN: usize = 10;
const SKIPPED_SHOWN: usize = 20;

pub struct Limits {
    pub min_revisions: i64,
    pub min_shared: i64,
    pub min_degree: i64,
    pub max_changeset: i64,
    pub max_modules: Option<i64>,
}

#[derive(Default)]
pub struct Reading {
    pub since: Option<String>,
    pub exclude_subjects: Vec<String>,
    pub context_paths: Vec<(String, String)>,
    pub collaborations: Vec<(BTreeSet<String>, Vec<String>)>,
}

struct Commit {
    sha: String,
    subject: String,
    files: BTreeSet<String>,
}

pub fn contexts_with_paths(registry: &Registry) -> Result<Vec<(String, String)>, String> {
    let mut found: Vec<(String, String)> = Vec::new();
    for context in registry.records("contexts")? {
        let Value::Object(context) = context else {
            continue;
        };
        let Some(path) = context.get("implementation_path").and_then(Value::as_str) else {
            continue;
        };
        let id = context.get("id").map(str_of).unwrap_or_default();
        match found.iter_mut().find(|(seen, _)| *seen == id) {
            Some(slot) => slot.1 = path.to_string(),
            None => found.push((id, path.to_string())),
        }
    }
    Ok(found)
}

fn ends_of(asset: &str, entry: &Object) -> Vec<(Value, Value)> {
    let field = |key: &str| entry.get(key).cloned().unwrap_or(Value::Null);
    let each = |key: &str| -> Vec<Value> {
        entry
            .get(key)
            .and_then(Value::as_array)
            .map(<[Value]>::to_vec)
            .unwrap_or_default()
    };
    match asset {
        "interactions" => vec![(field("producer_context"), field("consumer_context"))],
        "dependency-policies" => vec![(field("from_context"), field("to_context"))],
        "contracts" => each("consumer_contexts")
            .into_iter()
            .map(|consumer| (field("producer_context"), consumer))
            .collect(),
        _ => each("consumers")
            .into_iter()
            .map(|consumer| (field("owner_context"), consumer))
            .collect(),
    }
}

pub type Collaboration = (BTreeSet<String>, Vec<String>);

pub fn registered_collaborations(registry: &Registry) -> Result<Vec<Collaboration>, String> {
    let mut found: Vec<Collaboration> = Vec::new();
    for asset in ["interactions", "dependency-policies", "contracts", "events"] {
        for entry in registry.records(asset)? {
            let Value::Object(entry) = entry else {
                continue;
            };
            let name = format!(
                "{asset}/{}",
                entry
                    .get("id")
                    .map(message_text)
                    .unwrap_or_else(|| "null".to_string())
            );
            for (one, other) in ends_of(asset, &entry) {
                let (Value::Str(one), Value::Str(other)) = (one, other) else {
                    continue;
                };
                let pair = BTreeSet::from([one, other]);
                match found.iter_mut().find(|(seen, _)| *seen == pair) {
                    Some(slot) => slot.1.push(name.clone()),
                    None => found.push((pair, vec![name.clone()])),
                }
            }
        }
    }
    Ok(found)
}

fn posix_parts(text: &str) -> Vec<&str> {
    text.split('/')
        .filter(|part| !part.is_empty() && *part != ".")
        .collect()
}

pub fn root_parts(root: &str) -> Vec<&str> {
    posix_parts(root.trim_matches('/'))
}

fn under(path: &str, owned: &[Vec<&str>]) -> bool {
    let parts = posix_parts(path);
    owned
        .iter()
        .any(|root| parts.len() >= root.len() && parts[..root.len()] == root[..])
}

fn context_of(module: &str, context_paths: &[(String, String)]) -> Option<String> {
    let parts = posix_parts(module);
    context_paths
        .iter()
        .filter_map(|(context, path)| {
            let owned = root_parts(path);
            if owned.is_empty() || parts.len() < owned.len() {
                return None;
            }
            let found = (0..=parts.len() - owned.len())
                .any(|start| parts[start..start + owned.len()] == owned[..]);
            found.then(|| (owned.len(), context.clone()))
        })
        .max()
        .map(|(_, context)| context)
}

fn module_of(path: &str, roots: &[String], depth: i64) -> Option<String> {
    let parts = posix_parts(path);
    for root in roots {
        let owned = root_parts(root);
        if parts.len() >= owned.len()
            && parts[..owned.len()] == owned[..]
            && parts.len() > owned.len() + 1
        {
            let directories = &parts[owned.len()..parts.len() - 1];
            let kept =
                usize::try_from(depth).map_or(directories.len(), |d| d.min(directories.len()));
            let module: Vec<&str> = owned.iter().chain(&directories[..kept]).copied().collect();
            return Some(module.join("/"));
        }
    }
    None
}

struct ModuleFinder<'a> {
    repo_root: &'a Path,
    roots: &'a [String],
    depth: i64,
    exists: HashMap<String, bool>,
}

impl ModuleFinder<'_> {
    fn module_for(&mut self, path: &str) -> Option<String> {
        let module = module_of(path, self.roots, self.depth)?;
        let repo_root = self.repo_root;
        let present = *self
            .exists
            .entry(module.clone())
            .or_insert_with(|| repo_root.join(&module).is_dir());
        (!module.is_empty() && present).then_some(module)
    }
}

fn commits(repo_root: &Path, since: Option<&str>) -> Result<Vec<Commit>, String> {
    let mut arguments = vec![
        "-c",
        "core.quotepath=false",
        "log",
        "--no-merges",
        "--no-renames",
        "--name-only",
        "--format=%x1e%h %s",
    ];
    let given = since
        .filter(|text| !text.is_empty())
        .map(|text| format!("--since={text}"));
    if let Some(given) = &given {
        arguments.push(given);
    }
    let result = git::checked_git_bytes(repo_root, &arguments, "git could not read the history")?;
    let text = String::from_utf8_lossy(&result.stdout)
        .replace("\r\n", "\n")
        .replace('\r', "\n");
    Ok(text
        .split(COMMIT_MARKER)
        .skip(1)
        .map(commit)
        .filter(|commit| !commit.files.is_empty())
        .collect())
}

fn commit(block: &str) -> Commit {
    let (header, names) = block.split_once('\n').unwrap_or((block, ""));
    let (sha, subject) = header.split_once(' ').unwrap_or((header, ""));
    Commit {
        sha: sha.to_string(),
        subject: subject.to_string(),
        files: splitlines(names)
            .into_iter()
            .filter(|line| !py::strip(line).is_empty())
            .map(str::to_string)
            .collect(),
    }
}

fn subject_filter(patterns: &[String]) -> Result<Vec<Regex>, String> {
    patterns
        .iter()
        .map(|pattern| {
            pyregex::compile(pattern)
                .map_err(|error| format!("--exclude-subject is not a regular expression: {error}"))
        })
        .collect()
}

fn text_value(text: &str) -> Value {
    Value::Str(text.to_string())
}

fn strings(items: &[String]) -> Value {
    Value::Array(items.iter().map(|item| text_value(item)).collect())
}

fn sweep(commit: &Commit, counted: &[String], reading: &Reading) -> Value {
    let mut sweep = Object::new();
    sweep.insert("sha", text_value(&commit.sha));
    sweep.insert("subject", text_value(&commit.subject));
    sweep.insert("modules", strings(counted));
    if !reading.context_paths.is_empty() {
        let contexts: BTreeSet<String> = counted
            .iter()
            .filter_map(|module| context_of(module, &reading.context_paths))
            .collect();
        sweep.insert(
            "contexts",
            Value::Array(contexts.iter().map(|c| text_value(c)).collect()),
        );
    }
    Value::Object(sweep)
}

fn explanations(contexts: &[Option<String>], reading: &Reading) -> Vec<String> {
    let (Some(one), Some(other)) = (&contexts[0], &contexts[1]) else {
        return Vec::new();
    };
    if one == other {
        return vec![format!("contexts/{one}")];
    }
    let pair = BTreeSet::from([one.clone(), other.clone()]);
    let mut found = reading
        .collaborations
        .iter()
        .find(|(seen, _)| *seen == pair)
        .map(|(_, names)| names.clone())
        .unwrap_or_default();
    found.sort();
    found
}

struct Pair<'a> {
    modules: (&'a str, &'a str),
    subjects: &'a [String],
    degree: i64,
}

fn shown_pair(pair: &Pair, reading: &Reading) -> Value {
    let (first, second) = pair.modules;
    let mut shown = Object::new();
    shown.insert("modules", strings(&[first.to_string(), second.to_string()]));
    shown.insert("shared", Value::int(pair.subjects.len()));
    shown.insert("degree", Value::Int(pair.degree.to_string()));
    shown.insert(
        "subjects",
        strings(&pair.subjects[..pair.subjects.len().min(SUBJECTS_SHOWN)]),
    );
    if !reading.context_paths.is_empty() {
        let contexts = [
            context_of(first, &reading.context_paths),
            context_of(second, &reading.context_paths),
        ];
        shown.insert(
            "contexts",
            Value::Array(
                contexts
                    .iter()
                    .map(|c| c.as_deref().map_or(Value::Null, text_value))
                    .collect(),
            ),
        );
        let unmapped: Vec<String> = [first, second]
            .into_iter()
            .zip(&contexts)
            .filter(|(_, context)| context.is_none())
            .map(|(module, _)| module.to_string())
            .collect();
        if unmapped.is_empty() {
            shown.insert("explained_by", strings(&explanations(&contexts, reading)));
        } else {
            shown.insert("explained_by", Value::Null);
            shown.insert("unmapped", strings(&unmapped));
        }
    }
    Value::Object(shown)
}

fn counted(modules: Vec<String>, reading: &Reading) -> Vec<String> {
    if reading.context_paths.is_empty() {
        return modules;
    }
    modules
        .into_iter()
        .filter(|module| {
            context_of(module, &reading.context_paths).is_some_and(|context| !context.is_empty())
        })
        .collect()
}

fn degree(shared: usize, first: usize, second: usize) -> i64 {
    (200.0 * shared as f64 / (first + second) as f64).round_ties_even() as i64
}

pub fn change_coupling(
    repo_root: &Path,
    roots: &[String],
    depth: i64,
    limits: &Limits,
    reading: &Reading,
) -> Result<Value, String> {
    let excluded = subject_filter(&reading.exclude_subjects)?;
    let all = commits(repo_root, reading.since.as_deref())?;
    let wanted: Vec<&Commit> = all
        .iter()
        .filter(|commit| {
            !excluded
                .iter()
                .any(|pattern| pattern.search(&commit.subject))
        })
        .collect();
    let owned: Vec<Vec<&str>> = roots.iter().map(|root| root_parts(root)).collect();
    let under_roots = |commit: &Commit| {
        commit
            .files
            .iter()
            .filter(|path| under(path, &owned))
            .count()
    };
    let (mut kept, mut large) = (Vec::new(), Vec::new());
    for commit in &wanted {
        let inside = under_roots(commit);
        if i64::try_from(inside).is_ok_and(|inside| inside > limits.max_changeset) {
            large.push((*commit, inside));
        } else {
            kept.push(*commit);
        }
    }
    let mut finder = ModuleFinder {
        repo_root,
        roots,
        depth,
        exists: HashMap::new(),
    };
    let mut revisions: BTreeMap<String, usize> = BTreeMap::new();
    let mut shared: Vec<((String, String), Vec<String>)> = Vec::new();
    let mut shared_at: HashMap<(String, String), usize> = HashMap::new();
    let mut sweeping = Vec::new();
    for commit in kept {
        let modules: Vec<String> = commit
            .files
            .iter()
            .filter_map(|path| finder.module_for(path))
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect();
        let counted_modules = counted(modules.clone(), reading);
        if let Some(limit) = limits.max_modules
            && i64::try_from(counted_modules.len()).is_ok_and(|count| count > limit)
        {
            sweeping.push(sweep(commit, &counted_modules, reading));
            continue;
        }
        for module in &modules {
            *revisions.entry(module.clone()).or_insert(0) += 1;
        }
        for (index, first) in modules.iter().enumerate() {
            for second in &modules[index + 1..] {
                let key = (first.clone(), second.clone());
                let slot = *shared_at.entry(key.clone()).or_insert_with(|| {
                    shared.push((key, Vec::new()));
                    shared.len() - 1
                });
                shared[slot].1.push(commit.subject.clone());
            }
        }
    }
    let mut pairs: Vec<Pair> = shared
        .iter()
        .filter_map(|((first, second), subjects)| {
            let (one, other) = (revisions[first], revisions[second]);
            let enough = i64::try_from(subjects.len())
                .is_ok_and(|count| count >= limits.min_shared)
                && i64::try_from(one.min(other)).is_ok_and(|count| count >= limits.min_revisions);
            enough.then(|| Pair {
                modules: (first.as_str(), second.as_str()),
                subjects,
                degree: degree(subjects.len(), one, other),
            })
        })
        .filter(|pair| pair.degree >= limits.min_degree)
        .collect();
    pairs.sort_by(|left, right| {
        (-left.degree, -(left.subjects.len() as i64), left.modules).cmp(&(
            -right.degree,
            -(right.subjects.len() as i64),
            right.modules,
        ))
    });
    let mut report = Object::new();
    report.insert("commits", Value::int(all.len()));
    report.insert("excluded_commits", Value::int(all.len() - wanted.len()));
    report.insert("skipped_large_commits", Value::int(large.len()));
    report.insert(
        "skipped_large",
        Value::Array(
            large
                .iter()
                .take(SKIPPED_SHOWN)
                .map(|(commit, inside)| {
                    let mut row = Object::new();
                    row.insert("sha", text_value(&commit.sha));
                    row.insert("subject", text_value(&commit.subject));
                    row.insert("files", Value::int(commit.files.len()));
                    row.insert("files_under_roots", Value::int(*inside));
                    Value::Object(row)
                })
                .collect(),
        ),
    );
    report.insert("skipped_sweeping_commits", Value::int(sweeping.len()));
    sweeping.truncate(SKIPPED_SHOWN);
    report.insert("skipped_sweeping", Value::Array(sweeping));
    report.insert(
        "modules",
        Value::Array(
            revisions
                .iter()
                .map(|(module, count)| {
                    let mut row = Object::new();
                    row.insert("module", text_value(module));
                    row.insert("revisions", Value::int(*count));
                    Value::Object(row)
                })
                .collect(),
        ),
    );
    report.insert(
        "pairs",
        Value::Array(pairs.iter().map(|pair| shown_pair(pair, reading)).collect()),
    );
    Ok(Value::Object(report))
}
