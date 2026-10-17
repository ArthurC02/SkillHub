from collections import deque
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS
from .okf import LIST_RELATIONS, RELATION_TARGETS
from .registry import asset_records, summarized

HUB_ASSET = "contexts"


def dependents(root: Path, asset: str, record_id: str) -> dict[str, Any]:
    records = {name.removesuffix(".json"): asset_records(root, name) for name in ASSET_KEYS}
    by_key = {(name, entry.get("id")): entry for name, entries in records.items() for entry in entries}
    start = (asset, record_id)
    if start not in by_key:
        raise ValueError(f"{asset} record not found: {record_id}")
    edges = _edges(records)
    reached: dict[tuple[str, Any], dict[str, Any]] = {}
    queue = deque([(start, 0)])
    while queue:
        key, depth = queue.popleft()
        if key[0] == HUB_ASSET and key != start:
            continue
        for other, through in edges.get(key, []):
            if other == start or other in reached or other not in by_key:
                continue
            reached[other] = {**summarized(by_key[other]), "asset": other[0], "through": through, "depth": depth + 1}
            queue.append((other, depth + 1))
    return {
        "asset": asset,
        "id": record_id,
        "reached": sorted(reached.values(), key=lambda item: (item["depth"], item["asset"], str(item["id"]))),
    }


def _edges(records: dict[str, list[dict[str, Any]]]) -> dict[tuple[str, Any], list[tuple[tuple[str, Any], str]]]:
    edges: dict[tuple[str, Any], list[tuple[tuple[str, Any], str]]] = {}
    for name, entries in records.items():
        for entry in entries:
            source = (name, entry.get("id"))
            for field, target_asset in RELATION_TARGETS.items():
                value = entry.get(field)
                targets = value if field in LIST_RELATIONS and isinstance(value, list) else [value]
                for target in targets:
                    if not isinstance(target, str):
                        continue
                    through = f"{name}/{entry.get('id')}.{field}"
                    edges.setdefault(source, []).append(((target_asset, target), through))
                    edges.setdefault((target_asset, target), []).append((source, through))
    return edges
