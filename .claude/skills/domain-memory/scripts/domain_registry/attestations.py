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
    if value.get("proposal_revision") != proposal.get("proposal_revision"):
        errors.append("SCM attestation proposal revision does not match")
    if value.get("base_registry_revision") != proposal.get("base_registry_revision"):
        errors.append("SCM attestation Registry revision does not match")
    return errors


def verify_scm(path: Path, proposal: dict[str, Any]) -> list[str]:
    return verify_scm_value(load_json(path), proposal)
