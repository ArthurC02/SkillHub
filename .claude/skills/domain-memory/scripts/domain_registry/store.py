from __future__ import annotations

import re
import copy
import io
import tarfile
from collections.abc import Iterator
from contextlib import contextmanager
from contextvars import ContextVar
from pathlib import Path
from typing import Any

from .common import (
    ASSET_FORMATS,
    ASSET_KEYS,
    registry_dir,
    shown_path,
)
from .evidence import without_derived_keys
from .git import run_git_bytes
from .locate import domain_memory_hint
from .okf import OKF_VERSION, PROSE_FIELDS, TITLE_FIELDS, concept_path, decode, encode

MANIFEST = "manifest.json"
INDEX = "index.md"
MANIFEST_FORMAT = "domain-registry/v1"
DEFAULT_STATUS = "candidate"
CONCEPT_FILE_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]*")
INDEX_DESCRIPTION_LIMIT = 160
ASSET_HEADINGS = {
    "contexts": "Bounded Contexts",
    "vocabulary": "Vocabulary",
    "aggregates": "Aggregates",
    "rules": "Business Rules",
    "contracts": "Contracts",
    "interactions": "Interactions",
    "decisions": "Decisions",
    "events": "Domain Events",
    "capabilities": "Capabilities",
    "value-objects": "Value Objects",
    "dependency-policies": "Dependency Policies",
}


def asset_of(name: str) -> str:
    return name.removesuffix(".json")


def has_registry(root: Path) -> bool:
    return (registry_dir(root) / INDEX).is_file()


def missing_registry(root: Path, repo_root: Path | None = None) -> str:
    return (
        f"no Domain Registry at {shown_path(root, repo_root)}: it holds no registry/{INDEX}; "
        f"{domain_memory_hint(root, repo_root)}"
    )


def manifest_document() -> dict[str, Any]:
    return {"format": MANIFEST_FORMAT, "status": DEFAULT_STATUS, "artifacts": list(ASSET_KEYS)}


_READ_ONCE: ContextVar[dict[tuple[str, str], dict[str, Any]] | None] = ContextVar("documents_read_once", default=None)
_FILES_ONCE: ContextVar[dict[str, dict[str, bytes]] | None] = ContextVar("registry_files_read_once", default=None)


@contextmanager
def documents_read_once() -> Iterator[None]:
    documents = _READ_ONCE.set({})
    files = _FILES_ONCE.set({})
    try:
        yield
    finally:
        _FILES_ONCE.reset(files)
        _READ_ONCE.reset(documents)


def read_document(root: Path, name: str) -> dict[str, Any]:
    held = _READ_ONCE.get()
    if held is None:
        return _read_document(root, name)
    key = (str(root), name)
    if key not in held:
        held[key] = _read_document(root, name)
    return copy.deepcopy(held[key])


def _read_document(root: Path, name: str) -> dict[str, Any]:
    if not has_registry(root):
        raise ValueError(missing_registry(root))
    if name == MANIFEST:
        return manifest_document()
    asset = asset_of(name)
    held = registry_files(root)
    concepts = sorted(
        (stem, relative) for relative in held if (stem := _concept_stem(asset, relative)) is not None
    )
    return asset_document(name, [(stem, _text_or_refusal(relative, held[relative])) for stem, relative in concepts])


def _concept_stem(asset: str, relative: str) -> str | None:
    parent, _, name = relative.rpartition("/")
    if parent != asset or not name.endswith(".md") or name == INDEX:
        return None
    return name[: -len(".md")]


def registry_files(root: Path) -> dict[str, bytes]:
    held = _FILES_ONCE.get()
    if held is None:
        return _registry_files(root)
    key = str(root)
    if key not in held:
        held[key] = _registry_files(root)
    return held[key]


def _registry_files(root: Path) -> dict[str, bytes]:
    directory = registry_dir(root)
    return {path.relative_to(directory).as_posix(): path.read_bytes() for path in sorted(directory.rglob("*")) if path.is_file()}


def text_of(data: bytes, errors: str = "strict") -> str:
    return io.TextIOWrapper(io.BytesIO(data), encoding="utf-8", errors=errors).read()


def _text_or_refusal(relative: str, data: bytes) -> str:
    try:
        return text_of(data)
    except UnicodeDecodeError as error:
        raise ValueError(f"{relative}: {error}") from error


def read_record(root: Path, name: str, record_id: str) -> dict[str, Any] | None:
    asset = asset_of(name)
    try:
        concept = registry_dir(root) / asset / _concept_file_name(asset, record_id)
    except ValueError:
        return None
    if not concept.is_file():
        return None
    return _decoded(asset, record_id, _text_or_refusal(f"{asset}/{concept.name}", concept.read_bytes()))


def asset_document(name: str, concepts: list[tuple[str, str]]) -> dict[str, Any]:
    asset = asset_of(name)
    records = [_decoded(asset, record_id, text) for record_id, text in concepts]
    return {"format": ASSET_FORMATS[name], "status": DEFAULT_STATUS, ASSET_KEYS[name]: records}


def _decoded(asset: str, record_id: str, text: str) -> dict[str, Any]:
    try:
        return decode(asset, record_id, text)
    except ValueError as error:
        raise ValueError(f"{asset}/{record_id}.md: {error}") from error


def write_document(root: Path, name: str, document: dict[str, Any]) -> None:
    if name == MANIFEST:
        return
    asset = asset_of(name)
    directory = registry_dir(root) / asset
    directory.mkdir(parents=True, exist_ok=True)
    records = document.get(ASSET_KEYS[name], [])
    written = {}
    for record in records:
        file_name = _concept_file_name(asset, record.get("id"))
        if file_name in written:
            raise ValueError(f"{asset} holds two records with id {record['id']}; a concept file holds one")
        written[file_name] = encode(asset, without_derived_keys(record))
    for concept in directory.glob("*.md"):
        if concept.name != INDEX and concept.name not in written:
            concept.unlink()
    for file_name, text in written.items():
        _write_text(directory / file_name, text)
    _write_text(directory / INDEX, asset_index(asset, records))


def create_concept_layout(root: Path) -> None:
    registry = registry_dir(root)
    registry.mkdir(parents=True, exist_ok=True)
    _write_text(registry / INDEX, root_index())
    for name in ASSET_KEYS:
        write_document(root, name, {ASSET_KEYS[name]: []})


def _concept_file_name(asset: str, record_id: Any) -> str:
    if not isinstance(record_id, str) or not CONCEPT_FILE_NAME.fullmatch(record_id) or record_id == "index":
        raise ValueError(
            f"{asset} record id {record_id!r} cannot name a concept file; use letters, digits, '.', '_' or '-'"
        )
    return f"{record_id}.md"


def _write_text(path: Path, text: str) -> None:
    with path.open("w", encoding="utf-8", newline="\n") as output:
        output.write(text)


def stale_indexes(root: Path, documents: dict[str, dict[str, Any]] | None = None) -> list[Path]:
    expected = _expected_indexes(root, documents)
    return [path for path, text in expected.items() if not path.is_file() or path.read_text(encoding="utf-8") != text]


def refresh_indexes(root: Path) -> list[Path]:
    expected = _expected_indexes(root)
    stale = [path for path, text in expected.items() if not path.is_file() or path.read_text(encoding="utf-8") != text]
    for path in stale:
        _write_text(path, expected[path])
    return stale


def _expected_indexes(root: Path, documents: dict[str, dict[str, Any]] | None = None) -> dict[Path, str]:
    if not has_registry(root):
        return {}
    expected = {registry_dir(root) / INDEX: root_index()}
    for name in ASSET_KEYS:
        asset = asset_of(name)
        if not (registry_dir(root) / asset).is_dir() or (documents is not None and name not in documents):
            continue
        document = documents[name] if documents is not None else read_document(root, name)
        expected[registry_dir(root) / asset / INDEX] = asset_index(asset, document[ASSET_KEYS[name]])
    return expected


def root_index() -> str:
    lines = ["---", f'okf_version: "{OKF_VERSION}"', "---", "# Domain Registry", ""]
    lines += [f"- [{ASSET_HEADINGS[asset_of(name)]}](/{asset_of(name)}/index.md)" for name in ASSET_KEYS]
    return "\n".join(lines) + "\n"


def asset_index(asset: str, records: list[dict[str, Any]]) -> str:
    lines = [f"# {ASSET_HEADINGS[asset]}", ""]
    for record in sorted(records, key=lambda entry: entry["id"]):
        label = record.get(TITLE_FIELDS.get(asset, ""), record["id"])
        description = _first_sentence(record.get(PROSE_FIELDS.get(asset, "")))
        link = f"- [{label}]({concept_path(asset, record['id'])})"
        lines.append(f"{link} — {description}" if description else link)
    return "\n".join(lines) + "\n"


def _first_sentence(prose: Any) -> str:
    if not isinstance(prose, str):
        return ""
    sentence = prose.split("\n")[0].split(". ")[0].rstrip(".")
    if len(sentence) > INDEX_DESCRIPTION_LIMIT:
        sentence = sentence[: INDEX_DESCRIPTION_LIMIT - 1].rstrip() + "…"
    return sentence + "."


def committed_documents(repo_root: Path, commit: str, root: Path, names: tuple[str, ...]) -> dict[str, Any] | None:
    registry = registry_dir(root).resolve().relative_to(repo_root.resolve()).as_posix()
    listing = run_git_bytes(repo_root, "ls-tree", "-r", "--name-only", "-z", commit, "--", registry)
    if listing.returncode != 0 or not listing.stdout.strip():
        return None
    contents = _archived_texts(repo_root, commit, registry)
    if f"{registry}/{INDEX}" not in contents:
        return None
    documents: dict[str, Any] = {}
    for name in names:
        if name == MANIFEST:
            documents[name] = manifest_document()
            continue
        prefix = f"{registry}/{asset_of(name)}/"
        documents[name] = asset_document(name, sorted(
            (path[len(prefix):].removesuffix(".md"), text)
            for path, text in contents.items()
            if path.startswith(prefix) and path.endswith(".md") and "/" not in path[len(prefix):]
            and path[len(prefix):] != INDEX
        ))
    return documents


def _archived_texts(repo_root: Path, commit: str, directory: str) -> dict[str, str]:
    archive = run_git_bytes(repo_root, "archive", "--format=tar", commit, "--", directory)
    if archive.returncode != 0:
        stderr = archive.stderr.decode("utf-8", "replace").strip()
        raise ValueError(f"git could not read {commit}: {stderr or archive.returncode}")
    with tarfile.open(fileobj=io.BytesIO(archive.stdout)) as tar:
        return {member.name: tar.extractfile(member).read().decode("utf-8") for member in tar.getmembers() if member.isfile()}

