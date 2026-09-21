from __future__ import annotations

import json
import os
import re
import subprocess
from pathlib import Path
from typing import Any
from urllib.parse import urlparse
from urllib.request import Request, urlopen

from .common import completed_identifier, load_json


def github_api_path(pull_request: str) -> str:
    parsed = urlparse(pull_request)
    parts = [part for part in parsed.path.split("/") if part]
    if parsed.scheme != "https" or parsed.netloc != "github.com":
        raise ValueError("SCM attestation pull_request must be a GitHub pull request URL")
    if len(parts) != 4 or parts[2] != "pull" or not parts[3].isdigit():
        raise ValueError("SCM attestation pull_request must identify one pull request")
    return f"repos/{parts[0]}/{parts[1]}/pulls/{parts[3]}"


def github_json(url: str, token: str) -> Any:
    request = Request(
        url,
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {token}",
            "X-GitHub-Api-Version": "2022-11-28",
        },
    )
    with urlopen(request, timeout=15) as response:
        return json.loads(response.read().decode("utf-8"))


def verify_external_scm(
    value: dict[str, Any], token_env: str, require_checks: bool = True
) -> list[str]:
    if value.get("provider") != "github":
        return ["SCM governance verification currently supports GitHub only"]
    token = os.environ.get(token_env)
    if not token:
        return [f"SCM governance verification requires {token_env}"]
    try:
        endpoint = github_api_path(str(value.get("pull_request", "")))
        base = f"https://api.github.com/{endpoint}"
        pull = github_json(base, token)
        reviews = github_json(base + "/reviews", token)
        checks = (
            github_json(
                "https://api.github.com/repos/"
                + "/".join(endpoint.split("/")[1:3])
                + f"/commits/{value['commit']}/check-runs",
                token,
            )
            if require_checks
            else None
        )
    except (OSError, ValueError, json.JSONDecodeError) as error:
        return [f"SCM governance verification failed: {error}"]
    if not isinstance(pull, dict) or pull.get("state") != "closed" or not pull.get("merged_at"):
        return ["SCM pull request is not merged"]
    if pull.get("head", {}).get("sha") != value.get("commit"):
        return ["SCM pull request head does not match the attested commit"]
    latest_reviews: dict[str, str] = {}
    if isinstance(reviews, list):
        for review in reviews:
            user = review.get("user", {}).get("login") if isinstance(review, dict) else None
            state = review.get("state") if isinstance(review, dict) else None
            if isinstance(user, str) and isinstance(state, str):
                latest_reviews[user] = state
    if "APPROVED" not in latest_reviews.values():
        return ["SCM pull request has no current approval"]
    if not require_checks:
        return []
    check_runs = checks.get("check_runs") if isinstance(checks, dict) else None
    if not isinstance(check_runs, list) or not check_runs:
        return ["SCM attested commit has no check runs"]
    if any(
        not isinstance(run, dict)
        or run.get("status") != "completed"
        or run.get("conclusion") != "success"
        for run in check_runs
    ):
        return ["SCM attested commit has incomplete or unsuccessful checks"]
    return []


def verify_git_signed_commit(
    value: dict[str, Any], repo_root: Path, registry_root: Path, authorized_signers: list[str]
) -> list[str]:
    commit = value.get("commit")
    if not isinstance(commit, str):
        return ["Git commit attestation requires a commit"]
    verified = subprocess.run(
        ["git", "-C", str(repo_root), "verify-commit", "--raw", commit],
        capture_output=True, text=True, check=False,
    )
    if verified.returncode != 0:
        return ["Git commit signature is invalid"]
    output = verified.stdout + verified.stderr
    signers = set(re.findall(r"VALIDSIG ([0-9A-F]+)", output))
    signers.update(re.findall(r'Good "git" signature for (.+)', output))
    if not signers.intersection(authorized_signers):
        return ["Git commit signer is not authorized by Domain Memory policy"]
    changed = subprocess.run(
        ["git", "-C", str(repo_root), "diff-tree", "--no-commit-id", "--name-only", "-r", "--root", commit],
        capture_output=True, text=True, check=False,
    )
    root = registry_root.resolve().relative_to(repo_root.resolve()).as_posix()
    if changed.returncode != 0 or not any(
        path == root or path.startswith(root + "/") for path in changed.stdout.splitlines()
    ):
        return ["Git commit does not change Domain Memory files"]
    return []


def verify_scm_value(value: dict[str, Any], proposal: dict[str, Any]) -> list[str]:
    errors = []
    for key in ("provider", "commit", "status"):
        if not completed_identifier(value.get(key)):
            errors.append(f"SCM attestation requires {key}")
    if value.get("status") != "approved":
        errors.append("SCM attestation status must be approved")
    if value.get("provider") not in {"github", "git-signed-commit"}:
        errors.append("SCM attestation provider must be github or git-signed-commit")
    if value.get("provider") == "github":
        for key in ("pull_request", "checks_url"):
            if not isinstance(value.get(key), str) or not value[key].startswith("https://"):
                errors.append(f"SCM attestation requires an external {key} URL")
    commit = value.get("commit")
    if not isinstance(commit, str) or len(commit) != 40 or any(c not in "0123456789abcdef" for c in commit):
        errors.append("SCM attestation requires a pinned 40-character commit")
    if value.get("proposal_revision") != proposal.get("proposal_revision"):
        errors.append("SCM attestation proposal revision does not match")
    if value.get("base_registry_revision") != proposal.get("base_registry_revision"):
        errors.append("SCM attestation Registry revision does not match")
    return errors


def verify_scm(path: Path, proposal: dict[str, Any]) -> list[str]:
    return verify_scm_value(load_json(path), proposal)
