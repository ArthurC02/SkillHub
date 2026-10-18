import re
import subprocess
import sys
from collections.abc import Callable
from itertools import combinations
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS
from .evidence import all_references, classifier, source_map_for, verifier
from .git import run_git
from .sources import lies_under
from .store import read_document

COLLABORATION_ENDS = {
    "interactions.json": lambda entry: [(entry.get("producer_context"), entry.get("consumer_context"))],
    "dependency-policies.json": lambda entry: [(entry.get("from_context"), entry.get("to_context"))],
    "contracts.json": lambda entry: [
        (entry.get("producer_context"), consumer) for consumer in entry.get("consumer_contexts") or []
    ],
    "events.json": lambda entry: [(entry.get("owner_context"), consumer) for consumer in entry.get("consumers") or []],
}


HUNK = re.compile(r"^@@ -\S+ \+(\d+)(?:,(\d+))? @@")
WHOLE_FILE = [(1, sys.maxsize)]


def _changes(repo_root: Path, base: str) -> tuple[set[str], set[str]]:
    try:
        changed = run_git(repo_root, "diff", "--name-only", "-z", base, "--")
        untracked = run_git(repo_root, "ls-files", "--others", "--exclude-standard", "-z")
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ValueError(f"git could not list the changes since {base}: {error}") from error
    for result in (changed, untracked):
        if result.returncode != 0:
            raise ValueError(f"git could not list the changes since {base}: {result.stderr.strip()}")
    return ({path for path in changed.stdout.split("\0") if path}, {path for path in untracked.stdout.split("\0") if path})


def _hunks(repo_root: Path, base: str, paths: list[str]) -> dict[str, list[tuple[int, int]]]:
    if not paths:
        return {}
    try:
        result = run_git(repo_root, "--literal-pathspecs", "-c", "core.quotepath=false", "diff", "-U0", "--no-color",
                         "--no-ext-diff", base, "--", *paths)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ValueError(f"git could not read the changes since {base}: {error}") from error
    if result.returncode != 0:
        raise ValueError(f"git could not read the changes since {base}: {result.stderr.strip()}")
    hunks: dict[str, list[tuple[int, int]]] = {}
    current = None
    for line in result.stdout.splitlines():
        if line.startswith("+++ "):
            current = line.removeprefix("+++ b/") if line.startswith("+++ b/") else None
        elif current is not None and (match := HUNK.match(line)):
            start, count = int(match[1]), int(match[2] or 1)
            hunks.setdefault(current, []).append((start, start + count - 1))
    return hunks


def _cited_ranges(entry: dict[str, Any]) -> list[tuple[str, int, int]]:
    return [
        (reference["path"], lines["start"], lines["end"])
        for reference in all_references(entry)
        if isinstance(reference, dict) and isinstance(reference.get("path"), str)
        and isinstance(lines := reference.get("lines"), dict)
        and isinstance(lines.get("start"), int) and isinstance(lines.get("end"), int)
    ]


def touched_records(root: Path, repo_root: Path, base: str) -> list[dict[str, Any]]:
    tracked, untracked = _changes(repo_root, base)
    records = {name: _records_with_status(root, name) for name in ASSET_KEYS}
    cited = {path for entries in records.values() for entry in entries for path, _, _ in _cited_ranges(entry)}
    hunks = _hunks(repo_root, base, sorted(cited & tracked)) | dict.fromkeys(cited & untracked, WHOLE_FILE)
    return _evidence_overlapping(records, hunks, ASSET_KEYS)


def _evidence_overlapping(
    records: dict[str, list[dict[str, Any]]], hunks: dict[str, list[tuple[int, int]]], names: Any = COLLABORATION_ENDS
) -> list[dict[str, Any]]:
    touched = []
    for name in names:
        for entry in records[name]:
            overlapping = sorted({
                f"{path}:{start}-{end}"
                for path, start, end in _cited_ranges(entry)
                if any(start <= last and first <= end for first, last in hunks.get(path, []))
            })
            if overlapping:
                touched.append({
                    "asset": name.removesuffix(".json"),
                    "id": entry.get("id"),
                    "status": entry.get("status"),
                    "touched_evidence": overlapping,
                })
    return touched


def _records_with_status(root: Path, name: str) -> list[dict[str, Any]]:
    document = read_document(root, name)
    default = document.get("status")
    return [{**entry, "status": entry.get("status", default)} for entry in document.get(ASSET_KEYS[name], [])]


def _touched_contexts(contexts: list[dict[str, Any]], paths: list[str]) -> dict[str, list[str]]:
    touched = {
        context.get("id"): [path for path in paths if lies_under(path, context["implementation_path"])]
        for context in contexts
        if isinstance(context.get("implementation_path"), str)
    }
    return {identifier: files for identifier, files in touched.items() if files}


def registered_collaborations(root: Path) -> dict[frozenset, list[str]]:
    records = {name: _records_with_status(root, name) for name in COLLABORATION_ENDS}
    return {
        pair: [f"{entry['asset']}/{entry['id']}" for entry in entries]
        for pair, entries in _collaborations(records).items()
    }


def _collaborations(records: dict[str, list[dict[str, Any]]]) -> dict[frozenset, list[dict[str, Any]]]:
    found: dict[frozenset, list[dict[str, Any]]] = {}
    for name, ends in COLLABORATION_ENDS.items():
        for entry in records[name]:
            for pair in ends(entry):
                found.setdefault(frozenset(pair), []).append(
                    {"asset": name.removesuffix(".json"), "id": entry.get("id"), "status": entry.get("status")}
                )
    return found


UNCHANGED_EXCERPT = ("current", "shifted")


def _evidence_touched(
    records: dict[str, list[dict[str, Any]]], paths: set[str], check: Callable[[Any], dict[str, Any]]
) -> list[dict[str, Any]]:
    touched = []
    for name in COLLABORATION_ENDS:
        for entry in records[name]:
            changed = {
                cited
                for reference in all_references(entry)
                if (cited := reference.get("path") if isinstance(reference, dict) else reference) in paths
                and not (isinstance(reference, dict) and check(reference)["status"] in UNCHANGED_EXCERPT)
            }
            if changed:
                touched.append({
                    "asset": name.removesuffix(".json"),
                    "id": entry.get("id"),
                    "status": entry.get("status"),
                    "changed_evidence": sorted(changed),
                })
    return touched


def _verdict(pairs: list[dict[str, Any]], evidence: list[dict[str, Any]], *, mapped: bool) -> str:
    if any(not pair["collaborations"] for pair in pairs):
        return "unregistered-co-change"
    if evidence:
        return "boundary-evidence-changed"
    if pairs:
        return "registered-co-change"
    return "no-cross-context-change" if mapped else "unknown"


def boundary_impact(root: Path, repo_root: Path, base: str) -> dict[str, Any]:
    tracked, untracked = _changes(repo_root, base)
    paths = sorted(tracked | untracked)
    contexts = _records_with_status(root, "contexts.json")
    mapped = any(isinstance(context.get("implementation_path"), str) for context in contexts)
    touched = _touched_contexts(contexts, paths)
    records = {name: _records_with_status(root, name) for name in COLLABORATION_ENDS}
    collaborations = _collaborations(records)
    pairs = [
        {"contexts": [one, other], "collaborations": collaborations.get(frozenset((one, other)), [])}
        for one, other in combinations(sorted(touched), 2)
    ]
    evidence = _evidence_touched(records, set(paths), verifier(repo_root))
    cited = {path for entries in records.values() for entry in entries for path, _, _ in _cited_ranges(entry)}
    hunks = _hunks(repo_root, base, sorted(cited & tracked)) | dict.fromkeys(cited & untracked, WHOLE_FILE)
    owned = {path for files in touched.values() for path in files}
    outside = [path for path in paths if path not in owned]
    return {
        "base": base,
        "changed_files": len(paths),
        "mapped_files": len(owned),
        "verdict": _verdict(pairs, evidence, mapped=mapped),
        "contexts_touched": touched,
        "co_changed_pairs": pairs,
        "boundary_evidence_changed": evidence,
        "boundary_evidence_touched": _evidence_overlapping(records, hunks),
        "outside_every_context": outside,
        "cited_outside_contexts": _cited(root, set(outside)),
    }


def _cited(root: Path, paths: set[str]) -> list[dict[str, Any]]:
    citing: dict[str, set[str]] = {}
    for name in ASSET_KEYS:
        for entry in _records_with_status(root, name):
            for reference in all_references(entry):
                cited = reference.get("path") if isinstance(reference, dict) else reference
                if cited in paths:
                    citing.setdefault(cited, set()).add(f"{name.removesuffix('.json')}/{entry.get('id')}")
    if not citing:
        return []
    kind_of = classifier(source_map_for(root))
    return [
        {"path": path, "cited_by": sorted(records), "cited_as": kind_of({"path": path}).get("source_kind")}
        for path, records in sorted(citing.items())
    ]
