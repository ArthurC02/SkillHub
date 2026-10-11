from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS, load_json, registry_dir
from .evidence import citation, classified, excerpt_starts, source_map_for


@dataclass
class Relocation:
    repo_root: Path
    old: str
    new: str
    source_map: dict[str, Any]
    unmatched: list[dict[str, Any]] = field(default_factory=list)

    def target(self, path: str) -> str | None:
        old, new = self.old.rstrip("/"), self.new.rstrip("/")
        if path == old:
            return new
        return new + path[len(old):] if path.startswith(old + "/") else None

    def moved(self, value: Any) -> Any:
        if isinstance(value, dict):
            return {
                key: [self.reference(item) for item in child]
                if key == "evidence" and isinstance(child, list)
                else self.moved(child)
                for key, child in value.items()
            }
        if isinstance(value, list):
            return [self.moved(child) for child in value]
        return value

    def reference(self, reference: Any) -> Any:
        target = self.target(reference.get("path", "")) if isinstance(reference, dict) else None
        if target is None:
            return reference
        lines = reference.get("lines") or {}
        length = lines.get("end", 0) - lines.get("start", 0) + 1
        excerpt = reference.get("excerpt_sha256")
        starts = excerpt_starts(self.repo_root, target, length, excerpt) if excerpt and length > 0 else []
        if len(starts) != 1:
            self.unmatched.append({"path": reference.get("path"), "lines": lines, "target": target, "found": len(starts)})
            return reference
        recited = citation(self.repo_root, target, starts[0], starts[0] + length - 1)
        kept = {key: value for key, value in reference.items() if key != "source_kind"}
        return classified({**kept, **recited}, self.source_map)


def _status(name: str, document: dict[str, Any], entry: dict[str, Any]) -> Any:
    field_name = "review_status" if name == "decisions.json" else "status"
    return entry.get(field_name, document.get("status"))


def relocate_evidence(root: Path, repo_root: Path, old: str, new: str) -> dict[str, Any]:
    relocation = Relocation(repo_root, old, new, source_map_for(root))
    updates, candidates, supersedes = [], [], set()
    for name in ASSET_KEYS:
        document = load_json(registry_dir(root) / name)
        for entry in document.get(ASSET_KEYS[name], []):
            unmatched_before = len(relocation.unmatched)
            moved = relocation.moved(entry)
            for problem in relocation.unmatched[unmatched_before:]:
                problem.update(asset=name.removesuffix(".json"), id=entry.get("id"))
            if moved == entry:
                continue
            record = {key: value for key, value in moved.items() if key != "review"}
            if _status(name, document, entry) == "reviewed":
                updates.append({"operation": "upsert", "asset": name.removesuffix(".json"), "record": record})
                supersedes.add((entry.get("review") or {}).get("proposal_id"))
            else:
                candidates.append({"asset": name.removesuffix(".json"), "record": record})
    return {
        "from": old,
        "to": new,
        "registry_updates": updates,
        "supersedes": sorted(proposal for proposal in supersedes if proposal),
        "candidates": candidates,
        "unmatched": relocation.unmatched,
    }
