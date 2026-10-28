use std::collections::HashSet;
use std::path::Path;

use crate::json::{self, Object, Style, Value};
use crate::okf::{RELATION_TARGETS, prose_field, review_field};
use crate::policy::query_result_limit;
use crate::py;
use crate::pyvalue::{equal, str_of};
use crate::store::{ASSETS, Registry, asset_records};
use crate::unicode::{casefold, is_alphanumeric, nfkc};

const COMMON_SHARE: usize = 4;
const MIN_COMMON_RECORDS: usize = 3;
const MIN_STEM_LENGTH: usize = 3;
const SUFFIXES: &[&str] = &[
    "ations", "ation", "ings", "ing", "ency", "ence", "ents", "ent", "ions", "ion", "ures", "ured",
    "ure", "ens", "en", "ed", "es", "s", "e", "y",
];
const STOP_WORDS: &[&str] = &[
    "a", "after", "an", "and", "are", "at", "be", "before", "by", "can", "do", "does", "for",
    "how", "in", "is", "it", "of", "on", "or", "that", "the", "this", "to", "what", "when", "who",
    "with",
];
const IDEOGRAPHIC: &[(char, char)] = &[
    ('\u{3040}', '\u{30ff}'),
    ('\u{3400}', '\u{4dbf}'),
    ('\u{4e00}', '\u{9fff}'),
    ('\u{f900}', '\u{faff}'),
    ('\u{ac00}', '\u{d7af}'),
];
const ONE_LINE: Style = Style {
    indent: None,
    sort_keys: false,
    ensure_ascii: false,
    compact: false,
};

pub enum Found {
    Phrase,
    AllWords,
    SomeWords,
    CommonWords,
    Nothing,
}

pub fn ranked_lookup(root: &Path, asset: &str, query: &str) -> Result<(Vec<Value>, Found), String> {
    let entries = asset_records(root, asset)?;
    let limit = query_result_limit(root)?;
    let needle = casefold(query);
    let phrase: Vec<Value> = entries
        .iter()
        .filter(|entry| casefold(&json::dumps(entry, ONE_LINE)).contains(&needle))
        .take(limit)
        .cloned()
        .collect();
    if !phrase.is_empty() {
        return Ok((phrase, Found::Phrase));
    }
    let wanted = query_words(query);
    let held: Vec<HashSet<String>> = entries
        .iter()
        .map(|entry| {
            let mut texts = Vec::new();
            strings(entry, &mut texts);
            held_stems(&texts.join(" "))
        })
        .collect();
    let every: Vec<Value> = entries
        .iter()
        .zip(&held)
        .filter(|(_, words)| !wanted.is_empty() && wanted.iter().all(|word| holds(word, words)))
        .map(|(entry, _)| entry.clone())
        .take(limit)
        .collect();
    if !every.is_empty() {
        return Ok((every, Found::AllWords));
    }
    let common: Vec<&String> = wanted
        .iter()
        .filter(|word| {
            is_common(
                held.iter().filter(|words| holds(word, words)).count(),
                entries.len(),
            )
        })
        .collect();
    let mut shared: Vec<(usize, &Value)> = entries
        .iter()
        .zip(&held)
        .map(|(entry, words)| {
            let count = wanted
                .iter()
                .filter(|word| !common.contains(word) && holds(word, words))
                .count();
            (count, entry)
        })
        .collect();
    shared.sort_by_key(|(count, _)| std::cmp::Reverse(*count));
    let some: Vec<Value> = shared
        .into_iter()
        .filter(|(count, _)| *count > 0)
        .map(|(_, entry)| entry.clone())
        .take(limit)
        .collect();
    if !some.is_empty() {
        return Ok((some, Found::SomeWords));
    }
    let how = if common.is_empty() {
        Found::Nothing
    } else {
        Found::CommonWords
    };
    Ok((Vec::new(), how))
}

fn is_common(holders: usize, records: usize) -> bool {
    holders >= MIN_COMMON_RECORDS && holders * COMMON_SHARE > records
}

fn strings(value: &Value, out: &mut Vec<String>) {
    match value {
        Value::Str(text) => out.push(text.clone()),
        Value::Object(object) => object.iter().for_each(|(_, child)| strings(child, out)),
        Value::Array(items) => items.iter().for_each(|child| strings(child, out)),
        _ => {}
    }
}

fn camel_split(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let mut previous: Option<char> = None;
    for character in text.chars() {
        if previous.is_some_and(|before| before.is_ascii_lowercase() || before.is_ascii_digit())
            && character.is_ascii_uppercase()
        {
            out.push(' ');
        }
        out.push(character);
        previous = Some(character);
    }
    out
}

fn is_ideographic(character: char) -> bool {
    IDEOGRAPHIC
        .iter()
        .any(|(low, high)| (*low..=*high).contains(&character))
}

fn words(text: &str) -> Vec<String> {
    let plain = casefold(&nfkc(&camel_split(text)));
    let mut found = Vec::new();
    let tokens = plain
        .split(|character: char| !is_alphanumeric(character))
        .filter(|token| !token.is_empty());
    for token in tokens {
        let mut run = String::new();
        let mut run_is_ideographic = false;
        for character in token.chars() {
            let ideographic = is_ideographic(character);
            if !run.is_empty() && ideographic != run_is_ideographic {
                push_run(&mut found, std::mem::take(&mut run), run_is_ideographic);
            }
            run_is_ideographic = ideographic;
            run.push(character);
        }
        push_run(&mut found, run, run_is_ideographic);
    }
    found
}

fn push_run(found: &mut Vec<String>, run: String, ideographic: bool) {
    if ideographic {
        found.extend(run.chars().map(String::from));
    } else if !run.is_empty() && !STOP_WORDS.contains(&run.as_str()) {
        found.push(run);
    }
}

fn stem(word: &str) -> String {
    let length = word.chars().count();
    SUFFIXES
        .iter()
        .find(|suffix| word.ends_with(**suffix) && length - suffix.len() >= MIN_STEM_LENGTH)
        .map_or_else(
            || word.to_string(),
            |suffix| word[..word.len() - suffix.len()].to_string(),
        )
}

pub fn held_stems(text: &str) -> HashSet<String> {
    let stems: Vec<String> = words(text).iter().map(|word| stem(word)).collect();
    let pairs = stems
        .windows(2)
        .map(|pair| format!("{}{}", pair[0], pair[1]));
    stems.iter().cloned().chain(pairs).collect()
}

pub fn query_words(query: &str) -> Vec<String> {
    let mut chosen: Vec<(String, String)> = Vec::new();
    for word in words(query) {
        let key = stem(&word);
        match chosen.iter_mut().find(|(seen, _)| *seen == key) {
            Some(slot) => slot.1 = word,
            None => chosen.push((key, word)),
        }
    }
    chosen.into_iter().map(|(_, word)| word).collect()
}

pub fn holds(word: &str, held: &HashSet<String>) -> bool {
    if held.contains(&stem(word)) {
        return true;
    }
    let characters: Vec<char> = word.chars().collect();
    (MIN_STEM_LENGTH..characters.len().saturating_sub(1)).any(|split| {
        let head: String = characters[..split].iter().collect();
        let tail: String = characters[split..].iter().collect();
        held.contains(&format!("{}{}", stem(&head), stem(&tail)))
    })
}

pub const MATCH_KINDS: &[&str] = &["name", "synonym", "definition"];
pub const CONSTRAINT_ASSETS: &[&str] = &["rules", "contracts"];
const SUMMARY_FIELDS: &[&str] = &[
    "statement",
    "definition",
    "responsibility",
    "description",
    "name",
];
const DEFINING_FIELDS: &[&str] = &[
    "id",
    "name",
    "responsibility",
    "statement",
    "definition",
    "description",
];
const UNSEARCHED_FIELDS: &[&str] = &["status", "review_status"];

fn field_text(term: &Object, field: &str) -> String {
    casefold(&term.get(field).map(str_of).unwrap_or_default())
}

fn match_kind(term: &Object, needle: &str) -> Option<&'static str> {
    let names = ["id", "name"].map(|field| field_text(term, field)).to_vec();
    let synonyms: Vec<String> = term
        .get("synonyms")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .map(|label| casefold(&str_of(label)))
        .collect();
    for (kind, values) in [("name", names), ("synonym", synonyms)] {
        if values.iter().any(|value| {
            !value.is_empty() && (needle.contains(value.as_str()) || value.contains(needle))
        }) {
            return Some(kind);
        }
    }
    field_text(term, "definition")
        .contains(needle)
        .then_some("definition")
}

fn lists(record: &Object, key: &str, identifier: &str) -> bool {
    record
        .get(key)
        .and_then(Value::as_array)
        .is_some_and(|items| items.iter().any(|item| item.as_str() == Some(identifier)))
}

pub fn every_resolved_term(
    registry: &Registry,
    query: &str,
    context: Option<&str>,
    with_mentions: bool,
) -> Result<Vec<Object>, String> {
    let needle = casefold(query);
    let mut matches = Vec::new();
    for term in registry.records("vocabulary")? {
        let Value::Object(mut term) = term else {
            continue;
        };
        if context.is_some_and(|context| !lists(&term, "contexts", context)) {
            continue;
        }
        if let Some(kind) = match_kind(&term, &needle) {
            term.insert("match_kind", Value::Str(kind.to_string()));
            matches.push(term);
        }
    }
    matches.sort_by_key(|term| {
        let kind = term.get("match_kind").and_then(Value::as_str);
        MATCH_KINDS.iter().position(|known| Some(*known) == kind)
    });
    if !matches.is_empty() && !with_mentions {
        return Ok(matches);
    }
    let matched: Vec<&Value> = matches.iter().filter_map(|term| term.get("id")).collect();
    let mentions: Vec<Object> = prose_mentions(registry, &needle)?
        .into_iter()
        .filter(|mention| {
            mention.get("asset").and_then(Value::as_str) != Some("vocabulary")
                || !mention
                    .get("id")
                    .is_some_and(|id| matched.iter().any(|seen| equal(seen, id)))
        })
        .collect();
    matches.extend(mentions);
    Ok(matches)
}

pub fn prose_mentions(registry: &Registry, needle: &str) -> Result<Vec<Object>, String> {
    let wanted = query_words(needle);
    if wanted.is_empty() {
        return Ok(Vec::new());
    }
    let mut assets: Vec<&str> = ASSETS.iter().map(|(asset, _, _)| *asset).collect();
    assets.sort_by_key(|asset| format!("{asset}.json"));
    let mut mentions = Vec::new();
    for asset in assets {
        for entry in registry.records(asset)? {
            let Value::Object(entry) = entry else {
                continue;
            };
            if entry.get(review_field(asset)).and_then(Value::as_str) != Some("reviewed") {
                continue;
            }
            let fields = mentioning_fields(&entry, &wanted);
            if !fields.is_empty() {
                mentions.push(mention(asset, &entry, fields));
            }
        }
    }
    mentions.sort_by_key(|mention| {
        let fields = mention
            .get("fields")
            .and_then(Value::as_array)
            .unwrap_or_default();
        let defining = fields.iter().any(|field| {
            field
                .as_str()
                .is_some_and(|field| DEFINING_FIELDS.contains(&field))
        });
        let asset = mention
            .get("asset")
            .and_then(Value::as_str)
            .unwrap_or_default();
        (!defining, !CONSTRAINT_ASSETS.contains(&asset))
    });
    Ok(mentions)
}

fn mention(asset: &str, entry: &Object, fields: Vec<String>) -> Object {
    let summary = prose_field(asset)
        .into_iter()
        .chain(SUMMARY_FIELDS.iter().copied())
        .find_map(|field| entry.get(field).and_then(Value::as_str))
        .unwrap_or_default();
    let mut out = Object::new();
    out.insert("match_kind", Value::Str("mentioned".to_string()));
    out.insert("asset", Value::Str(asset.to_string()));
    out.insert("id", entry.get("id").cloned().unwrap_or(Value::Null));
    out.insert("status", Value::Str("reviewed".to_string()));
    out.insert("contexts", Value::Array(contexts_named(asset, entry)));
    out.insert(
        "summary",
        Value::Str(first_sentence(py::strip(summary)).to_string()),
    );
    out.insert(
        "fields",
        Value::Array(fields.into_iter().map(Value::Str).collect()),
    );
    out
}

fn first_sentence(text: &str) -> &str {
    let mut previous = None;
    for (at, character) in text.char_indices() {
        if matches!(previous, Some('.' | '!' | '?')) && py::is_space(character) {
            return &text[..at];
        }
        previous = Some(character);
    }
    text
}

fn contexts_named(asset: &str, entry: &Object) -> Vec<Value> {
    if asset == "contexts" {
        return vec![entry.get("id").cloned().unwrap_or(Value::Null)];
    }
    let mut named: Vec<Value> = Vec::new();
    for (field, value) in entry.iter() {
        if !RELATION_TARGETS
            .iter()
            .any(|(relation, noun)| relation == field && *noun == "Context")
        {
            continue;
        }
        let values = match value {
            Value::Array(items) => items.clone(),
            other => vec![other.clone()],
        };
        for value in values {
            if !named.iter().any(|seen| equal(seen, &value)) {
                named.push(value);
            }
        }
    }
    named
}

fn mentioning_fields(entry: &Object, wanted: &[String]) -> Vec<String> {
    entry
        .iter()
        .filter(|(field, value)| {
            !UNSEARCHED_FIELDS.contains(&field.as_str())
                && searchable(field, value).iter().any(|text| {
                    let stems = held_stems(text);
                    wanted.iter().all(|word| holds(word, &stems))
                })
        })
        .map(|(field, _)| field.clone())
        .collect()
}

fn searchable(field: &str, value: &Value) -> Vec<String> {
    match value {
        Value::Str(text) if field == "id" => vec![text.replace('-', " ")],
        _ if field == "id" => Vec::new(),
        Value::Str(text) => vec![text.clone()],
        Value::Array(items)
            if !RELATION_TARGETS
                .iter()
                .any(|(relation, _)| *relation == field) =>
        {
            items
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_string)
                .collect()
        }
        _ => Vec::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn words_split_camel_case_drop_stop_words_and_spell_out_ideographs() {
        assert_eq!(
            words("theRunID of 評估Run"),
            ["run", "id", "評", "估", "run"]
        );
    }

    #[test]
    fn a_stem_keeps_three_characters() {
        assert_eq!(stem("evaluations"), "evalu");
        assert_eq!(stem("runs"), "run");
        assert_eq!(stem("ages"), "age");
    }

    fn stems_of(text: &str) -> Vec<String> {
        let mut stems: Vec<String> = words(text).iter().map(|word| stem(word)).collect();
        stems.sort();
        stems.dedup();
        stems
    }

    #[test]
    fn forms_of_one_word_share_one_stem() {
        let rows = [
            (
                "concurrency and concurrent",
                "concurrency",
                "concurrent",
                "concurr",
            ),
            ("licensing and license", "licensing", "license", "licens"),
            ("deleted and deletion", "deleted", "deletion", "delet"),
            ("plural and singular", "runs", "run", "run"),
            ("participle in en and base", "taken", "take", "tak"),
            (
                "plural of a participle in en and base",
                "tokens",
                "token",
                "tok",
            ),
            ("noun in ure and past form", "exposure", "exposed", "expos"),
            (
                "noun in ure and past participle",
                "failure",
                "failed",
                "fail",
            ),
            ("verb and its plural in ure", "measure", "measures", "meas"),
            (
                "verb in ure and its past form",
                "measure",
                "measured",
                "meas",
            ),
        ];
        for (name, left, right, stem_shared) in rows {
            assert_eq!(
                (stems_of(left), stems_of(right)),
                (vec![stem_shared.to_string()], vec![stem_shared.to_string()]),
                "{name}"
            );
        }
    }

    #[test]
    fn a_word_with_a_longer_stem_is_not_joined_with_its_ending() {
        let rows = [
            ("prefix kept", "precheck", "precheck"),
            ("short word kept whole", "open", "open"),
            ("suffix leaving two characters kept whole", "ring", "ring"),
            ("past ending leaving one character kept whole", "bed", "bed"),
        ];
        for (name, word, expected) in rows {
            assert_eq!(stems_of(word), vec![expected.to_string()], "{name}");
        }
    }

    #[test]
    fn question_words_are_not_searched() {
        assert_eq!(
            stems_of("who can see what it does, how and when that is"),
            vec!["see".to_string()]
        );
    }

    #[test]
    fn a_query_spells_out_ideographs_folds_width_and_keeps_accents() {
        assert_eq!(
            query_words("訂單狀態 Ｃａｆé order"),
            ["訂", "單", "狀", "態", "café", "order"]
        );
    }

    #[test]
    fn a_word_is_common_from_three_holders_above_a_quarter_of_the_records() {
        let rows = [
            ("three of eleven", 3, 11, true),
            ("three of twelve is exactly a quarter", 3, 12, false),
            ("two of seven is under three holders", 2, 7, false),
            ("three of three", 3, 3, true),
        ];
        for (name, holders, records, expected) in rows {
            assert_eq!(is_common(holders, records), expected, "{name}");
        }
    }
}
