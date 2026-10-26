use std::cmp::Ordering;
use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use crate::files::load_json;
use crate::json::{Object, Value};
use crate::okf;
use crate::py;

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
const DEFAULT_STATUS: &str = "candidate";
const SOURCE_MAP_FORMAT: &str = "domain-source-map/v1";
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
    let mut documents = Object::new();
    for (asset, key, format) in ASSETS {
        if !has_registry(root) {
            return Err(missing_registry(root)?);
        }
        let held = registry_files(root)?;
        documents.insert(*asset, asset_document(asset, key, format, &held)?);
    }
    Ok(documents)
}

fn asset_document(asset: &str, key: &str, format: &str, held: &Files) -> Result<Value, String> {
    let mut concepts: Vec<(&str, &str)> = held
        .keys()
        .filter_map(|relative| concept_stem(asset, relative).map(|stem| (stem, relative.as_str())))
        .collect();
    concepts.sort_unstable();
    let texts = concepts
        .iter()
        .map(|(stem, relative)| {
            py::utf8_text(&held[*relative])
                .map(|text| (*stem, text))
                .map_err(|error| format!("{relative}: {error}"))
        })
        .collect::<Result<Vec<_>, _>>()?;
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
    Ok(format!(
        "no Domain Registry at {}: it holds no registry/{INDEX}; {}",
        posix(&py::resolve_path(root)),
        domain_memory_hint(root)?
    ))
}

fn domain_memory_hint(root: &Path) -> Result<String, String> {
    let asked = py::resolve_path(root);
    let same = |place: &Path| py::compare_paths(place, &asked) == Ordering::Equal;
    let places: Vec<String> = neighbours(&asked)?
        .into_iter()
        .filter(|place| holds_domain_memory(place) && !same(place))
        .map(|place| posix(&py::resolve_path(&place)))
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

fn neighbours(asked: &Path) -> Result<Vec<PathBuf>, String> {
    let parent = asked.parent().unwrap_or(asked).to_path_buf();
    let mut siblings = directories_in(&parent)?;
    siblings.retain(|place| py::compare_paths(place, asked) != Ordering::Equal);
    let children = directories_in(asked)?;
    Ok(std::iter::once(parent)
        .chain(siblings)
        .chain(children)
        .collect())
}

fn directories_in(directory: &Path) -> Result<Vec<PathBuf>, String> {
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

fn holds_domain_memory(place: &Path) -> bool {
    let source_map = load_json(&place.join("source-map.json"))
        .is_ok_and(|map| map.get("format").and_then(Value::as_str) == Some(SOURCE_MAP_FORMAT));
    source_map || registry_dir(place).join(INDEX).is_file()
}
