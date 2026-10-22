from __future__ import annotations

import base64
import json
import os
import re
import subprocess
from pathlib import Path
from typing import Any, NamedTuple
from urllib.parse import quote, urlparse

HTTP_NOT_FOUND = 404

from .common import (
    EVIDENCE_FILE,
    PROPOSAL_FILE,
    completed_identifier,
    load_json,
    readable_packages,
)
from .git import read_objects, run_bounded
from .revision import valid_commit


def github_api_path(pull_request: str) -> str:
    parsed = urlparse(pull_request)
    parts = [part for part in parsed.path.split("/") if part]
    if parsed.scheme != "https" or parsed.netloc != "github.com":
        raise ValueError("SCM attestation pull_request must be a GitHub pull request URL")
    if len(parts) != 4 or parts[2] != "pull" or not parts[3].isdigit():  # noqa: PLR2004
        raise ValueError("SCM attestation pull_request must identify one pull request")
    return f"repos/{parts[0]}/{parts[1]}/pulls/{parts[3]}"


def github_page(url: str, token: str) -> tuple[Any, str | None]:
    from urllib.request import Request, urlopen

    request = Request(
        url,
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {token}",
            "X-GitHub-Api-Version": "2022-11-28",
        },
    )
    with urlopen(request, timeout=15) as response:
        body = json.loads(response.read().decode("utf-8"))
        return body, next_page_url(response.headers.get("Link"))


def next_page_url(link_header: str | None) -> str | None:
    match = re.search(r'<([^>]+)>;\s*rel="next"', link_header or "")
    return match.group(1) if match else None


def github_json(url: str, token: str) -> Any:
    return github_page(url, token)[0]


def github_list(url: str, token: str, items_key: str | None = None) -> list[Any]:
    items: list[Any] = []
    page_url: str | None = f"{url}?per_page=100"
    while page_url:
        body, page_url = github_page(page_url, token)
        page = body.get(items_key) if items_key and isinstance(body, dict) else body
        if not isinstance(page, list):
            raise ValueError(f"the provider returned no list at {url}")
        items.extend(page)
    return items


def _pull_request_state(
    value: dict[str, Any], token: str, require_checks: bool  # noqa: FBT001
) -> tuple[Any, list[Any], list[Any] | None]:
    endpoint = github_api_path(str(value.get("pull_request", "")))
    api = "https://api.github.com/"
    base = api + endpoint
    repository = "/".join(endpoint.split("/")[:3])
    pull = github_json(base, token)
    reviews = github_list(base + "/reviews", token)
    checks = (
        github_list(f"{api}{repository}/commits/{value['commit']}/check-runs", token, "check_runs")
        if require_checks
        else None
    )
    return pull, reviews, checks


def _pull_request_error(pull: Any, commit: Any) -> str | None:
    if not isinstance(pull, dict) or pull.get("state") != "closed" or not pull.get("merged_at"):
        return "SCM pull request is not merged"
    if (pull.get("head") or {}).get("sha") != commit:
        return "SCM pull request head does not match the attested commit"
    return None


def _has_current_approval(reviews: list[Any]) -> bool:
    latest_reviews: dict[str, str] = {}
    for review in reviews:
        if not isinstance(review, dict):
            continue
        user = (review.get("user") or {}).get("login")
        state = review.get("state")
        if isinstance(user, str) and isinstance(state, str):
            latest_reviews[user] = state
    return "APPROVED" in latest_reviews.values()


def _checks_error(check_runs: list[Any] | None) -> str | None:
    if not check_runs:
        return "SCM attested commit has no check runs"
    if any(
        not isinstance(run, dict)
        or run.get("status") != "completed"
        or run.get("conclusion") != "success"
        for run in check_runs
    ):
        return "SCM attested commit has incomplete or unsuccessful checks"
    return None


def verify_external_scm(
    value: dict[str, Any], token_env: str, require_checks: bool = True  # noqa: FBT001, FBT002
) -> list[str]:
    if value.get("provider") != "github":
        return ["SCM governance verification currently supports GitHub only"]
    token = os.environ.get(token_env)
    if not token:
        return [f"SCM governance verification requires {token_env}"]
    try:
        pull, reviews, checks = _pull_request_state(value, token, require_checks)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        return [f"SCM governance verification failed: {error}"]
    error = _pull_request_error(pull, value.get("commit"))
    if error is None and not _has_current_approval(reviews):
        error = "SCM pull request has no current approval"
    if error is None and require_checks:
        error = _checks_error(checks)
    return [error] if error else []


def _git(repo_root: Path, *arguments: str) -> subprocess.CompletedProcess[str]:
    return run_bounded(["git", "-C", str(repo_root), *arguments], errors="replace")


UNRUNNABLE_PROGRAM = ("cannot spawn ", "cannot run ")


def _invalid_signature(repo_root: Path, commit: str, verifier_stderr: str) -> str:
    reasons = [line.removeprefix("error: ") for line in verifier_stderr.splitlines() if line.strip()]
    unrunnable = next((reason for reason in reasons if reason.startswith(UNRUNNABLE_PROGRAM)), None)
    if unrunnable:
        return (
            f"Git commit signature was not checked: {unrunnable}. Install the signing program, "
            "or point gpg.ssh.program or gpg.program at it, and verify again"
        )
    header = _git(repo_root, "cat-file", "commit", commit).stdout.partition("\n\n")[0]
    if "\ngpgsig" in header:
        reason = f" ({reasons[-1]})" if reasons else ""
        return f"Git commit signature is invalid: {commit} is signed, but the signature does not verify{reason}"
    return (
        f"Git commit signature is invalid: {commit} carries no signature. A rebase, amend or "
        "cherry-pick writes new commits unsigned unless told to sign; take in upstream work with a "
        "merge (git pull --no-rebase), which keeps signed and attested commits as they are, and "
        "an attestation naming a commit that was rewritten has to be made again"
    )


def verify_git_signed_commit(
    value: dict[str, Any], repo_root: Path, registry_root: Path, authorized_signers: list[str]
) -> list[str]:
    commit = value.get("commit")
    if not isinstance(commit, str):
        return ["Git commit attestation requires a commit"]
    verified = _git(repo_root, "verify-commit", "--raw", commit)
    if verified.returncode != 0:
        return [_invalid_signature(repo_root, commit, verified.stderr)]
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
    shown = _git(repo_root, "show", f"{commit}:{relative}/{PROPOSAL_FILE}")
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
        f"{quote(relative)}/{PROPOSAL_FILE}?ref={value.get('commit')}"
    )
    from urllib.error import HTTPError

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


def _read_objects(repo_root: Path, names: list[str]) -> dict[str, bytes]:
    try:
        return read_objects(repo_root, names)
    except subprocess.CalledProcessError:
        return {}


def _proposal_object(commit: str, relative: str) -> str:
    return f"{commit}:{relative}/{PROPOSAL_FILE}"


def _attested_commit(attestation: Any) -> str | None:
    commit = attestation.get("commit") if isinstance(attestation, dict) else None
    return commit if isinstance(commit, str) else None


class AuditedPackage(NamedTuple):
    package: Path
    proposal: dict[str, Any]
    attestation: Any


def _attestation_verdict(
    repo_root: Path, audited: AuditedPackage, token_env: str, objects: dict[str, bytes]
) -> tuple[str, str]:
    package, proposal, attestation = audited
    commit = _attested_commit(attestation)
    if commit is None:
        return "not carried", "the package has no scm_attestation commit"
    relative = package_path_in(repo_root, package)
    if f"{commit}^{{commit}}" in objects:
        if relative is None:
            errors = [OUTSIDE_THE_REPOSITORY]
        else:
            shown = objects.get(_proposal_object(commit, relative))
            text = None if shown is None else shown.decode("utf-8", "replace")
            errors = carried_proposal_errors(text, relative, proposal)
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
    if not changes_root.is_dir():
        raise ValueError(f"no Change Packages directory at {changes_root}; pass the directory that holds them")
    audited = []
    for package, proposal in readable_packages(changes_root):
        if proposal.get("status") not in AUDITED_STATUSES:
            continue
        evidence_file = package / EVIDENCE_FILE
        attestation = load_json(evidence_file).get("scm_attestation") if evidence_file.is_file() else None
        audited.append(AuditedPackage(package, proposal, attestation))
    names = []
    for package, _, attestation in audited:
        commit = _attested_commit(attestation)
        relative = package_path_in(repo_root, package)
        if commit is not None:
            names.append(f"{commit}^{{commit}}")
            if relative is not None:
                names.append(_proposal_object(commit, relative))
    objects = _read_objects(repo_root, names)
    findings = []
    for entry in audited:
        verdict, detail = _attestation_verdict(repo_root, entry, token_env, objects)
        findings.append({
            "package": package_path_in(repo_root, entry.package) or str(entry.package),
            "proposal_id": str(entry.proposal.get("proposal_id")),
            "status": str(entry.proposal.get("status")),
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
    if not valid_commit(value.get("commit")):
        errors.append("SCM attestation requires a pinned 40-character commit")
    if value.get("proposal_revision") != proposal.get("proposal_revision"):
        errors.append("SCM attestation proposal revision does not match")
    if value.get("base_registry_revision") != proposal.get("base_registry_revision"):
        errors.append("SCM attestation Registry revision does not match")
    return errors


def verify_scm(path: Path, proposal: dict[str, Any]) -> list[str]:
    return verify_scm_value(load_json(path), proposal)
