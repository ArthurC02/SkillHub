mod cli;
mod difflib;
mod digits;
mod files;
mod git;
mod json;
mod okf;
mod proposals;
mod py;
mod pyvalue;
mod records;
mod store;
mod tar;

use std::io::Write;

use cli::{Contract, Invocation, Outcome};
use json::{Object, Style, Value};

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
        other => Err(format!("{other} is not available in the Rust engine yet")),
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
