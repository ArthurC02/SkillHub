from __future__ import annotations

from collections import deque
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS
from .okf import LIST_RELATIONS, RELATION_TARGETS
from .registry import asset_records, summarized

HUB_ASSET = "contexts"
Key = tuple[str, Any]


@dataclass(frozen=True)
class DependencyGraph:
    by_key: dict[Key, dict[str, Any]]
    edges: dict[Key, list[tuple[Key, str]]]

    @classmethod
    def of(cls, root: Path) -> DependencyGraph:
        return cls.from_documents({name: asset_records(root, name) for name in ASSET_KEYS})

    @classmethod
    def from_documents(cls, documents: dict[str, list[dict[str, Any]]]) -> DependencyGraph:
        records = {name.removesuffix(".json"): entries for name, entries in documents.items()}
        by_key = {(name, entry.get("id")): entry for name, entries in records.items() for entry in entries}
        return cls(by_key, _edges(records))

    def record(self, asset: str, record_id: Any) -> dict[str, Any] | None:
        return self.by_key.get((asset, record_id))

    def dependents(self, asset: str, record_id: str) -> dict[str, Any]:
        start = (asset, record_id)
        if start not in self.by_key:
            raise ValueError(f"{asset} record not found: {record_id}")
        reached: dict[Key, dict[str, Any]] = {}
        queue = deque([(start, 0)])
        while queue:
            key, depth = queue.popleft()
            if key[0] == HUB_ASSET and key != start:
                continue
            for other, through in self.edges.get(key, []):
                if other == start or other in reached or other not in self.by_key:
                    continue
                reached[other] = {**summarized(self.by_key[other]), "asset": other[0], "through": through, "depth": depth + 1}
                queue.append((other, depth + 1))
        return {
            "asset": asset,
            "id": record_id,
            "reached": sorted(reached.values(), key=lambda item: (item["depth"], item["asset"], str(item["id"]))),
        }


def dependents(root: Path, asset: str, record_id: str, graph: DependencyGraph | None = None) -> dict[str, Any]:
    return (graph or DependencyGraph.of(root)).dependents(asset, record_id)


def _edges(records: dict[str, list[dict[str, Any]]]) -> dict[Key, list[tuple[Key, str]]]:
    edges: dict[Key, list[tuple[Key, str]]] = {}
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
