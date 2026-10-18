from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import Any

from .common import APPLY_OPERATION, load_json, shown_path
from .hitl import EVIDENCE_FILE, attest_signed_commit, finalize_proposal, proposal_path
from .revision import StaleBaseError, require_current_registry_revision
from .git import resolve_commit
from .updates import apply_approved_updates

FORMAT = "domain-memory-ship/v1"
ATTEST, FINALIZE, APPLY = "attest-signed-commit", "finalize-proposal", APPLY_OPERATION
STEPS_FROM = {"approved": (APPLY,), "applied": ()}
ALREADY_APPLIED = "nothing to do here; if the applied Registry change is not committed yet, sign a commit holding it, then push"
NEXT_STEPS = {
    "applied": "the Registry is updated; sign a commit holding the package and the Registry, then push",
    "approved": "the proposal is approved but not applied; run ship-package again",
    "verified": "the proposal is verified; ship-package --commit <the signed commit that carries the package>",
    "submitted": "the proposal is submitted again; correct what finalize named, verify-proposal, then ship-package",
}
STALE_NEXT = (
    "the proposal base is stale; rebase-proposal, then record-approval, verify-proposal, sign a commit holding the "
    "package, and ship-package --commit <that commit>"
)


def ship_package(
    package_root: Path, registry_root: Path, repo_root: Path, commit: str | None, verification_token_env: str = "GITHUB_TOKEN"
) -> dict[str, Any]:
    package_root, registry_root, repo_root = package_root.resolve(), registry_root.resolve(), repo_root.resolve()
    report: dict[str, Any] = {"format": FORMAT, "package": shown_path(package_root, repo_root), "completed": [], "errors": []}
    proposal = load_json(proposal_path(package_root))
    status = proposal.get("status")
    actions: dict[str, Callable[[], Any]] = {
        ATTEST: lambda: attest_signed_commit(package_root, registry_root, repo_root, commit),
        FINALIZE: lambda: finalize_proposal(package_root, registry_root, repo_root, verification_token_env),
        APPLY: lambda: apply_approved_updates(package_root, registry_root, repo_root),
    }
    names = STEPS_FROM.get(status, (ATTEST, FINALIZE, APPLY) if commit is not None else (FINALIZE, APPLY))
    if status in STEPS_FROM:
        report["errors"] += _another_commit(package_root, repo_root, status, commit)
        if not names and not report["errors"]:
            report["note"] = f"already {status}; nothing was written"
    stale_steps: list[str] = []
    if status == "verified":
        _completed(
            report, FINALIZE, lambda: require_current_registry_revision(proposal, registry_root, repo_root),
            stale_steps, record=False,
        )
    for name in names:
        if report["errors"] or not _completed(report, name, actions[name], stale_steps):
            break
    status = load_json(proposal_path(package_root)).get("status")
    report["status"] = status
    report["next"] = _next_step(report, status, stale=bool(stale_steps))
    return report


def _next_step(report: dict[str, Any], status: str, *, stale: bool) -> str:
    if "note" in report:
        return ALREADY_APPLIED
    if stale:
        return STALE_NEXT
    return NEXT_STEPS.get(status, f"the proposal is {status}; ship-package works from a verified proposal")


def _another_commit(package_root: Path, repo_root: Path, status: str, commit: str | None) -> list[str]:
    attested = (load_json(package_root / EVIDENCE_FILE).get("scm_attestation") or {}).get("commit")
    if commit is None or attested is None:
        return []
    given = resolve_commit(repo_root, commit) or commit
    if given == attested:
        return []
    return [f"{ATTEST}: the proposal is {status} with commit {attested}; {given} is another commit, so nothing was written"]


def _completed(
    report: dict[str, Any], name: str, step: Callable[[], Any], stale_steps: list[str], *, record: bool = True
) -> bool:
    try:
        result = step()
    except ValueError as error:
        report["errors"].append(f"{name}: {error}")
        if isinstance(error, StaleBaseError):
            stale_steps.append(name)
        return False
    if not record:
        return True
    report["completed"].append(name)
    if name == ATTEST:
        report["commit"] = result
    return True
