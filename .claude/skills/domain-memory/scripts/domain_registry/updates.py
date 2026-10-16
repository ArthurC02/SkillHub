import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .audit import read_events, reviewing_proposals
from .changes import (
    ParsedUpdate,
    parsed_update,
    superseded_ids,
    validate_change_package,
)
from .common import (
    ASSET_KEYS,
    PROPOSAL_FILE,
    completed_identifier,
    iso_timestamp,
    load_json,
    record_status,
    updates_record,
    reject_duplicate_keys,
    review_field,
    utc_now,
)
from .evidence import cited_excerpts, classify_all, hash_all, source_map_for, unclassified_paths
from .okf import contexts_named_by, verified_by
from .policy import review_mode
from .revision import (
    current_registry_revision,
    require_current_registry_revision,
)
from .store import read_document, write_document
from .transaction import mutate_registry, write_json


def reconciliation_path(root: Path) -> Path:
    return root / ".domain-registry-reconciliation.json"


def mark_applied(
    proposal_path: Path, proposal: dict[str, Any], registry_root: Path, repo_root: Path
) -> None:
    proposal["status"] = "applied"
    proposal["applied_at"] = utc_now()
    proposal["applied_registry_revision"] = current_registry_revision(
        registry_root, repo_root
    )
    write_json(proposal_path, proposal)


def rebased_revisions(
    base: dict[str, Any], accepted_digest: str, registry_root: Path, repo_root: Path
) -> dict[str, Any]:
    if base["registry_digest"] == accepted_digest:
        return {"from_revision": base}
    return {
        "from_revision": {**current_registry_revision(registry_root, repo_root), "registry_digest": accepted_digest},
        "proposal_base_revision": base,
    }


def reconcile_pending_update(registry_root: Path, repo_root: Path) -> str | None:
    marker_path = reconciliation_path(registry_root)
    if not marker_path.is_file():
        return None
    marker = load_json(marker_path)
    relative = marker.get("package")
    proposal_id = marker.get("proposal_id")
    if not isinstance(relative, str) or not completed_identifier(proposal_id):
        raise ValueError("registry reconciliation marker is malformed")
    package_root = (repo_root / relative).resolve()
    try:
        package_root.relative_to(repo_root.resolve())
    except ValueError as error:
        raise ValueError(
            "registry reconciliation marker points outside the repository"
        ) from error
    proposal_path = package_root / PROPOSAL_FILE
    proposal = load_json(proposal_path)
    if proposal.get("proposal_id") != proposal_id:
        raise ValueError("registry reconciliation marker does not match its proposal")
    outcome = _reconciled(proposal_path, proposal, registry_root, repo_root)
    marker_path.unlink()
    return outcome


def _was_applied(registry_root: Path, proposal_id: str) -> bool:
        return any(
        event.get("operation") == "apply-approved-updates" and event.get("proposal_id") == proposal_id
        for event in read_events(registry_root)
    )


def _reconciled(proposal_path: Path, proposal: dict[str, Any], registry_root: Path, repo_root: Path) -> str:
    proposal_id = proposal["proposal_id"]
    if proposal.get("status") == "applied":
        return f"{proposal_id} was already marked applied"
    if not _was_applied(registry_root, proposal_id):
        return f"{proposal_id} was never applied: the audit log holds no apply for it, so the Registry is unchanged"
    if proposal.get("status") != "approved":
        raise ValueError(
            f"the audit log records an apply of {proposal_id}, but its proposal is "
            f"{proposal.get('status')}; reconcile the package with the Registry by hand"
        )
    mark_applied(proposal_path, proposal, registry_root, repo_root)
    return f"{proposal_id} was applied before an interruption and is now marked applied"


def is_reviewed(
    asset: str, document: dict[str, Any], record: dict[str, Any] | None
) -> bool:
    return record is not None and record_status(asset, document, record) == "reviewed"


def asset_file_name(asset: str) -> str:
    name = f"{asset}.json"
    if name not in ASSET_KEYS:
        raise ValueError(f"unknown asset: {asset}")
    return name


def find_record(records: list[dict[str, Any]], record_id: Any) -> dict[str, Any] | None:
    return next((entry for entry in records if entry.get("id") == record_id), None)


def put_record(
    records: list[dict[str, Any]],
    existing: dict[str, Any] | None,
    record: dict[str, Any],
) -> None:
    if existing is None:
        records.append(record)
    else:
        records[records.index(existing)] = record


def demote_local_reviews(registry_root: Path, repo_root: Path) -> None:
    if review_mode(registry_root) != "local-draft-only":
        raise ValueError("review demotion is only for local-draft-only Domain Memory")

    def mutate(staging: Path) -> None:
        for name, key in ASSET_KEYS.items():
            document = read_document(staging, name)
            document["status"] = "candidate"
            for record in document[key]:
                record.pop("review", None)
                record.pop("verified", None)
                record[review_field(name.removesuffix(".json"))] = "candidate"
            write_document(staging, name, document)

    mutate_registry(
        registry_root, repo_root, mutate,
        audit_event=lambda: {"operation": "demote-local-reviews"},
    )


def read_update(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(
            path.read_text(encoding="utf-8"),
            object_pairs_hook=reject_duplicate_keys,
        )
    except FileNotFoundError as error:
        raise ValueError(f"no such record file: {path}") from error
    except (json.JSONDecodeError, OSError, ValueError) as error:
        raise ValueError(f"invalid JSON: {path}") from error
    if not isinstance(value, dict) or not completed_identifier(value.get("id")):
        raise ValueError(
            f"{path} must hold one record object with a completed id, such as "
            '{"id": "orders", "name": "Orders", "responsibility": "..."} — not a list of records, '
            "not a Change Package, and not the whole asset file. One record per file; "
            "give --record-file once for each record that belongs to the same change."
        )
    return value


def _batch_records(record_paths: tuple[Path, ...]) -> list[dict[str, Any]]:
    if not record_paths:
        raise ValueError("upserting a candidate requires at least one record file")
    records = [read_update(path) for path in record_paths]
    ids = [record["id"] for record in records]
    repeated = sorted({identifier for identifier in ids if ids.count(identifier) > 1})
    if repeated:
        raise ValueError(f"one batch names a record more than once: {', '.join(repeated)}")
    return records


@dataclass
class Upserted:
    outside: list[str] = field(default_factory=list)
    created: int = 0
    same_evidence: list[str] = field(default_factory=list)


def upsert_candidate(
    root: Path, repo_root: Path | None, asset: str, *record_paths: Path
) -> Upserted:
    name = asset_file_name(asset)
    batch = _batch_records(record_paths)
    if repo_root is not None:
        batch = [hash_all(record, repo_root) for record in batch]
    source_map = source_map_for(root)
    result = Upserted()
    outside = result.outside

    def mutate(staging: Path) -> None:
        document = read_document(staging, name)
        records = document[ASSET_KEYS[name]]
        result.created = 0
        result.same_evidence.clear()
        for record in batch:
            existing = find_record(records, record["id"])
            if is_reviewed(asset, document, existing):
                raise ValueError(f"cannot overwrite reviewed record: {record['id']}")
            result.created += existing is None
            result.same_evidence.extend(_citing_the_same_excerpts(asset, records, record))
            candidate = classify_all(dict(record), source_map)
            outside.extend(unclassified_paths(candidate))
            candidate.pop("review", None)
            candidate.pop("verified", None)
            candidate[review_field(asset)] = "candidate"
            put_record(records, existing, candidate)
        write_document(staging, name, document)

    identity = (
        {"record_id": batch[0]["id"]}
        if len(batch) == 1
        else {"record_ids": [record["id"] for record in batch]}
    )
    mutate_registry(
        root,
        repo_root,
        mutate,
        audit_event=lambda: {"operation": "upsert-candidate", "asset": asset, **identity},
    )
    result.outside = sorted(set(outside))
    return result


def _citing_the_same_excerpts(asset: str, records: list[dict[str, Any]], record: dict[str, Any]) -> list[str]:
    excerpts = cited_excerpts(record)
    return [
        f"{asset}/{other.get('id')}"
        for other in records
        if excerpts and other.get("id") != record["id"] and cited_excerpts(other) == excerpts
    ]


def retract_candidate(
    root: Path, repo_root: Path | None, asset: str, record_id: str, reason: str
) -> None:
    name = asset_file_name(asset)
    if not reason.strip():
        raise ValueError(
            "retracting a candidate requires the reason it does not belong"
        )

    def mutate(staging: Path) -> None:
        document = read_document(staging, name)
        records = document[ASSET_KEYS[name]]
        existing = find_record(records, record_id)
        if existing is None:
            raise ValueError(f"no such record: {asset}/{record_id}")
        if is_reviewed(asset, document, existing):
            raise ValueError(
                f"cannot retract reviewed record: {record_id}; a reviewed record "
                "is withdrawn through a superseding proposal"
            )
        records.remove(existing)
        write_document(staging, name, document)

    mutate_registry(
        root,
        repo_root,
        mutate,
        audit_event=lambda: {
            "operation": "retract-candidate",
            "asset": asset,
            "record_id": record_id,
            "reason": reason.strip(),
        },
    )


def _refuse_unsuperseded_overwrite(
    update: ParsedUpdate, document: dict[str, Any], existing: Any, proposal: dict[str, Any], reviewing: dict[str, str]
) -> None:
    if not is_reviewed(update.asset, document, existing):
        return
    reviewed_by = reviewing.get(f"{update.asset}/{update.record_id}")
    if not reviewed_by or reviewed_by not in (superseded_ids(proposal) or set()):
        raise ValueError(
            f"cannot overwrite reviewed record: {update.record_id}; a proposal that "
            f"replaces it must list {reviewed_by or 'the proposal that reviewed it'} in supersedes"
        )


def _reviewed_record(
    update: ParsedUpdate, proposal: dict[str, Any], source_map: Any
) -> dict[str, Any]:
    promoted = classify_all(dict(update.record or {}), source_map)
    outside = unclassified_paths(promoted)
    if outside:
        raise ValueError(
            f"cannot review a record whose evidence rests outside the "
            f"confirmed sources: {update.asset}/{update.record_id} cites "
            + ", ".join(outside)
            + "; cite a confirmed source, or select that path before "
            "the record is reviewed"
        )
    promoted.pop("review", None)
    promoted["verified"] = verified_by(proposal["approvals"])
    promoted[review_field(update.asset)] = "reviewed"
    return promoted


def _apply_update(
    update: ParsedUpdate, document: dict[str, Any], proposal: dict[str, Any], source_map: Any,
    reviewing: dict[str, str],
) -> None:
    records = document[ASSET_KEYS[update.document_name]]
    existing = find_record(records, update.record_id)
    _refuse_unsuperseded_overwrite(update, document, existing, proposal, reviewing)
    if update.record is not None:
        put_record(records, existing, _reviewed_record(update, proposal, source_map))
        return
    if existing is None:
        raise ValueError(
            f"cannot remove absent record: {update.asset}/{update.record_id}; a removal "
            "names a record the Registry holds"
        )
    records.remove(existing)


def _finalized_proposal(package_root: Path, registry_root: Path) -> dict[str, Any]:
    if review_mode(registry_root) != "scm-verified":
        raise ValueError(
            "reviewed updates require an scm-verified Domain Memory policy"
        )
    errors = validate_change_package(package_root, registry_root)
    if errors:
        raise ValueError("change package is invalid: " + "; ".join(errors))
    proposal = load_json(package_root / PROPOSAL_FILE)
    if proposal.get("status") != "approved" or not iso_timestamp(
        proposal.get("finalized_at")
    ):
        raise ValueError(
            "only a finalized approved proposal may apply registry updates"
        )
    return proposal


def apply_approved_updates(
    package_root: Path, registry_root: Path, repo_root: Path
) -> None:
    reconcile_pending_update(registry_root, repo_root)
    proposal = _finalized_proposal(package_root, registry_root)
    accepted_digest = require_current_registry_revision(proposal, registry_root, repo_root)
    updates = proposal.get("registry_updates", [])
    if not isinstance(updates, list) or not updates:
        raise ValueError("approved proposal has no registry_updates")
    source_map = source_map_for(registry_root)
    reviewing = reviewing_proposals(registry_root)
    parsed = [parsed_update(update) for update in updates]
    touched = sorted({f"{update.asset}/{update.record_id}" for update in parsed})

    def mutate(staging: Path) -> None:
        documents: dict[str, dict[str, Any]] = {}
        for update in parsed:
            if update.document_name not in documents:
                documents[update.document_name] = read_document(staging, update.document_name)
            _apply_update(update, documents[update.document_name], proposal, source_map, reviewing)
        for name, document in documents.items():
            write_document(staging, name, document)

    base_revision = proposal["base_registry_revision"]
    relative_package = package_root.resolve().relative_to(repo_root.resolve())
    write_json(
        reconciliation_path(registry_root),
        {
            "format": "domain-registry-reconciliation/v1",
            "package": relative_package.as_posix(),
            "proposal_id": proposal["proposal_id"],
        },
    )
    try:
        mutate_registry(
            registry_root,
            repo_root,
            mutate,
            accepted_digest,
            audit_event=lambda: {
                "operation": "apply-approved-updates",
                "proposal_id": proposal["proposal_id"],
                "records": touched,
                **rebased_revisions(base_revision, accepted_digest, registry_root, repo_root),
                "to_revision": current_registry_revision(registry_root, repo_root),
            },
        )
    except Exception:
        if not _was_applied(registry_root, proposal["proposal_id"]):
            reconciliation_path(registry_root).unlink(missing_ok=True)
        raise
    mark_applied(package_root / PROPOSAL_FILE, proposal, registry_root, repo_root)
    reconciliation_path(registry_root).unlink(missing_ok=True)

def promote_candidate(
    package_root: Path, registry_root: Path, asset: str, record_id: str
) -> None:
    path = package_root / PROPOSAL_FILE
    proposal = load_json(path)
    if proposal.get("status") != "draft":
        raise ValueError(
            "registry updates are added while the proposal is a draft; a submitted proposal's "
            "content is what its approvals cover. To change it, run supersede-proposal, then "
            "redraft-proposal into a new package, and promote into that draft"
        )
    name = asset_file_name(asset)
    document = read_document(registry_root, name)
    record = find_record(document[ASSET_KEYS[name]], record_id)
    if record is None:
        raise ValueError(
            f"no {asset}/{record_id} in the Registry; record it with upsert-candidate first"
        )
    if is_reviewed(asset, document, record):
        raise ValueError(
            f"{asset}/{record_id} is already reviewed; record the change as a candidate with "
            "upsert-candidate, then promote that"
        )
    promoted = {
        key: value
        for key, value in record.items()
        if key not in {review_field(asset), "review", "verified"}
    }
    update = {"operation": "upsert", "asset": asset, "record": promoted}
    updates = [
        entry
        for entry in proposal.get("registry_updates", [])
        if not updates_record(entry, asset, record_id)
    ]
    proposal["registry_updates"] = [*updates, update]
    named = proposal.get("affected_contexts")
    proposal["affected_contexts"] = sorted(
        ({context for context in named if isinstance(context, str)} if isinstance(named, list) else set())
        | contexts_named_by(asset, promoted)
    )
    write_json(path, proposal)
