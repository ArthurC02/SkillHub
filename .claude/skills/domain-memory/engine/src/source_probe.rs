use std::cmp::Ordering;
use std::collections::{BTreeSet, HashMap, HashSet};
use std::path::{Path, PathBuf};

use crate::files::{load_json, one_level_too_deep_hint};
use crate::git::{self, Output};
use crate::json::{Object, Value};
use crate::okf::truthy;
use crate::policy::validate_policy;
use crate::py;
use crate::pyvalue::{equal, str_of};
use crate::sha256::hex_digest;
use crate::store::{self, holds_domain_memory};

pub const EXCLUDED_DIRECTORIES: [&str; 9] = [
    ".git",
    ".hg",
    ".svn",
    "node_modules",
    ".venv",
    "vendor",
    "dist",
    "build",
    "generated",
];
const CI_MARKERS: [(&str, &str); 6] = [
    (".github/workflows", "github-actions"),
    (".gitlab-ci.yml", "gitlab-ci"),
    ("Jenkinsfile", "jenkins"),
    (".circleci/config.yml", "circleci"),
    (".buildkite/pipeline.yml", "buildkite"),
    ("azure-pipelines.yml", "azure-pipelines"),
];
const INSTRUCTION_FILE_NAME: &str = "AGENTS.md";
const DOMAIN_MEMORY_MARKERS: [&str; 2] =
    [":(glob)**/source-map.json", ":(glob)**/registry/index.md"];
const SOURCE_MAP_FORMAT: &str = "domain-source-map/v1";
const DEFAULT_SELECTION_STATUS: &str = "agent-asserted";
const HINT_SEPARATOR: &str = "; ";
const DOMAIN_MEMORY_HINT: &str = "check --registry-root (a Registry holds registry/index.md, \
     or a domain-source-map/v1 source-map.json); initialize one only if this repository has none yet";

pub enum Fail {
    Value(String),
    Os(String),
}

impl From<String> for Fail {
    fn from(message: String) -> Self {
        Fail::Value(message)
    }
}

impl Fail {
    pub fn text(self) -> String {
        match self {
            Fail::Value(message) | Fail::Os(message) => message,
        }
    }
}

pub type Res<T> = Result<T, Fail>;

pub struct Profile {
    pub implementation_roots: Vec<String>,
    pub excluded_directories: Vec<String>,
    pub decision_roots: Vec<String>,
    pub boundary_markers: Vec<String>,
}

pub struct Discovered {
    pub groups: Vec<(&'static str, Vec<String>)>,
    pub ci_tools: Vec<String>,
}

pub struct Files {
    pub names: Vec<String>,
    pub pruned_by_git: bool,
}

pub struct Listing {
    root: PathBuf,
    excluded: BTreeSet<String>,
    files: Option<Files>,
}

pub struct Verdict {
    pub status: String,
    pub selection_status: Value,
    pub reason: Option<String>,
    pub hint: Option<String>,
    pub changed_sources: Vec<String>,
    pub content_changed: Vec<String>,
    pub checked: Option<&'static str>,
    pub gone: Vec<String>,
    pub unlisted: Vec<String>,
}

fn verdict(status: &str, selection_status: &Value, reason: Option<String>) -> Verdict {
    Verdict {
        status: status.to_string(),
        selection_status: selection_status.clone(),
        reason,
        hint: None,
        changed_sources: Vec::new(),
        content_changed: Vec::new(),
        checked: None,
        gone: Vec::new(),
        unlisted: Vec::new(),
    }
}

pub fn os_failure(error: &std::io::Error, path: &Path) -> Fail {
    Fail::Os(py::os_error(error, path))
}

fn live(policy: Option<&Object>) -> Option<&Object> {
    policy.filter(|found| found.iter().next().is_some())
}

fn strings_of(value: Option<&Value>) -> Option<Vec<String>> {
    value?
        .as_array()?
        .iter()
        .map(|item| item.as_str().map(str::to_string))
        .collect()
}

pub fn policy_path(root: &Path) -> PathBuf {
    root.join("domain-memory-policy.json")
}

pub fn existing_policy(root: &Path) -> Result<Option<Object>, String> {
    let path = policy_path(root);
    if path.is_file() {
        return load_json(&path).map(Some);
    }
    Ok(None)
}

pub fn project_profile(policy: Option<&Object>) -> Profile {
    let section = live(policy)
        .and_then(|found| found.get("project_profile"))
        .and_then(Value::as_object);
    let pick = |key: &str, default: &[&str]| -> Vec<String> {
        match section.and_then(|section| section.get(key)) {
            Some(Value::Array(items)) => items
                .iter()
                .filter_map(|item| item.as_str().map(str::to_string))
                .collect(),
            _ => default.iter().map(|item| (*item).to_string()).collect(),
        }
    };
    Profile {
        implementation_roots: pick(
            "implementation_roots",
            &[
                "apps",
                "src",
                "services",
                "packages",
                "db",
                "database",
                "migrations",
                "schema",
            ],
        ),
        excluded_directories: pick("excluded_directories", &[]),
        decision_roots: pick(
            "decision_roots",
            &["docs/adr", "adr", "docs/architecture", "architecture"],
        ),
        boundary_markers: pick("boundary_markers", &["doc.go"]),
    }
}

pub fn excluded_directories(policy: Option<&Object>) -> BTreeSet<String> {
    EXCLUDED_DIRECTORIES
        .iter()
        .map(|name| (*name).to_string())
        .chain(project_profile(policy).excluded_directories)
        .collect()
}

pub fn shown_path(path: &Path, top: Option<&Path>) -> String {
    let resolved = py::resolve_path(path);
    if let Some(top) = top
        && let Ok(inside) = py::relative_to(&resolved, &py::resolve_path(top))
    {
        return if inside.is_empty() {
            ".".to_string()
        } else {
            inside
        };
    }
    store::posix(&resolved)
}

pub fn given_posix(text: &str) -> String {
    let text = if cfg!(windows) {
        text.replace('\\', "/")
    } else {
        text.to_string()
    };
    let (drive, rest) = match text.as_bytes() {
        [letter, b':', ..] if cfg!(windows) && letter.is_ascii_alphabetic() => text.split_at(2),
        _ => ("", text.as_str()),
    };
    let rooted = rest.starts_with('/');
    let parts: Vec<&str> = rest
        .split('/')
        .filter(|part| !part.is_empty() && *part != ".")
        .collect();
    let double_root = !cfg!(windows) && rest.starts_with("//") && !rest.starts_with("///");
    let root = match (rooted, double_root) {
        (_, true) => "//",
        (true, _) => "/",
        _ => "",
    };
    let joined = format!("{drive}{root}{}", parts.join("/"));
    if joined.is_empty() {
        ".".to_string()
    } else {
        joined
    }
}

pub fn joined_given(base: &str, relative: &str) -> String {
    given_posix(&PathBuf::from(base).join(relative).to_string_lossy())
}

pub fn shown_argument(given: &str) -> String {
    let text = given_posix(given);
    if text.contains(' ') {
        format!("\"{text}\"")
    } else {
        text
    }
}

pub fn counted(count: usize, noun: &str) -> String {
    format!("{count} {noun}{}", if count == 1 { "" } else { "s" })
}

pub fn refresh_listing_advice(registry_root: &str, repo_root: &str) -> String {
    format!(
        "run refresh-listing --registry-root {} --repo-root {} (no confirmation needed)",
        shown_argument(registry_root),
        shown_argument(repo_root)
    )
}

fn path_key(path: &Path) -> Vec<String> {
    let text = py::shown(path);
    let text = if cfg!(windows) {
        text.to_lowercase()
    } else {
        text
    };
    text.split(std::path::MAIN_SEPARATOR)
        .map(str::to_string)
        .collect()
}

pub fn path_order(left: &Path, right: &Path) -> Ordering {
    path_key(left).cmp(&path_key(right))
}

fn same_path(left: &Path, right: &Path) -> bool {
    py::compare_paths(left, right) == Ordering::Equal
}

pub fn sorted_paths(mut paths: Vec<PathBuf>) -> Vec<PathBuf> {
    paths.sort_by(|a, b| path_order(a, b));
    let mut seen: Vec<PathBuf> = Vec::new();
    for path in paths {
        if !seen.last().is_some_and(|last| same_path(last, &path)) {
            seen.push(path);
        }
    }
    seen
}

fn lossy(bytes: &[u8]) -> String {
    String::from_utf8_lossy(bytes)
        .replace("\r\n", "\n")
        .replace('\r', "\n")
}

fn run_lenient(
    root: &Path,
    arguments: &[&str],
    input: Option<&[u8]>,
) -> Res<Result<Output, String>> {
    git::run_git_lenient(root, arguments, input).map_err(Fail::Value)
}

fn run_strict(root: &Path, arguments: &[&str], input: Option<&[u8]>) -> Res<Output> {
    run_lenient(root, arguments, input)?.map_err(Fail::Os)
}

pub fn relative(root: &Path, path: &Path) -> Result<String, String> {
    let prefix = format!("{}{}", py::shown(root), std::path::MAIN_SEPARATOR);
    let text = py::shown(path);
    if let Some(rest) = text.strip_prefix(&prefix) {
        return Ok(rest.replace(std::path::MAIN_SEPARATOR, "/"));
    }
    let inside = py::relative_to(path, root)
        .or_else(|_| py::relative_to(&py::resolve_path(path), &py::resolve_path(root)))?;
    Ok(if inside.is_empty() {
        ".".to_string()
    } else {
        inside
    })
}

fn posix_parts(text: &str) -> Vec<&str> {
    text.split('/')
        .filter(|part| !part.is_empty() && *part != ".")
        .collect()
}

fn in_excluded_directory(relative_path: &str, excluded: &BTreeSet<String>) -> bool {
    posix_parts(relative_path)
        .iter()
        .any(|part| excluded.contains(*part))
}

fn fnmatch_case(name: &[char], pattern: &[char]) -> bool {
    let Some((head, rest)) = pattern.split_first() else {
        return name.is_empty();
    };
    match head {
        '*' => (0..=name.len()).any(|skip| fnmatch_case(&name[skip..], rest)),
        '?' => !name.is_empty() && fnmatch_case(&name[1..], rest),
        '[' => match class_end(pattern) {
            Some(end) => {
                !name.is_empty()
                    && class_holds(&pattern[1..end], name[0])
                    && fnmatch_case(&name[1..], &pattern[end + 1..])
            }
            None => name.first() == Some(&'[') && fnmatch_case(&name[1..], rest),
        },
        literal => name.first() == Some(literal) && fnmatch_case(&name[1..], rest),
    }
}

fn class_end(pattern: &[char]) -> Option<usize> {
    let mut at = 1;
    if pattern.get(at) == Some(&'!') {
        at += 1;
    }
    if pattern.get(at) == Some(&']') {
        at += 1;
    }
    (at..pattern.len()).find(|index| pattern[*index] == ']')
}

fn class_holds(stuff: &[char], character: char) -> bool {
    let (negated, members) = match stuff.split_first() {
        Some(('!', rest)) => (true, rest),
        _ => (false, stuff),
    };
    let mut found = false;
    let mut at = 0;
    while at < members.len() {
        if at + 2 < members.len() && members[at + 1] == '-' {
            found |= (members[at]..=members[at + 2]).contains(&character);
            at += 3;
        } else {
            found |= members[at] == character;
            at += 1;
        }
    }
    found != negated
}

fn policy_matches(path: &str, include: &[String], exclude: &[String]) -> bool {
    let name: Vec<char> = path.chars().collect();
    let hits = |pattern: &String| fnmatch_case(&name, &pattern.chars().collect::<Vec<_>>());
    include
        .iter()
        .any(|pattern| pattern == "**" || hits(pattern))
        && !exclude.iter().any(hits)
}

fn source_policy_patterns(policy: Option<&Object>) -> (Vec<String>, Vec<String>) {
    let section = live(policy)
        .and_then(|found| found.get("source_policy"))
        .and_then(Value::as_object);
    let listed = |key: &str, default: &[&str]| match section.and_then(|section| section.get(key)) {
        Some(found) => strings_of(Some(found)).unwrap_or_default(),
        None => default.iter().map(|item| (*item).to_string()).collect(),
    };
    (listed("include", &["**"]), listed("exclude", &[]))
}

fn regular_files_under(directory: &Path, excluded: &BTreeSet<String>) -> Vec<PathBuf> {
    let mut found = Vec::new();
    let mut pending = vec![directory.to_path_buf()];
    while let Some(current) = pending.pop() {
        let Ok(entries) = std::fs::read_dir(&current) else {
            continue;
        };
        for entry in entries.flatten() {
            let Ok(kind) = entry.file_type() else {
                continue;
            };
            if kind.is_dir() {
                if !excluded.contains(entry.file_name().to_string_lossy().as_ref()) {
                    pending.push(entry.path());
                }
            } else if kind.is_file() {
                found.push(entry.path());
            }
        }
    }
    found
}

fn regular_files(selected: &Path, excluded: &BTreeSet<String>) -> Vec<PathBuf> {
    if selected.is_file() {
        let linked =
            std::fs::symlink_metadata(selected).is_ok_and(|meta| meta.file_type().is_symlink());
        return if linked {
            Vec::new()
        } else {
            vec![selected.to_path_buf()]
        };
    }
    regular_files_under(selected, excluded)
}

fn ignored_relatives(root: &Path, relatives: &[String]) -> Res<HashSet<String>> {
    if relatives.is_empty() || !root.join(".git").exists() {
        return Ok(HashSet::new());
    }
    let input = relatives.join("\0");
    let result = run_strict(
        root,
        &["check-ignore", "-z", "--stdin"],
        Some(input.as_bytes()),
    )?;
    if result.code != 0 && result.code != 1 {
        return Err(Fail::Value(format!(
            "git check-ignore failed in {}, so ignored files cannot be told from sources: {}",
            py::shown(root),
            py::strip(&String::from_utf8_lossy(&result.stderr))
        )));
    }
    let asked: HashSet<&String> = relatives.iter().collect();
    Ok(String::from_utf8_lossy(&result.stdout)
        .split('\0')
        .map(str::to_string)
        .filter(|entry| asked.contains(entry))
        .collect())
}

fn git_ignored(root: &Path, candidates: &[PathBuf]) -> Res<HashSet<String>> {
    let mut by_relative: HashMap<String, String> = HashMap::new();
    let mut order: Vec<String> = Vec::new();
    for path in candidates {
        let name = relative(root, path)?;
        if by_relative.insert(name.clone(), py::shown(path)).is_none() {
            order.push(name);
        }
    }
    let ignored = ignored_relatives(root, &order)?;
    Ok(ignored
        .iter()
        .filter_map(|name| by_relative.get(name).cloned())
        .collect())
}

pub fn source_files(
    root: &Path,
    selected: &[PathBuf],
    policy: Option<&Object>,
) -> Res<Vec<PathBuf>> {
    let (include, exclude) = source_policy_patterns(policy);
    let excluded = excluded_directories(policy);
    let mut roots: Vec<PathBuf> = Vec::new();
    for path in selected {
        let resolved = py::resolve_path(path);
        let covered = roots.iter().any(|parent| {
            parent.is_dir()
                && resolved
                    .ancestors()
                    .any(|ancestor| same_path(ancestor, parent))
        });
        if !covered {
            roots.push(resolved);
        }
    }
    let mut files: Vec<PathBuf> = Vec::new();
    for resolved in &roots {
        for candidate in regular_files(resolved, &excluded) {
            let relative_path = relative(root, &candidate)?;
            if !in_excluded_directory(&relative_path, &excluded)
                && policy_matches(&relative_path, &include, &exclude)
            {
                files.push(candidate);
            }
        }
    }
    let files = sorted_paths(files);
    let ignored = git_ignored(root, &files)?;
    Ok(files
        .into_iter()
        .filter(|file| !ignored.contains(&py::shown(file)))
        .collect())
}

fn limit_of(policy: &Object, key: &str) -> Res<Value> {
    policy
        .get("limits")
        .and_then(Value::as_object)
        .and_then(|limits| limits.get(key))
        .cloned()
        .ok_or_else(|| Fail::Value(format!("'{key}'")))
}

fn over(value: u128, limit: &Value) -> bool {
    match limit {
        Value::Int(digits) => digits
            .parse::<u128>()
            .map_or(digits.starts_with('-'), |held| value > held),
        _ => false,
    }
}

pub fn policy_report_errors(root: &Path, files: &[PathBuf], policy: &Object) -> Res<Vec<String>> {
    let (max_count, max_total, max_file) = (
        limit_of(policy, "max_file_count")?,
        limit_of(policy, "max_total_bytes")?,
        limit_of(policy, "max_file_bytes")?,
    );
    let mut total: u128 = 0;
    let mut oversized: Vec<String> = Vec::new();
    for file in files {
        let size = u128::from(
            std::fs::metadata(file)
                .map_err(|error| os_failure(&error, file))?
                .len(),
        );
        total += size;
        if over(size, &max_file) {
            oversized.push(relative(root, file)?);
        }
    }
    let mut errors = Vec::new();
    if over(files.len() as u128, &max_count) {
        errors.push(format!(
            "selected sources hold {} files, over max_file_count {}",
            files.len(),
            str_of(&max_count)
        ));
    }
    if over(total, &max_total) {
        errors.push(format!(
            "selected sources hold {total} bytes, over max_total_bytes {}",
            str_of(&max_total)
        ));
    }
    if !oversized.is_empty() {
        let named = oversized
            .iter()
            .take(5)
            .cloned()
            .collect::<Vec<_>>()
            .join(", ");
        let remainder = if oversized.len() <= 5 {
            String::new()
        } else {
            format!(", and {} more", oversized.len() - 5)
        };
        errors.push(format!(
            "over max_file_bytes {}: {named}{remainder}",
            str_of(&max_file)
        ));
    }
    Ok(errors)
}

fn owned_by_selection(root: &Path, selected: &[String], files: &[PathBuf]) -> Vec<Vec<PathBuf>> {
    let resolved: Vec<(&String, PathBuf, bool)> = selected
        .iter()
        .map(|path| {
            let joined = root.join(path);
            (path, py::resolve_path(&joined), joined.is_dir())
        })
        .collect();
    let mut owner: HashMap<String, &String> = HashMap::new();
    for file in files {
        let parents: Vec<&Path> = file
            .parent()
            .map(|dir| dir.ancestors().collect())
            .unwrap_or_default();
        for (path, selected_path, is_directory) in &resolved {
            let held = owner
                .get(&py::shown(file))
                .map_or(0, |owned| owned.chars().count());
            let hit = same_path(file, selected_path)
                || (*is_directory && parents.iter().any(|dir| same_path(dir, selected_path)));
            if hit && path.chars().count() > held {
                owner.insert(py::shown(file), path);
            }
        }
    }
    selected
        .iter()
        .map(|path| {
            files
                .iter()
                .filter(|file| owner.get(&py::shown(file)) == Some(&path))
                .cloned()
                .collect()
        })
        .collect()
}

fn snapshot_of(root: &Path, path: &str, files: &[PathBuf]) -> Res<Object> {
    let mut digest: Vec<u8> = Vec::new();
    let mut listing: Vec<u8> = Vec::new();
    for file in files {
        let name = relative(root, file)?;
        listing.extend(name.as_bytes());
        listing.push(0);
        digest.extend(name.as_bytes());
        digest.push(0);
        digest.extend(std::fs::read(file).map_err(|error| os_failure(&error, file))?);
        digest.push(0);
    }
    let mut snapshot = Object::new();
    snapshot.insert("path", Value::Str(path.to_string()));
    snapshot.insert("file_count", Value::int(files.len()));
    snapshot.insert(
        "digest",
        Value::Str(format!("sha256:{}", hex_digest(&digest))),
    );
    snapshot.insert(
        "listing_sha256",
        Value::Str(format!("sha256:{}", hex_digest(&listing))),
    );
    Ok(snapshot)
}

fn source_snapshots(root: &Path, selected: &[String], policy: Option<&Object>) -> Res<Vec<Object>> {
    let joined: Vec<PathBuf> = selected.iter().map(|path| root.join(path)).collect();
    let files = source_files(root, &joined, policy)?;
    let owned = owned_by_selection(root, selected, &files);
    selected
        .iter()
        .zip(&owned)
        .map(|(path, files)| snapshot_of(root, path, files))
        .collect()
}

fn selection_moved(recorded: &Object, current: &Object) -> bool {
    let field = |object: &Object, key: &str| object.get(key).cloned().unwrap_or(Value::Null);
    let listing = recorded.get("listing_sha256").and_then(Value::as_str);
    !equal(
        &field(recorded, "file_count"),
        &field(current, "file_count"),
    ) || listing
        .is_some_and(|held| Some(held) != current.get("listing_sha256").and_then(Value::as_str))
}

fn snapshot_problem(snapshots: Option<&Value>, selected: Option<&Value>) -> Option<&'static str> {
    let Some(Value::Array(snapshots)) = snapshots else {
        return Some("source map has no snapshots");
    };
    let paths: Option<Vec<&str>> = snapshots
        .iter()
        .map(|snapshot| snapshot.as_object()?.get("path")?.as_str())
        .collect();
    let Some(paths) = paths else {
        return Some("source map has an invalid snapshot");
    };
    let chosen =
        strings_of(selected).filter(|chosen| chosen.iter().all(|path| !py::strip(path).is_empty()));
    let matching = chosen.is_some_and(|chosen| {
        let unique: BTreeSet<&str> = chosen.iter().map(String::as_str).collect();
        unique.len() == chosen.len() && unique == paths.iter().copied().collect::<BTreeSet<_>>()
    });
    (!matching).then_some("source map snapshots do not match selected_paths")
}

fn inside(root: &Path, path: &str) -> bool {
    let top = py::resolve_path(root);
    let located = py::resolve_path(&top.join(path));
    py::relative_to(&located, &top).is_ok()
}

fn sorted_strings(items: Vec<String>) -> Vec<String> {
    let mut items = items;
    items.sort();
    items
}

fn selection_problem(root: &Path, selected: &[String], policy: Option<&Object>) -> Option<String> {
    if let Some(escaping) = selected.iter().find(|path| !inside(root, path)) {
        return Some(format!("source path escapes repository: {escaping}"));
    }
    let policy_paths = live(policy)
        .and_then(|found| found.get("source_policy"))
        .and_then(Value::as_object)
        .and_then(|section| section.get("selected_paths"));
    if let Some(Value::Array(_)) = policy_paths {
        let held = strings_of(policy_paths).map(sorted_strings);
        if held != Some(sorted_strings(selected.to_vec())) {
            return Some("policy selected_paths do not match the source map".to_string());
        }
    }
    None
}

fn compare_snapshots(
    root: &Path,
    snapshots: &[&Object],
    policy: Option<&Object>,
) -> Res<(Vec<String>, Vec<String>)> {
    let paths: Vec<String> = snapshots
        .iter()
        .filter_map(|snapshot| {
            snapshot
                .get("path")
                .and_then(Value::as_str)
                .map(str::to_string)
        })
        .collect();
    let actual: HashMap<String, Object> = paths
        .iter()
        .cloned()
        .zip(source_snapshots(root, &paths, policy)?)
        .collect();
    let (mut changed, mut drifted) = (Vec::new(), Vec::new());
    for (snapshot, path) in snapshots.iter().zip(&paths) {
        let current = &actual[path];
        if !root.join(path).exists() || selection_moved(snapshot, current) {
            changed.push(path.clone());
        } else {
            let field = |object: &Object| object.get("digest").cloned().unwrap_or(Value::Null);
            if !equal(&field(snapshot), &field(current)) {
                drifted.push(path.clone());
            }
        }
    }
    Ok((changed, drifted))
}

fn group_paths(source_map: &Object) -> Vec<String> {
    source_map
        .get("source_groups")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .flat_map(|group| match group.get("paths") {
            Some(Value::Array(paths)) => paths
                .iter()
                .filter_map(|path| path.as_str().map(str::to_string))
                .collect(),
            Some(Value::Str(text)) => text.chars().map(String::from).collect(),
            _ => Vec::new(),
        })
        .collect()
}

fn listed_paths(source_map: &Object) -> BTreeSet<String> {
    group_paths(source_map).into_iter().collect()
}

fn discovered_but_gone(root: &Path, source_map: &Object) -> Vec<String> {
    let mut gone: Vec<String> = group_paths(source_map)
        .into_iter()
        .filter(|path| !root.join(path).exists())
        .collect();
    gone.sort();
    gone
}

pub fn verify_source_map(
    root: &Path,
    source_map_path: &Path,
    policy: Option<&Object>,
) -> Res<Verdict> {
    let root = py::resolve_path(root);
    let source_map = load_json(source_map_path)?;
    let selection_status = source_map
        .get("selection_status")
        .cloned()
        .unwrap_or_else(|| Value::Str(DEFAULT_SELECTION_STATUS.to_string()));
    let snapshots = source_map.get("source_snapshots");
    if let Some(problem) = snapshot_problem(snapshots, source_map.get("selected_paths")) {
        return Ok(verdict(
            "unverified",
            &selection_status,
            Some(problem.to_string()),
        ));
    }
    let selected = strings_of(source_map.get("selected_paths")).unwrap_or_default();
    if let Some(problem) = selection_problem(&root, &selected, policy) {
        return Ok(verdict("invalid", &selection_status, Some(problem)));
    }
    let snapshot_objects: Vec<&Object> = snapshots
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .collect();
    let (changed, drifted) = compare_snapshots(&root, &snapshot_objects, policy)?;
    if let Some(policy) = live(policy) {
        let files = source_files(
            &root,
            &selected
                .iter()
                .map(|path| root.join(path))
                .collect::<Vec<_>>(),
            Some(policy),
        )?;
        let errors = policy_report_errors(&root, &files, policy)?;
        if !errors.is_empty() {
            return Ok(verdict(
                "invalid",
                &selection_status,
                Some(errors.join("; ")),
            ));
        }
    }
    let mut found = verdict(
        if changed.is_empty() {
            "current"
        } else {
            "stale"
        },
        &selection_status,
        None,
    );
    found.changed_sources = changed;
    found.content_changed = drifted;
    found.gone = discovered_but_gone(&root, &source_map);
    Ok(found)
}

pub fn listing_drift(discovered: &Discovered, source_map: &Object) -> (Vec<String>, Vec<String>) {
    let listed = listed_paths(source_map);
    let current: BTreeSet<String> = discovered
        .groups
        .iter()
        .flat_map(|(_, paths)| paths.iter().cloned())
        .collect();
    (
        listed.difference(&current).cloned().collect(),
        current.difference(&listed).cloned().collect(),
    )
}

fn file_name(relative_path: &str) -> &str {
    relative_path
        .rsplit_once('/')
        .map_or(relative_path, |(_, name)| name)
}

fn is_test_file(name: &str) -> bool {
    name.ends_with("_test.go")
        || name.ends_with("_test.py")
        || name.contains(".test.")
        || name.contains(".spec.")
        || (name.starts_with("test_") && name.ends_with(".py"))
}

fn existing_directories(root: &Path, candidates: &[String]) -> Vec<String> {
    candidates
        .iter()
        .filter(|candidate| root.join(candidate).is_dir())
        .cloned()
        .collect()
}

fn git_ignored_entries(root: &Path) -> Res<Option<HashSet<String>>> {
    let arguments = [
        "ls-files",
        "--others",
        "--ignored",
        "--exclude-standard",
        "--directory",
        "-z",
    ];
    let Ok(listed) = run_lenient(root, &arguments, None)? else {
        return Ok(None);
    };
    if listed.code != 0 {
        return Ok(None);
    }
    Ok(Some(
        listed
            .stdout
            .split(|byte| *byte == 0)
            .filter(|entry| !entry.is_empty())
            .map(|entry| String::from_utf8_lossy(entry).into_owned())
            .collect(),
    ))
}

fn collect_files(
    directory: &Path,
    prefix: &str,
    excluded: &BTreeSet<String>,
    ignored: &HashSet<String>,
    found: &mut Vec<String>,
) {
    let Ok(entries) = std::fs::read_dir(directory) else {
        return;
    };
    let listed: Vec<std::fs::DirEntry> = entries.flatten().collect();
    if !prefix.is_empty() && listed.iter().any(|entry| entry.file_name() == ".git") {
        return;
    }
    let mut children = Vec::new();
    for entry in &listed {
        let entry_name = entry.file_name().to_string_lossy().into_owned();
        let name = format!("{prefix}{entry_name}");
        let linked = entry.file_type().is_ok_and(|kind| kind.is_symlink());
        if !entry.path().is_dir() {
            if !ignored.contains(&name) {
                found.push(name);
            }
        } else if !excluded.contains(&entry_name)
            && !linked
            && !ignored.contains(&format!("{name}/"))
        {
            children.push((entry.path(), format!("{name}/")));
        }
    }
    for (path, child_prefix) in children {
        collect_files(&path, &child_prefix, excluded, ignored, found);
    }
}

pub fn repository_files(root: &Path, excluded: &BTreeSet<String>) -> Res<Files> {
    let ignored = git_ignored_entries(root)?;
    let mut names = Vec::new();
    collect_files(
        root,
        "",
        excluded,
        &ignored.clone().unwrap_or_default(),
        &mut names,
    );
    Ok(Files {
        names,
        pruned_by_git: ignored.is_some(),
    })
}

impl Listing {
    pub fn new(root: &Path, excluded: BTreeSet<String>) -> Self {
        Self {
            root: root.to_path_buf(),
            excluded,
            files: None,
        }
    }

    pub fn default_for(root: &Path) -> Self {
        Self::new(
            root,
            EXCLUDED_DIRECTORIES
                .iter()
                .map(|name| (*name).to_string())
                .collect(),
        )
    }

    pub fn files(&mut self) -> Res<&Files> {
        if self.files.is_none() {
            self.files = Some(repository_files(&self.root, &self.excluded)?);
        }
        Ok(self.files.as_ref().expect("listed"))
    }
}

fn ignored_among(root: &Path, files: &Files, relatives: &[String]) -> Res<HashSet<String>> {
    if files.pruned_by_git {
        return Ok(HashSet::new());
    }
    ignored_relatives(root, relatives)
}

pub fn discover_ci_tools(root: &Path) -> Vec<String> {
    let mut found: Vec<&(&str, &str)> = CI_MARKERS
        .iter()
        .filter(|(path, _)| root.join(path).exists())
        .collect();
    found.sort_by_key(|(_, name)| *name);
    found.iter().map(|(_, name)| (*name).to_string()).collect()
}

pub fn discover_sources(
    root: &Path,
    profile: &Profile,
    listing: Option<&mut Listing>,
) -> Res<Discovered> {
    let root = py::resolve_path(root);
    let ci_tools = discover_ci_tools(&root);
    let mut own;
    let listing = match listing {
        Some(shared) => shared,
        None => {
            own = Listing::new(
                &root,
                EXCLUDED_DIRECTORIES
                    .iter()
                    .map(|name| (*name).to_string())
                    .chain(profile.excluded_directories.clone())
                    .collect(),
            );
            &mut own
        }
    };
    let files = listing.files()?;
    let sorted = |mut found: Vec<String>| {
        found.sort();
        found
    };
    let instructions = sorted(
        files
            .names
            .iter()
            .filter(|entry| file_name(entry) == INSTRUCTION_FILE_NAME)
            .cloned()
            .collect(),
    );
    let bases: Vec<String> = profile
        .implementation_roots
        .iter()
        .map(|base| format!("{base}/"))
        .collect();
    let boundaries = sorted(
        files
            .names
            .iter()
            .filter(|entry| {
                bases.iter().any(|base| entry.starts_with(base.as_str()))
                    && profile
                        .boundary_markers
                        .iter()
                        .any(|marker| marker == file_name(entry))
            })
            .cloned()
            .collect(),
    );
    let tests = sorted(
        files
            .names
            .iter()
            .filter(|entry| is_test_file(file_name(entry)))
            .cloned()
            .collect(),
    );
    let all: Vec<String> = [&instructions, &boundaries, &tests]
        .into_iter()
        .flat_map(|group| group.iter().cloned())
        .collect();
    let ignored = ignored_among(&root, files, &all)?;
    let kept = |group: Vec<String>| -> Vec<String> {
        group
            .into_iter()
            .filter(|entry| !ignored.contains(entry))
            .collect()
    };
    let requirements: Vec<String> = [
        "docs/plans",
        "docs/spec",
        "docs/requirements",
        "spec",
        "requirements",
    ]
    .iter()
    .map(|name| (*name).to_string())
    .collect();
    let contracts: Vec<String> = ["contracts", "openapi", "asyncapi"]
        .iter()
        .map(|name| (*name).to_string())
        .collect();
    Ok(Discovered {
        groups: vec![
            ("repository_instructions", kept(instructions)),
            (
                "decisions",
                existing_directories(&root, &profile.decision_roots),
            ),
            ("requirements", existing_directories(&root, &requirements)),
            ("contracts", existing_directories(&root, &contracts)),
            ("bounded_contexts", kept(boundaries)),
            (
                "implementation",
                existing_directories(&root, &profile.implementation_roots),
            ),
            ("tests", kept(tests)),
        ],
        ci_tools,
    })
}

type GitState = (Vec<(String, String)>, Vec<String>);

fn git_state(root: &Path, selected: &[String]) -> Res<Option<GitState>> {
    if selected.is_empty() || !root.join(".git").exists() {
        return Ok(None);
    }
    let heads: Vec<String> = selected.iter().map(|path| format!("HEAD:{path}")).collect();
    let mut arguments = vec!["rev-parse"];
    arguments.extend(heads.iter().map(String::as_str));
    let Ok(trees) = run_lenient(root, &arguments, None)? else {
        return Ok(None);
    };
    let mut status = vec!["status", "--porcelain", "-z", "--"];
    status.extend(selected.iter().map(String::as_str));
    let Ok(dirty) = run_lenient(root, &status, None)? else {
        return Ok(None);
    };
    if trees.code != 0 || dirty.code != 0 {
        return Ok(None);
    }
    let objects: Vec<String> = lossy(&trees.stdout)
        .split_whitespace()
        .map(str::to_string)
        .collect();
    if objects.len() != selected.len() {
        return Ok(None);
    }
    let dirty_sources = sorted_strings(
        lossy(&dirty.stdout)
            .split('\0')
            .filter(|entry| !entry.is_empty())
            .map(str::to_string)
            .collect(),
    );
    Ok(Some((
        selected.iter().cloned().zip(objects).collect(),
        dirty_sources,
    )))
}

fn git_shows_no_change(root: &Path, source_map: &Object) -> Res<bool> {
    let Some(Value::Object(recorded)) = source_map.get("git_state") else {
        return Ok(false);
    };
    let tracked = recorded.get("tracked_objects");
    if !tracked.is_some_and(truthy) || recorded.get("dirty_sources").is_some_and(truthy) {
        return Ok(false);
    }
    let selected = match source_map.get("selected_paths") {
        None => Vec::new(),
        Some(found) => match strings_of(Some(found)) {
            Some(listed) => listed,
            None => return Ok(false),
        },
    };
    let Some((objects, dirty)) = git_state(root, &selected)? else {
        return Ok(false);
    };
    let mut now = Object::new();
    for (path, object) in objects {
        now.insert(path, Value::Str(object));
    }
    let same = tracked.is_some_and(|held| equal(held, &Value::Object(now)));
    Ok(same && dirty.is_empty())
}

fn policy_verdict(
    root: &Path,
    source_map: &Object,
    policy: &Object,
    selection_status: &Value,
) -> Res<Option<Verdict>> {
    let errors = validate_policy(policy);
    if !errors.is_empty() {
        return Ok(Some(verdict(
            "invalid",
            selection_status,
            Some(errors.join("; ")),
        )));
    }
    let policy_paths = strings_of(
        policy
            .get("source_policy")
            .and_then(Value::as_object)
            .and_then(|section| section.get("selected_paths")),
    )
    .unwrap_or_default();
    let recorded = strings_of(source_map.get("selected_paths")).map(sorted_strings);
    if recorded != Some(sorted_strings(policy_paths.clone())) {
        return Ok(Some(verdict(
            "invalid",
            selection_status,
            Some("policy selected_paths do not match the source map".to_string()),
        )));
    }
    let joined: Vec<PathBuf> = policy_paths.iter().map(|path| root.join(path)).collect();
    let report = source_files(root, &joined, Some(policy))
        .and_then(|files| policy_report_errors(root, &files, policy));
    match report {
        Err(Fail::Os(message) | Fail::Value(message)) => {
            Ok(Some(verdict("invalid", selection_status, Some(message))))
        }
        Ok(errors) if !errors.is_empty() => Ok(Some(verdict(
            "invalid",
            selection_status,
            Some(errors.join("; ")),
        ))),
        Ok(_) => Ok(None),
    }
}

pub fn probe_sources(root: &Path, source_map_path: &Path, policy: Option<&Object>) -> Res<Verdict> {
    if !source_map_path.exists() {
        let mut absent = verdict(
            "absent",
            &Value::Null,
            Some(format!("no source map at {}", py::shown(source_map_path))),
        );
        let hint = one_level_too_deep_hint(source_map_path);
        absent.hint = hint
            .strip_prefix(HINT_SEPARATOR)
            .map(str::to_string)
            .or_else(|| (!hint.is_empty()).then_some(hint));
        return Ok(absent);
    }
    let source_map = load_json(source_map_path)?;
    let selection_status = source_map
        .get("selection_status")
        .cloned()
        .unwrap_or_else(|| Value::Str(DEFAULT_SELECTION_STATUS.to_string()));
    if let Some(policy) = policy
        && let Some(invalid) = policy_verdict(root, &source_map, policy, &selection_status)?
    {
        return Ok(invalid);
    }
    let mut result = if git_shows_no_change(root, &source_map)? {
        let mut current = verdict("current", &selection_status, None);
        current.checked = Some("git");
        current
    } else {
        let mut hashed = verify_source_map(root, source_map_path, policy)?;
        hashed.checked = Some("hash");
        hashed
    };
    if result.status != "current" {
        return Ok(result);
    }
    let profile = project_profile(policy);
    let discovered = discover_sources(root, &profile, None)?;
    let (gone, unlisted) = listing_drift(&discovered, &source_map);
    if !gone.is_empty() || !unlisted.is_empty() {
        result.status = "discovery_drifted".to_string();
    }
    result.gone = gone;
    result.unlisted = unlisted;
    Ok(result)
}

fn named(sources: &[String]) -> String {
    sources
        .iter()
        .take(5)
        .cloned()
        .collect::<Vec<_>>()
        .join(", ")
}

fn closed(sentence: String) -> String {
    if sentence.ends_with('?') {
        sentence
    } else {
        format!("{sentence}.")
    }
}

pub fn probe_summary(
    result: &Verdict,
    registry_root: &Path,
    repo_root: &Path,
    given_registry: &str,
    given_repo: &str,
) -> Res<String> {
    let where_ = format!(
        "Domain Memory at {}",
        shown_path(registry_root, Some(repo_root))
    );
    if result.status == "absent" {
        if let Some(hint) = &result.hint {
            return Ok(format!("{where_}: no source map here, but {hint}."));
        }
        let hint = domain_memory_hint(registry_root, Some(repo_root))?;
        return Ok(closed(format!("{where_}: none; {hint}")));
    }
    if result.status == "current" || result.status == "discovery_drifted" {
        let inside = if result.content_changed.is_empty() {
            String::new()
        } else {
            format!(
                "; {} selected sources changed inside ({}); verify-evidence checks the citations",
                result.content_changed.len(),
                named(&result.content_changed)
            )
        };
        let drifted = if result.status == "discovery_drifted" {
            format!(
                "; discovery drifted: {} gone, {} unlisted; {}",
                counted(result.gone.len(), "listed path"),
                counted(result.unlisted.len(), "matching path"),
                refresh_listing_advice(given_registry, given_repo)
            )
        } else {
            String::new()
        };
        return Ok(format!(
            "{where_}: current, {}, established by {}{inside}{drifted}.",
            str_of(&result.selection_status),
            result.checked.unwrap_or_default()
        ));
    }
    if result.status == "stale" {
        return Ok(format!(
            "{where_}: stale. {} selected sources gained, lost or renamed files ({}); run refresh-sources, then have a developer run confirm-sources.",
            result.changed_sources.len(),
            named(&result.changed_sources)
        ));
    }
    Ok(py::strip(&format!(
        "{where_}: {}. {}",
        result.status,
        result.reason.clone().unwrap_or_default()
    ))
    .to_string())
}

fn is_domain_source_map(path: &Path) -> bool {
    load_json(path)
        .is_ok_and(|map| map.get("format").and_then(Value::as_str) == Some(SOURCE_MAP_FORMAT))
}

pub enum Listed {
    Places(Vec<PathBuf>),
    Refused(String),
}

fn domain_memories_known_to_git(repo_root: &Path) -> Res<Listed> {
    let mut arguments = vec![
        "ls-files",
        "-z",
        "--cached",
        "--others",
        "--exclude-standard",
        "--",
    ];
    arguments.extend(DOMAIN_MEMORY_MARKERS);
    let result = match run_lenient(repo_root, &arguments, None)? {
        Ok(output) => output,
        Err(message) => return Ok(Listed::Refused(message)),
    };
    if result.code != 0 {
        return Ok(Listed::Refused(
            py::strip(&String::from_utf8_lossy(&result.stderr)).to_string(),
        ));
    }
    let places = String::from_utf8_lossy(&result.stdout)
        .split('\0')
        .filter(|path| !path.is_empty())
        .map(|path| repo_root.join(path))
        .filter(|marker| {
            marker
                .file_name()
                .is_some_and(|name| name != "source-map.json")
                || is_domain_source_map(marker)
        })
        .filter_map(|marker| {
            let home = if marker
                .file_name()
                .is_some_and(|name| name == "source-map.json")
            {
                marker.parent()
            } else {
                marker.parent().and_then(Path::parent)
            };
            home.map(Path::to_path_buf)
        })
        .collect();
    Ok(Listed::Places(places))
}

pub fn domain_memories_in(repo_root: &Path) -> Res<Listed> {
    Ok(match domain_memories_known_to_git(repo_root)? {
        Listed::Places(places) => Listed::Places(sorted_paths(
            places.iter().map(|place| py::resolve_path(place)).collect(),
        )),
        refused => refused,
    })
}

pub fn domain_memory_hint(root: &Path, repo_root: Option<&Path>) -> Res<String> {
    let asked = py::resolve_path(root);
    let candidates: Vec<PathBuf> = match repo_root.map(domain_memories_known_to_git).transpose()? {
        Some(Listed::Places(places)) => {
            sorted_paths(places.iter().map(|place| py::resolve_path(place)).collect())
        }
        _ => store::neighbours(&asked)?
            .into_iter()
            .filter(|place| holds_domain_memory(place))
            .collect(),
    };
    let places: Vec<PathBuf> = candidates
        .into_iter()
        .filter(|place| !same_path(place, &asked))
        .collect();
    let shown: Vec<String> = places
        .iter()
        .map(|place| shown_path(place, repo_root))
        .collect();
    Ok(match shown.as_slice() {
        [] => DOMAIN_MEMORY_HINT.to_string(),
        [only] => format!("{DOMAIN_MEMORY_HINT}; did you mean {only}?"),
        many => format!(
            "{DOMAIN_MEMORY_HINT}; Domain Memories in this repository: {}",
            many.join(", ")
        ),
    })
}
