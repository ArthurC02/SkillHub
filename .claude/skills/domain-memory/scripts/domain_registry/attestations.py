from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path
from typing import Any, NamedTuple

from .common import (
    inside,
    EVIDENCE_FILE,
    PROPOSAL_FILE,
    completed_identifier,
    load_json,
    readable_packages,
)
from .git import read_objects, run_git
from .revision import valid_commit


UNRUNNABLE_PROGRAM = ("cannot spawn ", "cannot run ")


def _invalid_signature(repo_root: Path, commit: str, verifier_stderr: str) -> str:
    reasons = [line.removeprefix("error: ") for line in verifier_stderr.splitlines() if line.strip()]
    unrunnable = next((reason for reason in reasons if reason.startswith(UNRUNNABLE_PROGRAM)), None)
    if unrunnable:
        return (
            f"Git commit signature was not checked: {unrunnable}. Install the signing program, "
            "or point gpg.ssh.program or gpg.program at it, and verify again"
        )
    header = run_git(repo_root, "cat-file", "commit", commit).stdout.partition("\n\n")[0]
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
    value: dict[str, Any], repo_root: Path, registry_root: Path, authorized_signers: list[str],
    distinct_signer: bool = False,  # noqa: FBT001, FBT002
) -> list[str]:
    commit = value.get("commit")
    if not isinstance(commit, str):
        return ["Git commit attestation requires a commit"]
    verified = run_git(repo_root, "verify-commit", "--raw", commit)
    if verified.returncode != 0:
        return [_invalid_signature(repo_root, commit, verified.stderr)]
    output = verified.stdout + verified.stderr
    signers = set(re.findall(r"VALIDSIG ([0-9A-F]+)", output))
    signers.update(re.findall(r'Good "git" signature for (\S+)', output))
    signers.update(re.findall(r"key (SHA256:[A-Za-z0-9+/=]+)", output))
    if not signers.intersection(authorized_signers):
        return ["Git commit signer is not authorized by Domain Memory policy"]
    if distinct_signer:
        same = _author_is_signer(repo_root, commit)
        if same:
            return [same]
    changed = run_git(
        repo_root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-m", "--first-parent", "--root", commit
    )
    root = registry_root.resolve().relative_to(repo_root.resolve()).as_posix()
    if changed.returncode != 0 or not any(
        path == root or path.startswith(root + "/") for path in changed.stdout.splitlines()
    ):
        return ["Git commit does not change Domain Memory files"]
    return []


def _author_is_signer(repo_root: Path, commit: str) -> str | None:
    identities = run_git(repo_root, "log", "-1", "--format=%ae%n%GS", commit).stdout.splitlines()
    author = identities[0].strip() if identities else ""
    signer = identities[1].strip() if len(identities) > 1 else ""
    if not author or not signer:
        return (
            f"Git commit {commit} author or signer identity could not be read, so a signer other than the author "
            "cannot be confirmed; the policy requires distinct_signer"
        )
    if author.casefold() in signer.casefold():
        return (
            f"Git commit author {author} is also its signer; the policy requires distinct_signer, a reviewer other "
            "than the author signing the commit that carries the package"
        )
    return None


OUTSIDE_THE_REPOSITORY = "the Change Package must be inside the repository, so the attested commit can carry it"


def package_path_in(repo_root: Path, package_root: Path) -> str | None:
    located = inside(repo_root, package_root)
    try:
        return None if located is None else located.relative_to(repo_root.resolve()).as_posix()
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
    shown = run_git(repo_root, "show", f"{commit}:{relative}/{PROPOSAL_FILE}")
    return carried_proposal_errors(shown.stdout if shown.returncode == 0 else None, relative, proposal)


AUDITED_STATUSES = {"approved", "applied"}


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


def _attestation_verdict(repo_root: Path, audited: AuditedPackage, objects: dict[str, bytes]) -> tuple[str, str]:
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
    else:
        return "unconfirmed", f"commit {commit} is not in this clone; fetch the full history"
    if not errors:
        return "carried", commit
    return "not carried", errors[0]


def audit_attestations(repo_root: Path, changes_root: Path) -> list[dict[str, str]]:
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
        verdict, detail = _attestation_verdict(repo_root, entry, objects)
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
    if value.get("provider") != "git-signed-commit":
        errors.append("SCM attestation provider must be git-signed-commit")
    if not valid_commit(value.get("commit")):
        errors.append("SCM attestation requires a pinned 40-character commit")
    if value.get("proposal_revision") != proposal.get("proposal_revision"):
        errors.append("SCM attestation proposal revision does not match")
    if value.get("base_registry_revision") != proposal.get("base_registry_revision"):
        errors.append("SCM attestation Registry revision does not match")
    return errors


def verify_scm(path: Path, proposal: dict[str, Any]) -> list[str]:
    return verify_scm_value(load_json(path), proposal)
