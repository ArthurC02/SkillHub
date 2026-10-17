import json
import re
import shutil
from collections import Counter
from collections.abc import Callable
from functools import cache
from itertools import pairwise
from pathlib import Path, PurePosixPath
from typing import Any

from .common import (
    ASSET_FORMATS,
    ASSET_KEYS,
    CONTEXT_SUBDOMAINS,
    REGISTRY_STATUSES,
    REVIEW_REQUIRED_FIELDS,
    record_status,
    registry_dir,
    template_dir,
)
from .evidence import (
    all_references,
    classifier,
    migrate_legacy,
    source_map_for,
    verifier,
    with_references,
    without_derived_keys,
)
from .okf import CONTEXT_RELATIONS, PROSE_FIELDS, RELATION_TARGETS, verified_by
from .policy import query_result_limit, review_mode
from .sources import lies_under, repository_files
from .store import (
    MANIFEST,
    create_concept_layout,
    document_path,
    has_document,
    has_registry,
    is_concept_layout,
    read_record,
    missing_registry,
    logical_documents,
    read_document,
    write_document,
)

STANDARD_FILES = (*ASSET_KEYS, "manifest.json")
ABSENCE_ASSETS = {
    "vocabulary": "without_vocabulary",
    "aggregates": "without_aggregate",
    "rules": "without_rule",
    "contracts": "without_contract",
    "events": "without_event",
}


def init_registry(output: Path) -> None:
    if output.exists() and any(output.iterdir()):
        raise FileExistsError(f"output directory is not empty: {output}")
    create_concept_layout(output)


def migrate_registry(root: Path) -> list[str]:
    if is_concept_layout(root):
        current = logical_documents(root)
        if refined_documents(current)[0] == current:
            return []
    from .transaction import mutate_registry

    created: list[str] = []
    reviewed_by: dict[str, str] = {}

    def mutate(staging: Path) -> None:
        target = registry_dir(staging)
        converting = not is_concept_layout(staging)
        if converting:
            created.extend(_copy_missing_templates(target))
        refined, found = refined_documents(logical_documents(staging))
        reviewed_by.update(found)
        if converting:
            for name in STANDARD_FILES:
                (target / name).unlink()
            create_concept_layout(staging)
        for name in ASSET_KEYS:
            write_document(staging, name, refined[name])
        if logical_documents(staging) != refined:
            raise ValueError("the concept layout would not hold the Registry unchanged; nothing was converted")

    mutate_registry(
        root, None, mutate,
        audit_event=lambda: {"operation": "migrate-registry", "layout": "okf", "reviewed_by": reviewed_by},
    )
    return created


def refined_documents(documents: dict[str, Any]) -> tuple[dict[str, Any], dict[str, str]]:
    reviewed_by: dict[str, str] = {}
    refined = {}
    for name, document in documents.items():
        key = ASSET_KEYS[name]
        records = document.get(key)
        if not isinstance(records, list):
            refined[name] = document
            continue
        refined[name] = {**document, key: [_refined_record(name, record, reviewed_by) for record in records]}
    return refined, reviewed_by


def _refined_record(name: str, record: dict[str, Any], reviewed_by: dict[str, str]) -> dict[str, Any]:
    refined = without_derived_keys(record)
    review = refined.pop("review", None)
    if not isinstance(review, dict):
        return refined
    if review.get("proposal_id"):
        reviewed_by[f"{name.removesuffix('.json')}/{refined.get('id')}"] = review["proposal_id"]
    verified = verified_by(review.get("approvals") or [])
    if verified:
        refined["verified"] = verified
    return refined


def _copy_missing_templates(target: Path) -> list[str]:
    copied = []
    for name in STANDARD_FILES:
        if not (target / name).exists():
            shutil.copyfile(template_dir() / name, target / name)
            copied.append(name)
    return copied


def asset_records(root: Path, name: str) -> list[dict[str, Any]]:
    return _records_of(name, read_document(root, name))


def _records_of(name: str, document: dict[str, Any]) -> list[dict[str, Any]]:
    value = document.get(ASSET_KEYS[name])
    if not isinstance(value, list) or not all(
        isinstance(record, dict) for record in value
    ):
        raise ValueError(f"{name} must contain a {ASSET_KEYS[name]} list of objects")
    return value


def _load_registry_assets(
    root: Path,
) -> tuple[dict[str, list[dict[str, Any]]], dict[str, dict[str, Any]], list[str]]:
    records: dict[str, list[dict[str, Any]]] = {}
    documents: dict[str, dict[str, Any]] = {}
    errors: list[str] = []
    for name in ASSET_KEYS:
        if not has_document(root, name):
            errors.append(f"missing asset: {document_path(root, name)}")
            continue
        try:
            document = read_document(root, name)
            records[name] = _records_of(name, document)
            documents[name] = document
        except ValueError as error:
            errors.append(str(error))
    return records, documents, errors


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
    label = f"{name.removesuffix('.json')}/{entry.get('id')}"
    if status not in REGISTRY_STATUSES:
        return [f"{label} has an invalid status"]
    errors: list[str] = []
    if "review" in entry:
        errors.append(f"{label} holds a legacy review block; run migrate-registry")
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
    documents: dict[str, dict[str, Any]],
    records: dict[str, list[dict[str, Any]]],
    require_reviewed: bool,  # noqa: FBT001
) -> list[str]:
    errors: list[str] = []
    for name, entries in records.items():
        asset = documents[name]
        errors.extend(_asset_header_errors(name, asset, entries))
        for entry in entries:
            status = record_status(name.removesuffix(".json"), asset, entry)
            errors.extend(_entry_status_errors(name, entry, status, require_reviewed))
    return errors


def cross_reference_errors(records: dict[str, list[dict[str, Any]]]) -> list[str]:
    return [
        *_context_placement_problems(records.get("contexts.json", [])),
        *_validate_confirmed_absences(records),
        *_validate_context_references(records),
        *_validate_term_labels(records.get("vocabulary.json", [])),
        *_validate_idempotency(records.get("contracts.json", [])),
    ]


def _context_placement_problems(entries: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    claimed: dict[str, str] = {}
    for entry in entries:
        identifier = entry.get("id")
        label = f"contexts/{identifier}"
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
        label = f"contexts/{entry.get('id')}"
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
        *_event_consumer_errors(records, known_contexts),
        *_contract_errors(records),
        *_aggregate_binding_errors(records),
    ]


def _context_list_errors(records: dict[str, list[dict[str, Any]]], known_contexts: set[Any]) -> list[str]:
    errors: list[str] = []
    for name in ("vocabulary.json", "rules.json"):
        for entry in records.get(name, []):
            contexts = entry.get("contexts")
            if not isinstance(contexts, list) or not contexts:
                errors.append(f"{name.removesuffix('.json')}/{entry.get('id')} requires contexts")
            elif set(contexts) - known_contexts:
                errors.append(f"{name.removesuffix('.json')}/{entry.get('id')} references {_unknown(set(contexts) - known_contexts)}")
    return errors


def _unknown(contexts: Any) -> str:
    return f"unknown context {', '.join(sorted(map(str, contexts)))}; add the Context first"


def _single_context_errors(records: dict[str, list[dict[str, Any]]], known_contexts: set[Any]) -> list[str]:
    errors = [
        f"{name.removesuffix('.json')}/{entry.get('id')} references {_unknown([entry.get(field)])}"
        for name, fields in SINGLE_CONTEXT_FIELDS
        for entry in records.get(name, [])
        for field in fields
        if entry.get(field) not in known_contexts
    ]
    errors.extend(
        f"dependency-policies/{entry.get('id')} references {_unknown(missing)}"
        for entry in records.get("dependency-policies.json", [])
        if (missing := {entry.get(end) for end in ("from_context", "to_context")} - known_contexts)
    )
    return errors


def _contract_party_errors(records: dict[str, list[dict[str, Any]]], known_contexts: set[Any]) -> list[str]:
    errors: list[str] = []
    for entry in records.get("contracts.json", []):
        if entry.get("producer_context") not in known_contexts:
            errors.append(
                f"contracts/{entry.get('id')} references as producer {_unknown([entry.get('producer_context')])}"
            )
        consumers = entry.get("consumer_contexts", [])
        if not isinstance(consumers, list):
            errors.append(f"contracts/{entry.get('id')} consumer_contexts must be a list of Context ids")
        elif set(consumers) - known_contexts:
            errors.append(
                f"contracts/{entry.get('id')} references as consumer {_unknown(set(consumers) - known_contexts)}"
            )
    return errors


def _event_consumer_errors(records: dict[str, list[dict[str, Any]]], known_contexts: set[Any]) -> list[str]:
    errors: list[str] = []
    for entry in records.get("events.json", []):
        consumers = entry.get("consumers")
        if consumers is None:
            continue
        if not isinstance(consumers, list):
            errors.append(f"events/{entry.get('id')} consumers must be a list of Context ids")
        elif set(consumers) - known_contexts:
            errors.append(f"events/{entry.get('id')} references as consumer {_unknown(set(consumers) - known_contexts)}")
    return errors


def _contract_errors(records: dict[str, list[dict[str, Any]]]) -> list[str]:
    errors: list[str] = []
    contract_ids = {entry.get("id") for entry in records.get("contracts.json", [])}
    for name in ("interactions.json", "events.json"):
        for entry in records.get(name, []):
            contract_id = entry.get("contract_id")
            if contract_id is not None and contract_id not in contract_ids:
                errors.append(f"{name.removesuffix('.json')}/{entry.get('id')} references unknown contract")
    return errors


def _aggregate_binding_errors(records: dict[str, list[dict[str, Any]]]) -> list[str]:
    errors: list[str] = []
    aggregate_contexts = {entry.get("id"): entry.get("context") for entry in records.get("aggregates.json", [])}
    for name in ("rules.json", "events.json"):
        for entry in records.get(name, []):
            aggregate = entry.get("aggregate")
            if aggregate is None:
                continue
            label = f"{name.removesuffix('.json')}/{entry.get('id')}"
            if aggregate not in aggregate_contexts:
                errors.append(f"{label} references unknown aggregate {aggregate}; add the Aggregate first")
            elif aggregate_contexts[aggregate] not in _named_contexts(entry):
                errors.append(
                    f"{label} aggregate {aggregate} belongs to context {aggregate_contexts[aggregate]}, "
                    "which the record does not name"
                )
    return errors


def _named_contexts(entry: dict[str, Any]) -> list[Any]:
    contexts = entry.get("contexts")
    return contexts if isinstance(contexts, list) else [entry.get("owner_context")]


def _validate_evidence(
    records: dict[str, list[dict[str, Any]]], repo_root: Path | None
) -> list[str]:
    if repo_root is None:
        return []
    check = verifier(repo_root)
    results = (check(reference) for reference in all_references(records))
    return [
        f"evidence {result['status']}: {result.get('reason', result.get('path'))}"
        for result in results
        if result["status"] in {"invalid", "missing", "unhashed"}
    ]


def validate(root: Path, repo_root: Path | None, require_reviewed: bool) -> list[str]:  # noqa: FBT001
    if not has_registry(root):
        return [missing_registry(root)]
    errors: list[str] = []
    if require_reviewed and review_mode(root) != "scm-verified":
        errors.append("local-draft-only Domain Memory cannot satisfy --require-reviewed")
    manifest = read_document(root, MANIFEST)
    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, list) or not set(ASSET_KEYS).issubset(artifacts):
        errors.append("manifest artifacts must list every standard registry asset file")
    records, documents, load_errors = _load_registry_assets(root)
    errors.extend(load_errors)
    errors.extend(_validate_asset_records(documents, records, require_reviewed))
    errors.extend(cross_reference_errors(records))
    errors.extend(_validate_evidence(records, repo_root))
    if repo_root is not None:
        errors.extend(_implementation_path_errors(records.get("contexts.json", []), repo_root))
    return errors


def _implementation_path_errors(contexts: list[dict[str, Any]], repo_root: Path) -> list[str]:
    return [
        f"contexts/{identifier} implementation_path {path} lies over no file in the repository; "
        "after a move, relocate-evidence --from <old repository path> --to <new> proposes the new path, or correct it"
        for identifier, path, reviewed in unplaced_implementation_paths(contexts, repo_root)
        if reviewed
    ]


def unplaced_implementation_paths(contexts: list[dict[str, Any]], repo_root: Path) -> list[tuple[Any, str, bool]]:
    placed = [(entry.get("id"), entry["implementation_path"], is_reviewed(entry)) for entry in contexts
              if isinstance(entry.get("implementation_path"), str) and entry["implementation_path"].strip()]
    if not placed:
        return []
    files = repository_files(repo_root)
    return [(identifier, path, reviewed) for identifier, path, reviewed in placed
            if not any(lies_under(file, path) for file in files)]


IDEMPOTENCY_FIELDS = ("key", "scope", "on_replay", "on_conflict")
OPTIONAL_IDEMPOTENCY_FIELDS = ("expiry",)


def _validate_idempotency(contracts: list[dict[str, Any]]) -> list[str]:
    errors = []
    for contract in contracts:
        idempotency = contract.get("idempotency")
        if idempotency is None:
            continue
        where = f"contracts/{contract.get('id')}"
        if not isinstance(idempotency, dict):
            errors.append(f"{where} idempotency must be an object")
            continue
        missing = [field for field in IDEMPOTENCY_FIELDS if field not in idempotency]
        unknown = sorted(set(idempotency) - {*IDEMPOTENCY_FIELDS, *OPTIONAL_IDEMPOTENCY_FIELDS})
        blank = sorted(
            field for field, value in idempotency.items()
            if field not in unknown and not (isinstance(value, str) and value.strip())
        )
        if missing:
            errors.append(f"{where} idempotency lacks {', '.join(missing)}")
        if unknown:
            errors.append(f"{where} idempotency has unknown fields {', '.join(unknown)}")
        if blank:
            errors.append(f"{where} idempotency fields must be non-empty text: {', '.join(blank)}")
    return errors


def _labels(entry: dict[str, Any], field: str) -> list[Any] | None:
    labels = entry.get(field, [])
    if not isinstance(labels, list) or not all(
        isinstance(label, str) and label.strip() for label in labels
    ):
        return None
    return labels


def _validate_term_labels(terms: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    term_ids = {term.get("id") for term in terms}
    for term in terms:
        where = f"vocabulary/{term.get('id')}"
        synonyms = _labels(term, "synonyms")
        look_alikes = _labels(term, "not_same_as")
        if synonyms is None or look_alikes is None:
            errors.append(f"{where} synonyms and not_same_as must be lists of non-empty strings")
            continue
        errors.extend(_synonym_errors(where, term, synonyms, terms))
        if set(look_alikes) - term_ids or term.get("id") in look_alikes:
            errors.append(f"{where} not_same_as must name other vocabulary term ids")
    return errors


def _synonym_errors(
    where: str, term: dict[str, Any], synonyms: list[str], terms: list[dict[str, Any]]
) -> list[str]:
    folded = [synonym.casefold() for synonym in synonyms]
    errors = []
    if len(set(folded)) != len(folded):
        errors.append(f"{where} repeats a synonym")
    if str(term.get("name", "")).casefold() in folded:
        errors.append(f"{where} lists its own name as a synonym")
    contexts = set(term.get("contexts") or [])
    errors.extend(
        f"{where} synonym {other.get('name')!r} is the name of {other.get('id')} in a shared context"
        for other in terms
        if other is not term
        and str(other.get("name", "")).casefold() in folded
        and contexts & set(other.get("contexts") or [])
    )
    return errors


EVIDENCE_STATUSES = ("current", "shifted", "stale", "unhashed", "missing", "invalid", "legacy-unverified")


def verify_evidence(root: Path, repo_root: Path, paths: list[str] | None = None) -> dict[str, Any]:
    source_map = source_map_for(root)
    paths = [PurePosixPath(Path(path).as_posix()).as_posix() for path in paths] if paths else None
    wanted = set(paths or ())
    classify, check = classifier(source_map), verifier(repo_root)
    results = [
        {"asset": name.removesuffix(".json"), "id": entry.get("id"), **check(classify(reference))}
        for name in ASSET_KEYS
        for entry in asset_records(root, name)
        for reference in all_references(entry)
        if not paths or (isinstance(reference, dict) and reference.get("path") in wanted)
    ]
    statuses = Counter(item["status"] for item in results)
    summary: dict[str, Any] = {status: statuses[status] for status in EVIDENCE_STATUSES}
    kinds = Counter(item["source_kind"] for item in results if "source_kind" in item)
    summary["by_source_kind"] = {kind: kinds[kind] for kind in sorted(kinds)}
    report: dict[str, Any] = {"results": results, "summary": summary}
    if paths:
        citations = Counter(item.get("path") for item in results)
        report["paths"] = {
            path: {"citations": citations[path], "exists": (repo_root / path).is_file()}
            for path in paths
        }
    return report


def migrate_evidence(root: Path, repo_root: Path) -> int:
    from .transaction import mutate_registry

    migrated = 0

    def migrated_reference(reference: Any) -> Any:
        nonlocal migrated
        after = migrate_legacy(reference, repo_root)
        migrated += reference != after
        return after

    def transform(value: Any) -> Any:
        return with_references(value, migrated_reference)

    def mutate(staging: Path) -> None:
        for name in ASSET_KEYS:
            write_document(staging, name, transform(read_document(staging, name)))

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
        "events": {entry.get("owner_context") for entry in records.get("events.json", [])},
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


def evidence_gaps(root: Path, records: dict[str, list[dict[str, Any]]] | None = None) -> list[dict[str, Any]]:
    from .evidence import classify_all, unclassified_paths

    source_map = source_map_for(root)
    gaps = []
    for name in sorted(ASSET_KEYS):
        if records is None and not has_document(root, name):
            continue
        for record in records[name] if records is not None else asset_records(root, name):
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
    records = {name: asset_records(root, name) if has_document(root, name) else [] for name in ASSET_KEYS}
    contexts, vocabulary, aggregates, rules, contracts, events = (
        records[name] for name in ("contexts.json", "vocabulary.json", "aggregates.json", "rules.json",
                                   "contracts.json", "events.json")
    )
    known = [entry["id"] for entry in contexts]
    holding = contexts_holding(records)
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
        "events": len(events),
        "gaps": {
            gap: [context for context in missing[gap] if context not in absent[asset]]
            for asset, gap in ABSENCE_ASSETS.items()
        },
        "confirmed_absent": {
            gap: [context for context in missing[gap] if context in absent[asset]]
            for asset, gap in ABSENCE_ASSETS.items()
        },
        "evidence_gaps": evidence_gaps(root, records),
    }


def lookup(root: Path, asset: str, query: str) -> list[dict[str, Any]]:
    return ranked_lookup(root, asset, query)[0]


def ranked_lookup(root: Path, asset: str, query: str) -> tuple[list[dict[str, Any]], str]:
    name = f"{asset}.json"
    if name not in ASSET_KEYS:
        raise ValueError(f"unknown asset: {asset}")
    entries, limit = asset_records(root, name), query_result_limit(root)
    needle = query.casefold()
    phrase = [entry for entry in entries if needle in json.dumps(entry, ensure_ascii=False).casefold()]
    if phrase:
        return phrase[:limit], "phrase"
    wanted = query_words(query)
    held = [held_stems(" ".join(_strings(entry))) for entry in entries]
    every = [
        entry for entry, words in zip(entries, held, strict=True) if wanted and all(holds(word, words) for word in wanted)
    ]
    if every:
        return every[:limit], "all-words"
    common = {word for word in wanted if _is_common(sum(holds(word, words) for words in held), len(entries))}
    shared = [
        (sum(holds(word, words) for word in wanted if word not in common), entry)
        for entry, words in zip(entries, held, strict=True)
    ]
    some = [entry for count, entry in sorted(shared, key=lambda pair: -pair[0]) if count]
    if some:
        return some[:limit], "some-words"
    return [], "common-words" if common else "none"


COMMON_SHARE = 4
MIN_COMMON_RECORDS = 3


def _is_common(holders: int, records: int) -> bool:
    return holders >= MIN_COMMON_RECORDS and holders * COMMON_SHARE > records


SUFFIXES = (
    "ations", "ation", "ings", "ing", "ency", "ence", "ents", "ent", "ions", "ion", "ures", "ured", "ure", "ens", "en", "ed", "es", "s",
    "e", "y",
)
MIN_STEM_LENGTH = 3
STOP_WORDS = frozenset({
    "a", "after", "an", "and", "are", "at", "be", "before", "by", "can", "do", "does", "for", "how", "in", "is", "it",
    "of", "on", "or", "that", "the", "this", "to", "what", "when", "who", "with",
})


CAMEL_BOUNDARY = re.compile(r"(?<=[a-z0-9])(?=[A-Z])")


def _words(text: str) -> list[str]:
    return [word for word in re.findall(r"[a-z0-9]+", CAMEL_BOUNDARY.sub(" ", text).casefold()) if word not in STOP_WORDS]


def word_stems(text: str) -> set[str]:
    return {_stem(word) for word in _words(text)}


def held_stems(text: str) -> set[str]:
    stems = [_stem(word) for word in _words(text)]
    return {*stems, *(first + second for first, second in pairwise(stems))}


def query_words(query: str) -> list[str]:
    return list({_stem(word): word for word in _words(query)}.values())


def holds(word: str, held: set[str]) -> bool:
    return not _candidate_stems(word).isdisjoint(held)


@cache
def _candidate_stems(word: str) -> frozenset[str]:
    return frozenset({
        _stem(word), *(_stem(word[:split]) + _stem(word[split:]) for split in range(MIN_STEM_LENGTH, len(word) - 1))
    })


@cache
def _stem(word: str) -> str:
    return next(
        (word[: -len(suffix)] for suffix in SUFFIXES
         if word.endswith(suffix) and len(word) - len(suffix) >= MIN_STEM_LENGTH),
        word,
    )


def _strings(value: Any) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, dict):
        return [text for child in value.values() for text in _strings(child)]
    if isinstance(value, list):
        return [text for child in value for text in _strings(child)]
    return []


def record_by_id(root: Path, asset: str, identifier: str) -> dict[str, Any] | None:
    return read_record(root, f"{asset}.json", identifier)


MATCH_KINDS = ("name", "synonym", "definition")


def _match_kind(term: dict[str, Any], needle: str) -> str | None:
    labels = {
        "name": [str(term.get(field, "")).casefold() for field in ("id", "name")],
        "synonym": [str(label).casefold() for label in term.get("synonyms") or []],
    }
    for kind, values in labels.items():
        if any(value and (value in needle or needle in value) for value in values):
            return kind
    if needle in str(term.get("definition", "")).casefold():
        return "definition"
    return None


def resolve_terms(
    root: Path, query: str, context: str | None, *, with_mentions: bool = False
) -> list[dict[str, Any]]:
    return every_resolved_term(root, query, context, with_mentions=with_mentions)[: query_result_limit(root)]


def every_resolved_term(
    root: Path, query: str, context: str | None, *, with_mentions: bool = False
) -> list[dict[str, Any]]:
    needle = query.casefold()
    matches = []
    for term in asset_records(root, "vocabulary.json"):
        if context is not None and context not in term.get("contexts", []):
            continue
        kind = _match_kind(term, needle)
        if kind is not None:
            matches.append({**term, "match_kind": kind})
    matches.sort(key=lambda match: MATCH_KINDS.index(match["match_kind"]))
    if matches and not with_mentions:
        return matches
    matched = {match["id"] for match in matches}
    mentions = [
        mention for mention in prose_mentions(root, needle)
        if mention["asset"] != "vocabulary" or mention["id"] not in matched
    ]
    return matches + mentions


def _reviewed_entries(root: Path, name: str) -> list[dict[str, Any]]:
    document = read_document(root, name)
    asset = name.removesuffix(".json")
    return [
        entry for entry in document.get(ASSET_KEYS[name], [])
        if record_status(asset, document, entry) == "reviewed"
    ]


DEFINING_FIELDS = ("id", "name", "responsibility", "statement", "definition", "description")
UNSEARCHED_FIELDS = ("status", "review_status")
CONSTRAINT_ASSETS = ("rules", "contracts")


SUMMARY_FIELDS = ("statement", "definition", "responsibility", "description", "name")
SENTENCE_END = re.compile(r"(?<=[.!?])\s")


def _mention(name: str, entry: dict[str, Any], fields: list[str]) -> dict[str, Any]:
    asset = name.removesuffix(".json")
    summary = next(
        (entry[field] for field in (PROSE_FIELDS.get(asset), *SUMMARY_FIELDS) if isinstance(entry.get(field), str)), ""
    )
    return {
        "match_kind": "mentioned", "asset": asset, "id": entry.get("id"), "status": "reviewed",
        "contexts": _contexts_named(asset, entry), "summary": SENTENCE_END.split(summary.strip(), maxsplit=1)[0],
        "fields": fields,
    }


def _contexts_named(asset: str, entry: dict[str, Any]) -> list[Any]:
    if asset == "contexts":
        return [entry.get("id")]
    named = [
        context
        for field, value in entry.items()
        if field in CONTEXT_RELATIONS
        for context in (value if isinstance(value, list) else [value])
    ]
    return list(dict.fromkeys(named))


def prose_mentions(root: Path, needle: str) -> list[dict[str, Any]]:
    wanted = query_words(needle)
    if not wanted:
        return []
    mentions = [
        _mention(name, entry, fields)
        for name in sorted(ASSET_KEYS)
        if has_document(root, name)
        for entry in _reviewed_entries(root, name)
        if (fields := _mentioning_fields(entry, wanted))
    ]
    return sorted(
        mentions,
        key=lambda mention: (
            not any(field in DEFINING_FIELDS for field in mention["fields"]),
            mention["asset"] not in CONSTRAINT_ASSETS,
        ),
    )


def _mentioning_fields(entry: dict[str, Any], wanted: list[str]) -> list[str]:
    return [
        field for field, value in entry.items()
        if field not in UNSEARCHED_FIELDS
        and any(all(holds(word, stems) for word in wanted) for stems in map(held_stems, _searchable(field, value)))
    ]


def _searchable(field: str, value: Any) -> list[str]:
    if field == "id":
        return [value.replace("-", " ")] if isinstance(value, str) else []
    if isinstance(value, str):
        return [value]
    if isinstance(value, list) and field not in RELATION_TARGETS:
        return [item for item in value if isinstance(item, str)]
    return []


def _records_where(
    root: Path, name: str, keep: Callable[[dict[str, Any]], bool]
) -> list[dict[str, Any]]:
    return [entry for entry in asset_records(root, name) if keep(entry)]


def context_model(root: Path, identifier: str) -> dict[str, Any] | None:
    context = record_by_id(root, "contexts", identifier)
    if context is None:
        return None
    contracts = asset_records(root, "contracts.json")
    rules = asset_records(root, "rules.json")
    interactions = asset_records(root, "interactions.json")
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
        "rules": [entry for entry in rules if identifier in entry.get("contexts", [])],
        "contracts": [
            entry
            for entry in contracts
            if identifier == entry.get("producer_context")
            or identifier in entry.get("consumer_contexts", [])
        ],
        "interactions": [
            entry for entry in interactions if identifier in {entry.get("producer_context"), entry.get("consumer_context")}
        ],
        "events": _records_where(
            root, "events.json",
            lambda entry: entry.get("owner_context") == identifier or identifier in (entry.get("consumers") or []),
        ),
        "capabilities": _records_where(
            root, "capabilities.json", lambda entry: entry.get("context") == identifier
        ),
        "value_objects": _records_where(
            root, "value-objects.json", lambda entry: entry.get("context") == identifier
        ),
        "rules_bound_through_collaborations": _bound_rules(identifier, contracts, interactions, rules),
    }


def _bound_rules(
    identifier: str, contracts: list[dict[str, Any]], interactions: list[dict[str, Any]], rules: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    collaborations = [
        (f"contracts/{entry.get('id')}", entry) for entry in contracts
        if identifier == entry.get("producer_context") or identifier in entry.get("consumer_contexts", [])
    ] + [
        (f"interactions/{entry.get('id')}", entry) for entry in interactions
        if identifier in {entry.get("producer_context"), entry.get("consumer_context")}
    ]
    bound = []
    for rule in rules:
        if identifier in rule.get("contexts", []):
            continue
        cited = _cited_paths(rule)
        via = [name for name, entry in collaborations if cited & _cited_paths(entry)]
        if via:
            bound.append({"rule": rule.get("id"), "contexts": rule.get("contexts", []), "statement": rule.get("statement"),
                          "via": via})
    return bound


def _cited_paths(record: dict[str, Any]) -> set[str]:
    return {
        reference["path"] for reference in all_references(record)
        if isinstance(reference, dict) and isinstance(reference.get("path"), str)
    }


def context_summary(model: dict[str, Any]) -> dict[str, Any]:
    return {
        key: [summarized(entry) for entry in value] if isinstance(value, list) and key != "rules_bound_through_collaborations"
        else {**summarized(value), "implementation_path": value.get("implementation_path")} if key == "context" else value
        for key, value in model.items()
    }


def summarized(entry: dict[str, Any]) -> dict[str, Any]:
    summary = next((entry[field] for field in SUMMARY_FIELDS if isinstance(entry.get(field), str)), None)
    return {"id": entry.get("id"), "status": entry.get("status", entry.get("review_status")), "summary": summary}


def boundary_analysis(root: Path, source: str, target: str) -> dict[str, Any]:
    records = {name: asset_records(root, name) for name in COLLABORATION_ASSETS}
    forward = _dependence(records, dependent=source, provider=target)
    backward = _dependence(records, dependent=target, provider=source)
    both = [*forward.values(), *backward.values()]
    return {
        "source_context": source,
        "target_context": target,
        "source_depends_on_target": forward,
        "target_depends_on_source": backward,
        "status": "known"
        if any(direction[key] for direction in (forward, backward) for key in ("interactions", "contracts", "events"))
        else "policy_only"
        if any(both)
        else "no_registered_collaboration",
    }


COLLABORATION_ASSETS = ("interactions.json", "contracts.json", "dependency-policies.json", "events.json")


def _dependence(records: dict[str, list[dict[str, Any]]], *, dependent: str, provider: str) -> dict[str, list[Any]]:
    return {
        "interactions": [
            entry for entry in records["interactions.json"]
            if entry.get("producer_context") == provider and entry.get("consumer_context") == dependent
        ],
        "contracts": [
            entry for entry in records["contracts.json"]
            if entry.get("producer_context") == provider and dependent in (entry.get("consumer_contexts") or [])
        ],
        "dependencies": [
            entry for entry in records["dependency-policies.json"]
            if entry.get("from_context") == dependent and entry.get("to_context") == provider
        ],
        "events": [
            entry for entry in records["events.json"]
            if entry.get("owner_context") == provider and dependent in (entry.get("consumers") or [])
        ],
    }
