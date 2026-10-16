from __future__ import annotations

from pathlib import Path
from typing import Any

from .common import load_json, registry_dir, write_json

MANIFEST = "manifest.json"


def document_path(root: Path, name: str) -> Path:
    return registry_dir(root) / name


def has_document(root: Path, name: str) -> bool:
    return document_path(root, name).is_file()


def has_registry(root: Path) -> bool:
    return has_document(root, MANIFEST)


def read_document(root: Path, name: str) -> dict[str, Any]:
    return load_json(document_path(root, name))


def write_document(root: Path, name: str, document: dict[str, Any]) -> None:
    write_json(document_path(root, name), document)
