from __future__ import annotations

from pathlib import Path
from typing import Any

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
        registry_path = registry_root.resolve()
        source_map = registry_path / "source-map.json"
        if not registry_path.exists():
            registry_status = "absent"
        elif not source_map.is_file():
            registry_status = "unverified"
            blocks.append("Domain Memory exists without a source map.")
        else:
            try:
                source_result = verify_source_map(repo_root, source_map)
            except ValueError as error:
                registry_status = "invalid"
                blocks.append(f"Domain Memory source map cannot be read: {error}")
            else:
                registry_status = source_result["status"]
                if registry_status != "current":
                    blocks.append(
                        "Domain Memory sources require review before they can guide implementation."
                    )
            if registry_status == "current":
                errors = validate(registry_path, repo_root, False)
                if errors:
                    registry_status = "invalid"
                    blocks.append(
                        "Domain Memory Registry is invalid: " + "; ".join(errors[:3])
                    )
                else:
                    report = coverage(registry_path)
                    registry_records = sum(
                        report[key]
                        for key in (
                            "contexts",
                            "vocabulary_terms",
                            "aggregates",
                            "rules",
                            "contracts",
                        )
                    )
        signals.append({"kind": "registry", "status": registry_status})
        signals.append({"kind": "registry_records", "count": registry_records})

    implementation = present["implementation"]
    dead = (
        bool(registry_root)
        and not product_sources
        and registry_status in UNUSABLE_REGISTRY_STATUSES
    )
    if dead:
        state, next_capability = "dead", "recover-or-reconfirm"
    elif not product_sources and registry_records == 0:
        state, next_capability = "empty", "discover"
    elif not implementation:
        state, next_capability = "greenfield", "design"
    else:
        state, next_capability = "brownfield", "read-and-maintain"
    if registry_status in UNUSABLE_REGISTRY_STATUSES:
        next_capability = "recover-or-reconfirm"
    return {
        "format": "domain-memory-readiness/v1",
        "state": state,
        "confidence": "high" if state != "dead" else "medium",
        "signals": signals,
        "blocks": blocks,
        "next_capability": next_capability,
    }
