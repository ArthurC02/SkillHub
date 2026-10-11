from __future__ import annotations

from pathlib import Path
from typing import Any

from .common import load_json
from .registry import coverage, validate
from .sources import discover_sources, verify_source_map

PRODUCT_GROUPS = (
    "decisions",
    "requirements",
    "contracts",
    "bounded_contexts",
    "implementation",
    "tests",
)
UNUSABLE_REGISTRY_STATUSES = {"invalid", "unverified", "stale"}


def registry_condition(
    repo_root: Path, registry_path: Path
) -> tuple[str, int, list[str]]:
    if not registry_path.exists():
        return "absent", 0, []
    source_map = registry_path / "source-map.json"
    if not source_map.is_file():
        return "unverified", 0, ["Domain Memory exists without a source map."]
    policy_path = registry_path / "domain-memory-policy.json"
    try:
        policy = load_json(policy_path) if policy_path.is_file() else None
        source_result = verify_source_map(repo_root, source_map, policy)
    except ValueError as error:
        return "invalid", 0, [f"Domain Memory source map cannot be read: {error}"]
    if source_result["status"] != "current":
        return (
            source_result["status"],
            0,
            [
                "Domain Memory sources require review "
                "before they can guide implementation."
            ],
        )
    errors = validate(registry_path, repo_root, False)  # noqa: FBT003
    if errors:
        return (
            "invalid",
            0,
            ["Domain Memory Registry is invalid: " + "; ".join(errors[:3])],
        )
    report = coverage(registry_path)
    records = sum(
        report[key]
        for key in ("contexts", "vocabulary_terms", "aggregates", "rules", "contracts")
    )
    manifest = load_json(registry_path / "registry" / "manifest.json")
    reviewed_empty = records == 0 and manifest.get("status") == "reviewed"
    return ("reviewed-empty" if reviewed_empty else "current"), records, []


def assess_readiness(
    repo_root: Path, registry_root: Path | None = None
) -> dict[str, Any]:
    repo_root = repo_root.resolve()
    discovered = discover_sources(repo_root)
    groups = {group["kind"]: group["paths"] for group in discovered["source_groups"]}
    present = {kind: bool(groups.get(kind)) for kind in PRODUCT_GROUPS}
    product_sources = any(present.values())
    signals: list[dict[str, Any]] = [
        {"kind": kind, "status": "present" if present[kind] else "missing"}
        for kind in PRODUCT_GROUPS
    ]
    blocks: list[str] = []
    registry_status = "absent"
    registry_records = 0
    if registry_root:
        registry_status, registry_records, blocks = registry_condition(
            repo_root, registry_root.resolve()
        )
        signals.append({"kind": "registry", "status": registry_status})
        signals.append({"kind": "registry_records", "count": registry_records})

    dead = (
        bool(registry_root)
        and not product_sources
        and registry_status in UNUSABLE_REGISTRY_STATUSES
    )
    if dead:
        state, next_capability = "dead", "recover-or-reconfirm"
    elif not product_sources and registry_records == 0:
        state, next_capability = "empty", "discover"
    elif not present["implementation"]:
        state, next_capability = "greenfield", "design"
    else:
        state, next_capability = "brownfield", "read-and-maintain"
    if registry_status in UNUSABLE_REGISTRY_STATUSES:
        next_capability = "recover-or-reconfirm"
    return {
        "format": "domain-memory-readiness/v1",
        "state": state,
        "confidence": "medium" if dead else "high",
        "signals": signals,
        "governance_candidates": discovered["governance_candidates"],
        "blocks": blocks,
        "next_capability": next_capability,
    }
