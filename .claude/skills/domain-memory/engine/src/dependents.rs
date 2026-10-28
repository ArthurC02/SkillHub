use std::collections::{HashMap, HashSet, VecDeque};
use std::path::Path;

use crate::context::summarized;
use crate::json::{Object, Value};
use crate::okf::{LIST_RELATIONS, RELATION_TARGETS};
use crate::records::{did_you_mean, read_record};
use crate::store::{ASSETS, Registry, asset_records};

const HUB_ASSET: &str = "contexts";
const NOUN_ASSETS: &[(&str, &str)] = &[
    ("Context", "contexts"),
    ("Aggregate", "aggregates"),
    ("Contract", "contracts"),
    ("Term", "vocabulary"),
    ("Decision", "decisions"),
];

type Key = (String, String);

pub fn dependents(root: &Path, asset: &str, identifier: &str) -> Result<Value, String> {
    if read_record(root, asset, identifier)?.is_none() {
        let known = asset_records(root, asset)?;
        return Err(format!(
            "{asset} record not found: {identifier}{}",
            did_you_mean(identifier, &known)
        ));
    }
    let registry = Registry::open(root)?;
    let mut records: Vec<(&str, Vec<Object>)> = Vec::new();
    for (name, _, _) in ASSETS {
        let entries = registry
            .records(name)?
            .into_iter()
            .filter_map(|entry| match entry {
                Value::Object(entry) => Some(entry),
                _ => None,
            })
            .collect();
        records.push((name, entries));
    }
    let reached = DependencyGraph::from_records(&records).reached(asset, identifier);
    let mut reply = Object::new();
    reply.insert("asset", Value::Str(asset.to_string()));
    reply.insert("id", Value::Str(identifier.to_string()));
    reply.insert(
        "reached",
        Value::Array(reached.into_iter().map(Value::Object).collect()),
    );
    Ok(Value::Object(reply))
}

pub struct DependencyGraph {
    by_key: HashMap<Key, Object>,
    edges: HashMap<Key, Vec<(Key, String)>>,
}

impl DependencyGraph {
    pub fn from_records(records: &[(&str, Vec<Object>)]) -> Self {
        let mut by_key: HashMap<Key, Object> = HashMap::new();
        let mut edges: HashMap<Key, Vec<(Key, String)>> = HashMap::new();
        for (name, entries) in records {
            for entry in entries {
                let id = entry.get("id").and_then(Value::as_str).unwrap_or_default();
                let source = ((*name).to_string(), id.to_string());
                link(&mut edges, &source, entry);
                by_key.insert(source, entry.clone());
            }
        }
        Self { by_key, edges }
    }

    pub fn record(&self, asset: &str, identifier: &str) -> Option<&Object> {
        self.by_key
            .get(&(asset.to_string(), identifier.to_string()))
    }

    pub fn reached(&self, asset: &str, identifier: &str) -> Vec<Object> {
        let start = (asset.to_string(), identifier.to_string());
        let mut seen: HashSet<Key> = HashSet::new();
        let mut reached: Vec<(usize, Key, Object)> = Vec::new();
        let mut queue = VecDeque::from([(start.clone(), 0)]);
        while let Some((key, depth)) = queue.pop_front() {
            if key.0 == HUB_ASSET && key != start {
                continue;
            }
            for (other, through) in self.edges.get(&key).map(Vec::as_slice).unwrap_or_default() {
                if *other == start || seen.contains(other) {
                    continue;
                }
                let Some(entry) = self.by_key.get(other) else {
                    continue;
                };
                let mut item = summarized(entry);
                item.insert("asset", Value::Str(other.0.clone()));
                item.insert("through", Value::Str(through.clone()));
                item.insert("depth", Value::int(depth + 1));
                seen.insert(other.clone());
                reached.push((depth + 1, other.clone(), item));
                queue.push_back((other.clone(), depth + 1));
            }
        }
        reached.sort_by(|left, right| (left.0, &left.1).cmp(&(right.0, &right.1)));
        reached.into_iter().map(|(_, _, item)| item).collect()
    }
}

fn link(edges: &mut HashMap<Key, Vec<(Key, String)>>, source: &Key, entry: &Object) {
    for (field, noun) in RELATION_TARGETS {
        let Some((_, target_asset)) = NOUN_ASSETS.iter().find(|(name, _)| name == noun) else {
            continue;
        };
        let value = entry.get(field);
        let targets: Vec<&Value> = match value {
            Some(Value::Array(items)) if LIST_RELATIONS.contains(field) => items.iter().collect(),
            Some(single) => vec![single],
            None => Vec::new(),
        };
        for target in targets.into_iter().filter_map(Value::as_str) {
            let through = format!("{}/{}.{field}", source.0, source.1);
            let target = ((*target_asset).to_string(), target.to_string());
            edges
                .entry(source.clone())
                .or_default()
                .push((target.clone(), through.clone()));
            edges
                .entry(target)
                .or_default()
                .push((source.clone(), through));
        }
    }
}
