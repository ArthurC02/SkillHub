import subprocess
from itertools import combinations
from pathlib import Path, PurePosixPath
from typing import Any

from .common import ASSET_KEYS, load_json, registry_dir
from .evidence import all_references
from .sources import run_git

COLLABORATION_ENDS = {
    "interactions.json": lambda entry: [(entry.get("producer_context"), entry.get("consumer_context"))],
    "dependency-policies.json": lambda entry: [(entry.get("from_context"), entry.get("to_context"))],
    "contracts.json": lambda entry: [
        (entry.get("producer_context"), consumer) for consumer in entry.get("consumer_contexts") or []
    ],
}


def changed_paths(repo_root: Path, base: str) -> list[str]:
    try:
        changed = run_git(repo_root, "diff", "--name-only", "-z", base, "--")
        untracked = run_git(repo_root, "ls-files", "--others", "--exclude-standard", "-z")
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ValueError(f"git could not list the changes since {base}: {error}") from error
    for result in (changed, untracked):
        if result.returncode != 0:
            raise ValueError(f"git could not list the changes since {base}: {result.stderr.strip()}")
    return sorted({path for path in (changed.stdout + untracked.stdout).split("\0") if path})


def _records_with_status(root: Path, name: str) -> list[dict[str, Any]]:
    document = load_json(registry_dir(root) / name)
    default = document.get("status")
    return [{**entry, "status": entry.get("status", default)} for entry in document.get(ASSET_KEYS[name], [])]


def _lies_under(path: str, implementation_path: str) -> bool:
    directories = PurePosixPath(path).parts[:-1]
    owned = PurePosixPath(implementation_path.strip("/")).parts
    return any(
        directories[start : start + len(owned)] == owned
        for start in range(len(directories) - len(owned) + 1)
    )


def _touched_contexts(contexts: list[dict[str, Any]], paths: list[str]) -> dict[str, list[str]]:
    touched = {
        context.get("id"): [path for path in paths if _lies_under(path, context["implementation_path"])]
        for context in contexts
        if isinstance(context.get("implementation_path"), str)
    }
    return {identifier: files for identifier, files in touched.items() if files}


def _collaborations(root: Path) -> dict[frozenset, list[dict[str, Any]]]:
    found: dict[frozenset, list[dict[str, Any]]] = {}
    for name, ends in COLLABORATION_ENDS.items():
        for entry in _records_with_status(root, name):
            for pair in ends(entry):
                found.setdefault(frozenset(pair), []).append(
                    {"asset": name.removesuffix(".json"), "id": entry.get("id"), "status": entry.get("status")}
                )
    return found


def _evidence_touched(root: Path, paths: set[str]) -> list[dict[str, Any]]:
    touched = []
    for name in COLLABORATION_ENDS:
        for entry in _records_with_status(root, name):
            cited = {
                reference.get("path") if isinstance(reference, dict) else reference
                for reference in all_references(entry)
            }
            if cited & paths:
                touched.append({
                    "asset": name.removesuffix(".json"),
                    "id": entry.get("id"),
                    "status": entry.get("status"),
                    "changed_evidence": sorted(cited & paths),
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
    paths = changed_paths(repo_root, base)
    contexts = _records_with_status(root, "contexts.json")
    mapped = any(isinstance(context.get("implementation_path"), str) for context in contexts)
    touched = _touched_contexts(contexts, paths)
    collaborations = _collaborations(root)
    pairs = [
        {"contexts": [one, other], "collaborations": collaborations.get(frozenset((one, other)), [])}
        for one, other in combinations(sorted(touched), 2)
    ]
    evidence = _evidence_touched(root, set(paths))
    owned = {path for files in touched.values() for path in files}
    return {
        "base": base,
        "changed_files": len(paths),
        "verdict": _verdict(pairs, evidence, mapped=mapped),
        "contexts_touched": touched,
        "co_changed_pairs": pairs,
        "boundary_evidence_changed": evidence,
        "outside_every_context": [path for path in paths if path not in owned],
    }
