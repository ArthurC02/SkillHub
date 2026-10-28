use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, ExitStatus, Stdio};
use std::thread;
use std::time::{Duration, Instant};

use crate::json::float_repr;
use crate::py::{self, Raised};

const TIMEOUT_VARIABLE: &str = "DOMAIN_MEMORY_GIT_TIMEOUT";
const DEFAULT_SECONDS: f64 = 15.0;
const NOT_FOUND_CODE: i32 = 2;
const POLL: Duration = Duration::from_millis(5);

pub struct Output {
    pub code: i32,
    pub stdout: Vec<u8>,
    pub stderr: Vec<u8>,
}

#[derive(Clone, Copy, PartialEq, Eq)]
pub enum Failure {
    Spawn,
    Deadline,
    Invalid,
}

pub struct GitError {
    pub message: String,
    pub kind: Failure,
}

impl GitError {
    fn new(kind: Failure, message: String) -> Self {
        Self { message, kind }
    }
}

impl From<GitError> for Raised {
    fn from(error: GitError) -> Self {
        match error.kind {
            Failure::Spawn => Raised::Os(error.message),
            Failure::Deadline => Raised::Subprocess(error.message),
            Failure::Invalid => Raised::Value(error.message),
        }
    }
}

pub fn run_git_bytes(root: &Path, arguments: &[&str]) -> Result<Output, String> {
    run_git(root, arguments, None).map_err(|error| error.message)
}

pub struct TextOutput {
    pub code: i32,
    pub stdout: String,
    pub stderr: String,
}

pub fn run_git_input(
    root: &Path,
    arguments: &[&str],
    input: Option<&[u8]>,
) -> Result<Output, String> {
    run_git(root, arguments, input).map_err(|error| error.message)
}

pub fn run_git_text(root: &Path, arguments: &[&str]) -> Result<TextOutput, String> {
    let done = run_git_bytes(root, arguments)?;
    Ok(TextOutput {
        code: done.code,
        stdout: universal_newlines(&done.stdout),
        stderr: universal_newlines(&done.stderr),
    })
}

pub fn resolve_commit(repo_root: &Path, reference: &str) -> Result<Option<String>, String> {
    let resolved = run_git_text(
        repo_root,
        &[
            "rev-parse",
            "--verify",
            "--quiet",
            &format!("{reference}^{{commit}}"),
        ],
    )?;
    Ok((resolved.code == 0).then(|| py::strip(&resolved.stdout).to_string()))
}

pub fn toplevel(repo_root: &Path) -> Result<Option<PathBuf>, String> {
    let found = run_git_text(repo_root, &["rev-parse", "--show-toplevel"])?;
    Ok((found.code == 0).then(|| py::resolve(py::strip(&found.stdout))))
}

pub fn universal_newlines(bytes: &[u8]) -> String {
    String::from_utf8_lossy(bytes)
        .replace("\r\n", "\n")
        .replace('\r', "\n")
}

pub fn run_git(root: &Path, arguments: &[&str], input: Option<&[u8]>) -> Result<Output, GitError> {
    let root_text = py::shown(root);
    let mut argv = vec!["git", "-C", root_text.as_str()];
    argv.extend_from_slice(arguments);
    run_bounded(&argv, input)
}

pub fn checked_git_bytes(root: &Path, arguments: &[&str], failure: &str) -> Result<Output, String> {
    deadline_seconds()?;
    let result = run_git_bytes(root, arguments).map_err(|error| format!("{failure}: {error}"))?;
    if result.code != 0 {
        let stderr = String::from_utf8_lossy(&result.stderr);
        return Err(format!("{failure}: {}", py::strip(&stderr)));
    }
    Ok(result)
}

pub fn deadline_seconds() -> Result<f64, String> {
    let Some(given) = std::env::var_os(TIMEOUT_VARIABLE) else {
        return Ok(DEFAULT_SECONDS);
    };
    let given = given.to_string_lossy().into_owned();
    let seconds = py::strip(&given).parse::<f64>().unwrap_or(0.0);
    if seconds > 0.0 {
        Ok(seconds)
    } else {
        Err(format!(
            "{TIMEOUT_VARIABLE} must be a positive number of seconds, not {}",
            py::repr(&given)
        ))
    }
}

fn shown_seconds(seconds: f64) -> String {
    if seconds.fract() == 0.0 && seconds.is_finite() {
        format!("{seconds:.0}")
    } else {
        float_repr(seconds)
    }
}

pub fn deadline_message(argv: &[&str], seconds: f64) -> String {
    let command = argv
        .iter()
        .map(|arg| py::repr(arg))
        .collect::<Vec<_>>()
        .join(", ");
    let safe = if argv.first() == Some(&"git") && !argv.contains(&"config") {
        "; nothing was written, so it is safe to run again"
    } else {
        ""
    };
    format!(
        "Command '[{command}]' timed out after {} seconds{safe}; set {TIMEOUT_VARIABLE} to a larger number of seconds to wait longer",
        shown_seconds(seconds)
    )
}

pub fn run_bounded(argv: &[&str], input: Option<&[u8]>) -> Result<Output, GitError> {
    let seconds = deadline_seconds().map_err(|message| GitError::new(Failure::Invalid, message))?;
    let mut command = Command::new(argv[0]);
    command
        .args(&argv[1..])
        .stdin(if input.is_some() {
            Stdio::piped()
        } else {
            Stdio::null()
        })
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let mut child = in_own_process_group(&mut command)
        .spawn()
        .map_err(|error| GitError::new(Failure::Spawn, spawn_failure(&error, argv[0])))?;
    let deadline = Duration::try_from_secs_f64(seconds)
        .ok()
        .and_then(|span| Instant::now().checked_add(span));
    let feeder = fed(child.stdin.take(), input);
    let finished = finished_by(child, deadline)
        .map_err(|error| GitError::new(Failure::Invalid, error.to_string()))?;
    drop(feeder);
    let Some(finished) = finished else {
        return Err(GitError::new(
            Failure::Deadline,
            deadline_message(argv, seconds),
        ));
    };
    Ok(Output {
        code: finished.status.code().unwrap_or(-1),
        stdout: finished.stdout,
        stderr: finished.stderr,
    })
}

pub struct Finished {
    pub status: ExitStatus,
    pub stdout: Vec<u8>,
    pub stderr: Vec<u8>,
}

fn in_own_process_group(command: &mut Command) -> &mut Command {
    #[cfg(unix)]
    {
        use std::os::unix::process::CommandExt;
        command.process_group(0);
    }
    command
}

pub fn finished_by(
    mut child: Child,
    deadline: Option<Instant>,
) -> std::io::Result<Option<Finished>> {
    let stdout = drained(child.stdout.take());
    let stderr = drained(child.stderr.take());
    let mut status = None;
    loop {
        if status.is_none() {
            status = child.try_wait()?;
        }
        if let Some(status) = status
            && stdout.is_finished()
            && stderr.is_finished()
        {
            return Ok(Some(Finished {
                status,
                stdout: stdout.join().unwrap_or_default(),
                stderr: stderr.join().unwrap_or_default(),
            }));
        }
        if deadline.is_some_and(|limit| Instant::now() >= limit) {
            kill_tree(&mut child);
            return Ok(None);
        }
        thread::sleep(POLL);
    }
}

fn fed(
    pipe: Option<impl Write + Send + 'static>,
    input: Option<&[u8]>,
) -> Option<thread::JoinHandle<()>> {
    let (mut pipe, bytes) = (pipe?, input?.to_vec());
    Some(thread::spawn(move || {
        let _ = pipe.write_all(&bytes);
    }))
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

fn kill_tree(child: &mut Child) {
    let pid = child.id().to_string();
    let _ = if cfg!(windows) {
        Command::new("taskkill")
            .args(["/T", "/F", "/PID", &pid])
            .output()
    } else {
        Command::new("sh")
            .args([
                "-c",
                "kill -s KILL -- -\"$0\" 2>/dev/null; pkill -KILL -P \"$0\"",
                &pid,
            ])
            .output()
    };
    let _ = child.kill();
    let _ = child.wait();
}

pub fn spawn_failure(error: &std::io::Error, program: &str) -> String {
    if error.raw_os_error().is_none() && error.kind() == std::io::ErrorKind::NotFound {
        return spawn_failure(&std::io::Error::from_raw_os_error(NOT_FOUND_CODE), program);
    }
    let text = error.to_string();
    match error.raw_os_error() {
        Some(code) if cfg!(windows) => {
            let bare = text
                .strip_suffix(&format!(" (os error {code})"))
                .unwrap_or(&text);
            format!("[WinError {code}] {}", bare.trim_end_matches(['.', ' ']))
        }
        Some(code) => {
            let bare = text
                .strip_suffix(&format!(" (os error {code})"))
                .unwrap_or(&text);
            format!("[Errno {code}] {bare}: {}", py::repr(program))
        }
        None => text,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_deadline_reads_like_python_timeout_expired() {
        assert_eq!(
            deadline_message(&["git", "-C", r"C:\r", "archive"], 15.0),
            r"Command '['git', '-C', 'C:\\r', 'archive']' timed out after 15 seconds; nothing was written, so it is safe to run again; set DOMAIN_MEMORY_GIT_TIMEOUT to a larger number of seconds to wait longer"
        );
        assert!(deadline_message(&["git", "config"], 0.5).contains("after 0.5 seconds; set"));
    }

    #[cfg(unix)]
    fn spawned(script: &str) -> Child {
        let mut command = Command::new("sh");
        command
            .args(["-c", script])
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        in_own_process_group(&mut command)
            .spawn()
            .expect("sh starts")
    }

    #[cfg(unix)]
    fn alive(pid: &str) -> bool {
        if let Ok(stat) = std::fs::read_to_string(format!("/proc/{pid}/stat")) {
            return !stat
                .rsplit_once(") ")
                .is_some_and(|(_, state)| state.starts_with('Z'));
        }
        Command::new("kill")
            .args(["-0", pid])
            .stderr(Stdio::null())
            .status()
            .is_ok_and(|status| status.success())
    }

    #[cfg(unix)]
    #[test]
    fn a_grandchild_holding_the_output_open_after_its_parent_exits_is_stopped_at_the_deadline() {
        let pid_file =
            std::env::temp_dir().join(format!("domain-memory-grandchild-{}", std::process::id()));
        let script = format!("sleep 30 & echo $! > '{}'", pid_file.display());
        let started = Instant::now();
        let finished = finished_by(
            spawned(&script),
            Some(Instant::now() + Duration::from_secs(2)),
        )
        .expect("the child can be waited on");
        assert!(finished.is_none());
        assert!(started.elapsed() < Duration::from_secs(10));
        let grandchild = std::fs::read_to_string(&pid_file).expect("the grandchild wrote its pid");
        let _ = std::fs::remove_file(&pid_file);
        let gone_by = Instant::now() + Duration::from_secs(5);
        while alive(grandchild.trim()) && Instant::now() < gone_by {
            thread::sleep(POLL);
        }
        assert!(!alive(grandchild.trim()));
    }

    #[cfg(unix)]
    #[test]
    fn a_grandchild_that_closes_the_output_in_time_has_all_its_output_kept() {
        let finished = finished_by(
            spawned("(sleep 0.3; echo late) & echo early"),
            Some(Instant::now() + Duration::from_secs(20)),
        )
        .expect("the child can be waited on")
        .expect("it finishes before the deadline");
        assert!(finished.status.success());
        assert_eq!(finished.stdout, b"early\nlate\n");
    }
}
