from __future__ import annotations

from pathlib import Path
from typing import Any

from .common import ASSET_KEYS
from .compare import compare_registries
from .evidence import hash_all, source_map_for
from .okf import CONTEXT_RELATIONS
from .store import read_document, write_document
from .transaction import mutate_registry
from .updates import Staging, Upserted, stage_candidates


def harvest_candidates(root: Path, repo_root: Path, from_root: Path, context_map: dict[str, str]) -> dict[str, Any]:
    scratch = _registry(from_root)
    paired = {
        pair["candidate"]: pair["gold"]
        for pair in compare_registries(_registry(root), scratch)["assets"]["contexts"]["matched"]
        if pair["candidate"] != pair["gold"]
    }
    paired.update(context_map)
    batches = {
        asset: [
            hash_all(_renamed(record, paired), repo_root)
            for record in document[key]
            if not (asset == "contexts" and record["id"] in paired)
        ]
        for asset, key, document in _documents(scratch)
    }
    staging = Staging(source_map_for(root), skip_reviewed=True)
    result = Upserted()

    def mutate(staged: Path) -> None:
        result.reset()
        for asset, batch in batches.items():
            if batch:
                document = read_document(staged, f"{asset}.json")
                stage_candidates(document, asset, batch, staging, result)
                write_document(staged, f"{asset}.json", document)

    def harvested() -> list[str]:
        return [
            f"{asset}/{record['id']}" for asset, batch in batches.items() for record in batch
            if f"{asset}/{record['id']}" not in result.skipped_reviewed
        ]

    mutate_registry(root, repo_root, mutate, audit_event=lambda: {"operation": "harvest-candidates", "records": harvested()})
    return {
        "context_map": dict(sorted(paired.items())),
        "harvested": harvested(),
        "created": result.created,
        "updated": len(harvested()) - result.created,
        "skipped_reviewed": result.skipped_reviewed,
        "outside_sources": sorted(set(result.outside)),
        "possible_duplicates": sorted(set(result.same_evidence)),
    }


def _registry(root: Path) -> dict[str, dict[str, Any]]:
    registry = {}
    for name, key in ASSET_KEYS.items():
        document = read_document(root, name)
        registry[name.removesuffix(".json")] = {
            key: [record for record in document.get(key, []) if isinstance(record, dict) and record.get("id")]
        }
    return registry


def _documents(registry: dict[str, dict[str, Any]]) -> list[tuple[str, str, dict[str, Any]]]:
    return [(name.removesuffix(".json"), key, registry[name.removesuffix(".json")]) for name, key in ASSET_KEYS.items()]


def _renamed(record: dict[str, Any], paired: dict[str, str]) -> dict[str, Any]:
    renamed = dict(record)
    for field in CONTEXT_RELATIONS:
        value = record.get(field)
        if isinstance(value, str):
            renamed[field] = paired.get(value, value)
        elif isinstance(value, list):
            renamed[field] = [paired.get(entry, entry) if isinstance(entry, str) else entry for entry in value]
    return renamed
