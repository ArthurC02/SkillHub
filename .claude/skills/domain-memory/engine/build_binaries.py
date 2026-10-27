from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

ENGINE = Path(__file__).resolve().parent
BIN = ENGINE / "bin"
MANIFEST = BIN / "manifest.json"
FORMAT = "domain-memory-engine-binaries/v1"
SOURCES = ("Cargo.toml", "Cargo.lock", "rust-toolchain.toml", "contract/cli.json")
PLATFORMS = {
    "linux-x86_64/domain-memory": ("x86_64-unknown-linux-musl", "domain-memory", {"CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER": "rust-lld"}),
    "windows-x86_64/domain-memory.exe": ("x86_64-pc-windows-gnullvm", "domain-memory.exe", {}),
}


def source_sha256() -> str:
    paths = [*(ENGINE / name for name in SOURCES), *sorted((ENGINE / "src").rglob("*.rs"))]
    digest = hashlib.sha256()
    for path in sorted(paths, key=lambda path: path.relative_to(ENGINE).as_posix()):
        digest.update(path.relative_to(ENGINE).as_posix().encode() + b"\0" + path.read_bytes() + b"\0")
    return digest.hexdigest()


def file_sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def expected_manifest() -> dict:
    return {
        "format": FORMAT,
        "source_sha256": source_sha256(),
        "binaries": {
            name: {"target": target, "sha256": file_sha256(BIN / name)}
            for name, (target, _, _) in PLATFORMS.items()
            if (BIN / name).is_file()
        },
    }


def problems() -> list[str]:
    if not MANIFEST.is_file():
        return [f"{MANIFEST.relative_to(ENGINE).as_posix()} is missing; run build_binaries.py"]
    recorded = json.loads(MANIFEST.read_text(encoding="utf-8"))
    found = []
    if recorded.get("source_sha256") != source_sha256():
        found.append("the engine source changed after the binaries were built; run build_binaries.py")
    for name in PLATFORMS:
        path = BIN / name
        entry = recorded.get("binaries", {}).get(name)
        if not path.is_file():
            found.append(f"bin/{name} is missing; run build_binaries.py")
        elif entry is None or entry.get("sha256") != file_sha256(path):
            found.append(f"bin/{name} is not the binary the manifest records; run build_binaries.py")
    return found


def build() -> None:
    for name, (target, built, linker) in PLATFORMS.items():
        subprocess.run(["cargo", "build", "--release", "--locked", "--target", target], cwd=ENGINE,
                       env={**os.environ, **linker}, check=True)
        destination = BIN / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ENGINE / "target" / target / "release" / built, destination)
        destination.chmod(0o755)
    MANIFEST.write_text(json.dumps(expected_manifest(), indent=2) + "\n", encoding="utf-8", newline="\n")


def main() -> int:
    parser = argparse.ArgumentParser(description="Build the committed engine binaries, or check that they match the source.")
    parser.add_argument("--check", action="store_true", help="compare without building")
    if not parser.parse_args().check:
        build()
    found = problems()
    for problem in found:
        print(f"ERROR: {problem}", file=sys.stderr)
    return 1 if found else 0


if __name__ == "__main__":
    raise SystemExit(main())
