use std::path::Path;

use crate::candidate_evidence::without_derived_keys;
use crate::indexes::{asset_index, root_index};
use crate::json::{Object, Value};
use crate::okf_write::encode;
use crate::py;
use crate::pyvalue::repr_of;
use crate::store::{ASSETS, Registry, registry_dir};

const INDEX: &str = "index.md";

pub fn asset_names() -> impl Iterator<Item = &'static str> {
    ASSETS.iter().map(|(asset, _, _)| *asset)
}

pub fn read_records(root: &Path, asset: &str) -> Result<Vec<Object>, String> {
    Ok(Registry::open(root)?
        .records(asset)?
        .into_iter()
        .filter_map(|record| record.as_object().cloned())
        .collect())
}

pub fn is_reviewed(asset: &str, record: &Object) -> bool {
    record
        .get(crate::okf::review_field(asset))
        .is_some_and(|status| status.as_str() == Some("reviewed"))
}

pub fn position_of(records: &[Object], record_id: &str) -> Option<usize> {
    records
        .iter()
        .position(|record| record.get("id").and_then(Value::as_str) == Some(record_id))
}

fn concept_file_name(asset: &str, record_id: Option<&Value>) -> Result<String, String> {
    let usable = record_id.and_then(Value::as_str).filter(|id| {
        *id != "index"
            && id.starts_with(|first: char| first.is_ascii_alphanumeric())
            && id
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '_' | '-'))
    });
    match usable {
        Some(id) => Ok(format!("{id}.md")),
        None => Err(format!(
            "{asset} record id {} cannot name a concept file; use letters, digits, '.', '_' or '-'",
            record_id.map_or_else(|| "None".to_string(), repr_of)
        )),
    }
}

fn write_text(path: &Path, text: &str) -> Result<(), String> {
    std::fs::write(path, text).map_err(|error| py::open_error(&error, path))
}

pub fn write_records(root: &Path, asset: &str, records: &[Object]) -> Result<(), String> {
    let directory = registry_dir(root).join(asset);
    std::fs::create_dir_all(&directory).map_err(|error| py::os_error(&error, &directory))?;
    let mut written = Vec::new();
    for record in records {
        let file_name = concept_file_name(asset, record.get("id"))?;
        let plain = without_derived_keys(&Value::Object(record.clone()))?;
        let Value::Object(plain) = plain else {
            continue;
        };
        written.push((file_name, encode(asset, &plain)?));
    }
    let listing =
        std::fs::read_dir(&directory).map_err(|error| py::os_error(&error, &directory))?;
    for entry in listing {
        let path = entry
            .map_err(|error| py::os_error(&error, &directory))?
            .path();
        let name = path
            .file_name()
            .map(|name| name.to_string_lossy().into_owned())
            .unwrap_or_default();
        if name.ends_with(".md") && name != INDEX && !written.iter().any(|(kept, _)| *kept == name)
        {
            std::fs::remove_file(&path).map_err(|error| py::os_error(&error, &path))?;
        }
    }
    for (file_name, text) in &written {
        write_text(&directory.join(file_name), text)?;
    }
    write_text(&directory.join(INDEX), &asset_index(asset, records))
}

pub fn without(record: &Object, dropped: &[&str]) -> Object {
    let mut kept = Object::new();
    for (key, value) in record.iter() {
        if !dropped.contains(&key.as_str()) {
            kept.insert(key.clone(), value.clone());
        }
    }
    kept
}

fn create_concept_layout(root: &Path) -> Result<(), String> {
    let registry = registry_dir(root);
    std::fs::create_dir_all(&registry).map_err(|error| py::os_error(&error, &registry))?;
    write_text(&registry.join(INDEX), &root_index())?;
    asset_names().try_for_each(|asset| write_records(root, asset, &[]))
}

pub fn init_registry(output: &Path) -> Result<(), String> {
    if output.exists() {
        let mut entries =
            std::fs::read_dir(output).map_err(|error| py::os_error(&error, output))?;
        if entries.next().is_some() {
            return Err(format!(
                "output directory is not empty: {}",
                py::shown(output)
            ));
        }
    }
    create_concept_layout(output)
}
