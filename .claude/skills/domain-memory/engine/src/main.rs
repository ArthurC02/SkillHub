mod attestations;
mod audit_append;
mod audit_commands;
mod audit_log;
mod candidate_evidence;
mod cite;
mod cli;
mod compare;
mod context;
mod coupling;
mod coverage;
mod crossrefs;
mod dependents;
mod difflib;
mod digits;
mod evidence;
mod files;
mod fnmatch;
mod git;
mod git_governance;
mod indexes;
mod json;
mod listing;
mod okf;
mod okf_write;
mod policy;
mod promotion;
mod proposals;
mod py;
mod pyre;
mod pyregex;
mod pyvalue;
mod read_commands;
mod readiness;
mod reconciliation;
mod records;
mod registry_check;
mod registry_write;
mod revision;
mod search;
mod secret_patterns;
mod secrets;
mod sha256;
mod source_probe;
mod sources;
mod store;
mod tar;
mod transaction;
mod unicode;
mod updates;
mod validate;
mod write_commands;
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
        "coverage" => {
            let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
            let changes_root = match invocation.text("changes_root") {
                Some(given) => Some(py::resolve(given)),
                None => Some(root.join("changes")).filter(|beside| beside.is_dir()),
            };
            let report = coverage::coverage(&root, changes_root)?;
            print_json(&report.coverage);
            if let Some(beside) = report.missing_changes {
                emit_stderr(&format!(
                    "NOTE: no Change Packages at {}; pass --changes-root to count the rules a test proves.\n",
                    py::shown(&beside)
                ));
            }
            Ok(0)
        }
        "analyze-boundary" => {
            let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
            print_json(&context::boundary_analysis(
                &root,
                invocation.text("source_context").unwrap_or_default(),
                invocation.text("target_context").unwrap_or_default(),
            )?);
            Ok(0)
        }
        "dependents" => {
            let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
            print_json(&dependents::dependents(
                &root,
                invocation.text("asset").unwrap_or_default(),
                invocation.text("id").unwrap_or_default(),
            )?);
            Ok(0)
        }
        "verify-evidence" => verify_evidence(invocation),
        "cite" => {
            let cited = cite::cite(
                &py::resolve(invocation.text("repo_root").unwrap_or(".")),
                invocation.text("path").unwrap_or_default(),
                invocation.text("lines"),
                invocation.text("function"),
                invocation.text("registry_root").map(py::resolve).as_deref(),
                invocation.flag("show"),
            )?;
            print_json(&Value::Object(cited.reference));
            for note in cited.notes {
                emit_stderr(&format!("{note}\n"));
            }
            Ok(0)
        }
        "scan-secrets" => scan_secrets(invocation),
        "discover-sources" => discover_sources(invocation),
        "verify-sources" => verify_sources(invocation),
        "verify-audit" => audit_commands::verify_audit(invocation),
        "audit-attestations" => audit_commands::audit_attestations(invocation),
        "verify-scm-attestation" => audit_commands::verify_scm_attestation(invocation),
        "verify-git-governance" => audit_commands::verify_git_governance(invocation),
        "governance-readiness" => audit_commands::governance_readiness(invocation),
        "validate" => validate_command(invocation),
        "refresh-indexes" => refresh_indexes_command(invocation),
        "change-coupling" => read_commands::change_coupling(invocation),
        "compare-registry" => read_commands::compare_registry(invocation),
        "probe" => read_commands::probe(invocation),
        "readiness" => read_commands::readiness(invocation),
        "upsert-candidate" => write_commands::upsert(invocation),
        "retract-candidate" => write_commands::retract(invocation),
        "promote-candidate" => write_commands::promote(invocation),
        "demote-local-reviews" => write_commands::demote(invocation),
        "recover-registry-update" => write_commands::recover(invocation),
        other => Err(format!("{other} is not available in the Rust engine yet")),
    }
}

fn scan_secrets(invocation: &Invocation) -> Result<i32, String> {
    let root = py::resolve(invocation.text("repo_root").unwrap_or("."));
    let (report, code) = secrets::scan_report(&root, secrets::LIMITS)?;
    let style = Style {
        indent: None,
        ..Style::PRINTED
    };
    emit_stdout(&format!("{}\n", json::dumps(&Value::Object(report), style)));
    Ok(code)
}

fn discover_sources(invocation: &Invocation) -> Result<i32, String> {
    let mut given = Object::new();
    for (flag, key) in [
        ("implementation_root", "implementation_roots"),
        ("excluded_directory", "excluded_directories"),
        ("decision_root", "decision_roots"),
        ("boundary_marker", "boundary_markers"),
    ] {
        let values = invocation.texts(flag);
        if !values.is_empty() {
            let items = values.iter().map(|item| Value::Str(item.clone())).collect();
            given.insert(key, Value::Array(items));
        }
    }
    let given = Value::Object(given);
    let errors = policy::profile_errors(Some(&given));
    if !errors.is_empty() {
        return Err(format!("invalid project profile: {}", errors.join("; ")));
    }
    let root = py::resolve(invocation.text("repo_root").unwrap_or("."));
    let source_map = Value::Object(sources::discover_sources(
        &root,
        &sources::Profile::from_section(Some(&given)),
    )?);
    if let Some(output) = invocation.text("output") {
        files::write_source_file(&py::resolve(output), &source_map)?;
    }
    print_json(&source_map);
    Ok(0)
}

fn verify_sources(invocation: &Invocation) -> Result<i32, String> {
    let policy = match invocation.text("policy") {
        Some(given) => Some(files::load_json(&py::resolve(given))?),
        None => None,
    };
    let repo_given = invocation.text("repo_root").unwrap_or(".");
    let map_given = invocation.text("source_map").unwrap_or(".");
    let result = sources::verify_against_discovery(
        &py::resolve(repo_given),
        &py::resolve(map_given),
        policy.as_ref(),
    )?;
    print_json(&Value::Object(result.clone()));
    if let Some(note) = sources::discovery_drift_note(
        &result,
        &py::PurePath::new(map_given).parent(),
        &py::PurePath::new(repo_given),
    ) {
        emit_stderr(&format!("{note}\n"));
    }
    let status = result.get("status").and_then(Value::as_str);
    Ok(if matches!(status, Some("current" | "discovery_drifted")) {
        0
    } else {
        1
    })
}

fn validate_command(invocation: &Invocation) -> Result<i32, String> {
    let registry_root = py::resolve(invocation.text("registry_root").unwrap_or("."));
    let mut repository = invocation
        .text("repo_root")
        .map(|given| listing::Repository::new(&py::resolve(given)));
    let errors = validate::validate(
        &registry_root,
        repository.as_mut(),
        invocation.flag("require_reviewed"),
    )?;
    let code = if errors.is_empty() {
        emit_stdout("Registry is valid.\n");
        0
    } else {
        let lines: Vec<String> = errors
            .iter()
            .map(|error| format!("ERROR: {error}"))
            .collect();
        emit_stderr(&format!("{}\n", lines.join("\n")));
        1
    };
    let mut notes = validate::unsettled_notes(&registry_root)?;
    if code == 0 {
        notes.extend(validate::consistency_notes(&registry_root)?);
        if let Some(repository) = repository.as_mut() {
            notes.extend(validate::unplaced_note(&registry_root, repository)?);
        }
    }
    for note in notes {
        emit_stderr(&format!("{note}\n"));
    }
    Ok(code)
}

fn refresh_indexes_command(invocation: &Invocation) -> Result<i32, String> {
    let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
    let refreshed = indexes::refresh_indexes(&root)?
        .iter()
        .map(|path| Value::Str(store::shown_path(path, Some(&root))))
        .collect();
    let mut reply = Object::new();
    reply.insert("refreshed", Value::Array(refreshed));
    let style = Style {
        indent: None,
        ..Style::PRINTED
    };
    emit_stdout(&format!("{}\n", json::dumps(&Value::Object(reply), style)));
    Ok(0)
}

fn verify_evidence(invocation: &Invocation) -> Result<i32, String> {
    let repo_root = py::resolve(invocation.text("repo_root").unwrap_or("."));
    let staged = invocation.flag("staged");
    let overlay = match invocation.text("package_root") {
        Some(package_root) => evidence::package_overlay(&py::resolve(package_root))?,
        None => Vec::new(),
    };
    let mut verifier = evidence::Verifier::new(&repo_root, staged);
    let report = evidence::verify_evidence(
        &py::resolve(invocation.text("registry_root").unwrap_or(".")),
        &mut verifier,
        invocation.texts("path"),
        &overlay,
    )?;
    print_json(&Value::Object(report.clone()));
    let mut failing: Vec<&str> = evidence::FAILING.to_vec();
    if invocation.flag("strict") {
        failing.push("shifted");
    }
    match evidence::failure_message(&report, &failing, staged) {
        Some(message) => {
            emit_stderr(&format!("{message}\n"));
            Ok(1)
        }
        None => Ok(0),
    }
}

fn resolve_terms(invocation: &Invocation) -> Result<i32, String> {
    let root = py::resolve(invocation.text("registry_root").unwrap_or("."));
    let with_mentions = invocation.flag("with_mentions");
    let limit = policy::query_result_limit(&root)?;
    let registry = store::Registry::open(&root)?;
    let mut found: Vec<(&str, Vec<Object>)> = Vec::new();
    for query in invocation.texts("query") {
        let matches = search::every_resolved_term(
            &registry,
            query,
            invocation.text("context"),
            with_mentions,
        )?;
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
            search::prose_mentions(&registry, query)?
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
