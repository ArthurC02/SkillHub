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

pub fn make_directories(path: &Path) -> Result<(), String> {
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

#[cfg(test)]
mod tests {
    use super::*;

    const OLD: &str = "{\"old\": true}\n";

    fn scratch(label: &str) -> PathBuf {
        let root = std::env::temp_dir().join(format!("dm-files-{label}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(&root).unwrap();
        root
    }

    #[cfg(windows)]
    fn new_value() -> Value {
        let mut object = Object::new();
        object.insert("new", Value::Bool(true));
        Value::Object(object)
    }

    fn entries(directory: &Path) -> Vec<String> {
        let mut names: Vec<String> = std::fs::read_dir(directory)
            .unwrap()
            .map(|entry| entry.unwrap().file_name().to_string_lossy().into_owned())
            .collect();
        names.sort();
        names
    }

    #[cfg(windows)]
    fn set_refusing(target: &Path, refusing: bool) {
        let mut permissions = std::fs::metadata(target).unwrap().permissions();
        permissions.set_readonly(refusing);
        std::fs::set_permissions(target, permissions).unwrap();
    }

    #[cfg(windows)]
    #[test]
    fn a_replace_refused_for_a_moment_is_retried() {
        let root = scratch("retried");
        let target = root.join("value.json");
        std::fs::write(&target, OLD).unwrap();
        set_refusing(&target, true);
        let releasing = target.clone();
        let release = std::thread::spawn(move || {
            std::thread::sleep(std::time::Duration::from_millis(60));
            set_refusing(&releasing, false);
        });
        let started = std::time::Instant::now();
        let outcome = write_json(&target, &new_value());
        let waited = started.elapsed();
        release.join().unwrap();
        let content = std::fs::read_to_string(&target).unwrap();
        let _ = std::fs::remove_dir_all(&root);
        assert_eq!(
            (
                outcome,
                content.as_str(),
                waited >= std::time::Duration::from_millis(60)
            ),
            (Ok(()), "{\n  \"new\": true\n}\n", true)
        );
    }

    #[cfg(windows)]
    #[test]
    fn a_replace_refused_every_time_fails_and_leaves_no_temporary_file() {
        let root = scratch("refused");
        let target = root.join("value.json");
        std::fs::write(&target, OLD).unwrap();
        set_refusing(&target, true);
        let outcome = write_json(&target, &new_value());
        set_refusing(&target, false);
        let (names, content) = (entries(&root), std::fs::read_to_string(&target).unwrap());
        let _ = std::fs::remove_dir_all(&root);
        let message = outcome.unwrap_err();
        assert!(message.starts_with("[WinError 5] "), "{message}");
        assert!(message.ends_with("value.json'"), "{message}");
        assert_eq!(
            (names, content.as_str()),
            (vec!["value.json".to_string()], OLD)
        );
    }

    #[cfg(unix)]
    fn set_refusing(directory: &Path, refusing: bool) {
        use std::os::unix::fs::PermissionsExt;
        let mode = if refusing { 0o500 } else { 0o700 };
        std::fs::set_permissions(directory, std::fs::Permissions::from_mode(mode)).unwrap();
    }

    #[cfg(unix)]
    fn refusing_directory_binds(directory: &Path) -> bool {
        std::fs::File::create(directory.join("probe")).is_err()
    }

    #[cfg(unix)]
    #[test]
    fn a_rename_refused_for_a_moment_is_retried() {
        let root = scratch("retried");
        let (source, target) = (root.join("value.json.tmp"), root.join("value.json"));
        std::fs::write(&source, "new\n").unwrap();
        std::fs::write(&target, OLD).unwrap();
        set_refusing(&root, true);
        if !refusing_directory_binds(&root) {
            eprintln!("running as root: permissions do not stop the rename, test skipped");
            set_refusing(&root, false);
            let _ = std::fs::remove_dir_all(&root);
            return;
        }
        let releasing = root.clone();
        let release = std::thread::spawn(move || {
            std::thread::sleep(std::time::Duration::from_millis(60));
            set_refusing(&releasing, false);
        });
        let started = std::time::Instant::now();
        let outcome = rename_retrying(&source, &target);
        let waited = started.elapsed();
        release.join().unwrap();
        let content = std::fs::read_to_string(&target).unwrap();
        let _ = std::fs::remove_dir_all(&root);
        assert_eq!(
            (
                outcome.is_ok(),
                content.as_str(),
                waited >= std::time::Duration::from_millis(60)
            ),
            (true, "new\n", true)
        );
    }

    #[cfg(unix)]
    #[test]
    fn a_rename_refused_every_time_fails_with_permission_denied_and_changes_nothing() {
        let root = scratch("refused");
        let (source, target) = (root.join("value.json.tmp"), root.join("value.json"));
        std::fs::write(&source, "new\n").unwrap();
        std::fs::write(&target, OLD).unwrap();
        set_refusing(&root, true);
        if !refusing_directory_binds(&root) {
            eprintln!("running as root: permissions do not stop the rename, test skipped");
            set_refusing(&root, false);
            let _ = std::fs::remove_dir_all(&root);
            return;
        }
        let outcome = rename_retrying(&source, &target);
        set_refusing(&root, false);
        let (names, content) = (entries(&root), std::fs::read_to_string(&target).unwrap());
        let _ = std::fs::remove_dir_all(&root);
        assert_eq!(
            (outcome.unwrap_err().kind(), names, content.as_str()),
            (
                std::io::ErrorKind::PermissionDenied,
                vec!["value.json".to_string(), "value.json.tmp".to_string()],
                OLD
            )
        );
    }
}
