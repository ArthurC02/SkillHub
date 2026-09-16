from .changes import validate_change_package
from .common import ASSET_KEYS, completed_identifier, iso_timestamp, load_json, registry_dir
from .revision import current_registry_revision, require_current_registry_revision, valid_registry_revision
from .policy import review_mode
from .audit import append as append_audit
from .registry import asset_records
from .evidence import classify_all, source_map_for
from .transaction import mutate_registry

from pathlib import Path
from typing import Any
import json
from datetime import datetime, timezone


def review_status(asset: str, record: dict[str, Any], asset_status: str) -> str:
    if asset == "decisions":
        return record.get("review_status", asset_status)
    return record.get("status", asset_status)


def read_update(path: Path) -> dict[str, Any]:
    value = load_json(path)
    if not completed_identifier(value.get("id")):
        raise ValueError(f"update record requires a completed id: {path}")
    return value


def upsert_candidate(root: Path, repo_root: Path | None, asset: str, record_path: Path) -> None:
    name = f"{asset}.json"
    if name not in ASSET_KEYS:
        raise ValueError(f"unknown asset: {asset}")
    record = read_update(record_path)
    source_map = source_map_for(root)
    def mutate(staging: Path) -> None:
        path = registry_dir(staging) / name
        document = load_json(path)
        records = document[ASSET_KEYS[name]]
        existing = next((entry for entry in records if entry.get("id") == record["id"]), None)
        if existing is not None and review_status(asset, existing, document.get("status", "candidate")) == "reviewed":
            raise ValueError(f"cannot overwrite reviewed record: {record['id']}")
        candidate = classify_all(dict(record), source_map)
        if asset == "decisions":
            candidate["review_status"] = "candidate"
        else:
            candidate["status"] = "candidate"
        if existing is None:
            records.append(candidate)
        else:
            records[records.index(existing)] = candidate
        from .transaction import write_json
        write_json(path, document)
    mutate_registry(root, repo_root, mutate)
    append_audit(root, {"operation": "upsert-candidate", "asset": asset, "record_id": record["id"]})


def apply_approved_updates(package_root: Path, registry_root: Path, repo_root: Path) -> None:
    if review_mode(registry_root) != "scm-verified":
        raise ValueError("reviewed updates require an scm-verified Domain Memory policy")
    errors = validate_change_package(package_root, registry_root)
    if errors:
        raise ValueError("change package is invalid: " + "; ".join(errors))
    proposal = load_json(package_root / "domain-change-proposal.json")
    if proposal.get("status") != "approved" or not iso_timestamp(proposal.get("finalized_at")):
        raise ValueError("only a finalized approved proposal may apply registry updates")
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
            name = f"{asset}.json"
            if name not in ASSET_KEYS or not isinstance(record, dict) or not completed_identifier(record.get("id")):
                raise ValueError("registry update has an invalid asset or record")
            document = documents.setdefault(name, load_json(registry_dir(staging) / name))
            records = document[ASSET_KEYS[name]]
            existing = next((entry for entry in records if entry.get("id") == record["id"]), None)
            if existing is not None and review_status(asset, existing, document.get("status", "candidate")) == "reviewed":
                raise ValueError(f"cannot overwrite reviewed record: {record['id']}")
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
        from .transaction import write_json
        for name, document in documents.items():
            write_json(registry_dir(staging) / name, document)
    base_revision = proposal.get("base_registry_revision")
    if not valid_registry_revision(base_revision):
        raise ValueError("approved proposal has an invalid base Registry revision")
    mutate_registry(registry_root, repo_root, mutate, base_revision["registry_digest"])
    proposal["status"] = "applied"
    proposal["applied_at"] = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    proposal["applied_registry_revision"] = current_registry_revision(registry_root, repo_root)
    path = package_root / "domain-change-proposal.json"
    temporary = path.with_suffix(".json.tmp")
    temporary.write_text(json.dumps(proposal, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    temporary.replace(path)
    append_audit(registry_root, {"operation": "apply-approved-updates", "proposal_id": proposal["proposal_id"], "from_revision": base_revision, "to_revision": proposal["applied_registry_revision"]})
