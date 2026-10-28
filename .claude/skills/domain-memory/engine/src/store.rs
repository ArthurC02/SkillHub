use std::cmp::Ordering;
use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use crate::files::load_json;
use crate::git::{self, Failure};
use crate::json::{Object, Value};
use crate::okf;
use crate::py;
use crate::tar;

pub const ASSETS: &[(&str, &str, &str)] = &[
    ("contexts", "contexts", "domain-contexts/v1"),
    ("vocabulary", "terms", "domain-vocabulary/v1"),
    ("aggregates", "aggregates", "domain-aggregates/v1"),
    ("rules", "rules", "domain-rules/v1"),
    ("contracts", "contracts", "domain-contracts/v1"),
    ("interactions", "interactions", "domain-interactions/v1"),
    ("decisions", "decisions", "domain-decisions/v1"),
    ("events", "events", "domain-events/v1"),
    ("capabilities", "capabilities", "domain-capabilities/v1"),
    ("value-objects", "value_objects", "domain-value-objects/v1"),
    (
        "dependency-policies",
        "dependencies",
        "domain-dependencies/v1",
    ),
];

const INDEX: &str = "index.md";
pub const DEFAULT_STATUS: &str = "candidate";
pub const SOURCE_MAP_FORMAT: &str = "domain-source-map/v1";
const DOMAIN_MEMORY_HINT: &str = "check --registry-root (a Registry holds registry/index.md, \
     or a domain-source-map/v1 source-map.json); initialize one only if this repository has none yet";

pub type Files = BTreeMap<String, Vec<u8>>;

pub fn registry_dir(root: &Path) -> PathBuf {
    root.join("registry")
}

pub fn has_registry(root: &Path) -> bool {
    registry_dir(root).join(INDEX).is_file()
}

pub fn read_documents(root: &Path) -> Result<Object, String> {
    if !has_registry(root) {
        return Err(missing_registry(root)?);
    }
    let held = registry_files(root)?;
    let mut documents = Object::new();
    for (asset, key, format) in ASSETS {
        documents.insert(
            *asset,
            asset_document(asset, key, format, concept_texts(asset, &held)?)?,
        );
    }
    Ok(documents)
}

pub fn asset_records(root: &Path, asset: &str) -> Result<Vec<Value>, String> {
    known_asset(asset)?;
    Registry::open(root)?.records(asset)
}

fn known_asset(asset: &str) -> Result<&'static (&'static str, &'static str, &'static str), String> {
    ASSETS
        .iter()
        .find(|(name, _, _)| *name == asset)
        .ok_or_else(|| format!("unknown asset {asset}"))
}

pub struct Registry {
    files: Files,
}

impl Registry {
    pub fn files(&self) -> &Files {
        &self.files
    }

    pub fn open(root: &Path) -> Result<Self, String> {
        if !has_registry(root) {
            return Err(missing_registry(root)?);
        }
        Ok(Self {
            files: registry_files(root)?,
        })
    }

    pub fn records(&self, asset: &str) -> Result<Vec<Value>, String> {
        let (_, key, format) = known_asset(asset)?;
        let document = asset_document(asset, key, format, concept_texts(asset, &self.files)?)?;
        Ok(document
            .as_object()
            .and_then(|document| document.get(key))
            .and_then(Value::as_array)
            .map(<[Value]>::to_vec)
            .unwrap_or_default())
    }
}

pub fn committed_documents(
    repo_root: &Path,
    commit: &str,
    root: &Path,
) -> Result<Option<Object>, String> {
    let registry = py::relative_to(
        &py::resolve_path(&registry_dir(root)),
        &py::resolve_path(repo_root),
    )?;
    let listing = git::run_git_bytes(
        repo_root,
        &[
            "ls-tree",
            "-r",
            "--name-only",
            "-z",
            commit,
            "--",
            &registry,
        ],
    )?;
    if listing.code != 0 || listing.stdout.trim_ascii().is_empty() {
        return Ok(None);
    }
    let archive = git::run_git_bytes(
        repo_root,
        &["archive", "--format=tar", commit, "--", &registry],
    )?;
    if archive.code != 0 {
        let stderr = String::from_utf8_lossy(&archive.stderr);
        let reason = py::strip(&stderr);
        let reason = if reason.is_empty() {
            archive.code.to_string()
        } else {
            reason.to_string()
        };
        return Err(format!("git could not read {commit}: {reason}"));
    }
    let mut contents = BTreeMap::new();
    for member in tar::regular_files(&archive.stdout)? {
        contents.insert(member.name, py::utf8_strict(&member.data)?);
    }
    if !contents.contains_key(&format!("{registry}/{INDEX}")) {
        return Ok(None);
    }
    let mut documents = Object::new();
    for (asset, key, format) in ASSETS {
        let prefix = format!("{registry}/{asset}/");
        let mut concepts: Vec<(String, String)> = contents
            .iter()
            .filter_map(|(path, text)| {
                let rest = path.strip_prefix(&prefix)?;
                let stem = rest.strip_suffix(".md")?;
                (!rest.contains('/') && rest != INDEX).then(|| (stem.to_string(), text.clone()))
            })
            .collect();
        concepts.sort();
        documents.insert(*asset, asset_document(asset, key, format, concepts)?);
    }
    Ok(Some(documents))
}

fn concept_texts(asset: &str, held: &Files) -> Result<Vec<(String, String)>, String> {
    let mut concepts: Vec<(&str, &str)> = held
        .keys()
        .filter_map(|relative| concept_stem(asset, relative).map(|stem| (stem, relative.as_str())))
        .collect();
    concepts.sort_unstable();
    concepts
        .iter()
        .map(|(stem, relative)| {
            py::utf8_text(&held[*relative])
                .map(|text| ((*stem).to_string(), text))
                .map_err(|error| format!("{relative}: {error}"))
        })
        .collect()
}

fn asset_document(
    asset: &str,
    key: &str,
    format: &str,
    texts: Vec<(String, String)>,
) -> Result<Value, String> {
    let records = texts
        .iter()
        .map(|(stem, text)| {
            okf::decode(asset, stem, text)
                .map(Value::Object)
                .map_err(|error| format!("{asset}/{stem}.md: {error}"))
        })
        .collect::<Result<Vec<_>, _>>()?;
    let mut document = Object::new();
    document.insert("format", Value::Str(format.to_string()));
    document.insert("status", Value::Str(DEFAULT_STATUS.to_string()));
    document.insert(key, Value::Array(records));
    Ok(Value::Object(document))
}

fn concept_stem<'a>(asset: &str, relative: &'a str) -> Option<&'a str> {
    let (parent, name) = relative.rsplit_once('/')?;
    if parent != asset || name == INDEX {
        return None;
    }
    name.strip_suffix(".md")
}

pub fn registry_files(root: &Path) -> Result<Files, String> {
    let directory = registry_dir(root);
    let mut paths = Vec::new();
    walk(&directory, &mut paths);
    paths.sort_by(|left, right| py::compare_paths(left, right));
    let mut files = Files::new();
    for path in paths {
        if !path.is_file() {
            continue;
        }
        let data = std::fs::read(&path).map_err(|error| py::os_error(&error, &path))?;
        let relative = path.strip_prefix(&directory).map(posix).unwrap_or_default();
        files.insert(relative, data);
    }
    Ok(files)
}

fn walk(directory: &Path, paths: &mut Vec<PathBuf>) {
    let Ok(entries) = std::fs::read_dir(directory) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if entry.file_type().is_ok_and(|kind| kind.is_dir()) {
            walk(&path, paths);
        }
        paths.push(path);
    }
}

pub fn posix(path: &Path) -> String {
    let shown = py::shown(path);
    if cfg!(windows) {
        shown.replace('\\', "/")
    } else {
        shown
    }
}

pub fn missing_registry(root: &Path) -> Result<String, String> {
    missing_registry_for(root, None)
}

pub fn missing_registry_for(root: &Path, repo_root: Option<&Path>) -> Result<String, String> {
    Ok(format!(
        "no Domain Registry at {}: it holds no registry/{INDEX}; {}",
        shown_path(root, repo_root),
        domain_memory_hint(root, repo_root)?
    ))
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
    posix(&resolved)
}

pub fn domain_memory_hint(root: &Path, repo_root: Option<&Path>) -> Result<String, String> {
    let asked = py::resolve_path(root);
    let places: Vec<String> = candidates(&asked, repo_root)?
        .into_iter()
        .filter(|place| py::compare_paths(place, &asked) != Ordering::Equal)
        .map(|place| shown_path(&place, repo_root))
        .collect();
    Ok(match places.as_slice() {
        [] => DOMAIN_MEMORY_HINT.to_string(),
        [only] => format!("{DOMAIN_MEMORY_HINT}; did you mean {only}?"),
        many => format!(
            "{DOMAIN_MEMORY_HINT}; Domain Memories in this repository: {}",
            many.join(", ")
        ),
    })
}

fn candidates(asked: &Path, repo_root: Option<&Path>) -> Result<Vec<PathBuf>, String> {
    if let Some(repo_root) = repo_root
        && let Listed::Places(listed) = domain_memories_in(repo_root)?
    {
        return Ok(listed);
    }
    Ok(neighbours(asked)?
        .into_iter()
        .filter(|place| holds_domain_memory(place))
        .collect())
}

pub enum Listed {
    Places(Vec<PathBuf>),
    Refused(String),
}

pub fn domain_memories_in(repo_root: &Path) -> Result<Listed, String> {
    let listed = match git::run_git(
        repo_root,
        &[
            "ls-files",
            "-z",
            "--cached",
            "--others",
            "--exclude-standard",
            "--",
            ":(glob)**/source-map.json",
            ":(glob)**/registry/index.md",
        ],
        None,
    ) {
        Ok(listed) => listed,
        Err(error) if error.kind == Failure::Spawn => return Ok(Listed::Refused(error.message)),
        Err(error) => return Err(error.message),
    };
    if listed.code != 0 {
        let stderr = git::universal_newlines(&listed.stderr);
        return Ok(Listed::Refused(py::strip(&stderr).to_string()));
    }
    let mut homes: Vec<PathBuf> = Vec::new();
    for entry in String::from_utf8_lossy(&listed.stdout).split('\0') {
        if entry.is_empty() {
            continue;
        }
        let marker = repo_root.join(entry);
        let is_map = marker
            .file_name()
            .is_some_and(|name| name == "source-map.json");
        if is_map && !is_domain_source_map(&marker) {
            continue;
        }
        let home = if is_map {
            marker.parent()
        } else {
            marker.parent().and_then(Path::parent)
        };
        let resolved = py::resolve_path(home.unwrap_or(&marker));
        if !homes
            .iter()
            .any(|seen| py::compare_paths(seen, &resolved) == Ordering::Equal)
        {
            homes.push(resolved);
        }
    }
    homes.sort_by(|left, right| py::compare_paths(left, right));
    Ok(Listed::Places(homes))
}

pub fn neighbours(asked: &Path) -> Result<Vec<PathBuf>, String> {
    let parent = asked.parent().unwrap_or(asked).to_path_buf();
    let mut siblings = directories_in(&parent)?;
    siblings.retain(|place| py::compare_paths(place, asked) != Ordering::Equal);
    let children = directories_in(asked)?;
    Ok(std::iter::once(parent)
        .chain(siblings)
        .chain(children)
        .collect())
}

pub fn directories_in(directory: &Path) -> Result<Vec<PathBuf>, String> {
    if !directory.is_dir() {
        return Ok(Vec::new());
    }
    let entries = std::fs::read_dir(directory).map_err(|error| py::os_error(&error, directory))?;
    let mut found = Vec::new();
    for entry in entries {
        let path = entry
            .map_err(|error| py::os_error(&error, directory))?
            .path();
        if path.is_dir() {
            found.push(path);
        }
    }
    found.sort_by(|left, right| py::compare_paths(left, right));
    Ok(found)
}

fn is_domain_source_map(path: &Path) -> bool {
    load_json(path)
        .is_ok_and(|map| map.get("format").and_then(Value::as_str) == Some(SOURCE_MAP_FORMAT))
}

pub fn holds_domain_memory(place: &Path) -> bool {
    is_domain_source_map(&place.join("source-map.json"))
        || registry_dir(place).join(INDEX).is_file()
}
