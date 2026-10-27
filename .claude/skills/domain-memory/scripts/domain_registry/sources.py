import fnmatch
import hashlib
import os
import subprocess
from functools import cache
from collections.abc import Callable, Iterable, Iterator
from pathlib import Path, PurePosixPath
from typing import Any

from .common import (
    inside,
    SOURCE_MAP_FORMAT,
    shown_path,
    completed_identifier,
    load_json,
    one_level_too_deep_hint,
    write_json,
    utc_now,
    writer_lock,
)
from .locate import domain_memory_hint
from .git import run_git, run_git_bytes
from .policy import excluded_directories, project_profile

SELECTED_KIND = "selected"
EXCLUDED_DIRECTORIES = frozenset({
    ".git",
    ".hg",
    ".svn",
    "node_modules",
    ".venv",
    "vendor",
    "dist",
    "build",
    "generated",
})

CI_MARKERS = {
    ".github/workflows": "github-actions",
    ".gitlab-ci.yml": "gitlab-ci",
    "Jenkinsfile": "jenkins",
    ".circleci/config.yml": "circleci",
    ".buildkite/pipeline.yml": "buildkite",
    "azure-pipelines.yml": "azure-pipelines",
}


def governance_candidates(ci_tools: list[str]) -> dict[str, object]:
    return {"ci_tools": ci_tools, "recommended_verifier": "git-signed-commit", "recommended_trigger": "git-push"}


def discover_ci_tools(root: Path) -> list[str]:
    return [name for _, name in discover_ci_markers(root)]


def discover_ci_markers(root: Path) -> list[tuple[str, str]]:
    return sorted(((path, name) for path, name in CI_MARKERS.items() if (root / path).exists()), key=lambda marker: marker[1])


def git_ignored(root: Path, candidates: list[Path]) -> set[Path]:
    by_relative = {relative(root, path): path for path in candidates}
    return {by_relative[entry] for entry in ignored_relatives(root, list(by_relative))}


TRACKED_PREFILTER_MIN_PATHS = 512
INDEX_TAGS_GIT_NEVER_IGNORES = ("H ", "M ")


def _untracked(root: Path, relatives: list[str]) -> list[str]:
    if len(relatives) < TRACKED_PREFILTER_MIN_PATHS:
        return relatives
    try:
        listed = run_git(root, "ls-files", "-z", "-t")
    except OSError:
        return relatives
    tracked = {entry[2:] for entry in listed.stdout.split(chr(0)) if entry.startswith(INDEX_TAGS_GIT_NEVER_IGNORES)}
    return [entry for entry in relatives if entry not in tracked]


def ignored_among(root: Path, files: list[str], relatives: list[str]) -> set[str]:
    if getattr(files, "pruned_by_git", False):
        return set()
    return ignored_relatives(root, relatives)


def ignored_relatives(root: Path, relatives: list[str]) -> set[str]:
    if not relatives or not (root / ".git").exists():
        return set()
    relatives = _untracked(root, relatives)
    result = run_git(root, "check-ignore", "-z", "--stdin", stdin=chr(0).join(relatives))
    if result.returncode not in (0, 1):
        raise ValueError(
            f"git check-ignore failed in {root}, so ignored files cannot be told from sources: {result.stderr.strip()}"
        )
    asked = set(relatives)
    return {entry for entry in result.stdout.split(chr(0)) if entry in asked}


def relative(root: Path, path: Path) -> str:
    prefix = str(root) + os.sep
    text = str(path)
    if text.startswith(prefix):
        return text[len(prefix) :].replace(os.sep, "/")
    try:
        return path.relative_to(root).as_posix()
    except ValueError:
        return path.resolve().relative_to(root.resolve()).as_posix()


def in_excluded_directory(relative_path: str, excluded: frozenset[str] = EXCLUDED_DIRECTORIES) -> bool:
    return any(part in excluded for part in PurePosixPath(relative_path).parts)


def existing_directories(root: Path, candidates: tuple[str, ...]) -> list[str]:
    return [candidate for candidate in candidates if (root / candidate).is_dir()]


class Files(list[str]):
    pruned_by_git = False


Listing = Callable[[], list[str]]


def listing_of(root: Path, excluded: frozenset[str] = EXCLUDED_DIRECTORIES) -> Listing:
    return cache(lambda: repository_files(root, excluded))


def repository_files(root: Path, excluded: frozenset[str] = EXCLUDED_DIRECTORIES) -> Files:
    ignored = git_ignored_entries(root)
    found = Files()
    found.pruned_by_git = ignored is not None
    _collect_files(str(root), "", excluded, ignored or frozenset(), found)
    return found


def git_ignored_entries(root: Path) -> frozenset[str] | None:
    try:
        listed = run_git_bytes(root, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
    except (OSError, subprocess.SubprocessError):
        return None
    if listed.returncode != 0:
        return None
    return frozenset(entry.decode("utf-8", "surrogateescape") for entry in listed.stdout.split(b"\0") if entry)


def _collect_files(directory: str, prefix: str, excluded: frozenset[str], ignored: frozenset[str], found: list[str]) -> None:
    try:
        with os.scandir(directory) as entries:
            listed = list(entries)
    except OSError:
        return
    if prefix and any(entry.name == ".git" for entry in listed):
        return
    children = []
    for entry in listed:
        name = prefix + entry.name
        if not entry.is_dir():
            if name not in ignored:
                found.append(name)
        elif entry.name not in excluded and not entry.is_symlink() and name + "/" not in ignored:
            children.append(entry)
    for entry in children:
        _collect_files(entry.path, prefix + entry.name + "/", excluded, ignored, found)


def file_name(relative_path: str) -> str:
    return relative_path.rpartition("/")[2]


INSTRUCTION_FILE_NAME = "AGENTS.md"


def _instruction_candidates(files: list[str]) -> list[str]:
    return sorted(entry for entry in files if file_name(entry) == INSTRUCTION_FILE_NAME)


def _boundary_candidates(files: list[str], profile: dict[str, list[str]]) -> list[str]:
    bases = tuple(f"{base}/" for base in profile["implementation_roots"])
    markers = set(profile["boundary_markers"])
    return sorted(entry for entry in files if entry.startswith(bases) and file_name(entry) in markers)


def test_candidates(files: list[str]) -> list[str]:
    return sorted(entry for entry in files if is_test_file(file_name(entry)))


def is_test_file(name: str) -> bool:
    return (
        name.endswith(("_test.go", "_test.py"))
        or ".test." in name
        or ".spec." in name
        or (name.startswith("test_") and name.endswith(".py"))
    )


def discover_sources(root: Path, profile: dict[str, list[str]] | None = None, listing: Listing | None = None) -> dict[str, Any]:
    root = root.resolve()
    profile = profile or project_profile(None)
    ci_tools = discover_ci_tools(root)
    files = (listing or listing_of(root, EXCLUDED_DIRECTORIES | frozenset(profile["excluded_directories"])))()
    candidates = (_instruction_candidates(files), _boundary_candidates(files, profile), test_candidates(files))
    ignored = ignored_among(root, files, [entry for group in candidates for entry in group])
    instructions, boundaries, tests = ([entry for entry in group if entry not in ignored] for group in candidates)
    groups = [
        {
            "kind": "repository_instructions",
            "authority": "instructions to coding agents, not a statement of the business domain",
            "caution": "These files exist to steer agents working in this repository. They are rewritten whenever the workflow changes, and a Domain Memory that rests on one loses its evidence when it is. Select them only for a domain fact no other source states, and record the gap.",
            "paths": instructions,
        },
        {
            "kind": "decisions",
            "authority": "current approved decisions when their status says so",
            "paths": existing_directories(root, tuple(profile["decision_roots"])),
        },
        {
            "kind": "requirements",
            "authority": "intended behavior and acceptance criteria",
            "paths": existing_directories(
                root,
                (
                    "docs/plans",
                    "docs/spec",
                    "docs/requirements",
                    "spec",
                    "requirements",
                ),
            ),
        },
        {
            "kind": "contracts",
            "authority": "inter-process promises and compatibility",
            "paths": existing_directories(root, ("contracts", "openapi", "asyncapi")),
        },
        {
            "kind": "bounded_contexts",
            "authority": "candidate technical boundaries requiring domain review",
            "paths": boundaries,
        },
        {
            "kind": "implementation",
            "authority": "current executable behavior",
            "paths": existing_directories(root, tuple(profile["implementation_roots"])),
        },
        {
            "kind": "tests",
            "authority": "evidence that a specific behavior is checked",
            "paths": tests,
        },
    ]
    return {
        "format": SOURCE_MAP_FORMAT,
        "repo_root": ".",
        "source_order": [group["kind"] for group in groups],
        "source_groups": groups,
        "missing_groups": [group["kind"] for group in groups if not group["paths"]],
        "selection_status": "discovered",
        "governance_candidates": governance_candidates(ci_tools),
        "note": "The map locates candidate sources. It does not establish a domain fact or source authority by itself.",
    }


def policy_matches(path: str, include: list[str], exclude: list[str]) -> bool:
    included = any(
        pattern == "**" or fnmatch.fnmatchcase(path, pattern) for pattern in include
    )
    excluded = any(fnmatch.fnmatchcase(path, pattern) for pattern in exclude)
    return included and not excluded


def _directory_entries(directory: str) -> list[os.DirEntry[str]]:
    try:
        with os.scandir(directory) as entries:
            return list(entries)
    except OSError:
        return []


def _regular_files_under(directory: Path, excluded: frozenset[str]) -> Iterator[Path]:
    pending = [str(directory)]
    while pending:
        for entry in _directory_entries(pending.pop()):
            if entry.is_dir(follow_symlinks=False):
                if entry.name not in excluded:
                    pending.append(entry.path)
            elif entry.is_file(follow_symlinks=False):
                yield Path(entry.path)


def _regular_files(selected: Path, excluded: frozenset[str]) -> Iterable[Path]:
    if selected.is_file():
        return [] if selected.is_symlink() else [selected]
    return _regular_files_under(selected, excluded)


def source_files(
    root: Path, selected_paths: list[Path], policy: dict[str, Any] | None = None
) -> list[Path]:
    source_policy = policy.get("source_policy", {}) if policy else {}
    include = source_policy.get("include", ["**"])
    exclude = source_policy.get("exclude", [])
    excluded = excluded_directories(policy)
    files: set[Path] = set()
    roots = []
    for selected in selected_paths:
        resolved = selected.resolve()
        if any(
            resolved == parent or parent in resolved.parents
            for parent in roots
            if parent.is_dir()
        ):
            continue
        roots.append(resolved)
    for resolved in roots:
        for candidate in _regular_files(resolved, excluded):
            relative_path = relative(root, candidate)
            if not in_excluded_directory(relative_path, excluded) and policy_matches(
                relative_path, include, exclude
            ):
                files.add(candidate)
    return sorted(files - git_ignored(root, list(files)))


def source_policy_report(
    root: Path, selected_paths: list[Path], policy: dict[str, Any]
) -> dict[str, Any]:
    return _policy_report_of(root, source_files(root, selected_paths, policy), policy)


def _policy_report_of(root: Path, files: list[Path], policy: dict[str, Any]) -> dict[str, Any]:
    limits = policy["limits"]
    sizes = {path: path.stat().st_size for path in files}
    total_bytes = sum(sizes.values())
    oversized = [
        relative(root, path)
        for path, size in sizes.items()
        if size > limits["max_file_bytes"]
    ]
    errors = []
    if len(files) > limits["max_file_count"]:
        errors.append(
            f"selected sources hold {len(files)} files, over max_file_count {limits['max_file_count']}"
        )
    if total_bytes > limits["max_total_bytes"]:
        errors.append(
            f"selected sources hold {total_bytes} bytes, over max_total_bytes {limits['max_total_bytes']}"
        )
    if oversized:
        named = ", ".join(oversized[:5])
        remainder = "" if len(oversized) <= 5 else f", and {len(oversized) - 5} more"  # noqa: PLR2004
        errors.append(
            f"over max_file_bytes {limits['max_file_bytes']}: {named}{remainder}"
        )
    return {
        "files": len(files),
        "total_bytes": total_bytes,
        "oversized_files": oversized,
        "errors": errors,
    }


def owned_source_files(
    root: Path, selected: list[str], policy: dict[str, Any] | None = None
) -> dict[str, list[Path]]:
    return _owned_by_selection(root, selected, source_files(root, [root / path for path in selected], policy))


def _owned_by_selection(root: Path, selected: list[str], files: list[Path]) -> dict[str, list[Path]]:
    resolved = [
        (path, (root / path).resolve(), (root / path).is_dir()) for path in selected
    ]
    ancestors_of: dict[Path, set[Path]] = {}
    owner = {}
    for file in files:
        directory = file.parent
        if directory not in ancestors_of:
            ancestors_of[directory] = {directory, *directory.parents}
        parents = ancestors_of[directory]
        for path, selected_path, is_directory in resolved:
            if (
                file == selected_path
                or (is_directory and selected_path in parents)
            ) and len(path) > len(owner.get(file, "")):
                owner[file] = path
    return {
        path: [file for file in files if owner[file] == path] for path in selected
    }


def source_snapshot(root: Path, path: str, files: list[Path]) -> dict[str, Any]:
    digest = hashlib.sha256()
    listing = hashlib.sha256()
    for file in files:
        name = relative(root, file).encode("utf-8")
        listing.update(name)
        listing.update(b"\0")
        digest.update(name)
        digest.update(b"\0")
        digest.update(file.read_bytes())
        digest.update(b"\0")
    return {
        "path": path,
        "file_count": len(files),
        "digest": f"sha256:{digest.hexdigest()}",
        "listing_sha256": f"sha256:{listing.hexdigest()}",
    }


def selection_moved(recorded: dict[str, Any], current: dict[str, Any]) -> bool:
    listing = recorded.get("listing_sha256")
    return recorded.get("file_count") != current.get("file_count") or (
        isinstance(listing, str) and listing != current.get("listing_sha256")
    )


def source_snapshots(
    root: Path, selected: list[str], policy: dict[str, Any] | None = None
) -> list[dict[str, Any]]:
    return _snapshots_of(root, selected, owned_source_files(root, selected, policy))


def _snapshots_of(root: Path, selected: list[str], owned: dict[str, list[Path]]) -> list[dict[str, Any]]:
    return [source_snapshot(root, path, owned[path]) for path in selected]


def source_kinds(source_map: dict[str, Any]) -> dict[str, str]:
    kind_of = {
        path: group.get("kind", "unclassified")
        for group in source_map.get("source_groups", [])
        for path in group.get("paths", [])
    }
    return {
        path: kind_of.get(path, "unclassified")
        for path in source_map.get("selected_paths", [])
    }


def source_kind_index(source_map: dict[str, Any]) -> Callable[[str], str]:
    kinds = source_map.get("source_kinds") or source_kinds(source_map)
    group_kinds: dict[str, Any] = {}
    for group in source_map.get("source_groups", []):
        for path in group.get("paths", []):
            group_kinds.setdefault(path, group.get("kind", "unclassified"))

    def kind_of(cited_path: str) -> str:
        owners = _containing_paths(cited_path)
        owner = next((path for path in owners if path in kinds), None)
        if owner is not None and kinds[owner] != "unclassified":
            return kinds[owner]
        kind = next((group_kinds[path] for path in owners if path in group_kinds), "unclassified")
        if kind == "unclassified" and owner is not None:
            return SELECTED_KIND
        return kind if isinstance(kind, str) else "unclassified"

    return kind_of


def _containing_paths(path: str) -> list[str]:
    parts = path.split("/")
    return [owner for size in range(len(parts), 0, -1) if (owner := "/".join(parts[:size]))]


def selected_source_map(
    root: Path, selected_paths: list[Path], policy: dict[str, Any] | None = None,
    profile: dict[str, list[str]] | None = None,
) -> dict[str, Any]:
    root = root.resolve()
    selected = []
    for path in selected_paths:
        resolved = path.resolve()
        try:
            selected.append(relative(root, resolved))
        except ValueError as error:
            raise ValueError(f"source path is outside repository: {path}") from error
        if not resolved.exists():
            raise ValueError(f"source path does not exist: {path}")
    if not selected:
        raise ValueError("at least one source path is required")
    source_map = discover_sources(root, profile or project_profile(policy))
    source_map["selection_status"] = "agent-asserted"
    selected = source_map["selected_paths"] = sorted(set(selected))
    files = source_files(root, [root / path for path in selected], policy)
    if policy:
        report = _policy_report_of(root, files, policy)
        if report["errors"]:
            raise ValueError(
                "source policy limits failed: " + "; ".join(report["errors"])
            )
        source_map["source_policy_report"] = report
    source_map["source_kinds"] = source_kinds(source_map)
    source_map["git_state"] = git_state(root, selected)
    source_map["source_snapshots"] = _snapshots_of(root, selected, _owned_by_selection(root, selected, files))
    return source_map


def confirm_sources(
    root: Path, repo_root: Path, confirmed_by: str, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    path = root / "source-map.json"
    if not path.is_file():
        raise ValueError(_no_domain_memory(root, repo_root))
    if not completed_identifier(confirmed_by):
        raise ValueError(
            "confirming sources records who chose them, so it needs the developer's own identity as they gave it. "
            "An agent cannot confirm on their behalf, and an identity read out of a git config, a commit or a file "
            "in the repository is not the developer answering."
        )
    freshness = verify_source_map(repo_root, path, policy)
    if freshness["status"] == "stale":
        raise ValueError(
            "the selected sources have moved since this map was written, so confirming it would attest to a corpus "
            "that is no longer there; refresh the map first, then confirm what the developer actually looked at: "
            + ", ".join(
                source["path"] for source in freshness.get("changed_sources", [])
            )
        )
    if freshness["status"] != "current":
        raise ValueError(
            "the source map does not verify, so there is nothing to confirm yet: "
            + str(freshness.get("reason", freshness["status"]))
        )
    source_map = load_json(path)
    source_map["selection_status"] = "developer-confirmed"
    source_map["confirmed_by"] = confirmed_by
    source_map["confirmed_at"] = utc_now()
    with writer_lock(root):
        write_and_audit(
            root,
            source_map,
            {
                "operation": "confirm-sources",
                "confirmed_by": confirmed_by,
                "selected_paths": source_map.get("selected_paths", []),
            },
        )
    return source_map


def _no_domain_memory(root: Path, repo_root: Path) -> str:
    return f"no Domain Memory at {shown_path(root, repo_root)}: {domain_memory_hint(root, repo_root)}"


def _selected_paths(root: Path, repo_root: Path) -> list[str]:
    path = root / "source-map.json"
    if not path.is_file():
        raise ValueError(_no_domain_memory(root, repo_root))
    selected = load_json(path).get("selected_paths")
    if not isinstance(selected, list) or not all(
        isinstance(item, str) and item.strip() for item in selected
    ):
        raise ValueError("source map has no valid selected paths to refresh")
    return selected


def adjusted_selection(
    root: Path, repo_root: Path, add: list[Path], remove: list[Path]
) -> list[Path]:
    selected = _selected_paths(root, repo_root)
    repo = repo_root.resolve()
    removing = {relative(repo, path.resolve()) for path in remove}
    unselected = sorted(removing - set(selected))
    if unselected:
        raise ValueError(
            "cannot remove a source that is not selected: " + ", ".join(unselected)
        )
    return [repo / item for item in selected if item not in removing] + add


def refresh_sources(
    root: Path, repo_root: Path, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    selected = _selected_paths(root, repo_root)
    source_map = selected_source_map(
        repo_root, [repo_root / item for item in selected], policy
    )
    with writer_lock(root):
        write_and_audit(
            root,
            source_map,
            {
                "operation": "refresh-sources",
                "selected_paths": source_map["selected_paths"],
            },
        )
    return source_map


def _listed_paths(source_map: dict[str, Any]) -> set[str]:
    return {
        path
        for group in source_map.get("source_groups", [])
        for path in group.get("paths", [])
    }


def _selection_trouble(verdict: dict[str, Any]) -> str:
    changed = ", ".join(source["path"] for source in verdict.get("changed_sources", []))
    return str(verdict.get("reason") or changed or verdict["status"])


def refresh_listing(
    root: Path, repo_root: Path, policy: dict[str, Any] | None = None
) -> dict[str, list[str]]:
    path = root / "source-map.json"
    if not path.is_file():
        raise ValueError(_no_domain_memory(root, repo_root))
    verdict = verify_source_map(repo_root, path, policy)
    if verdict["status"] != "current":
        raise ValueError(
            f"the selection is {verdict['status']}: {_selection_trouble(verdict)}; "
            "run refresh-sources, then confirm-sources"
        )
    source_map = load_json(path)
    discovered = discover_sources(repo_root, project_profile(policy))
    listed = _listed_paths(source_map)
    current = _listed_paths(discovered)
    change = {"added": sorted(current - listed), "removed": sorted(listed - current)}
    if not change["added"] and not change["removed"]:
        return change
    source_map["source_groups"] = discovered["source_groups"]
    source_map["missing_groups"] = discovered["missing_groups"]
    with writer_lock(root):
        write_and_audit(root, source_map, {"operation": "refresh-listing", **change})
    return change


def refine_sources(
    root: Path, repo_root: Path, selected_paths: list[Path], policy: dict[str, Any]
) -> dict[str, Any]:
    if not selected_paths:
        raise ValueError("refining sources requires at least one focused path")
    source_map = selected_source_map(repo_root, selected_paths, policy)
    policy["source_policy"]["selected_paths"] = source_map["selected_paths"]
    with writer_lock(root):
        write_json(root / "domain-memory-policy.json", policy)
        write_and_audit(
            root,
            source_map,
            {
                "operation": "refine-sources",
                "selected_paths": source_map["selected_paths"],
            },
        )
    return source_map


def source_verdict(status: str, selection_status: str, reason: str) -> dict[str, str]:
    return {"status": status, "selection_status": selection_status, "reason": reason}


def _snapshot_problem(snapshots: Any, selected_paths: Any) -> str | None:
    if not isinstance(snapshots, list):
        return "source map has no snapshots"
    if not all(
        isinstance(snapshot, dict) and isinstance(snapshot.get("path"), str)
        for snapshot in snapshots
    ):
        return "source map has an invalid snapshot"
    if (
        not isinstance(selected_paths, list)
        or not all(isinstance(path, str) and path.strip() for path in selected_paths)
        or len(selected_paths) != len(set(selected_paths))
        or set(selected_paths) != {snapshot["path"] for snapshot in snapshots}
    ):
        return "source map snapshots do not match selected_paths"
    return None


def _selection_problem(
    root: Path, selected_paths: list[str], policy: dict[str, Any] | None
) -> str | None:
    escaping = next((path for path in selected_paths if inside(root, path) is None), None)
    if escaping is not None:
        return f"source path escapes repository: {escaping}"
    policy_paths = (policy or {}).get("source_policy", {}).get("selected_paths")
    if isinstance(policy_paths, list) and sorted(policy_paths) != sorted(selected_paths):
        return "policy selected_paths do not match the source map"
    return None


def _compare_snapshots(
    root: Path, snapshots: list[dict[str, Any]], policy: dict[str, Any] | None
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    actual = {
        snapshot["path"]: snapshot
        for snapshot in source_snapshots(root, [s["path"] for s in snapshots], policy)
    }
    changed = []
    drifted = []
    for snapshot in snapshots:
        current = actual[snapshot["path"]]
        if not (root / snapshot["path"]).exists():
            changed.append({"path": snapshot["path"], "status": "missing"})
        elif selection_moved(snapshot, current):
            changed.append(
                {"path": snapshot["path"], "status": "changed", "expected": snapshot, "actual": current}
            )
        elif snapshot.get("digest") != current.get("digest"):
            drifted.append({"path": snapshot["path"], "status": "content-changed"})
    return changed, drifted


def verify_source_map(
    root: Path, source_map_path: Path, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    root = root.resolve()
    source_map = load_json(source_map_path)
    snapshots = source_map.get("source_snapshots")
    selected_paths = source_map.get("selected_paths")
    selection_status = source_map.get("selection_status", "agent-asserted")
    unverified = _snapshot_problem(snapshots, selected_paths)
    if unverified:
        return source_verdict("unverified", selection_status, unverified)
    invalid = _selection_problem(root, selected_paths, policy)
    if invalid:
        return source_verdict("invalid", selection_status, invalid)
    changed, drifted = _compare_snapshots(root, snapshots, policy)
    if policy:
        report = source_policy_report(
            root, [root / path for path in selected_paths], policy
        )
        if report["errors"]:
            return source_verdict(
                "invalid", selection_status, "; ".join(report["errors"])
            ) | {"source_policy_report": report}
    return {
        "status": "stale" if changed else "current",
        "selection_status": selection_status,
        "changed_sources": changed,
        "content_changed": drifted,
        "discovered_but_gone": _discovered_but_gone(root, source_map),
    }


def _discovered_but_gone(root: Path, source_map: dict[str, Any]) -> list[str]:
    return sorted(
        path
        for group in source_map.get("source_groups", [])
        for path in group.get("paths", [])
        if not (root / path).exists()
    )


def listing_drift(discovered: dict[str, Any], source_map: dict[str, Any]) -> tuple[list[str], list[str]]:
    listed, current = _listed_paths(source_map), _listed_paths(discovered)
    return sorted(listed - current), sorted(current - listed)


def verify_against_discovery(
    root: Path, source_map_path: Path, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    result = verify_source_map(root, source_map_path, policy)
    if "discovered_but_gone" not in result:
        return result
    gone, unlisted = listing_drift(discover_sources(root.resolve(), project_profile(policy)), load_json(source_map_path))
    status = "discovery_drifted" if (gone or unlisted) and result["status"] == "current" else result["status"]
    return result | {"status": status, "discovered_but_gone": gone, "discovered_but_unlisted": unlisted}


def shown_argument(path: Path) -> str:
    text = path.as_posix()
    return f'"{text}"' if " " in text else text


def discovery_placements(repo_root: Path, paths: list[str], policy: dict[str, Any] | None = None) -> list[tuple[str, str]]:
    kind_of = source_kind_index(discover_sources(repo_root, project_profile(policy)))
    placed = [(path, kind_of(path)) for path in paths]
    return [(path, kind) for path, kind in placed if kind != "unclassified"]


def refresh_listing_advice(registry_root: Path, repo_root: Path) -> str:
    return (
        f"run refresh-listing --registry-root {shown_argument(registry_root)} "
        f"--repo-root {shown_argument(repo_root)} (no confirmation needed)"
    )


def counted(count: int, noun: str) -> str:
    return f"{count} {noun}{'' if count == 1 else 's'}"


def discovery_drift_note(result: dict[str, Any], registry_root: Path, repo_root: Path) -> str | None:
    gone = len(result.get("discovered_but_gone", []))
    unlisted = len(result.get("discovered_but_unlisted", []))
    if not gone and not unlisted:
        return None
    advice = (
        refresh_listing_advice(registry_root, repo_root)
        if result.get("status") == "discovery_drifted"
        else f"the selection is {result.get('status')}, so run refresh-sources, then confirm-sources"
    )
    changed_inside = len(result.get("content_changed", []))
    inside = (
        f"{counted(changed_inside, 'selected source')} changed inside; verify-evidence checks the citations; "
        if changed_inside
        else ""
    )
    return (
        f"NOTE: discovery drifted: {counted(gone, 'listed path')} gone, "
        f"{counted(unlisted, 'matching path')} unlisted; {inside}{advice}"
    )


def refresh_confirmation_notice(selected_paths: list[str]) -> str:
    count = len(selected_paths)
    demonstrative = "this" if count == 1 else "these"
    return (
        f"Selection reset to agent-asserted; confirm {demonstrative} {counted(count, 'selected source')}: "
        + ", ".join(selected_paths)
    )


def write_source_map(path: Path, source_map: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    write_json(path, source_map)


def write_and_audit(
    root: Path, source_map: dict[str, Any], event: dict[str, Any]
) -> None:
    from .audit import append_locked

    write_source_map(root / "source-map.json", source_map)
    append_locked(root, event)


def lies_under(path: str, implementation_path: str) -> bool:
    return owned_directory(implementation_path) in directory_of(path)


def directory_of(path: str) -> str:
    return "/" + path.rpartition("/")[0] + "/"


def owned_directory(implementation_path: str) -> str:
    parts = PurePosixPath(implementation_path.strip("/")).parts
    return "/" + "/".join(parts) + "/" if parts else "/"


def git_state(root: Path, selected_paths: list[str]) -> dict[str, Any]:
    if not selected_paths or not (root / ".git").exists():
        return {}
    try:
        trees = run_git(root, "rev-parse", *(f"HEAD:{path}" for path in selected_paths))
        dirty = run_git(root, "status", "--porcelain", "-z", "--", *selected_paths)
    except OSError:
        return {}
    if trees.returncode != 0 or dirty.returncode != 0:
        return {}
    objects = trees.stdout.split()
    if len(objects) != len(selected_paths):
        return {}
    return {
        "tracked_objects": dict(zip(selected_paths, objects, strict=True)),
        "dirty_sources": sorted(entry for entry in dirty.stdout.split(chr(0)) if entry),
    }


def _policy_verdict(
    root: Path, source_map: dict[str, Any], policy: dict[str, Any], selection_status: str
) -> dict[str, Any] | None:
    from .policy import validate_policy

    errors = validate_policy(policy)
    if errors:
        return source_verdict("invalid", selection_status, "; ".join(errors))
    policy_paths = policy["source_policy"]["selected_paths"]
    if sorted(policy_paths) != sorted(source_map.get("selected_paths", [])):
        return source_verdict(
            "invalid", selection_status, "policy selected_paths do not match the source map"
        )
    try:
        report = source_policy_report(root, [root / path for path in policy_paths], policy)
    except (OSError, ValueError) as error:
        return source_verdict("invalid", selection_status, str(error))
    if report["errors"]:
        return source_verdict("invalid", selection_status, "; ".join(report["errors"])) | {
            "source_policy_report": report
        }
    return None


def _git_shows_no_change(root: Path, source_map: dict[str, Any]) -> bool:
    recorded = source_map.get("git_state")
    if not isinstance(recorded, dict) or not recorded.get("tracked_objects") or recorded.get("dirty_sources"):
        return False
    now = git_state(root, source_map.get("selected_paths", []))
    return now.get("tracked_objects") == recorded["tracked_objects"] and not now.get("dirty_sources")


def probe_sources(
    root: Path, source_map_path: Path, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    if not source_map_path.exists():
        absent = {"status": "absent", "reason": f"no source map at {source_map_path}"}
        hint = one_level_too_deep_hint(source_map_path)
        return {**absent, "hint": hint.removeprefix("; ")} if hint else absent
    source_map = load_json(source_map_path)
    selection_status = source_map.get("selection_status", "agent-asserted")
    if policy is not None:
        invalid = _policy_verdict(root, source_map, policy, selection_status)
        if invalid:
            return invalid
    if _git_shows_no_change(root, source_map):
        result = {"status": "current", "selection_status": selection_status, "changed_sources": [], "checked": "git"}
    else:
        result = verify_source_map(root, source_map_path, policy) | {"checked": "hash"}
    if result["status"] != "current":
        return result
    gone, unlisted = listing_drift(discover_sources(root.resolve(), project_profile(policy)), source_map)
    status = "discovery_drifted" if gone or unlisted else "current"
    return result | {"status": status, "discovered_but_gone": gone, "discovered_but_unlisted": unlisted}
