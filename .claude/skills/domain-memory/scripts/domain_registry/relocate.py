from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS, load_json, record_status, registry_dir
from .evidence import (
    WIDEN_TO_UNIQUE,
    all_references,
    citation,
    classified,
    excerpt_starts,
    source_map_for,
    verify,
    with_references,
)


@dataclass
class Relocation:
    repo_root: Path
    old: str
    new: str
    source_map: dict[str, Any]
    unmatched: list[dict[str, Any]] = field(default_factory=list)

    def target(self, path: str) -> str | None:
        old, new = self.old.rstrip("/"), self.new.rstrip("/")
        if old in ("", ".") and new in ("", "."):
            return path
        if path == old:
            return new
        return new + path[len(old):] if path.startswith(old + "/") else None

    def moved(self, value: Any) -> Any:
        return with_references(value, self.reference)

    def reference(self, reference: Any) -> Any:
        target = self.target(reference.get("path", "")) if isinstance(reference, dict) else None
        if target is None or (target == reference.get("path") and verify(reference, self.repo_root)["status"] == "current"):
            return reference
        lines = reference.get("lines") or {}
        length = lines.get("end", 0) - lines.get("start", 0) + 1
        excerpt = reference.get("excerpt_sha256")
        starts = excerpt_starts(self.repo_root, target, length, excerpt) if excerpt and length > 0 else []
        if len(starts) != 1:
            self.unmatched.append({
                "path": reference.get("path"), "lines": lines, "target": target, "found": len(starts),
                "reason": f"the cited excerpt occurs {len(starts)} times in {target}; "
                + ("its content changed, so check the fact and recite it" if not starts
                   else f"recite it with the lines that hold the fact. {WIDEN_TO_UNIQUE}"),
            })
            return reference
        recited = citation(self.repo_root, target, starts[0], starts[0] + length - 1)
        kept = {key: value for key, value in reference.items() if key != "source_kind"}
        return classified({**kept, **recited}, self.source_map)


def _record_update(name: str, document: dict[str, Any], entry: dict[str, Any], changed: dict[str, Any]) -> dict[str, Any]:
    record = {key: value for key, value in changed.items() if key != "review"}
    asset = name.removesuffix(".json")
    if record_status(asset, document, entry) == "reviewed":
        return {
            "registry_updates": [{"operation": "upsert", "asset": asset, "record": record}],
            "supersedes": [proposal for proposal in [(entry.get("review") or {}).get("proposal_id")] if proposal],
            "candidates": [],
        }
    return {"registry_updates": [], "supersedes": [], "candidates": [{"asset": asset, "record": record}]}


def line_range(text: str) -> tuple[int, int]:
    start, _, end = text.partition("-")
    if not start.isdigit() or not (end or start).isdigit():
        raise ValueError(f"lines must be START or START-END, got {text}")
    return int(start), int(end or start)


@dataclass(frozen=True)
class Recitation:
    asset: str
    record_id: str
    path: str
    lines: str
    replace: str | None = None


def recite(root: Path, repo_root: Path, request: Recitation) -> dict[str, Any]:
    asset, record_id, path, lines, old = (
        request.asset, request.record_id, request.path, request.lines, request.replace
    )
    name = f"{asset}.json"
    document = load_json(registry_dir(root) / name)
    entry = next((item for item in document.get(ASSET_KEYS[name], []) if item.get("id") == record_id), None)
    if entry is None:
        raise ValueError(f"no {asset} record {record_id}")
    old_path, _, old_lines = (old or path).partition(":")
    wanted = line_range(old_lines) if old_lines else None

    def replaced(reference: Any) -> bool:
        if not isinstance(reference, dict) or reference.get("path") != old_path:
            return False
        spans = reference.get("lines") or {}
        return wanted is None or (spans.get("start"), spans.get("end")) == wanted

    matches = [reference for reference in all_references(entry) if replaced(reference)]
    if len(matches) != 1:
        cited = sorted(
            f"{reference.get('path')}:{(reference.get('lines') or {}).get('start')}-{(reference.get('lines') or {}).get('end')}"
            for reference in all_references(entry) if isinstance(reference, dict)
        )
        raise ValueError(
            f"{asset}/{record_id} has {len(matches)} citations of {old or path}; name the one to replace "
            f"with --replace PATH:START-END. It cites: {', '.join(cited) or 'nothing'}"
        )
    start, end = line_range(lines)
    recited = classified(citation(repo_root, path, start, end), source_map_for(root))
    changed = with_references(entry, lambda reference: recited if reference is matches[0] else reference)
    return _record_update(name, document, entry, changed)


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
            update = _record_update(name, document, entry, moved)
            updates += update["registry_updates"]
            supersedes.update(update["supersedes"])
            candidates += update["candidates"]
    return {
        "from": old,
        "to": new,
        "registry_updates": updates,
        "supersedes": sorted(supersedes),
        "candidates": candidates,
        "unmatched": relocation.unmatched,
    }
