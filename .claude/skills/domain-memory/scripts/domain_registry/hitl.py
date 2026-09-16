from .changes import test_attestation_errors, validate_change_package
from .common import load_json
from .common import completed_identifier, iso_timestamp, load_json
from .revision import current_registry_revision, require_current_registry_revision

import json
from datetime import datetime, timezone
from pathlib import Path


def write_document(path: Path, value: dict) -> None:
    temporary = path.with_suffix(f"{path.suffix}.tmp")
    temporary.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
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


def record_approval(root: Path, role: str, reviewer: str, scope: str, approved_at: str | None) -> None:
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
    if any(entry.get("role") == role and entry.get("proposal_revision") == proposal.get("proposal_revision") for entry in approvals):
        raise ValueError("the role already approved this proposal revision")
    approvals.append({"role": role, "reviewer": reviewer, "decision": "approved", "approved_at": timestamp, "scope": scope, "proposal_revision": proposal.get("proposal_revision"), "base_registry_revision": proposal.get("base_registry_revision")})
    write_document(proposal_path(root), proposal)


def verify_proposal(root: Path, registry_root: Path, repo_root: Path) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    if proposal.get("status") != "submitted":
        raise ValueError("only a submitted proposal may be verified")
    require_current_registry_revision(proposal, registry_root, repo_root)
    obligations = load_json(root / "test-obligations.json").get("obligations", [])
    obligation_ids = {entry.get("id") for entry in obligations if isinstance(entry, dict) and completed_identifier(entry.get("id"))}
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


def finalize_proposal(root: Path, registry_root: Path, repo_root: Path) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    if proposal.get("status") != "verified":
        raise ValueError("only a verified proposal may be finalized")
    require_current_registry_revision(proposal, registry_root, repo_root)
    proposal["status"] = "approved"
    proposal["finalized_at"] = now()
    write_document(path, proposal)
    errors = validate_change_package(root, registry_root)
    if errors:
        proposal["status"] = "submitted"
        proposal.pop("finalized_at", None)
        write_document(path, proposal)
        raise ValueError("proposal lacks required approvals or traceability: " + "; ".join(errors))
