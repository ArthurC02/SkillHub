from collections import Counter
from pathlib import Path, PurePosixPath
from typing import Any

from .common import (
    ASSET_KEYS,
    CLOSED_FIELDS,
    CONTEXT_SUBDOMAINS,
    PROPOSAL_FILE,
    REGISTRY_STATUSES,
    REVIEW_REQUIRED_FIELDS,
    conflict_marked_line,
    invalid_choice,
    is_reviewed,
    load_json,
    record_status,
    registry_dir,
    review_field,
    shown_path,
)
from .evidence import (
    ContentReader,
    all_references,
    classifier,
    source_map_for,
    verifier,
    without_derived_keys,
)
from .policy import existing_policy, review_mode, validate_policy
from .sources import Listing, directory_of, listing_of, owned_directory
from .store import (
    create_concept_layout,
    has_registry,
    read_record,
    missing_registry,
    read_document,
    registry_files,
    stale_indexes,
    text_of,
)

ABSENCE_ASSETS = {
    "vocabulary": "without_vocabulary",
    "aggregates": "without_aggregate",
    "rules": "without_rule",
    "contracts": "without_contract",
    "events": "without_event",
}
KNOWN_CONSISTENCIES = ("synchronous", "eventual")


def init_registry(output: Path) -> None:
    if output.exists() and any(output.iterdir()):
        raise ValueError(f"output directory is not empty: {output}")
    create_concept_layout(output)


def asset_records(root: Path, name: str) -> list[dict[str, Any]]:
    return _records_of(name, read_document(root, name))


def registry_consistency_notes(root: Path) -> list[str]:
    return consistency_notes(
        [(f"interactions/{record.get('id')}", record) for record in asset_records(root, "interactions.json")]
    )


def package_consistency_notes(package_root: Path) -> list[str]:
    proposal = load_json(package_root / PROPOSAL_FILE)
    interactions = [
        update.get("record") for update in proposal.get("registry_updates", [])
        if isinstance(update, dict) and update.get("asset") == "interactions"
    ]
    return consistency_notes([
        (f"{package_root.name} boundary_decision", proposal.get("boundary_decision")),
        *((f"{package_root.name} interactions/{record.get('id')}", record)
          for record in interactions if isinstance(record, dict)),
    ])


def consistency_notes(holders: list[tuple[str, Any]]) -> list[str]:
    return [
        f'NOTE: {where} consistency "{holder["consistency"]}" is not one of the known values '
        f'{", ".join(KNOWN_CONSISTENCIES)}; it is accepted as written, but a reader looking for those values misses it.'
        for where, holder in holders
        if isinstance(holder, dict) and isinstance(holder.get("consistency"), str)
        and holder["consistency"] not in KNOWN_CONSISTENCIES
    ]


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
        loaded = _loaded_asset(root, name)
        if isinstance(loaded, str):
            errors.append(loaded)
        else:
            records[name], documents[name] = loaded
    return records, documents, errors


def _loaded_asset(root: Path, name: str) -> tuple[list[dict[str, Any]], dict[str, Any]] | str:
    try:
        document = read_document(root, name)
        return _records_of(name, document), document
    except ValueError as error:
        return str(error)


def _entry_status_errors(
    name: str, entry: dict[str, Any], status: Any, require_reviewed: bool  # noqa: FBT001
) -> list[str]:
    label = f"{name.removesuffix('.json')}/{entry.get('id')}"
    if status not in REGISTRY_STATUSES:
        return [invalid_choice(label, "status", status, REGISTRY_STATUSES)]
    errors: list[str] = []
    if "review" in entry:
        errors.append(f"{label} holds a review block; this version reads verified only")
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
                + ("; shared-kernel is a relationship between two Contexts, recorded on their Interaction"
                   if subdomain == "shared-kernel" else "")
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
        *_closed_field_errors(records),
    ]


CLOSED_FIELD_HINTS = {
    ("relationship", "separate-ways"):
        "; separate-ways is a decision not to collaborate, recorded as a dependency policy with policy forbidden and mode none",
}


def _closed_field_errors(records: dict[str, list[dict[str, Any]]]) -> list[str]:
    return [
        f"{name.removesuffix('.json')}/{entry.get('id')} has an unknown {field}: "
        f"{entry[field]!r} is not one of {', '.join(sorted(allowed))}"
        + (CLOSED_FIELD_HINTS.get((field, entry[field]), "") if isinstance(entry[field], str) else "")
        for name, fields in CLOSED_FIELDS.items()
        for entry in records.get(name, [])
        for field, allowed in fields.items()
        if entry.get(field) is not None and (not isinstance(entry[field], str) or entry[field] not in allowed)
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


def validate(root: Path, repo_root: Path | None, require_reviewed: bool, listing: Listing | None = None) -> list[str]:  # noqa: FBT001
    if not has_registry(root):
        return [missing_registry(root, repo_root)]
    errors = _conflict_marker_errors(root)
    if errors:
        return errors
    errors.extend(_policy_errors(root))
    if errors:
        return errors
    if require_reviewed and review_mode(root) != "scm-verified":
        errors.append("local-draft-only Domain Memory cannot satisfy --require-reviewed")
    records, documents, load_errors = _load_registry_assets(root)
    errors.extend(load_errors)
    errors.extend(_validate_asset_records(documents, records, require_reviewed))
    errors.extend(cross_reference_errors(records))
    errors.extend(_validate_evidence(records, repo_root))
    errors.extend(
        f"{shown_path(path, root)} does not list the records beside it; run refresh-indexes --registry-root {shown_path(root, None)}"
        for path in stale_indexes(root, documents)
    )
    if repo_root is not None and "contexts.json" in documents:
        errors.extend(_implementation_path_errors(documents["contexts.json"], repo_root, listing))
    return errors


def _policy_errors(root: Path) -> list[str]:
    try:
        policy = existing_policy(root)
    except ValueError as error:
        return [str(error)]
    return [] if policy is None else validate_policy(policy)


def _conflict_marker_errors(root: Path) -> list[str]:
    texts = [(registry_dir(root) / relative, text_of(data, errors="replace")) for relative, data in registry_files(root).items()]
    elsewhere = (*root.glob("*.json"), *(root / "changes").rglob("*"))
    texts.extend((path, path.read_text(encoding="utf-8", errors="replace")) for path in elsewhere if path.is_file())
    marked = ((path, conflict_marked_line(text)) for path, text in sorted(texts, key=lambda held: held[0]))
    return [
        f"{shown_path(path, root)} holds git conflict markers at line {line}; resolve the merge before validating"
        for path, line in marked
        if line is not None
    ]


def _implementation_path_errors(contexts: dict[str, Any], repo_root: Path, listing: Listing | None = None) -> list[str]:
    return [
        f"contexts/{identifier} implementation_path {path} lies over no file in the repository; "
        "after a move, relocate-evidence --from <old repository path> --to <new> proposes the new path, or correct it"
        for identifier, path, reviewed in unplaced_implementation_paths(contexts, repo_root, listing)
        if reviewed
    ]


def unplaced_implementation_paths(document: dict[str, Any], repo_root: Path,
                                  listing: Listing | None = None) -> list[tuple[Any, str, bool]]:
    contexts = _records_of("contexts.json", document)
    placed = [(entry.get("id"), entry["implementation_path"], is_reviewed("contexts", document, entry)) for entry in contexts
              if isinstance(entry.get("implementation_path"), str) and entry["implementation_path"].strip()]
    if not placed:
        return []
    directories = {directory_of(file) for file in (listing or listing_of(repo_root))()}
    return [(identifier, path, reviewed) for identifier, path, reviewed in placed
            if not _placed(owned_directory(path), directories)]


def _placed(owned: str, directories: set[str]) -> bool:
    return any(owned in directory for directory in directories)


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
    labels = entry.get(field)
    if labels is None:
        labels = []
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


EVIDENCE_STATUSES = ("current", "shifted", "stale", "unhashed", "missing", "invalid")


RecordOverlay = dict[str, dict[str, dict[str, Any] | None]]


def verify_evidence(
    root: Path, repo_root: Path, paths: list[str] | None = None, content_of: ContentReader | None = None,
    overlay: RecordOverlay | None = None,
) -> dict[str, Any]:
    source_map = source_map_for(root)
    paths = [PurePosixPath(Path(path).as_posix()).as_posix() for path in paths] if paths else None
    wanted = set(paths or ())
    classify, check = classifier(source_map), verifier(repo_root, content_of)
    results = [
        {"asset": name.removesuffix(".json"), "id": entry.get("id"), **check(classify(reference))}
        for name in ASSET_KEYS
        for entry in _overlaid(asset_records(root, name), (overlay or {}).get(name, {}))
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
            path: {
                "citations": citations[path],
                "exists": (repo_root / path).is_file() if content_of is None else content_of(path) is not None,
            }
            for path in paths
        }
    return report


def _overlaid(records: list[dict[str, Any]], changes: dict[str, dict[str, Any] | None]) -> list[dict[str, Any]]:
    if not changes:
        return records
    present = {entry.get("id") for entry in records}
    replaced = [changes[entry["id"]] if entry.get("id") in changes else entry for entry in records]
    added = [record for record_id, record in changes.items() if record_id not in present]
    return [entry for entry in [*replaced, *added] if entry is not None]


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
            for context in entry.get("contexts") or []
        },
        "aggregates": {
            entry.get("context") for entry in records.get("aggregates.json", [])
        },
        "rules": {
            context
            for entry in records.get("rules.json", [])
            for context in entry.get("contexts") or []
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


def as_stored(asset: str, record: dict[str, Any]) -> Any:
    review_keys = (review_field(asset), "verified")
    return without_derived_keys({key: value for key, value in record.items() if key not in review_keys})


def evidence_gaps(root: Path, documents: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    from .evidence import classifier, unclassified_paths, with_references

    source_map = source_map_for(root)
    classify = classifier(source_map)
    gaps = []
    for name in sorted(documents):
        for record in _records_of(name, documents[name]):
            if not is_reviewed(name.removesuffix(".json"), documents[name], record):
                continue
            entry = {"asset": name.removesuffix(".json"), "id": record.get("id")}
            if not record.get("evidence"):
                gaps.append(entry | {"reason": "no evidence"})
                continue
            outside = unclassified_paths(with_references(dict(record), classify))
            if outside:
                gaps.append(
                    entry
                    | {"reason": "outside the selected sources", "paths": outside}
                )
    return gaps


def coverage(root: Path) -> dict[str, Any]:
    documents = {name: read_document(root, name) for name in ASSET_KEYS}
    records = {name: _records_of(name, documents[name]) for name in ASSET_KEYS}
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
        "evidence_gaps": evidence_gaps(root, documents),
    }


def record_by_id(root: Path, asset: str, identifier: str) -> dict[str, Any] | None:
    return read_record(root, f"{asset}.json", identifier)


SUMMARY_FIELDS = ("statement", "definition", "responsibility", "description", "name")


def context_model(root: Path, identifier: str,
                  records: dict[str, list[dict[str, Any]]] | None = None) -> dict[str, Any] | None:
    def held(name: str) -> list[dict[str, Any]]:
        return asset_records(root, name) if records is None else records[name]

    context = (
        record_by_id(root, "contexts", identifier) if records is None
        else next((entry for entry in records["contexts.json"] if entry.get("id") == identifier), None)
    )
    if context is None:
        return None
    contracts = [
        entry for entry in held("contracts.json")
        if identifier == entry.get("producer_context") or identifier in (entry.get("consumer_contexts") or [])
    ]
    interactions = [
        entry for entry in held("interactions.json")
        if identifier in {entry.get("producer_context"), entry.get("consumer_context")}
    ]
    rules = held("rules.json")
    mode = review_mode(root)
    return {
        "usage": "constraint" if mode == "scm-verified" else "working-memory",
        "review_mode": mode,
        "context": context,
        "vocabulary": [entry for entry in held("vocabulary.json") if identifier in (entry.get("contexts") or [])],
        "aggregates": [entry for entry in held("aggregates.json") if entry.get("context") == identifier],
        "rules": [entry for entry in rules if identifier in (entry.get("contexts") or [])],
        "contracts": contracts,
        "interactions": interactions,
        "events": [
            entry for entry in held("events.json")
            if entry.get("owner_context") == identifier or identifier in (entry.get("consumers") or [])
        ],
        "capabilities": [entry for entry in held("capabilities.json") if entry.get("context") == identifier],
        "value_objects": [entry for entry in held("value-objects.json") if entry.get("context") == identifier],
        "rules_bound_through_collaborations": _bound_rules(identifier, contracts, interactions, rules),
    }


def _bound_rules(
    identifier: str, contracts: list[dict[str, Any]], interactions: list[dict[str, Any]], rules: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    collaborations = [(f"contracts/{entry.get('id')}", cited_paths(entry)) for entry in contracts] + [
        (f"interactions/{entry.get('id')}", cited_paths(entry)) for entry in interactions
    ]
    bound = []
    for rule in rules:
        if identifier in (rule.get("contexts") or []):
            continue
        cited = cited_paths(rule)
        via = [name for name, paths in collaborations if cited & paths]
        if via:
            bound.append({"rule": rule.get("id"), "contexts": rule.get("contexts", []), "statement": rule.get("statement"),
                          "via": via})
    return bound


def cited_paths(record: dict[str, Any]) -> set[str]:
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
