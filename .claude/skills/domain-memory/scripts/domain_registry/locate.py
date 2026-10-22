from __future__ import annotations

from pathlib import Path

from .common import DOMAIN_MEMORY_HINT, REGISTRY_MARKERS, holds_domain_memory, is_domain_source_map, shown_path
from .git import run_git

DOMAIN_MEMORY_MARKERS = (":(glob)**/source-map.json", *(f":(glob)**/registry/{marker}" for marker in REGISTRY_MARKERS))


def domain_memory_hint(root: Path, repo_root: Path | None = None) -> str:
    asked = root.resolve()
    places = [place for place in _candidates(asked, repo_root) if place != asked]
    if not places:
        return DOMAIN_MEMORY_HINT
    shown = [shown_path(place, repo_root) for place in places]
    if len(shown) == 1:
        return f"{DOMAIN_MEMORY_HINT}; did you mean {shown[0]}?"
    return f"{DOMAIN_MEMORY_HINT}; Domain Memories in this repository: {', '.join(shown)}"


def domain_memories_in(repo_root: Path) -> list[Path]:
    return sorted({place.resolve() for place in _domain_memories_known_to_git(repo_root) or []})


def _candidates(asked: Path, repo_root: Path | None) -> list[Path]:
    if repo_root is not None:
        listed = _domain_memories_known_to_git(repo_root)
        if listed is not None:
            return sorted({place.resolve() for place in listed})
    return [place for place in _neighbours(asked) if holds_domain_memory(place)]


def _domain_memories_known_to_git(repo_root: Path) -> list[Path] | None:
    try:
        result = run_git(
            repo_root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", *DOMAIN_MEMORY_MARKERS
        )
    except OSError:
        return None
    if result.returncode != 0:
        return None
    markers = [repo_root / path for path in result.stdout.split("\0") if path]
    return [_home_of(marker) for marker in markers if marker.name != "source-map.json" or is_domain_source_map(marker)]


def _home_of(marker: Path) -> Path:
    return marker.parent if marker.name == "source-map.json" else marker.parent.parent


def _neighbours(asked: Path) -> list[Path]:
    parent = asked.parent
    siblings = sorted(place for place in parent.iterdir() if place.is_dir() and place != asked) if parent.is_dir() else []
    children = sorted(place for place in asked.iterdir() if place.is_dir()) if asked.is_dir() else []
    return [parent, *siblings, *children]
