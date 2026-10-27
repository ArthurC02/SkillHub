use std::path::Path;

use crate::json::{self, Object, Value};
use crate::py::shown;

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

fn one_level_too_deep_hint(path: &Path) -> String {
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
