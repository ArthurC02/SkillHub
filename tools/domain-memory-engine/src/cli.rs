use std::collections::{HashMap, HashSet};

use crate::json::{self, Value};
use crate::py;

const CONTRACT: &str = include_str!("../contract/cli.json");

#[derive(Clone, Copy, PartialEq)]
enum Kind {
    Store,
    Append,
    StoreTrue,
    Help,
}

#[derive(Clone, Copy, PartialEq)]
enum Type {
    Text,
    Path,
    Int,
    Asset,
    Query,
}

struct Action {
    dest: String,
    options: Vec<String>,
    kind: Kind,
    required: bool,
    kind_of_value: Type,
    choices: Option<Vec<String>>,
    default: Value,
}

struct Mutex {
    required: bool,
    dests: Vec<String>,
}

pub struct Command {
    pub name: String,
    pub prog: String,
    pub usage: String,
    help: String,
    actions: Vec<Action>,
    mutex: Vec<Mutex>,
}

pub struct Contract {
    pub prog: String,
    pub usage: String,
    help: String,
    commands: Vec<Command>,
}

#[derive(Clone, Debug, PartialEq)]
pub enum Arg {
    Absent,
    Text(String),
    Int(i64),
    Flag(bool),
    List(Vec<String>),
}

pub struct Invocation {
    pub command: String,
    values: HashMap<String, Arg>,
}

impl Invocation {
    pub fn flag(&self, dest: &str) -> bool {
        matches!(self.values.get(dest), Some(Arg::Flag(true)))
    }

    pub fn texts(&self, dest: &str) -> &[String] {
        match self.values.get(dest) {
            Some(Arg::List(items)) => items,
            _ => &[],
        }
    }

    pub fn text(&self, dest: &str) -> Option<&str> {
        match self.values.get(dest) {
            Some(Arg::Text(text)) => Some(text),
            _ => None,
        }
    }
}

pub enum Outcome {
    Run(Invocation),
    Help(String),
    Usage {
        usage: String,
        prog: String,
        message: String,
    },
}

impl Contract {
    pub fn embedded() -> Contract {
        let value = json::parse(CONTRACT).expect("the embedded CLI contract is JSON");
        let root = value.as_object().expect("the CLI contract is an object");
        let text = |key: &str| {
            root.get(key)
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_string()
        };
        let commands = root
            .get("commands")
            .and_then(Value::as_object)
            .map(|commands| {
                commands
                    .iter()
                    .map(|(name, spec)| command_from(name, spec))
                    .collect()
            })
            .unwrap_or_default();
        Contract {
            prog: text("prog"),
            usage: text("usage"),
            help: text("help"),
            commands,
        }
    }

    pub fn command(&self, name: &str) -> Option<&Command> {
        self.commands.iter().find(|command| command.name == name)
    }
}

fn command_from(name: &str, spec: &Value) -> Command {
    let object = spec.as_object().expect("a command contract is an object");
    let text = |key: &str| {
        object
            .get(key)
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string()
    };
    let actions = object
        .get("actions")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .map(action_from)
        .collect();
    let mutex = object
        .get("mutex")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_object)
        .map(|group| Mutex {
            required: matches!(group.get("required"), Some(Value::Bool(true))),
            dests: strings(group.get("dests")),
        })
        .collect();
    Command {
        name: name.to_string(),
        prog: text("prog"),
        usage: text("usage"),
        help: text("help"),
        actions,
        mutex,
    }
}

fn action_from(spec: &Value) -> Action {
    let object = spec.as_object().expect("an action contract is an object");
    let text = |key: &str| object.get(key).and_then(Value::as_str).unwrap_or_default();
    Action {
        dest: text("dest").to_string(),
        options: strings(object.get("options")),
        kind: match text("kind") {
            "append" => Kind::Append,
            "store_true" => Kind::StoreTrue,
            "help" => Kind::Help,
            _ => Kind::Store,
        },
        required: matches!(object.get("required"), Some(Value::Bool(true))),
        kind_of_value: match text("type") {
            "path" => Type::Path,
            "int" => Type::Int,
            "asset" => Type::Asset,
            "query" => Type::Query,
            _ => Type::Text,
        },
        choices: object
            .get("choices")
            .and_then(Value::as_array)
            .map(|items| strings(Some(&Value::Array(items.to_vec())))),
        default: object.get("default").cloned().unwrap_or(Value::Null),
    }
}

fn strings(value: Option<&Value>) -> Vec<String> {
    value
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_str)
        .map(str::to_string)
        .collect()
}

impl Action {
    fn name(&self) -> String {
        self.options.join("/")
    }
}

pub fn parse(contract: &Contract, args: &[String]) -> Outcome {
    let root_usage = |message: String| Outcome::Usage {
        usage: contract.usage.clone(),
        prog: contract.prog.clone(),
        message,
    };
    let mut root_extras = Vec::new();
    let mut index = 0;
    let mut after_double_dash = false;
    while index < args.len() {
        let arg = &args[index];
        if !after_double_dash && arg == "--" {
            after_double_dash = true;
            index += 1;
            continue;
        }
        if after_double_dash || !looks_optional(arg) {
            break;
        }
        let (prefix, explicit) = split_explicit(arg);
        if arg == "-h"
            || (prefix.len() > 2 && prefix.starts_with("--") && "--help".starts_with(prefix))
        {
            if let Some(value) = explicit {
                return root_usage(format!(
                    "argument -h/--help: ignored explicit argument {}",
                    py::repr(value)
                ));
            }
            return Outcome::Help(contract.help.clone());
        }
        if prefix == "-h"
            && let Some(value) = explicit
        {
            return root_usage(format!(
                "argument -h/--help: ignored explicit argument {}",
                py::repr(value)
            ));
        }
        if arg.starts_with("-h") && !arg.starts_with("--") {
            return Outcome::Help(contract.help.clone());
        }
        root_extras.push(arg.clone());
        index += 1;
    }
    let Some(name) = args.get(index) else {
        return root_usage("the following arguments are required: command".to_string());
    };
    let Some(command) = contract.command(name) else {
        let choices: Vec<String> = contract
            .commands
            .iter()
            .map(|command| py::repr(&command.name))
            .collect();
        return root_usage(format!(
            "argument command: invalid choice: {} (choose from {})",
            py::repr(name),
            choices.join(", ")
        ));
    };
    let usage = |message: String| Outcome::Usage {
        usage: command.usage.clone(),
        prog: command.prog.clone(),
        message,
    };
    match parse_command(command, &args[index + 1..]) {
        Err(Stop::Help) => Outcome::Help(command.help.clone()),
        Err(Stop::Error(message)) => usage(message),
        Ok((values, extras)) => {
            root_extras.extend(extras);
            if root_extras.is_empty() {
                Outcome::Run(Invocation {
                    command: command.name.clone(),
                    values,
                })
            } else {
                usage(format!("unrecognized arguments: {}", root_extras.join(" ")))
            }
        }
    }
}

enum Stop {
    Help,
    Error(String),
}

enum Token {
    Positional(String),
    DoubleDash,
    Known {
        action: usize,
        explicit: Option<String>,
        attached: bool,
    },
    Unknown(String),
}

fn looks_optional(arg: &str) -> bool {
    arg.len() > 1 && arg.starts_with('-') && !is_negative_number(arg) && !arg.contains(' ')
}

fn is_negative_number(arg: &str) -> bool {
    let Some(body) = arg.strip_prefix('-') else {
        return false;
    };
    let (whole, fraction) = body
        .split_once('.')
        .map_or((body, None), |(whole, fraction)| (whole, Some(fraction)));
    let digits = |text: &str| text.bytes().all(|byte| byte.is_ascii_digit());
    match fraction {
        None => !whole.is_empty() && digits(whole),
        Some(fraction) => !fraction.is_empty() && digits(whole) && digits(fraction),
    }
}

fn split_explicit(arg: &str) -> (&str, Option<&str>) {
    arg.split_once('=')
        .map_or((arg, None), |(prefix, explicit)| (prefix, Some(explicit)))
}

fn classify(command: &Command, arg: &str) -> Result<Token, Stop> {
    if arg == "--" {
        return Ok(Token::DoubleDash);
    }
    if !arg.starts_with('-') || arg.len() == 1 {
        return Ok(Token::Positional(arg.to_string()));
    }
    let known = |option: &str| {
        command
            .actions
            .iter()
            .position(|action| action.options.iter().any(|name| name == option))
    };
    if let Some(action) = known(arg) {
        return Ok(Token::Known {
            action,
            explicit: None,
            attached: false,
        });
    }
    let (prefix, explicit) = split_explicit(arg);
    if explicit.is_some()
        && let Some(action) = known(prefix)
    {
        return Ok(Token::Known {
            action,
            explicit: explicit.map(str::to_string),
            attached: false,
        });
    }
    if !arg.starts_with("--") && arg.starts_with("-h") {
        return Ok(Token::Known {
            action: known("-h").unwrap_or(0),
            explicit: Some(arg[2..].to_string()),
            attached: true,
        });
    }
    let matches: Vec<(usize, &str)> = command
        .actions
        .iter()
        .enumerate()
        .flat_map(|(index, action)| {
            action
                .options
                .iter()
                .map(move |option| (index, option.as_str()))
        })
        .filter(|(_, option)| arg.starts_with("--") && option.starts_with(prefix))
        .collect();
    match matches.as_slice() {
        [] => {}
        [(action, _)] => {
            return Ok(Token::Known {
                action: *action,
                explicit: explicit.map(str::to_string),
                attached: false,
            });
        }
        several => {
            let names: Vec<&str> = several.iter().map(|(_, option)| *option).collect();
            return Err(Stop::Error(format!(
                "ambiguous option: {arg} could match {}",
                names.join(", ")
            )));
        }
    }
    if is_negative_number(arg) || arg.contains(' ') {
        return Ok(Token::Positional(arg.to_string()));
    }
    Ok(Token::Unknown(arg.to_string()))
}

fn parse_command(
    command: &Command,
    args: &[String],
) -> Result<(HashMap<String, Arg>, Vec<String>), Stop> {
    let mut tokens = Vec::with_capacity(args.len());
    let mut after_double_dash = false;
    for arg in args {
        if after_double_dash {
            tokens.push(Token::Positional(arg.clone()));
            continue;
        }
        let token = classify(command, arg)?;
        after_double_dash = matches!(token, Token::DoubleDash);
        tokens.push(token);
    }
    let mut values: HashMap<String, Arg> = HashMap::new();
    let mut seen: HashSet<usize> = HashSet::new();
    let mut extras = Vec::new();
    let mut index = 0;
    while index < tokens.len() {
        match &tokens[index] {
            Token::Positional(text) | Token::Unknown(text) => extras.push(text.clone()),
            Token::DoubleDash => {}
            Token::Known {
                action,
                explicit,
                attached,
            } => {
                let spec = &command.actions[*action];
                let value = match spec.kind {
                    Kind::Help => return help_or_refusal(spec, explicit.as_deref(), *attached),
                    Kind::StoreTrue => {
                        if let Some(explicit) = explicit {
                            return Err(refusal(
                                spec,
                                &format!("ignored explicit argument {}", py::repr(explicit)),
                            ));
                        }
                        Arg::Flag(true)
                    }
                    Kind::Store | Kind::Append => {
                        let given = match explicit {
                            Some(explicit) => explicit.clone(),
                            None => match tokens.get(index + 1) {
                                Some(Token::Positional(next)) => {
                                    index += 1;
                                    next.clone()
                                }
                                _ => return Err(refusal(spec, "expected one argument")),
                            },
                        };
                        converted(spec, &given)?
                    }
                };
                conflict(command, spec, &seen)?;
                seen.insert(*action);
                store(&mut values, spec, value);
            }
        }
        index += 1;
    }
    finish(command, values, &seen).map(|values| (values, extras))
}

fn help_or_refusal(
    spec: &Action,
    explicit: Option<&str>,
    attached: bool,
) -> Result<(HashMap<String, Arg>, Vec<String>), Stop> {
    match explicit {
        None => Err(Stop::Help),
        Some(_) if attached => Err(Stop::Help),
        Some(rest) => Err(refusal(
            spec,
            &format!("ignored explicit argument {}", py::repr(rest)),
        )),
    }
}

fn refusal(spec: &Action, message: &str) -> Stop {
    Stop::Error(format!("argument {}: {message}", spec.name()))
}

fn converted(spec: &Action, given: &str) -> Result<Arg, Stop> {
    let value = match spec.kind_of_value {
        Type::Int => match python_int(given) {
            Some(number) => return checked(spec, Arg::Int(number), &number.to_string()),
            None => {
                return Err(refusal(
                    spec,
                    &format!("invalid int value: {}", py::repr(given)),
                ));
            }
        },
        Type::Query if py::strip(given).is_empty() => {
            return Err(refusal(spec, "the query is empty"));
        }
        Type::Asset => singular_to_plural(spec, given),
        Type::Text | Type::Path | Type::Query => given.to_string(),
    };
    checked(spec, Arg::Text(value.clone()), &value)
}

fn checked(spec: &Action, value: Arg, shown: &str) -> Result<Arg, Stop> {
    let Some(choices) = &spec.choices else {
        return Ok(value);
    };
    if choices.iter().any(|choice| choice == shown) {
        return Ok(value);
    }
    let listed: Vec<String> = choices.iter().map(|choice| py::repr(choice)).collect();
    Err(refusal(
        spec,
        &format!(
            "invalid choice: {} (choose from {})",
            py::repr(shown),
            listed.join(", ")
        ),
    ))
}

fn singular_to_plural(spec: &Action, given: &str) -> String {
    spec.choices
        .iter()
        .flatten()
        .find(|asset| {
            let singular = match asset.strip_suffix("ies") {
                Some(stem) => format!("{stem}y"),
                None => asset.strip_suffix('s').unwrap_or(asset).to_string(),
            };
            singular == given
        })
        .cloned()
        .unwrap_or_else(|| given.to_string())
}

fn python_int(given: &str) -> Option<i64> {
    let text = py::strip(given);
    let (sign, digits) = match text.as_bytes().first() {
        Some(b'-') => (-1, &text[1..]),
        Some(b'+') => (1, &text[1..]),
        _ => (1, text),
    };
    let valid = !digits.is_empty()
        && !digits.starts_with('_')
        && !digits.ends_with('_')
        && !digits.contains("__")
        && digits
            .bytes()
            .all(|byte| byte.is_ascii_digit() || byte == b'_');
    if !valid {
        return None;
    }
    digits
        .replace('_', "")
        .parse::<i64>()
        .ok()
        .map(|number| sign * number)
}

fn conflict(command: &Command, spec: &Action, seen: &HashSet<usize>) -> Result<(), Stop> {
    for group in command
        .mutex
        .iter()
        .filter(|group| group.dests.contains(&spec.dest))
    {
        let earlier = command.actions.iter().enumerate().find(|(index, other)| {
            seen.contains(index) && other.dest != spec.dest && group.dests.contains(&other.dest)
        });
        if let Some((_, other)) = earlier {
            return Err(refusal(
                spec,
                &format!("not allowed with argument {}", other.name()),
            ));
        }
    }
    Ok(())
}

fn store(values: &mut HashMap<String, Arg>, spec: &Action, value: Arg) {
    if spec.kind == Kind::Append {
        let Arg::Text(item) = value else { return };
        match values
            .entry(spec.dest.clone())
            .or_insert_with(|| Arg::List(Vec::new()))
        {
            Arg::List(items) => items.push(item),
            slot => *slot = Arg::List(vec![item]),
        }
    } else {
        values.insert(spec.dest.clone(), value);
    }
}

fn finish(
    command: &Command,
    mut values: HashMap<String, Arg>,
    seen: &HashSet<usize>,
) -> Result<HashMap<String, Arg>, Stop> {
    let missing: Vec<String> = command
        .actions
        .iter()
        .enumerate()
        .filter(|(index, action)| action.required && !seen.contains(index))
        .map(|(_, action)| action.name())
        .collect();
    if !missing.is_empty() {
        return Err(Stop::Error(format!(
            "the following arguments are required: {}",
            missing.join(", ")
        )));
    }
    for group in command.mutex.iter().filter(|group| group.required) {
        let chosen = command
            .actions
            .iter()
            .enumerate()
            .any(|(index, action)| seen.contains(&index) && group.dests.contains(&action.dest));
        if !chosen {
            let names: Vec<String> = command
                .actions
                .iter()
                .filter(|action| group.dests.contains(&action.dest))
                .map(Action::name)
                .collect();
            return Err(Stop::Error(format!(
                "one of the arguments {} is required",
                names.join(" ")
            )));
        }
    }
    for action in command
        .actions
        .iter()
        .filter(|action| action.kind != Kind::Help)
    {
        values
            .entry(action.dest.clone())
            .or_insert_with(|| default_of(action));
    }
    Ok(values)
}

fn default_of(action: &Action) -> Arg {
    match (&action.default, action.kind) {
        (Value::Bool(flag), _) => Arg::Flag(*flag),
        (Value::Str(text), _) => Arg::Text(text.clone()),
        (Value::Int(digits), _) => digits.parse().map_or(Arg::Absent, Arg::Int),
        (Value::Array(items), _) => Arg::List(
            items
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_string)
                .collect(),
        ),
        (_, Kind::StoreTrue) => Arg::Flag(false),
        _ => Arg::Absent,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn words(line: &str) -> Vec<String> {
        line.split_whitespace().map(str::to_string).collect()
    }

    fn refused(line: &str) -> String {
        match parse(&Contract::embedded(), &words(line)) {
            Outcome::Usage { prog, message, .. } => format!("{prog}: error: {message}"),
            Outcome::Help(_) => "help".to_string(),
            Outcome::Run(_) => "ran".to_string(),
        }
    }

    #[test]
    fn an_abbreviation_shared_by_several_options_is_ambiguous() {
        assert_eq!(
            refused("validate --re ."),
            "registry_tools.py validate: error: ambiguous option: --re could match --registry-root, --repo-root, --require-reviewed"
        );
    }

    #[test]
    fn a_unique_abbreviation_with_an_explicit_value_selects_its_option() {
        let Outcome::Run(invocation) = parse(
            &Contract::embedded(),
            &words("list-proposals --changes-r=docs"),
        ) else {
            panic!("expected a run");
        };
        assert_eq!(invocation.text("changes_root"), Some("docs"));
    }

    #[test]
    fn a_negative_number_is_a_value_not_an_option() {
        assert_eq!(refused("change-coupling --repo-root . --depth -1"), "ran");
    }

    #[test]
    fn python_int_accepts_signs_spaces_and_single_underscores_only() {
        assert_eq!(python_int(" +1_000 "), Some(1000));
        assert_eq!(python_int("-7"), Some(-7));
        for bad in ["", "1__0", "_1", "1_", "x", "1.0"] {
            assert_eq!(python_int(bad), None, "{bad}");
        }
    }

    #[test]
    fn a_required_mutually_exclusive_group_names_every_member() {
        assert_eq!(
            refused("verify-git-governance --registry-root r --repo-root r"),
            "registry_tools.py verify-git-governance: error: one of the arguments --commit --stdin is required"
        );
    }

    #[test]
    fn a_second_member_of_a_mutually_exclusive_group_is_refused() {
        assert_eq!(
            refused("verify-git-governance --registry-root r --repo-root r --commit c --stdin"),
            "registry_tools.py verify-git-governance: error: argument --stdin: not allowed with argument --commit"
        );
    }
}
