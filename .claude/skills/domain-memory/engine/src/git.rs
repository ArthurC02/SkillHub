use std::io::Read;
use std::path::Path;
use std::process::{Child, Command, Stdio};
use std::thread;
use std::time::{Duration, Instant};

use crate::json::float_repr;
use crate::py;

const TIMEOUT_VARIABLE: &str = "DOMAIN_MEMORY_GIT_TIMEOUT";
const DEFAULT_SECONDS: f64 = 15.0;
const POLL: Duration = Duration::from_millis(5);

pub struct Output {
    pub code: i32,
    pub stdout: Vec<u8>,
    pub stderr: Vec<u8>,
}

pub fn run_git_bytes(root: &Path, arguments: &[&str]) -> Result<Output, String> {
    let root_text = py::shown(root);
    let mut argv = vec!["git", "-C", root_text.as_str()];
    argv.extend_from_slice(arguments);
    run_bounded(&argv)
}

fn deadline_seconds() -> Result<f64, String> {
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

fn run_bounded(argv: &[&str]) -> Result<Output, String> {
    let seconds = deadline_seconds()?;
    let mut child = Command::new(argv[0])
        .args(&argv[1..])
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|error| spawn_failure(&error, argv[0]))?;
    let stdout = drained(child.stdout.take());
    let stderr = drained(child.stderr.take());
    let deadline = Duration::try_from_secs_f64(seconds)
        .ok()
        .and_then(|span| Instant::now().checked_add(span));
    let status = loop {
        if let Some(status) = child.try_wait().map_err(|error| error.to_string())? {
            break status;
        }
        if deadline.is_some_and(|limit| Instant::now() >= limit) {
            kill_tree(&mut child);
            return Err(deadline_message(argv, seconds));
        }
        thread::sleep(POLL);
    };
    Ok(Output {
        code: status.code().unwrap_or(-1),
        stdout: stdout.join().unwrap_or_default(),
        stderr: stderr.join().unwrap_or_default(),
    })
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
        Command::new("pkill").args(["-KILL", "-P", &pid]).output()
    };
    let _ = child.kill();
    let _ = child.wait();
}

fn spawn_failure(error: &std::io::Error, program: &str) -> String {
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
}
