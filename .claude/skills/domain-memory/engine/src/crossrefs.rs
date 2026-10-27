use std::collections::HashSet;

use crate::coverage::holding;
use crate::indexes::Loaded;
use crate::json::{Object, Value};
use crate::py;
use crate::pyvalue::{repr_of, str_of};
use crate::unicode::casefold;

const ABSENCE_ASSETS: [&str; 5] = ["aggregates", "contracts", "events", "rules", "vocabulary"];
const SUBDOMAINS: [&str; 3] = ["core", "generic", "supporting"];
const SHARED_KERNEL_HINT: &str =
    "; shared-kernel is a relationship between two Contexts, recorded on their Interaction";
const SEPARATE_WAYS_HINT: &str = "; separate-ways is a decision not to collaborate, recorded as a dependency policy with policy forbidden and mode none";
const SINGLE_CONTEXT_FIELDS: &[(&str, &[&str])] = &[
    ("aggregates", &["context"]),
    ("interactions", &["producer_context", "consumer_context"]),
    ("events", &["owner_context"]),
    ("capabilities", &["context"]),
    ("value-objects", &["context"]),
];
const CLOSED_FIELDS: &[(&str, &str, &[&str])] = &[
    (
        "interactions",
        "relationship",
        &[
            "anticorruption-layer",
            "conformist",
            "customer-supplier",
            "open-host-service",
            "partnership",
            "published-language",
            "shared-kernel",
        ],
    ),
    (
        "dependency-policies",
        "mode",
        &[
            "asynchronous",
            "none",
            "synchronous-query",
            "synchronous-write",
        ],
    ),
    ("dependency-policies", "policy", &["allowed", "forbidden"]),
];
const IDEMPOTENCY_FIELDS: [&str; 4] = ["key", "scope", "on_replay", "on_conflict"];
const OPTIONAL_IDEMPOTENCY_FIELDS: [&str; 1] = ["expiry"];

pub fn records_of<'a>(loaded: &'a Loaded, asset: &str) -> &'a [Object] {
    loaded
        .iter()
        .find(|(name, _)| *name == asset)
        .map_or(&[], |(_, records)| records)
}

pub fn identifier(entry: &Object) -> String {
    entry.get("id").map_or_else(|| "None".to_string(), str_of)
}

fn present<'a>(entry: &'a Object, key: &str) -> Option<&'a Value> {
    entry.get(key).filter(|value| !matches!(value, Value::Null))
}

fn text<'a>(entry: &'a Object, key: &str) -> Option<&'a str> {
    entry.get(key).and_then(Value::as_str)
}

fn strings<'a>(entry: &'a Object, key: &str) -> Vec<&'a str> {
    entry
        .get(key)
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_str)
        .collect()
}

fn nonblank(value: &Value) -> bool {
    value
        .as_str()
        .is_some_and(|text| !py::strip(text).is_empty())
}

fn unknown_contexts(names: &[Option<&str>]) -> String {
    let mut shown: Vec<&str> = names.iter().map(|name| name.unwrap_or("None")).collect();
    shown.sort_unstable();
    format!(
        "unknown context {}; add the Context first",
        shown.join(", ")
    )
}

fn unknown_among(named: &[&str], known: &HashSet<&str>) -> Option<String> {
    let mut missing: Vec<Option<&str>> = Vec::new();
    for name in named.iter().filter(|name| !known.contains(**name)) {
        if !missing.contains(&Some(*name)) {
            missing.push(Some(name));
        }
    }
    (!missing.is_empty()).then(|| unknown_contexts(&missing))
}

pub fn cross_reference_errors(loaded: &Loaded) -> Vec<String> {
    let mut errors = Vec::new();
    placement_errors(records_of(loaded, "contexts"), &mut errors);
    confirmed_absence_errors(loaded, &mut errors);
    context_reference_errors(loaded, &mut errors);
    term_label_errors(records_of(loaded, "vocabulary"), &mut errors);
    idempotency_errors(records_of(loaded, "contracts"), &mut errors);
    errors
}

fn placement_errors(contexts: &[Object], errors: &mut Vec<String>) {
    let mut claimed: Vec<(String, String)> = Vec::new();
    for entry in contexts {
        let label = format!("contexts/{}", identifier(entry));
        if let Some(subdomain) = present(entry, "subdomain")
            && !subdomain
                .as_str()
                .is_some_and(|name| SUBDOMAINS.contains(&name))
        {
            let hint = if subdomain.as_str() == Some("shared-kernel") {
                SHARED_KERNEL_HINT
            } else {
                ""
            };
            errors.push(format!(
                "{label} has an unknown subdomain: {} is not one of {}{hint}",
                repr_of(subdomain),
                SUBDOMAINS.join(", ")
            ));
        }
        if let Some(prefixes) = present(entry, "requirement_prefixes")
            && !prefixes
                .as_array()
                .is_some_and(|items| items.iter().all(nonblank))
        {
            errors.push(format!(
                "{label} requirement_prefixes must be a list of non-empty strings"
            ));
        }
        let Some(path) = present(entry, "implementation_path") else {
            continue;
        };
        let Some(path) = path.as_str().filter(|path| !py::strip(path).is_empty()) else {
            errors.push(format!(
                "{label} implementation_path must be a non-empty string"
            ));
            continue;
        };
        for (other, owner) in &claimed {
            if paths_overlap(path, other) {
                errors.push(format!(
                    "{label} claims {path}, which overlaps {other} claimed by {owner}: code belongs to one Context"
                ));
            }
        }
        let owner = identifier(entry);
        match claimed.iter_mut().find(|(seen, _)| seen == path) {
            Some(slot) => slot.1 = owner,
            None => claimed.push((path.to_string(), owner)),
        }
    }
}

fn paths_overlap(one: &str, other: &str) -> bool {
    let first: Vec<&str> = one.trim_matches('/').split('/').collect();
    let second: Vec<&str> = other.trim_matches('/').split('/').collect();
    let shared = first.len().min(second.len());
    first[..shared] == second[..shared]
}

fn absent_asset(declaration: &Value) -> Option<&str> {
    declaration
        .as_object()
        .and_then(|declaration| text(declaration, "asset"))
        .filter(|asset| ABSENCE_ASSETS.contains(asset))
}

fn confirmed_absence_errors(loaded: &Loaded, errors: &mut Vec<String>) {
    for entry in records_of(loaded, "contexts") {
        let label = format!("contexts/{}", identifier(entry));
        let declarations = match entry.get("confirmed_absences") {
            None => &[][..],
            Some(Value::Array(items)) => items,
            Some(_) => {
                errors.push(format!("{label} confirmed_absences must be a list"));
                continue;
            }
        };
        for declaration in declarations {
            let Some(asset) = absent_asset(declaration) else {
                errors.push(format!(
                    "{label} confirms an absence of an unknown asset; use one of {}",
                    ABSENCE_ASSETS.join(", ")
                ));
                continue;
            };
            let reason = declaration
                .as_object()
                .and_then(|found| found.get("reason"));
            if !reason.is_some_and(nonblank) {
                errors.push(format!(
                    "{label} confirms an absence of {asset} without a reason; a confirmed absence is a finding and needs one"
                ));
            } else if text(entry, "id")
                .is_some_and(|id| holding(asset, records_of(loaded, asset)).contains(&id))
            {
                errors.push(format!(
                    "{label} confirms an absence of {asset}, but {asset} records name it"
                ));
            }
        }
    }
}

fn context_reference_errors(loaded: &Loaded, errors: &mut Vec<String>) {
    let known: HashSet<&str> = records_of(loaded, "contexts")
        .iter()
        .filter_map(|entry| text(entry, "id"))
        .collect();
    context_list_errors(loaded, &known, errors);
    single_context_errors(loaded, &known, errors);
    contract_party_errors(records_of(loaded, "contracts"), &known, errors);
    event_consumer_errors(records_of(loaded, "events"), &known, errors);
    contract_errors(loaded, errors);
    aggregate_binding_errors(loaded, errors);
    closed_field_errors(loaded, errors);
}

fn context_list_errors(loaded: &Loaded, known: &HashSet<&str>, errors: &mut Vec<String>) {
    for asset in ["vocabulary", "rules"] {
        for entry in records_of(loaded, asset) {
            let label = format!("{asset}/{}", identifier(entry));
            match entry.get("contexts") {
                Some(Value::Array(items)) if !items.is_empty() => {
                    if let Some(unknown) = unknown_among(&strings(entry, "contexts"), known) {
                        errors.push(format!("{label} references {unknown}"));
                    }
                }
                _ => errors.push(format!("{label} requires contexts")),
            }
        }
    }
}

fn single_context_errors(loaded: &Loaded, known: &HashSet<&str>, errors: &mut Vec<String>) {
    for (asset, fields) in SINGLE_CONTEXT_FIELDS {
        for entry in records_of(loaded, asset) {
            for field in *fields {
                let named = text(entry, field);
                if !named.is_some_and(|name| known.contains(name)) {
                    errors.push(format!(
                        "{asset}/{} references {}",
                        identifier(entry),
                        unknown_contexts(&[named])
                    ));
                }
            }
        }
    }
    for entry in records_of(loaded, "dependency-policies") {
        let mut ends = vec![text(entry, "from_context")];
        let to = text(entry, "to_context");
        if !ends.contains(&to) {
            ends.push(to);
        }
        let missing: Vec<Option<&str>> = ends
            .into_iter()
            .filter(|end| !end.is_some_and(|name| known.contains(name)))
            .collect();
        if !missing.is_empty() {
            errors.push(format!(
                "dependency-policies/{} references {}",
                identifier(entry),
                unknown_contexts(&missing)
            ));
        }
    }
}

fn contract_party_errors(contracts: &[Object], known: &HashSet<&str>, errors: &mut Vec<String>) {
    for entry in contracts {
        let label = format!("contracts/{}", identifier(entry));
        let producer = text(entry, "producer_context");
        if !producer.is_some_and(|name| known.contains(name)) {
            errors.push(format!(
                "{label} references as producer {}",
                unknown_contexts(&[producer])
            ));
        }
        match entry.get("consumer_contexts") {
            None => {}
            Some(Value::Array(_)) => {
                if let Some(unknown) = unknown_among(&strings(entry, "consumer_contexts"), known) {
                    errors.push(format!("{label} references as consumer {unknown}"));
                }
            }
            Some(_) => errors.push(format!(
                "{label} consumer_contexts must be a list of Context ids"
            )),
        }
    }
}

fn event_consumer_errors(events: &[Object], known: &HashSet<&str>, errors: &mut Vec<String>) {
    for entry in events {
        let label = format!("events/{}", identifier(entry));
        match present(entry, "consumers") {
            None => {}
            Some(Value::Array(_)) => {
                if let Some(unknown) = unknown_among(&strings(entry, "consumers"), known) {
                    errors.push(format!("{label} references as consumer {unknown}"));
                }
            }
            Some(_) => errors.push(format!("{label} consumers must be a list of Context ids")),
        }
    }
}

fn contract_errors(loaded: &Loaded, errors: &mut Vec<String>) {
    let contract_ids: HashSet<&str> = records_of(loaded, "contracts")
        .iter()
        .filter_map(|entry| text(entry, "id"))
        .collect();
    for asset in ["interactions", "events"] {
        for entry in records_of(loaded, asset) {
            if let Some(contract) = present(entry, "contract_id")
                && !contract
                    .as_str()
                    .is_some_and(|id| contract_ids.contains(id))
            {
                errors.push(format!(
                    "{asset}/{} references unknown contract",
                    identifier(entry)
                ));
            }
        }
    }
}

fn named_contexts(entry: &Object) -> Vec<Option<&str>> {
    match entry.get("contexts") {
        Some(Value::Array(_)) => strings(entry, "contexts").into_iter().map(Some).collect(),
        _ => vec![text(entry, "owner_context")],
    }
}

fn aggregate_binding_errors(loaded: &Loaded, errors: &mut Vec<String>) {
    let homes: Vec<(&str, Option<&str>)> = records_of(loaded, "aggregates")
        .iter()
        .filter_map(|entry| Some((text(entry, "id")?, text(entry, "context"))))
        .collect();
    for asset in ["rules", "events"] {
        for entry in records_of(loaded, asset) {
            let Some(aggregate) = text(entry, "aggregate") else {
                continue;
            };
            let label = format!("{asset}/{}", identifier(entry));
            match homes.iter().find(|(id, _)| *id == aggregate) {
                None => errors.push(format!(
                    "{label} references unknown aggregate {aggregate}; add the Aggregate first"
                )),
                Some((_, home)) if !named_contexts(entry).contains(home) => {
                    errors.push(format!(
                        "{label} aggregate {aggregate} belongs to context {}, which the record does not name",
                        home.unwrap_or("None")
                    ));
                }
                Some(_) => {}
            }
        }
    }
}

fn closed_field_errors(loaded: &Loaded, errors: &mut Vec<String>) {
    for asset in ["interactions", "dependency-policies"] {
        for entry in records_of(loaded, asset) {
            for (_, field, allowed) in CLOSED_FIELDS.iter().filter(|(owner, _, _)| *owner == asset)
            {
                let Some(value) = present(entry, field) else {
                    continue;
                };
                if value.as_str().is_some_and(|name| allowed.contains(&name)) {
                    continue;
                }
                let hint = match (*field, value.as_str()) {
                    ("relationship", Some("separate-ways")) => SEPARATE_WAYS_HINT,
                    _ => "",
                };
                errors.push(format!(
                    "{asset}/{} has an unknown {field}: {} is not one of {}{hint}",
                    identifier(entry),
                    repr_of(value),
                    allowed.join(", ")
                ));
            }
        }
    }
}

fn labels(entry: &Object, field: &str) -> Option<Vec<String>> {
    match present(entry, field) {
        None => Some(Vec::new()),
        Some(Value::Array(items)) if items.iter().all(nonblank) => Some(
            items
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_string)
                .collect(),
        ),
        Some(_) => None,
    }
}

fn name_text(term: &Object) -> String {
    term.get("name").map_or_else(String::new, str_of)
}

fn term_label_errors(terms: &[Object], errors: &mut Vec<String>) {
    let term_ids: HashSet<&str> = terms.iter().filter_map(|term| text(term, "id")).collect();
    for (at, term) in terms.iter().enumerate() {
        let location = format!("vocabulary/{}", identifier(term));
        let (Some(synonyms), Some(look_alikes)) =
            (labels(term, "synonyms"), labels(term, "not_same_as"))
        else {
            errors.push(format!(
                "{location} synonyms and not_same_as must be lists of non-empty strings"
            ));
            continue;
        };
        synonym_errors(&location, at, terms, &synonyms, errors);
        let names_other = look_alikes
            .iter()
            .any(|name| !term_ids.contains(name.as_str()))
            || text(term, "id").is_some_and(|id| look_alikes.iter().any(|name| name == id));
        if names_other {
            errors.push(format!(
                "{location} not_same_as must name other vocabulary term ids"
            ));
        }
    }
}

fn synonym_errors(
    location: &str,
    at: usize,
    terms: &[Object],
    synonyms: &[String],
    errors: &mut Vec<String>,
) {
    let folded: Vec<String> = synonyms.iter().map(|synonym| casefold(synonym)).collect();
    let distinct: HashSet<&String> = folded.iter().collect();
    if distinct.len() != folded.len() {
        errors.push(format!("{location} repeats a synonym"));
    }
    let term = &terms[at];
    if folded.contains(&casefold(&name_text(term))) {
        errors.push(format!("{location} lists its own name as a synonym"));
    }
    let contexts = strings(term, "contexts");
    for (index, other) in terms.iter().enumerate() {
        let shares_context = strings(other, "contexts")
            .iter()
            .any(|context| contexts.contains(context));
        if index != at && folded.contains(&casefold(&name_text(other))) && shares_context {
            errors.push(format!(
                "{location} synonym {} is the name of {} in a shared context",
                other
                    .get("name")
                    .map_or_else(|| "None".to_string(), repr_of),
                identifier(other)
            ));
        }
    }
}

fn idempotency_errors(contracts: &[Object], errors: &mut Vec<String>) {
    for contract in contracts {
        let Some(idempotency) = present(contract, "idempotency") else {
            continue;
        };
        let location = format!("contracts/{}", identifier(contract));
        let Some(idempotency) = idempotency.as_object() else {
            errors.push(format!("{location} idempotency must be an object"));
            continue;
        };
        let keys: Vec<&str> = idempotency.iter().map(|(key, _)| key.as_str()).collect();
        let missing: Vec<&str> = IDEMPOTENCY_FIELDS
            .into_iter()
            .filter(|field| !keys.contains(field))
            .collect();
        let mut unknown: Vec<&str> = keys
            .iter()
            .copied()
            .filter(|key| {
                !IDEMPOTENCY_FIELDS.contains(key) && !OPTIONAL_IDEMPOTENCY_FIELDS.contains(key)
            })
            .collect();
        unknown.sort_unstable();
        let mut blank: Vec<&str> = idempotency
            .iter()
            .filter(|(key, value)| !unknown.contains(&key.as_str()) && !nonblank(value))
            .map(|(key, _)| key.as_str())
            .collect();
        blank.sort_unstable();
        if !missing.is_empty() {
            errors.push(format!(
                "{location} idempotency lacks {}",
                missing.join(", ")
            ));
        }
        if !unknown.is_empty() {
            errors.push(format!(
                "{location} idempotency has unknown fields {}",
                unknown.join(", ")
            ));
        }
        if !blank.is_empty() {
            errors.push(format!(
                "{location} idempotency fields must be non-empty text: {}",
                blank.join(", ")
            ));
        }
    }
}
