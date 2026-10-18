from __future__ import annotations

import subprocess
from pathlib import Path

from .attestations import verify_git_signed_commit
from .common import replace_path
from .policy import review_governance, review_mode

HOOK_MARKER = "domain-memory-pre-push"
EMPTY_SHA = "0" * 40


def inside_repo(path: Path, repo_root: Path) -> str:
    resolved = path.resolve()
    try:
        return resolved.relative_to(repo_root.resolve()).as_posix()
    except ValueError:
        return resolved.as_posix()


def hooks_dir(repo_root: Path) -> Path:
    result = subprocess.run(
        ["git", "-C", str(repo_root), "rev-parse", "--git-path", "hooks"],
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
    )
    if result.returncode != 0:
        return repo_root / ".git" / "hooks"
    reported = Path(result.stdout.strip())
    return reported if reported.is_absolute() else repo_root / reported


def governance_readiness(registry_root: Path, repo_root: Path) -> dict[str, object]:
    mode = review_mode(registry_root)
    if mode == "local-draft-only":
        return {"status": "working-memory", "blocks": []}
    governance = review_governance(registry_root)
    blocks = []
    if governance["verifier"] == "git-signed-commit":
        if not governance["authorized_signers"]:
            blocks.append("no authorized Git signers")
        hook = hooks_dir(repo_root) / "pre-push"
        if not hook.is_file() or HOOK_MARKER not in hook.read_text(encoding="utf-8"):
            blocks.append("Git HITL pre-push hook is not installed on this machine")
    return {"status": "needs-setup" if blocks else "ready", "blocks": blocks}


def verify_git_governance(registry_root: Path, repo_root: Path, commits: list[str]) -> list[str]:
    governance = review_governance(registry_root)
    if governance["verifier"] != "git-signed-commit":
        return ["Domain Memory policy does not select git-signed-commit"]
    if not commits:
        return ["no commit to verify"]
    return [
        error if commit in error else f"{commit}: {error}"
        for commit in commits
        for error in verify_git_signed_commit(
            {"commit": commit}, repo_root, registry_root, governance["authorized_signers"]
        )
    ]


def _install_hook(name: str, registry_root: Path, repo_root: Path, script: Path, body: str) -> Path:
    hooks = hooks_dir(repo_root)
    if not hooks.is_dir():
        raise ValueError(f"Git hooks require an existing hooks directory at {hooks}")
    relative = registry_root.resolve().relative_to(repo_root.resolve()).as_posix()
    marker = f"domain-memory-{name}"
    hook = hooks / name
    backup = hooks / f"{name}.domain-memory-existing"
    if hook.exists() and marker not in hook.read_text(encoding="utf-8"):
        if backup.exists():
            raise ValueError(f"existing Domain Memory {name} hook backup already exists")
        replace_path(hook, backup)
    header = (
        "#!/bin/sh\n"
        f"# {marker}\n"
        f"tool='{inside_repo(script, repo_root)}'\n"
        f"memory='{relative}'\n"
        f"existing='{inside_repo(backup, repo_root)}'\n"
    )
    hook.write_text(header + body, encoding="utf-8", newline="\n")
    hook.chmod(hook.stat().st_mode | 0o111)
    return hook


def _python_for_tool(name: str) -> str:
    return (
        "python_for_tool() {\n"
        "  for candidate in python3 python; do\n"
        "    \"$candidate\" -c 'import sys; sys.exit(sys.version_info < (3, 10))' 2>/dev/null && echo \"$candidate\" && return 0\n"
        "  done\n"
        f"  echo 'domain-memory {name}: no Python 3.10 or later on PATH as python3 or python' >&2\n"
        "  return 1\n"
        "}\n"
    )


def _run_existing_hook(name: str, invocation: str) -> str:
    return (
        "if [ -e \"$existing\" ]; then\n"
        "  if [ ! -x \"$existing\" ]; then\n"
        f"    echo \"domain-memory {name}: the chained hook $existing is not executable, so it would not run; make it executable (chmod +x) or delete it\" >&2\n"
        "    exit 1\n"
        "  fi\n"
        f"  {invocation} || exit 1\n"
        "fi\n"
    )


def install_pre_push_hook(registry_root: Path, repo_root: Path, script: Path) -> Path:
    body = (
        "refs=$(cat)\n"
        + _python_for_tool("pre-push")
        + "py=''\n"
        + _run_existing_hook("pre-push", "printf '%s\\n' \"$refs\" | \"$existing\" \"$@\"")
        + "printf '%s\\n' \"$refs\" | while read local_ref local_sha remote_ref remote_sha; do\n"
        "  test -n \"$local_sha\" || continue\n"
        f"  test \"$local_sha\" = \"{EMPTY_SHA}\" && continue\n"
        "  range=\"$local_sha --not --remotes\"\n"
        f"  test \"$remote_sha\" = \"{EMPTY_SHA}\" || range=$remote_sha..$local_sha\n"
        "  commits=$(git rev-list $range -- \"$memory\") || exit 1\n"
        "  test -n \"$commits\" || continue\n"
        "  test -n \"$py\" || py=$(python_for_tool) || exit 1\n"
        "  printf '%s\\n' \"$commits\" | \"$py\" \"$tool\" verify-git-governance --registry-root \"$memory\" --repo-root . --stdin || exit 1\n"
        "done || exit 1\n"
    )
    return _install_hook("pre-push", registry_root, repo_root, script, body)


def install_pre_commit_hook(registry_root: Path, repo_root: Path, script: Path) -> Path:
    body = (
        _python_for_tool("pre-commit")
        + _run_existing_hook("pre-commit", "\"$existing\" \"$@\"")
        + "staged=$(git -c core.quotepath=false diff --cached --name-only --diff-filter=ACMR) || exit 1\n"
        "test -n \"$staged\" || exit 0\n"
        "printf '%s\\n' \"$staged\" | git grep -qF --untracked -f - -- \"$memory\"\n"
        "case $? in\n"
        "  0) ;;\n"
        "  1) exit 0 ;;\n"
        "  *) echo 'domain-memory pre-commit: could not search the Registry for the staged files' >&2; exit 1 ;;\n"
        "esac\n"
        "set --\n"
        "while IFS= read -r path; do set -- \"$@\" --path \"$path\"; done <<EOF\n"
        "$staged\n"
        "EOF\n"
        "py=$(python_for_tool) || exit 1\n"
        "if ! report=$(\"$py\" \"$tool\" verify-evidence --registry-root \"$memory\" --repo-root . \"$@\" 2>&1); then\n"
        "  printf '%s\\n' \"$report\" >&2\n"
        "  echo 'domain-memory pre-commit: a staged file no longer matches what the Registry cites from it' >&2\n"
        "  exit 1\n"
        "fi\n"
        "git rev-parse -q --verify HEAD >/dev/null || exit 0\n"
        "if ! impact=$(\"$py\" \"$tool\" boundary-impact --registry-root \"$memory\" --repo-root . --base HEAD); then\n"
        "  echo 'NOTE: domain-memory pre-commit: boundary-impact against HEAD did not run; the commit goes ahead' >&2\n"
        "  exit 0\n"
        "fi\n"
        "verdict=$(printf '%s\\n' \"$impact\" | sed -n 's/^  \"verdict\": \"\\(.*\\)\",$/\\1/p')\n"
        "test \"$verdict\" = no-cross-context-change || echo \"NOTE: domain-memory pre-commit: boundary-impact against HEAD"
        " reports ${verdict:-no verdict}; run boundary-impact --base HEAD to see the Contexts this change touches\" >&2\n"
        "exit 0\n"
    )
    return _install_hook("pre-commit", registry_root, repo_root, script, body)
