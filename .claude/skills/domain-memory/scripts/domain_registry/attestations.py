from __future__ import annotations

from pathlib import Path
from typing import Any

from .common import completed_identifier, load_json


def verify_scm_value(value: dict[str, Any], proposal: dict[str, Any]) -> list[str]:
    errors = []
    for key in ("provider", "pull_request", "commit", "status"):
        if not completed_identifier(value.get(key)):
            errors.append(f"SCM attestation requires {key}")
    if value.get("status") != "approved":
        errors.append("SCM attestation status must be approved")
    if value.get("provider") not in {"github", "gitlab"}:
        errors.append("SCM attestation provider must be an externally verifiable host")
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
