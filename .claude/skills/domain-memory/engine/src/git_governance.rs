use std::path::{Path, PathBuf};

use crate::attestations::verify_git_signed_commit;
use crate::files::{rename_retrying, replace_error};
use crate::git;
use crate::json::{Object, Value};
use crate::policy::{review_governance, review_mode};
use crate::py;
use crate::store::shown_path;

const HOOK_MARKER: &str = "domain-memory-pre-push";

pub struct Readiness {
    pub status: &'static str,
    pub blocks: Vec<String>,
}

pub fn hooks_dir(repo_root: &Path) -> Result<PathBuf, String> {
    let located = git::run_git_text(repo_root, &["rev-parse", "--git-path", "hooks"])?;
    let joined = if located.code != 0 {
        repo_root.join(".git").join("hooks")
    } else {
        repo_root.join(py::strip(&located.stdout))
    };
    Ok(PathBuf::from(
        py::PurePath::new(&py::shown(&joined)).native(),
    ))
}

fn pre_push_installed(repo_root: &Path) -> Result<bool, String> {
    let hook = hooks_dir(repo_root)?.join("pre-push");
    if !hook.is_file() {
        return Ok(false);
    }
    let bytes = std::fs::read(&hook).map_err(|error| py::os_error(&error, &hook))?;
    Ok(py::utf8_text(&bytes)?.contains(HOOK_MARKER))
}

fn verifier(governance: &Object) -> Option<&str> {
    governance.get("verifier").and_then(Value::as_str)
}

fn signers(governance: &Object) -> Vec<String> {
    governance
        .get("authorized_signers")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(|signer| signer.as_str().map(str::to_string))
        .collect()
}

pub fn governance_readiness(registry_root: &Path, repo_root: &Path) -> Result<Readiness, String> {
    if review_mode(registry_root)? == "local-draft-only" {
        return Ok(Readiness {
            status: "working-memory",
            blocks: Vec::new(),
        });
    }
    let governance = review_governance(registry_root)?;
    let mut blocks = Vec::new();
    if verifier(&governance) == Some("git-signed-commit") {
        if signers(&governance).is_empty() {
            blocks.push("no authorized Git signers".to_string());
        }
        if !pre_push_installed(repo_root)? {
            blocks.push("Git HITL pre-push hook is not installed on this machine".to_string());
        }
    }
    Ok(Readiness {
        status: if blocks.is_empty() {
            "ready"
        } else {
            "needs-setup"
        },
        blocks,
    })
}

pub fn verify_git_governance(
    registry_root: &Path,
    repo_root: &Path,
    commits: &[String],
) -> Result<Vec<String>, String> {
    let governance = review_governance(registry_root)?;
    if verifier(&governance) != Some("git-signed-commit") {
        return Ok(vec![
            "Domain Memory policy does not select git-signed-commit".to_string(),
        ]);
    }
    if commits.is_empty() {
        return Ok(vec!["no commit to verify".to_string()]);
    }
    let authorized = signers(&governance);
    let mut errors = Vec::new();
    for commit in commits {
        for error in verify_git_signed_commit(commit, repo_root, registry_root, &authorized, false)?
        {
            errors.push(if error.contains(commit.as_str()) {
                error
            } else {
                format!("{commit}: {error}")
            });
        }
    }
    Ok(errors)
}

const EMPTY_SHA: &str = "0000000000000000000000000000000000000000";

fn tool_for_hook(name: &str) -> String {
    format!(
        r##"tool_for_hook() {{
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) tool="$engine/bin/linux-x86_64/domain-memory" ;;
    Darwin-*) tool="$engine/bin/macos-universal/domain-memory" ;;
    MINGW*-x86_64|MSYS*-x86_64|CYGWIN*-x86_64) tool="$engine/bin/windows-x86_64/domain-memory.exe" ;;
    *) tool="$engine/target/release/domain-memory" ;;
  esac
  test -x "$tool" && return 0
  echo "domain-memory {name}: no Domain Memory executable at $tool; build one with cargo build --release --manifest-path $engine/Cargo.toml" >&2
  return 1
}}
"##
    )
}

fn run_existing_hook(name: &str, invocation: &str) -> String {
    format!(
        r##"if [ -e "$existing" ]; then
  if [ ! -x "$existing" ]; then
    echo "domain-memory {name}: the chained hook $existing is not executable, so it would not run; make it executable (chmod +x) or delete it" >&2
    exit 1
  fi
  {invocation} || exit 1
fi
"##
    )
}

fn pre_push_body() -> String {
    let chained = run_existing_hook("pre-push", r##"printf '%s\n' "$refs" | "$existing" "$@""##);
    format!(
        r##"refs=$(cat)
{finder}tool=''
{chained}printf '%s\n' "$refs" | while read local_ref local_sha remote_ref remote_sha; do
  test -n "$local_sha" || continue
  test "$local_sha" = "{EMPTY_SHA}" && continue
  range="$local_sha --not --remotes"
  test "$remote_sha" = "{EMPTY_SHA}" || range=$remote_sha..$local_sha
  commits=$(git rev-list $range -- "$memory") || exit 1
  test -n "$commits" || continue
  test -n "$tool" || tool_for_hook || exit 1
  printf '%s\n' "$commits" | "$tool" verify-git-governance --registry-root "$memory" --repo-root . --stdin || exit 1
done || exit 1
"##,
        finder = tool_for_hook("pre-push")
    )
}

const PRE_COMMIT_CHECKS: &str = r##"changes=$(git -c core.quotepath=false diff --cached --name-status --find-renames --diff-filter=ACMRD) || exit 1
test -n "$changes" || exit 0
staged=$(printf '%s\n' "$changes" | cut -f2- | tr '\t' '\n')
printf '%s\n' "$staged" | while IFS= read -r path; do
  printf '"path": "%s"\nresource: "%s#L\npath: "%s"\npath: %s\n' "$path" "$path" "$path" "$path"
done | git grep -qF --untracked -f - -- "$memory/registry" "$memory/source-map.json"
case $? in
  0) ;;
  1) exit 0 ;;
  *) echo 'domain-memory pre-commit: could not search the Registry for the staged files' >&2; exit 1 ;;
esac
set --
while IFS= read -r path; do set -- "$@" --path "$path"; done <<EOF
$staged
EOF
tool_for_hook || exit 1
if ! report=$("$tool" verify-evidence --registry-root "$memory" --repo-root . --staged "$@"); then
  test -z "$DOMAIN_MEMORY_HOOK_VERBOSE" || printf '%s\n' "$report" >&2
  printf '%s\n' "$changes" | awk -F'\t' '$1 ~ /^R/ { print $2 " is renamed to " $3 " in this commit; run relocate-evidence --from " $2 " --to " $3 }' >&2
  echo "domain-memory pre-commit: a staged file no longer matches what the Registry cites from it; details: verify-evidence --registry-root $memory --repo-root . --staged $* prints the full report, as this hook does when DOMAIN_MEMORY_HOOK_VERBOSE is set" >&2
  exit 1
fi
git rev-parse -q --verify HEAD >/dev/null || exit 0
if ! impact=$("$tool" boundary-impact --registry-root "$memory" --repo-root . --base HEAD); then
  echo 'NOTE: domain-memory pre-commit: boundary-impact against HEAD did not run; the commit goes ahead' >&2
  exit 0
fi
verdict=$(printf '%s\n' "$impact" | sed -n 's/^  "verdict": "\(.*\)",$/\1/p')
test "$verdict" = no-cross-context-change || echo "NOTE: domain-memory pre-commit: boundary-impact against HEAD reports ${verdict:-no verdict}; run boundary-impact --base HEAD to see the Contexts this change touches" >&2
exit 0
"##;

fn pre_commit_body() -> String {
    format!(
        "{}{}{PRE_COMMIT_CHECKS}",
        tool_for_hook("pre-commit"),
        run_existing_hook("pre-commit", r##""$existing" "$@""##)
    )
}

fn engine_directory() -> Result<PathBuf, String> {
    let executable = std::env::current_exe().map_err(|error| error.to_string())?;
    executable
        .ancestors()
        .nth(3)
        .map(Path::to_path_buf)
        .ok_or_else(|| "the executable is not inside the engine directory".to_string())
}

#[cfg(unix)]
fn make_executable(hook: &Path) -> Result<(), String> {
    use std::os::unix::fs::PermissionsExt;
    let mut permissions = std::fs::metadata(hook)
        .map_err(|error| py::os_error(&error, hook))?
        .permissions();
    permissions.set_mode(permissions.mode() | 0o111);
    std::fs::set_permissions(hook, permissions).map_err(|error| py::os_error(&error, hook))
}

#[cfg(not(unix))]
fn make_executable(_hook: &Path) -> Result<(), String> {
    Ok(())
}

fn existing_hook_text(hook: &Path) -> Result<String, String> {
    let bytes = std::fs::read(hook).map_err(|error| py::open_error(&error, hook))?;
    py::utf8_text(&bytes)
}

fn install_hook(
    name: &str,
    registry_root: &Path,
    repo_root: &Path,
    body: &str,
) -> Result<PathBuf, String> {
    let hooks = hooks_dir(repo_root)?;
    if !hooks.is_dir() {
        return Err(format!(
            "Git hooks require an existing hooks directory at {}",
            py::shown(&hooks)
        ));
    }
    let relative = py::relative_to(
        &py::resolve_path(registry_root),
        &py::resolve_path(repo_root),
    )?;
    let relative = if relative.is_empty() { "." } else { &relative };
    let marker = format!("domain-memory-{name}");
    let hook = hooks.join(name);
    let backup_name = format!("{name}.domain-memory-existing");
    let backup = hooks.join(&backup_name);
    if hook.exists() && !existing_hook_text(&hook)?.contains(&marker) {
        if backup.exists() {
            return Err(format!(
                "existing Domain Memory {name} hook backup already exists"
            ));
        }
        rename_retrying(&hook, &backup).map_err(|error| replace_error(&error, &hook, &backup))?;
    }
    let header = format!(
        "#!/bin/sh\n# {marker}\nengine='{}'\nmemory='{relative}'\nexisting=\"$(git rev-parse --git-path hooks)/{backup_name}\" || exit 1\n\
         test -n \"$DOMAIN_MEMORY_GIT_TIMEOUT\" || DOMAIN_MEMORY_GIT_TIMEOUT=$(git config --get domain-memory.git-timeout) || unset DOMAIN_MEMORY_GIT_TIMEOUT\n\
         export DOMAIN_MEMORY_GIT_TIMEOUT\n",
        shown_path(&engine_directory()?, Some(repo_root)),
    );
    std::fs::write(&hook, format!("{header}{body}"))
        .map_err(|error| py::open_error(&error, &hook))?;
    make_executable(&hook)?;
    Ok(hook)
}

pub fn install_pre_push_hook(registry_root: &Path, repo_root: &Path) -> Result<PathBuf, String> {
    install_hook("pre-push", registry_root, repo_root, &pre_push_body())
}

pub fn install_pre_commit_hook(registry_root: &Path, repo_root: &Path) -> Result<PathBuf, String> {
    install_hook("pre-commit", registry_root, repo_root, &pre_commit_body())
}
