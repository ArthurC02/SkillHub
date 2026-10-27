mod cli;
mod context;
mod difflib;
mod digits;
mod files;
mod git;
mod json;
mod okf;
mod policy;
mod proposals;
mod py;
mod pyvalue;
mod records;
mod search;
mod store;
mod tar;
mod unicode;
#[rustfmt::skip]
mod unicode_data;

use std::io::Write;

use cli::{Contract, Invocation, Outcome};
use json::{Object, Style, Value};
use pyvalue::str_of;

const COUNTERFACTUAL_FORMAT: &str = "domain-memory-counterfactual/v1";
const COUNTERFACTUAL_USAGE_EXIT: i32 = 3;

fn main() {
    let args: Vec<String> = std::env::args_os()
        .skip(1)
        .map(|arg| arg.to_string_lossy().into_owned())
        .collect();
    std::process::exit(run(&args));
}

fn run(args: &[String]) -> i32 {
    let contract = Contract::embedded();
    match cli::parse(&contract, args) {
        Outcome::Help(text) => {
            emit_stdout(&text);
            0
        }
        Outcome::Usage {
            usage,
            prog,
            message,
        } => refuse_usage(args, &usage, &format!("{prog}: error: {message}")),
        Outcome::Run(invocation) => match execute(&invocation) {
            Ok(code) => code,
            Err(error) => {
                emit_stderr(&format!("ERROR: {error}\n"));
                1
            }
        },
    }
}

fn refuse_usage(args: &[String], usage: &str, error: &str) -> i32 {
    if args.first().map(String::as_str) == Some("counterfactual") {
        let mut reply = Object::new();
        reply.insert("format", Value::Str(COUNTERFACTUAL_FORMAT.to_string()));
        reply.insert("verdict", Value::Str("error".to_string()));
        reply.insert("reason", Value::Str(error.to_string()));
        print_json(&Value::Object(reply));
        return COUNTERFACTUAL_USAGE_EXIT;
    }
    emit_stderr(&format!("{usage}{error}\n"));
    2
}

fn execute(invocation: &Invocation) -> Result<i32, String> {
    match invocation.command.as_str() {
        "list-proposals" => {
            let root = py::resolve(invocation.text("changes_root").unwrap_or("."));
            print_json(&Value::Array(proposals::list_proposals(&root)?));
            Ok(0)
        }
        "export" => {
            let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
            let documents = match invocation.text("commit") {
                None => store::read_documents(&root)?,
                Some(commit) => {
                    let repo_root = invocation
                        .text("repo_root")
                        .ok_or("export --commit needs --repo-root")?;
                    store::committed_documents(&py::resolve(repo_root), commit, &root)?
                        .ok_or_else(|| format!("no Domain Memory Registry at {commit}"))?
                }
            };
            print_json(&Value::Object(documents));
            Ok(0)
        }
        "get-record" => {
            let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
            let reply = records::get_record(
                &root,
                invocation.text("asset").unwrap_or_default(),
                invocation.text("id").unwrap_or_default(),
                invocation.text("changes_root"),
            )?;
            for note in &reply.notes {
                emit_stderr(&format!("{note}\n"));
            }
            print_json(&reply.record);
            Ok(0)
        }
        "get-context" => {
            let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
            print_json(&context::get_context(
                &root,
                invocation.text("id").unwrap_or_default(),
                invocation.flag("summary"),
            )?);
            Ok(0)
        }
        "validate-policy" => {
            let path = py::resolve(invocation.text("policy").unwrap_or("."));
            let errors = policy::validate_policy(&files::load_json(&path)?);
            if errors.is_empty() {
                emit_stdout("Domain Memory policy is valid.\n");
                return Ok(0);
            }
            let lines: Vec<String> = errors
                .iter()
                .map(|error| format!("ERROR: {error}"))
                .collect();
            emit_stderr(&format!("{}\n", lines.join("\n")));
            Ok(1)
        }
        "lookup" => {
            let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
            let asset = invocation.text("asset").unwrap_or_default();
            let query = invocation.text("query").unwrap_or_default();
            let (found, how) = search::ranked_lookup(&root, asset, query)?;
            print_json(&Value::Array(found));
            if let Some(note) = lookup_note(asset, query, &how) {
                emit_stderr(&format!("NOTE: {note}\n"));
            }
            Ok(0)
        }
        "resolve-terms" => resolve_terms(invocation),
        other => Err(format!("{other} is not available in the Rust engine yet")),
    }
}

fn resolve_terms(invocation: &Invocation) -> Result<i32, String> {
    let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
    let with_mentions = invocation.flag("with_mentions");
    let limit = policy::query_result_limit(&root)?;
    let mut found: Vec<(&str, Vec<Object>)> = Vec::new();
    for query in invocation.texts("query") {
        let matches =
            search::every_resolved_term(&root, query, invocation.text("context"), with_mentions)?;
        match found.iter_mut().find(|(seen, _)| seen == query) {
            Some(slot) => slot.1 = matches,
            None => found.push((query, matches)),
        }
    }
    let shown = found
        .iter()
        .flat_map(|(query, matches)| {
            matches.iter().take(limit).map(|matched| {
                let mut matched = matched.clone();
                matched.insert("query", Value::Str((*query).to_string()));
                Value::Object(matched)
            })
        })
        .collect();
    print_json(&Value::Array(shown));
    for (query, matches) in &found {
        if matches.len() > limit {
            emit_stderr(&format!(
                "NOTE: \"{query}\" has {} matches; the first {limit} are shown, rules and contracts first among mentions.\n",
                matches.len()
            ));
        }
        let constraints: Vec<Object> = if with_mentions {
            Vec::new()
        } else {
            search::prose_mentions(&root, query)?
                .into_iter()
                .filter(|mention| {
                    mention
                        .get("asset")
                        .and_then(Value::as_str)
                        .is_some_and(|asset| search::CONSTRAINT_ASSETS.contains(&asset))
                })
                .collect()
        };
        let resolved = matches.iter().any(|matched| {
            matched
                .get("match_kind")
                .and_then(Value::as_str)
                .is_some_and(|kind| search::MATCH_KINDS.contains(&kind))
        });
        if resolved && !constraints.is_empty() {
            let mut distinct: Vec<(String, String)> = Vec::new();
            for mention in &constraints {
                let key = (
                    mention.get("asset").map(str_of).unwrap_or_default(),
                    mention.get("id").map(str_of).unwrap_or_default(),
                );
                if !distinct.contains(&key) {
                    distinct.push(key);
                }
            }
            emit_stderr(&format!(
                "NOTE: {} reviewed rules or contracts mention \"{query}\"; pass --with-mentions to list them.\n",
                distinct.len()
            ));
        }
    }
    Ok(0)
}

fn lookup_note(asset: &str, query: &str, how: &search::Found) -> Option<String> {
    const NOT_PROOF: &str = "That is not proof that no constraint applies: read get-context for the Context you are changing, or look up another asset.";
    match how {
        search::Found::SomeWords => Some(format!(
            "no {asset} record holds every word of \"{query}\"; these hold some of them, most first."
        )),
        search::Found::Nothing => Some(format!(
            "no {asset} record mentions \"{query}\" or any of its words. {NOT_PROOF}"
        )),
        search::Found::CommonWords => Some(format!(
            "the {asset} records share with \"{query}\" only words many of them hold, so none is listed. {NOT_PROOF}"
        )),
        search::Found::Phrase | search::Found::AllWords => None,
    }
}

fn print_json(value: &Value) {
    emit_stdout(&format!("{}\n", json::dumps(value, Style::PRINTED)));
}

fn emit_stdout(text: &str) {
    let mut out = std::io::stdout().lock();
    let _ = out.write_all(text.as_bytes());
    let _ = out.flush();
}

fn emit_stderr(text: &str) {
    let mut err = std::io::stderr().lock();
    let _ = err.write_all(text.as_bytes());
    let _ = err.flush();
}
