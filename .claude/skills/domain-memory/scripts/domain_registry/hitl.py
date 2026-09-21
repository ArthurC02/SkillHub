import json
from datetime import datetime, timezone
from pathlib import Path

from .changes import test_attestation_errors, validate_change_package
from .attestations import verify_external_scm, verify_git_signed_commit
from .common import completed_identifier, iso_timestamp, load_json
from .policy import review_governance
from .revision import current_registry_revision, require_current_registry_revision


def write_document(path: Path, value: dict) -> None:
    temporary = path.with_suffix(f"{path.suffix}.tmp")
    temporary.write_text(
        json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
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
    require_current_registry_revision(proposal, registry_root, repo_root)
    obligations = load_json(root / "test-obligations.json").get("obligations", [])
    obligation_ids: set[str] = set()
    for entry in obligations:
        if not isinstance(entry, dict):
            continue
        identifier = entry.get("id")
        if isinstance(identifier, str) and completed_identifier(identifier):
            obligation_ids.add(identifier)
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


def finalize_proposal(
    root: Path, registry_root: Path, repo_root: Path, verification_token_env: str = "GITHUB_TOKEN"
) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    if proposal.get("status") != "verified":
        raise ValueError("only a verified proposal may be finalized")
    require_current_registry_revision(proposal, registry_root, repo_root)
    evidence = load_json(root / "evidence-bundle.json")
    attestation = evidence.get("scm_attestation") if isinstance(evidence, dict) else None
    if not isinstance(attestation, dict):
        raise ValueError("finalizing a proposal requires SCM attestation")
    governance = review_governance(registry_root)
    if governance["verifier"] == "github-pr":
        errors = verify_external_scm(
            attestation,
            verification_token_env,
            governance["ci_requirement"] == "required",
        )
    elif governance["verifier"] == "git-signed-commit":
        errors = verify_git_signed_commit(
            attestation, repo_root, registry_root, governance["authorized_signers"]
        )
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
            "proposal lacks required approvals or traceability: " + "; ".join(errors)
        )
