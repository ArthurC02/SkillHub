import json
import shlex
import subprocess
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

from .changes import missing_approval_roles, test_attestation_errors, validate_change_package
from .attestations import (
    commit_carries_proposal,
    github_commit_carries_proposal,
    verify_external_scm,
    verify_git_signed_commit,
)
from .common import completed_identifier, iso_timestamp, load_json
from .counterfactual import OUTPUT_TAIL, digest
from .policy import approved_command_profiles, review_governance
from .revision import current_registry_revision, require_current_registry_revision


def write_document(path: Path, value: dict) -> None:
    temporary = path.with_suffix(f"{path.suffix}.tmp")
    temporary.write_text(
        json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8", newline="\n"
    )
    temporary.replace(path)


def now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def proposal_path(root: Path) -> Path:
    return root / "domain-change-proposal.json"


def submit_proposal(root: Path, registry_root: Path, repo_root: Path) -> None:
    proposal = load_json(proposal_path(root))
    if proposal.get("status") != "draft":
        raise ValueError("only a draft proposal may be submitted")
    if not completed_identifier(proposal.get("proposer")):
        raise ValueError("a proposal requires a proposer before submission")
    revision = current_registry_revision(registry_root, repo_root)
    proposal["base_registry_revision"] = revision
    evidence_path = root / "evidence-bundle.json"
    evidence = load_json(evidence_path)
    evidence["registry_revision"] = revision
    errors = validate_change_package(root, None)
    if errors:
        raise ValueError("draft proposal is invalid: " + "; ".join(errors))
    proposal["status"] = "submitted"
    proposal["submitted_at"] = now()
    write_document(proposal_path(root), proposal)
    write_document(evidence_path, evidence)


SUPERSEDABLE_STATUSES = {"draft", "submitted", "verified", "approved", "applied"}


def supersede_proposal(root: Path, reason: str, superseded_by: str | None) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    status = proposal.get("status")
    if status not in SUPERSEDABLE_STATUSES:
        raise ValueError(
            f"a {status} proposal cannot be superseded; create a new draft instead"
        )
    if not completed_identifier(reason):
        raise ValueError(
            "superseding a proposal requires a reason: what its conclusion got wrong, or what replaced it"
        )
    if superseded_by is not None and not completed_identifier(superseded_by):
        raise ValueError("superseded_by must name the proposal that replaces this one")
    proposal["superseded_from_status"] = status
    proposal["status"] = "superseded"
    proposal["superseded_at"] = now()
    proposal["superseded_reason"] = reason
    if superseded_by is not None:
        proposal["superseded_by"] = superseded_by
    write_document(path, proposal)


def record_approval(
    root: Path, role: str, reviewer: str, scope: str, approved_at: str | None
) -> None:
    proposal = load_json(proposal_path(root))
    if proposal.get("status") != "submitted":
        raise ValueError("approvals may be recorded only for a submitted proposal")
    if not all(completed_identifier(value) for value in (role, reviewer, scope)):
        raise ValueError("approval requires role, reviewer, and scope")
    if reviewer == proposal.get("proposer"):
        raise ValueError("the proposer cannot approve the proposal")
    timestamp = approved_at or now()
    if not iso_timestamp(timestamp):
        raise ValueError("approved_at must be an ISO-8601 timestamp")
    approvals = proposal.setdefault("approvals", [])
    if any(
        entry.get("role") == role
        and entry.get("proposal_revision") == proposal.get("proposal_revision")
        for entry in approvals
    ):
        raise ValueError("the role already approved this proposal revision")
    approvals.append(
        {
            "role": role,
            "reviewer": reviewer,
            "decision": "approved",
            "approved_at": timestamp,
            "scope": scope,
            "proposal_revision": proposal.get("proposal_revision"),
            "base_registry_revision": proposal.get("base_registry_revision"),
        }
    )
    write_document(proposal_path(root), proposal)


def verify_proposal(root: Path, registry_root: Path, repo_root: Path) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    if proposal.get("status") != "submitted":
        raise ValueError("only a submitted proposal may be verified")
    missing = missing_approval_roles(load_json(root / "requirement-normalization.json"), proposal)
    if missing:
        raise ValueError(
            "a proposal is verified after its required approvals, which can be recorded only "
            "while it is submitted; first run record-approval for each of: " + ", ".join(missing)
        )
    require_current_registry_revision(proposal, registry_root, repo_root)
    obligations = load_json(root / "test-obligations.json").get("obligations", [])
    obligation_ids = {
        entry["id"]
        for entry in obligations
        if isinstance(entry, dict) and completed_identifier(entry.get("id"))
    }
    evidence = load_json(root / "evidence-bundle.json")
    errors = test_attestation_errors(evidence, obligation_ids, registry_root)
    if errors:
        raise ValueError("proposal lacks verified test evidence: " + "; ".join(errors))
    proposal["status"] = "verified"
    proposal["verified_at"] = now()
    write_document(path, proposal)
    errors = validate_change_package(root, registry_root)
    if errors:
        proposal["status"] = "submitted"
        proposal["verified_at"] = None
        write_document(path, proposal)
        raise ValueError("verified proposal is invalid: " + "; ".join(errors))


@dataclass(frozen=True)
class ObligationRun:
    obligation_id: str
    command: str
    command_profile: str


def record_test_result(
    root: Path, registry_root: Path, repo_root: Path, run: ObligationRun, timeout: int
) -> None:
    obligation_id, command, command_profile = run.obligation_id, run.command, run.command_profile
    obligations = load_json(root / "test-obligations.json").get("obligations", [])
    if not any(isinstance(entry, dict) and entry.get("id") == obligation_id for entry in obligations):
        raise ValueError(f"test-obligations.json has no obligation {obligation_id}")
    profiles = approved_command_profiles(registry_root)
    if profiles and command_profile not in profiles:
        raise ValueError(
            f"command profile {command_profile} is not approved by Domain Memory policy; "
            "approved: " + ", ".join(sorted(profiles))
        )
    try:
        completed = subprocess.run(
            shlex.split(command), cwd=repo_root, capture_output=True, timeout=timeout, check=False
        )
    except subprocess.TimeoutExpired as error:
        raise ValueError(f"{command} gave no result within {timeout} seconds; nothing recorded") from error
    except OSError as error:
        raise ValueError(f"{command} could not start: {error}; nothing recorded") from error
    if completed.returncode != 0:
        tail = (completed.stdout + completed.stderr)[-OUTPUT_TAIL:].decode("utf-8", "replace")
        raise ValueError(
            f"{command} exited {completed.returncode}; only a passing run is recorded, "
            f"and nothing was written:\n{tail}"
        )
    result = {
        "obligation_id": obligation_id,
        "status": "passed",
        "evidence": command,
        "command_profile": command_profile,
        "exit_code": 0,
        "output_sha256": digest(completed.stdout + completed.stderr),
        "finished_at": now(),
    }
    evidence_path = root / "evidence-bundle.json"
    evidence = load_json(evidence_path)
    kept = [
        entry for entry in evidence.get("test_results", [])
        if not (isinstance(entry, dict) and entry.get("obligation_id") == obligation_id)
    ]
    evidence["test_results"] = [*kept, result]
    write_document(evidence_path, evidence)


def attest_signed_commit(root: Path, registry_root: Path, repo_root: Path, commit: str) -> str:
    governance = review_governance(registry_root)
    if governance["verifier"] != "git-signed-commit":
        raise ValueError(
            f"attest-signed-commit is for the git-signed-commit verifier; this policy uses "
            f"{governance['verifier']}"
        )
    proposal = load_json(proposal_path(root))
    if proposal.get("base_registry_revision") is None:
        raise ValueError("submit the proposal first, so the attestation can bind to its base revision")
    resolved = subprocess.run(
        ["git", "-C", str(repo_root), "rev-parse", "--verify", f"{commit}^{{commit}}"],
        capture_output=True, text=True, check=False,
    )
    if resolved.returncode != 0:
        raise ValueError(f"no commit {commit} in {repo_root}")
    full = resolved.stdout.strip()
    errors = verify_git_signed_commit(
        {"commit": full}, repo_root, registry_root, governance["authorized_signers"]
    ) or commit_carries_proposal(repo_root, full, root, proposal)
    if errors:
        raise ValueError("the commit cannot attest this proposal: " + "; ".join(errors))
    evidence_path = root / "evidence-bundle.json"
    evidence = load_json(evidence_path)
    evidence["scm_attestation"] = {
        "provider": "git-signed-commit",
        "commit": full,
        "status": "approved",
        "proposal_revision": proposal.get("proposal_revision"),
        "base_registry_revision": proposal["base_registry_revision"],
    }
    write_document(evidence_path, evidence)
    return full


ATTESTATION_STEPS = {
    "github-pr": (
        "after the pull request merges, add scm_attestation to evidence-bundle.json with provider "
        '"github", status "approved", the merged head commit as 40 hex characters, pull_request and '
        "checks_url as https URLs, and proposal_revision and base_registry_revision copied from "
        "domain-change-proposal.json; check it with verify-scm-attestation"
    ),
    "git-signed-commit": (
        "after the signed commit that adds this Change Package and changes the Domain Memory files "
        "exists, run attest-signed-commit --commit <that commit>; it checks the signature and the "
        'package, and writes scm_attestation with provider "git-signed-commit"'
    ),
}


def missing_attestation_message(verifier: str) -> str:
    steps = ATTESTATION_STEPS.get(verifier)
    if steps is None:
        return "finalizing a proposal requires SCM attestation, and the policy names no review verifier"
    return f"finalizing a proposal requires SCM attestation: {steps}, then finalize again"


def finalize_proposal(
    root: Path, registry_root: Path, repo_root: Path, verification_token_env: str = "GITHUB_TOKEN"
) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    if proposal.get("status") != "verified":
        raise ValueError("only a verified proposal may be finalized")
    require_current_registry_revision(proposal, registry_root, repo_root)
    evidence = load_json(root / "evidence-bundle.json")
    attestation = evidence.get("scm_attestation")
    governance = review_governance(registry_root)
    if not isinstance(attestation, dict):
        raise ValueError(missing_attestation_message(governance["verifier"]))
    if governance["verifier"] == "github-pr":
        errors = verify_external_scm(
            attestation,
            verification_token_env,
            governance["ci_requirement"] == "required",
        ) or github_commit_carries_proposal(
            attestation, verification_token_env, repo_root, root, proposal
        )
    elif governance["verifier"] == "git-signed-commit":
        errors = verify_git_signed_commit(
            attestation, repo_root, registry_root, governance["authorized_signers"]
        ) or commit_carries_proposal(repo_root, attestation["commit"], root, proposal)
    else:
        errors = ["policy has no review verifier"]
    if errors:
        raise ValueError("SCM governance is not externally verified: " + "; ".join(errors))
    proposal["status"] = "approved"
    proposal["finalized_at"] = now()
    write_document(path, proposal)
    errors = validate_change_package(root, registry_root)
    if errors:
        proposal["status"] = "submitted"
        proposal.pop("finalized_at", None)
        write_document(path, proposal)
        raise ValueError(
            "proposal lacks required approvals or traceability, and is submitted again so it "
            "can be corrected, then verified and finalized again: " + "; ".join(errors)
        )
