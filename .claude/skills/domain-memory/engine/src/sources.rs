use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{MAIN_SEPARATOR, Path, PathBuf};

use crate::audit_append::{append, utc_now};
use crate::coverage::SourceKinds;
use crate::files::{load_json, one_level_too_deep_hint, write_json, write_source_file};
use crate::fnmatch::fnmatchcase;
use crate::git::{self, Failure};
use crate::json::{Object, Value};
use crate::listing::{
    EXCLUDED_DIRECTORIES, Repository, ignored_among, ignored_relatives, listing_of,
};
use crate::okf::truthy;
use crate::policy::validate_policy;
use crate::py::{self, Raised};
use crate::pyvalue::{equal, kind_of, str_of};
use crate::sha256::Sha256;
use crate::store::{SOURCE_MAP_FORMAT, domain_memory_hint, shown_path};
use crate::transaction::with_writer_lock;
const INSTRUCTION_FILE_NAME: &str = "AGENTS.md";
const MAX_NAMED_OVERSIZED: usize = 5;
const CI_MARKERS: [(&str, &str); 6] = [
    (".github/workflows", "github-actions"),
    (".gitlab-ci.yml", "gitlab-ci"),
    ("Jenkinsfile", "jenkins"),
    (".circleci/config.yml", "circleci"),
    (".buildkite/pipeline.yml", "buildkite"),
    ("azure-pipelines.yml", "azure-pipelines"),
];
const DEFAULT_IMPLEMENTATION_ROOTS: [&str; 8] = [
    "apps",
    "src",
    "services",
    "packages",
    "db",
    "database",
    "migrations",
    "schema",
];
const DEFAULT_DECISION_ROOTS: [&str; 4] = ["docs/adr", "adr", "docs/architecture", "architecture"];
const DEFAULT_BOUNDARY_MARKERS: [&str; 1] = ["doc.go"];
const REQUIREMENT_ROOTS: [&str; 5] = [
    "docs/plans",
    "docs/spec",
    "docs/requirements",
    "spec",
    "requirements",
];
const CONTRACT_ROOTS: [&str; 3] = ["contracts", "openapi", "asyncapi"];
const INSTRUCTION_CAUTION: &str = "These files exist to steer agents working in this repository. They are rewritten whenever the workflow changes, and a Domain Memory that rests on one loses its evidence when it is. Select them only for a domain fact no other source states, and record the gap.";

pub struct Profile {
    implementation_roots: Vec<Value>,
    excluded_directories: Vec<Value>,
    decision_roots: Vec<Value>,
    boundary_markers: Vec<Value>,
}

impl Profile {
    pub fn from_section(section: Option<&Value>) -> Self {
        let given = section.and_then(Value::as_object);
        let listed = |key: &str, default: &[&str]| -> Vec<Value> {
            match given.and_then(|given| given.get(key)) {
                Some(Value::Array(items)) => items.clone(),
                _ => default
                    .iter()
                    .map(|name| Value::Str((*name).to_string()))
                    .collect(),
            }
        };
        Self {
            implementation_roots: listed("implementation_roots", &DEFAULT_IMPLEMENTATION_ROOTS),
            excluded_directories: listed("excluded_directories", &[]),
            decision_roots: listed("decision_roots", &DEFAULT_DECISION_ROOTS),
            boundary_markers: listed("boundary_markers", &DEFAULT_BOUNDARY_MARKERS),
        }
    }

    pub fn of_policy(policy: Option<&Object>) -> Self {
        Self::from_section(policy.and_then(|policy| policy.get("project_profile")))
    }

    pub fn to_value(&self) -> Value {
        let mut section = Object::new();
        section.insert(
            "implementation_roots",
            Value::Array(self.implementation_roots.clone()),
        );
        section.insert(
            "excluded_directories",
            Value::Array(self.excluded_directories.clone()),
        );
        section.insert("decision_roots", Value::Array(self.decision_roots.clone()));
        section.insert(
            "boundary_markers",
            Value::Array(self.boundary_markers.clone()),
        );
        Value::Object(section)
    }

    pub fn has_excluded_directories(&self) -> bool {
        !self.excluded_directories.is_empty()
    }

    fn excluded_set(&self) -> HashSet<String> {
        EXCLUDED_DIRECTORIES
            .iter()
            .map(|name| (*name).to_string())
            .chain(texts(&self.excluded_directories))
            .collect()
    }
}

fn texts(items: &[Value]) -> impl Iterator<Item = String> + '_ {
    items
        .iter()
        .filter_map(|item| item.as_str().map(str::to_string))
}

fn only_texts(items: &[Value], key: &str) -> Result<Vec<String>, String> {
    items
        .iter()
        .map(|item| {
            item.as_str().map(str::to_string).ok_or_else(|| {
                format!(
                    "project_profile.{key} must hold text, got {}",
                    kind_of(item)
                )
            })
        })
        .collect()
}

fn strings(values: &[String]) -> Value {
    Value::Array(values.iter().map(|text| Value::Str(text.clone())).collect())
}

fn file_name(relative: &str) -> &str {
    relative.rsplit_once('/').map_or(relative, |(_, name)| name)
}

fn is_test_file(name: &str) -> bool {
    name.ends_with("_test.go")
        || name.ends_with("_test.py")
        || name.contains(".test.")
        || name.contains(".spec.")
        || (name.starts_with("test_") && name.ends_with(".py"))
}

fn sorted_where(files: &[String], keep: impl Fn(&String) -> bool) -> Vec<String> {
    let mut chosen: Vec<String> = files.iter().filter(|entry| keep(entry)).cloned().collect();
    chosen.sort();
    chosen
}

fn existing_directories(root: &Path, candidates: &[String]) -> Vec<String> {
    candidates
        .iter()
        .filter(|candidate| root.join(candidate).is_dir())
        .cloned()
        .collect()
}

fn discover_ci_tools(root: &Path) -> Vec<String> {
    let mut tools: Vec<&str> = CI_MARKERS
        .iter()
        .filter(|(marker, _)| root.join(marker).exists())
        .map(|(_, name)| *name)
        .collect();
    tools.sort_unstable();
    tools.into_iter().map(str::to_string).collect()
}

fn source_group(
    kind: &str,
    authority: &str,
    caution: Option<&str>,
    paths: Vec<String>,
) -> (String, Object) {
    let mut group = Object::new();
    group.insert("kind", Value::Str(kind.to_string()));
    group.insert("authority", Value::Str(authority.to_string()));
    if let Some(caution) = caution {
        group.insert("caution", Value::Str(caution.to_string()));
    }
    group.insert("paths", strings(&paths));
    (kind.to_string(), group)
}

fn roots_among(root: &Path, items: &[Value], key: &str) -> Result<Vec<String>, String> {
    Ok(existing_directories(root, &only_texts(items, key)?))
}

pub fn discover_sources(
    root: &Path,
    profile: &Profile,
    listing: Option<&mut Repository>,
) -> Result<Object, Raised> {
    let root = py::resolve_path(root);
    let ci_tools = discover_ci_tools(&root);
    let own;
    let files = match listing {
        Some(shared) => shared.files()?,
        None => {
            own = listing_of(&root, &profile.excluded_set())?;
            &own
        }
    };
    let bases: Vec<String> = profile
        .implementation_roots
        .iter()
        .map(|base| format!("{}/", str_of(base)))
        .collect();
    let markers: HashSet<String> = texts(&profile.boundary_markers).collect();
    let candidates = [
        sorted_where(&files.entries, |entry| {
            file_name(entry) == INSTRUCTION_FILE_NAME
        }),
        sorted_where(&files.entries, |entry| {
            bases.iter().any(|base| entry.starts_with(base)) && markers.contains(file_name(entry))
        }),
        sorted_where(&files.entries, |entry| is_test_file(file_name(entry))),
    ];
    let every: Vec<String> = candidates.iter().flatten().cloned().collect();
    let ignored = ignored_among(&root, files, &every)?;
    let [instructions, boundaries, tests] = candidates.map(|group| {
        group
            .into_iter()
            .filter(|entry| !ignored.contains(entry))
            .collect::<Vec<_>>()
    });
    let named =
        |names: &[&str]| -> Vec<String> { names.iter().map(|n| (*n).to_string()).collect() };
    let groups = [
        source_group(
            "repository_instructions",
            "instructions to coding agents, not a statement of the business domain",
            Some(INSTRUCTION_CAUTION),
            instructions,
        ),
        source_group(
            "decisions",
            "current approved decisions when their status says so",
            None,
            roots_among(&root, &profile.decision_roots, "decision_roots")?,
        ),
        source_group(
            "requirements",
            "intended behavior and acceptance criteria",
            None,
            existing_directories(&root, &named(&REQUIREMENT_ROOTS)),
        ),
        source_group(
            "contracts",
            "inter-process promises and compatibility",
            None,
            existing_directories(&root, &named(&CONTRACT_ROOTS)),
        ),
        source_group(
            "bounded_contexts",
            "candidate technical boundaries requiring domain review",
            None,
            boundaries,
        ),
        source_group(
            "implementation",
            "current executable behavior",
            None,
            roots_among(&root, &profile.implementation_roots, "implementation_roots")?,
        ),
        source_group(
            "tests",
            "evidence that a specific behavior is checked",
            None,
            tests,
        ),
    ];
    let missing: Vec<String> = groups
        .iter()
        .filter(|(_, group)| paths_of(group).is_empty())
        .map(|(kind, _)| kind.clone())
        .collect();
    let order: Vec<String> = groups.iter().map(|(kind, _)| kind.clone()).collect();
    let mut governance = Object::new();
    governance.insert("ci_tools", strings(&ci_tools));
    governance.insert(
        "recommended_verifier",
        Value::Str("git-signed-commit".to_string()),
    );
    governance.insert("recommended_trigger", Value::Str("git-push".to_string()));
    let mut map = Object::new();
    map.insert("format", Value::Str(SOURCE_MAP_FORMAT.to_string()));
    map.insert("repo_root", Value::Str(".".to_string()));
    map.insert("source_order", strings(&order));
    map.insert(
        "source_groups",
        Value::Array(
            groups
                .into_iter()
                .map(|(_, group)| Value::Object(group))
                .collect(),
        ),
    );
    map.insert("missing_groups", strings(&missing));
    map.insert("selection_status", Value::Str("discovered".to_string()));
    map.insert("governance_candidates", Value::Object(governance));
    map.insert(
        "note",
        Value::Str("The map locates candidate sources. It does not establish a domain fact or source authority by itself.".to_string()),
    );
    Ok(map)
}

fn paths_of(group: &Object) -> &[Value] {
    group
        .get("paths")
        .and_then(Value::as_array)
        .unwrap_or_default()
}

fn source_policy_of(policy: Option<&Object>) -> Result<Option<&Object>, String> {
    match policy.and_then(|policy| policy.get("source_policy")) {
        None => Ok(None),
        Some(Value::Object(section)) => Ok(Some(section)),
        Some(other) => Err(format!(
            "source_policy must be an object, got {}",
            kind_of(other)
        )),
    }
}

fn pattern_list(
    section: Option<&Object>,
    key: &str,
    default: &[&str],
) -> Result<Vec<Value>, String> {
    match section.and_then(|section| section.get(key)) {
        None => Ok(default
            .iter()
            .map(|p| Value::Str((*p).to_string()))
            .collect()),
        Some(Value::Array(items)) => Ok(items.clone()),
        Some(Value::Str(text)) => Ok(text.chars().map(|c| Value::Str(c.to_string())).collect()),
        Some(Value::Object(object)) => {
            Ok(object.iter().map(|(k, _)| Value::Str(k.clone())).collect())
        }
        Some(other) => Err(format!(
            "source_policy.{key} is not iterable: {}",
            kind_of(other)
        )),
    }
}

fn pattern_matches(path: &str, pattern: &Value) -> Result<bool, String> {
    match pattern {
        Value::Str(text) => fnmatchcase(path, text),
        other => Err(format!(
            "source_policy patterns must be text, got {}",
            kind_of(other)
        )),
    }
}

fn policy_matches(path: &str, include: &[Value], exclude: &[Value]) -> Result<bool, String> {
    let mut included = false;
    for pattern in include {
        if pattern.as_str() == Some("**") || pattern_matches(path, pattern)? {
            included = true;
            break;
        }
    }
    for pattern in exclude {
        if pattern_matches(path, pattern)? {
            return Ok(false);
        }
    }
    Ok(included)
}

fn relative(root: &Path, path: &Path) -> Result<String, String> {
    let prefix = format!("{}{MAIN_SEPARATOR}", py::shown(root));
    if let Some(rest) = py::shown(path).strip_prefix(&prefix) {
        return Ok(rest.replace(MAIN_SEPARATOR, "/"));
    }
    let found = py::relative_to(path, root)
        .or_else(|_| py::relative_to(&py::resolve_path(path), &py::resolve_path(root)))?;
    Ok(if found.is_empty() {
        ".".to_string()
    } else {
        found
    })
}

fn in_excluded_directory(relative: &str, excluded: &HashSet<String>) -> bool {
    relative
        .split('/')
        .filter(|part| !part.is_empty() && *part != ".")
        .any(|part| excluded.contains(part))
}

fn same(left: &Path, right: &Path) -> bool {
    py::compare_paths(left, right).is_eq()
}

fn path_key(path: &Path) -> Vec<String> {
    let text = py::shown(path);
    if cfg!(windows) {
        text.to_lowercase()
            .split(MAIN_SEPARATOR)
            .map(str::to_string)
            .collect()
    } else {
        text.split(MAIN_SEPARATOR).map(str::to_string).collect()
    }
}

fn is_symlink(path: &Path) -> bool {
    fs::symlink_metadata(path).is_ok_and(|metadata| metadata.file_type().is_symlink())
}

fn regular_files_under(directory: &Path, excluded: &HashSet<String>) -> Vec<PathBuf> {
    let mut found = Vec::new();
    let mut pending = vec![directory.to_path_buf()];
    while let Some(current) = pending.pop() {
        let Ok(entries) = fs::read_dir(&current) else {
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

fn regular_files(selected: &Path, excluded: &HashSet<String>) -> Vec<PathBuf> {
    if selected.is_file() {
        return if is_symlink(selected) {
            Vec::new()
        } else {
            vec![selected.to_path_buf()]
        };
    }
    regular_files_under(selected, excluded)
}

fn selection_roots(selected_paths: &[PathBuf]) -> Vec<PathBuf> {
    let mut roots: Vec<PathBuf> = Vec::new();
    for selected in selected_paths {
        let resolved = py::resolve_path(selected);
        let covered = roots
            .iter()
            .filter(|parent| parent.is_dir())
            .any(|parent| resolved.ancestors().any(|ancestor| same(ancestor, parent)));
        if !covered {
            roots.push(resolved);
        }
    }
    roots
}

fn source_files(
    root: &Path,
    selected_paths: &[PathBuf],
    policy: Option<&Object>,
) -> Result<Vec<PathBuf>, Raised> {
    let section = source_policy_of(policy)?;
    let include = pattern_list(section, "include", &["**"])?;
    let exclude = pattern_list(section, "exclude", &[])?;
    let excluded = Profile::of_policy(policy).excluded_set();
    let mut chosen: Vec<(String, PathBuf)> = Vec::new();
    let mut seen: HashSet<Vec<String>> = HashSet::new();
    for resolved in selection_roots(selected_paths) {
        for candidate in regular_files(&resolved, &excluded) {
            let named = relative(root, &candidate)?;
            if !in_excluded_directory(&named, &excluded)
                && policy_matches(&named, &include, &exclude)?
                && seen.insert(path_key(&candidate))
            {
                chosen.push((named, candidate));
            }
        }
    }
    let names: Vec<String> = chosen.iter().map(|(named, _)| named.clone()).collect();
    let ignored = ignored_relatives(root, &names)?;
    let mut kept: Vec<PathBuf> = chosen
        .into_iter()
        .filter(|(named, _)| !ignored.contains(named))
        .map(|(_, path)| path)
        .collect();
    kept.sort_by_key(|path| path_key(path));
    Ok(kept)
}

struct Snapshot {
    path: String,
    file_count: usize,
    digest: String,
    listing: String,
}

impl Snapshot {
    fn to_value(&self) -> Value {
        let mut item = Object::new();
        item.insert("path", Value::Str(self.path.clone()));
        item.insert("file_count", Value::int(self.file_count));
        item.insert("digest", Value::Str(self.digest.clone()));
        item.insert("listing_sha256", Value::Str(self.listing.clone()));
        Value::Object(item)
    }
}

fn owned_by_selection(root: &Path, selected: &[String], files: &[PathBuf]) -> Vec<Vec<PathBuf>> {
    let resolved: Vec<(PathBuf, bool)> = selected
        .iter()
        .map(|path| {
            let joined = root.join(path);
            (py::resolve_path(&joined), joined.is_dir())
        })
        .collect();
    let mut owned = vec![Vec::new(); selected.len()];
    for file in files {
        let mut owner: Option<usize> = None;
        for (index, (selected_path, is_directory)) in resolved.iter().enumerate() {
            let contains = same(file, selected_path)
                || (*is_directory
                    && file.parent().is_some_and(|directory| {
                        directory.ancestors().any(|a| same(a, selected_path))
                    }));
            let longer = owner.is_none_or(|current| {
                selected[index].chars().count() > selected[current].chars().count()
            });
            if contains && longer {
                owner = Some(index);
            }
        }
        if let Some(index) = owner {
            owned[index].push(file.clone());
        }
    }
    owned
}

fn source_snapshot(root: &Path, path: &str, files: &[PathBuf]) -> Result<Snapshot, Raised> {
    let (mut digest, mut listing) = (Sha256::start(), Sha256::start());
    for file in files {
        let name = relative(root, file)?;
        listing.update(name.as_bytes());
        listing.update(&[0]);
        digest.update(name.as_bytes());
        digest.update(&[0]);
        digest.update(&fs::read(file).map_err(|error| Raised::open(&error, file))?);
        digest.update(&[0]);
    }
    Ok(Snapshot {
        path: path.to_string(),
        file_count: files.len(),
        digest: format!("sha256:{}", digest.hex_finish()),
        listing: format!("sha256:{}", listing.hex_finish()),
    })
}

fn source_snapshots(
    root: &Path,
    selected: &[String],
    policy: Option<&Object>,
) -> Result<HashMap<String, Snapshot>, Raised> {
    let mut unique: Vec<String> = Vec::new();
    for path in selected {
        if !unique.contains(path) {
            unique.push(path.clone());
        }
    }
    let joined: Vec<PathBuf> = unique.iter().map(|path| root.join(path)).collect();
    let files = source_files(root, &joined, policy)?;
    let owned = owned_by_selection(root, &unique, &files);
    let mut snapshots = HashMap::new();
    for (path, files) in unique.iter().zip(&owned) {
        snapshots.insert(path.clone(), source_snapshot(root, path, files)?);
    }
    Ok(snapshots)
}

fn selection_moved(recorded: &Object, current: &Snapshot) -> bool {
    let counted = recorded
        .get("file_count")
        .is_some_and(|count| equal(count, &Value::int(current.file_count)));
    let listing_moved = recorded
        .get("listing_sha256")
        .and_then(Value::as_str)
        .is_some_and(|listing| listing != current.listing);
    !counted || listing_moved
}

fn number_of(value: &Value) -> Option<f64> {
    match value {
        Value::Int(digits) => digits.parse().ok(),
        Value::Float(number) => Some(*number),
        Value::Bool(flag) => Some(f64::from(u8::from(*flag))),
        _ => None,
    }
}

fn limit_of(limits: &Object, key: &str) -> Result<(f64, String), String> {
    let value = limits
        .get(key)
        .ok_or_else(|| format!("policy limits has no {key}"))?;
    let number = number_of(value).ok_or_else(|| {
        format!(
            "policy limits {key} must be a number, got {}",
            kind_of(value)
        )
    })?;
    Ok((number, str_of(value)))
}

fn source_policy_report(
    root: &Path,
    selected_paths: &[PathBuf],
    policy: &Object,
) -> Result<Object, Raised> {
    policy_report_of(
        root,
        &source_files(root, selected_paths, Some(policy))?,
        policy,
    )
}

fn policy_report_of(root: &Path, files: &[PathBuf], policy: &Object) -> Result<Object, Raised> {
    let limits = policy
        .get("limits")
        .and_then(Value::as_object)
        .ok_or_else(|| "policy has no limits".to_string())?;
    let (max_count, shown_count) = limit_of(limits, "max_file_count")?;
    let (max_total, shown_total) = limit_of(limits, "max_total_bytes")?;
    let (max_file, shown_file) = limit_of(limits, "max_file_bytes")?;
    let mut sizes = Vec::new();
    for file in files {
        let size = fs::metadata(file)
            .map_err(|error| Raised::os(&error, file))?
            .len();
        sizes.push((relative(root, file)?, size));
    }
    let total: u64 = sizes.iter().map(|(_, size)| size).sum();
    let oversized: Vec<String> = sizes
        .iter()
        .filter(|(_, size)| *size as f64 > max_file)
        .map(|(named, _)| named.clone())
        .collect();
    let mut errors = Vec::new();
    if files.len() as f64 > max_count {
        errors.push(format!(
            "selected sources hold {} files, over max_file_count {shown_count}",
            files.len()
        ));
    }
    if total as f64 > max_total {
        errors.push(format!(
            "selected sources hold {total} bytes, over max_total_bytes {shown_total}"
        ));
    }
    if !oversized.is_empty() {
        let remainder = if oversized.len() <= MAX_NAMED_OVERSIZED {
            String::new()
        } else {
            format!(", and {} more", oversized.len() - MAX_NAMED_OVERSIZED)
        };
        errors.push(format!(
            "over max_file_bytes {shown_file}: {}{remainder}",
            oversized[..oversized.len().min(MAX_NAMED_OVERSIZED)].join(", ")
        ));
    }
    let mut report = Object::new();
    report.insert("files", Value::int(files.len()));
    report.insert("total_bytes", Value::Int(total.to_string()));
    report.insert("oversized_files", strings(&oversized));
    report.insert("errors", strings(&errors));
    Ok(report)
}

fn verdict(status: &str, selection_status: &Value, reason: &str) -> Object {
    let mut result = Object::new();
    result.insert("status", Value::Str(status.to_string()));
    result.insert("selection_status", selection_status.clone());
    result.insert("reason", Value::Str(reason.to_string()));
    result
}

fn snapshot_problem(snapshots: Option<&Value>, selected: Option<&Value>) -> Option<&'static str> {
    let Some(Value::Array(snapshots)) = snapshots else {
        return Some("source map has no snapshots");
    };
    let mut snapshot_paths = Vec::new();
    for snapshot in snapshots {
        match snapshot.as_object().and_then(|item| item.get("path")) {
            Some(Value::Str(path)) => snapshot_paths.push(path.as_str()),
            _ => return Some("source map has an invalid snapshot"),
        }
    }
    let mismatch = Some("source map snapshots do not match selected_paths");
    let Some(Value::Array(selected)) = selected else {
        return mismatch;
    };
    let mut chosen = HashSet::new();
    for path in selected {
        match path {
            Value::Str(text) if !py::strip(text).is_empty() => chosen.insert(text.as_str()),
            _ => return mismatch,
        };
    }
    let listed: HashSet<&str> = snapshot_paths.iter().copied().collect();
    if chosen.len() != selected.len() || chosen != listed {
        return mismatch;
    }
    None
}

fn inside(root: &Path, path: &str) -> bool {
    let top = py::resolve_path(root);
    let located = py::resolve_path(&top.join(path));
    py::relative_to(&located, &top).is_ok()
}

fn selection_problem(
    root: &Path,
    selected: &[String],
    policy: Option<&Object>,
) -> Result<Option<String>, String> {
    if let Some(escaping) = selected.iter().find(|path| !inside(root, path)) {
        return Ok(Some(format!("source path escapes repository: {escaping}")));
    }
    let recorded = source_policy_of(policy)?.and_then(|section| section.get("selected_paths"));
    if let Some(Value::Array(policy_paths)) = recorded {
        let mut given: Vec<&str> = Vec::new();
        for path in policy_paths {
            match path {
                Value::Str(text) => given.push(text),
                _ => {
                    return Ok(Some(
                        "policy selected_paths do not match the source map".to_string(),
                    ));
                }
            }
        }
        let mut expected: Vec<&str> = selected.iter().map(String::as_str).collect();
        given.sort_unstable();
        expected.sort_unstable();
        if given != expected {
            return Ok(Some(
                "policy selected_paths do not match the source map".to_string(),
            ));
        }
    }
    Ok(None)
}

fn compare_snapshots(
    root: &Path,
    snapshots: &[&Object],
    policy: Option<&Object>,
) -> Result<(Vec<Value>, Vec<Value>), Raised> {
    let paths: Vec<String> = snapshots.iter().map(|item| path_text(item)).collect();
    let actual = source_snapshots(root, &paths, policy)?;
    let (mut changed, mut drifted) = (Vec::new(), Vec::new());
    for snapshot in snapshots {
        let path = path_text(snapshot);
        let current = &actual[&path];
        let mut entry = Object::new();
        entry.insert("path", Value::Str(path.clone()));
        if !root.join(&path).exists() {
            entry.insert("status", Value::Str("missing".to_string()));
            changed.push(Value::Object(entry));
        } else if selection_moved(snapshot, current) {
            entry.insert("status", Value::Str("changed".to_string()));
            entry.insert("expected", Value::Object((*snapshot).clone()));
            entry.insert("actual", current.to_value());
            changed.push(Value::Object(entry));
        } else if snapshot.get("digest").and_then(Value::as_str) != Some(current.digest.as_str()) {
            entry.insert("status", Value::Str("content-changed".to_string()));
            drifted.push(Value::Object(entry));
        }
    }
    Ok((changed, drifted))
}

fn path_text(snapshot: &Object) -> String {
    snapshot
        .get("path")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

fn listed_paths(source_map: &Object) -> Result<Vec<String>, String> {
    let mut listed = Vec::new();
    let groups = match source_map.get("source_groups") {
        None => return Ok(listed),
        Some(Value::Array(groups)) => groups,
        Some(Value::Object(groups)) if groups.iter().next().is_none() => return Ok(listed),
        Some(Value::Str(text)) if text.is_empty() => return Ok(listed),
        Some(other) => {
            return Err(format!(
                "source_groups must be a list, got {}",
                kind_of(other)
            ));
        }
    };
    for group in groups {
        let Some(group) = group.as_object() else {
            return Err(format!(
                "a source group must be an object, got {}",
                kind_of(group)
            ));
        };
        match group.get("paths") {
            None => {}
            Some(Value::Array(paths)) => {
                for path in paths {
                    let Value::Str(text) = path else {
                        return Err(format!("a listed path must be text, got {}", kind_of(path)));
                    };
                    listed.push(text.clone());
                }
            }
            Some(Value::Str(text)) => listed.extend(text.chars().map(String::from)),
            Some(Value::Object(keys)) => listed.extend(keys.iter().map(|(key, _)| key.clone())),
            Some(other) => {
                return Err(format!(
                    "source group paths must be a list, got {}",
                    kind_of(other)
                ));
            }
        }
    }
    Ok(listed)
}

fn discovered_but_gone(root: &Path, source_map: &Object) -> Result<Vec<String>, String> {
    let mut gone: Vec<String> = listed_paths(source_map)?
        .into_iter()
        .filter(|path| !root.join(path).exists())
        .collect();
    gone.sort();
    Ok(gone)
}

pub fn verify_source_map(
    root: &Path,
    source_map_path: &Path,
    policy: Option<&Object>,
) -> Result<Object, Raised> {
    let policy = policy.filter(|policy| policy.iter().next().is_some());
    let root = py::resolve_path(root);
    let source_map = load_json(source_map_path)?;
    let selection_status = source_map
        .get("selection_status")
        .cloned()
        .unwrap_or_else(|| Value::Str("agent-asserted".to_string()));
    if let Some(reason) = snapshot_problem(
        source_map.get("source_snapshots"),
        source_map.get("selected_paths"),
    ) {
        return Ok(verdict("unverified", &selection_status, reason));
    }
    let selected: Vec<String> = texts(
        source_map
            .get("selected_paths")
            .and_then(Value::as_array)
            .unwrap_or_default(),
    )
    .collect();
    if let Some(reason) = selection_problem(&root, &selected, policy)? {
        return Ok(verdict("invalid", &selection_status, &reason));
    }
    let snapshots: Vec<&Object> = source_map
        .get("source_snapshots")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .collect();
    let (changed, drifted) = compare_snapshots(&root, &snapshots, policy)?;
    if let Some(policy) = policy {
        let joined: Vec<PathBuf> = selected.iter().map(|path| root.join(path)).collect();
        let report = source_policy_report(&root, &joined, policy)?;
        let errors = report
            .get("errors")
            .and_then(Value::as_array)
            .unwrap_or_default();
        if !errors.is_empty() {
            let reason: Vec<String> = texts(errors).collect();
            let mut result = verdict("invalid", &selection_status, &reason.join("; "));
            result.insert("source_policy_report", Value::Object(report));
            return Ok(result);
        }
    }
    let status = if changed.is_empty() {
        "current"
    } else {
        "stale"
    };
    let mut result = Object::new();
    result.insert("status", Value::Str(status.to_string()));
    result.insert("selection_status", selection_status);
    result.insert("changed_sources", Value::Array(changed));
    result.insert("content_changed", Value::Array(drifted));
    result.insert(
        "discovered_but_gone",
        strings(&discovered_but_gone(&root, &source_map)?),
    );
    Ok(result)
}

fn sorted_difference(left: &HashSet<String>, right: &HashSet<String>) -> Vec<String> {
    let mut difference: Vec<String> = left.difference(right).cloned().collect();
    difference.sort();
    difference
}

pub fn listing_drift(
    discovered: &Object,
    source_map: &Object,
) -> Result<(Vec<String>, Vec<String>), String> {
    let listed: HashSet<String> = listed_paths(source_map)?.into_iter().collect();
    let current: HashSet<String> = listed_paths(discovered)?.into_iter().collect();
    Ok((
        sorted_difference(&listed, &current),
        sorted_difference(&current, &listed),
    ))
}

pub fn verify_against_discovery(
    root: &Path,
    source_map_path: &Path,
    policy: Option<&Object>,
) -> Result<Object, Raised> {
    let mut result = verify_source_map(root, source_map_path, policy)?;
    if result.get("discovered_but_gone").is_none() {
        return Ok(result);
    }
    let discovered = discover_sources(&py::resolve_path(root), &Profile::of_policy(policy), None)?;
    let (gone, unlisted) = listing_drift(&discovered, &load_json(source_map_path)?)?;
    let drifted = (!gone.is_empty() || !unlisted.is_empty())
        && result.get("status").and_then(Value::as_str) == Some("current");
    if drifted {
        result.insert("status", Value::Str("discovery_drifted".to_string()));
    }
    result.insert("discovered_but_gone", strings(&gone));
    result.insert("discovered_but_unlisted", strings(&unlisted));
    Ok(result)
}

fn strings_of(value: &Value) -> Option<Vec<String>> {
    value
        .as_array()?
        .iter()
        .map(|item| item.as_str().map(str::to_string))
        .collect()
}

fn sorted_strings(mut items: Vec<String>) -> Vec<String> {
    items.sort();
    items
}

type GitState = (Vec<(String, String)>, Vec<String>);

fn git_unless_unavailable(root: &Path, arguments: &[&str]) -> Result<Option<git::Output>, Raised> {
    match git::run_git(root, arguments, None) {
        Ok(output) => Ok(Some(output)),
        Err(error) if error.kind == Failure::Spawn => Ok(None),
        Err(error) => Err(error.into()),
    }
}

fn git_state(root: &Path, selected: &[String]) -> Result<Option<GitState>, Raised> {
    if selected.is_empty() || !root.join(".git").exists() {
        return Ok(None);
    }
    let heads: Vec<String> = selected.iter().map(|path| format!("HEAD:{path}")).collect();
    let mut revisions = vec!["rev-parse"];
    revisions.extend(heads.iter().map(String::as_str));
    let Some(trees) = git_unless_unavailable(root, &revisions)? else {
        return Ok(None);
    };
    let mut status = vec!["status", "--porcelain", "-z", "--"];
    status.extend(selected.iter().map(String::as_str));
    let Some(dirty) = git_unless_unavailable(root, &status)? else {
        return Ok(None);
    };
    if trees.code != 0 || dirty.code != 0 {
        return Ok(None);
    }
    let objects: Vec<String> = git::universal_newlines(&trees.stdout)
        .split_whitespace()
        .map(str::to_string)
        .collect();
    if objects.len() != selected.len() {
        return Ok(None);
    }
    let dirty_sources = sorted_strings(
        git::universal_newlines(&dirty.stdout)
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

fn git_shows_no_change(root: &Path, source_map: &Object) -> Result<bool, Raised> {
    let Some(Value::Object(recorded)) = source_map.get("git_state") else {
        return Ok(false);
    };
    let tracked = recorded.get("tracked_objects");
    if !tracked.is_some_and(truthy) || recorded.get("dirty_sources").is_some_and(truthy) {
        return Ok(false);
    }
    let selected = match source_map.get("selected_paths") {
        None => Vec::new(),
        Some(found) => match strings_of(found) {
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
) -> Result<Option<Object>, Raised> {
    let errors = validate_policy(policy);
    if !errors.is_empty() {
        return Ok(Some(verdict(
            "invalid",
            selection_status,
            &errors.join("; "),
        )));
    }
    let policy_paths = policy
        .get("source_policy")
        .and_then(Value::as_object)
        .and_then(|section| section.get("selected_paths"))
        .and_then(strings_of)
        .unwrap_or_default();
    let recorded = source_map.get("selected_paths").and_then(strings_of);
    if recorded.map(sorted_strings) != Some(sorted_strings(policy_paths.clone())) {
        return Ok(Some(verdict(
            "invalid",
            selection_status,
            "policy selected_paths do not match the source map",
        )));
    }
    let joined: Vec<PathBuf> = policy_paths.iter().map(|path| root.join(path)).collect();
    match source_policy_report(root, &joined, policy) {
        Err(Raised::Subprocess(message)) => Err(Raised::Subprocess(message)),
        Err(Raised::Os(message) | Raised::Value(message)) => {
            Ok(Some(verdict("invalid", selection_status, &message)))
        }
        Ok(report) => {
            let errors: Vec<String> = texts(
                report
                    .get("errors")
                    .and_then(Value::as_array)
                    .unwrap_or_default(),
            )
            .collect();
            if errors.is_empty() {
                return Ok(None);
            }
            let mut result = verdict("invalid", selection_status, &errors.join("; "));
            result.insert("source_policy_report", Value::Object(report));
            Ok(Some(result))
        }
    }
}

pub fn probe_sources(
    root: &Path,
    source_map_path: &Path,
    policy: Option<&Object>,
) -> Result<Object, Raised> {
    if !source_map_path.exists() {
        let mut absent = Object::new();
        absent.insert("status", Value::Str("absent".to_string()));
        absent.insert(
            "reason",
            Value::Str(format!("no source map at {}", py::shown(source_map_path))),
        );
        let hint = one_level_too_deep_hint(source_map_path);
        if !hint.is_empty() {
            let bare = hint.strip_prefix("; ").unwrap_or(&hint);
            absent.insert("hint", Value::Str(bare.to_string()));
        }
        return Ok(absent);
    }
    let source_map = load_json(source_map_path)?;
    let selection_status = source_map
        .get("selection_status")
        .cloned()
        .unwrap_or_else(|| Value::Str("agent-asserted".to_string()));
    if let Some(policy) = policy
        && let Some(invalid) = policy_verdict(root, &source_map, policy, &selection_status)?
    {
        return Ok(invalid);
    }
    let mut result = if git_shows_no_change(root, &source_map)? {
        let mut unchanged = Object::new();
        unchanged.insert("status", Value::Str("current".to_string()));
        unchanged.insert("selection_status", selection_status);
        unchanged.insert("changed_sources", Value::Array(Vec::new()));
        unchanged.insert("checked", Value::Str("git".to_string()));
        unchanged
    } else {
        let mut hashed = verify_source_map(root, source_map_path, policy)?;
        hashed.insert("checked", Value::Str("hash".to_string()));
        hashed
    };
    if result.get("status").and_then(Value::as_str) != Some("current") {
        return Ok(result);
    }
    let discovered = discover_sources(&py::resolve_path(root), &Profile::of_policy(policy), None)?;
    let (gone, unlisted) = listing_drift(&discovered, &source_map)?;
    let drifted = !gone.is_empty() || !unlisted.is_empty();
    let status = if drifted {
        "discovery_drifted"
    } else {
        "current"
    };
    result.insert("status", Value::Str(status.to_string()));
    result.insert("discovered_but_gone", strings(&gone));
    result.insert("discovered_but_unlisted", strings(&unlisted));
    Ok(result)
}

pub fn counted(count: usize, noun: &str) -> String {
    format!("{count} {noun}{}", if count == 1 { "" } else { "s" })
}

fn shown_argument(path: &py::PurePath) -> String {
    let text = path.as_posix();
    if text.contains(' ') {
        format!("\"{text}\"")
    } else {
        text
    }
}

pub fn refresh_listing_advice(registry_root: &py::PurePath, repo_root: &py::PurePath) -> String {
    format!(
        "run refresh-listing --registry-root {} --repo-root {} (no confirmation needed)",
        shown_argument(registry_root),
        shown_argument(repo_root)
    )
}

pub fn discovery_placements(
    repo_root: &Path,
    paths: &[String],
    policy: Option<&Object>,
) -> Result<Vec<(String, String)>, String> {
    let discovered = discover_sources(repo_root, &Profile::of_policy(policy), None)?;
    let kinds = SourceKinds::of(&discovered);
    Ok(paths
        .iter()
        .filter_map(|path| match kinds.kind_of(path) {
            Value::Str(kind) if kind != "unclassified" => Some((path.clone(), kind)),
            _ => None,
        })
        .collect())
}

fn length_of(result: &Object, key: &str) -> usize {
    result
        .get(key)
        .and_then(Value::as_array)
        .map_or(0, <[Value]>::len)
}

pub fn discovery_drift_note(
    result: &Object,
    registry_root: &py::PurePath,
    repo_root: &py::PurePath,
) -> Option<String> {
    let gone = length_of(result, "discovered_but_gone");
    let unlisted = length_of(result, "discovered_but_unlisted");
    if gone == 0 && unlisted == 0 {
        return None;
    }
    let status = result.get("status").map(str_of).unwrap_or_default();
    let advice = if status == "discovery_drifted" {
        refresh_listing_advice(registry_root, repo_root)
    } else {
        format!("the selection is {status}, so run refresh-sources, then confirm-sources")
    };
    let changed_inside = length_of(result, "content_changed");
    let inside = if changed_inside > 0 {
        format!(
            "{} changed inside; verify-evidence checks the citations; ",
            counted(changed_inside, "selected source")
        )
    } else {
        String::new()
    };
    Some(format!(
        "NOTE: discovery drifted: {} gone, {} unlisted; {inside}{advice}",
        counted(gone, "listed path"),
        counted(unlisted, "matching path")
    ))
}

pub fn under(root: &Path, relative: &str) -> PathBuf {
    let joined = root.join(py::PurePath::new(relative).native());
    PathBuf::from(py::PurePath::new(&py::shown(&joined)).native())
}

fn audit_event(operation: &str, extra: Vec<(&str, Value)>) -> Object {
    let mut event = Object::new();
    event.insert("operation", Value::Str(operation.to_string()));
    for (key, value) in extra {
        event.insert(key, value);
    }
    event
}

fn write_and_audit(root: &Path, source_map: &Object, event: &Object) -> Result<(), String> {
    write_source_file(
        &root.join("source-map.json"),
        &Value::Object(source_map.clone()),
    )?;
    append(root, event)
}

fn source_kinds(discovered: &Object, selected: &[String]) -> Object {
    let mut kind_of: HashMap<&str, &str> = HashMap::new();
    for group in discovered
        .get("source_groups")
        .and_then(Value::as_array)
        .unwrap_or_default()
    {
        let Some(group) = group.as_object() else {
            continue;
        };
        let kind = group
            .get("kind")
            .and_then(Value::as_str)
            .unwrap_or("unclassified");
        for path in paths_of(group).iter().filter_map(Value::as_str) {
            kind_of.insert(path, kind);
        }
    }
    let mut kinds = Object::new();
    for path in selected {
        let kind = kind_of.get(path.as_str()).copied();
        kinds.insert(
            path.clone(),
            Value::Str(kind.unwrap_or("unclassified").to_string()),
        );
    }
    kinds
}

fn git_state_object(root: &Path, selected: &[String]) -> Result<Object, Raised> {
    let mut state = Object::new();
    if let Some((objects, dirty)) = git_state(root, selected)? {
        let mut tracked = Object::new();
        for (path, object) in objects {
            tracked.insert(path, Value::Str(object));
        }
        state.insert("tracked_objects", Value::Object(tracked));
        state.insert("dirty_sources", strings(&dirty));
    }
    Ok(state)
}

pub fn selected_source_map(
    root: &Path,
    selected_paths: &[PathBuf],
    policy: Option<&Object>,
    profile: Option<&Profile>,
) -> Result<Object, Raised> {
    let root = py::resolve_path(root);
    let mut chosen: Vec<String> = Vec::new();
    for path in selected_paths {
        let resolved = py::resolve_path(path);
        let named = relative(&root, &resolved).map_err(|_| {
            Raised::Value(format!(
                "source path is outside repository: {}",
                py::shown(path)
            ))
        })?;
        chosen.push(named);
        if !resolved.exists() {
            return Err(Raised::Value(format!(
                "source path does not exist: {}",
                py::shown(path)
            )));
        }
    }
    if chosen.is_empty() {
        return Err(Raised::Value(
            "at least one source path is required".to_string(),
        ));
    }
    let fallback;
    let profile = match profile {
        Some(given) => given,
        None => {
            fallback = Profile::of_policy(policy);
            &fallback
        }
    };
    let mut source_map = discover_sources(&root, profile, None)?;
    source_map.insert("selection_status", Value::Str("agent-asserted".to_string()));
    chosen.sort();
    chosen.dedup();
    source_map.insert("selected_paths", strings(&chosen));
    let joined: Vec<PathBuf> = chosen.iter().map(|path| root.join(path)).collect();
    let files = source_files(&root, &joined, policy)?;
    if let Some(policy) = policy.filter(|policy| policy.iter().next().is_some()) {
        let report = policy_report_of(&root, &files, policy)?;
        let errors: Vec<String> = texts(
            report
                .get("errors")
                .and_then(Value::as_array)
                .unwrap_or_default(),
        )
        .collect();
        if !errors.is_empty() {
            return Err(Raised::Value(format!(
                "source policy limits failed: {}",
                errors.join("; ")
            )));
        }
        source_map.insert("source_policy_report", Value::Object(report));
    }
    let kinds = source_kinds(&source_map, &chosen);
    source_map.insert("source_kinds", Value::Object(kinds));
    source_map.insert(
        "git_state",
        Value::Object(git_state_object(&root, &chosen)?),
    );
    let owned = owned_by_selection(&root, &chosen, &files);
    let mut snapshots = Vec::new();
    for (path, held) in chosen.iter().zip(&owned) {
        snapshots.push(source_snapshot(&root, path, held)?.to_value());
    }
    source_map.insert("source_snapshots", Value::Array(snapshots));
    Ok(source_map)
}

fn no_domain_memory(root: &Path, repo_root: &Path) -> Result<String, String> {
    Ok(format!(
        "no Domain Memory at {}: {}",
        shown_path(root, Some(repo_root)),
        domain_memory_hint(root, Some(repo_root))?
    ))
}

fn completed_identifier(value: &str) -> bool {
    !py::strip(value).is_empty() && !value.starts_with('<')
}

fn status_of(verdict: &Object) -> String {
    verdict.get("status").map(str_of).unwrap_or_default()
}

fn changed_paths(verdict: &Object) -> Vec<String> {
    verdict
        .get("changed_sources")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .map(path_text)
        .collect()
}

pub fn confirm_sources(
    root: &Path,
    repo_root: &Path,
    confirmed_by: &str,
    policy: Option<&Object>,
) -> Result<Object, Raised> {
    let path = root.join("source-map.json");
    if !path.is_file() {
        return Err(no_domain_memory(root, repo_root)?.into());
    }
    if !completed_identifier(confirmed_by) {
        return Err(Raised::Value(
            "confirming sources records who chose them, so it needs the developer's own identity as they gave it. \
             An agent cannot confirm on their behalf, and an identity read out of a git config, a commit or a file \
             in the repository is not the developer answering."
                .to_string(),
        ));
    }
    let freshness = verify_source_map(repo_root, &path, policy)?;
    let status = status_of(&freshness);
    if status == "stale" {
        return Err(Raised::Value(format!(
            "the selected sources have moved since this map was written, so confirming it would attest to a corpus \
             that is no longer there; refresh the map first, then confirm what the developer actually looked at: {}",
            changed_paths(&freshness).join(", ")
        )));
    }
    if status != "current" {
        let reason = freshness
            .get("reason")
            .map_or_else(|| status.clone(), str_of);
        return Err(Raised::Value(format!(
            "the source map does not verify, so there is nothing to confirm yet: {reason}"
        )));
    }
    let mut source_map = load_json(&path)?;
    source_map.insert(
        "selection_status",
        Value::Str("developer-confirmed".to_string()),
    );
    source_map.insert("confirmed_by", Value::Str(confirmed_by.to_string()));
    source_map.insert("confirmed_at", Value::Str(utc_now()));
    let selected = source_map
        .get("selected_paths")
        .cloned()
        .unwrap_or_else(|| Value::Array(Vec::new()));
    let event = audit_event(
        "confirm-sources",
        vec![
            ("confirmed_by", Value::Str(confirmed_by.to_string())),
            ("selected_paths", selected),
        ],
    );
    with_writer_lock(root, || write_and_audit(root, &source_map, &event))?;
    Ok(source_map)
}

fn selected_paths_of(root: &Path, repo_root: &Path) -> Result<Vec<String>, String> {
    let path = root.join("source-map.json");
    if !path.is_file() {
        return Err(no_domain_memory(root, repo_root)?);
    }
    let listed = match load_json(&path)?.get("selected_paths") {
        Some(Value::Array(items)) => items
            .iter()
            .map(|item| item.as_str().filter(|text| !py::strip(text).is_empty()))
            .collect::<Option<Vec<&str>>>()
            .map(|texts| texts.into_iter().map(str::to_string).collect()),
        _ => None,
    };
    listed.ok_or_else(|| "source map has no valid selected paths to refresh".to_string())
}

pub fn adjusted_selection(
    root: &Path,
    repo_root: &Path,
    add: Vec<PathBuf>,
    remove: &[PathBuf],
) -> Result<Vec<PathBuf>, String> {
    let selected = selected_paths_of(root, repo_root)?;
    let repo = py::resolve_path(repo_root);
    let mut removing: Vec<String> = Vec::new();
    for path in remove {
        let named = relative(&repo, &py::resolve_path(path))?;
        if !removing.contains(&named) {
            removing.push(named);
        }
    }
    let mut unselected: Vec<&str> = removing
        .iter()
        .filter(|named| !selected.contains(named))
        .map(String::as_str)
        .collect();
    unselected.sort_unstable();
    if !unselected.is_empty() {
        return Err(format!(
            "cannot remove a source that is not selected: {}",
            unselected.join(", ")
        ));
    }
    let mut kept: Vec<PathBuf> = selected
        .iter()
        .filter(|item| !removing.contains(item))
        .map(|item| under(&repo, item))
        .collect();
    kept.extend(add);
    Ok(kept)
}

pub fn refresh_sources(
    root: &Path,
    repo_root: &Path,
    policy: Option<&Object>,
) -> Result<Object, Raised> {
    let selected = selected_paths_of(root, repo_root)?;
    let joined: Vec<PathBuf> = selected.iter().map(|item| under(repo_root, item)).collect();
    let source_map = selected_source_map(repo_root, &joined, policy, None)?;
    let event = audit_event(
        "refresh-sources",
        vec![(
            "selected_paths",
            source_map
                .get("selected_paths")
                .cloned()
                .unwrap_or(Value::Null),
        )],
    );
    with_writer_lock(root, || write_and_audit(root, &source_map, &event))?;
    Ok(source_map)
}

fn selection_trouble(verdict: &Object) -> String {
    let reason = verdict
        .get("reason")
        .map(str_of)
        .filter(|reason| !reason.is_empty());
    let changed = changed_paths(verdict).join(", ");
    reason
        .or_else(|| Some(changed).filter(|changed| !changed.is_empty()))
        .unwrap_or_else(|| status_of(verdict))
}

pub struct ListingChange {
    pub added: Vec<String>,
    pub removed: Vec<String>,
}

pub fn refresh_listing(
    root: &Path,
    repo_root: &Path,
    policy: Option<&Object>,
) -> Result<ListingChange, Raised> {
    let path = root.join("source-map.json");
    if !path.is_file() {
        return Err(no_domain_memory(root, repo_root)?.into());
    }
    let verdict = verify_source_map(repo_root, &path, policy)?;
    if status_of(&verdict) != "current" {
        return Err(Raised::Value(format!(
            "the selection is {}: {}; run refresh-sources, then confirm-sources",
            status_of(&verdict),
            selection_trouble(&verdict)
        )));
    }
    let mut source_map = load_json(&path)?;
    let discovered = discover_sources(repo_root, &Profile::of_policy(policy), None)?;
    let listed: HashSet<String> = listed_paths(&source_map)?.into_iter().collect();
    let current: HashSet<String> = listed_paths(&discovered)?.into_iter().collect();
    let change = ListingChange {
        added: sorted_difference(&current, &listed),
        removed: sorted_difference(&listed, &current),
    };
    if change.added.is_empty() && change.removed.is_empty() {
        return Ok(change);
    }
    for key in ["source_groups", "missing_groups"] {
        source_map.insert(key, discovered.get(key).cloned().unwrap_or(Value::Null));
    }
    let event = audit_event(
        "refresh-listing",
        vec![
            ("added", strings(&change.added)),
            ("removed", strings(&change.removed)),
        ],
    );
    with_writer_lock(root, || write_and_audit(root, &source_map, &event))?;
    Ok(change)
}

pub fn refine_sources(
    root: &Path,
    repo_root: &Path,
    selected_paths: &[PathBuf],
    policy: &Object,
) -> Result<Object, Raised> {
    if selected_paths.is_empty() {
        return Err(Raised::Value(
            "refining sources requires at least one focused path".to_string(),
        ));
    }
    let source_map = selected_source_map(repo_root, selected_paths, Some(policy), None)?;
    let mut policy = policy.clone();
    let chosen = source_map
        .get("selected_paths")
        .cloned()
        .unwrap_or(Value::Null);
    match policy.get_mut("source_policy") {
        Some(Value::Object(section)) => section.insert("selected_paths", chosen.clone()),
        _ => return Err(Raised::Value("'source_policy'".to_string())),
    }
    let event = audit_event("refine-sources", vec![("selected_paths", chosen)]);
    with_writer_lock(root, || {
        write_json(&crate::policy::policy_path(root), &Value::Object(policy))?;
        write_and_audit(root, &source_map, &event)
    })?;
    Ok(source_map)
}

pub fn refresh_confirmation_notice(selected_paths: &[String]) -> String {
    let count = selected_paths.len();
    let demonstrative = if count == 1 { "this" } else { "these" };
    format!(
        "Selection reset to agent-asserted; confirm {demonstrative} {}: {}",
        counted(count, "selected source"),
        selected_paths.join(", ")
    )
}
