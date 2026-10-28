use std::io::Write;
use std::path::Path;
use std::time::{SystemTime, UNIX_EPOCH};

use crate::audit_log::{
    MANIFEST_FORMAT, audit_manifest_path, event_digest, events_path, is_valid, successor, verify,
};
use crate::files::write_json;
use crate::json::{self, Object, Style, Value};
use crate::py;

const RESERVED_FIELDS: [&str; 4] = [
    "sequence",
    "previous_event_sha256",
    "recorded_at",
    "event_sha256",
];

const LOG_LINE: Style = Style {
    indent: None,
    sort_keys: true,
    ensure_ascii: false,
    compact: false,
};

pub fn utc_now() -> String {
    let since_epoch = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default();
    let (days, seconds) = (
        since_epoch.as_secs() / 86_400,
        since_epoch.as_secs() % 86_400,
    );
    let (year, month, day) = civil_date(days as i64);
    let clock = format!(
        "{:02}:{:02}:{:02}",
        seconds / 3600,
        seconds % 3600 / 60,
        seconds % 60
    );
    let micros = since_epoch.subsec_micros();
    let fraction = if micros == 0 {
        String::new()
    } else {
        format!(".{micros:06}")
    };
    format!("{year:04}-{month:02}-{day:02}T{clock}{fraction}Z")
}

fn civil_date(days_since_epoch: i64) -> (i64, i64, i64) {
    let shifted = days_since_epoch + 719_468;
    let era = shifted.div_euclid(146_097);
    let day_of_era = shifted.rem_euclid(146_097);
    let year_of_era =
        (day_of_era - day_of_era / 1460 + day_of_era / 36_524 - day_of_era / 146_096) / 365;
    let day_of_year = day_of_era - (365 * year_of_era + year_of_era / 4 - year_of_era / 100);
    let month_index = (5 * day_of_year + 2) / 153;
    let day = day_of_year - (153 * month_index + 2) / 5 + 1;
    let month = if month_index < 10 {
        month_index + 3
    } else {
        month_index - 9
    };
    let year = year_of_era + era * 400 + i64::from(month <= 2);
    (year, month, day)
}

fn text_of(result: &Object, key: &str) -> String {
    result
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

fn ensure_audit_directory(root: &Path) -> Result<(), String> {
    let directory = root.join("audit");
    match std::fs::create_dir(&directory) {
        Ok(()) => Ok(()),
        Err(_) if directory.is_dir() => Ok(()),
        Err(error) => Err(py::os_error(&error, &directory)),
    }
}

fn previous_heads(heads: &[Value]) -> Value {
    match heads {
        [] => Value::Null,
        [only] => only.clone(),
        many => Value::Array(many.to_vec()),
    }
}

pub fn append(root: &Path, event: &Object) -> Result<(), String> {
    if RESERVED_FIELDS
        .iter()
        .any(|field| event.get(field).is_some())
    {
        return Err("audit event contains reserved fields".to_string());
    }
    ensure_audit_directory(root)?;
    let existing = verify(root)?;
    if !is_valid(&existing) {
        return Err(format!(
            "audit log is invalid: {}",
            text_of(&existing, "reason")
        ));
    }
    let heads = existing
        .get("heads")
        .and_then(Value::as_array)
        .unwrap_or_default();
    let mut value = Object::new();
    value.insert(
        "sequence",
        successor(existing.get("height").unwrap_or(&Value::Null)),
    );
    value.insert("previous_event_sha256", previous_heads(heads));
    value.insert("recorded_at", Value::Str(utc_now()));
    for (key, item) in event.iter() {
        value.insert(key.clone(), item.clone());
    }
    let digest = event_digest(&value);
    value.insert("event_sha256", Value::Str(digest.clone()));
    let path = events_path(root);
    let mut output = std::fs::OpenOptions::new()
        .append(true)
        .create(true)
        .open(&path)
        .map_err(|error| py::open_error(&error, &path))?;
    let line = format!("{}\n", json::dumps(&Value::Object(value), LOG_LINE));
    output
        .write_all(line.as_bytes())
        .and_then(|()| output.sync_all())
        .map_err(|error| py::os_error(&error, &path))?;
    drop(output);
    write_manifest(root, &existing, &digest)
}

fn write_manifest(root: &Path, existing: &Object, head: &str) -> Result<(), String> {
    let events = successor(existing.get("events").unwrap_or(&Value::Null));
    let mut manifest = Object::new();
    manifest.insert("format", Value::Str(MANIFEST_FORMAT.to_string()));
    manifest.insert("events", events);
    manifest.insert("head_sha256", Value::Str(head.to_string()));
    manifest.insert("heads", Value::Array(vec![Value::Str(head.to_string())]));
    write_json(&audit_manifest_path(root), &Value::Object(manifest))
}
