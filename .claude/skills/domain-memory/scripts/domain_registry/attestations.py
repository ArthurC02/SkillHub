from __future__ import annotations

import base64
import json
import os
import re
import subprocess
from pathlib import Path
from typing import Any
from urllib.error import HTTPError
from urllib.parse import quote, urlparse
from urllib.request import Request, urlopen

HTTP_NOT_FOUND = 404

from .common import completed_identifier, load_json


def github_api_path(pull_request: str) -> str:
    parsed = urlparse(pull_request)
    parts = [part for part in parsed.path.split("/") if part]
    if parsed.scheme != "https" or parsed.netloc != "github.com":
        raise ValueError("SCM attestation pull_request must be a GitHub pull request URL")
    if len(parts) != 4 or parts[2] != "pull" or not parts[3].isdigit():  # noqa: PLR2004
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


def verify_external_scm(  # noqa: C901, PLR0911
    value: dict[str, Any], token_env: str, require_checks: bool = True  # noqa: FBT001, FBT002
) -> list[str]:
    if value.get("provider") != "github":
        return ["SCM governance verification currently supports GitHub only"]
    token = os.environ.get(token_env)
    if not token:
        return [f"SCM governance verification requires {token_env}"]
    try:
        endpoint = github_api_path(str(value.get("pull_request", "")))
        api = "https://api.github.com/"
        base = api + endpoint
        repository = "/".join(endpoint.split("/")[:3])
        pull = github_json(base, token)
        reviews = github_json(base + "/reviews", token)
        checks = (
            github_json(f"{api}{repository}/commits/{value['commit']}/check-runs", token)
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
    for review in reviews if isinstance(reviews, list) else []:
        if not isinstance(review, dict):
            continue
        user = review.get("user", {}).get("login")
        state = review.get("state")
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


def _git(repo_root: Path, *arguments: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(repo_root), *arguments],
        capture_output=True, text=True, encoding="utf-8", errors="replace", check=False,
    )


def verify_git_signed_commit(
    value: dict[str, Any], repo_root: Path, registry_root: Path, authorized_signers: list[str]
) -> list[str]:
    commit = value.get("commit")
    if not isinstance(commit, str):
        return ["Git commit attestation requires a commit"]
    verified = _git(repo_root, "verify-commit", "--raw", commit)
    if verified.returncode != 0:
        return ["Git commit signature is invalid"]
    output = verified.stdout + verified.stderr
    signers = set(re.findall(r"VALIDSIG ([0-9A-F]+)", output))
    signers.update(re.findall(r'Good "git" signature for (\S+)', output))
    signers.update(re.findall(r"key (SHA256:[A-Za-z0-9+/=]+)", output))
    if not signers.intersection(authorized_signers):
        return ["Git commit signer is not authorized by Domain Memory policy"]
    changed = _git(
        repo_root, "diff-tree", "--no-commit-id", "--name-only", "-r", "--root", commit
    )
    root = registry_root.resolve().relative_to(repo_root.resolve()).as_posix()
    if changed.returncode != 0 or not any(
        path == root or path.startswith(root + "/") for path in changed.stdout.splitlines()
    ):
        return ["Git commit does not change Domain Memory files"]
    return []


OUTSIDE_THE_REPOSITORY = "the Change Package must be inside the repository, so the attested commit can carry it"


def package_path_in(repo_root: Path, package_root: Path) -> str | None:
    try:
        return package_root.resolve().relative_to(repo_root.resolve()).as_posix()
    except ValueError:
        return None


def carried_proposal_errors(
    committed_text: str | None, relative: str, proposal: dict[str, Any]
) -> list[str]:
    if committed_text is None:
        return [
            f"Git commit does not carry this Change Package at {relative}; the attested commit "
            "must contain the package, so its review covers this proposal"
        ]
    try:
        committed = json.loads(committed_text)
    except json.JSONDecodeError:
        return ["Git commit carries an unreadable domain-change-proposal.json"]
    identity = ("proposal_id", "proposal_revision")
    if not isinstance(committed, dict) or any(
        committed.get(key) != proposal.get(key) for key in identity
    ):
        return [
            "Git commit carries another proposal or revision; attest a commit with this "
            "proposal_id and proposal_revision"
        ]
    return []


def commit_carries_proposal(
    repo_root: Path, commit: str, package_root: Path, proposal: dict[str, Any]
) -> list[str]:
    relative = package_path_in(repo_root, package_root)
    if relative is None:
        return [OUTSIDE_THE_REPOSITORY]
    shown = _git(repo_root, "show", f"{commit}:{relative}/domain-change-proposal.json")
    return carried_proposal_errors(shown.stdout if shown.returncode == 0 else None, relative, proposal)


def github_commit_carries_proposal(
    value: dict[str, Any], token_env: str, repo_root: Path, package_root: Path,
    proposal: dict[str, Any],
) -> list[str]:
    relative = package_path_in(repo_root, package_root)
    if relative is None:
        return [OUTSIDE_THE_REPOSITORY]
    token = os.environ.get(token_env)
    if not token:
        return [f"SCM governance verification requires {token_env}"]
    try:
        text = _proposal_at_pull_request_commit(value, token, relative)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        return [f"SCM governance verification failed: {error}"]
    return carried_proposal_errors(text, relative, proposal)


def _proposal_at_pull_request_commit(value: dict[str, Any], token: str, relative: str) -> str | None:
    repository = "/".join(github_api_path(str(value.get("pull_request", ""))).split("/")[:3])
    url = (
        f"https://api.github.com/{repository}/contents/"
        f"{quote(relative)}/domain-change-proposal.json?ref={value.get('commit')}"
    )
    try:
        content = github_json(url, token)
    except HTTPError as error:
        if error.code == HTTP_NOT_FOUND:
            return None
        raise
    if not isinstance(content, dict) or content.get("encoding") != "base64":
        raise ValueError("the provider returned no file content")
    return base64.b64decode(content.get("content", "")).decode("utf-8", "replace")


AUDITED_STATUSES = {"approved", "applied"}
UNVERIFIED_PREFIX = "SCM governance verification"


def _attestation_verdict(
    repo_root: Path, package: Path, proposal: dict[str, Any], attestation: Any, token_env: str
) -> tuple[str, str]:
    if not isinstance(attestation, dict) or not isinstance(attestation.get("commit"), str):
        return "not carried", "the package has no scm_attestation commit"
    commit = attestation["commit"]
    if _git(repo_root, "cat-file", "-e", f"{commit}^{{commit}}").returncode == 0:
        errors = commit_carries_proposal(repo_root, commit, package, proposal)
    elif attestation.get("provider") == "github":
        errors = github_commit_carries_proposal(attestation, token_env, repo_root, package, proposal)
    else:
        return "unconfirmed", f"commit {commit} is not in this clone; fetch the full history"
    if not errors:
        return "carried", commit
    if errors[0].startswith(UNVERIFIED_PREFIX):
        return "unconfirmed", errors[0]
    return "not carried", errors[0]


def audit_attestations(repo_root: Path, changes_root: Path, token_env: str) -> list[dict[str, str]]:
    findings = []
    for proposal_file in sorted(changes_root.glob("*/domain-change-proposal.json")):
        package = proposal_file.parent
        proposal = load_json(proposal_file)
        if proposal.get("status") not in AUDITED_STATUSES:
            continue
        evidence_file = package / "evidence-bundle.json"
        attestation = load_json(evidence_file).get("scm_attestation") if evidence_file.is_file() else None
        verdict, detail = _attestation_verdict(repo_root, package, proposal, attestation, token_env)
        findings.append({
            "package": package_path_in(repo_root, package) or str(package),
            "proposal_id": str(proposal.get("proposal_id")),
            "status": str(proposal.get("status")),
            "verdict": verdict,
            "detail": detail,
        })
    return findings


def verify_scm_value(value: dict[str, Any], proposal: dict[str, Any]) -> list[str]:
    errors = [
        f"SCM attestation requires {key}"
        for key in ("provider", "commit", "status")
        if not completed_identifier(value.get(key))
    ]
    if value.get("status") != "approved":
        errors.append("SCM attestation status must be approved")
    if value.get("provider") not in {"github", "git-signed-commit"}:
        errors.append("SCM attestation provider must be github or git-signed-commit")
    if value.get("provider") == "github":
        errors.extend(
            f"SCM attestation requires an external {key} URL"
            for key in ("pull_request", "checks_url")
            if not isinstance(value.get(key), str) or not value[key].startswith("https://")
        )
    commit = value.get("commit")
    if not isinstance(commit, str) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        errors.append("SCM attestation requires a pinned 40-character commit")
    if value.get("proposal_revision") != proposal.get("proposal_revision"):
        errors.append("SCM attestation proposal revision does not match")
    if value.get("base_registry_revision") != proposal.get("base_registry_revision"):
        errors.append("SCM attestation Registry revision does not match")
    return errors


def verify_scm(path: Path, proposal: dict[str, Any]) -> list[str]:
    return verify_scm_value(load_json(path), proposal)
