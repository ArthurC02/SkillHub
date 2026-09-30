from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .common import load_json, writer_lock

STORAGE_MODES = {"tracked", "ignored", "external"}
CLASSIFICATIONS = {"public", "internal", "confidential", "restricted"}
REVIEW_MODES = {"local-draft-only", "scm-verified"}
GOVERNANCE_VERIFIERS = {"github-pr", "git-signed-commit", "none"}
GOVERNANCE_TRIGGERS = {"external-scm", "git-commit", "git-push", "none"}
CI_REQUIREMENTS = {"required", "optional", "none"}


def policy_path(root: Path) -> Path:
    return root / "domain-memory-policy.json"


def _nonblank_strings(value: Any) -> bool:
    return isinstance(value, list) and all(
        isinstance(item, str) and item.strip() for item in value
    )


def validate_policy(value: dict[str, Any]) -> list[str]:
    errors = []
    if value.get("format") != "domain-memory-policy/v1":
        errors.append("policy has an invalid format")
    if value.get("storage_mode") not in STORAGE_MODES:
        errors.append("policy has an invalid storage_mode")
    if value.get("data_classification") not in CLASSIFICATIONS:
        errors.append("policy has an invalid data_classification")
    if value.get("review_mode") not in REVIEW_MODES:
        errors.append("policy has an invalid review_mode")
    governance = value.get("review_governance")
    if not isinstance(governance, dict):
        errors.append("policy requires review_governance")
    elif (
        governance.get("verifier") not in GOVERNANCE_VERIFIERS
        or governance.get("trigger") not in GOVERNANCE_TRIGGERS
        or governance.get("ci_requirement") not in CI_REQUIREMENTS
        or not _nonblank_strings(governance.get("authorized_signers"))
    ):
        errors.append("policy has invalid review_governance")
    elif value.get("review_mode") == "local-draft-only" and governance["verifier"] != "none":
        errors.append("local-draft-only policy must not select a review verifier")
    elif value.get("review_mode") == "scm-verified" and governance["verifier"] == "none":
        errors.append("scm-verified policy requires a review verifier")
    elif governance["verifier"] == "git-signed-commit" and not governance["authorized_signers"]:
        errors.append("git-signed-commit policy requires authorized_signers")
    elif governance["verifier"] == "git-signed-commit" and governance.get("trigger") != "git-push":
        errors.append("git-signed-commit policy requires git-push enforcement")
    source = value.get("source_policy")
    if (
        not isinstance(source, dict)
        or not _nonblank_strings(source.get("selected_paths"))
        or not source["selected_paths"]
        or not isinstance(source.get("authority"), str)
        or not source["authority"].strip()
    ):
        errors.append("policy requires selected source paths and authority")
    elif (
        not _nonblank_strings(source.get("include"))
        or not source["include"]
        or not _nonblank_strings(source.get("exclude"))
    ):
        errors.append("policy requires include patterns and valid exclude patterns")
    limits = value.get("limits")
    if not isinstance(limits, dict) or any(
        not isinstance(limits.get(key), int) or limits[key] < 1
        for key in (
            "max_file_count",
            "max_file_bytes",
            "max_total_bytes",
            "max_query_results",
        )
    ):
        errors.append("policy has invalid limits")
    return errors


def _default_governance(review_mode: str) -> dict[str, Any]:
    local = review_mode == "local-draft-only"
    return {
        "verifier": "none" if local else "git-signed-commit",
        "trigger": "none" if local else "git-commit",
        "ci_requirement": "none",
        "authorized_signers": [],
    }


def _write_policy_file(path: Path, value: dict[str, Any]) -> None:
    path.write_text(
        json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )


def write_policy(
    root: Path,
    source_map: dict[str, Any],
    storage_mode: str,
    data_classification: str,
    review_mode: str,
    authority: str,
    include: list[str] | None = None,
    exclude: list[str] | None = None,
    governance: dict[str, Any] | None = None,
) -> None:
    template = Path(__file__).parents[2] / "templates" / "domain-memory-policy.json"
    value = load_json(template)
    value["storage_mode"] = storage_mode
    value["data_classification"] = data_classification
    value["review_mode"] = review_mode
    value["review_governance"] = governance or _default_governance(review_mode)
    value["source_policy"]["selected_paths"] = source_map["selected_paths"]
    value["source_policy"]["authority"] = authority
    if include is not None:
        value["source_policy"]["include"] = include
    if exclude is not None:
        value["source_policy"]["exclude"] = exclude
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid policy: " + "; ".join(errors))
    _write_policy_file(policy_path(root), value)


AMENDABLE_FIELDS = {
    "review_mode": ("review_mode",),
    "storage_mode": ("storage_mode",),
    "data_classification": ("data_classification",),
    "source_authority": ("source_policy", "authority"),
    "review_verifier": ("review_governance", "verifier"),
    "review_trigger": ("review_governance", "trigger"),
    "ci_requirement": ("review_governance", "ci_requirement"),
    "authorized_signers": ("review_governance", "authorized_signers"),
}

LIST_FIELDS = {"authorized_signers"}


def amend_policy(
    root: Path, field: str, value: str, reason: str, verifier: str | None = None
) -> dict[str, Any]:
    from .audit import append_locked

    if field not in AMENDABLE_FIELDS:
        raise ValueError(
            f"{field} is not an amendable policy field; amend one of {', '.join(sorted(AMENDABLE_FIELDS))}. "
            "Selected source paths and limits are settled when a Domain Memory is initialized."
        )
    if not isinstance(reason, str) or not reason.strip():
        raise ValueError(
            "amending a Domain Memory policy requires a reason: the policy decides what the Registry may become"
        )
    path = policy_path(root)
    if not path.is_file():
        raise ValueError(f"no Domain Memory policy at {path}")
    value_document = load_json(path)
    keys = AMENDABLE_FIELDS[field]
    target = value_document
    for key in keys[:-1]:
        target = target[key]
    amended: Any = value
    if field in LIST_FIELDS:
        amended = [entry.strip() for entry in value.split(",") if entry.strip()]
    change = {
        "operation": "amend-policy",
        "field": field,
        "from": target.get(keys[-1]),
        "to": amended,
        "reason": reason.strip(),
    }
    target[keys[-1]] = amended
    if field == "review_mode" and value == "local-draft-only":
        value_document["review_governance"] = _default_governance(value)
    elif field == "review_mode" and value == "scm-verified":
        if not isinstance(verifier, str) or not verifier.strip():
            raise ValueError(
                "leaving local-draft-only requires the verifier that replaces it, "
                f"one of {', '.join(sorted(GOVERNANCE_VERIFIERS - {'none'}))}: "
                "a review mode and the authority that backs it move together"
            )
        value_document["review_governance"]["verifier"] = verifier.strip()
        change["verifier"] = verifier.strip()
    errors = validate_policy(value_document)
    if errors:
        raise ValueError("amended policy is invalid: " + "; ".join(errors))
    with writer_lock(root):
        _write_policy_file(path, value_document)
        append_locked(root, change)
    return change


def _valid_policy(path: Path) -> dict[str, Any]:
    value = load_json(path)
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid Domain Memory policy: " + "; ".join(errors))
    return value


def review_mode(root: Path) -> str:
    path = policy_path(root)
    if not path.is_file():
        return "legacy-unverified"
    return _valid_policy(path)["review_mode"]


def review_governance(root: Path) -> dict[str, Any]:
    return _valid_policy(policy_path(root))["review_governance"]


def approved_command_profiles(root: Path | None) -> set[str]:
    if root is None:
        return set()
    path = policy_path(root)
    if not path.is_file():
        return set()
    profiles = _valid_policy(path).get("approved_command_profiles", [])
    return {
        profile for profile in profiles if isinstance(profile, str) and profile.strip()
    }


def query_result_limit(root: Path) -> int:
    path = policy_path(root)
    if not path.is_file():
        return 100
    return _valid_policy(path)["limits"]["max_query_results"]
