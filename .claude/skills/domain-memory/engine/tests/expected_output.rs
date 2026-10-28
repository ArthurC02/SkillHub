#[allow(dead_code, unused_imports)]
#[path = "../src/json.rs"]
mod json;

mod okf {
    pub const INT_MAX_STR_DIGITS: usize = 4300;
}

use std::collections::{BTreeMap, BTreeSet};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Command, ExitStatus, Stdio};
use std::sync::Mutex;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};

use json::{Object, Value};

const RUN_SECONDS: u64 = 120;
const FIXED_GIT_DATE: &str = "2026-01-01T00:00:00Z";
const UNKNOWN_OPTION: &str = "--not-an-option-of-any-command";
const PLATFORM: &str = if cfg!(windows) { "windows" } else { "posix" };
const VOLATILE_FIELDS: [&str; 5] = [
    "event_sha256",
    "previous_event_sha256",
    "head_sha256",
    "heads",
    "operation_id",
];
const RANDOM_NAME_PREFIXES: [&[u8]; 2] = [b"domain-registry-stage-", b"domain-registry-backup-"];

struct Run {
    exit: String,
    stdout: Vec<u8>,
    stderr: Vec<u8>,
}

struct Outcome {
    exit: String,
    stdout: Vec<u8>,
    stderr: Vec<u8>,
    files: Option<BTreeMap<String, Vec<u8>>>,
}

struct Suite {
    binary: PathBuf,
    snapshot: PathBuf,
    plugin: PathBuf,
    repo: PathBuf,
    scratch_root: PathBuf,
    next_scratch: AtomicUsize,
}

fn engine() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn read_json(path: &Path) -> Value {
    let text = std::fs::read_to_string(path)
        .unwrap_or_else(|error| panic!("cannot read {}: {error}", path.display()));
    json::parse(&text).unwrap_or_else(|()| panic!("{} is not JSON", path.display()))
}

fn generated_cases(contract: &Value) -> Vec<Value> {
    let named = |name: &str, args: &[&str]| {
        let mut case = Object::new();
        case.insert("name", Value::Str(name.to_string()));
        case.insert(
            "args",
            Value::Array(args.iter().map(|arg| Value::Str(arg.to_string())).collect()),
        );
        Value::Object(case)
    };
    let mut cases = vec![named("no command", &[]), named("root help", &["--help"])];
    let commands = contract
        .as_object()
        .and_then(|contract| contract.get("commands"))
        .and_then(Value::as_object)
        .expect("the CLI contract lists its commands");
    for (command, _) in commands.iter() {
        cases.push(named(&format!("{command} help"), &[command, "-h"]));
        cases.push(named(
            &format!("{command} unknown option"),
            &[command, UNKNOWN_OPTION],
        ));
    }
    cases
}

fn field<'a>(case: &'a Value, key: &str) -> Option<&'a Value> {
    case.as_object().and_then(|case| case.get(key))
}

fn text<'a>(case: &'a Value, key: &str) -> &'a str {
    field(case, key).and_then(Value::as_str).unwrap_or("")
}

fn flag(case: &Value, key: &str) -> bool {
    matches!(field(case, key), Some(Value::Bool(true)))
}

fn stored_bytes(value: &Value) -> Vec<u8> {
    match value {
        Value::Str(text) => text.as_bytes().to_vec(),
        Value::Object(object) => {
            let hex = object.get("hex").and_then(Value::as_str).unwrap_or("");
            (0..hex.len())
                .step_by(2)
                .map(|at| u8::from_str_radix(&hex[at..at + 2], 16).expect("hex content"))
                .collect()
        }
        _ => Vec::new(),
    }
}

fn written_fixture(root: &Path, files: Option<&Value>) {
    let Some(files) = files.and_then(Value::as_object) else {
        return;
    };
    for (relative, content) in files.iter() {
        let path = root.join(relative);
        if matches!(content, Value::Null) {
            std::fs::create_dir_all(&path).expect("fixture directory");
            continue;
        }
        std::fs::create_dir_all(path.parent().expect("a fixture file has a parent"))
            .expect("fixture directory");
        std::fs::write(&path, stored_bytes(content)).expect("fixture file");
    }
}

fn git_in(root: &Path, args: &[&str]) {
    let done = Command::new("git")
        .arg("-C")
        .arg(root)
        .args(["-c", "core.autocrlf=false", "-c", "maintenance.auto=false"])
        .args(args)
        .env("GIT_AUTHOR_DATE", FIXED_GIT_DATE)
        .env("GIT_COMMITTER_DATE", FIXED_GIT_DATE)
        .output()
        .expect("git runs");
    assert!(
        done.status.success(),
        "git {} failed in {}: {}{}",
        args.join(" "),
        root.display(),
        String::from_utf8_lossy(&done.stdout),
        String::from_utf8_lossy(&done.stderr)
    );
}

fn prepared(case: &Value, fixture: &Path) {
    written_fixture(fixture, field(case, "files"));
    if flag(case, "git") {
        git_in(fixture, &["init", "-q"]);
        git_in(fixture, &["add", "-A"]);
        git_in(
            fixture,
            &[
                "-c",
                "user.name=parity",
                "-c",
                "user.email=parity@example.invalid",
                "commit",
                "-q",
                "-m",
                "fixture",
            ],
        );
    }
}

fn exit_text(status: ExitStatus) -> String {
    #[cfg(unix)]
    {
        use std::os::unix::process::ExitStatusExt;
        if let Some(signal) = status.signal() {
            return format!("-{signal}");
        }
    }
    match status.code() {
        Some(code) if cfg!(windows) => (code as u32).to_string(),
        Some(code) => code.to_string(),
        None => "no exit code".to_string(),
    }
}

fn drained(mut pipe: impl Read + Send + 'static) -> std::thread::JoinHandle<Vec<u8>> {
    std::thread::spawn(move || {
        let mut bytes = Vec::new();
        let _ = pipe.read_to_end(&mut bytes);
        bytes
    })
}

fn run(command: &mut Command, stdin: Vec<u8>) -> Run {
    let mut child = command
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("the command starts");
    let mut input = child.stdin.take().expect("stdin pipe");
    let feeder = std::thread::spawn(move || {
        let _ = input.write_all(&stdin);
    });
    let stdout = drained(child.stdout.take().expect("stdout pipe"));
    let stderr = drained(child.stderr.take().expect("stderr pipe"));
    let deadline = Instant::now() + Duration::from_secs(RUN_SECONDS);
    let exit = loop {
        if let Some(status) = child.try_wait().expect("the command can be waited on") {
            break exit_text(status);
        }
        if Instant::now() >= deadline {
            let _ = child.kill();
            let _ = child.wait();
            break format!("killed after {RUN_SECONDS} s");
        }
        std::thread::sleep(Duration::from_millis(10));
    };
    let _ = feeder.join();
    Run {
        exit,
        stdout: stdout.join().unwrap_or_default(),
        stderr: stderr.join().unwrap_or_default(),
    }
}

fn is_hex(byte: u8) -> bool {
    byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte)
}

fn hex_run(data: &[u8], at: usize, length: usize) -> bool {
    data.len() >= at + length && data[at..at + length].iter().all(|&byte| is_hex(byte))
}

fn without_digests(value: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(value.len());
    let mut at = 0;
    while at < value.len() {
        let digest = if value[at..].starts_with(b"sha256:") && hex_run(value, at + 7, 64) {
            71
        } else if hex_run(value, at, 64) {
            64
        } else if hex_run(value, at, 32) {
            32
        } else {
            0
        };
        if digest > 0 {
            out.push(b'~');
            at += digest;
        } else {
            out.push(value[at]);
            at += 1;
        }
    }
    out
}

fn field_value_length(rest: &[u8]) -> Option<usize> {
    match rest.first()? {
        b'"' => rest[1..]
            .iter()
            .position(|&byte| byte == b'"')
            .map(|end| end + 2),
        b'[' => rest[1..]
            .iter()
            .position(|&byte| byte == b']')
            .map(|end| end + 2),
        _ if rest.starts_with(b"null") => Some(4),
        _ => None,
    }
}

fn volatile_field_at(data: &[u8], at: usize) -> Option<(usize, usize)> {
    if data[at] != b'"' {
        return None;
    }
    VOLATILE_FIELDS.iter().find_map(|name| {
        let key = format!("\"{name}\": ");
        let start = at + key.len();
        if !data[at..].starts_with(key.as_bytes()) {
            return None;
        }
        field_value_length(&data[start..]).map(|length| (start, start + length))
    })
}

fn timestamp_at(data: &[u8], at: usize) -> bool {
    const SHAPE: &[u8] = b"dddd-dd-ddTdd:dd:dd.ddddddZ";
    data.len() >= at + SHAPE.len()
        && SHAPE.iter().zip(&data[at..]).all(|(&shape, &byte)| {
            if shape == b'd' {
                byte.is_ascii_digit()
            } else {
                shape == byte
            }
        })
}

fn masked(data: &[u8]) -> Vec<u8> {
    let mut fields = Vec::with_capacity(data.len());
    let mut at = 0;
    while at < data.len() {
        if let Some((start, end)) = volatile_field_at(data, at) {
            fields.extend_from_slice(&data[at..start]);
            fields.extend(without_digests(&data[start..end]));
            at = end;
        } else {
            fields.push(data[at]);
            at += 1;
        }
    }
    let mut out = Vec::with_capacity(fields.len());
    let mut at = 0;
    while at < fields.len() {
        let random_name = hex_run(&fields, at, 32)
            && RANDOM_NAME_PREFIXES
                .iter()
                .any(|prefix| fields[..at].ends_with(prefix));
        if timestamp_at(&fields, at) {
            out.push(b'~');
            at += 27;
        } else if random_name {
            out.push(b'~');
            at += 32;
        } else {
            out.push(fields[at]);
            at += 1;
        }
    }
    out
}

fn native(path: &Path) -> String {
    path.to_string_lossy()
        .trim_start_matches(r"\\?\")
        .to_string()
}

fn spellings(path: &Path) -> Vec<Vec<u8>> {
    let mut found = BTreeSet::new();
    let canonical = std::fs::canonicalize(path).unwrap_or_else(|_| path.to_path_buf());
    for spelled in [native(path), native(&canonical)] {
        found.insert(spelled.replace('\\', "/"));
        found.insert(spelled.replace('\\', "\\\\"));
        found.insert(spelled.replace('\\', "\\\\\\\\"));
        found.insert(spelled);
    }
    let mut spellings: Vec<Vec<u8>> = found.into_iter().map(String::into_bytes).collect();
    spellings.sort_by_key(|spelling| std::cmp::Reverse(spelling.len()));
    spellings
}

fn replaced(data: &[u8], from: &[u8], to: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(data.len());
    let mut at = 0;
    while at < data.len() {
        if !from.is_empty() && data[at..].starts_with(from) {
            out.extend_from_slice(to);
            at += from.len();
        } else {
            out.push(data[at]);
            at += 1;
        }
    }
    out
}

const OS_ERROR_LABELS: [&[u8]; 2] = [b"[Errno ", b"[WinError "];
const OS_WORDING_ENDS: [&[u8]; 6] = [b": ", b". ", b"\n", b"\\n", b"\"", b"'"];

fn found_in(data: &[u8], mark: &[u8]) -> Option<usize> {
    data.windows(mark.len()).position(|window| window == mark)
}

fn os_wording_start(data: &[u8], at: usize) -> Option<usize> {
    OS_ERROR_LABELS.iter().find_map(|label| {
        if !data[at..].starts_with(label) {
            return None;
        }
        let digits_start = at + label.len();
        let digits = data[digits_start..]
            .iter()
            .take_while(|byte| byte.is_ascii_digit())
            .count();
        let close = digits_start + digits;
        (digits > 0 && data[close..].starts_with(b"] ")).then_some(close + 2)
    })
}

fn without_os_wording(data: Vec<u8>) -> Vec<u8> {
    let mut kept = Vec::with_capacity(data.len());
    let mut at = 0;
    while at < data.len() {
        if let Some(wording) = os_wording_start(&data, at) {
            kept.extend_from_slice(b"[OS error] ~");
            at = OS_WORDING_ENDS
                .iter()
                .filter_map(|mark| found_in(&data[wording..], mark).map(|end| wording + end))
                .min()
                .unwrap_or(data.len());
        } else {
            kept.push(data[at]);
            at += 1;
        }
    }
    kept
}

fn tool_output_cut(mut data: Vec<u8>, prefixes: &[String]) -> Vec<u8> {
    for prefix in prefixes {
        if let Some(at) = found_in(&data, prefix.as_bytes()) {
            data.truncate(at + prefix.len());
            data.push(b'~');
        }
    }
    data
}

fn server_info(version: &str) -> Vec<u8> {
    format!("\"name\": \"domain-memory\", \"version\": \"{version}\"").into_bytes()
}

fn plugin_version() -> String {
    let manifest = read_json(&engine().join("../.claude-plugin/plugin.json"));
    text(&manifest, "version").to_string()
}

fn placed(data: &[u8], places: &[(&[u8], Vec<Vec<u8>>)]) -> Vec<u8> {
    let mut data = data.to_vec();
    for (token, spellings) in places {
        for spelling in spellings {
            data = replaced(&data, spelling, token);
        }
    }
    let data = replaced(
        &data,
        &server_info(&plugin_version()),
        &server_info("{version}"),
    );
    without_os_wording(data)
}

fn tree_of(root: &Path) -> BTreeMap<String, Vec<u8>> {
    let mut tree = BTreeMap::new();
    let mut pending = vec![root.to_path_buf()];
    while let Some(directory) = pending.pop() {
        let Ok(entries) = std::fs::read_dir(&directory) else {
            continue;
        };
        for entry in entries.flatten() {
            let path = entry.path();
            if path.file_name().is_some_and(|name| name == ".git") {
                continue;
            }
            if path.is_dir() {
                pending.push(path);
            } else if path.is_file() {
                let relative = path
                    .strip_prefix(root)
                    .expect("inside the fixture")
                    .components()
                    .map(|part| part.as_os_str().to_string_lossy().into_owned())
                    .collect::<Vec<_>>()
                    .join("/");
                tree.insert(relative, masked(&std::fs::read(&path).unwrap_or_default()));
            }
        }
    }
    tree
}

fn json_escaped(text: &str) -> String {
    text.replace('\\', "\\\\").replace('"', "\\\"")
}

fn audit_files(root: &Path, found: &mut Vec<PathBuf>) {
    let Ok(entries) = std::fs::read_dir(root) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if path.is_dir() {
            audit_files(&path, found);
        } else if path.ends_with("audit/events.jsonl") {
            found.push(path);
        }
    }
}

impl Suite {
    fn command(&self, args: &[String], case: &Value) -> Command {
        let mut command = Command::new(&self.binary);
        command
            .args(args)
            .current_dir(&self.snapshot)
            .env("COLUMNS", "80");
        if let Some(environment) = field(case, "env").and_then(Value::as_object) {
            for (name, value) in environment.iter() {
                command.env(name, value.as_str().unwrap_or(""));
            }
        }
        command
    }

    fn audit_problems(&self, fixture: &Path) -> Vec<String> {
        let mut events = Vec::new();
        audit_files(fixture, &mut events);
        events.sort();
        events
            .iter()
            .filter_map(|events| {
                let registry = events.parent()?.parent()?;
                let checked = run(
                    Command::new(&self.binary)
                        .arg("verify-audit")
                        .arg("--registry-root")
                        .arg(registry)
                        .current_dir(&self.repo),
                    Vec::new(),
                );
                (checked.exit != "0").then(|| {
                    format!(
                        "audit written under {} fails verify-audit: {}",
                        registry.strip_prefix(fixture).unwrap_or(registry).display(),
                        String::from_utf8_lossy(&checked.stdout).trim()
                    )
                })
            })
            .collect()
    }

    fn observed(&self, case: &Value) -> (Outcome, Vec<String>) {
        let scratch = self.scratch_root.join(
            self.next_scratch
                .fetch_add(1, Ordering::Relaxed)
                .to_string(),
        );
        let fixture = scratch.join("f");
        std::fs::create_dir_all(&fixture).expect("fixture directory");
        prepared(case, &fixture);
        let fixture_text = fixture.to_string_lossy().into_owned();
        let snapshot_text = self.snapshot.to_string_lossy().into_owned();
        let args: Vec<String> = field(case, "args")
            .and_then(Value::as_array)
            .unwrap_or_default()
            .iter()
            .map(|arg| {
                arg.as_str()
                    .unwrap_or("")
                    .replace("{fixture}", &fixture_text)
                    .replace("{repo}", &snapshot_text)
            })
            .collect();
        let stdin = text(case, "stdin").replace("{fixture}", &json_escaped(&fixture_text));
        let result = run(&mut self.command(&args, case), stdin.into_bytes());
        let places: [(&[u8], Vec<Vec<u8>>); 4] = [
            (b"{fixture}", spellings(&fixture)),
            (b"{scratch}", spellings(&scratch)),
            (b"{repo}", spellings(&self.snapshot)),
            (b"{plugin}", spellings(&self.plugin)),
        ];
        let tool_output: Vec<String> = field(case, "tool_output_after")
            .and_then(Value::as_array)
            .unwrap_or_default()
            .iter()
            .filter_map(|prefix| prefix.as_str().map(str::to_string))
            .collect();
        let mut stdout = tool_output_cut(placed(&result.stdout, &places), &tool_output);
        let mut stderr = tool_output_cut(placed(&result.stderr, &places), &tool_output);
        let mut files = None;
        let mut problems = Vec::new();
        if flag(case, "writes") {
            stdout = masked(&stdout);
            stderr = masked(&stderr);
            files = Some(
                tree_of(&fixture)
                    .into_iter()
                    .map(|(name, data)| (name, placed(&data, &places)))
                    .collect(),
            );
            problems = self.audit_problems(&fixture);
        }
        let _ = std::fs::remove_dir_all(&scratch);
        (
            Outcome {
                exit: result.exit,
                stdout,
                stderr,
                files,
            },
            problems,
        )
    }

    fn against(&self, case: &Value, expected: Option<&Value>) -> Vec<String> {
        let Some(expected) = expected else {
            return vec!["no expected output is recorded for this case".to_string()];
        };
        let (actual, mut problems) = self.observed(case);
        let mut differences = Vec::new();
        for (stream, written) in [("stdout", &actual.stdout), ("stderr", &actual.stderr)] {
            let wanted = field(expected, stream)
                .map(stored_bytes)
                .unwrap_or_default();
            if &wanted != written {
                differences.push(format!("{stream}: {}", first_difference(&wanted, written)));
            }
        }
        let wanted_files: BTreeMap<String, Vec<u8>> = field(expected, "files")
            .and_then(Value::as_object)
            .map(|files| {
                files
                    .iter()
                    .map(|(name, content)| (name.clone(), stored_bytes(content)))
                    .collect()
            })
            .unwrap_or_default();
        let written_files = actual.files.unwrap_or_default();
        let names: BTreeSet<&String> = wanted_files.keys().chain(written_files.keys()).collect();
        for name in names {
            let (wanted, written) = (wanted_files.get(name), written_files.get(name));
            if wanted != written {
                differences.push(format!(
                    "file {name}: {}",
                    first_difference(
                        wanted.map_or(&[][..], Vec::as_slice),
                        written.map_or(&[][..], Vec::as_slice)
                    )
                ));
            }
        }
        differences.append(&mut problems);
        let wanted_exit = match field(expected, "exit") {
            Some(Value::Int(code)) => code.clone(),
            Some(Value::Str(reason)) => reason.clone(),
            _ => String::new(),
        };
        if wanted_exit != actual.exit {
            differences.push(format!(
                "exit: expected {wanted_exit} / actual {}",
                actual.exit
            ));
        }
        differences
    }
}

fn first_difference(expected: &[u8], actual: &[u8]) -> String {
    let expected = String::from_utf8_lossy(expected);
    let actual = String::from_utf8_lossy(actual);
    let split = |text: &str| -> Vec<String> {
        text.split('\n')
            .map(|line| line.trim_end_matches('\r').to_string())
            .collect()
    };
    let (expected_lines, actual_lines) = (split(&expected), split(&actual));
    for (number, (one, other)) in expected_lines.iter().zip(&actual_lines).enumerate() {
        if one != other {
            return format!("line {}: expected {one:?} / actual {other:?}", number + 1);
        }
    }
    format!(
        "expected has {} lines, actual has {}",
        expected_lines.len(),
        actual_lines.len()
    )
}

fn expected_cases(directory: &Path) -> (String, Object) {
    let mut pin = String::new();
    let mut cases = Object::new();
    for platform in ["posix", PLATFORM] {
        let recorded = directory.join(format!("{platform}.json"));
        if !recorded.exists() {
            continue;
        }
        let recording = read_json(&recorded);
        pin = text(&recording, "pin").to_string();
        if let Some(recorded_cases) = field(&recording, "cases").and_then(Value::as_object) {
            for (name, outcome) in recorded_cases.iter() {
                cases.insert(name.clone(), outcome.clone());
            }
        }
    }
    (pin, cases)
}

fn option_value(name: &str) -> Option<String> {
    let args: Vec<String> = std::env::args().collect();
    args.iter()
        .position(|arg| arg == name)
        .and_then(|at| args.get(at + 1).cloned())
}

fn main() {
    let engine = engine();
    let plugin = engine
        .parent()
        .expect("the plugin holds the engine")
        .to_path_buf();
    let repo = engine
        .ancestors()
        .nth(4)
        .expect("the repository holds the plugin")
        .to_path_buf();
    let binary = std::env::var_os("DOMAIN_MEMORY_BINARY")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from(env!("CARGO_BIN_EXE_domain-memory")));
    let binary = std::fs::canonicalize(&binary)
        .unwrap_or_else(|error| panic!("no executable at {}: {error}", binary.display()));
    let only = option_value("--only");
    let mut cases = generated_cases(&read_json(&engine.join("contract").join("cli.json")));
    if let Value::Array(recorded) = read_json(&engine.join("parity").join("cases.json")) {
        cases.extend(recorded);
    }
    let chosen: Vec<Value> = cases
        .into_iter()
        .filter(|case| {
            only.as_deref()
                .is_none_or(|only| text(case, "name").contains(only))
        })
        .collect();
    let (pin, expected) = expected_cases(&engine.join("parity").join("expected"));
    let scratch_root =
        std::env::temp_dir().join(format!("domain-memory-expected-{}", std::process::id()));
    let snapshot = scratch_root.join("repo");
    std::fs::create_dir_all(&scratch_root).expect("scratch directory");
    git_in(
        &repo,
        &[
            "clone",
            "-q",
            "--shared",
            "--no-checkout",
            &repo.to_string_lossy(),
            &snapshot.to_string_lossy(),
        ],
    );
    git_in(&snapshot, &["checkout", "-q", "--detach", &pin]);
    let suite = Suite {
        binary,
        snapshot,
        plugin,
        repo,
        scratch_root: scratch_root.join("cases"),
        next_scratch: AtomicUsize::new(0),
    };
    let results: Vec<Mutex<Vec<String>>> = chosen.iter().map(|_| Mutex::new(Vec::new())).collect();
    let next = AtomicUsize::new(0);
    let workers = std::thread::available_parallelism().map_or(1, usize::from);
    std::thread::scope(|scope| {
        for _ in 0..workers {
            scope.spawn(|| {
                loop {
                    let at = next.fetch_add(1, Ordering::Relaxed);
                    let Some(case) = chosen.get(at) else { break };
                    let problems = suite.against(case, expected.get(text(case, "name")));
                    *results[at].lock().expect("result slot") = problems;
                }
            });
        }
    });
    let _ = std::fs::remove_dir_all(&scratch_root);
    let mut failed = Vec::new();
    for (case, problems) in chosen.iter().zip(&results) {
        let problems = problems.lock().expect("result slot");
        if !problems.is_empty() {
            failed.push(text(case, "name"));
            println!("DIFF {}", text(case, "name"));
            for problem in problems.iter() {
                println!("     {problem}");
            }
        }
    }
    if !failed.is_empty() {
        println!("Cases that differ:");
        for name in &failed {
            println!("  {name}");
        }
    }
    println!(
        "{} of {} cases match",
        chosen.len() - failed.len(),
        chosen.len()
    );
    if !failed.is_empty() || chosen.is_empty() {
        std::process::exit(1);
    }
}
