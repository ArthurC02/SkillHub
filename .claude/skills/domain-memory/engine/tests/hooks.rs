use std::ffi::OsStr;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::{Command, Output, Stdio};
use std::sync::atomic::{AtomicUsize, Ordering};

const EXECUTABLE: &str = env!("CARGO_BIN_EXE_domain-memory");
const EMPTY_SHA: &str = "0000000000000000000000000000000000000000";
const TOOL_LOCATIONS: [&str; 4] = [
    "bin/linux-x86_64/domain-memory",
    "bin/macos-universal/domain-memory",
    "bin/windows-x86_64/domain-memory.exe",
    "target/release/domain-memory",
];
const REFUSED_STALE: &str =
    "domain-memory pre-commit: a staged file no longer matches what the Registry cites from it";
const FULL_REPORT_OF_A: &str = "domain-memory pre-commit: a staged file no longer matches what the Registry cites from it; details: verify-evidence --registry-root memory --repo-root . --staged --path src/a.py prints the full report, as this hook does when DOMAIN_MEMORY_HOOK_VERBOSE is set";
const FULL_REPORT_OF_A_AND_B: &str = "domain-memory pre-commit: a staged file no longer matches what the Registry cites from it; details: verify-evidence --registry-root memory --repo-root . --staged --path src/a.py --path src/b.py prints the full report, as this hook does when DOMAIN_MEMORY_HOOK_VERBOSE is set";
const STALE_FIRST_LINE_OF_A: &str = "rules/cited-rule: src/a.py:1-1 changed; check that its statement still holds; recite only if it does, otherwise amend it";
const MISSING_LINE_OF_A: &str = "rules/cited-rule: src/a.py is not in this commit; relocate-evidence if it moved, otherwise amend or retire the record\nverify-evidence failed: 1 missing citation\n";
const UNMAPPED_NOTE: &str = "NOTE: domain-memory pre-commit: boundary-impact against HEAD reports unknown; run boundary-impact --base HEAD to see the Contexts this change touches\n";
const NOTE: &str = "NOTE: domain-memory pre-commit:";
const INSTALLER_NAME: &str = if cfg!(windows) {
    "domain-memory.exe"
} else {
    "domain-memory"
};

static NEXT_SANDBOX: AtomicUsize = AtomicUsize::new(0);

struct Run {
    code: i32,
    stdout: String,
    stderr: String,
}

impl Run {
    fn of(output: &Output) -> Run {
        Run {
            code: output.status.code().unwrap_or(-1),
            stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
            stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        }
    }

    fn outcome(&self) -> (i32, &str) {
        (self.code, self.stderr.as_str())
    }
}

#[derive(Default)]
struct HookCall<'a> {
    cwd: Option<&'a Path>,
    args: &'a [&'a str],
    stdin: &'a str,
    env: &'a [(&'a str, &'a str)],
    shims: Option<&'a Path>,
}

enum Tool<'a> {
    Real,
    Missing,
    Script(&'a str),
    Replacing(&'a str, &'a str),
}

struct Sandbox {
    base: PathBuf,
    repo: PathBuf,
    gitconfig: PathBuf,
    engine: PathBuf,
}

impl Drop for Sandbox {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.base);
    }
}

fn forward(path: &Path) -> String {
    path.to_string_lossy().replace('\\', "/")
}

fn link(source: &Path, destination: &Path) {
    fs::create_dir_all(destination.parent().unwrap()).unwrap();
    if fs::hard_link(source, destination).is_err() {
        fs::copy(source, destination).unwrap();
    }
}

#[cfg(unix)]
fn make_executable(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

#[cfg(not(unix))]
fn make_executable(_path: &Path) {}

fn write_script(path: &Path, body: &str) {
    fs::create_dir_all(path.parent().unwrap()).unwrap();
    fs::write(path, format!("#!/bin/sh\n{body}\n")).unwrap();
    make_executable(path);
}

fn real_git() -> String {
    let search = std::env::var_os("PATH").unwrap();
    std::env::split_paths(&search)
        .flat_map(|directory| [directory.join("git.exe"), directory.join("git")])
        .find(|candidate| candidate.is_file())
        .map(|found| forward(&found))
        .unwrap()
}

fn platform_location() -> &'static str {
    if cfg!(all(target_os = "linux", target_arch = "x86_64")) {
        TOOL_LOCATIONS[0]
    } else if cfg!(target_os = "macos") {
        TOOL_LOCATIONS[1]
    } else if cfg!(all(windows, target_arch = "x86_64")) {
        TOOL_LOCATIONS[2]
    } else {
        TOOL_LOCATIONS[3]
    }
}

fn excerpt_hash_in(cite_output: &str) -> String {
    let marker = "\"excerpt_sha256\": \"";
    let start = cite_output.find(marker).unwrap() + marker.len();
    let length = cite_output[start..].find('"').unwrap();
    cite_output[start..start + length].to_string()
}

fn fingerprint_in(signing_output: &str) -> String {
    let marker = "\"fingerprint\": \"";
    let start = signing_output.find(marker).unwrap() + marker.len();
    let length = signing_output[start..].find('"').unwrap();
    signing_output[start..start + length].to_string()
}

impl Sandbox {
    fn new() -> Sandbox {
        let base = std::env::temp_dir().join(format!(
            "dm-hooks-{}-{}",
            std::process::id(),
            NEXT_SANDBOX.fetch_add(1, Ordering::SeqCst)
        ));
        let repo = base.join("repo");
        fs::create_dir_all(&repo).unwrap();
        let gitconfig = base.join("gitconfig");
        fs::write(&gitconfig, "").unwrap();
        let mut sandbox = Sandbox {
            engine: base.join("engine"),
            base,
            repo,
            gitconfig,
        };
        sandbox.engine = sandbox.engine("engine", &Tool::Real);
        sandbox.git(&["init", "-q"]);
        sandbox.git(&["config", "core.autocrlf", "false"]);
        sandbox.git(&["config", "gc.auto", "0"]);
        fs::create_dir_all(sandbox.repo.join(".git").join("hooks")).unwrap();
        sandbox
    }

    fn seeded(signer: &str) -> Sandbox {
        let sandbox = Sandbox::new();
        sandbox.seed(signer);
        sandbox
    }

    fn seed(&self, signer: &str) {
        self.write("docs/kept.md", "kept\n");
        let run = self.domain(&[
            "init-domain-memory",
            "--repo-root",
            ".",
            "--output",
            "memory",
            "--source",
            "docs",
            "--storage-mode",
            "tracked",
            "--data-classification",
            "internal",
            "--review-mode",
            "scm-verified",
            "--source-authority",
            "test",
            "--review-verifier",
            "git-signed-commit",
            "--review-trigger",
            "git-push",
            "--ci-requirement",
            "required",
            "--authorized-signer",
            signer,
        ]);
        assert_eq!(0, run.code, "{}", run.stderr);
        self.commit_all("seed");
    }

    fn engine(&self, name: &str, tool: &Tool) -> PathBuf {
        let engine = self.base.join(name);
        let installer = engine.join("inst").join("inst").join(INSTALLER_NAME);
        link(Path::new(EXECUTABLE), &installer);
        for location in TOOL_LOCATIONS {
            let executable = engine.join(location);
            match tool {
                Tool::Real => link(Path::new(EXECUTABLE), &executable),
                Tool::Missing => {}
                Tool::Script(script) => write_script(&executable, script),
                Tool::Replacing(command, behaviour) => write_script(
                    &executable,
                    &format!(
                        "test \"$1\" = {command} && {{ {behaviour}; }}\nexec \"{}\" \"$@\"",
                        forward(&installer)
                    ),
                ),
            }
        }
        engine
    }

    fn command(&self, program: impl AsRef<OsStr>, cwd: &Path) -> Command {
        let mut command = Command::new(program);
        command
            .current_dir(cwd)
            .env("GIT_CONFIG_GLOBAL", &self.gitconfig)
            .env("GIT_CONFIG_NOSYSTEM", "1")
            .env("GIT_TERMINAL_PROMPT", "0")
            .env_remove("DOMAIN_MEMORY_GIT_TIMEOUT")
            .env_remove("DOMAIN_MEMORY_HOOK_VERBOSE")
            .env_remove("DOMAIN_MEMORY_SIGNING_KEY");
        command
    }

    fn git_command(&self, cwd: &Path) -> Command {
        let mut command = self.command("git", cwd);
        command.args([
            "-c",
            "user.name=T",
            "-c",
            "user.email=t@example.com",
            "-c",
            "commit.gpgsign=false",
        ]);
        command
    }

    fn git(&self, args: &[&str]) -> String {
        let output = self.git_command(&self.repo).args(args).output().unwrap();
        assert!(
            output.status.success(),
            "git {args:?} failed: {}",
            String::from_utf8_lossy(&output.stderr)
        );
        String::from_utf8_lossy(&output.stdout).trim().to_string()
    }

    fn domain(&self, args: &[&str]) -> Run {
        self.domain_with(&self.engine, args)
    }

    fn domain_with(&self, engine: &Path, args: &[&str]) -> Run {
        let installer = engine.join("inst").join("inst").join(INSTALLER_NAME);
        Run::of(
            &self
                .command(installer, &self.repo)
                .args(args)
                .output()
                .unwrap(),
        )
    }

    fn write(&self, relative: &str, text: &str) {
        let path = self.repo.join(relative);
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(path, text).unwrap();
    }

    fn stage(&self, relative: &str, text: &str) {
        self.write(relative, text);
        self.git(&["add", relative]);
    }

    fn commit_all(&self, message: &str) {
        self.git(&["add", "-A"]);
        self.git(&["commit", "-qm", message]);
    }

    fn commit_file(&self, relative: &str) -> String {
        self.stage(relative, relative);
        self.git(&["commit", "-qm", &format!("add {relative}")]);
        self.git(&["rev-parse", "HEAD"])
    }

    fn head(&self) -> String {
        self.git(&["rev-parse", "HEAD"])
    }

    fn context(&self, id: &str, implementation_path: Option<&str>) {
        let placed = implementation_path
            .map(|path| format!("implementation_path: {path}\n"))
            .unwrap_or_default();
        self.write(
            &format!("memory/registry/contexts/{id}.md"),
            &format!(
                "---\ntype: Bounded Context\ntitle: {id}\nstatus: stable\n{placed}---\nOwn {id}.\n"
            ),
        );
    }

    fn rule(&self, id: &str, statement: &str, sources: &str) {
        self.write(
            &format!("memory/registry/rules/{id}.md"),
            &format!(
                "---\ntype: Business Rule\nstatus: stable\nsources:\n{sources}---\n{statement}\n\n# Relations\n- Contexts: [orders](/contexts/orders)\n"
            ),
        );
    }

    fn first_line_citation(&self, relative: &str) -> String {
        let run = self.domain(&[
            "cite",
            "--repo-root",
            ".",
            "--path",
            relative,
            "--lines",
            "1-1",
        ]);
        assert_eq!(0, run.code, "{}", run.stderr);
        format!(
            "  - resource: \"{relative}#L1-1\"\n    excerpt_sha256: \"{}\"\n",
            excerpt_hash_in(&run.stdout)
        )
    }

    fn rule_citing_first_line_of(&self, relative: &str) {
        let sources = self.first_line_citation(relative);
        self.rule("cited-rule", "S.", &sources);
        self.commit_all("cite");
    }

    fn cited_source(&self) {
        self.write("src/a.py", "one\ntwo\n");
        self.context("orders", None);
        self.rule_citing_first_line_of("src/a.py");
    }

    fn mapped_contexts_with_a_cited_port(&self) {
        self.context("orders", Some("src/orders"));
        self.context("billing", Some("src/billing"));
        self.write("src/orders/port.py", "v1\n");
        self.write("src/billing/invoice.py", "v1\n");
        self.commit_all("mapped contexts");
        self.rule_citing_first_line_of("src/orders/port.py");
    }

    fn install(&self, engine: &Path) -> PathBuf {
        let run = self.domain_with(
            engine,
            &[
                "install-pre-commit-hook",
                "--registry-root",
                &forward(&self.repo.join("memory")),
                "--repo-root",
                &forward(&self.repo),
            ],
        );
        assert_eq!(0, run.code, "{}", run.stderr);
        self.repo.join(".git").join("hooks").join("pre-commit")
    }

    fn install_pre_push(&self, engine: &Path) -> PathBuf {
        let run = self.domain_with(
            engine,
            &[
                "install-git-hitl-hook",
                "--registry-root",
                &forward(&self.repo.join("memory")),
                "--repo-root",
                &forward(&self.repo),
            ],
        );
        assert_eq!(0, run.code, "{}", run.stderr);
        self.repo.join(".git").join("hooks").join("pre-push")
    }

    fn existing_hook(&self, name: &str, text: &str, executable: bool) -> PathBuf {
        let hooks = self.repo.join(".git").join("hooks");
        fs::create_dir_all(&hooks).unwrap();
        fs::write(hooks.join(name), text).unwrap();
        if executable {
            make_executable(&hooks.join(name));
        }
        hooks.join(format!("{name}.domain-memory-existing"))
    }

    fn git_shim(&self, label: &str, body: &str) -> PathBuf {
        let directory = self.base.join(format!("shims-{label}"));
        write_script(&directory.join("git"), body);
        directory
    }

    fn git_shim_failing(&self, label: &str, condition: &str) -> PathBuf {
        self.git_shim(
            label,
            &format!("{condition} && exit 128\nexec \"{}\" \"$@\"", real_git()),
        )
    }

    fn run_hook(&self, hook: &Path, call: &HookCall) -> Run {
        let mut command = self.command("sh", call.cwd.unwrap_or(&self.repo));
        command
            .arg(hook)
            .args(call.args)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        for (name, value) in call.env {
            command.env(name, value);
        }
        if let Some(shims) = call.shims {
            let inherited = std::env::var_os("PATH").unwrap();
            let search = std::env::join_paths(
                std::iter::once(shims.to_path_buf()).chain(std::env::split_paths(&inherited)),
            )
            .unwrap();
            command.env("PATH", search);
        }
        let mut child = command.spawn().unwrap();
        child
            .stdin
            .take()
            .unwrap()
            .write_all(call.stdin.as_bytes())
            .unwrap();
        Run::of(&child.wait_with_output().unwrap())
    }

    fn commit_hook(&self, hook: &Path) -> Run {
        self.run_hook(hook, &HookCall::default())
    }

    fn push_hook(&self, hook: &Path, refs: &str) -> Run {
        self.run_hook(
            hook,
            &HookCall {
                args: &["origin", "unused"],
                stdin: refs,
                ..HookCall::default()
            },
        )
    }

    fn push_fixture(&self, engine: &Path) -> PathBuf {
        self.seed("SHA256:NOBODY");
        let remote = self.base.join("remote.git");
        self.git(&["init", "-q", "--bare", &forward(&remote)]);
        self.git(&["remote", "add", "origin", &forward(&remote)]);
        self.git(&["push", "-q", "origin", "HEAD:refs/heads/main"]);
        self.git(&["fetch", "-q", "origin"]);
        self.install_pre_push(engine)
    }

    fn new_branch(&self, head: &str) -> String {
        format!("refs/heads/feature {head} refs/heads/feature {EMPTY_SHA}\n")
    }

    fn signed_commit_changing(&self, relative: &str) -> String {
        self.stage(relative, relative);
        let output = self
            .git_command(&self.repo)
            .args([
                "-c",
                "user.name=Probe",
                "-c",
                "user.email=probe@example.com",
                "commit",
                "-qS",
                "-m",
                &format!("change {relative}"),
            ])
            .output()
            .unwrap();
        assert!(
            output.status.success(),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
        self.head()
    }

    fn signing_for_the_registry(&self) {
        let key_file = self.base.join("keys").join("signing-key");
        let run = self.domain(&[
            "init-signing-key",
            "--repo-root",
            ".",
            "--principal",
            "probe@example.com",
            "--key-file",
            &forward(&key_file),
        ]);
        assert_eq!(0, run.code, "{}", run.stderr);
        self.seed(&fingerprint_in(&run.stdout));
    }
}

fn assert_missing_executable(run: &Run, hook: &str, engine: &Path) {
    assert_eq!(1, run.code, "{}", run.stderr);
    let prefix = format!("domain-memory {hook}: no Domain Memory executable at ");
    let rest = run
        .stderr
        .strip_prefix(&prefix)
        .unwrap_or_else(|| panic!("unexpected refusal: {}", run.stderr));
    let (tool, manifest) = rest
        .split_once("; build one with cargo build --release --manifest-path ")
        .unwrap_or_else(|| panic!("unexpected refusal: {}", run.stderr));
    let shown_engine = manifest.strip_suffix("/Cargo.toml\n").unwrap();
    assert_eq!(format!("{shown_engine}/{}", platform_location()), tool);
    assert_eq!(
        engine.file_name().unwrap().to_str().unwrap(),
        shown_engine.rsplit('/').next().unwrap()
    );
}

#[cfg(unix)]
#[test]
fn the_installed_pre_commit_hook_is_executable() {
    use std::os::unix::fs::PermissionsExt;
    let sandbox = Sandbox::seeded("SHA256:seed");
    let hook = sandbox.install(&sandbox.engine);
    let mode = fs::metadata(hook).unwrap().permissions().mode();
    assert_eq!(0o111, mode & 0o111);
}

#[test]
fn the_command_line_installs_the_pre_commit_hook_where_git_runs_hooks() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    let run = sandbox.domain(&[
        "install-pre-commit-hook",
        "--registry-root",
        &forward(&sandbox.repo.join("memory")),
        "--repo-root",
        &forward(&sandbox.repo),
    ]);
    let hook = fs::read_to_string(sandbox.repo.join(".git/hooks/pre-commit")).unwrap();
    assert_eq!(0, run.code, "{}", run.stderr);
    let announced = run.stdout.replace('\\', "/");
    assert!(
        announced.starts_with("Installed Domain Memory pre-commit hook: ")
            && announced.ends_with(".git/hooks/pre-commit\n"),
        "{announced}"
    );
    assert!(hook.starts_with("#!/bin/sh\n# domain-memory-pre-commit\nengine='"));
    assert!(hook.contains("\nmemory='memory'\n"));
    assert!(!hook.contains('\r'));
}

#[test]
fn a_relative_hooks_path_receives_the_hook() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    fs::create_dir(sandbox.repo.join(".githooks")).unwrap();
    sandbox.git(&["config", "core.hooksPath", ".githooks"]);
    let run = sandbox.domain(&[
        "install-pre-commit-hook",
        "--registry-root",
        &forward(&sandbox.repo.join("memory")),
        "--repo-root",
        &forward(&sandbox.repo),
    ]);
    let hook = fs::read_to_string(sandbox.repo.join(".githooks/pre-commit")).unwrap();
    assert_eq!(0, run.code, "{}", run.stderr);
    assert!(hook.contains(
        "existing=\"$(git rev-parse --git-path hooks)/pre-commit.domain-memory-existing\" || exit 1\n"
    ));
    assert!(!sandbox.repo.join(".git/hooks/pre-commit").exists());
}

#[test]
fn an_absolute_hooks_path_receives_the_hook() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    let shared = sandbox.base.join("shared-hooks");
    fs::create_dir(&shared).unwrap();
    sandbox.git(&["config", "core.hooksPath", &forward(&shared)]);
    let run = sandbox.domain(&[
        "install-pre-commit-hook",
        "--registry-root",
        &forward(&sandbox.repo.join("memory")),
        "--repo-root",
        &forward(&sandbox.repo),
    ]);
    assert_eq!(0, run.code, "{}", run.stderr);
    assert!(
        fs::read_to_string(shared.join("pre-commit"))
            .unwrap()
            .contains("# domain-memory-pre-commit\n")
    );
    assert!(!sandbox.repo.join(".git/hooks/pre-commit").exists());
}

#[test]
fn a_configured_hooks_path_that_does_not_exist_is_refused_by_name() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.git(&["config", "core.hooksPath", ".githooks"]);
    let run = sandbox.domain(&[
        "install-pre-commit-hook",
        "--registry-root",
        &forward(&sandbox.repo.join("memory")),
        "--repo-root",
        &forward(&sandbox.repo),
    ]);
    assert_eq!(1, run.code);
    assert!(
        run.stderr
            .contains("Git hooks require an existing hooks directory at ")
            && run.stderr.trim_end().ends_with(".githooks"),
        "{}",
        run.stderr
    );
}

#[test]
fn installing_again_replaces_only_its_own_hook() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    let backup = sandbox.existing_hook("pre-commit", "#!/bin/sh\nexit 0\n", true);
    sandbox.install(&sandbox.engine);
    let hook = sandbox.install(&sandbox.engine);
    assert_eq!("#!/bin/sh\nexit 0\n", fs::read_to_string(backup).unwrap());
    assert!(
        fs::read_to_string(hook)
            .unwrap()
            .contains("# domain-memory-pre-commit\n")
    );
}

#[test]
fn a_foreign_hook_is_refused_when_a_backup_already_exists() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.existing_hook("pre-commit", "#!/bin/sh\nexit 0\n", true);
    let hook = sandbox.install(&sandbox.engine);
    fs::write(&hook, "#!/bin/sh\necho other\n").unwrap();
    let run = sandbox.domain(&[
        "install-pre-commit-hook",
        "--registry-root",
        &forward(&sandbox.repo.join("memory")),
        "--repo-root",
        &forward(&sandbox.repo),
    ]);
    assert_eq!(1, run.code);
    assert!(
        run.stderr
            .contains("existing Domain Memory pre-commit hook backup already exists"),
        "{}",
        run.stderr
    );
    assert_eq!("#!/bin/sh\necho other\n", fs::read_to_string(hook).unwrap());
}

#[test]
fn a_staged_file_whose_cited_lines_changed_stops_the_commit_with_one_line_per_record_and_no_report()
{
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/a.py", "changed\ntwo\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            1,
            format!(
                "{STALE_FIRST_LINE_OF_A}\nverify-evidence failed: 1 stale citation\n{FULL_REPORT_OF_A}\n"
            )
            .as_str()
        ),
        run.outcome()
    );
}

#[test]
fn the_verbose_variable_adds_the_full_report_after_the_record_lines() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/a.py", "changed\ntwo\n");
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            env: &[("DOMAIN_MEMORY_HOOK_VERBOSE", "1")],
            ..HookCall::default()
        },
    );
    let lines: Vec<&str> = run.stderr.lines().collect();
    let report = lines[2..lines.len() - 1].join("\n");
    assert_eq!(
        (
            1,
            STALE_FIRST_LINE_OF_A,
            "verify-evidence failed: 1 stale citation",
            "{",
            FULL_REPORT_OF_A
        ),
        (
            run.code,
            lines[0],
            lines[1],
            lines[2],
            lines[lines.len() - 1]
        )
    );
    assert!(report.contains("\"asset\": \"rules\""), "{report}");
    assert!(report.contains("\"id\": \"cited-rule\""), "{report}");
    assert!(report.contains("\"status\": \"stale\""), "{report}");
}

#[test]
fn a_refusal_the_check_prints_as_an_error_is_shown_without_the_verbose_variable() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let engine = sandbox.engine(
        "refusing-engine",
        &Tool::Replacing(
            "verify-evidence",
            "echo \"ERROR: index unreadable\" >&2; exit 1",
        ),
    );
    let hook = sandbox.install(&engine);
    sandbox.stage("src/a.py", "changed\ntwo\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            1,
            format!("ERROR: index unreadable\n{FULL_REPORT_OF_A}\n").as_str()
        ),
        run.outcome()
    );
}

fn assert_a_cited_path_is_still_checked(relative: &str) {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.write(relative, "one\ntwo\n");
    sandbox.context("orders", None);
    sandbox.rule_citing_first_line_of(relative);
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage(relative, "changed\ntwo\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(1, run.code, "{}", run.stderr);
    assert_eq!(
        format!(
            "rules/cited-rule: {relative}:1-1 changed; check that its statement still holds; recite only if it does, otherwise amend it"
        ),
        run.stderr.lines().next().unwrap()
    );
}

#[test]
fn a_cited_path_holding_a_space_is_still_checked() {
    assert_a_cited_path_is_still_checked("my docs/a.py");
}

#[test]
fn a_cited_path_holding_non_ascii_letters_is_still_checked() {
    assert_a_cited_path_is_still_checked("資料/a.py");
}

#[test]
fn a_weakened_line_staged_while_the_disk_holds_the_original_stops_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/a.py", "changed\ntwo\n");
    sandbox.write("src/a.py", "one\ntwo\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            1,
            format!(
                "{STALE_FIRST_LINE_OF_A}\nverify-evidence failed: 1 stale citation\n{FULL_REPORT_OF_A}\n"
            )
            .as_str()
        ),
        run.outcome()
    );
}

#[test]
fn an_intact_line_staged_while_the_disk_holds_a_weakened_one_commits() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/a.py", "one\nadded\n");
    sandbox.write("src/a.py", "changed\nadded\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, UNMAPPED_NOTE), run.outcome());
}

#[test]
fn a_stale_citation_of_a_file_left_unstaged_does_not_stop_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    sandbox.write("src/b.py", "kept\n");
    let sources = format!(
        "{}{}",
        sandbox.first_line_citation("src/a.py"),
        sandbox.first_line_citation("src/b.py")
    );
    sandbox.rule("two-files", "S.", &sources);
    sandbox.commit_all("cite both");
    let hook = sandbox.install(&sandbox.engine);
    sandbox.write("src/a.py", "changed\ntwo\n");
    sandbox.stage("src/b.py", "kept\nadded\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, UNMAPPED_NOTE), run.outcome());
}

#[test]
fn a_staged_listing_that_fails_stops_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("notes/uncited.md", "anything\n");
    let shims = sandbox.git_shim_failing("listing", "test \"$3\" = diff");
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            shims: Some(&shims),
            ..HookCall::default()
        },
    );
    assert_eq!((1, ""), run.outcome());
}

#[test]
fn a_commit_staging_only_uncited_files_needs_no_executable() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.install(&engine);
    sandbox.stage("notes/uncited.md", "anything\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, ""), run.outcome());
}

#[test]
fn deleting_a_cited_file_stops_the_commit_and_names_the_record_and_path() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.git(&["rm", "-q", "src/a.py"]);
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            1,
            format!("{MISSING_LINE_OF_A}{FULL_REPORT_OF_A}\n").as_str()
        ),
        run.outcome()
    );
}

fn expected_rename_refusal() -> String {
    format!(
        "{MISSING_LINE_OF_A}src/a.py is renamed to src/b.py in this commit; run relocate-evidence --from src/a.py --to src/b.py\n{FULL_REPORT_OF_A_AND_B}\n"
    )
}

#[test]
fn renaming_a_cited_file_stops_the_commit_and_names_the_relocation() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.git(&["mv", "src/a.py", "src/b.py"]);
    let run = sandbox.commit_hook(&hook);
    assert_eq!((1, expected_rename_refusal().as_str()), run.outcome());
}

#[test]
fn the_rename_hint_does_not_depend_on_the_diff_renames_setting() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.git(&["config", "diff.renames", "false"]);
    sandbox.git(&["mv", "src/a.py", "src/b.py"]);
    let run = sandbox.commit_hook(&hook);
    assert_eq!((1, expected_rename_refusal().as_str()), run.outcome());
}

#[test]
fn a_rename_commits_once_relocate_evidence_has_moved_the_citations() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.git(&["mv", "src/a.py", "src/b.py"]);
    let relocated = sandbox.domain(&[
        "relocate-evidence",
        "--registry-root",
        "memory",
        "--repo-root",
        ".",
        "--from",
        "src/a.py",
        "--to",
        "src/b.py",
    ]);
    assert_eq!(0, relocated.code, "{}", relocated.stderr);
    let sources = sandbox.first_line_citation("src/b.py");
    sandbox.rule("cited-rule", "S.", &sources);
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, UNMAPPED_NOTE), run.outcome());
}

#[test]
fn a_cited_file_staged_where_no_executable_is_found_stops_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.install(&engine);
    sandbox.stage("src/a.py", "one\nchanged\n");
    assert_missing_executable(&sandbox.commit_hook(&hook), "pre-commit", &engine);
}

#[test]
fn a_registry_search_that_fails_stops_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("notes/uncited.md", "anything\n");
    let shims = sandbox.git_shim_failing("search", "test \"$1\" = grep");
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            shims: Some(&shims),
            ..HookCall::default()
        },
    );
    assert_eq!(
        (
            1,
            "domain-memory pre-commit: could not search the Registry for the staged files\n"
        ),
        run.outcome()
    );
}

#[test]
fn a_staged_file_named_only_in_prose_needs_no_executable() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.write("src/a.py", "one\ntwo\n");
    sandbox.context("orders", None);
    sandbox.write(
        "memory/changes/old/requirement-normalization.json",
        "{\"text\": \"Update README.md and src/a.py.\", \"path\": \"README.md\"}\n",
    );
    let sources = sandbox.first_line_citation("src/a.py");
    sandbox.rule("cited-rule", "README.md says so.", &sources);
    sandbox.commit_all("prose");
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.install(&engine);
    sandbox.stage("README.md", "anything\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, ""), run.outcome());
}

#[test]
fn a_staged_file_whose_path_begins_a_cited_path_needs_no_executable() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.install(&engine);
    sandbox.stage("src/a", "anything\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, ""), run.outcome());
}

fn assert_staging_needs_executable(sandbox: &Sandbox, staged: &str) {
    sandbox.commit_all("arranged");
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.install(&engine);
    sandbox.stage(staged, "one\nchanged\n");
    assert_missing_executable(&sandbox.commit_hook(&hook), "pre-commit", &engine);
}

#[test]
fn a_file_named_in_the_source_map_needs_the_executable() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    sandbox.write(
        "memory/source-map.json",
        "{\"sources\": [{\"path\": \"docs/selected.md\"}]}\n",
    );
    assert_staging_needs_executable(&sandbox, "docs/selected.md");
}

#[test]
fn a_plain_path_kept_beside_a_plain_text_citation_needs_the_executable() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    sandbox.write("src/b.py", "one\ntwo\n");
    sandbox.rule(
        "mixed",
        "S.",
        "  - resource: \"src/b.py:2\"\n  - path: src/b.py\n",
    );
    assert_staging_needs_executable(&sandbox, "src/b.py");
}

#[test]
fn a_quoted_path_kept_beside_a_plain_text_citation_needs_the_executable() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    sandbox.write(".hidden/b.py", "one\ntwo\n");
    sandbox.rule(
        "mixed",
        "S.",
        "  - resource: \".hidden/b.py:2\"\n  - path: \".hidden/b.py\"\n",
    );
    assert_staging_needs_executable(&sandbox, ".hidden/b.py");
}

#[test]
fn a_change_across_two_contexts_commits_with_the_boundary_note() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.mapped_contexts_with_a_cited_port();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/orders/port.py", "v1\nadded\n");
    sandbox.stage("src/billing/invoice.py", "v2\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            0,
            format!(
                "{NOTE} boundary-impact against HEAD reports unregistered-co-change; run boundary-impact --base HEAD to see the Contexts this change touches\n"
            )
            .as_str()
        ),
        run.outcome()
    );
}

#[test]
fn a_change_inside_one_context_commits_without_a_note() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.mapped_contexts_with_a_cited_port();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/orders/port.py", "v1\nadded\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, ""), run.outcome());
}

#[test]
fn the_first_commit_runs_no_boundary_check() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let hook = sandbox.install(&sandbox.engine);
    sandbox.git(&["update-ref", "-d", "HEAD"]);
    let run = sandbox.commit_hook(&hook);
    assert_eq!((0, ""), run.outcome());
}

#[test]
fn a_boundary_check_that_fails_notes_it_and_commits() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.mapped_contexts_with_a_cited_port();
    let engine = sandbox.engine(
        "failing-boundary",
        &Tool::Replacing("boundary-impact", "exit 1"),
    );
    let hook = sandbox.install(&engine);
    sandbox.stage("src/orders/port.py", "v1\nadded\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            0,
            format!("{NOTE} boundary-impact against HEAD did not run; the commit goes ahead\n")
                .as_str()
        ),
        run.outcome()
    );
}

#[test]
fn a_boundary_check_without_a_verdict_notes_that_it_has_none() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.mapped_contexts_with_a_cited_port();
    let engine = sandbox.engine(
        "silent-boundary",
        &Tool::Replacing("boundary-impact", "echo '{}'; exit 0"),
    );
    let hook = sandbox.install(&engine);
    sandbox.stage("src/orders/port.py", "v1\nadded\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            0,
            format!(
                "{NOTE} boundary-impact against HEAD reports no verdict; run boundary-impact --base HEAD to see the Contexts this change touches\n"
            )
            .as_str()
        ),
        run.outcome()
    );
}

#[test]
fn an_existing_hook_is_kept_and_runs_before_the_citation_check() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    let backup = sandbox.existing_hook(
        "pre-commit",
        "#!/bin/sh\necho existing-hook-refused >&2\nexit 1\n",
        true,
    );
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/a.py", "changed\ntwo\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!((1, "existing-hook-refused\n"), run.outcome());
    assert_eq!(
        "#!/bin/sh\necho existing-hook-refused >&2\nexit 1\n",
        fs::read_to_string(backup).unwrap()
    );
}

#[test]
fn an_existing_hook_that_passes_lets_the_citation_check_run() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.cited_source();
    sandbox.existing_hook("pre-commit", "#!/bin/sh\nexit 0\n", true);
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("src/a.py", "changed\ntwo\n");
    let run = sandbox.commit_hook(&hook);
    assert_eq!(1, run.code);
    assert!(run.stderr.contains(REFUSED_STALE), "{}", run.stderr);
}

fn linked_worktree(sandbox: &Sandbox) -> PathBuf {
    let linked = sandbox.base.join("linked");
    sandbox.git(&["worktree", "add", "-q", &forward(&linked)]);
    linked
}

#[test]
fn an_existing_hook_refusing_in_a_linked_worktree_stops_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.existing_hook(
        "pre-commit",
        "#!/bin/sh\necho existing-hook-refused >&2\nexit 1\n",
        true,
    );
    let hook = sandbox.install(&sandbox.engine);
    let linked = linked_worktree(&sandbox);
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            cwd: Some(&linked),
            ..HookCall::default()
        },
    );
    assert_eq!((1, "existing-hook-refused\n"), run.outcome());
}

#[test]
fn a_hooks_directory_git_cannot_name_stops_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    let hook = sandbox.install(&sandbox.engine);
    sandbox.stage("notes/uncited.md", "anything\n");
    let shims = sandbox.git_shim_failing("hooks-path", "test \"$1\" = rev-parse");
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            shims: Some(&shims),
            ..HookCall::default()
        },
    );
    assert_eq!((1, ""), run.outcome());
}

#[test]
fn a_chained_hook_that_is_not_executable_stops_the_commit() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.existing_hook("pre-commit", "exit 0\n", false);
    let hook = sandbox.install(&sandbox.engine);
    let run = sandbox.commit_hook(&hook);
    assert_eq!(
        (
            1,
            "domain-memory pre-commit: the chained hook .git/hooks/pre-commit.domain-memory-existing is not executable, so it would not run; make it executable (chmod +x) or delete it\n"
        ),
        run.outcome()
    );
}

#[test]
fn installing_the_pre_push_hook_keeps_an_existing_hook_as_the_backup() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    let backup = sandbox.existing_hook("pre-push", "#!/bin/sh\nexit 0\n", true);
    let hook = sandbox.install_pre_push(&sandbox.engine);
    let content = fs::read_to_string(hook).unwrap();
    assert_eq!("#!/bin/sh\nexit 0\n", fs::read_to_string(backup).unwrap());
    assert!(content.contains("git rev-list $range -- \"$memory\""));
    assert!(content.contains("verify-git-governance"));
}

#[test]
fn an_existing_pre_push_hook_refusing_in_a_linked_worktree_stops_the_push() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.existing_hook(
        "pre-push",
        "#!/bin/sh\necho existing-push-refused >&2\nexit 1\n",
        true,
    );
    let hook = sandbox.install_pre_push(&sandbox.engine);
    let linked = linked_worktree(&sandbox);
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            cwd: Some(&linked),
            args: &["origin", "https://example.com/repo.git"],
            ..HookCall::default()
        },
    );
    assert_eq!((1, "existing-push-refused\n"), run.outcome());
}

#[test]
fn a_chained_hook_that_is_not_executable_stops_the_push() {
    let sandbox = Sandbox::seeded("SHA256:seed");
    sandbox.existing_hook("pre-push", "exit 0\n", false);
    let hook = sandbox.install_pre_push(&sandbox.engine);
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            args: &["origin", "https://example.com/repo.git"],
            ..HookCall::default()
        },
    );
    assert_eq!(1, run.code, "{}{}", run.stdout, run.stderr);
    assert!(
        run.stderr.contains(
            "domain-memory pre-push: the chained hook .git/hooks/pre-push.domain-memory-existing is not executable, so it would not run; make it executable (chmod +x) or delete it"
        ),
        "{}",
        run.stderr
    );
}

#[test]
fn an_existing_pre_push_hook_reading_stdin_does_not_disarm_the_check() {
    let sandbox = Sandbox::seeded("SHA256:NOBODY");
    sandbox.existing_hook("pre-push", "#!/bin/sh\ncat >/dev/null\nexit 0\n", true);
    let hook = sandbox.install_pre_push(&sandbox.engine);
    sandbox.write(
        "memory/registry/rules/extra.md",
        "---\ntype: Business Rule\n---\nS.\n",
    );
    sandbox.git(&["add", "memory"]);
    sandbox.git(&["commit", "-qm", "unsigned domain memory change"]);
    let head = sandbox.head();
    let parent = sandbox.git(&["rev-parse", "HEAD~1"]);
    let run = sandbox.run_hook(
        &hook,
        &HookCall {
            args: &["origin", "https://example.com/repo.git"],
            stdin: &format!("refs/heads/main {head} refs/heads/main {parent}\n"),
            ..HookCall::default()
        },
    );
    assert_ne!(0, run.code, "{}{}", run.stdout, run.stderr);
    assert!(
        format!("{}{}", run.stdout, run.stderr).contains(&format!(
            "Git commit signature is invalid: {head} carries no signature"
        )),
        "{}{}",
        run.stdout,
        run.stderr
    );
}

fn timeout_seen_by_the_tool(configured: Option<&str>, environment: &[(&str, &str)]) -> Run {
    let sandbox = Sandbox::seeded("SHA256:seed");
    let engine = sandbox.engine(
        "timeout-engine",
        &Tool::Script("echo \"timeout=$DOMAIN_MEMORY_GIT_TIMEOUT\" >&2\nexit 1"),
    );
    let hook = sandbox.install_pre_push(&engine);
    if let Some(value) = configured {
        sandbox.git(&["config", "domain-memory.git-timeout", value]);
    }
    let head = sandbox.head();
    sandbox.run_hook(
        &hook,
        &HookCall {
            args: &["origin", "https://example.com/repo.git"],
            stdin: &format!("refs/heads/main {head} refs/heads/main {EMPTY_SHA}\n"),
            env: environment,
            ..HookCall::default()
        },
    )
}

#[test]
fn the_pre_push_hook_takes_the_git_timeout_from_git_config() {
    let run = timeout_seen_by_the_tool(Some("42"), &[]);
    assert_eq!((1, "timeout=42\n"), run.outcome());
}

#[test]
fn the_git_timeout_from_the_environment_wins_over_git_config() {
    let run = timeout_seen_by_the_tool(Some("42"), &[("DOMAIN_MEMORY_GIT_TIMEOUT", "7")]);
    assert_eq!((1, "timeout=7\n"), run.outcome());
}

#[test]
fn the_pre_push_hook_passes_no_git_timeout_when_neither_is_set() {
    let run = timeout_seen_by_the_tool(None, &[]);
    assert_eq!((1, "timeout=\n"), run.outcome());
}

#[test]
fn a_push_of_several_signed_memory_commits_reports_them_once() {
    let sandbox = Sandbox::new();
    sandbox.signing_for_the_registry();
    let base = sandbox.head();
    sandbox.signed_commit_changing("memory/a.txt");
    let tip = sandbox.signed_commit_changing("memory/b.txt");
    let hook = sandbox.install_pre_push(&sandbox.engine);
    let run = sandbox.push_hook(
        &hook,
        &format!("refs/heads/main {tip} refs/heads/main {base}\n"),
    );
    assert_eq!(
        (0, "Git governance is valid for 2 commits."),
        (run.code, run.stdout.trim()),
        "{}",
        run.stderr
    );
}

#[test]
fn a_push_of_signed_commits_outside_the_memory_reports_nothing() {
    let sandbox = Sandbox::new();
    sandbox.signing_for_the_registry();
    sandbox.signed_commit_changing("memory/a.txt");
    let tip = sandbox.signed_commit_changing("memory/b.txt");
    let outside = sandbox.signed_commit_changing("src/c.txt");
    let hook = sandbox.install_pre_push(&sandbox.engine);
    let run = sandbox.push_hook(
        &hook,
        &format!("refs/heads/main {outside} refs/heads/main {tip}\n"),
    );
    assert_eq!((0, ""), (run.code, run.stdout.trim()), "{}", run.stderr);
}

#[test]
fn a_new_branch_skips_commits_the_remote_already_has() {
    let sandbox = Sandbox::new();
    let hook = sandbox.push_fixture(&sandbox.engine);
    let head = sandbox.commit_file("elsewhere/readme.txt");
    let run = sandbox.push_hook(&hook, &sandbox.new_branch(&head));
    assert_eq!(0, run.code, "{}{}", run.stdout, run.stderr);
}

#[test]
fn a_new_branch_still_verifies_its_own_unsigned_memory_commit() {
    let sandbox = Sandbox::new();
    let hook = sandbox.push_fixture(&sandbox.engine);
    let head = sandbox.commit_file("memory/registry/extra.json");
    let run = sandbox.push_hook(&hook, &sandbox.new_branch(&head));
    assert_ne!(0, run.code);
    assert!(
        format!("{}{}", run.stdout, run.stderr).contains("Git commit signature is invalid"),
        "{}{}",
        run.stdout,
        run.stderr
    );
}

#[test]
fn a_push_without_memory_commits_needs_no_executable() {
    let sandbox = Sandbox::new();
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.push_fixture(&engine);
    let head = sandbox.commit_file("elsewhere/readme.txt");
    let run = sandbox.push_hook(&hook, &sandbox.new_branch(&head));
    assert_eq!((0, ""), run.outcome());
}

#[test]
fn deleting_a_branch_that_holds_memory_commits_checks_nothing() {
    let sandbox = Sandbox::new();
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.push_fixture(&engine);
    let head = sandbox.commit_file("memory/registry/extra.json");
    let deleted = format!("(delete) {EMPTY_SHA} refs/heads/feature {head}\n");
    let run = sandbox.push_hook(&hook, &deleted);
    assert_eq!((0, ""), run.outcome());
}

#[test]
fn a_push_of_memory_commits_names_a_missing_executable_and_refuses() {
    let sandbox = Sandbox::new();
    let engine = sandbox.engine("no-engine", &Tool::Missing);
    let hook = sandbox.push_fixture(&engine);
    let head = sandbox.commit_file("memory/registry/extra.json");
    let run = sandbox.push_hook(&hook, &sandbox.new_branch(&head));
    assert_missing_executable(&run, "pre-push", &engine);
}
