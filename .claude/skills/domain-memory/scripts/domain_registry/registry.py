import json
import shutil
from collections.abc import Callable
from pathlib import Path
from typing import Any

from .common import (
    ASSET_FORMATS,
    ASSET_KEYS,
    REGISTRY_STATUSES,
    REVIEW_REQUIRED_FIELDS,
    load_json,
    registry_dir,
    template_dir,
)
from .evidence import all_references, classified, migrate_legacy, source_map_for, verify
from .policy import query_result_limit, review_mode


def init_registry(output: Path) -> None:
    if output.exists() and any(output.iterdir()):
        raise FileExistsError(f"output directory is not empty: {output}")
    target_registry = registry_dir(output)
    target_registry.mkdir(parents=True, exist_ok=True)
    templates = template_dir()
    for name in set(ASSET_KEYS) | {"manifest.json"}:
        shutil.copyfile(templates / name, target_registry / name)


def migrate_registry(root: Path) -> list[str]:
    target = registry_dir(root)
    templates = template_dir()
    created = []
    for name in set(ASSET_KEYS) | {"manifest.json"}:
        path = target / name
        if not path.exists():
            shutil.copyfile(templates / name, path)
            created.append(name)
    manifest = load_json(target / "manifest.json")
    artifacts = manifest.setdefault("artifacts", [])
    for name in ASSET_KEYS:
        if name not in artifacts:
            artifacts.append(name)
    (target / "manifest.json").write_text(
        json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )
    return created


def asset_records(root: Path, name: str) -> list[dict[str, Any]]:
    value = load_json(registry_dir(root) / name).get(ASSET_KEYS[name])
    if not isinstance(value, list) or not all(
        isinstance(record, dict) for record in value
    ):
        raise ValueError(f"{name} must contain a {ASSET_KEYS[name]} list of objects")
    return value


def evidence_values(value: Any) -> list[Any]:
    return all_references(value)


def _load_registry_assets(
    root: Path, errors: list[str]
) -> dict[str, list[dict[str, Any]]]:
    records: dict[str, list[dict[str, Any]]] = {}
    for name in ASSET_KEYS:
        path = registry_dir(root) / name
        if not path.is_file():
            errors.append(f"missing asset: {path}")
            continue
        try:
            records[name] = asset_records(root, name)
        except ValueError as error:
            errors.append(str(error))
    return records


def _validate_asset_records(
    root: Path,
    records: dict[str, list[dict[str, Any]]],
    require_reviewed: bool,
) -> list[str]:
    errors: list[str] = []
    for name, entries in records.items():
        asset = load_json(registry_dir(root) / name)
        expected_format = ASSET_FORMATS[name]
        if asset.get("format") != expected_format:
            errors.append(f"{name} has an invalid format")
        asset_status = asset.get("status")
        if asset_status not in REGISTRY_STATUSES:
            errors.append(f"{name} has an invalid status")
        identifiers = [entry.get("id") for entry in entries]
        if not all(
            isinstance(identifier, str) and identifier for identifier in identifiers
        ):
            errors.append(f"{name} entries require non-empty id")
        elif len(identifiers) != len(set(identifiers)):
            errors.append(f"{name} contains duplicate id")
        for entry in entries:
            status = (
                entry.get("review_status", asset_status)
                if name == "decisions.json"
                else entry.get("status", asset_status)
            )
            if status not in REGISTRY_STATUSES:
                errors.append(f"{name}:{entry.get('id')} has an invalid status")
                continue
            if status == "reviewed":
                missing = [
                    field
                    for field in REVIEW_REQUIRED_FIELDS[name]
                    if not entry.get(field)
                ]
                if missing:
                    errors.append(
                        f"{name}:{entry.get('id')} is reviewed but missing {', '.join(missing)}"
                    )
                if not entry.get("evidence"):
                    errors.append(
                        f"{name}:{entry.get('id')} is reviewed but lacks evidence"
                    )
            if require_reviewed and status != "reviewed":
                errors.append(f"{name}:{entry.get('id')} is not reviewed")
        if require_reviewed and asset_status != "reviewed":
            errors.append(f"{name} is not reviewed")
    return errors


def _validate_confirmed_absences(
    records: dict[str, list[dict[str, Any]]],
) -> list[str]:
    errors: list[str] = []
    holding = contexts_holding(records)
    for entry in records.get("contexts.json", []):
        declarations = entry.get("confirmed_absences", [])
        if not isinstance(declarations, list):
            errors.append(
                f"contexts.json:{entry.get('id')} confirmed_absences must be a list"
            )
            continue
        for declaration in declarations:
            if (
                not isinstance(declaration, dict)
                or declaration.get("asset") not in ABSENCE_ASSETS
            ):
                errors.append(
                    f"contexts.json:{entry.get('id')} confirms an absence of an unknown asset; "
                    f"use one of {', '.join(sorted(ABSENCE_ASSETS))}"
                )
                continue
            if (
                not isinstance(declaration.get("reason"), str)
                or not declaration["reason"].strip()
            ):
                errors.append(
                    f"contexts.json:{entry.get('id')} confirms an absence of {declaration['asset']} "
                    "without a reason; a confirmed absence is a finding and needs one"
                )
            elif entry.get("id") in holding[declaration["asset"]]:
                errors.append(
                    f"contexts.json:{entry.get('id')} confirms an absence of {declaration['asset']}, "
                    f"but {declaration['asset']} records name it"
                )
    return errors


def _validate_context_references(
    records: dict[str, list[dict[str, Any]]],
) -> list[str]:
    errors: list[str] = []
    known_contexts = {entry.get("id") for entry in records.get("contexts.json", [])}
    for name in ("vocabulary.json", "rules.json"):
        for entry in records.get(name, []):
            contexts = entry.get("contexts")
            if not isinstance(contexts, list) or not contexts:
                errors.append(f"{name}:{entry.get('id')} requires contexts")
            elif set(contexts) - known_contexts:
                errors.append(f"{name}:{entry.get('id')} references unknown context")
    for name in ("aggregates.json", "interactions.json"):
        for entry in records.get(name, []):
            fields = (
                ["context"]
                if name == "aggregates.json"
                else ["producer_context", "consumer_context"]
            )
            for field in fields:
                if entry.get(field) not in known_contexts:
                    errors.append(
                        f"{name}:{entry.get('id')} references unknown context"
                    )
    contract_ids = {entry.get("id") for entry in records.get("contracts.json", [])}
    for name, field in (
        ("events.json", "owner_context"),
        ("capabilities.json", "context"),
        ("value-objects.json", "context"),
    ):
        for entry in records.get(name, []):
            if entry.get(field) not in known_contexts:
                errors.append(f"{name}:{entry.get('id')} references unknown context")
    for entry in records.get("dependency-policies.json", []):
        if (
            entry.get("from_context") not in known_contexts
            or entry.get("to_context") not in known_contexts
        ):
            errors.append(
                f"dependency-policies.json:{entry.get('id')} references unknown context"
            )
    for entry in records.get("contracts.json", []):
        if entry.get("producer_context") not in known_contexts:
            errors.append(
                f"contracts.json:{entry.get('id')} references unknown producer context"
            )
        consumers = entry.get("consumer_contexts", [])
        if not isinstance(consumers, list) or set(consumers) - known_contexts:
            errors.append(
                f"contracts.json:{entry.get('id')} references unknown consumer context"
            )
    for entry in records.get("interactions.json", []):
        contract_id = entry.get("contract_id")
        if contract_id is not None and contract_id not in contract_ids:
            errors.append(
                f"interactions.json:{entry.get('id')} references unknown contract"
            )
    return errors


def _validate_evidence(
    records: dict[str, list[dict[str, Any]]], repo_root: Path | None
) -> list[str]:
    if repo_root is None:
        return []
    errors: list[str] = []
    for reference in evidence_values(records):
        result = verify(reference, repo_root)
        if result["status"] in {"invalid", "missing"}:
            errors.append(
                f"evidence {result['status']}: {result.get('reason', result.get('path'))}"
            )
    return errors


def validate(root: Path, repo_root: Path | None, require_reviewed: bool) -> list[str]:
    manifest_path = registry_dir(root) / "manifest.json"
    if not manifest_path.is_file():
        return [f"missing manifest: {manifest_path}"]
    errors: list[str] = []
    if require_reviewed and review_mode(root) != "scm-verified":
        errors.append("local-draft-only Domain Memory cannot satisfy --require-reviewed")
    manifest = load_json(manifest_path)
    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, list) or not set(ASSET_KEYS).issubset(artifacts):
        errors.append("manifest artifacts must list every standard registry asset file")
    records = _load_registry_assets(root, errors)
    errors.extend(_validate_asset_records(root, records, require_reviewed))
    errors.extend(_validate_confirmed_absences(records))
    errors.extend(_validate_context_references(records))
    errors.extend(_validate_evidence(records, repo_root))
    return errors


def verify_evidence(root: Path, repo_root: Path) -> dict[str, Any]:
    source_map = source_map_for(root)
    results = [
        verify(classified(reference, source_map), repo_root)
        for reference in evidence_values(
            {name: asset_records(root, name) for name in ASSET_KEYS}
        )
    ]
    summary: dict[str, Any] = {
        status: sum(item["status"] == status for item in results)
        for status in ("current", "stale", "missing", "invalid", "legacy-unverified")
    }
    kinds = sorted({item["source_kind"] for item in results if "source_kind" in item})
    summary["by_source_kind"] = {
        kind: sum(item.get("source_kind") == kind for item in results) for kind in kinds
    }
    return {"results": results, "summary": summary}


def migrate_evidence(root: Path, repo_root: Path) -> int:
    from .transaction import mutate_registry, write_json

    migrated = 0
    source_map = source_map_for(root)

    def transform(value: Any) -> Any:
        nonlocal migrated
        if isinstance(value, dict):
            result = {}
            for key, child in value.items():
                if key == "evidence" and isinstance(child, list):
                    replacement = [
                        classified(migrate_legacy(item, repo_root), source_map)
                        for item in child
                    ]
                    migrated += sum(
                        before != after
                        for before, after in zip(child, replacement, strict=True)
                    )
                    result[key] = replacement
                else:
                    result[key] = transform(child)
            return result
        if isinstance(value, list):
            return [transform(child) for child in value]
        return value

    def mutate(staging: Path) -> None:
        for name in ASSET_KEYS:
            path = registry_dir(staging) / name
            write_json(path, transform(load_json(path)))

    audit_event: Callable[[], dict[str, Any]] | None = None
    if migrated:
        audit_event = lambda: {"operation": "migrate-evidence", "migrated": migrated}
    mutate_registry(root, repo_root, mutate, audit_event=audit_event)
    return migrated


ABSENCE_ASSETS = {
    "vocabulary": "without_vocabulary",
    "aggregates": "without_aggregate",
    "rules": "without_rule",
    "contracts": "without_contract",
}


def contexts_holding(records: dict[str, list[dict[str, Any]]]) -> dict[str, set[Any]]:
    contracts = set()
    for entry in records.get("contracts.json", []):
        contracts.add(entry.get("producer_context"))
        consumers = entry.get("consumer_contexts", [])
        if isinstance(consumers, list):
            contracts.update(consumers)
    return {
        "vocabulary": {
            context
            for entry in records.get("vocabulary.json", [])
            for context in entry.get("contexts", [])
        },
        "aggregates": {
            entry.get("context") for entry in records.get("aggregates.json", [])
        },
        "rules": {
            context
            for entry in records.get("rules.json", [])
            for context in entry.get("contexts", [])
        },
        "contracts": contracts,
    }


def declared_absences(contexts: list[dict[str, Any]]) -> dict[str, set[Any]]:
    absent: dict[str, set[Any]] = {asset: set() for asset in ABSENCE_ASSETS}
    for entry in contexts:
        declarations = entry.get("confirmed_absences")
        if not isinstance(declarations, list):
            continue
        for declaration in declarations:
            if isinstance(declaration, dict) and declaration.get("asset") in absent:
                absent[declaration["asset"]].add(entry.get("id"))
    return absent


def coverage(root: Path) -> dict[str, Any]:
    contexts = asset_records(root, "contexts.json")
    vocabulary = asset_records(root, "vocabulary.json")
    aggregates = asset_records(root, "aggregates.json")
    rules = asset_records(root, "rules.json")
    contracts = asset_records(root, "contracts.json")
    known = [entry["id"] for entry in contexts]
    holding = contexts_holding(
        {
            "vocabulary.json": vocabulary,
            "aggregates.json": aggregates,
            "rules.json": rules,
            "contracts.json": contracts,
        }
    )
    absent = declared_absences(contexts)
    missing = {
        gap: [context for context in known if context not in holding[asset]]
        for asset, gap in ABSENCE_ASSETS.items()
    }
    return {
        "contexts": len(known),
        "vocabulary_terms": len(vocabulary),
        "aggregates": len(aggregates),
        "rules": len(rules),
        "contracts": len(contracts),
        "gaps": {
            gap: [context for context in missing[gap] if context not in absent[asset]]
            for asset, gap in ABSENCE_ASSETS.items()
        },
        "confirmed_absent": {
            gap: [context for context in missing[gap] if context in absent[asset]]
            for asset, gap in ABSENCE_ASSETS.items()
        },
    }


def lookup(root: Path, asset: str, query: str) -> list[dict[str, Any]]:
    name = f"{asset}.json"
    if name not in ASSET_KEYS:
        raise ValueError(f"unknown asset: {asset}")
    needle = query.casefold()
    return [
        entry
        for entry in asset_records(root, name)
        if needle in json.dumps(entry, ensure_ascii=False).casefold()
    ][: query_result_limit(root)]


def record_by_id(root: Path, asset: str, identifier: str) -> dict[str, Any] | None:
    for entry in asset_records(root, f"{asset}.json"):
        if entry.get("id") == identifier:
            return entry
    return None


def resolve_terms(root: Path, query: str, context: str | None) -> list[dict[str, Any]]:
    needle = query.casefold()
    matches = []
    for term in asset_records(root, "vocabulary.json"):
        contexts = term.get("contexts", [])
        if context is not None and context not in contexts:
            continue
        searchable = " ".join(
            str(term.get(field, "")) for field in ("id", "name", "definition")
        ).casefold()
        named = [str(term.get(field, "")).casefold() for field in ("id", "name")]
        if needle in searchable or any(name and name in needle for name in named):
            matches.append(term)
    return matches[: query_result_limit(root)]


def context_model(root: Path, identifier: str) -> dict[str, Any] | None:
    context = record_by_id(root, "contexts", identifier)
    if context is None:
        return None
    contracts = asset_records(root, "contracts.json")
    mode = review_mode(root)
    return {
        "usage": "constraint" if mode == "scm-verified" else "working-memory",
        "review_mode": mode,
        "context": context,
        "vocabulary": [
            entry
            for entry in asset_records(root, "vocabulary.json")
            if identifier in entry.get("contexts", [])
        ],
        "aggregates": [
            entry
            for entry in asset_records(root, "aggregates.json")
            if entry.get("context") == identifier
        ],
        "rules": [
            entry
            for entry in asset_records(root, "rules.json")
            if identifier in entry.get("contexts", [])
        ],
        "contracts": [
            entry
            for entry in contracts
            if identifier == entry.get("producer_context")
            or identifier in entry.get("consumer_contexts", [])
        ],
        "interactions": [
            entry
            for entry in asset_records(root, "interactions.json")
            if identifier
            in {entry.get("producer_context"), entry.get("consumer_context")}
        ],
        "events": [
            entry
            for entry in asset_records(root, "events.json")
            if entry.get("owner_context") == identifier
        ],
        "capabilities": [
            entry
            for entry in asset_records(root, "capabilities.json")
            if entry.get("context") == identifier
        ],
        "value_objects": [
            entry
            for entry in asset_records(root, "value-objects.json")
            if entry.get("context") == identifier
        ],
    }


def boundary_analysis(root: Path, source: str, target: str) -> dict[str, Any]:
    interactions = [
        entry
        for entry in asset_records(root, "interactions.json")
        if entry.get("producer_context") == source
        and entry.get("consumer_context") == target
    ]
    contracts = [
        entry
        for entry in asset_records(root, "contracts.json")
        if entry.get("producer_context") == source
        and target in entry.get("consumer_contexts", [])
    ]
    dependencies = [
        entry
        for entry in asset_records(root, "dependency-policies.json")
        if entry.get("from_context") == source and entry.get("to_context") == target
    ]
    return {
        "source_context": source,
        "target_context": target,
        "interactions": interactions,
        "contracts": contracts,
        "dependencies": dependencies,
        "status": "known"
        if interactions or contracts
        else "no_registered_collaboration",
    }
