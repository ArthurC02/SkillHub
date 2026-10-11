import json
import shutil
from collections.abc import Callable
from pathlib import Path
from typing import Any

from .common import (
    ASSET_FORMATS,
    ASSET_KEYS,
    CONTEXT_SUBDOMAINS,
    REGISTRY_STATUSES,
    REVIEW_REQUIRED_FIELDS,
    load_json,
    one_level_too_deep_hint,
    registry_dir,
    template_dir,
)
from .evidence import all_references, classified, migrate_legacy, source_map_for, verify
from .policy import query_result_limit, review_mode

STANDARD_FILES = (*ASSET_KEYS, "manifest.json")
ABSENCE_ASSETS = {
    "vocabulary": "without_vocabulary",
    "aggregates": "without_aggregate",
    "rules": "without_rule",
    "contracts": "without_contract",
}


def init_registry(output: Path) -> None:
    if output.exists() and any(output.iterdir()):
        raise FileExistsError(f"output directory is not empty: {output}")
    target_registry = registry_dir(output)
    target_registry.mkdir(parents=True, exist_ok=True)
    templates = template_dir()
    for name in STANDARD_FILES:
        shutil.copyfile(templates / name, target_registry / name)


def migrate_registry(root: Path) -> list[str]:
    target = registry_dir(root)
    templates = template_dir()
    created = []
    for name in STANDARD_FILES:
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
        json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8", newline="\n"
    )
    return created


def asset_records(root: Path, name: str) -> list[dict[str, Any]]:
    value = load_json(registry_dir(root) / name).get(ASSET_KEYS[name])
    if not isinstance(value, list) or not all(
        isinstance(record, dict) for record in value
    ):
        raise ValueError(f"{name} must contain a {ASSET_KEYS[name]} list of objects")
    return value


def _load_registry_assets(
    root: Path,
) -> tuple[dict[str, list[dict[str, Any]]], list[str]]:
    records: dict[str, list[dict[str, Any]]] = {}
    errors: list[str] = []
    for name in ASSET_KEYS:
        path = registry_dir(root) / name
        if not path.is_file():
            errors.append(f"missing asset: {path}")
            continue
        try:
            records[name] = asset_records(root, name)
        except ValueError as error:
            errors.append(str(error))
    return records, errors


def _asset_header_errors(name: str, asset: dict[str, Any], entries: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    if asset.get("format") != ASSET_FORMATS[name]:
        errors.append(f"{name} has an invalid format")
    if asset.get("status") not in REGISTRY_STATUSES:
        errors.append(f"{name} has an invalid status")
    identifiers = [entry.get("id") for entry in entries]
    if not all(isinstance(identifier, str) and identifier for identifier in identifiers):
        errors.append(f"{name} entries require non-empty id")
    elif len(identifiers) != len(set(identifiers)):
        errors.append(f"{name} contains duplicate id")
    return errors


def _entry_status_errors(
    name: str, entry: dict[str, Any], status: Any, require_reviewed: bool  # noqa: FBT001
) -> list[str]:
    label = f"{name}:{entry.get('id')}"
    if status not in REGISTRY_STATUSES:
        return [f"{label} has an invalid status"]
    errors: list[str] = []
    if status == "reviewed":
        missing = [field for field in REVIEW_REQUIRED_FIELDS[name] if not entry.get(field)]
        if missing:
            errors.append(f"{label} is reviewed but missing {', '.join(missing)}")
        if not entry.get("evidence"):
            errors.append(f"{label} is reviewed but lacks evidence")
    if require_reviewed and status != "reviewed":
        errors.append(f"{label} is not reviewed")
    return errors


def _validate_asset_records(
    root: Path,
    records: dict[str, list[dict[str, Any]]],
    require_reviewed: bool,  # noqa: FBT001
) -> list[str]:
    errors: list[str] = []
    for name, entries in records.items():
        asset = load_json(registry_dir(root) / name)
        errors.extend(_asset_header_errors(name, asset, entries))
        status_field = "review_status" if name == "decisions.json" else "status"
        for entry in entries:
            status = entry.get(status_field, asset.get("status"))
            errors.extend(_entry_status_errors(name, entry, status, require_reviewed))
    errors.extend(_context_placement_problems(records.get("contexts.json", [])))
    return errors


def _context_placement_problems(entries: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    claimed: dict[str, str] = {}
    for entry in entries:
        identifier = entry.get("id")
        label = f"contexts.json:{identifier}"
        subdomain = entry.get("subdomain")
        if subdomain is not None and subdomain not in CONTEXT_SUBDOMAINS:
            errors.append(
                f"{label} has an unknown subdomain: "
                f"{subdomain!r} is not one of {', '.join(sorted(CONTEXT_SUBDOMAINS))}"
            )
        prefixes = entry.get("requirement_prefixes")
        if prefixes is not None and (
            not isinstance(prefixes, list)
            or not all(isinstance(p, str) and p.strip() for p in prefixes)
        ):
            errors.append(
                f"{label} requirement_prefixes must be a list of non-empty strings"
            )
        path = entry.get("implementation_path")
        if path is None:
            continue
        if not isinstance(path, str) or not path.strip():
            errors.append(f"{label} implementation_path must be a non-empty string")
            continue
        for other, owner in claimed.items():
            if _paths_overlap(path, other):
                errors.append(
                    f"{label} claims {path}, which overlaps "
                    f"{other} claimed by {owner}: code belongs to one Context"
                )
        claimed[path] = str(identifier)
    return errors


def _paths_overlap(one: str, other: str) -> bool:
    first = one.strip("/").split("/")
    second = other.strip("/").split("/")
    shared = min(len(first), len(second))
    return first[:shared] == second[:shared]


def _validate_confirmed_absences(
    records: dict[str, list[dict[str, Any]]],
) -> list[str]:
    errors: list[str] = []
    holding = contexts_holding(records)
    for entry in records.get("contexts.json", []):
        label = f"contexts.json:{entry.get('id')}"
        declarations = entry.get("confirmed_absences", [])
        if not isinstance(declarations, list):
            errors.append(f"{label} confirmed_absences must be a list")
            continue
        for declaration in declarations:
            if (
                not isinstance(declaration, dict)
                or declaration.get("asset") not in ABSENCE_ASSETS
            ):
                errors.append(
                    f"{label} confirms an absence of an unknown asset; "
                    f"use one of {', '.join(sorted(ABSENCE_ASSETS))}"
                )
                continue
            asset = declaration["asset"]
            reason = declaration.get("reason")
            if not isinstance(reason, str) or not reason.strip():
                errors.append(
                    f"{label} confirms an absence of {asset} "
                    "without a reason; a confirmed absence is a finding and needs one"
                )
            elif entry.get("id") in holding[asset]:
                errors.append(
                    f"{label} confirms an absence of {asset}, "
                    f"but {asset} records name it"
                )
    return errors


SINGLE_CONTEXT_FIELDS = (
    ("aggregates.json", ("context",)),
    ("interactions.json", ("producer_context", "consumer_context")),
    ("events.json", ("owner_context",)),
    ("capabilities.json", ("context",)),
    ("value-objects.json", ("context",)),
)


def _validate_context_references(
    records: dict[str, list[dict[str, Any]]],
) -> list[str]:
    known_contexts = {entry.get("id") for entry in records.get("contexts.json", [])}
    return [
        *_context_list_errors(records, known_contexts),
        *_single_context_errors(records, known_contexts),
        *_contract_party_errors(records, known_contexts),
        *_interaction_contract_errors(records),
    ]


def _context_list_errors(records: dict[str, list[dict[str, Any]]], known_contexts: set[Any]) -> list[str]:
    errors: list[str] = []
    for name in ("vocabulary.json", "rules.json"):
        for entry in records.get(name, []):
            contexts = entry.get("contexts")
            if not isinstance(contexts, list) or not contexts:
                errors.append(f"{name}:{entry.get('id')} requires contexts")
            elif set(contexts) - known_contexts:
                errors.append(f"{name}:{entry.get('id')} references unknown context")
    return errors


def _single_context_errors(records: dict[str, list[dict[str, Any]]], known_contexts: set[Any]) -> list[str]:
    errors = [
        f"{name}:{entry.get('id')} references unknown context"
        for name, fields in SINGLE_CONTEXT_FIELDS
        for entry in records.get(name, [])
        for field in fields
        if entry.get(field) not in known_contexts
    ]
    errors.extend(
        f"dependency-policies.json:{entry.get('id')} references unknown context"
        for entry in records.get("dependency-policies.json", [])
        if entry.get("from_context") not in known_contexts
        or entry.get("to_context") not in known_contexts
    )
    return errors


def _contract_party_errors(records: dict[str, list[dict[str, Any]]], known_contexts: set[Any]) -> list[str]:
    errors: list[str] = []
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
    return errors


def _interaction_contract_errors(records: dict[str, list[dict[str, Any]]]) -> list[str]:
    errors: list[str] = []
    contract_ids = {entry.get("id") for entry in records.get("contracts.json", [])}
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
    results = (verify(reference, repo_root) for reference in all_references(records))
    return [
        f"evidence {result['status']}: {result.get('reason', result.get('path'))}"
        for result in results
        if result["status"] in {"invalid", "missing"}
    ]


def validate(root: Path, repo_root: Path | None, require_reviewed: bool) -> list[str]:  # noqa: FBT001
    manifest_path = registry_dir(root) / "manifest.json"
    if not manifest_path.is_file():
        return [f"missing manifest: {manifest_path}{one_level_too_deep_hint(manifest_path)}"]
    errors: list[str] = []
    if require_reviewed and review_mode(root) != "scm-verified":
        errors.append("local-draft-only Domain Memory cannot satisfy --require-reviewed")
    manifest = load_json(manifest_path)
    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, list) or not set(ASSET_KEYS).issubset(artifacts):
        errors.append("manifest artifacts must list every standard registry asset file")
    records, load_errors = _load_registry_assets(root)
    errors.extend(load_errors)
    errors.extend(_validate_asset_records(root, records, require_reviewed))
    errors.extend(_validate_confirmed_absences(records))
    errors.extend(_validate_context_references(records))
    errors.extend(_validate_evidence(records, repo_root))
    return errors


def verify_evidence(root: Path, repo_root: Path) -> dict[str, Any]:
    source_map = source_map_for(root)
    results = [
        verify(classified(reference, source_map), repo_root)
        for reference in all_references(
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

    mutate_registry(
        root,
        repo_root,
        mutate,
        audit_event=lambda: {"operation": "migrate-evidence", "migrated": migrated},
    )
    return migrated


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


def is_reviewed(record: dict[str, Any]) -> bool:
    return "reviewed" in (record.get("status"), record.get("review_status"))


def evidence_gaps(root: Path) -> list[dict[str, Any]]:
    from .evidence import classify_all, unclassified_paths

    source_map = source_map_for(root)
    gaps = []
    for name in sorted(ASSET_KEYS):
        if not (registry_dir(root) / name).is_file():
            continue
        for record in asset_records(root, name):
            if not is_reviewed(record):
                continue
            entry = {"asset": name.removesuffix(".json"), "id": record.get("id")}
            if not record.get("evidence"):
                gaps.append(entry | {"reason": "no evidence"})
                continue
            outside = unclassified_paths(classify_all(dict(record), source_map))
            if outside:
                gaps.append(
                    entry
                    | {"reason": "outside the selected sources", "paths": outside}
                )
    return gaps


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
        "evidence_gaps": evidence_gaps(root),
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
    return next(
        (
            entry
            for entry in asset_records(root, f"{asset}.json")
            if entry.get("id") == identifier
        ),
        None,
    )


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


def _records_where(
    root: Path, name: str, keep: Callable[[dict[str, Any]], bool]
) -> list[dict[str, Any]]:
    return [entry for entry in asset_records(root, name) if keep(entry)]


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
        "vocabulary": _records_where(
            root,
            "vocabulary.json",
            lambda entry: identifier in entry.get("contexts", []),
        ),
        "aggregates": _records_where(
            root, "aggregates.json", lambda entry: entry.get("context") == identifier
        ),
        "rules": _records_where(
            root, "rules.json", lambda entry: identifier in entry.get("contexts", [])
        ),
        "contracts": [
            entry
            for entry in contracts
            if identifier == entry.get("producer_context")
            or identifier in entry.get("consumer_contexts", [])
        ],
        "interactions": _records_where(
            root,
            "interactions.json",
            lambda entry: identifier
            in {entry.get("producer_context"), entry.get("consumer_context")},
        ),
        "events": _records_where(
            root, "events.json", lambda entry: entry.get("owner_context") == identifier
        ),
        "capabilities": _records_where(
            root, "capabilities.json", lambda entry: entry.get("context") == identifier
        ),
        "value_objects": _records_where(
            root, "value-objects.json", lambda entry: entry.get("context") == identifier
        ),
    }


def boundary_analysis(root: Path, source: str, target: str) -> dict[str, Any]:
    interactions = _records_where(
        root,
        "interactions.json",
        lambda entry: entry.get("producer_context") == source
        and entry.get("consumer_context") == target,
    )
    contracts = _records_where(
        root,
        "contracts.json",
        lambda entry: entry.get("producer_context") == source
        and target in entry.get("consumer_contexts", []),
    )
    dependencies = _records_where(
        root,
        "dependency-policies.json",
        lambda entry: entry.get("from_context") == source
        and entry.get("to_context") == target,
    )
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
