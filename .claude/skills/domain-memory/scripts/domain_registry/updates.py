import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from .changes import PROPOSAL_FILE, validate_change_package
from .common import (
    ASSET_KEYS,
    completed_identifier,
    iso_timestamp,
    load_json,
    registry_dir,
    reject_duplicate_keys,
)
from .evidence import classify_all, source_map_for, unclassified_paths
from .policy import review_mode
from .revision import (
    current_registry_revision,
    require_current_registry_revision,
    valid_registry_revision,
)
from .transaction import mutate_registry, write_json


def reconciliation_path(root: Path) -> Path:
    return root / ".domain-registry-reconciliation.json"


def mark_applied(
    proposal_path: Path, proposal: dict[str, Any], registry_root: Path, repo_root: Path
) -> None:
    proposal["status"] = "applied"
    proposal["applied_at"] = (
        datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    )
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


def reconcile_pending_update(registry_root: Path, repo_root: Path) -> None:
    marker_path = reconciliation_path(registry_root)
    if not marker_path.is_file():
        return
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
    proposal_path = package_root / "domain-change-proposal.json"
    proposal = load_json(proposal_path)
    if proposal.get("proposal_id") != proposal_id:
        raise ValueError("registry reconciliation marker does not match its proposal")
    if proposal.get("status") == "applied":
        marker_path.unlink()
        return
    if proposal.get("status") != "approved":
        raise ValueError("registry reconciliation requires an approved proposal")
    from .audit import read_events

    events = read_events(registry_root)
    if (
        not events
        or events[-1].get("operation") != "apply-approved-updates"
        or events[-1].get("proposal_id") != proposal_id
    ):
        raise ValueError(
            "registry reconciliation cannot prove the approved update was committed"
        )
    mark_applied(proposal_path, proposal, registry_root, repo_root)
    marker_path.unlink()


def review_field(asset: str) -> str:
    return "review_status" if asset == "decisions" else "status"


def is_reviewed(
    asset: str, document: dict[str, Any], record: dict[str, Any] | None
) -> bool:
    return (
        record is not None
        and record.get(review_field(asset), document.get("status", "candidate"))
        == "reviewed"
    )


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
            path = registry_dir(staging) / name
            document = load_json(path)
            document["status"] = "candidate"
            for record in document[key]:
                record.pop("review", None)
                record[review_field(name.removesuffix(".json"))] = "candidate"
            write_json(path, document)

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
    except (json.JSONDecodeError, OSError, ValueError) as error:
        raise ValueError(f"invalid JSON: {path}") from error
    if not isinstance(value, dict) or not completed_identifier(value.get("id")):
        raise ValueError(
            f"{path} must hold one record object with a completed id, such as "
            '{"id": "orders", "name": "Orders", "responsibility": "..."} — not a list of records, '
            "not a Change Package, and not the whole asset file. One record per call."
        )
    return value


def upsert_candidate(
    root: Path, repo_root: Path | None, asset: str, record_path: Path
) -> list[str]:
    name = asset_file_name(asset)
    record = read_update(record_path)
    source_map = source_map_for(root)
    outside: list[str] = []

    def mutate(staging: Path) -> None:
        path = registry_dir(staging) / name
        document = load_json(path)
        records = document[ASSET_KEYS[name]]
        existing = find_record(records, record["id"])
        if is_reviewed(asset, document, existing):
            raise ValueError(f"cannot overwrite reviewed record: {record['id']}")
        candidate = classify_all(dict(record), source_map)
        outside.extend(unclassified_paths(candidate))
        candidate.pop("review", None)
        candidate[review_field(asset)] = "candidate"
        put_record(records, existing, candidate)
        write_json(path, document)

    mutate_registry(
        root,
        repo_root,
        mutate,
        audit_event=lambda: {
            "operation": "upsert-candidate",
            "asset": asset,
            "record_id": record["id"],
        },
    )
    return sorted(set(outside))


def retract_candidate(
    root: Path, repo_root: Path | None, asset: str, record_id: str, reason: str
) -> None:
    name = asset_file_name(asset)
    if not reason.strip():
        raise ValueError(
            "retracting a candidate requires the reason it does not belong"
        )

    def mutate(staging: Path) -> None:
        path = registry_dir(staging) / name
        document = load_json(path)
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
        write_json(path, document)

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


def apply_approved_updates(  # noqa: C901, PLR0915
    package_root: Path, registry_root: Path, repo_root: Path
) -> None:
    reconcile_pending_update(registry_root, repo_root)
    if review_mode(registry_root) != "scm-verified":
        raise ValueError(
            "reviewed updates require an scm-verified Domain Memory policy"
        )
    errors = validate_change_package(package_root, registry_root)
    if errors:
        raise ValueError("change package is invalid: " + "; ".join(errors))
    proposal_path = package_root / "domain-change-proposal.json"
    proposal = load_json(proposal_path)
    if proposal.get("status") != "approved" or not iso_timestamp(
        proposal.get("finalized_at")
    ):
        raise ValueError(
            "only a finalized approved proposal may apply registry updates"
        )
    accepted_digest = require_current_registry_revision(proposal, registry_root, repo_root)
    updates = proposal.get("registry_updates", [])
    if not isinstance(updates, list) or not updates:
        raise ValueError("approved proposal has no registry_updates")
    source_map = source_map_for(registry_root)

    def mutate(staging: Path) -> None:
        documents: dict[str, dict[str, Any]] = {}
        for update in updates:
            if not isinstance(update, dict) or update.get("operation") not in (
                "upsert",
                "remove",
            ):
                raise ValueError("each registry update must be an upsert or a remove")
            removing = update.get("operation") == "remove"
            asset = update.get("asset")
            record = update.get("id") if removing else update.get("record")
            record_id = record if removing else (record or {}).get("id")
            if (
                not isinstance(asset, str)
                or f"{asset}.json" not in ASSET_KEYS
                or (not removing and not isinstance(record, dict))
                or not completed_identifier(record_id)
            ):
                raise ValueError("registry update has an invalid asset or record")
            name = f"{asset}.json"
            document = documents.setdefault(
                name, load_json(registry_dir(staging) / name)
            )
            records = document[ASSET_KEYS[name]]
            existing = find_record(records, record_id)
            if is_reviewed(asset, document, existing):
                reviewed_by = (existing.get("review") or {}).get("proposal_id")
                if not reviewed_by or proposal.get("supersedes") != reviewed_by:
                    raise ValueError(
                        f"cannot overwrite reviewed record: {record_id}; a proposal that "
                        f"replaces it must supersede {reviewed_by or 'the proposal that reviewed it'}"
                    )
            if removing:
                if existing is None:
                    raise ValueError(
                        f"cannot remove absent record: {asset}/{record_id}; a removal "
                        "names a record the Registry holds"
                    )
                records.remove(existing)
                continue
            promoted = classify_all(dict(record), source_map)
            outside = unclassified_paths(promoted)
            if outside:
                raise ValueError(
                    f"cannot review a record whose evidence rests outside the "
                    f"confirmed sources: {asset}/{record_id} cites "
                    + ", ".join(outside)
                    + "; cite a confirmed source, or select that path before "
                    "the record is reviewed"
                )
            promoted["review"] = {
                "proposal_id": proposal["proposal_id"],
                "proposal_revision": proposal["proposal_revision"],
                "approvals": proposal["approvals"],
            }
            promoted[review_field(asset)] = "reviewed"
            put_record(records, existing, promoted)
        for name, document in documents.items():
            write_json(registry_dir(staging) / name, document)

    base_revision = proposal.get("base_registry_revision")
    if not valid_registry_revision(base_revision):
        raise ValueError("approved proposal has an invalid base Registry revision")
    relative_package = package_root.resolve().relative_to(repo_root.resolve())
    write_json(
        reconciliation_path(registry_root),
        {
            "format": "domain-registry-reconciliation/v1",
            "package": relative_package.as_posix(),
            "proposal_id": proposal["proposal_id"],
        },
    )
    mutate_registry(
        registry_root,
        repo_root,
        mutate,
        accepted_digest,
        audit_event=lambda: {
            "operation": "apply-approved-updates",
            "proposal_id": proposal["proposal_id"],
            **rebased_revisions(base_revision, accepted_digest, registry_root, repo_root),
            "to_revision": current_registry_revision(registry_root, repo_root),
        },
    )
    mark_applied(proposal_path, proposal, registry_root, repo_root)
    reconciliation_path(registry_root).unlink(missing_ok=True)


def promote_candidate(
    package_root: Path, registry_root: Path, asset: str, record_id: str
) -> None:
    path = package_root / PROPOSAL_FILE
    proposal = load_json(path)
    if proposal.get("status") != "draft":
        raise ValueError(
            "registry updates are added while the proposal is a draft; a submitted proposal's "
            "content is what its approvals cover"
        )
    name = asset_file_name(asset)
    document = load_json(registry_dir(registry_root) / name)
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
        if key not in {review_field(asset), "review"}
    }
    update = {"operation": "upsert", "asset": asset, "record": promoted}
    updates = [
        entry
        for entry in proposal.get("registry_updates", [])
        if not (entry.get("asset") == asset and (entry.get("record") or {}).get("id") == record_id)
    ]
    proposal["registry_updates"] = [*updates, update]
    write_json(path, proposal)
