use std::fs::File;
use std::io::Write;
use std::path::{Path, PathBuf};

use crate::json::{self, Object, Style, Value};
use crate::py::{self, shown};

pub const MAX_JSON_BYTES: u64 = 16 * 1024 * 1024;
pub const PROPOSAL_FILE: &str = "domain-change-proposal.json";

pub fn load_json(path: &Path) -> Result<Object, String> {
    if !path.is_file() {
        return Err(format!(
            "JSON file required: {}{}",
            shown(path),
            one_level_too_deep_hint(path)
        ));
    }
    if path.metadata().map(|meta| meta.len()).unwrap_or(0) > MAX_JSON_BYTES {
        return Err(format!(
            "JSON file exceeds {MAX_JSON_BYTES} bytes: {}",
            shown(path)
        ));
    }
    let bytes =
        std::fs::read(path).map_err(|_| format!("cannot read JSON file: {}", shown(path)))?;
    let invalid = || format!("invalid JSON: {}", shown(path));
    let text = String::from_utf8(bytes).map_err(|_| invalid())?;
    match json::parse(&text).map_err(|()| invalid())? {
        Value::Object(object) => Ok(object),
        _ => Err(format!("JSON object required: {}", shown(path))),
    }
}

pub fn one_level_too_deep_hint(path: &Path) -> String {
    let (Some(parent), Some(name)) = (path.parent().and_then(Path::parent), path.file_name())
    else {
        return String::new();
    };
    let nearby = parent.join(name);
    if !nearby.is_file() {
        return String::new();
    }
    format!(
        "; {} exists, so a root option probably points one directory too deep: a registry root \
         holds registry/, source-map.json, and the policy",
        shown(&nearby)
    )
}

const REPLACE_DELAYS_MILLIS: [u64; 5] = [50, 100, 200, 400, 800];

pub fn write_source_file(path: &Path, value: &Value) -> Result<(), String> {
    if let Some(parent) = path.parent() {
        make_directories(parent)?;
    }
    write_json(path, value)
}

fn make_directories(path: &Path) -> Result<(), String> {
    match std::fs::create_dir(path) {
        Ok(()) => Ok(()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            let Some(parent) = path.parent().filter(|parent| *parent != path) else {
                return Err(py::os_error(&error, path));
            };
            make_directories(parent)?;
            std::fs::create_dir(path).or_else(|error| already_a_directory(path, &error))
        }
        Err(error) => already_a_directory(path, &error),
    }
}

fn already_a_directory(path: &Path, error: &std::io::Error) -> Result<(), String> {
    if path.is_dir() {
        Ok(())
    } else {
        Err(py::os_error(error, path))
    }
}

pub fn write_json(path: &Path, value: &Value) -> Result<(), String> {
    let temporary = PathBuf::from(format!("{}.tmp", shown(path)));
    let text = format!("{}\n", json::dumps(value, Style::PRINTED));
    let mut output =
        File::create(&temporary).map_err(|error| py::open_error(&error, &temporary))?;
    output
        .write_all(text.as_bytes())
        .and_then(|()| output.sync_all())
        .map_err(|error| py::os_error(&error, &temporary))?;
    drop(output);
    replace_path(&temporary, path)
        .inspect_err(|(denied, _)| {
            if *denied {
                let _ = std::fs::remove_file(&temporary);
            }
        })
        .map_err(|(_, message)| message)
}

pub fn rename_retrying(source: &Path, target: &Path) -> std::io::Result<()> {
    if cfg!(windows) && target.is_dir() {
        return Err(std::io::Error::from_raw_os_error(5));
    }
    for delay in REPLACE_DELAYS_MILLIS.iter().map(Some).chain([None]) {
        match std::fs::rename(source, target) {
            Ok(()) => return Ok(()),
            Err(error) => {
                let Some(delay) =
                    delay.filter(|_| error.kind() == std::io::ErrorKind::PermissionDenied)
                else {
                    return Err(error);
                };
                std::thread::sleep(std::time::Duration::from_millis(*delay));
            }
        }
    }
    Ok(())
}

pub fn replace_error(error: &std::io::Error, source: &Path, target: &Path) -> String {
    format!(
        "{} -> {}",
        py::os_error(error, source),
        py::repr(&shown(target))
    )
}

fn replace_path(source: &Path, target: &Path) -> Result<(), (bool, String)> {
    rename_retrying(source, target).map_err(|error| {
        (
            error.kind() == std::io::ErrorKind::PermissionDenied,
            replace_error(&error, source, target),
        )
    })
}
