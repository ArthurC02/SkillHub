#[allow(dead_code, unused_imports)]
#[path = "../src/json.rs"]
mod json;

mod okf {
    pub const INT_MAX_STR_DIGITS: usize = 4300;
}

use std::collections::{BTreeMap, BTreeSet};
use std::path::{Path, PathBuf};

use json::Value;

const CODE_RULES_HEADING: &str = "## When you are the one writing the code";
const ROOT_OPTIONS: [&str; 4] = ["--registry-root", "x", "--repo-root", "x"];

struct OptionSpec {
    names: Vec<String>,
    dest: String,
    kind: String,
    required: bool,
    choices: Option<Vec<String>>,
}

struct CommandContract {
    options: Vec<OptionSpec>,
    mutex: Vec<(bool, Vec<String>)>,
}

fn with_roots(arguments: &[String], roots: &[&str]) -> Vec<String> {
    let mut all = arguments.to_vec();
    all.extend(roots.iter().map(ToString::to_string));
    all
}

fn plugin() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .expect("the engine sits inside the plugin")
        .to_path_buf()
}

fn read(path: &Path) -> String {
    std::fs::read_to_string(path)
        .unwrap_or_else(|error| panic!("cannot read {}: {error}", path.display()))
        .replace("\r\n", "\n")
}

fn member<'a>(value: &'a Value, key: &str) -> &'a Value {
    value
        .as_object()
        .and_then(|object| object.get(key))
        .unwrap_or_else(|| panic!("the CLI contract lacks {key}"))
}

fn contract() -> Value {
    let path = plugin().join("engine/contract/cli.json");
    json::parse(&read(&path)).unwrap_or_else(|()| panic!("{} is not JSON", path.display()))
}

fn text_of(value: &Value) -> String {
    value.as_str().expect("a string").to_string()
}

fn strings_of(value: &Value) -> Vec<String> {
    value
        .as_array()
        .expect("an array")
        .iter()
        .map(text_of)
        .collect()
}

fn command_contracts() -> BTreeMap<String, CommandContract> {
    let contract = contract();
    let commands = member(&contract, "commands").as_object().expect("commands");
    commands
        .iter()
        .map(|(name, command)| {
            let options = member(command, "actions")
                .as_array()
                .expect("actions")
                .iter()
                .filter(|action| text_of(member(action, "kind")) != "help")
                .map(|action| OptionSpec {
                    names: strings_of(member(action, "options")),
                    dest: text_of(member(action, "dest")),
                    kind: text_of(member(action, "kind")),
                    required: matches!(member(action, "required"), Value::Bool(true)),
                    choices: match member(action, "choices") {
                        Value::Null => None,
                        choices => Some(strings_of(choices)),
                    },
                })
                .collect();
            let mutex = member(command, "mutex")
                .as_array()
                .expect("mutex")
                .iter()
                .map(|group| {
                    (
                        matches!(member(group, "required"), Value::Bool(true)),
                        strings_of(member(group, "dests")),
                    )
                })
                .collect();
            (name.clone(), CommandContract { options, mutex })
        })
        .collect()
}

fn root_help() -> String {
    text_of(member(&contract(), "help"))
}

fn commands_of_group(group_label: &str) -> Vec<String> {
    let help = root_help();
    let after = help
        .split_once(group_label)
        .unwrap_or_else(|| panic!("root help lacks {group_label}"))
        .1;
    let list = after.split("\n\n").next().expect("a group list");
    list.split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
        .trim_end_matches('.')
        .split(", ")
        .map(str::to_string)
        .collect()
}

fn command_groups() -> BTreeMap<&'static str, Vec<String>> {
    BTreeMap::from([
        ("everyday", commands_of_group("Everyday commands:")),
        ("advanced", commands_of_group("Advanced commands:")),
    ])
}

fn pages() -> Vec<PathBuf> {
    let root = plugin();
    let mut pages = vec![root.join("SKILL.md"), root.join("AGENTS.md")];
    pages.extend(sorted_matches(&root.join("skills"), |directory| {
        Some(directory.join("SKILL.md")).filter(|skill| skill.is_file())
    }));
    pages.extend(sorted_matches(&root.join("references"), |path| {
        Some(path.to_path_buf()).filter(|path| path.extension().is_some_and(|ext| ext == "md"))
    }));
    pages
}

fn sorted_matches(directory: &Path, pick: impl Fn(&Path) -> Option<PathBuf>) -> Vec<PathBuf> {
    let mut found: Vec<PathBuf> = std::fs::read_dir(directory)
        .unwrap_or_else(|error| panic!("cannot list {}: {error}", directory.display()))
        .filter_map(|entry| pick(&entry.expect("a directory entry").path()))
        .collect();
    found.sort();
    found
}

fn local_targets(page: &Path) -> Vec<String> {
    let text = read(page);
    let mut targets = Vec::new();
    let mut rest = text.as_str();
    while let Some(at) = rest.find("](") {
        rest = &rest[at + 2..];
        let Some(end) = rest.find(')') else { break };
        let link = &rest[..end];
        let remote =
            link.starts_with("http://") || link.starts_with("https://") || link.starts_with('/');
        let target = link.split('#').next().unwrap_or("");
        if !remote && !target.is_empty() {
            targets.push(target.to_string());
        }
    }
    targets
}

fn relative(page: &Path) -> String {
    page.strip_prefix(plugin())
        .expect("a page of the plugin")
        .to_string_lossy()
        .replace('\\', "/")
}

fn section(text: &str, heading: &str) -> String {
    let after = text
        .split_once(heading)
        .unwrap_or_else(|| panic!("no section {heading}"))
        .1;
    after.split("\n## ").next().unwrap_or("").to_string()
}

fn command_rows(reference: &str) -> Vec<(String, Option<String>)> {
    let mut heading: Option<String> = None;
    let mut rows = Vec::new();
    for line in reference.lines() {
        if line.starts_with("## ") {
            heading = Some(line.to_string());
        } else if let Some(name) = row_name(line) {
            rows.push((name, heading.clone()));
        }
    }
    rows
}

fn row_name(line: &str) -> Option<String> {
    let name = line.strip_prefix("### `")?.split('`').next()?;
    let mut chars = name.chars();
    let first_is_letter = chars.next().is_some_and(|c| c.is_ascii_lowercase());
    let rest_fits = chars.all(|c| c.is_ascii_lowercase() || c == '-');
    (first_is_letter && rest_fits && line.starts_with(&format!("### `{name}`")))
        .then(|| name.to_string())
}

fn documented_groups() -> BTreeMap<String, BTreeSet<String>> {
    let reference = read(&plugin().join("references/script-api.md"));
    let mut groups: BTreeMap<String, BTreeSet<String>> = BTreeMap::new();
    for (name, heading) in command_rows(&reference) {
        let group = match heading.as_deref() {
            Some("## Everyday commands") => "everyday".to_string(),
            Some("## Advanced commands") => "advanced".to_string(),
            Some(other) => other.to_string(),
            None => "no heading".to_string(),
        };
        groups.entry(name).or_default().insert(group);
    }
    groups
}

fn documented_tasks() -> BTreeMap<String, String> {
    let reference = read(&plugin().join("references/script-api.md"));
    let lines: Vec<&str> = reference.lines().collect();
    let mut tasks = BTreeMap::new();
    for (at, line) in lines.iter().enumerate() {
        let Some(name) = row_name(line) else { continue };
        let task = lines.get(at + 2).and_then(|sentence| {
            sentence
                .strip_prefix('*')
                .and_then(|inner| inner.strip_suffix(".*"))
        });
        if lines.get(at + 1) == Some(&"") && task.is_some_and(|task| !task.is_empty()) {
            tasks
                .entry(name)
                .or_insert_with(|| task.unwrap_or("").trim().to_string());
        }
    }
    tasks
}

fn one_line_helps() -> BTreeMap<String, String> {
    let help = root_help();
    let listing = help
        .split_once("positional arguments:\n  command\n")
        .expect("root help lists the commands")
        .1
        .split("\n\noptions:")
        .next()
        .expect("a command listing");
    let mut helps: BTreeMap<String, String> = BTreeMap::new();
    let mut current = String::new();
    for line in listing.lines() {
        if line.starts_with("    ") && !line.starts_with("     ") {
            let mut words = line.trim().splitn(2, char::is_whitespace);
            current = words.next().unwrap_or("").to_string();
            helps.insert(
                current.clone(),
                words.next().unwrap_or("").trim().to_string(),
            );
        } else {
            let entry = helps
                .get_mut(&current)
                .expect("a continuation of a command");
            if !entry.is_empty() {
                entry.push(' ');
            }
            entry.push_str(line.trim());
        }
    }
    helps
}

fn common_task_invocations(commands: &BTreeMap<String, CommandContract>) -> Vec<String> {
    let text = read(&plugin().join("AGENTS.md"));
    let table = section(&text, "## Common tasks");
    let mut snippets = Vec::new();
    for (at, piece) in table.split('`').enumerate() {
        let first = piece.split(' ').next().unwrap_or("");
        let starts_like_command = first.chars().next().is_some_and(|c| c.is_ascii_lowercase())
            && first.chars().all(|c| c.is_ascii_lowercase() || c == '-')
            && piece.contains(' ')
            && piece.contains("--");
        if at % 2 == 1 && starts_like_command && commands.contains_key(first) {
            snippets.push(piece.to_string());
        }
    }
    snippets
}

fn invocation_tokens(snippet: &str, command: &CommandContract) -> Vec<String> {
    let mut plain = String::new();
    let mut inside = false;
    for c in snippet.chars() {
        match c {
            '<' => inside = true,
            '>' if inside => {
                inside = false;
                plain.push('x');
            }
            '[' | ']' if !inside => {}
            _ if inside => {}
            _ => plain.push(c),
        }
    }
    let mut tokens: Vec<String> = plain.split_whitespace().map(str::to_string).collect();
    for at in 0..tokens.len().saturating_sub(1) {
        let choices = command
            .options
            .iter()
            .find(|option| option.names.contains(&tokens[at]))
            .and_then(|option| option.choices.as_ref());
        if let Some(choices) = choices.filter(|_| tokens[at + 1] == "x") {
            let mut sorted = choices.clone();
            sorted.sort();
            tokens[at + 1] = sorted[0].clone();
        }
    }
    tokens
}

fn refusal(command: &CommandContract, arguments: &[String]) -> Option<String> {
    let mut seen: BTreeSet<&str> = BTreeSet::new();
    let mut at = 0;
    while at < arguments.len() {
        let token = &arguments[at];
        let Some(option) = command
            .options
            .iter()
            .find(|option| option.names.contains(token))
        else {
            return Some(format!("unrecognized arguments: {token}"));
        };
        seen.insert(&option.dest);
        at += 1;
        if option.kind == "store_true" {
            continue;
        }
        let Some(value) = arguments.get(at) else {
            return Some(format!("argument {token}: expected one argument"));
        };
        if option
            .choices
            .as_ref()
            .is_some_and(|choices| !choices.contains(value))
        {
            return Some(format!("argument {token}: invalid choice: '{value}'"));
        }
        at += 1;
    }
    if let Some(missing) = command
        .options
        .iter()
        .find(|option| option.required && !seen.contains(option.dest.as_str()))
    {
        return Some(format!(
            "the following arguments are required: {}",
            missing.names[0]
        ));
    }
    for (required, dests) in &command.mutex {
        let present = dests
            .iter()
            .filter(|dest| seen.contains(dest.as_str()))
            .count();
        if present > 1 || (*required && present == 0) {
            return Some(format!("mutually exclusive group violated: {dests:?}"));
        }
    }
    None
}

fn engine_commands() -> Vec<String> {
    let dispatch = read(&plugin().join("engine/src/main.rs"));
    let mut names: Vec<String> = dispatch
        .lines()
        .filter_map(|line| {
            let quoted = line.strip_prefix("        ")?.strip_prefix('"')?;
            let (name, after) = quoted.split_once('"')?;
            let letters_only = name.chars().next().is_some_and(|c| c.is_ascii_lowercase())
                && name.chars().all(|c| c.is_ascii_lowercase() || c == '-');
            (letters_only && after.starts_with(" =>")).then(|| name.to_string())
        })
        .collect();
    names.sort();
    names
}

fn code_rules(page: &Path) -> usize {
    let text = read(page);
    let rules = section(&text, CODE_RULES_HEADING);
    rules
        .lines()
        .filter(|line| {
            let digits = line.chars().take_while(char::is_ascii_digit).count();
            digits > 0 && line[digits..].starts_with(". ") && line.len() > digits + 2
        })
        .count()
}

#[test]
fn every_page_links_only_to_files_the_plugin_holds() {
    let pages = pages();
    assert!(pages.len() > 10, "found only {} pages", pages.len());
    let broken: Vec<(String, String)> = pages
        .iter()
        .flat_map(|page| {
            local_targets(page)
                .into_iter()
                .filter(|target| !page.parent().expect("a parent").join(target).is_file())
                .map(|target| (relative(page), target))
        })
        .collect();
    assert_eq!(Vec::<(String, String)>::new(), broken);
}

#[test]
fn every_reference_is_linked_from_a_page_that_chooses_work() {
    let linked: BTreeSet<PathBuf> = pages()
        .iter()
        .flat_map(|page| {
            local_targets(page).into_iter().filter_map(|target| {
                page.parent()
                    .expect("a parent")
                    .join(target)
                    .canonicalize()
                    .ok()
            })
        })
        .collect();
    let references = sorted_matches(&plugin().join("references"), |path| {
        Some(path.to_path_buf()).filter(|path| path.extension().is_some_and(|ext| ext == "md"))
    });
    assert!(!references.is_empty());
    let orphans: Vec<String> = references
        .iter()
        .filter(|path| !linked.contains(&path.canonicalize().expect("a reference")))
        .map(|path| {
            path.file_name()
                .expect("a name")
                .to_string_lossy()
                .to_string()
        })
        .collect();
    assert_eq!(Vec::<String>::new(), orphans);
}

#[test]
fn each_command_is_documented_only_under_the_heading_of_its_group() {
    let expected: BTreeMap<String, BTreeSet<String>> = command_groups()
        .into_iter()
        .flat_map(|(group, names)| {
            names
                .into_iter()
                .map(move |name| (name, BTreeSet::from([group.to_string()])))
        })
        .collect();
    assert_eq!(62, expected.len());
    assert_eq!(expected, documented_groups());
}

#[test]
fn the_root_help_groups_name_exactly_the_commands_the_contract_holds() {
    let grouped: Vec<String> = command_groups().into_values().flatten().collect();
    let held: BTreeSet<String> = command_contracts().into_keys().collect();
    assert_eq!(grouped.len(), held.len());
    assert_eq!(held, grouped.into_iter().collect::<BTreeSet<_>>());
}

#[test]
fn every_invocation_in_the_common_tasks_table_parses_with_the_roots_the_convention_leaves_out() {
    let commands = command_contracts();
    let invocations = common_task_invocations(&commands);
    assert!(
        invocations.len() > 10,
        "found {} invocations",
        invocations.len()
    );
    let mut refused = BTreeMap::new();
    for snippet in invocations {
        let command = &commands[snippet.split(' ').next().expect("a command name")];
        let tokens = invocation_tokens(&snippet, command);
        let arguments = &tokens[1..];
        let variants = [
            with_roots(arguments, &ROOT_OPTIONS[..]),
            with_roots(arguments, &ROOT_OPTIONS[..2]),
            with_roots(arguments, &ROOT_OPTIONS[2..]),
            arguments.to_vec(),
        ];
        let refusals: Vec<Option<String>> = variants
            .iter()
            .map(|variant| {
                let variant: Vec<String> = variant.iter().map(ToString::to_string).collect();
                refusal(command, &variant)
            })
            .collect();
        if refusals.iter().all(Option::is_some) {
            refused.insert(snippet, refusals[0].clone());
        }
    }
    assert_eq!(BTreeMap::<String, Option<String>>::new(), refused);
}

#[test]
fn a_snippet_naming_an_option_its_command_lacks_is_refused_in_every_variant() {
    let commands = command_contracts();
    let command = &commands["readiness"];
    let tokens = invocation_tokens("readiness --repo-root <repo> --not-an-option <x>", command);
    let refusals: Vec<Option<String>> = [&ROOT_OPTIONS[..], &ROOT_OPTIONS[2..]]
        .iter()
        .map(|roots| refusal(command, &with_roots(&tokens[1..], roots)))
        .collect();
    assert_eq!(
        vec![
            Some("unrecognized arguments: --not-an-option".to_string()),
            Some("unrecognized arguments: --not-an-option".to_string())
        ],
        refusals
    );
}

#[test]
fn the_commands_the_contract_holds_equal_the_commands_the_engine_dispatches() {
    let contract: Vec<String> = command_contracts().into_keys().collect();
    assert_eq!(contract, engine_commands());
}

#[test]
fn the_code_rules_live_on_the_entry_page_alone_and_the_hygiene_page_points_there() {
    let hygiene = read(&plugin().join("skills/domain-memory-implementation-hygiene/SKILL.md"));
    assert_eq!(
        (6, true, false),
        (
            code_rules(&plugin().join("AGENTS.md")),
            hygiene.contains("AGENTS.md#when-you-are-the-one-writing-the-code"),
            hygiene.contains(CODE_RULES_HEADING),
        )
    );
}

#[test]
fn every_command_says_in_one_line_what_it_is_for_as_the_reference_does() {
    let helps = one_line_helps();
    let held: BTreeSet<String> = command_contracts().into_keys().collect();
    assert_eq!(held, helps.keys().cloned().collect::<BTreeSet<_>>());
    assert_eq!(helps, documented_tasks());
}

#[test]
fn the_schema_page_names_the_known_values_and_leaves_kind_and_delivery_unchecked() {
    let page = read(&plugin().join("references/registry-schema.md"));
    let missing: Vec<&str> = [
        "is `synchronous` when the consumer sees the change",
        "or `eventual` when it arrives later",
        "A Contract's `kind` and an Interaction's `delivery` stay free prose and are not checked",
    ]
    .into_iter()
    .filter(|sentence| !page.contains(sentence))
    .collect();
    assert_eq!(Vec::<&str>::new(), missing);
}
