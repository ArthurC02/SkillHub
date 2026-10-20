#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import time
from collections.abc import Iterator
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

MAX_JSON_BYTES = 16 * 1024 * 1024
MAX_EVIDENCE_BYTES = 16 * 1024 * 1024
MAX_NESTING_DEPTH = 100
GIT_COMMAND_TIMEOUT_SECONDS = 15
REPLACE_DELAYS_SECONDS = (0.05, 0.1, 0.2, 0.4, 0.8)

ASSET_KEYS = {
    "contexts.json": "contexts",
    "vocabulary.json": "terms",
    "aggregates.json": "aggregates",
    "rules.json": "rules",
    "contracts.json": "contracts",
    "interactions.json": "interactions",
    "decisions.json": "decisions",
    "events.json": "events",
    "capabilities.json": "capabilities",
    "value-objects.json": "value_objects",
    "dependency-policies.json": "dependencies",
}

ASSET_FORMATS = {
    "contexts.json": "domain-contexts/v1",
    "vocabulary.json": "domain-vocabulary/v1",
    "aggregates.json": "domain-aggregates/v1",
    "rules.json": "domain-rules/v1",
    "contracts.json": "domain-contracts/v1",
    "interactions.json": "domain-interactions/v1",
    "decisions.json": "domain-decisions/v1",
    "events.json": "domain-events/v1",
    "capabilities.json": "domain-capabilities/v1",
    "value-objects.json": "domain-value-objects/v1",
    "dependency-policies.json": "domain-dependencies/v1",
}


def write_json(path: Path, value: dict) -> None:
    temporary = path.with_suffix(f"{path.suffix}.tmp")
    with temporary.open("w", encoding="utf-8", newline="\n") as output:
        output.write(json.dumps(value, indent=2, ensure_ascii=False) + "\n")
        output.flush()
        os.fsync(output.fileno())
    try:
        replace_path(temporary, path)
    except PermissionError:
        temporary.unlink(missing_ok=True)
        raise


def replace_path(source: Path, target: Path) -> None:
    for delay in (*REPLACE_DELAYS_SECONDS, None):
        try:
            source.replace(target)
            break
        except PermissionError:
            if delay is None:
                raise
            time.sleep(delay)


def review_field(asset: str) -> str:
    return "review_status" if asset == "decisions" else "status"


def record_status(asset: str, document: dict[str, Any], record: dict[str, Any]) -> Any:
    return record.get(review_field(asset), document.get("status"))


def is_reviewed(asset: str, document: dict[str, Any], record: dict[str, Any] | None) -> bool:
    return record is not None and record_status(asset, document, record) == "reviewed"


def find_record(records: Any, record_id: Any) -> dict[str, Any] | None:
    return next((entry for entry in records if isinstance(entry, dict) and entry.get("id") == record_id), None)


def updates_record(update: Any, asset: str, record_id: Any) -> bool:
    return update.get("asset") == asset and (update.get("record") or {}).get("id") == record_id


def shown_path(path: Path, top: Path | None) -> str:
    resolved = path.resolve()
    if top is not None and resolved.is_relative_to(top.resolve()):
        return resolved.relative_to(top.resolve()).as_posix()
    return resolved.as_posix()


PROPOSAL_FILE = "domain-change-proposal.json"
REQUIREMENT_FILE = "requirement-normalization.json"
OBLIGATIONS_FILE = "test-obligations.json"
EVIDENCE_FILE = "evidence-bundle.json"
APPLY_OPERATION = "apply-approved-updates"
FAILING_EVIDENCE = ("stale", "unhashed", "missing", "invalid")

CHANGE_PACKAGE_FILES = (
    REQUIREMENT_FILE,
    PROPOSAL_FILE,
    OBLIGATIONS_FILE,
    EVIDENCE_FILE,
    "draft-pr.md",
)

CHANGE_PACKAGE_FORMATS = {
    REQUIREMENT_FILE: "domain-requirement/v1",
    PROPOSAL_FILE: "domain-change-proposal/v1",
    OBLIGATIONS_FILE: "domain-test-obligations/v1",
    EVIDENCE_FILE: "domain-evidence-bundle/v1",
}

CHANGE_CLASSIFICATIONS = {"routine", "material"}

RISK_FLAGS = {
    "cross_context",
    "public_contract",
    "regulated_rule",
    "sensitive_data",
    "financial_decision",
    "irreversible_change",
}

PROPOSAL_STATUSES = {
    "draft",
    "submitted",
    "verified",
    "approved",
    "rejected",
    "superseded",
    "applied",
}
OBLIGATION_SOURCE_TYPES = {"acceptance-criterion", "rule", "contract", "invariant"}
OBLIGATION_STATUSES = {"planned", "executed", "blocked"}
REGISTRY_STATUSES = {"candidate", "reviewed", "deprecated", "superseded"}
CONTEXT_SUBDOMAINS = {"core", "supporting", "generic"}
CONTEXT_RELATIONSHIPS = {
    "partnership",
    "shared-kernel",
    "customer-supplier",
    "conformist",
    "anticorruption-layer",
    "open-host-service",
    "published-language",
}
DEPENDENCY_MODES = {"synchronous-query", "synchronous-write", "asynchronous", "none"}
DEPENDENCY_POLICIES = {"allowed", "forbidden"}
CLOSED_FIELDS = {
    "interactions.json": {"relationship": CONTEXT_RELATIONSHIPS},
    "dependency-policies.json": {"mode": DEPENDENCY_MODES, "policy": DEPENDENCY_POLICIES},
}
REVIEW_REQUIRED_FIELDS = {
    "contexts.json": ("name", "responsibility", "verified"),
    "vocabulary.json": ("name", "definition", "contexts", "verified"),
    "aggregates.json": ("context", "root", "invariants", "verified"),
    "rules.json": ("contexts", "statement", "verified"),
    "contracts.json": (
        "kind",
        "producer_context",
        "consumer_contexts",
        "version",
        "compatibility_policy",
        "data_classification",
        "verified",
    ),
    "interactions.json": (
        "producer_context",
        "consumer_context",
        "consistency",
        "delivery",
        "verified",
    ),
    "decisions.json": ("statement", "source", "verified"),
    "events.json": ("owner_context", "meaning", "schema", "verified"),
    "capabilities.json": ("context", "meaning", "verified"),
    "value-objects.json": ("context", "meaning", "fields", "verified"),
    "dependency-policies.json": (
        "from_context",
        "to_context",
        "mode",
        "policy",
        "verified",
    ),
}


def reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    for key, item in pairs:
        if key in value:
            raise ValueError(f"duplicate JSON key: {key}")
        value[key] = item
    return value


DOMAIN_MEMORY_HINT = (
    "check --registry-root (a Registry holds registry/index.md or registry/manifest.json, "
    "or a domain-source-map/v1 source-map.json); "
    "initialize one only if this repository has none yet"
)


SOURCE_MAP_FORMAT = "domain-source-map/v1"
REGISTRY_MARKERS = ("index.md", "manifest.json")


def holds_domain_memory(path: Path) -> bool:
    return is_domain_source_map(path / "source-map.json") or any(
        (path / "registry" / marker).is_file() for marker in REGISTRY_MARKERS
    )


def is_domain_source_map(path: Path) -> bool:
    try:
        return load_json(path).get("format") == SOURCE_MAP_FORMAT
    except (OSError, ValueError, AttributeError):
        return False


def one_level_too_deep_hint(path: Path) -> str:
    nearby = path.parent.parent / path.name
    if not nearby.is_file():
        return ""
    return (
        f"; {nearby} exists, so a root option probably points one directory too deep: a registry root "
        "holds registry/, source-map.json, and the policy"
    )


def load_json(path: Path) -> dict[str, Any]:
    if not path.is_file():
        raise ValueError(f"JSON file required: {path}{one_level_too_deep_hint(path)}")
    if path.stat().st_size > MAX_JSON_BYTES:
        raise ValueError(f"JSON file exceeds {MAX_JSON_BYTES} bytes: {path}")
    try:
        with path.open(encoding="utf-8") as source:
            value = json.load(source, object_pairs_hook=reject_duplicate_keys)
    except (ValueError, RecursionError) as error:
        raise ValueError(f"invalid JSON: {path}") from error
    except OSError as error:
        raise ValueError(f"cannot read JSON file: {path}") from error
    if not isinstance(value, dict):
        raise ValueError(f"JSON object required: {path}")
    return value


def iter_packages(changes_root: Path) -> Iterator[tuple[Path, dict[str, Any] | ValueError]]:
    for package in sorted(path for path in changes_root.iterdir() if (path / PROPOSAL_FILE).is_file()):
        yield package, _proposal_or_error(package)


def _proposal_or_error(package: Path) -> dict[str, Any] | ValueError:
    try:
        return load_json(package / PROPOSAL_FILE)
    except ValueError as error:
        return error


def readable_packages(changes_root: Path) -> list[tuple[Path, dict[str, Any]]]:
    packages = []
    for package, proposal in iter_packages(changes_root):
        if isinstance(proposal, ValueError):
            raise ValueError(f"Change Package {package.name} cannot be read: {proposal}")
        packages.append((package, proposal))
    return packages


def registry_dir(root: Path) -> Path:
    return root / "registry"


@contextmanager
def writer_lock(root: Path):
    lock = root / ".domain-registry.lock"
    try:
        lock.mkdir()
    except FileExistsError as error:
        raise ValueError(
            f"another Domain Registry update is in progress: {lock}"
        ) from error
    try:
        yield
    finally:
        lock.rmdir()


def template_dir() -> Path:
    return Path(__file__).parents[2] / "templates" / "domain-registry"


def change_template_dir() -> Path:
    return Path(__file__).parents[2] / "templates"


def completed_identifier(value: Any) -> bool:
    return isinstance(value, str) and bool(value.strip()) and not value.startswith("<")


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def iso_timestamp(value: Any) -> bool:
    if not completed_identifier(value):
        return False
    try:
        datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return False
    return True
