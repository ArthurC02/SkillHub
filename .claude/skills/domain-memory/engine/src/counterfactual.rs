use std::collections::HashMap;
use std::fs;
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::thread;
use std::time::{Duration, Instant, SystemTime};

use crate::files::{load_json, write_json};
use crate::git::spawn_failure;
use crate::json::{Object, Value};
use crate::listing::EXCLUDED_DIRECTORIES;
use crate::py::{self, PurePath};
use crate::pyregex;
use crate::pyvalue::{equal, str_of};
use crate::sha256::hex_digest;
use crate::shlex;
use crate::test_output::{build_failed, failing_tests, failure_evidence, no_test_passed};
use crate::unicode::casefold;

pub const FORMAT: &str = "domain-memory-counterfactual/v1";
const SUITE_FORMAT: &str = "domain-memory-counterfactual-suite/v1";
const VERDICTS: [&str; 4] = ["killed", "survived", "inconclusive", "error"];
const SIDECAR_SUFFIX: &str = ".counterfactual-original";
const DEFAULT_TIMEOUT_SECONDS: i64 = 600;
const MTIME_STEP_SECONDS: u64 = 2;
const RESTORE_DELAYS_MILLIS: [u64; 6] = [100, 250, 500, 1000, 2000, 4000];
const OBLIGATION_PLACEHOLDER: &str = "<obligation-id>";
const LISTED_LINES: usize = 10;
const PROPOSAL_FILE: &str = "domain-change-proposal.json";
const OBLIGATIONS_FILE: &str = "test-obligations.json";
const EVIDENCE_FILE: &str = "evidence-bundle.json";
const RECORDABLE_STATUSES: [&str; 2] = ["draft", "submitted"];
const POLL: Duration = Duration::from_millis(5);
const VERDICT_WORDS: [&str; 8] = [
    "fail", "failed", "failure", "pass", "passed", "killed", "survived", "error",
];

pub struct Refusal {
    pub reason: String,
    pub details: Object,
}

impl From<String> for Refusal {
    fn from(reason: String) -> Self {
        Self {
            reason,
            details: Object::new(),
        }
    }
}

type Outcome<T> = Result<T, Refusal>;

pub fn exit_code(verdict: &str) -> i32 {
    match verdict {
        "killed" => 0,
        "survived" => 1,
        "inconclusive" => 2,
        _ => 3,
    }
}

pub struct Mutation {
    pub file: String,
    pub find: String,
    pub replace: String,
}

pub struct Proof {
    pub test_command: String,
    pub timeout: i64,
    pub cwd: Option<String>,
    pub expect: Option<String>,
    pub obligation_id: Option<String>,
}

#[derive(Clone)]
struct TestRun {
    exit_code: Option<i64>,
    output: String,
}

impl TestRun {
    fn passed(&self) -> bool {
        self.exit_code == Some(0)
    }

    fn timed_out(&self) -> bool {
        self.exit_code.is_none()
    }
}

type IntactRuns = HashMap<(String, PathBuf, i64), TestRun>;

fn native(given: &str) -> String {
    PurePath::new(given).native()
}

fn decoded(output: Vec<u8>) -> String {
    match String::from_utf8(output) {
        Ok(text) => text,
        Err(error) => String::from_utf8_lossy(error.as_bytes()).into_owned(),
    }
}

fn inside(root: &Path, path: &str) -> Option<PathBuf> {
    let top = py::resolve_path(root);
    let located = py::resolve_path(&top.join(path));
    py::relative_to(&located, &top).ok().map(|_| located)
}

fn working_directory(root: &Path, cwd: Option<&str>) -> Outcome<PathBuf> {
    let Some(cwd) = cwd else {
        return Ok(root.to_path_buf());
    };
    match inside(root, cwd) {
        Some(directory) if directory.is_dir() => Ok(directory),
        _ => Err(format!("--cwd must name a directory inside the repository: {cwd}").into()),
    }
}

fn target_inside(root: &Path, file: &str) -> Outcome<PathBuf> {
    let target = inside(root, file)
        .ok_or_else(|| format!("file is outside the repository: {}", native(file)))?;
    if !target.is_file() {
        return Err(format!("file does not exist: {}", native(file)).into());
    }
    Ok(target)
}

fn sidecar_of(target: &Path) -> PathBuf {
    let name = target
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_default();
    target.with_file_name(format!("{name}{SIDECAR_SUFFIX}"))
}

fn is_directory(entry: &fs::DirEntry) -> bool {
    match entry.file_type() {
        Ok(kind) if kind.is_symlink() => fs::metadata(entry.path()).is_ok_and(|meta| meta.is_dir()),
        Ok(kind) => kind.is_dir(),
        Err(_) => false,
    }
}

fn walk_sidecars(directory: &Path, prefix: &str, found: &mut Vec<String>) {
    let Ok(entries) = fs::read_dir(directory) else {
        return;
    };
    let mut children = Vec::new();
    for entry in entries.flatten() {
        let name = entry.file_name().to_string_lossy().into_owned();
        if !is_directory(&entry) {
            if name.ends_with(SIDECAR_SUFFIX) {
                found.push(format!("{prefix}{name}"));
            }
        } else if !EXCLUDED_DIRECTORIES.contains(&name.as_str())
            && !entry.file_type().is_ok_and(|kind| kind.is_symlink())
        {
            children.push((entry.path(), format!("{prefix}{name}/")));
        }
    }
    for (path, child_prefix) in children {
        walk_sidecars(&path, &child_prefix, found);
    }
}

fn refuse_leftovers(root: &Path) -> Outcome<()> {
    let mut leftovers = Vec::new();
    walk_sidecars(root, "", &mut leftovers);
    leftovers.sort();
    if leftovers.is_empty() {
        return Ok(());
    }
    Err(format!(
        "an earlier run left {}; each holds the original content of the file beside it, so restore from it and remove it before proving anything",
        leftovers.join(", ")
    )
    .into())
}

fn occurrences(original: &[u8], find: &[u8]) -> Vec<usize> {
    let mut found = Vec::new();
    let mut from = 0;
    while from + find.len() <= original.len() {
        match original[from..]
            .windows(find.len())
            .position(|window| window == find)
        {
            Some(at) => {
                found.push(from + at);
                from += at + find.len();
            }
            None => break,
        }
    }
    found
}

fn listed(lines: &[usize]) -> String {
    let shown: Vec<String> = lines
        .iter()
        .take(LISTED_LINES)
        .map(usize::to_string)
        .collect();
    if lines.len() > LISTED_LINES {
        format!(
            "{} and {} more",
            shown.join(", "),
            lines.len() - LISTED_LINES
        )
    } else {
        shown.join(", ")
    }
}

fn line_at(original: &[u8], offset: usize) -> usize {
    original[..offset]
        .iter()
        .filter(|byte| **byte == b'\n')
        .count()
        + 1
}

fn mutated(original: &[u8], mutation: &Mutation) -> Outcome<Vec<u8>> {
    let (find, replace) = (mutation.find.as_bytes(), mutation.replace.as_bytes());
    if find.is_empty() {
        return Err("the text to break must not be empty".to_string().into());
    }
    let found = occurrences(original, find);
    let file = native(&mutation.file);
    if found.is_empty() {
        return Err(
            format!("the text to break must occur once in {file}, and it occurs 0 times").into(),
        );
    }
    if found.len() > 1 {
        let mut lines: Vec<usize> = found.iter().map(|at| line_at(original, *at)).collect();
        lines.dedup();
        return Err(format!(
            "the text to break must occur once in {file}, and it occurs {} times, at lines {}; widen --find to text that occurs once",
            found.len(),
            listed(&lines)
        )
        .into());
    }
    if find == replace {
        return Err("the replacement is the text it replaces".to_string().into());
    }
    let mut broken = original[..found[0]].to_vec();
    broken.extend_from_slice(replace);
    broken.extend_from_slice(&original[found[0] + find.len()..]);
    Ok(broken)
}

fn write_as_newer(target: &Path, content: &[u8], seconds_ahead: u64) -> Result<(), String> {
    let modified = fs::metadata(target)
        .and_then(|meta| meta.modified())
        .map_err(|error| py::os_error(&error, target))?;
    let newest = modified.max(SystemTime::now()) + Duration::from_secs(seconds_ahead);
    fs::write(target, content).map_err(|error| py::open_error(&error, target))?;
    fs::OpenOptions::new()
        .write(true)
        .open(target)
        .and_then(|file| file.set_modified(newest))
        .map_err(|error| py::os_error(&error, target))
}

fn restore(target: &Path, original: &[u8], sidecar: &Path) -> Outcome<()> {
    for delay in RESTORE_DELAYS_MILLIS.iter().map(Some).chain([None]) {
        match write_as_newer(target, original, MTIME_STEP_SECONDS) {
            Ok(()) => break,
            Err(error) => match delay {
                None => {
                    return Err(format!(
                        "could not restore {}: {error}. It still holds the mutation; copy {} over it, then delete {}",
                        py::shown(target),
                        py::shown(sidecar),
                        sidecar
                            .file_name()
                            .map(|name| name.to_string_lossy().into_owned())
                            .unwrap_or_default()
                    )
                    .into());
                }
                Some(millis) => thread::sleep(Duration::from_millis(*millis)),
            },
        }
    }
    fs::remove_file(sidecar).map_err(|error| py::os_error(&error, sidecar))?;
    Ok(())
}

fn start_failure(command: &str, error: &str) -> String {
    format!(
        "{command} could not start: {error}. The command runs as written, with no shell, so give the full invocation and run a pipeline or several commands from a script"
    )
}

fn drained(pipe: Option<impl Read + Send + 'static>) -> thread::JoinHandle<Vec<u8>> {
    thread::spawn(move || {
        let mut bytes = Vec::new();
        if let Some(mut pipe) = pipe {
            let _ = pipe.read_to_end(&mut bytes);
        }
        bytes
    })
}

fn run_tests(directory: &Path, command: &str, timeout: i64) -> Outcome<TestRun> {
    let argv = shlex::split(command)?;
    let Some(program) = argv.first() else {
        let error = if cfg!(windows) {
            "[WinError 87] The parameter is incorrect"
        } else {
            "list index out of range"
        };
        return Err(start_failure(command, error).into());
    };
    let mut child = Command::new(program)
        .args(&argv[1..])
        .current_dir(directory)
        .stdin(Stdio::inherit())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|error| start_failure(command, &spawn_failure(&error, program)))?;
    let stdout = drained(child.stdout.take());
    let stderr = drained(child.stderr.take());
    let seconds = u64::try_from(timeout).unwrap_or(0);
    let deadline = Instant::now().checked_add(Duration::from_secs(seconds));
    let status = loop {
        if let Some(status) = child
            .try_wait()
            .map_err(|error| py::os_error(&error, directory))?
        {
            break status;
        }
        if deadline.is_some_and(|limit| Instant::now() >= limit) {
            let _ = child.kill();
            let _ = child.wait();
            return Ok(TestRun {
                exit_code: None,
                output: format!("no result within {timeout} seconds"),
            });
        }
        thread::sleep(POLL);
    };
    let mut bytes = stdout.join().unwrap_or_default();
    bytes.extend(stderr.join().unwrap_or_default());
    Ok(TestRun {
        exit_code: Some(i64::from(status.code().unwrap_or(-1))),
        output: decoded(bytes),
    })
}

fn run_broken(
    target: &Path,
    original: &[u8],
    broken: &[u8],
    run_now: impl FnOnce() -> Outcome<TestRun>,
) -> Outcome<TestRun> {
    let sidecar = sidecar_of(target);
    fs::write(&sidecar, original).map_err(|error| py::open_error(&error, &sidecar))?;
    let ran = write_as_newer(target, broken, MTIME_STEP_SECONDS)
        .map_err(Refusal::from)
        .and_then(|()| run_now());
    restore(target, original, &sidecar)?;
    ran
}

fn verdict_of(
    broken_run: &TestRun,
    expect: Option<&str>,
    nothing_ran: Option<String>,
) -> (&'static str, String) {
    if broken_run.timed_out() {
        return (
            "inconclusive",
            "the tests gave no result in time".to_string(),
        );
    }
    if broken_run.passed() {
        return match nothing_ran {
            Some(why) => (
                "inconclusive",
                format!("the tests passed on the broken file, but {why}; a run that judges nothing kills nothing"),
            ),
            None => (
                "survived",
                "the tests still pass with this change; either a test is missing, or the change does not break the protection (an equivalent mutation); if the test command selects only some tests, run it over the whole suite, where a kill names the protecting test in failing_tests".to_string(),
            ),
        };
    }
    if build_failed(&broken_run.output) {
        return (
            "inconclusive",
            "the broken code did not build, so no test judged it, whatever --expect matches; break the protection so it still builds".to_string(),
        );
    }
    if let Some(expect) = expect {
        let matches =
            pyregex::compile(expect).is_ok_and(|pattern| pattern.search(&broken_run.output));
        return if matches {
            (
                "killed",
                format!("the tests failed with output matching --expect {expect}"),
            )
        } else {
            (
                "inconclusive",
                format!("the failure does not match --expect {expect}"),
            )
        };
    }
    (
        "killed",
        "the tests failed once the protection was broken".to_string(),
    )
}

fn one_line(text: &str) -> String {
    text.split(py::is_space)
        .filter(|word| !word.is_empty())
        .collect::<Vec<_>>()
        .join(" ")
}

fn require_pattern(expect: Option<&str>) -> Outcome<()> {
    let Some(expect) = expect else {
        return Ok(());
    };
    if VERDICT_WORDS.contains(&casefold(py::strip(expect)).as_str()) {
        return Err(format!(
            "--expect is a regular expression the failing output must match, such as the assertion message, not a verdict: {} would match almost any failure",
            py::repr(expect)
        )
        .into());
    }
    pyregex::compile(expect)
        .map(|_| ())
        .map_err(|error| format!("--expect is not a regular expression: {error}").into())
}

fn baseline_log(output: &str) -> Result<PathBuf, String> {
    const LETTERS: &[u8] = b"abcdefghijklmnopqrstuvwxyz0123456789_";
    let directory = std::env::temp_dir();
    for _ in 0..100 {
        let seed = std::collections::hash_map::RandomState::new();
        let mut word = std::hash::BuildHasher::hash_one(&seed, SystemTime::now());
        let name: String = (0..8)
            .map(|_| {
                let letter = LETTERS[(word % LETTERS.len() as u64) as usize] as char;
                word /= LETTERS.len() as u64;
                letter
            })
            .collect();
        let path = directory.join(format!("counterfactual-baseline-{name}.log"));
        match fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&path)
        {
            Ok(mut log) => {
                log.write_all(output.as_bytes())
                    .map_err(|error| py::os_error(&error, &path))?;
                return Ok(path);
            }
            Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => continue,
            Err(error) => return Err(py::open_error(&error, &path)),
        }
    }
    Err("No usable temporary file name found".to_string())
}

fn failing_baseline(output: &str) -> Refusal {
    let path = match baseline_log(output) {
        Ok(path) => py::shown(&path),
        Err(error) => return error.into(),
    };
    let mut details = Object::new();
    details.insert("failure_evidence", Value::Str(failure_evidence(output)));
    details.insert(
        "failing_tests",
        Value::Array(failing_tests(output).into_iter().map(Value::Str).collect()),
    );
    details.insert("log_path", Value::Str(path.clone()));
    Refusal {
        reason: format!(
            "the tests do not pass before anything is broken, so a failure afterwards would prove nothing; the whole output is in {path}"
        ),
        details,
    }
}

fn intact_run(directory: &Path, proof: &Proof, runs: &mut IntactRuns) -> Outcome<TestRun> {
    let key = (
        proof.test_command.clone(),
        directory.to_path_buf(),
        proof.timeout,
    );
    if !runs.contains_key(&key) {
        let ran = run_tests(directory, &proof.test_command, proof.timeout)?;
        runs.insert(key.clone(), ran);
    }
    Ok(runs[&key].clone())
}

fn prove(
    root: &Path,
    mutation: &Mutation,
    proof: &Proof,
    runs: &mut IntactRuns,
) -> Outcome<Object> {
    let target = target_inside(root, &mutation.file)?;
    let directory = working_directory(root, proof.cwd.as_deref())?;
    require_pattern(proof.expect.as_deref())?;
    let original = fs::read(&target).map_err(|error| py::open_error(&error, &target))?;
    let broken = mutated(&original, mutation)?;
    let argv = shlex::split(&proof.test_command)?;
    let intact = intact_run(&directory, proof, runs)?;
    if !intact.passed() {
        return Err(failing_baseline(&intact.output));
    }
    if let Some(nothing_ran) = no_test_passed(&argv, &intact.output) {
        return Err(format!(
            "the tests pass before anything is broken, but {nothing_ran}; a run that judges nothing can kill nothing"
        )
        .into());
    }
    let broken_run = run_broken(&target, &original, &broken, || {
        run_tests(&directory, &proof.test_command, proof.timeout)
    })?;
    let (verdict, reason) = verdict_of(
        &broken_run,
        proof.expect.as_deref(),
        no_test_passed(&argv, &broken_run.output),
    );
    let restored = fs::read(&target).map_err(|error| py::open_error(&error, &target))? == original;
    let shown = py::relative_to(&target, root)?;
    let first = occurrences(&original, mutation.find.as_bytes())[0];
    let line = line_at(&original, first);
    let evidence = failure_evidence(&broken_run.output);
    let mut protection = Object::new();
    protection.insert("from", Value::Str(mutation.find.clone()));
    protection.insert("to", Value::Str(mutation.replace.clone()));
    let mut restoration = Object::new();
    restoration.insert("restored", Value::Bool(restored));
    restoration.insert(
        "content_sha256",
        Value::Str(format!("sha256:{}", hex_digest(&original))),
    );
    let mut result = Object::new();
    result.insert("format", Value::Str(FORMAT.to_string()));
    result.insert("file", Value::Str(shown.clone()));
    result.insert("line", Value::int(line));
    result.insert("mutated_protection", Value::Object(protection));
    result.insert("test_command", Value::Str(proof.test_command.clone()));
    result.insert("verdict", Value::Str(verdict.to_string()));
    result.insert("reason", Value::Str(reason));
    result.insert("failure_evidence", Value::Str(evidence.clone()));
    result.insert(
        "failing_tests",
        Value::Array(
            failing_tests(&broken_run.output)
                .into_iter()
                .map(Value::Str)
                .collect(),
        ),
    );
    result.insert("restoration_result", Value::Object(restoration));
    if verdict == "killed" && restored {
        let mut check = Object::new();
        check.insert("status", Value::Str("passed".to_string()));
        let obligation = proof
            .obligation_id
            .as_deref()
            .filter(|id| !id.is_empty())
            .unwrap_or(OBLIGATION_PLACEHOLDER);
        check.insert("obligation_id", Value::Str(obligation.to_string()));
        check.insert(
            "mutated_protection",
            Value::Str(format!(
                "{shown}:{line}: {} -> {}",
                one_line(&mutation.find),
                one_line(&mutation.replace)
            )),
        );
        check.insert("failure_evidence", Value::Str(evidence));
        check.insert("restored", Value::Bool(true));
        result.insert("counterfactual_check", Value::Object(check));
    }
    Ok(result)
}

pub fn counterfactual(given_root: &str, mutation: &Mutation, proof: &Proof) -> Outcome<Object> {
    let root = py::resolve(given_root);
    refuse_leftovers(&root)?;
    prove(&root, mutation, proof, &mut IntactRuns::new())
}

fn text_field(values: &Object, key: &str) -> Option<String> {
    values.get(key).and_then(Value::as_str).map(str::to_string)
}

fn suite_entry(entry: &Value, suite: &Object, index: usize) -> Outcome<(Mutation, Proof)> {
    let Value::Object(entry) = entry else {
        return Err(
            format!("mutations[{index}] must be an object with file, find and replace").into(),
        );
    };
    let mut values = Object::new();
    for key in ["test_command", "cwd", "timeout", "expect"] {
        values.insert(key, suite.get(key).cloned().unwrap_or(Value::Null));
    }
    for (key, value) in entry.iter() {
        values.insert(key.clone(), value.clone());
    }
    let missing: Vec<&str> = ["file", "find", "replace", "test_command"]
        .into_iter()
        .filter(|key| text_field(&values, key).is_none())
        .collect();
    if !missing.is_empty() {
        return Err(format!(
            "mutations[{index}] needs {}; test_command may be set once for the suite",
            missing.join(", ")
        )
        .into());
    }
    let timeout =
        match values.get("timeout") {
            None | Some(Value::Null) => DEFAULT_TIMEOUT_SECONDS,
            Some(Value::Int(digits)) => digits
                .parse::<i64>()
                .unwrap_or(if digits.starts_with('-') { 0 } else { i64::MAX }),
            Some(_) => 0,
        };
    if timeout < 1 {
        return Err(
            format!("mutations[{index}] timeout must be a positive number of seconds").into(),
        );
    }
    for key in ["cwd", "expect", "obligation_id"] {
        if values
            .get(key)
            .is_some_and(|found| !matches!(found, Value::Null | Value::Str(_)))
        {
            return Err(format!("mutations[{index}] {key} must be text").into());
        }
    }
    let text = |key: &str| text_field(&values, key).unwrap_or_default();
    Ok((
        Mutation {
            file: text("file"),
            find: text("find"),
            replace: text("replace"),
        },
        Proof {
            test_command: text("test_command"),
            timeout,
            cwd: text_field(&values, "cwd"),
            expect: text_field(&values, "expect"),
            obligation_id: text_field(&values, "obligation_id"),
        },
    ))
}

fn error_row(reason: String, details: Object, lead: Vec<(String, Value)>) -> Object {
    let mut row = Object::new();
    row.insert("format", Value::Str(FORMAT.to_string()));
    for (key, value) in lead {
        row.insert(key, value);
    }
    row.insert("verdict", Value::Str("error".to_string()));
    row.insert("reason", Value::Str(reason));
    for (key, value) in details.iter() {
        row.insert(key.clone(), value.clone());
    }
    row
}

fn suite_result(
    root: &Path,
    entry: &Value,
    suite: &Object,
    index: usize,
    runs: &mut IntactRuns,
) -> Object {
    let mut identity: Vec<(String, Value)> = Vec::new();
    if let Value::Object(held) = entry {
        for key in ["obligation_id", "file", "test_command"] {
            let own = held.get(key).is_some_and(|value| value.as_str().is_some());
            let inherited =
                key == "test_command" && suite.get(key).is_some_and(|v| v.as_str().is_some());
            if own || inherited {
                let mut value = held.get(key).cloned().unwrap_or(Value::Null);
                if key == "test_command" && matches!(value, Value::Null) {
                    value = suite.get(key).cloned().unwrap_or(Value::Null);
                }
                identity.push((key.to_string(), value));
            }
        }
    }
    let proven = suite_entry(entry, suite, index).and_then(|(mutation, proof)| {
        prove(root, &mutation, &proof, runs).map(|result| (result, proof))
    });
    match proven {
        Ok((mut result, proof)) => {
            result.insert("entry", Value::int(index));
            if let Some(id) = proof.obligation_id.filter(|id| !id.is_empty()) {
                result.insert("obligation_id", Value::Str(id));
            }
            result
        }
        Err(refusal) => {
            let mut lead = vec![("entry".to_string(), Value::int(index))];
            lead.extend(identity);
            error_row(refusal.reason, refusal.details, lead)
        }
    }
}

pub fn counterfactual_suite(given_root: &str, suite: &Object) -> Outcome<Object> {
    let root = py::resolve(given_root);
    let mutations = match suite.get("mutations") {
        Some(Value::Array(items)) if !items.is_empty() => items,
        _ => {
            return Err(
                "--suite must be a JSON object whose mutations is a non-empty list"
                    .to_string()
                    .into(),
            );
        }
    };
    refuse_leftovers(&root)?;
    let mut runs = IntactRuns::new();
    let results: Vec<Object> = mutations
        .iter()
        .enumerate()
        .map(|(position, entry)| suite_result(&root, entry, suite, position + 1, &mut runs))
        .collect();
    let mut summary = Object::new();
    for verdict in VERDICTS {
        let count = results
            .iter()
            .filter(|row| row.get("verdict").and_then(Value::as_str) == Some(verdict))
            .count();
        summary.insert(verdict, Value::int(count));
    }
    let mut report = Object::new();
    report.insert("format", Value::Str(SUITE_FORMAT.to_string()));
    report.insert(
        "results",
        Value::Array(results.into_iter().map(Value::Object).collect()),
    );
    report.insert("summary", Value::Object(summary));
    Ok(report)
}

pub fn require_recordable(package: &Path, obligation_id: Option<&str>) -> Outcome<()> {
    let Some(obligation_id) =
        obligation_id.filter(|id| !id.is_empty() && *id != OBLIGATION_PLACEHOLDER)
    else {
        return Err(
            "--package-root records the check under an obligation; name it with --obligation-id"
                .to_string()
                .into(),
        );
    };
    let proposal = load_json(&package.join(PROPOSAL_FILE))?;
    let status = proposal
        .get("status")
        .map_or_else(|| "None".to_string(), str_of);
    if !RECORDABLE_STATUSES.contains(&status.as_str()) {
        return Err(format!(
            "the proposal is {status}: its counterfactual checks are what verify-proposal checked, so they stay as they are; redraft it to record new ones"
        )
        .into());
    }
    let obligations = load_json(&package.join(OBLIGATIONS_FILE))?;
    let named = match obligations.get("obligations") {
        None => false,
        Some(Value::Array(items)) => items.iter().any(|item| {
            item.as_object()
                .and_then(|found| found.get("id"))
                .is_some_and(|id| id.as_str() == Some(obligation_id))
        }),
        Some(_) => false,
    };
    if !named {
        return Err(format!("test-obligations.json has no obligation {obligation_id}").into());
    }
    let planned = proposal
        .get("implementation_design")
        .and_then(Value::as_object)
        .and_then(|design| design.get("proof_obligations"))
        .and_then(Value::as_array);
    if let Some(planned) = planned
        && !planned
            .iter()
            .any(|item| item.as_str() == Some(obligation_id))
    {
        let listed: Vec<String> = planned.iter().map(str_of).collect();
        return Err(format!(
            "obligation {obligation_id} is not one of implementation_design's proof_obligations: {}",
            listed.join(", ")
        )
        .into());
    }
    Ok(())
}

fn record_counterfactual(package: &Path, check: &Object) -> Outcome<PathBuf> {
    let obligation = check.get("obligation_id").and_then(Value::as_str);
    require_recordable(package, obligation)?;
    let path = package.join(EVIDENCE_FILE);
    let mut evidence = load_json(&path)?;
    let runs: Vec<Value> = match evidence.get("counterfactual_check") {
        Some(Value::Array(items)) => items.clone(),
        Some(Value::Object(held))
            if held.get("status").and_then(Value::as_str) == Some("passed") =>
        {
            vec![Value::Object(held.clone())]
        }
        _ => Vec::new(),
    };
    let target = check.get("obligation_id").cloned().unwrap_or(Value::Null);
    let mut kept: Vec<Value> = runs
        .into_iter()
        .filter(|run| {
            !run.as_object().is_some_and(|held| {
                equal(held.get("obligation_id").unwrap_or(&Value::Null), &target)
            })
        })
        .collect();
    kept.push(Value::Object(check.clone()));
    evidence.insert("counterfactual_check", Value::Array(kept));
    write_json(&path, &Value::Object(evidence))?;
    Ok(path)
}

pub fn with_record(package: &Path, repo_root: &str, mut result: Object) -> Object {
    let Some(Value::Object(check)) = result.get("counterfactual_check").cloned() else {
        result.insert(
            "not_recorded",
            Value::Str(
                "only a killed counterfactual whose file was restored is recorded".to_string(),
            ),
        );
        return result;
    };
    match record_counterfactual(package, &check) {
        Ok(recorded) => {
            let top = py::resolve(repo_root);
            result.insert(
                "recorded_in",
                Value::Str(crate::store::shown_path(&recorded, Some(&top))),
            );
        }
        Err(refusal) => result.insert("not_recorded", Value::Str(refusal.reason)),
    }
    result
}

pub fn error_result(refusal: Refusal) -> Object {
    error_row(refusal.reason, refusal.details, Vec::new())
}
