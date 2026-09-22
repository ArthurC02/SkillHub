#!/usr/bin/env python3

from __future__ import annotations

import json
from contextlib import contextmanager
from datetime import datetime
from pathlib import Path
from typing import Any

MAX_JSON_BYTES = 16 * 1024 * 1024
MAX_EVIDENCE_BYTES = 16 * 1024 * 1024
MAX_NESTING_DEPTH = 100
GIT_COMMAND_TIMEOUT_SECONDS = 15

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

CHANGE_PACKAGE_FILES = (
    "requirement-normalization.json",
    "domain-change-proposal.json",
    "test-obligations.json",
    "evidence-bundle.json",
    "draft-pr.md",
)

CHANGE_PACKAGE_FORMATS = {
    "requirement-normalization.json": "domain-requirement/v1",
    "domain-change-proposal.json": "domain-change-proposal/v1",
    "test-obligations.json": "domain-test-obligations/v1",
    "evidence-bundle.json": "domain-evidence-bundle/v1",
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
CONTEXT_SUBDOMAINS = {"core", "supporting", "generic", "shared-kernel"}
REVIEW_REQUIRED_FIELDS = {
    "contexts.json": ("name", "responsibility", "review"),
    "vocabulary.json": ("name", "definition", "contexts", "review"),
    "aggregates.json": ("context", "root", "invariants", "review"),
    "rules.json": ("contexts", "statement", "review"),
    "contracts.json": (
        "kind",
        "producer_context",
        "consumer_contexts",
        "version",
        "compatibility_policy",
        "data_classification",
        "review",
    ),
    "interactions.json": (
        "producer_context",
        "consumer_context",
        "consistency",
        "delivery",
        "review",
    ),
    "decisions.json": ("statement", "source", "review"),
    "events.json": ("owner_context", "meaning", "schema", "review"),
    "capabilities.json": ("context", "meaning", "review"),
    "value-objects.json": ("context", "meaning", "fields", "review"),
    "dependency-policies.json": (
        "from_context",
        "to_context",
        "mode",
        "policy",
        "review",
    ),
}


def reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    for key, item in pairs:
        if key in value:
            raise ValueError(f"duplicate JSON key: {key}")
        value[key] = item
    return value


def load_json(path: Path) -> dict[str, Any]:
    if not path.is_file():
        raise ValueError(f"JSON file required: {path}")
    if path.stat().st_size > MAX_JSON_BYTES:
        raise ValueError(f"JSON file exceeds {MAX_JSON_BYTES} bytes: {path}")
    try:
        with path.open(encoding="utf-8") as source:
            value = json.load(source, object_pairs_hook=reject_duplicate_keys)
    except (json.JSONDecodeError, ValueError) as error:
        raise ValueError(f"invalid JSON: {path}") from error
    except OSError as error:
        raise ValueError(f"cannot read JSON file: {path}") from error
    if not isinstance(value, dict):
        raise ValueError(f"JSON object required: {path}")  # noqa: TRY004
    return value


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


def iso_timestamp(value: Any) -> bool:
    if not completed_identifier(value):
        return False
    try:
        datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return False
    return True
