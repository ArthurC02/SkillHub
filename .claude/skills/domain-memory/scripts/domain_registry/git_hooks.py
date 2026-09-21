from __future__ import annotations

from pathlib import Path

from .attestations import verify_git_signed_commit
from .policy import review_governance, review_mode


def governance_readiness(registry_root: Path, repo_root: Path) -> dict[str, object]:
    mode = review_mode(registry_root)
    if mode == "local-draft-only":
        return {"status": "working-memory", "blocks": []}
    governance = review_governance(registry_root)
    blocks = []
    if governance["verifier"] == "git-signed-commit":
        if not governance["authorized_signers"]:
            blocks.append("no authorized Git signers")
        hook = repo_root / ".git" / "hooks" / "pre-push"
        if not hook.is_file() or "domain-memory-pre-push" not in hook.read_text(encoding="utf-8"):
            blocks.append("Git HITL pre-push hook is not installed on this machine")
    return {"status": "ready" if not blocks else "needs-setup", "blocks": blocks}


def verify_git_governance(registry_root: Path, repo_root: Path, commit: str) -> list[str]:
    governance = review_governance(registry_root)
    if governance["verifier"] != "git-signed-commit":
        return ["Domain Memory policy does not select git-signed-commit"]
    return verify_git_signed_commit(
        {"commit": commit}, repo_root, registry_root, governance["authorized_signers"]
    )


def install_pre_push_hook(registry_root: Path, repo_root: Path, script: Path) -> Path:
    hooks = repo_root / ".git" / "hooks"
    if not hooks.is_dir():
        raise ValueError("Git hooks require a repository with a .git/hooks directory")
    relative = registry_root.resolve().relative_to(repo_root.resolve()).as_posix()
    hook = hooks / "pre-push"
    backup = hooks / "pre-push.domain-memory-existing"
    if hook.exists() and "domain-memory-pre-push" not in hook.read_text(encoding="utf-8"):
        if backup.exists():
            raise ValueError("existing Domain Memory pre-push hook backup already exists")
        hook.replace(backup)
    hook.write_text(
        "#!/bin/sh\n"
        "# domain-memory-pre-push\n"
        f"tool='{script.resolve().as_posix()}'\n"
        f"memory='{relative}'\n"
        f"existing='{backup.as_posix()}'\n"
        "test ! -x \"$existing\" || \"$existing\" \"$@\" || exit 1\n"
        "while read local_ref local_sha remote_ref remote_sha; do\n"
        "  test \"$local_sha\" = \"0000000000000000000000000000000000000000\" && continue\n"
        "  range=$local_sha\n"
        "  test \"$remote_sha\" = \"0000000000000000000000000000000000000000\" || range=$remote_sha..$local_sha\n"
        "  for commit in $(git rev-list $range -- \"$memory\"); do\n"
        "    python \"$tool\" verify-git-governance --registry-root \"$memory\" --repo-root . --commit \"$commit\" || exit 1\n"
        "  done\n"
        "done\n",
        encoding="utf-8",
    )
    return hook
