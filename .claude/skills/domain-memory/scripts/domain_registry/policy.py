from __future__ import annotations

from pathlib import Path
from typing import Any

from .common import change_template_dir, invalid_choice, kind_of, load_json, whole_number, write_json, writer_lock

STORAGE_MODES = ("tracked", "ignored", "external")
CLASSIFICATIONS = ("public", "internal", "confidential", "restricted")
REVIEW_MODES = ("local-draft-only", "scm-verified")
GOVERNANCE_VERIFIERS = ("git-signed-commit", "none")
GOVERNANCE_TRIGGERS = ("git-commit", "git-push", "none")
REMOVED_VERIFIER = (
    "policy names the github-pr verifier, which the plugin no longer provides: it verifies git signatures only; "
    "amend-policy --set review_verifier=git-signed-commit --set review_trigger=git-push "
    "--set authorized_signers=<fingerprint,...> --reason <text>"
)
CI_REQUIREMENTS = ("required", "optional", "none")


def existing_policy(root: Path) -> dict[str, Any] | None:
    path = policy_path(root)
    return load_json(path) if path.is_file() else None


def policy_path(root: Path) -> Path:
    return root / "domain-memory-policy.json"


def _nonblank_strings(value: Any) -> bool:
    return isinstance(value, list) and all(
        isinstance(item, str) and item.strip() for item in value
    )


ENUMERATED_FIELDS = (
    ("format", ("domain-memory-policy/v1",)),
    ("storage_mode", STORAGE_MODES),
    ("data_classification", CLASSIFICATIONS),
    ("review_mode", REVIEW_MODES),
)
LIMIT_KEYS = ("max_file_count", "max_file_bytes", "max_total_bytes", "max_query_results")
SCHEMA_CONVENTION_DEFAULTS: dict[str, Any] = {
    "exempt_columns": ["id", "created_at", "updated_at"],
    "status_like_suffixes": ["status", "state", "kind", "mode", "tier", "role", "decision", "verdict", "outcome",
                             "phase", "stage", "level", "priority", "severity", "visibility", "category"],
    "free_text_markers": ["free text", "free-text"],
    "value_comment_limit": 12,
}
CONVENTION_LISTS = ("exempt_columns", "status_like_suffixes", "free_text_markers")
PROJECT_PROFILE_DEFAULTS: dict[str, list[str]] = {
    "implementation_roots": ["apps", "src", "services", "packages", "db", "database", "migrations", "schema"],
    "excluded_directories": [],
    "decision_roots": ["docs/adr", "adr", "docs/architecture", "architecture"],
    "boundary_markers": ["doc.go"],
}
BARE_NAME_KEYS = ("excluded_directories", "boundary_markers")
GOVERNANCE_RULES = (
    (lambda mode, rules: mode == "local-draft-only" and rules["verifier"] != "none",
     "local-draft-only policy must not select a review verifier"),
    (lambda mode, rules: mode == "scm-verified" and rules["verifier"] == "none",
     "scm-verified policy requires a review verifier"),
    (lambda _mode, rules: rules["verifier"] == "git-signed-commit" and not rules["authorized_signers"],
     "git-signed-commit policy requires authorized_signers"),
    (lambda _mode, rules: rules["verifier"] == "git-signed-commit" and rules.get("trigger") != "git-push",
     "git-signed-commit policy requires git-push enforcement"),
)


def _governance_error(review_mode: Any, governance: Any) -> str | None:
    if not isinstance(governance, dict):
        return "policy requires review_governance"
    if governance.get("verifier") == "github-pr" or governance.get("trigger") == "external-scm":
        return REMOVED_VERIFIER
    if "distinct_signer" in governance and not isinstance(governance["distinct_signer"], bool):
        return f"review_governance.distinct_signer must be true or false, got {governance['distinct_signer']!r}"
    if (
        governance.get("verifier") not in GOVERNANCE_VERIFIERS
        or governance.get("trigger") not in GOVERNANCE_TRIGGERS
        or governance.get("ci_requirement") not in CI_REQUIREMENTS
        or not _nonblank_strings(governance.get("authorized_signers"))
    ):
        return "policy has invalid review_governance"
    return next(
        (message for broken, message in GOVERNANCE_RULES if broken(review_mode, governance)), None
    )


def _source_error(source: Any) -> str | None:
    if (
        not isinstance(source, dict)
        or not _nonblank_strings(source.get("selected_paths"))
        or not source["selected_paths"]
        or not isinstance(source.get("authority"), str)
        or not source["authority"].strip()
    ):
        return "policy requires selected source paths and authority"
    if (
        not _nonblank_strings(source.get("include"))
        or not source["include"]
        or not _nonblank_strings(source.get("exclude"))
    ):
        return "policy requires include patterns and valid exclude patterns"
    return None


def _limits_error(limits: Any) -> str | None:
    if not isinstance(limits, dict) or any(
        not whole_number(limits.get(key)) or limits[key] < 1 for key in LIMIT_KEYS
    ):
        return "policy has invalid limits"
    return None


def schema_conventions(policy: dict[str, Any] | None) -> dict[str, Any]:
    given = policy.get("schema_conventions") if policy else None
    return {**SCHEMA_CONVENTION_DEFAULTS, **(given if isinstance(given, dict) else {})}


def _conventions_errors(value: Any) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, dict):
        return [f"schema_conventions must be an object, got {kind_of(value)}"]
    errors = [f"schema_conventions.{key} is not a known key{_closest(key)}" for key in value if key not in SCHEMA_CONVENTION_DEFAULTS]
    for key in CONVENTION_LISTS:
        if key in value:
            errors.extend(_list_errors(f"schema_conventions.{key}", value[key]))
    limit = value.get("value_comment_limit", 0)
    if not whole_number(limit) or limit < 0:
        errors.append(f"schema_conventions.value_comment_limit must be a whole number 0 or more, got {limit!r}")
    return errors


def _list_errors(name: str, items: Any) -> list[str]:
    if not isinstance(items, list):
        return [f"{name} must be a list, got {kind_of(items)}"]
    return [f"{name}[{index}] is blank" if isinstance(item, str) else f"{name}[{index}] must be a string, got {kind_of(item)}"
            for index, item in enumerate(items) if not (isinstance(item, str) and item.strip())]


def _closest(key: str, known: dict[str, Any] = SCHEMA_CONVENTION_DEFAULTS) -> str:
    import difflib

    close = difflib.get_close_matches(key, list(known), n=1, cutoff=0.6)
    return f"; did you mean {close[0]}?" if close else ""


def profile_from(section: Any) -> dict[str, list[str]]:
    given = section if isinstance(section, dict) else {}
    return {key: list(given[key]) if isinstance(given.get(key), list) else list(default)
            for key, default in PROJECT_PROFILE_DEFAULTS.items()}


def project_profile(policy: dict[str, Any] | None) -> dict[str, list[str]]:
    return profile_from(policy.get("project_profile") if policy else None)


def excluded_directories(policy: dict[str, Any] | None) -> frozenset[str]:
    from .sources import EXCLUDED_DIRECTORIES

    return EXCLUDED_DIRECTORIES | frozenset(project_profile(policy)["excluded_directories"])


def profile_errors(value: Any) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, dict):
        return [f"project_profile must be an object, got {kind_of(value)}"]
    errors = [f"project_profile.{key} is not a known key{_closest(key, PROJECT_PROFILE_DEFAULTS)}"
              for key in value if key not in PROJECT_PROFILE_DEFAULTS]
    for key in PROJECT_PROFILE_DEFAULTS:
        if key in value:
            errors.extend(_list_errors(f"project_profile.{key}", value[key]) or _entry_errors(key, value[key]))
    return errors


def _entry_errors(key: str, items: list[str]) -> list[str]:
    if key in BARE_NAME_KEYS:
        return [f"project_profile.{key}[{index}] must be a bare name without a path separator, got {item!r}"
                for index, item in enumerate(items) if "/" in item or "\\" in item]
    return [f"project_profile.{key}[{index}] must be a relative path inside the repository, got {item!r}"
            for index, item in enumerate(items)
            if item.startswith(("/", "\\")) or ".." in item.replace("\\", "/").split("/") or ":" in item]


def validate_policy(value: dict[str, Any]) -> list[str]:
    errors = [
        invalid_choice("policy", key, value.get(key), allowed)
        for key, allowed in ENUMERATED_FIELDS
        if value.get(key) not in allowed
    ]
    found = (
        _governance_error(value.get("review_mode"), value.get("review_governance")),
        _source_error(value.get("source_policy")),
        _limits_error(value.get("limits")),
    )
    return (errors + [error for error in found if error] + _conventions_errors(value.get("schema_conventions"))
            + profile_errors(value.get("project_profile")))


def _default_governance(review_mode: str) -> dict[str, Any]:
    local = review_mode == "local-draft-only"
    return {
        "verifier": "none" if local else "git-signed-commit",
        "trigger": "none" if local else "git-commit",
        "ci_requirement": "none",
        "authorized_signers": [],
    }


def write_policy(  # noqa: PLR0913, PLR0917
    root: Path,
    source_map: dict[str, Any],
    storage_mode: str,
    data_classification: str,
    review_mode: str,
    authority: str,
    include: list[str] | None = None,
    exclude: list[str] | None = None,
    governance: dict[str, Any] | None = None,
    profile: dict[str, list[str]] | None = None,
) -> None:
    template = change_template_dir() / "domain-memory-policy.json"
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
    value["project_profile"] = profile_from(profile)
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid policy: " + "; ".join(errors))
    write_json(policy_path(root), value)


AMENDABLE_FIELDS = {
    "review_mode": ("review_mode",),
    "storage_mode": ("storage_mode",),
    "data_classification": ("data_classification",),
    "source_authority": ("source_policy", "authority"),
    "review_verifier": ("review_governance", "verifier"),
    "review_trigger": ("review_governance", "trigger"),
    "ci_requirement": ("review_governance", "ci_requirement"),
    "authorized_signers": ("review_governance", "authorized_signers"),
    "distinct_signer": ("review_governance", "distinct_signer"),
    "implementation_roots": ("project_profile", "implementation_roots"),
    "excluded_directories": ("project_profile", "excluded_directories"),
    "decision_roots": ("project_profile", "decision_roots"),
    "boundary_markers": ("project_profile", "boundary_markers"),
}

LIST_FIELDS = {"authorized_signers", *PROJECT_PROFILE_DEFAULTS}
BOOLEAN_FIELDS = {"distinct_signer"}


def _amended_value(field: str, value: str) -> Any:
    if field in LIST_FIELDS:
        return [entry.strip() for entry in value.split(",") if entry.strip()]
    if field in BOOLEAN_FIELDS:
        if value.strip().lower() not in ("true", "false"):
            raise ValueError(f"{field} takes true or false, got {value!r}")
        return value.strip().lower() == "true"
    return value


def _field_parent(document: dict[str, Any], field: str) -> dict[str, Any]:
    target = document
    for key in AMENDABLE_FIELDS[field][:-1]:
        target = target.setdefault(key, {})
    return target


def _apply_field(
    document: dict[str, Any], field: str, value: str, verifier: str | None
) -> dict[str, Any]:
    key = AMENDABLE_FIELDS[field][-1]
    target = _field_parent(document, field)
    amended = _amended_value(field, value)
    change = {"operation": "amend-policy", "field": field, "to": amended}
    target[key] = amended
    if field == "review_mode" and value == "local-draft-only":
        document["review_governance"] = _default_governance(value)
    elif field == "review_mode" and value == "scm-verified":
        if not isinstance(verifier, str) or not verifier.strip():
            raise ValueError(
                "leaving local-draft-only requires the verifier that replaces it, "
                f"one of {', '.join(sorted(verifier for verifier in GOVERNANCE_VERIFIERS if verifier != 'none'))}: "
                "a review mode and the authority that backs it move together"
            )
        document["review_governance"]["verifier"] = verifier.strip()
        change["verifier"] = verifier.strip()
    return change


def amend_policy_fields(
    root: Path, assignments: list[tuple[str, str]], reason: str, verifier: str | None = None
) -> list[dict[str, Any]]:
    from .audit import append_locked

    fields = [field for field, _ in assignments]
    unknown = [field for field in fields if field not in AMENDABLE_FIELDS]
    if not assignments or unknown:
        raise ValueError(
            f"{', '.join(unknown) or 'nothing'} is not an amendable policy field; amend one of "
            f"{', '.join(sorted(AMENDABLE_FIELDS))}. "
            "Selected source paths and limits are settled when a Domain Memory is initialized."
        )
    repeated = sorted({field for field in fields if fields.count(field) > 1})
    if repeated:
        raise ValueError(f"{', '.join(repeated)} is named more than once; give each field one value")
    if not isinstance(reason, str) or not reason.strip():
        raise ValueError(
            "amending a Domain Memory policy requires a reason: the policy decides what the Registry may become"
        )
    path = policy_path(root)
    if not path.is_file():
        raise ValueError(f"no Domain Memory policy at {path}")
    document = load_json(path)
    before = load_json(path)
    values = dict(assignments)
    mode_first = sorted(assignments, key=lambda assignment: assignment[0] != "review_mode")
    changes = [
        {
            **_apply_field(document, field, value, verifier or values.get("review_verifier")),
            "from": _field_parent(before, field).get(AMENDABLE_FIELDS[field][-1]),
            "reason": reason.strip(),
        }
        for field, value in mode_first
    ]
    errors = validate_policy(document)
    if errors:
        raise ValueError("amended policy is invalid: " + "; ".join(errors))
    with writer_lock(root):
        write_json(path, document)
        for change in changes:
            append_locked(root, change)
    return changes


def _valid_policy(path: Path) -> dict[str, Any]:
    value = load_json(path)
    errors = validate_policy(value)
    if errors:
        raise ValueError("invalid Domain Memory policy: " + "; ".join(errors))
    return value


def _valid_policy_if_any(root: Path) -> dict[str, Any] | None:
    path = policy_path(root)
    return _valid_policy(path) if path.is_file() else None


def review_mode(root: Path) -> str:
    policy = _valid_policy_if_any(root)
    return policy["review_mode"] if policy else "no-policy"


def review_governance(root: Path) -> dict[str, Any]:
    return _valid_policy(policy_path(root))["review_governance"]


def approved_command_profiles(root: Path | None) -> set[str]:
    policy = _valid_policy_if_any(root) if root is not None else None
    if policy is None:
        return set()
    profiles = policy.get("approved_command_profiles", [])
    return {
        profile for profile in profiles if isinstance(profile, str) and profile.strip()
    }


def query_result_limit(root: Path) -> int:
    policy = _valid_policy_if_any(root)
    return policy["limits"]["max_query_results"] if policy else 100
