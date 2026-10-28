use std::collections::{BTreeSet, HashMap};

use crate::coupling::root_parts;
use crate::coverage::all_references;
use crate::json::{Object, Value};
use crate::okf::{RELATION_TARGETS, truthy};
use crate::pyvalue::str_of;
use crate::store::ASSETS;
use crate::unicode::casefold;

const PAIRED_ASSETS: &[(&str, &str, &str)] = &[
    ("interactions", "producer_context", "consumer_context"),
    ("dependency-policies", "from_context", "to_context"),
];
const NAME_FIELDS: &[(&str, &str)] = &[
    ("contexts", "name"),
    ("vocabulary", "name"),
    ("aggregates", "root"),
];
const WORDING_FIELDS: &[&str] = &[
    "id",
    "name",
    "statement",
    "definition",
    "responsibility",
    "description",
];
const EXACT_PATH: u8 = 5;
const PATH_SUFFIX: u8 = 4;
const NESTED_PATH: u8 = 3;
const SAME_NAME: u8 = 2;
const SHARED_EVIDENCE: u8 = 1;
const POSSIBLE_PAIR_SIMILARITY: f64 = 0.25;
const MIN_WORD_LENGTH: usize = 3;

type Records<'a> = Vec<&'a Object>;
type Citation = (String, i128, i128);
type Score = (u8, i128);

pub struct Comparison {
    pub report: Object,
    pub copied_matches: usize,
    pub attempted_matches: usize,
    pub without_candidate: Vec<String>,
}

struct Matched {
    gold: usize,
    candidate: usize,
    pairs: Vec<Object>,
    result: Object,
}

fn label(tier: u8) -> &'static str {
    match tier {
        EXACT_PATH => "path",
        PATH_SUFFIX => "path-suffix",
        NESTED_PATH => "nested-path",
        SAME_NAME => "name",
        _ => "context-and-evidence",
    }
}

fn id_of(record: &Object) -> String {
    record.get("id").map(str_of).unwrap_or_default()
}

fn records_of<'a>(documents: &'a Object, asset: &str, key: &str) -> Records<'a> {
    documents
        .get(asset)
        .and_then(Value::as_object)
        .and_then(|document| document.get(key))
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .filter(|record| record.get("id").is_some_and(truthy))
        .collect()
}

fn context_fields() -> impl Iterator<Item = &'static str> {
    RELATION_TARGETS
        .iter()
        .filter(|(_, noun)| *noun == "Context")
        .map(|(field, _)| *field)
}

pub fn contexts_named_by(asset: &str, record: &Object) -> BTreeSet<String> {
    let mut named = BTreeSet::new();
    if asset == "contexts"
        && let Some(Value::Str(id)) = record.get("id")
    {
        named.insert(id.clone());
    }
    for field in context_fields() {
        match record.get(field) {
            Some(Value::Array(items)) => {
                named.extend(items.iter().filter_map(Value::as_str).map(str::to_string));
            }
            Some(Value::Str(text)) => {
                named.insert(text.clone());
            }
            _ => {}
        }
    }
    named
}

fn normalized(value: Option<&Value>) -> String {
    match value {
        Some(Value::Str(text)) => casefold(text)
            .chars()
            .filter(|c| c.is_ascii_digit() || c.is_ascii_lowercase())
            .collect(),
        _ => String::new(),
    }
}

fn names(asset: &str, record: &Object, with_synonyms: bool) -> BTreeSet<String> {
    let mut found = BTreeSet::from([normalized(record.get("id"))]);
    if let Some((_, field)) = NAME_FIELDS.iter().find(|(name, _)| *name == asset) {
        found.insert(normalized(record.get(field)));
    }
    if with_synonyms
        && asset == "vocabulary"
        && let Some(Value::Array(synonyms)) = record.get("synonyms")
    {
        found.extend(synonyms.iter().map(|synonym| normalized(Some(synonym))));
    }
    found.remove("");
    found
}

fn whole_number(value: Option<&Value>) -> Option<i128> {
    match value {
        Some(Value::Int(digits)) => Some(digits.parse().unwrap_or(i128::MAX)),
        _ => None,
    }
}

fn citations(record: &Object) -> Result<Vec<Citation>, String> {
    let mut found = Vec::new();
    for reference in all_references(&Value::Object(record.clone()), 0)? {
        let Value::Object(reference) = reference else {
            continue;
        };
        let Some(path) = reference.get("path").and_then(Value::as_str) else {
            continue;
        };
        let lines = reference.get("lines").and_then(Value::as_object);
        let start = whole_number(lines.and_then(|lines| lines.get("start")));
        let end = whole_number(lines.and_then(|lines| lines.get("end")));
        found.push(match (start, end) {
            (Some(start), Some(end)) => (path.to_string(), start, end),
            _ => (path.to_string(), 0, -1),
        });
    }
    Ok(found)
}

fn cited_excerpts(record: &Object) -> Result<BTreeSet<(String, String)>, String> {
    let mut found = BTreeSet::new();
    for reference in all_references(&Value::Object(record.clone()), 0)? {
        let Value::Object(reference) = reference else {
            continue;
        };
        if let (Some(path), Some(hash)) = (
            reference.get("path").and_then(Value::as_str),
            reference.get("excerpt_sha256").and_then(Value::as_str),
        ) {
            found.insert((path.to_string(), hash.to_string()));
        }
    }
    Ok(found)
}

fn cites_within_the_other(one: &Object, other: &Object) -> Result<bool, String> {
    let (mine, theirs) = (cited_excerpts(one)?, cited_excerpts(other)?);
    Ok(!mine.is_empty()
        && !theirs.is_empty()
        && (mine.is_subset(&theirs) || theirs.is_subset(&mine)))
}

fn shared_files(gold: &[Citation], candidate: &[Citation]) -> BTreeSet<String> {
    let in_gold: BTreeSet<&str> = gold.iter().map(|(path, _, _)| path.as_str()).collect();
    candidate
        .iter()
        .map(|(path, _, _)| path.as_str())
        .filter(|path| in_gold.contains(path))
        .map(str::to_string)
        .collect()
}

fn overlapping_lines(gold: &[Citation], candidate: &[Citation], shared: &BTreeSet<String>) -> i128 {
    gold.iter()
        .filter(|(path, _, _)| shared.contains(path))
        .map(|(path, start, end)| {
            candidate
                .iter()
                .filter(|(other, _, _)| other == path)
                .map(|(_, other_start, other_end)| {
                    0.max(*end.min(other_end) - *start.max(other_start) + 1)
                })
                .sum::<i128>()
        })
        .sum()
}

fn path_parts(value: Option<&Value>) -> Option<Vec<String>> {
    let Some(Value::Str(path)) = value else {
        return None;
    };
    let parts: Vec<String> = root_parts(path).into_iter().map(str::to_string).collect();
    (!parts.is_empty()).then_some(parts)
}

fn path_score(gold: &Option<Vec<String>>, candidate: &Option<Vec<String>>) -> u8 {
    let (Some(gold), Some(candidate)) = (gold, candidate) else {
        return 0;
    };
    if gold == candidate {
        return EXACT_PATH;
    }
    let (shorter, longer) = if candidate.len() < gold.len() {
        (candidate, gold)
    } else {
        (gold, candidate)
    };
    if longer[longer.len() - shorter.len()..] == shorter[..] {
        return PATH_SUFFIX;
    }
    let inside = (0..=longer.len() - shorter.len())
        .any(|start| longer[start..start + shorter.len()] == shorter[..]);
    if inside { NESTED_PATH } else { 0 }
}

fn mapped(
    asset: &str,
    record: &Object,
    context_of: &HashMap<String, String>,
) -> BTreeSet<Option<String>> {
    contexts_named_by(asset, record)
        .iter()
        .map(|context| context_of.get(context).cloned())
        .collect()
}

fn words(record: &Object) -> BTreeSet<String> {
    let text = WORDING_FIELDS
        .iter()
        .map(|field| record.get(field).map(str_of).unwrap_or_default())
        .collect::<Vec<_>>()
        .join(" ");
    let folded = casefold(&text);
    let mut found = BTreeSet::new();
    let mut current = String::new();
    for character in folded.chars().chain(std::iter::once(' ')) {
        if character.is_ascii_digit() || character.is_ascii_lowercase() {
            current.push(character);
        } else {
            if current.len() >= MIN_WORD_LENGTH {
                found.insert(std::mem::take(&mut current));
            }
            current.clear();
        }
    }
    found
}

fn similarity(first: &BTreeSet<String>, second: &BTreeSet<String>) -> f64 {
    let union = first.union(second).count();
    if union == 0 {
        return 0.0;
    }
    first.intersection(second).count() as f64 / union as f64
}

fn rounded_to_hundredths(number: f64) -> f64 {
    format!("{number:.2}").parse().unwrap_or(number)
}

fn scores(matched: usize, gold: usize, candidate: usize) -> [(&'static str, Value); 3] {
    let ratio = |top: usize, bottom: usize| {
        if bottom == 0 {
            Value::Null
        } else {
            Value::Float(top as f64 / bottom as f64)
        }
    };
    [
        ("precision", ratio(matched, candidate)),
        ("recall", ratio(matched, gold)),
        (
            "f1",
            if gold + candidate == 0 {
                Value::Null
            } else {
                Value::Float((2 * matched) as f64 / (gold + candidate) as f64)
            },
        ),
    ]
}

fn name_list(items: Vec<String>) -> Value {
    Value::Array(items.into_iter().map(Value::Str).collect())
}

fn possible_pairs(
    asset: &str,
    gold: &Records,
    candidate: &Records,
    context_of: &HashMap<String, String>,
    unpaired: (&[usize], &[usize]),
) -> Vec<Object> {
    if asset == "contexts" || PAIRED_ASSETS.iter().any(|(name, _, _)| *name == asset) {
        return Vec::new();
    }
    let (missed, extra) = unpaired;
    let candidate_words: Vec<BTreeSet<String>> =
        extra.iter().map(|i| words(candidate[*i])).collect();
    let candidate_contexts: Vec<BTreeSet<Option<String>>> = extra
        .iter()
        .map(|i| mapped(asset, candidate[*i], context_of))
        .collect();
    let mut possible: Vec<(f64, String, Object)> = Vec::new();
    for gold_index in missed {
        let expected = words(gold[*gold_index]);
        let named = contexts_named_by(asset, gold[*gold_index]);
        let best = extra
            .iter()
            .enumerate()
            .filter(|(position, _)| {
                named
                    .iter()
                    .any(|context| candidate_contexts[*position].contains(&Some(context.clone())))
            })
            .map(|(position, index)| {
                (
                    similarity(&expected, &candidate_words[position]),
                    id_of(candidate[*index]),
                )
            })
            .max_by(|left, right| {
                left.0
                    .total_cmp(&right.0)
                    .then_with(|| left.1.cmp(&right.1))
            });
        if let Some((score, candidate_id)) = best
            && score >= POSSIBLE_PAIR_SIMILARITY
        {
            let shown = rounded_to_hundredths(score);
            let mut pair = Object::new();
            pair.insert("gold", Value::Str(id_of(gold[*gold_index])));
            pair.insert("candidate", Value::Str(candidate_id));
            pair.insert("similarity", Value::Float(shown));
            possible.push((shown, id_of(gold[*gold_index]), pair));
        }
    }
    possible.sort_by(|left, right| {
        right
            .0
            .total_cmp(&left.0)
            .then_with(|| left.1.cmp(&right.1))
    });
    possible.into_iter().map(|(_, _, pair)| pair).collect()
}

type EndPair = (Option<String>, Option<String>);
type EndLists = (Vec<EndPair>, Vec<EndPair>);

struct Scorer<'a> {
    asset: &'a str,
    ends: Option<EndLists>,
    gold_names: Vec<BTreeSet<String>>,
    candidate_names: Vec<BTreeSet<String>>,
    gold_parts: Vec<Option<Vec<String>>>,
    candidate_parts: Vec<Option<Vec<String>>>,
    gold_contexts: Vec<BTreeSet<String>>,
    candidate_contexts: Vec<BTreeSet<Option<String>>>,
    gold_citations: Vec<Vec<Citation>>,
    candidate_citations: Vec<Vec<Citation>>,
}

fn end_of(record: &Object, end: &str) -> Option<String> {
    match record.get(end) {
        Some(Value::Str(text)) => Some(text.clone()),
        _ => None,
    }
}

impl<'a> Scorer<'a> {
    fn new(
        asset: &'a str,
        gold: &'a Records<'a>,
        candidate: &'a Records<'a>,
        context_of: &HashMap<String, String>,
    ) -> Result<Self, String> {
        let mut scorer = Self {
            asset,
            ends: None,
            gold_names: Vec::new(),
            candidate_names: Vec::new(),
            gold_parts: Vec::new(),
            candidate_parts: Vec::new(),
            gold_contexts: Vec::new(),
            candidate_contexts: Vec::new(),
            gold_citations: Vec::new(),
            candidate_citations: Vec::new(),
        };
        if let Some((_, from, to)) = PAIRED_ASSETS.iter().find(|(name, _, _)| *name == asset) {
            let gold_ends = gold
                .iter()
                .map(|record| (end_of(record, from), end_of(record, to)))
                .collect();
            let candidate_ends = candidate
                .iter()
                .map(|record| {
                    let mapped = |end: &str| {
                        end_of(record, end).and_then(|context| context_of.get(&context).cloned())
                    };
                    (mapped(from), mapped(to))
                })
                .collect();
            scorer.ends = Some((gold_ends, candidate_ends));
            return Ok(scorer);
        }
        scorer.gold_names = gold.iter().map(|r| names(asset, r, true)).collect();
        scorer.candidate_names = candidate.iter().map(|r| names(asset, r, false)).collect();
        if asset == "contexts" {
            scorer.gold_parts = gold
                .iter()
                .map(|r| path_parts(r.get("implementation_path")))
                .collect();
            scorer.candidate_parts = candidate
                .iter()
                .map(|r| path_parts(r.get("implementation_path")))
                .collect();
            return Ok(scorer);
        }
        scorer.gold_contexts = gold.iter().map(|r| contexts_named_by(asset, r)).collect();
        scorer.candidate_contexts = candidate
            .iter()
            .map(|r| mapped(asset, r, context_of))
            .collect();
        scorer.gold_citations = gold
            .iter()
            .map(|r| citations(r))
            .collect::<Result<_, _>>()?;
        scorer.candidate_citations = candidate
            .iter()
            .map(|r| citations(r))
            .collect::<Result<_, _>>()?;
        Ok(scorer)
    }

    fn score(&self, gold_index: usize, index: usize) -> Option<Score> {
        if let Some((gold_ends, candidate_ends)) = &self.ends {
            return (candidate_ends[index] == gold_ends[gold_index]).then_some((EXACT_PATH, 0));
        }
        let same_name = !self.gold_names[gold_index].is_disjoint(&self.candidate_names[index]);
        if self.asset == "contexts" {
            let tier = path_score(&self.gold_parts[gold_index], &self.candidate_parts[index]);
            let tier = if tier == 0 && same_name {
                SAME_NAME
            } else {
                tier
            };
            return (tier != 0).then_some((tier, 0));
        }
        if same_name {
            return Some((SAME_NAME, 0));
        }
        let shares_context = self.gold_contexts[gold_index]
            .iter()
            .any(|context| self.candidate_contexts[index].contains(&Some(context.clone())));
        if !shares_context {
            return None;
        }
        let (gold, candidate) = (
            &self.gold_citations[gold_index],
            &self.candidate_citations[index],
        );
        let shared = shared_files(gold, candidate);
        if shared.is_empty() {
            return None;
        }
        Some((SHARED_EVIDENCE, overlapping_lines(gold, candidate, &shared)))
    }
}

fn matched(
    asset: &str,
    gold: &Records,
    candidate: &Records,
    context_of: &HashMap<String, String>,
) -> Result<Matched, String> {
    let scorer = Scorer::new(asset, gold, candidate, context_of)?;
    let mut scored: Vec<(Score, usize, usize)> = Vec::new();
    for index in 0..candidate.len() {
        for gold_index in 0..gold.len() {
            if let Some(score) = scorer.score(gold_index, index) {
                scored.push((score, index, gold_index));
            }
        }
    }
    scored.sort_by_key(|((tier, lines), index, gold_index)| {
        (
            std::cmp::Reverse(*tier),
            std::cmp::Reverse(*lines),
            *index,
            *gold_index,
        )
    });
    let mut taken_candidates = BTreeSet::new();
    let mut taken_gold = BTreeSet::new();
    let mut pairs: Vec<Object> = Vec::new();
    for ((tier, _), index, gold_index) in scored {
        if taken_candidates.contains(&index) || taken_gold.contains(&gold_index) {
            continue;
        }
        taken_candidates.insert(index);
        taken_gold.insert(gold_index);
        let mut pair = Object::new();
        pair.insert("candidate", Value::Str(id_of(candidate[index])));
        pair.insert("gold", Value::Str(id_of(gold[gold_index])));
        pair.insert("by", Value::Str(label(tier).to_string()));
        if asset != "contexts" && cites_within_the_other(gold[gold_index], candidate[index])? {
            pair.insert("copied_evidence", Value::Bool(true));
        }
        pairs.push(pair);
    }
    let missed: Vec<usize> = (0..gold.len())
        .filter(|i| !taken_gold.contains(i))
        .collect();
    let extra: Vec<usize> = (0..candidate.len())
        .filter(|i| !taken_candidates.contains(i))
        .collect();
    pairs.sort_by_key(|pair| pair.get("gold").map(str_of).unwrap_or_default());
    let sorted_ids = |indices: &[usize], records: &Records| {
        let mut ids: Vec<String> = indices.iter().map(|i| id_of(records[*i])).collect();
        ids.sort();
        name_list(ids)
    };
    let mut result = Object::new();
    result.insert("gold", Value::int(gold.len()));
    result.insert("candidate", Value::int(candidate.len()));
    result.insert(
        "matched",
        Value::Array(pairs.iter().cloned().map(Value::Object).collect()),
    );
    result.insert("missed", sorted_ids(&missed, gold));
    result.insert("extra", sorted_ids(&extra, candidate));
    result.insert(
        "possible_pairs",
        Value::Array(
            possible_pairs(asset, gold, candidate, context_of, (&missed, &extra))
                .into_iter()
                .map(Value::Object)
                .collect(),
        ),
    );
    for (key, value) in scores(pairs.len(), gold.len(), candidate.len()) {
        result.insert(key, value);
    }
    Ok(Matched {
        gold: gold.len(),
        candidate: candidate.len(),
        pairs,
        result,
    })
}

fn overall(results: &[&Matched], left_out: usize) -> Value {
    let matched: usize = results.iter().map(|r| r.pairs.len()).sum::<usize>() - left_out;
    let gold: usize = results.iter().map(|r| r.gold).sum::<usize>() - left_out;
    let candidate: usize = results.iter().map(|r| r.candidate).sum::<usize>() - left_out;
    let mut counts = Object::new();
    counts.insert("matched", Value::int(matched));
    counts.insert("gold", Value::int(gold));
    counts.insert("candidate", Value::int(candidate));
    for (key, value) in scores(matched, gold, candidate) {
        counts.insert(key, value);
    }
    Value::Object(counts)
}

fn within<'a>(
    records: &[(&'a str, Records<'a>)],
    contexts: &BTreeSet<String>,
) -> Vec<(&'a str, Records<'a>)> {
    records
        .iter()
        .map(|(asset, entries)| {
            let kept = entries
                .iter()
                .filter(|record| {
                    if *asset == "contexts" {
                        contexts.contains(&id_of(record))
                    } else {
                        !contexts_named_by(asset, record).is_disjoint(contexts)
                    }
                })
                .copied()
                .collect();
            (*asset, kept)
        })
        .collect()
}

fn records_by_asset(documents: &Object) -> Vec<(&'static str, Records<'_>)> {
    ASSETS
        .iter()
        .map(|(asset, key, _)| (*asset, records_of(documents, asset, key)))
        .collect()
}

fn find<'a, 'b>(records: &'b [(&str, Records<'a>)], asset: &str) -> &'b Records<'a> {
    &records
        .iter()
        .find(|(name, _)| *name == asset)
        .expect("asset")
        .1
}

fn context_map(contexts: &Matched) -> HashMap<String, String> {
    contexts
        .pairs
        .iter()
        .map(|pair| {
            (
                pair.get("candidate").map(str_of).unwrap_or_default(),
                pair.get("gold").map(str_of).unwrap_or_default(),
            )
        })
        .collect()
}

pub fn compare_registries(
    gold: &Object,
    candidate: &Object,
    contexts: &[String],
) -> Result<Comparison, String> {
    let mut gold_records = records_by_asset(gold);
    let mut candidate_records = records_by_asset(candidate);
    let mut matched_contexts = matched(
        "contexts",
        find(&gold_records, "contexts"),
        find(&candidate_records, "contexts"),
        &HashMap::new(),
    )?;
    let context_of = context_map(&matched_contexts);
    let wanted: BTreeSet<String> = contexts.iter().cloned().collect();
    if !contexts.is_empty() {
        let known: BTreeSet<String> = find(&gold_records, "contexts")
            .iter()
            .map(|record| id_of(record))
            .collect();
        let unknown: Vec<String> = wanted.difference(&known).cloned().collect();
        if !unknown.is_empty() {
            return Err(format!(
                "no Context {} in the gold Registry",
                unknown.join(", ")
            ));
        }
        gold_records = within(&gold_records, &wanted);
        let paired_candidates: BTreeSet<String> = context_of
            .iter()
            .filter(|(_, paired)| wanted.contains(*paired))
            .map(|(candidate, _)| candidate.clone())
            .collect();
        candidate_records = within(&candidate_records, &paired_candidates);
        matched_contexts = matched(
            "contexts",
            find(&gold_records, "contexts"),
            find(&candidate_records, "contexts"),
            &HashMap::new(),
        )?;
    }
    let mut assets: Vec<(&str, Matched)> = vec![("contexts", matched_contexts)];
    for (asset, entries) in &gold_records {
        if *asset != "contexts" {
            let result = matched(asset, entries, find(&candidate_records, asset), &context_of)?;
            assets.push((asset, result));
        }
    }
    let every: Vec<&Matched> = assets.iter().map(|(_, result)| result).collect();
    let attempted: Vec<&Matched> = every
        .iter()
        .copied()
        .filter(|result| result.candidate > 0 && result.gold > 0)
        .collect();
    let copied = attempted
        .iter()
        .flat_map(|result| &result.pairs)
        .filter(|pair| pair.get("copied_evidence").is_some())
        .count();
    let attempted_matches: usize = attempted.iter().map(|result| result.pairs.len()).sum();
    let mut shown_assets = Object::new();
    for (asset, result) in &assets {
        shown_assets.insert(*asset, Value::Object(result.result.clone()));
    }
    let mut report = Object::new();
    report.insert("assets", Value::Object(shown_assets));
    report.insert("overall", overall(&every, 0));
    report.insert("overall_attempted", overall(&attempted, 0));
    report.insert("overall_independent", overall(&attempted, copied));
    report.insert("copied_matches", Value::int(copied));
    let mut without_candidate = Vec::new();
    if !contexts.is_empty() {
        let paired: BTreeSet<String> = assets[0]
            .1
            .pairs
            .iter()
            .map(|pair| pair.get("gold").map(str_of).unwrap_or_default())
            .collect();
        without_candidate = wanted.difference(&paired).cloned().collect();
        let mut scope = Object::new();
        scope.insert("contexts", name_list(wanted.iter().cloned().collect()));
        scope.insert("without_candidate", name_list(without_candidate.clone()));
        report.insert("scope", Value::Object(scope));
    }
    Ok(Comparison {
        report,
        copied_matches: copied,
        attempted_matches,
        without_candidate,
    })
}
