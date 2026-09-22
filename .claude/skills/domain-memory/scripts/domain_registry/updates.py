import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from .changes import validate_change_package
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
    proposal["status"] = "applied"
    proposal["applied_at"] = (
        datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    )
    proposal["applied_registry_revision"] = current_registry_revision(
        registry_root, repo_root
    )
    write_json(proposal_path, proposal)
    marker_path.unlink()


def review_status(asset: str, record: dict[str, Any], asset_status: str) -> str:
    if asset == "decisions":
        return record.get("review_status", asset_status)
    return record.get("status", asset_status)


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
                if name == "decisions.json":
                    record["review_status"] = "candidate"
                else:
                    record["status"] = "candidate"
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
    name = f"{asset}.json"
    if name not in ASSET_KEYS:
        raise ValueError(f"unknown asset: {asset}")
    record = read_update(record_path)
    source_map = source_map_for(root)
    outside: list[str] = []

    def mutate(staging: Path) -> None:
        path = registry_dir(staging) / name
        document = load_json(path)
        records = document[ASSET_KEYS[name]]
        existing = next(
            (entry for entry in records if entry.get("id") == record["id"]), None
        )
        if (
            existing is not None
            and review_status(asset, existing, document.get("status", "candidate"))
            == "reviewed"
        ):
            raise ValueError(f"cannot overwrite reviewed record: {record['id']}")
        candidate = classify_all(dict(record), source_map)
        outside.extend(unclassified_paths(candidate))
        candidate.pop("review", None)
        if asset == "decisions":
            candidate["review_status"] = "candidate"
        else:
            candidate["status"] = "candidate"
        if existing is None:
            records.append(candidate)
        else:
            records[records.index(existing)] = candidate
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
    name = f"{asset}.json"
    if name not in ASSET_KEYS:
        raise ValueError(f"unknown asset: {asset}")
    if not reason.strip():
        raise ValueError(
            "retracting a candidate requires the reason it does not belong"
        )

    def mutate(staging: Path) -> None:
        path = registry_dir(staging) / name
        document = load_json(path)
        records = document[ASSET_KEYS[name]]
        existing = next(
            (entry for entry in records if entry.get("id") == record_id), None
        )
        if existing is None:
            raise ValueError(f"no such record: {asset}/{record_id}")
        if (
            review_status(asset, existing, document.get("status", "candidate"))
            == "reviewed"
        ):
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


def apply_approved_updates(
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
    proposal = load_json(package_root / "domain-change-proposal.json")
    if proposal.get("status") != "approved" or not iso_timestamp(
        proposal.get("finalized_at")
    ):
        raise ValueError(
            "only a finalized approved proposal may apply registry updates"
        )
    require_current_registry_revision(proposal, registry_root, repo_root)
    updates = proposal.get("registry_updates", [])
    if not isinstance(updates, list) or not updates:
        raise ValueError("approved proposal has no registry_updates")

    def mutate(staging: Path) -> None:
        documents: dict[str, dict[str, Any]] = {}
        for update in updates:
            if not isinstance(update, dict) or update.get("operation") != "upsert":
                raise ValueError("each registry update must be an upsert")
            asset = update.get("asset")
            record = update.get("record")
            if (
                not isinstance(asset, str)
                or f"{asset}.json" not in ASSET_KEYS
                or not isinstance(record, dict)
                or not completed_identifier(record.get("id"))
            ):
                raise ValueError("registry update has an invalid asset or record")
            name = f"{asset}.json"
            document = documents.setdefault(
                name, load_json(registry_dir(staging) / name)
            )
            records = document[ASSET_KEYS[name]]
            existing = next(
                (entry for entry in records if entry.get("id") == record["id"]), None
            )
            if (
                existing is not None
                and review_status(asset, existing, document.get("status", "candidate"))
                == "reviewed"
            ):
                reviewed_by = (existing.get("review") or {}).get("proposal_id")
                if not reviewed_by or proposal.get("supersedes") != reviewed_by:
                    raise ValueError(
                        f"cannot overwrite reviewed record: {record['id']}; a proposal that "
                        f"replaces it must supersede {reviewed_by or 'the proposal that reviewed it'}"
                    )
            promoted = dict(record)
            promoted["review"] = {
                "proposal_id": proposal["proposal_id"],
                "proposal_revision": proposal["proposal_revision"],
                "approvals": proposal["approvals"],
            }
            if asset == "decisions":
                promoted["review_status"] = "reviewed"
            else:
                promoted["status"] = "reviewed"
            if existing is None:
                records.append(promoted)
            else:
                records[records.index(existing)] = promoted
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
        base_revision["registry_digest"],
        audit_event=lambda: {
            "operation": "apply-approved-updates",
            "proposal_id": proposal["proposal_id"],
            "from_revision": base_revision,
            "to_revision": current_registry_revision(registry_root, repo_root),
        },
    )
    proposal["status"] = "applied"
    proposal["applied_at"] = (
        datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    )
    proposal["applied_registry_revision"] = current_registry_revision(
        registry_root, repo_root
    )
    path = package_root / "domain-change-proposal.json"
    write_json(path, proposal)
    reconciliation_path(registry_root).unlink(missing_ok=True)
