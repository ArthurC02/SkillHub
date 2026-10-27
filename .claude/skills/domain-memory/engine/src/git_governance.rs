use std::path::{Path, PathBuf};

use crate::attestations::verify_git_signed_commit;
use crate::git;
use crate::json::{Object, Value};
use crate::policy::{review_governance, review_mode};
use crate::py;

const HOOK_MARKER: &str = "domain-memory-pre-push";

pub struct Readiness {
    pub status: &'static str,
    pub blocks: Vec<String>,
}

fn hooks_dir(repo_root: &Path) -> Result<PathBuf, String> {
    let located = git::run_git_text(repo_root, &["rev-parse", "--git-path", "hooks"])?;
    if located.code != 0 {
        return Ok(repo_root.join(".git").join("hooks"));
    }
    Ok(repo_root.join(py::strip(&located.stdout)))
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
        for error in verify_git_signed_commit(commit, repo_root, registry_root, &authorized)? {
            errors.push(if error.contains(commit.as_str()) {
                error
            } else {
                format!("{commit}: {error}")
            });
        }
    }
    Ok(errors)
}
