use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use crate::audit_append;
use crate::audit_log::read_events;
use crate::files::{MAX_JSON_BYTES, rename_retrying, replace_error, write_json};
use crate::json::{self, Fault, Object, Value};
use crate::listing::Repository;
use crate::py;
use crate::sha256::hex_digest;
use crate::store::registry_dir;
use crate::validate::validate;

const JOURNAL_FORMAT: &str = "domain-registry-transaction/v2";
const MANUAL_RECONCILIATION: &str = "registry transaction requires manual reconciliation";

pub fn transaction_path(root: &Path) -> PathBuf {
    root.join(".domain-registry-transaction.json")
}

fn lock_path(root: &Path) -> PathBuf {
    root.join(".domain-registry.lock")
}

fn replaced(source: &Path, target: &Path) -> Result<(), String> {
    rename_retrying(source, target).map_err(|error| replace_error(&error, source, target))
}

fn remove_tree(path: &Path) -> Result<(), String> {
    std::fs::remove_dir_all(path).map_err(|error| py::os_error(&error, path))
}

fn unlink_if_present(path: &Path) -> Result<(), String> {
    match std::fs::remove_file(path) {
        Err(error) if error.kind() != std::io::ErrorKind::NotFound => {
            Err(py::os_error(&error, path))
        }
        _ => Ok(()),
    }
}

fn copy_tree(source: &Path, target: &Path) -> Result<(), String> {
    let entries = std::fs::read_dir(source).map_err(|error| py::os_error(&error, source))?;
    let mut listed = Vec::new();
    for entry in entries {
        listed.push(entry.map_err(|error| py::os_error(&error, source))?.path());
    }
    std::fs::create_dir_all(target).map_err(|error| py::os_error(&error, target))?;
    for path in listed {
        let Some(name) = path.file_name() else {
            continue;
        };
        let destination = target.join(name);
        if path.is_dir() {
            copy_tree(&path, &destination)?;
        } else {
            std::fs::copy(&path, &destination)
                .map_err(|error| replace_error(&error, &path, &destination))?;
        }
    }
    Ok(())
}

fn with_writer_lock<T>(root: &Path, work: impl FnOnce() -> Result<T, String>) -> Result<T, String> {
    let lock = lock_path(root);
    match std::fs::create_dir(&lock) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {
            return Err(format!(
                "another Domain Registry update is in progress: {}",
                py::shown(&lock)
            ));
        }
        Err(error) => return Err(py::os_error(&error, &lock)),
    }
    let outcome = work();
    std::fs::remove_dir(&lock).map_err(|error| py::os_error(&error, &lock))?;
    outcome
}

fn journal_document(journal: &Path) -> Result<Value, String> {
    let size = journal.metadata().map(|meta| meta.len()).unwrap_or(0);
    if size > MAX_JSON_BYTES {
        return Err("registry recovery journal is too large".to_string());
    }
    let bytes = std::fs::read(journal).map_err(|error| py::os_error(&error, journal))?;
    match json::parse_naming_duplicate(&py::utf8_text(&bytes)?) {
        Ok(value) => Ok(value),
        Err(Fault::Malformed) => Err("registry recovery journal is not JSON".to_string()),
        Err(Fault::DuplicateKey(key)) => Err(format!("duplicate JSON key: {key}")),
    }
}

struct Interrupted {
    root: PathBuf,
    journal: PathBuf,
    operation_id: String,
    backup: PathBuf,
    staging: PathBuf,
}

impl Interrupted {
    fn discard(&self) -> Result<(), String> {
        for leftover in [&self.backup, &self.staging] {
            if leftover.exists() {
                remove_tree(leftover)?;
            }
        }
        unlink_if_present(&self.journal)
    }

    fn was_audited(&self) -> Result<bool, String> {
        Ok(read_events(&self.root)?.iter().any(|event| {
            event.get("operation_id").and_then(Value::as_str) == Some(self.operation_id.as_str())
        }))
    }

    fn recover_installed(&self) -> Result<(), String> {
        if self.was_audited()? {
            return self.discard();
        }
        if !self.backup.exists() || !self.staging.exists() {
            return Err(MANUAL_RECONCILIATION.to_string());
        }
        let target = registry_dir(&self.root);
        if target.exists() {
            replaced(&target, &registry_dir(&self.staging))?;
        }
        replaced(&self.backup, &target)?;
        remove_tree(&self.staging)?;
        unlink_if_present(&self.journal)
    }

    fn recover_prepared(&self) -> Result<(), String> {
        let target = registry_dir(&self.root);
        if !target.exists() && self.backup.exists() {
            replaced(&self.backup, &target)?;
        }
        self.discard()
    }
}

fn names_a_leftover(value: &str, kind: &str) -> bool {
    let separated = value.contains('/') || (cfg!(windows) && value.contains('\\'));
    !separated && value.starts_with(&format!(".domain-registry-{kind}-"))
}

fn journal_text(journal: &Object, key: &str) -> Option<String> {
    journal.get(key).and_then(Value::as_str).map(str::to_string)
}

fn checked_journal(journal: &Path) -> Result<(Interrupted, String), String> {
    let malformed = || "registry recovery journal is malformed".to_string();
    let Value::Object(value) = journal_document(journal)? else {
        return Err(malformed());
    };
    let (Some(backup), Some(staging), Some(phase), Some(operation_id)) = (
        journal_text(&value, "backup"),
        journal_text(&value, "staging"),
        journal_text(&value, "phase"),
        journal_text(&value, "operation_id"),
    ) else {
        return Err(malformed());
    };
    if !names_a_leftover(&backup, "backup") || !names_a_leftover(&staging, "stage") {
        return Err("registry recovery journal contains an unsafe path".to_string());
    }
    let root = journal.parent().map(Path::to_path_buf).unwrap_or_default();
    let interrupted = Interrupted {
        journal: journal.to_path_buf(),
        backup: root.join(backup),
        staging: root.join(staging),
        root,
        operation_id,
    };
    Ok((interrupted, phase))
}

pub fn recover(root: &Path) -> Result<(), String> {
    let journal = transaction_path(root);
    if !journal.is_file() {
        return Ok(());
    }
    let (interrupted, phase) = checked_journal(&journal)?;
    match phase.as_str() {
        "prepared" => interrupted.recover_prepared(),
        "installed" => interrupted.recover_installed(),
        "audited" => interrupted.discard(),
        "reconciliation-required" => Err(MANUAL_RECONCILIATION.to_string()),
        _ => Err("registry recovery journal has an invalid phase".to_string()),
    }
}

pub fn recover_interrupted_update(root: &Path, force: bool) -> Result<(), String> {
    let lock = lock_path(root);
    if lock.exists() {
        if !force {
            return Err(
                "registry update lock exists; confirm the writer stopped, then rerun recovery with --force"
                    .to_string(),
            );
        }
        std::fs::remove_dir(&lock).map_err(|error| py::os_error(&error, &lock))?;
    }
    recover(root)
}

fn new_operation_id() -> String {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let seed = format!("{nanos}-{}-{:p}", std::process::id(), &nanos);
    hex_digest(seed.as_bytes())[..32].to_string()
}

fn held_open(root: &Path, error: &std::io::Error) -> String {
    let code = error
        .raw_os_error()
        .map_or_else(|| "None".to_string(), |code| code.to_string());
    format!(
        "another process is holding {} or a file in it open (error {code}), so the update could not be swapped; nothing was changed. Close it and rerun.",
        py::shown(&registry_dir(root))
    )
}

fn invalid_update(
    root: &Path,
    staging: &Path,
    repository: &mut Repository,
) -> Result<Option<String>, String> {
    let mut errors = validate(staging, Some(&mut *repository), false)?;
    if !errors.is_empty() {
        let already = validate(root, Some(repository), false)?;
        errors.retain(|error| !already.contains(error));
    }
    Ok((!errors.is_empty()).then(|| format!("registry update is invalid: {}", errors.join("; "))))
}

fn prepared_journal(operation: &str, staging: &Path, backup: &Path) -> Object {
    let name = |path: &Path| {
        Value::Str(
            path.file_name()
                .map(|name| name.to_string_lossy().into_owned())
                .unwrap_or_default(),
        )
    };
    let mut journal = Object::new();
    journal.insert("format", Value::Str(JOURNAL_FORMAT.to_string()));
    journal.insert("operation_id", Value::Str(operation.to_string()));
    journal.insert("phase", Value::Str("prepared".to_string()));
    journal.insert("staging", name(staging));
    journal.insert("backup", name(backup));
    journal.insert("expected_digest", Value::Null);
    journal
}

fn mark_phase(journal_path: &Path, journal: &mut Object, phase: &str) -> Result<(), String> {
    journal.insert("phase", Value::Str(phase.to_string()));
    write_json(journal_path, &Value::Object(journal.clone()))
}

struct Swap<'a> {
    root: &'a Path,
    staging_registry: PathBuf,
    backup: PathBuf,
    journal: PathBuf,
}

impl Swap<'_> {
    fn install(&self) -> Result<(), String> {
        let live = registry_dir(self.root);
        if let Err(error) = rename_retrying(&live, &self.backup) {
            unlink_if_present(&self.journal)?;
            return Err(held_open(self.root, &error));
        }
        if let Err(error) = rename_retrying(&self.staging_registry, &live) {
            replaced(&self.backup, &live)?;
            unlink_if_present(&self.journal)?;
            return Err(held_open(self.root, &error));
        }
        Ok(())
    }

    fn restore_after_failed_audit(&self) -> Result<(), String> {
        let live = registry_dir(self.root);
        replaced(&live, &self.staging_registry)?;
        replaced(&self.backup, &live)?;
        unlink_if_present(&self.journal)
    }
}

pub fn mutate_registry(
    root: &Path,
    repo_root: &Path,
    mutate: impl FnOnce(&Path) -> Result<(), String>,
    audit_event: impl FnOnce() -> Object,
) -> Result<(), String> {
    with_writer_lock(root, || {
        recover(root)?;
        let operation = new_operation_id();
        let staging = root.join(format!(".domain-registry-stage-{operation}"));
        let backup = root.join(format!(".domain-registry-backup-{operation}"));
        let outcome = update_through_staging(
            root,
            repo_root,
            (&operation, &staging, &backup),
            mutate,
            audit_event,
        );
        if outcome.is_err() && staging.exists() {
            remove_tree(&staging)?;
        }
        outcome
    })
}

fn update_through_staging(
    root: &Path,
    repo_root: &Path,
    (operation, staging, backup): (&str, &Path, &Path),
    mutate: impl FnOnce(&Path) -> Result<(), String>,
    audit_event: impl FnOnce() -> Object,
) -> Result<(), String> {
    copy_tree(&registry_dir(root), &registry_dir(staging))?;
    mutate(staging)?;
    let mut repository = Repository::new(repo_root);
    if let Some(refusal) = invalid_update(root, staging, &mut repository)? {
        return Err(refusal);
    }
    let journal_path = transaction_path(root);
    let mut journal = prepared_journal(operation, staging, backup);
    write_json(&journal_path, &Value::Object(journal.clone()))?;
    let swap = Swap {
        root,
        staging_registry: registry_dir(staging),
        backup: backup.to_path_buf(),
        journal: journal_path.clone(),
    };
    swap.install()?;
    mark_phase(&journal_path, &mut journal, "installed")?;
    let mut event = audit_event();
    event.insert("operation_id", Value::Str(operation.to_string()));
    if let Err(failure) = audit_append::append(root, &event) {
        swap.restore_after_failed_audit()?;
        return Err(failure);
    }
    mark_phase(&journal_path, &mut journal, "audited")?;
    remove_tree(backup)?;
    remove_tree(staging)?;
    unlink_if_present(&journal_path)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn scratch_root(label: &str) -> PathBuf {
        let root = std::env::temp_dir().join(format!("dm-lock-{label}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(&root).unwrap();
        root
    }

    #[test]
    fn the_writer_lock_is_held_during_the_work_and_released_after_success() {
        let root = scratch_root("success");
        let held = with_writer_lock(&root, || Ok(lock_path(&root).is_dir()));
        assert_eq!(held, Ok(true));
        assert!(!lock_path(&root).exists());
        std::fs::remove_dir_all(&root).unwrap();
    }

    #[test]
    fn the_writer_lock_is_released_after_a_refusal() {
        let root = scratch_root("refusal");
        let outcome: Result<(), String> = with_writer_lock(&root, || Err("refused".to_string()));
        assert_eq!(outcome, Err("refused".to_string()));
        assert!(!lock_path(&root).exists());
        std::fs::remove_dir_all(&root).unwrap();
    }

    #[test]
    fn a_second_writer_is_turned_away_while_the_lock_is_held() {
        let root = scratch_root("second");
        let outcome = with_writer_lock(&root, || with_writer_lock(&root, || Ok(())));
        assert!(
            outcome
                .unwrap_err()
                .starts_with("another Domain Registry update is in progress: ")
        );
        assert!(!lock_path(&root).exists());
        std::fs::remove_dir_all(&root).unwrap();
    }
}
