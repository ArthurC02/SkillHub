from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import Any

from .common import load_json
from .hitl import attest_signed_commit, finalize_proposal, proposal_path
from .updates import apply_approved_updates

FORMAT = "domain-memory-ship/v1"
NEXT_STEPS = {
    "applied": "the Registry is updated; sign a commit holding the package and the Registry, then push",
    "approved": "the proposal is approved but not applied; run apply-approved-updates, or ship-package again",
    "verified": "the proposal is verified; ship-package --commit <the signed commit that carries the package>",
    "submitted": "the proposal is submitted again; correct what finalize named, verify-proposal, then ship-package",
}


def ship_package(
    package_root: Path, registry_root: Path, repo_root: Path, commit: str | None, verification_token_env: str = "GITHUB_TOKEN"
) -> dict[str, Any]:
    package_root, registry_root, repo_root = package_root.resolve(), registry_root.resolve(), repo_root.resolve()
    report: dict[str, Any] = {"format": FORMAT, "package": _shown(package_root, repo_root), "completed": [], "errors": []}
    steps: list[tuple[str, Callable[[], Any]]] = []
    if commit is not None:
        steps.append(("attest-signed-commit", lambda: attest_signed_commit(package_root, registry_root, repo_root, commit)))
    steps.append(("finalize-proposal", lambda: finalize_proposal(package_root, registry_root, repo_root, verification_token_env)))
    steps.append(("apply-approved-updates", lambda: apply_approved_updates(package_root, registry_root, repo_root)))
    for name, step in steps:
        if not _completed(report, name, step):
            break
    status = load_json(proposal_path(package_root)).get("status")
    report["status"] = status
    report["next"] = NEXT_STEPS.get(status, f"the proposal is {status}; ship-package works from a verified proposal")
    return report


def _completed(report: dict[str, Any], name: str, step: Callable[[], Any]) -> bool:
    try:
        result = step()
    except ValueError as error:
        report["errors"].append(f"{name}: {error}")
        return False
    report["completed"].append(name)
    if name == "attest-signed-commit":
        report["commit"] = result
    return True


def _shown(package_root: Path, repo_root: Path) -> str:
    return package_root.relative_to(repo_root).as_posix() if package_root.is_relative_to(repo_root) else str(package_root)
