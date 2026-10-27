use std::path::{Path, PathBuf};

use crate::json::{Object, Value};
use crate::okf::{prose_field, title_field};
use crate::py;
use crate::pyvalue::str_of;
use crate::store::{ASSETS, Registry, has_registry, registry_dir};

const OKF_VERSION: &str = "0.2";
const INDEX: &str = "index.md";
const INDEX_DESCRIPTION_LIMIT: usize = 160;
const HEADINGS: &[(&str, &str)] = &[
    ("contexts", "Bounded Contexts"),
    ("vocabulary", "Vocabulary"),
    ("aggregates", "Aggregates"),
    ("rules", "Business Rules"),
    ("contracts", "Contracts"),
    ("interactions", "Interactions"),
    ("decisions", "Decisions"),
    ("events", "Domain Events"),
    ("capabilities", "Capabilities"),
    ("value-objects", "Value Objects"),
    ("dependency-policies", "Dependency Policies"),
];

fn heading(asset: &str) -> &'static str {
    HEADINGS
        .iter()
        .find(|(name, _)| *name == asset)
        .map_or("", |(_, heading)| heading)
}

pub fn root_index() -> String {
    let mut lines = vec![
        "---".to_string(),
        format!("okf_version: \"{OKF_VERSION}\""),
        "---".to_string(),
        "# Domain Registry".to_string(),
        String::new(),
    ];
    for (asset, _, _) in ASSETS {
        lines.push(format!("- [{}](/{asset}/{INDEX})", heading(asset)));
    }
    format!("{}\n", lines.join("\n"))
}

pub fn asset_index(asset: &str, records: &[Object]) -> String {
    let mut lines = vec![format!("# {}", heading(asset)), String::new()];
    let mut sorted: Vec<&Object> = records.iter().collect();
    sorted.sort_by_key(|record| record.get("id").map(str_of).unwrap_or_default());
    for record in sorted {
        let id = record.get("id").map(str_of).unwrap_or_default();
        let label = title_field(asset)
            .and_then(|field| record.get(field))
            .map_or_else(|| id.clone(), str_of);
        let description = first_sentence(prose_field(asset).and_then(|field| record.get(field)));
        let link = format!("- [{label}](/{asset}/{id})");
        lines.push(if description.is_empty() {
            link
        } else {
            format!("{link} — {description}")
        });
    }
    format!("{}\n", lines.join("\n"))
}

fn first_sentence(prose: Option<&Value>) -> String {
    let Some(Value::Str(prose)) = prose else {
        return String::new();
    };
    let first_line = prose.split('\n').next().unwrap_or_default();
    let sentence = first_line
        .split(". ")
        .next()
        .unwrap_or_default()
        .trim_end_matches('.');
    if sentence.chars().count() <= INDEX_DESCRIPTION_LIMIT {
        return format!("{sentence}.");
    }
    let kept: String = sentence.chars().take(INDEX_DESCRIPTION_LIMIT - 1).collect();
    format!("{}….", kept.trim_end_matches(py::is_space))
}

pub type Loaded = [(&'static str, Vec<Object>)];

fn expected_indexes(
    root: &Path,
    mut records_of: impl FnMut(&str) -> Result<Option<Vec<Object>>, String>,
) -> Result<Vec<(PathBuf, String)>, String> {
    if !has_registry(root) {
        return Ok(Vec::new());
    }
    let mut expected = vec![(registry_dir(root).join(INDEX), root_index())];
    for (asset, _, _) in ASSETS {
        let directory = registry_dir(root).join(asset);
        if !directory.is_dir() {
            continue;
        }
        if let Some(records) = records_of(asset)? {
            expected.push((directory.join(INDEX), asset_index(asset, &records)));
        }
    }
    Ok(expected)
}

fn is_stale(path: &Path, expected: &str) -> Result<bool, String> {
    if !path.is_file() {
        return Ok(true);
    }
    let data = std::fs::read(path).map_err(|error| py::open_error(&error, path))?;
    Ok(py::utf8_text(&data)? != expected)
}

fn stale_among(expected: Vec<(PathBuf, String)>) -> Result<Vec<(PathBuf, String)>, String> {
    let mut stale = Vec::new();
    for (path, text) in expected {
        if is_stale(&path, &text)? {
            stale.push((path, text));
        }
    }
    Ok(stale)
}

pub fn stale_indexes(root: &Path, loaded: &Loaded) -> Result<Vec<PathBuf>, String> {
    let expected = expected_indexes(root, |asset| {
        Ok(loaded
            .iter()
            .find(|(name, _)| *name == asset)
            .map(|(_, records)| records.clone()))
    })?;
    Ok(stale_among(expected)?
        .into_iter()
        .map(|(path, _)| path)
        .collect())
}

pub fn refresh_indexes(root: &Path) -> Result<Vec<PathBuf>, String> {
    if !has_registry(root) {
        return Ok(Vec::new());
    }
    let registry = Registry::open(root)?;
    let expected = expected_indexes(root, |asset| {
        let records = registry.records(asset)?;
        Ok(Some(
            records
                .into_iter()
                .filter_map(|record| record.as_object().cloned())
                .collect(),
        ))
    })?;
    let stale = stale_among(expected)?;
    for (path, text) in &stale {
        std::fs::write(path, text).map_err(|error| py::open_error(&error, path))?;
    }
    Ok(stale.into_iter().map(|(path, _)| path).collect())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn prose(text: &str) -> Value {
        Value::Str(text.to_string())
    }

    #[test]
    fn a_first_sentence_stops_at_a_full_stop_and_ends_with_one() {
        assert_eq!(first_sentence(Some(&prose("One. Two."))), "One.");
        assert_eq!(first_sentence(Some(&prose("Line\nTwo."))), "Line.");
        assert_eq!(first_sentence(Some(&prose(""))), ".");
        assert_eq!(first_sentence(None), "");
    }

    #[test]
    fn a_first_sentence_is_cut_after_the_limit_with_an_ellipsis() {
        let at_limit = "x".repeat(INDEX_DESCRIPTION_LIMIT);
        assert_eq!(
            first_sentence(Some(&prose(&at_limit))),
            format!("{at_limit}.")
        );
        let past = "x".repeat(INDEX_DESCRIPTION_LIMIT + 1);
        let cut = format!("{}….", "x".repeat(INDEX_DESCRIPTION_LIMIT - 1));
        assert_eq!(first_sentence(Some(&prose(&past))), cut);
    }
}
