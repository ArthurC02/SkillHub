import shlex
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .changes import missing_approval_roles, test_attestation_errors, unconfirmed_evidence_errors, validate_change_package
from .attestations import commit_carries_proposal, verify_git_signed_commit
from .common import (
    sha256_of,
    EVIDENCE_FILE,
    OBLIGATIONS_FILE,
    PROPOSAL_FILE,
    REQUIREMENT_FILE,
    completed_identifier,
    find_record,
    iso_timestamp,
    load_json,
    shown_path,
    utc_now,
    write_json,
)
from .counterfactual import (
    OBLIGATION_PLACEHOLDER,
    OUTPUT_TAIL,
    decoded,
    is_go_test,
    no_test_passed,
    reports_tests,
    working_directory,
)
from .git import resolve_commit
from .policy import approved_command_profiles, review_governance
from .revision import StaleBaseError, base_registry_revision, require_current_registry_revision


def proposal_path(root: Path) -> Path:
    return root / PROPOSAL_FILE


def submit_proposal(root: Path, registry_root: Path, repo_root: Path) -> None:
    proposal = load_json(proposal_path(root))
    if proposal.get("status") != "draft":
        raise ValueError("only a draft proposal may be submitted")
    silent = [
        entry.get("id")
        for entry in load_json(root / OBLIGATIONS_FILE).get("obligations", [])
        if isinstance(entry, dict) and is_go_test(argv := str(entry.get("check", "")).split()) and not reports_tests(argv)
    ]
    if silent:
        raise ValueError(
            f"obligations {', '.join(map(str, silent))} run go test without -v or -json, so record-test-result "
            "cannot tell a passing test from a skipped one; add -v to their check"
        )
    if not completed_identifier(proposal.get("proposer")):
        raise ValueError("a proposal requires a proposer before submission")
    revision = base_registry_revision(registry_root, repo_root)
    proposal["base_registry_revision"] = revision
    evidence_path = root / EVIDENCE_FILE
    evidence = load_json(evidence_path)
    evidence["registry_revision"] = revision
    errors = validate_change_package(root, None) + unconfirmed_evidence_errors(proposal, registry_root)
    if errors:
        raise ValueError("draft proposal is invalid: " + "; ".join(errors))
    proposal["status"] = "submitted"
    proposal["submitted_at"] = utc_now()
    write_json(proposal_path(root), proposal)
    write_json(evidence_path, evidence)


REBASABLE_STATUSES = ("submitted", "verified", "approved")
NOT_REBASED = {
    "draft": "submit-proposal stamps its base",
    "applied": "it is applied; supersede it and open a new draft",
    "superseded": "it is retired; redraft-proposal opens a new draft from it",
}
NEXT_WHEN_CURRENT = {
    "submitted": "record-approval for each required role, then verify-proposal",
    "verified": "sign a commit holding the package, then ship-package --commit <that commit>",
    "approved": "run ship-package to apply it",
}
AFTER_REBASE = (
    "record-approval for each required role, verify-proposal, sign a commit holding the package, "
    "then ship-package --commit <that commit>"
)


def _article(word: Any) -> str:
    return "an" if str(word)[:1] in "aeiou" else "a"


def _stale(proposal: dict[str, Any], registry_root: Path, repo_root: Path) -> bool:
    try:
        require_current_registry_revision(proposal, registry_root, repo_root)
    except StaleBaseError:
        return True
    return False


def rebase_proposal(root: Path, registry_root: Path, repo_root: Path) -> dict[str, Any]:
    root, registry_root, repo_root = root.resolve(), registry_root.resolve(), repo_root.resolve()
    proposal = load_json(proposal_path(root))
    status = proposal.get("status")
    if status not in REBASABLE_STATUSES:
        reason = NOT_REBASED.get(status, "only a submitted, verified or approved proposal is")
        raise ValueError(f"{_article(status)} {status} proposal is not rebased; {reason}")
    shown = shown_path(root, repo_root)
    if not _stale(proposal, registry_root, repo_root):
        return {
            "package": shown,
            "status": status,
            "proposal_revision": proposal.get("proposal_revision"),
            "base_registry_revision": proposal.get("base_registry_revision"),
            "note": "the proposal base is current; nothing was written",
            "next": NEXT_WHEN_CURRENT[status],
        }
    revision = base_registry_revision(registry_root, repo_root)
    retired = sum(
        1 for approval in proposal.get("approvals") or []
        if isinstance(approval, dict) and approval.get("proposal_revision") == proposal.get("proposal_revision")
    )
    proposal.update(
        base_registry_revision=revision, proposal_revision=proposal["proposal_revision"] + 1,
        status="submitted", verified_at=None, finalized_at=None,
    )
    evidence_path = root / EVIDENCE_FILE
    evidence = load_json(evidence_path)
    attested = isinstance(evidence.get("scm_attestation"), dict)
    evidence.update(registry_revision=revision, scm_attestation=None)
    write_json(evidence_path, evidence)
    write_json(proposal_path(root), proposal)
    return {
        "package": shown,
        "status": "submitted",
        "proposal_revision": proposal["proposal_revision"],
        "base_registry_revision": revision,
        "approvals_retired": retired,
        "attestation_removed": attested,
        "next": AFTER_REBASE,
    }


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
    proposal["superseded_at"] = utc_now()
    proposal["superseded_reason"] = reason
    if superseded_by is not None:
        proposal["superseded_by"] = superseded_by
    write_json(path, proposal)


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
    timestamp = approved_at or utc_now()
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
    write_json(proposal_path(root), proposal)


RECORDABLE_STATUSES = {"draft", "submitted"}
CI_LINK_PREFIX = "https://"
def same_command(command: str, check: Any) -> bool:
    return isinstance(check, str) and command.split() == check.split()


def _evidence_check_errors(evidence: dict[str, Any], obligations: list[Any]) -> list[str]:
    checks = {entry.get("id"): entry.get("check") for entry in obligations if isinstance(entry, dict)}
    return [
        f"the test result of {result['obligation_id']} ran {result.get('evidence')!r}, not the obligation's check "
        f"{checks[result['obligation_id']]!r}; record the check, or cite the CI run as an https:// link"
        for result in evidence.get("test_results", [])
        if isinstance(result, dict) and result.get("obligation_id") in checks
        and not str(result.get("evidence")).startswith(CI_LINK_PREFIX)
        and not same_command(str(result.get("evidence")), checks[result["obligation_id"]])
    ]


def verify_proposal(root: Path, registry_root: Path, repo_root: Path) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    if proposal.get("status") != "submitted":
        raise ValueError("only a submitted proposal may be verified")
    missing = missing_approval_roles(load_json(root / REQUIREMENT_FILE), proposal)
    if missing:
        raise ValueError(
            "a proposal is verified after its required approvals, which can be recorded only "
            "while it is submitted; first run record-approval for each of: " + ", ".join(missing)
        )
    require_current_registry_revision(proposal, registry_root, repo_root)
    obligations = load_json(root / OBLIGATIONS_FILE).get("obligations", [])
    obligation_ids = {
        entry["id"]
        for entry in obligations
        if isinstance(entry, dict) and completed_identifier(entry.get("id"))
    }
    evidence = load_json(root / EVIDENCE_FILE)
    errors = test_attestation_errors(evidence, obligation_ids, registry_root) + _evidence_check_errors(
        evidence, obligations
    )
    if errors:
        raise ValueError("proposal lacks verified test evidence: " + "; ".join(errors))
    errors = validate_change_package(root, registry_root, "verified")
    if errors:
        raise ValueError("verified proposal is invalid: " + "; ".join(errors))
    proposal["status"] = "verified"
    proposal["verified_at"] = utc_now()
    write_json(path, proposal)


@dataclass(frozen=True)
class ObligationRun:
    obligation_id: str
    command_profile: str
    cwd: str | None = None


def record_test_result(
    root: Path, registry_root: Path, repo_root: Path, run: ObligationRun, timeout: int
) -> None:
    obligation_id, command_profile = run.obligation_id, run.command_profile
    status = load_json(proposal_path(root)).get("status")
    if status not in RECORDABLE_STATUSES:
        raise ValueError(
            f"the proposal is {status}: its test results are what verify-proposal checked, so they stay as they are; "
            "redraft it to record new ones"
        )
    obligations = load_json(root / OBLIGATIONS_FILE).get("obligations", [])
    obligation = find_record(obligations, obligation_id)
    if obligation is None:
        raise ValueError(f"test-obligations.json has no obligation {obligation_id}")
    command = (obligation.get("check") or "").strip()
    directory = working_directory(repo_root, run.cwd)
    profiles = approved_command_profiles(registry_root)
    if profiles and command_profile not in profiles:
        raise ValueError(
            f"command profile {command_profile} is not approved by Domain Memory policy; "
            "approved: " + ", ".join(sorted(profiles))
        )
    try:
        completed = subprocess.run(
            argv := shlex.split(command), cwd=directory, capture_output=True, timeout=timeout, check=False
        )
    except subprocess.TimeoutExpired as error:
        raise ValueError(f"{command} gave no result within {timeout} seconds; nothing recorded") from error
    except OSError as error:
        raise ValueError(
            f"{command} could not start: {error}. The command runs as written, with no shell and "
            "no aliases, so give the full invocation, such as python3 <path>/registry_tools.py "
            "verify-audit --registry-root <root>; nothing recorded"
        ) from error
    output = decoded(completed.stdout + completed.stderr)
    if completed.returncode != 0:
        raise ValueError(
            f"{command} exited {completed.returncode}; only a passing run is recorded, "
            f"and nothing was written:\n{output[-OUTPUT_TAIL:]}"
        )
    reason = no_test_passed(argv, output)
    if reason:
        raise ValueError(f"{command} exited 0, but {reason}; nothing recorded")
    result = {
        "obligation_id": obligation_id,
        "status": "passed",
        "evidence": command,
        "command_profile": command_profile,
        "exit_code": 0,
        "output_sha256": sha256_of(completed.stdout + completed.stderr),
        "finished_at": utc_now(),
    }
    if run.cwd is not None:
        result["working_directory"] = directory.relative_to(repo_root.resolve()).as_posix()
    evidence_path = root / EVIDENCE_FILE
    evidence = load_json(evidence_path)
    kept = [
        entry for entry in evidence.get("test_results", [])
        if not (isinstance(entry, dict) and entry.get("obligation_id") == obligation_id)
    ]
    evidence["test_results"] = [*kept, result]
    write_json(evidence_path, evidence)


def require_recordable_counterfactual(root: Path, obligation_id: str | None) -> None:
    if not obligation_id or obligation_id == OBLIGATION_PLACEHOLDER:
        raise ValueError("--package-root records the check under an obligation; name it with --obligation-id")
    proposal = load_json(proposal_path(root))
    status = proposal.get("status")
    if status not in RECORDABLE_STATUSES:
        raise ValueError(
            f"the proposal is {status}: its counterfactual checks are what verify-proposal checked, so they stay "
            "as they are; redraft it to record new ones"
        )
    if find_record(load_json(root / OBLIGATIONS_FILE).get("obligations", []), obligation_id) is None:
        raise ValueError(f"test-obligations.json has no obligation {obligation_id}")
    design = proposal.get("implementation_design")
    planned = design.get("proof_obligations") if isinstance(design, dict) else None
    if isinstance(planned, list) and obligation_id not in planned:
        raise ValueError(
            f"obligation {obligation_id} is not one of implementation_design's proof_obligations: "
            + ", ".join(str(planned_id) for planned_id in planned)
        )


def record_counterfactual(root: Path, check: dict[str, Any]) -> Path:
    require_recordable_counterfactual(root, check.get("obligation_id"))
    path = root / EVIDENCE_FILE
    evidence = load_json(path)
    recorded = evidence.get("counterfactual_check")
    runs = recorded if isinstance(recorded, list) else [recorded] if _passed_run(recorded) else []
    kept = [run for run in runs if not (isinstance(run, dict) and run.get("obligation_id") == check["obligation_id"])]
    evidence["counterfactual_check"] = [*kept, check]
    write_json(path, evidence)
    return path


def _passed_run(recorded: Any) -> bool:
    return isinstance(recorded, dict) and recorded.get("status") == "passed"


def attest_signed_commit(root: Path, registry_root: Path, repo_root: Path, commit: str) -> str:
    governance = review_governance(registry_root)
    if governance["verifier"] != "git-signed-commit":
        raise ValueError(
            f"--commit is for the git-signed-commit verifier; this policy uses "
            f"{governance['verifier']}"
        )
    proposal = load_json(proposal_path(root))
    if proposal.get("base_registry_revision") is None:
        raise ValueError("submit the proposal first, so the attestation can bind to its base revision")
    full = resolve_commit(repo_root, commit)
    if full is None:
        raise ValueError(f"no commit {commit} in {repo_root}")
    errors = verify_git_signed_commit(
        {"commit": full}, repo_root, registry_root, governance["authorized_signers"],
        governance.get("distinct_signer", False),
    ) or commit_carries_proposal(repo_root, full, root, proposal)
    if errors:
        raise ValueError("the commit cannot attest this proposal: " + "; ".join(errors))
    evidence_path = root / EVIDENCE_FILE
    evidence = load_json(evidence_path)
    evidence["scm_attestation"] = {
        "provider": "git-signed-commit",
        "commit": full,
        "status": "approved",
        "proposal_revision": proposal.get("proposal_revision"),
        "base_registry_revision": proposal["base_registry_revision"],
    }
    write_json(evidence_path, evidence)
    return full


ATTESTATION_STEPS = {
    "git-signed-commit": (
        "after the signed commit that adds this Change Package and changes the Domain Memory files "
        "exists, run ship-package --commit <that commit>; it checks the signature and the "
        'package, and writes scm_attestation with provider "git-signed-commit"'
    ),
}


def missing_attestation_message(verifier: str) -> str:
    steps = ATTESTATION_STEPS.get(verifier)
    if steps is None:
        return "finalizing a proposal requires SCM attestation, and the policy names no review verifier"
    return f"finalizing a proposal requires SCM attestation: {steps}, then finalize again"


def finalize_proposal(root: Path, registry_root: Path, repo_root: Path) -> None:
    path = proposal_path(root)
    proposal = load_json(path)
    if proposal.get("status") != "verified":
        raise ValueError("only a verified proposal may be finalized")
    require_current_registry_revision(proposal, registry_root, repo_root)
    evidence = load_json(root / EVIDENCE_FILE)
    attestation = evidence.get("scm_attestation")
    governance = review_governance(registry_root)
    if not isinstance(attestation, dict):
        raise ValueError(missing_attestation_message(governance["verifier"]))
    if governance["verifier"] == "git-signed-commit":
        errors = verify_git_signed_commit(
            attestation, repo_root, registry_root, governance["authorized_signers"],
            governance.get("distinct_signer", False),
        ) or commit_carries_proposal(repo_root, attestation["commit"], root, proposal)
    else:
        errors = ["policy has no review verifier"]
    if errors:
        raise ValueError("SCM governance is not externally verified: " + "; ".join(errors))
    errors = validate_change_package(root, registry_root, "approved")
    if not errors:
        proposal["status"] = "approved"
        proposal["finalized_at"] = utc_now()
        write_json(path, proposal)
        return
    proposal["status"] = "submitted"
    proposal.pop("finalized_at", None)
    write_json(path, proposal)
    raise ValueError(
        "proposal lacks required approvals or traceability, and is submitted again so it "
        "can be corrected, then verified and finalized again: " + "; ".join(errors)
    )
