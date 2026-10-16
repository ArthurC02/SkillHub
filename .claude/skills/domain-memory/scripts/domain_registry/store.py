from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path
from typing import Any

from .common import (
    ASSET_FORMATS,
    ASSET_KEYS,
    GIT_COMMAND_TIMEOUT_SECONDS,
    load_json,
    registry_dir,
    review_field,
    write_json,
)
from .evidence import without_derived_keys
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


def is_concept_layout(root: Path) -> bool:
    return (registry_dir(root) / INDEX).is_file()


def document_path(root: Path, name: str) -> Path:
    if is_concept_layout(root):
        return registry_dir(root) / (INDEX if name == MANIFEST else asset_of(name))
    return registry_dir(root) / name


def has_document(root: Path, name: str) -> bool:
    if is_concept_layout(root):
        return True
    return document_path(root, name).is_file()


def has_registry(root: Path) -> bool:
    return is_concept_layout(root) or (registry_dir(root) / MANIFEST).is_file()


def manifest_document() -> dict[str, Any]:
    return {"format": MANIFEST_FORMAT, "status": DEFAULT_STATUS, "artifacts": list(ASSET_KEYS)}


def read_document(root: Path, name: str) -> dict[str, Any]:
    if not is_concept_layout(root):
        return load_json(registry_dir(root) / name)
    if name == MANIFEST:
        return manifest_document()
    directory = registry_dir(root) / asset_of(name)
    concepts = sorted(directory.glob("*.md")) if directory.is_dir() else []
    return asset_document(name, [
        (concept.stem, concept.read_text(encoding="utf-8"))
        for concept in concepts
        if concept.name != INDEX
    ])


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
    if not is_concept_layout(root):
        write_json(registry_dir(root) / name, document)
        return
    if name == MANIFEST:
        return
    asset = asset_of(name)
    directory = registry_dir(root) / asset
    directory.mkdir(parents=True, exist_ok=True)
    records = _records_with_their_status(name, document)
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


def logical_documents(root: Path) -> dict[str, Any]:
    documents = {}
    for name in ASSET_KEYS:
        document = read_document(root, name)
        records = document.get(ASSET_KEYS[name], [])
        rest = {key: value for key, value in document.items() if key not in ("format", "status", ASSET_KEYS[name])}
        documents[name] = {
            **rest,
            ASSET_KEYS[name]: sorted(_records_with_their_status(name, document), key=lambda record: str(record.get("id")))
            if isinstance(records, list) else records,
        }
    return documents


def create_concept_layout(root: Path) -> None:
    registry = registry_dir(root)
    registry.mkdir(parents=True, exist_ok=True)
    _write_text(registry / INDEX, root_index())
    for name in ASSET_KEYS:
        write_document(root, name, {ASSET_KEYS[name]: []})


def _records_with_their_status(name: str, document: dict[str, Any]) -> list[dict[str, Any]]:
    default = document.get("status", DEFAULT_STATUS)
    records = document.get(ASSET_KEYS[name], [])
    if default == DEFAULT_STATUS:
        return records
    field = review_field(asset_of(name))
    return [record if field in record else {**record, field: default} for record in records]


def _concept_file_name(asset: str, record_id: Any) -> str:
    if not isinstance(record_id, str) or not CONCEPT_FILE_NAME.fullmatch(record_id) or record_id == "index":
        raise ValueError(
            f"{asset} record id {record_id!r} cannot name a concept file; use letters, digits, '.', '_' or '-'"
        )
    return f"{record_id}.md"


def _write_text(path: Path, text: str) -> None:
    with path.open("w", encoding="utf-8", newline="\n") as output:
        output.write(text)


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
    listing = _git(repo_root, "ls-tree", "-r", "--name-only", "-z", commit, "--", registry)
    if listing is None:
        return None
    paths = [path for path in listing.decode("utf-8", "surrogateescape").split("\0") if path]
    contents = _blobs(repo_root, commit, paths)
    if f"{registry}/{INDEX}" not in contents:
        return _committed_json_documents(contents, registry, names)
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


def _committed_json_documents(contents: dict[str, str], registry: str, names: tuple[str, ...]) -> dict[str, Any] | None:

    documents = {}
    for name in names:
        path = f"{registry}/{name}"
        if path not in contents:
            return None
        try:
            documents[name] = json.loads(contents[path])
        except ValueError as error:
            raise ValueError(f"{path} is not JSON: {error}") from error
    return documents


def _git(repo_root: Path, *arguments: str, stdin: bytes | None = None) -> bytes | None:
    result = subprocess.run(
        ["git", "-C", str(repo_root), *arguments],
        input=stdin, capture_output=True, check=False, timeout=GIT_COMMAND_TIMEOUT_SECONDS,
    )
    return result.stdout if result.returncode == 0 else None


def _blobs(repo_root: Path, commit: str, paths: list[str]) -> dict[str, str]:
    if not paths:
        return {}
    request = "".join(f"{commit}:{path}\n" for path in paths).encode("utf-8", "surrogateescape")
    output = _git(repo_root, "cat-file", "--batch", stdin=request) or b""
    contents: dict[str, str] = {}
    position = 0
    for path in paths:
        header_end = output.index(b"\n", position)
        size = int(output[position:header_end].split()[2])
        start = header_end + 1
        contents[path] = output[start:start + size].decode("utf-8")
        position = start + size + 1
    return contents

