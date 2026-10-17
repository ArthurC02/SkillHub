from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path, PurePosixPath
from typing import Any

from .audit import reviewing_proposals
from .common import ASSET_KEYS, record_status, updates_record
from .evidence import (
    WIDEN_TO_UNIQUE,
    all_references,
    citation,
    classified,
    classifier,
    excerpt_starts,
    source_map_for,
    verify,
    with_references,
)
from .store import read_document


@dataclass
class Relocation:
    repo_root: Path
    old: str
    new: str
    classify: Callable[[Any], Any]
    unmatched: list[dict[str, Any]] = field(default_factory=list)
    in_place: int = 0

    def target(self, path: str) -> str | None:
        old, new = self.old.rstrip("/"), self.new.rstrip("/")
        if old in ("", ".") and new in ("", "."):
            return path
        if path == old:
            return new
        return new + path[len(old):] if path.startswith(old + "/") else None

    def placement(self, implementation_path: str) -> str | None:
        target = self.target(implementation_path)
        if target is not None:
            return target
        owned = PurePosixPath(implementation_path.strip("/")).parts
        old, new = PurePosixPath(self.old.rstrip("/")).parts, PurePosixPath(self.new.rstrip("/")).parts
        if not owned or old[-len(owned):] != owned:
            return None
        return "/".join(new[-len(owned):])

    def moved(self, value: Any) -> Any:
        return with_references(value, self.reference)

    def placed(self, name: str, record: dict[str, Any]) -> dict[str, Any]:
        path = record.get("implementation_path")
        target = self.placement(path) if name == "contexts.json" and isinstance(path, str) else None
        return record if target is None or target == path else {**record, "implementation_path": target}

    def reference(self, reference: Any) -> Any:
        target = self.target(reference.get("path", "")) if isinstance(reference, dict) else None
        if target is None:
            return reference
        status = verify(reference, self.repo_root)["status"]
        if status == "current" or (status == "shifted" and target != reference.get("path")):
            self.in_place += 1
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
        return self.classify({**kept, **recited})


def _record_update(
    name: str, document: dict[str, Any], entry: dict[str, Any], changed: dict[str, Any], reviewing: dict[str, str]
) -> dict[str, Any]:
    record = {key: value for key, value in changed.items() if key not in ("review", "verified")}
    asset = name.removesuffix(".json")
    if record_status(asset, document, entry) == "reviewed":
        return {
            "registry_updates": [{"operation": "upsert", "asset": asset, "record": record}],
            "supersedes": [proposal for proposal in [reviewing.get(f"{asset}/{entry.get('id')}")] if proposal],
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
    onto: dict[str, Any] | None = None


def _earlier_record(onto: dict[str, Any] | None, asset: str, record_id: str) -> dict[str, Any] | None:
    proposed = [*(onto or {}).get("registry_updates", []), *(onto or {}).get("candidates", [])]
    return next(
        (item["record"] for item in proposed
         if updates_record(item, asset, record_id)),
        None,
    )


def _merged(onto: dict[str, Any], update: dict[str, Any], asset: str, record_id: str, resolved: dict) -> dict[str, Any]:
    def others(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
        return [item for item in items
                if not updates_record(item, asset, record_id)]

    return {
        **onto,
        "registry_updates": others(onto.get("registry_updates", [])) + update["registry_updates"],
        "supersedes": sorted({*onto.get("supersedes", []), *update["supersedes"]}),
        "candidates": others(onto.get("candidates", [])) + update["candidates"],
        **({"unmatched": [
            problem for problem in onto["unmatched"]
            if (problem.get("asset"), problem.get("id"), problem.get("path"), problem.get("lines"))
            != (asset, record_id, resolved.get("path"), resolved.get("lines"))
        ]} if "unmatched" in onto else {}),
    }


def recite(root: Path, repo_root: Path, request: Recitation) -> dict[str, Any]:
    asset, record_id, path, lines, old = (
        request.asset, request.record_id, request.path, request.lines, request.replace
    )
    name = f"{asset}.json"
    document = read_document(root, name)
    entry = next((item for item in document.get(ASSET_KEYS[name], []) if item.get("id") == record_id), None)
    if entry is None:
        raise ValueError(f"no {asset} record {record_id}")
    working = _earlier_record(request.onto, asset, record_id) or entry
    old_path, _, old_lines = (old or path).partition(":")
    wanted = line_range(old_lines) if old_lines else None

    def replaced(reference: Any) -> bool:
        if not isinstance(reference, dict) or reference.get("path") != old_path:
            return False
        spans = reference.get("lines") or {}
        return wanted is None or (spans.get("start"), spans.get("end")) == wanted

    matches = [reference for reference in all_references(working) if replaced(reference)]
    if len(matches) != 1:
        cited = sorted(
            f"{reference.get('path')}:{(reference.get('lines') or {}).get('start')}-{(reference.get('lines') or {}).get('end')}"
            for reference in all_references(working) if isinstance(reference, dict)
        )
        raise ValueError(
            f"{asset}/{record_id} has {len(matches)} citations of {old or path}; name the one to replace "
            f"with --replace PATH:START-END. It cites: {', '.join(cited) or 'nothing'}"
        )
    start, end = line_range(lines)
    recited = classified(citation(repo_root, path, start, end), source_map_for(root))
    changed = with_references(working, lambda reference: recited if reference is matches[0] else reference)
    update = _record_update(name, document, entry, changed, reviewing_proposals(root))
    if request.onto is None:
        return update
    return _merged(request.onto, update, asset, record_id, matches[0])


def relocate_evidence(root: Path, repo_root: Path, old: str, new: str) -> dict[str, Any]:
    relocation = Relocation(repo_root, old, new, classifier(source_map_for(root)))
    reviewing = reviewing_proposals(root)
    updates, candidates, supersedes = [], [], set()
    for name in ASSET_KEYS:
        document = read_document(root, name)
        for entry in document.get(ASSET_KEYS[name], []):
            unmatched_before = len(relocation.unmatched)
            moved = relocation.placed(name, relocation.moved(entry))
            for problem in relocation.unmatched[unmatched_before:]:
                problem.update(asset=name.removesuffix(".json"), id=entry.get("id"))
            if moved == entry:
                continue
            update = _record_update(name, document, entry, moved, reviewing)
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
        "citations_in_place": relocation.in_place,
    }
