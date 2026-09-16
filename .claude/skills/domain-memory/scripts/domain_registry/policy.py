from __future__ import annotations

from .common import load_json

import json
import shutil
from pathlib import Path
from typing import Any


STORAGE_MODES = {"tracked", "ignored", "external"}
CLASSIFICATIONS = {"public", "internal", "confidential", "restricted"}
REVIEW_MODES = {"local-draft-only", "scm-verified"}


def policy_path(root: Path) -> Path:
    return root / "domain-memory-policy.json"


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
    source = value.get("source_policy")
    if not isinstance(source, dict) or not isinstance(source.get("selected_paths"), list) or not source["selected_paths"] or not all(isinstance(path, str) and path.strip() for path in source["selected_paths"]) or not isinstance(source.get("authority"), str) or not source["authority"].strip():
        errors.append("policy requires selected source paths and authority")
    elif not isinstance(source.get("include"), list) or not source["include"] or not all(isinstance(pattern, str) and pattern.strip() for pattern in source["include"]) or not isinstance(source.get("exclude"), list) or not all(isinstance(pattern, str) and pattern.strip() for pattern in source["exclude"]):
        errors.append("policy requires include patterns and valid exclude patterns")
    limits = value.get("limits")
    if not isinstance(limits, dict) or any(not isinstance(limits.get(key), int) or limits[key] < 1 for key in ("max_file_count", "max_file_bytes", "max_total_bytes", "max_query_results")):
        errors.append("policy has invalid limits")
    return errors


def write_policy(root: Path, source_map: dict[str, Any], storage_mode: str, data_classification: str, review_mode: str, authority: str, include: list[str] | None = None, exclude: list[str] | None = None) -> None:
    template = Path(__file__).parents[2] / "templates" / "domain-memory-policy.json"
    value = load_json(template)
    value["storage_mode"] = storage_mode
    value["data_classification"] = data_classification
    value["review_mode"] = review_mode
    value["source_policy"]["selected_paths"] = source_map["selected_paths"]
    value["source_policy"]["authority"] = authority
    if include is not None:
        value["source_policy"]["include"] = include
    if exclude is not None:
        value["source_policy"]["exclude"] = exclude
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid policy: " + "; ".join(errors))
    policy_path(root).write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


AMENDABLE_FIELDS = {
    "review_mode": ("review_mode",),
    "storage_mode": ("storage_mode",),
    "data_classification": ("data_classification",),
    "source_authority": ("source_policy", "authority"),
}


def amend_policy(root: Path, field: str, value: str, reason: str) -> dict[str, Any]:
    from .audit import append as append_audit

    if field not in AMENDABLE_FIELDS:
        raise ValueError(f"{field} is not an amendable policy field; amend one of {', '.join(sorted(AMENDABLE_FIELDS))}. "
                         "Selected source paths and limits are settled when a Domain Memory is initialized.")
    if not isinstance(reason, str) or not reason.strip():
        raise ValueError("amending a Domain Memory policy requires a reason: the policy decides what the Registry may become")
    path = policy_path(root)
    if not path.is_file():
        raise ValueError(f"no Domain Memory policy at {path}")
    value_document = load_json(path)
    keys = AMENDABLE_FIELDS[field]
    target = value_document
    for key in keys[:-1]:
        target = target[key]
    change = {"operation": "amend-policy", "field": field, "from": target.get(keys[-1]), "to": value, "reason": reason.strip()}
    target[keys[-1]] = value
    errors = validate_policy(value_document)
    if errors:
        raise ValueError("amended policy is invalid: " + "; ".join(errors))
    path.write_text(json.dumps(value_document, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    append_audit(root, change)
    return change


def review_mode(root: Path) -> str:
    path = policy_path(root)
    if not path.is_file():
        return "legacy-unverified"
    value = load_json(path)
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid Domain Memory policy: " + "; ".join(errors))
    return value["review_mode"]


def approved_command_profiles(root: Path | None) -> set[str]:
    if root is None:
        return set()
    path = policy_path(root)
    if not path.is_file():
        return set()
    value = load_json(path)
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid Domain Memory policy: " + "; ".join(errors))
    profiles = value.get("approved_command_profiles", [])
    return {profile for profile in profiles if isinstance(profile, str) and profile.strip()}


def query_result_limit(root: Path) -> int:
    path = policy_path(root)
    if not path.is_file():
        return 100
    value = load_json(path)
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid Domain Memory policy: " + "; ".join(errors))
    return value["limits"]["max_query_results"]
