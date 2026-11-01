mod key;

use key::{is_rust_build_input, tree_key};
use std::env;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::{Command, ExitCode};
use std::time::SystemTime;

const DEVCTL_DIR: &str = "tools/devctl";
const HARNESS_DIR: &str = "tools/harness";
const HARNESS_SOURCE_KEY: &str = env!("HARNESS_SOURCE_KEY");
const REBUILT: &str = "SKILLHUB_HARNESS_REBUILT";
const CACHED_BUILDS: usize = 5;
const GO_BUILD_ENVIRONMENT: [&str; 5] = ["GOFLAGS", "GOTOOLCHAIN", "GOOS", "GOARCH", "CGO_ENABLED"];

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();
    match run(&args) {
        Ok(code) => code,
        Err(err) => {
            eprintln!("harness: {err}");
            ExitCode::FAILURE
        }
    }
}

fn run(args: &[String]) -> Result<ExitCode, String> {
    let cwd = env::current_dir().map_err(|e| format!("current directory: {e}"))?;
    let root = find_root(&cwd)?;
    if let Some(code) = rebuild_if_stale(&root, args)? {
        return Ok(code);
    }
    match args.split_first() {
        Some((command, rest)) if command == "devctl" => devctl(&root, rest),
        _ => Err("usage: harness devctl <args...>".into()),
    }
}

fn rebuild_if_stale(root: &Path, args: &[String]) -> Result<Option<ExitCode>, String> {
    let source = root.join(HARNESS_DIR);
    if is_current(&source, HARNESS_SOURCE_KEY)? {
        return Ok(None);
    }
    if env::var_os(REBUILT).is_some() {
        return Err(format!(
            "the rebuilt harness still differs from {}",
            source.display()
        ));
    }
    let executable = env::current_exe().map_err(|e| format!("locate harness: {e}"))?;
    let aside = executable.with_extension("old");
    let _ = fs::remove_file(&aside);
    fs::rename(&executable, &aside)
        .map_err(|e| format!("move {} aside: {e}", executable.display()))?;
    let built = Command::new("cargo")
        .args(["build", "--quiet", "--release", "--manifest-path"])
        .arg(source.join("Cargo.toml"))
        .status()
        .map_err(|e| format!("start cargo build: {e}"))?;
    if !built.success() {
        let _ = fs::rename(&aside, &executable);
        return Err(format!(
            "cargo build of {} failed ({built})",
            source.display()
        ));
    }
    let status = Command::new(&executable)
        .args(args)
        .env(REBUILT, "1")
        .status()
        .map_err(|e| format!("start {}: {e}", executable.display()))?;
    Ok(Some(exit_code(status.code())))
}

fn is_current(source: &Path, built_from: &str) -> Result<bool, String> {
    Ok(format!("{:016x}", tree_key(source, is_rust_build_input, &[])?) == built_from)
}

fn devctl(root: &Path, args: &[String]) -> Result<ExitCode, String> {
    let source = root.join(DEVCTL_DIR);
    let executable = cached_devctl(&source, &cache_dir()?, &|name| env::var(name).ok())?;
    let status = Command::new(&executable)
        .args(args)
        .current_dir(&source)
        .status()
        .map_err(|e| format!("start {}: {e}", executable.display()))?;
    Ok(exit_code(status.code()))
}

fn exit_code(code: Option<i32>) -> ExitCode {
    code.and_then(|code| u8::try_from(code).ok())
        .map_or(ExitCode::FAILURE, ExitCode::from)
}

fn find_root(start: &Path) -> Result<PathBuf, String> {
    start
        .ancestors()
        .find(|dir| dir.join(DEVCTL_DIR).join("go.mod").is_file())
        .map(Path::to_path_buf)
        .ok_or_else(|| {
            format!(
                "no {DEVCTL_DIR}/go.mod above {}; run harness inside the SkillHub repository",
                start.display()
            )
        })
}

fn cache_dir() -> Result<PathBuf, String> {
    let base = env::var_os("SKILLHUB_HARNESS_CACHE")
        .map(PathBuf::from)
        .or_else(|| env::var_os("LOCALAPPDATA").map(|dir| PathBuf::from(dir).join("skillhub")))
        .or_else(|| env::var_os("XDG_CACHE_HOME").map(|dir| PathBuf::from(dir).join("skillhub")))
        .or_else(|| {
            env::var_os("HOME").map(|dir| PathBuf::from(dir).join(".cache").join("skillhub"))
        })
        .ok_or("no cache directory: set SKILLHUB_HARNESS_CACHE, XDG_CACHE_HOME or HOME")?;
    let dir = base.join("devctl");
    fs::create_dir_all(&dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    Ok(dir)
}

fn cached_devctl(
    source: &Path,
    cache: &Path,
    environment: &dyn Fn(&str) -> Option<String>,
) -> Result<PathBuf, String> {
    let salt: Vec<(&str, String)> = GO_BUILD_ENVIRONMENT
        .iter()
        .map(|name| (*name, environment(name).unwrap_or_default()))
        .collect();
    let key = tree_key(source, is_go_build_input, &salt)?;
    let executable = cache.join(format!("devctl-{key:016x}{}", env::consts::EXE_SUFFIX));
    if executable.is_file() {
        return Ok(executable);
    }
    let partial = cache.join(format!(
        "devctl-{key:016x}-{}.partial{}",
        std::process::id(),
        env::consts::EXE_SUFFIX
    ));
    let status = Command::new("go")
        .args(["build", "-o"])
        .arg(&partial)
        .arg(".")
        .current_dir(source)
        .status()
        .map_err(|e| format!("start go build in {}: {e}", source.display()))?;
    if !status.success() {
        return Err(format!(
            "go build in {} failed ({status})",
            source.display()
        ));
    }
    if fs::rename(&partial, &executable).is_err() {
        let _ = fs::remove_file(&partial);
        if !executable.is_file() {
            return Err(format!("could not place {}", executable.display()));
        }
    }
    prune(cache, &executable);
    Ok(executable)
}

fn is_go_build_input(name: &str) -> bool {
    name == "go.mod" || name == "go.sum" || (name.ends_with(".go") && !name.ends_with("_test.go"))
}

fn prune(cache: &Path, keep: &Path) {
    let Ok(entries) = fs::read_dir(cache) else {
        return;
    };
    let mut builds: Vec<(SystemTime, PathBuf)> = entries
        .filter_map(Result::ok)
        .filter(|entry| entry.file_name().to_string_lossy().starts_with("devctl-"))
        .filter_map(|entry| Some((entry.metadata().ok()?.modified().ok()?, entry.path())))
        .filter(|(_, path)| path != keep)
        .collect();
    builds.sort_by_key(|build| std::cmp::Reverse(build.0));
    for (_, stale) in builds.into_iter().skip(CACHED_BUILDS - 1) {
        let _ = fs::remove_file(stale);
    }
}

#[cfg(test)]
mod tests;
