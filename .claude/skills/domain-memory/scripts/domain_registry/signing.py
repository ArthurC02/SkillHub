from __future__ import annotations

import os
import stat
import subprocess
from pathlib import Path
from typing import Any

SIGNING_KEY_ENV = "DOMAIN_MEMORY_SIGNING_KEY"


def default_key_file() -> Path:
    return Path.home() / ".domain-memory" / "signing-key"


def public_key_path(key_file: Path) -> Path:
    return Path(str(key_file) + ".pub")


def allowed_signers_path(key_file: Path) -> Path:
    return Path(str(key_file) + ".allowed_signers")


def _write_verbatim(path: Path, text: str) -> None:
    with open(path, "w", encoding="utf-8", newline="\n") as handle:
        handle.write(text if text.endswith("\n") else text + "\n")


def _restrict(path: Path) -> None:
    try:
        path.chmod(stat.S_IRUSR | stat.S_IWUSR)
    except OSError:
        pass


def _run(command: list[str], stdin: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        command, input=stdin, capture_output=True, text=True, check=False
    )


def _generate(key_file: Path, principal: str) -> None:
    result = _run(
        ["ssh-keygen", "-t", "ed25519", "-f", str(key_file), "-N", "", "-C", principal, "-q"]
    )
    if result.returncode != 0 or not key_file.is_file():
        raise ValueError(f"ssh-keygen could not create a signing key: {result.stderr.strip()}")


def _adopt_from_environment(key_file: Path, material: str) -> None:
    _write_verbatim(key_file, material)
    _restrict(key_file)
    result = _run(["ssh-keygen", "-y", "-f", str(key_file)])
    if result.returncode != 0:
        key_file.unlink(missing_ok=True)
        raise ValueError(
            f"{SIGNING_KEY_ENV} does not hold a readable OpenSSH private key: {result.stderr.strip()}"
        )
    _write_verbatim(public_key_path(key_file), result.stdout.strip())


def fingerprint(key_file: Path) -> str:
    result = _run(["ssh-keygen", "-lf", str(public_key_path(key_file))])
    if result.returncode != 0:
        raise ValueError(f"ssh-keygen could not read the public key: {result.stderr.strip()}")
    for field in result.stdout.split():
        if field.startswith("SHA256:"):
            return field
    raise ValueError("ssh-keygen did not report a SHA256 fingerprint")


def init_signing_key(
    repo_root: Path,
    principal: str,
    key_file: Path | None = None,
    force: bool = False,
    sign_every_commit: bool = False,
) -> dict[str, Any]:
    if not isinstance(principal, str) or not principal.strip():
        raise ValueError(
            "a signing key needs a principal: the identity an approval is attributed to"
        )
    principal = principal.strip()
    key_file = Path(key_file) if key_file else default_key_file()
    key_file.parent.mkdir(parents=True, exist_ok=True)

    material = os.environ.get(SIGNING_KEY_ENV, "")
    if material.strip():
        if key_file.exists() and not force:
            raise ValueError(
                f"{key_file} already exists; pass --force to replace it from {SIGNING_KEY_ENV}"
            )
        _adopt_from_environment(key_file, material)
        source = "environment"
    elif key_file.exists():
        if force:
            raise ValueError(
                f"--force replaces a key from {SIGNING_KEY_ENV}, and {SIGNING_KEY_ENV} is not set; "
                f"delete {key_file} yourself if you mean to discard it"
            )
        source = "reused"
    else:
        _generate(key_file, principal)
        source = "generated"
    _restrict(key_file)

    public_key = public_key_path(key_file).read_text(encoding="utf-8").strip()
    allowed = allowed_signers_path(key_file)
    _write_verbatim(allowed, f"{principal} {public_key}")

    digest = fingerprint(key_file)
    settings = {
        "gpg.format": "ssh",
        "user.signingkey": str(key_file),
        "gpg.ssh.allowedSignersFile": str(allowed),
    }
    if sign_every_commit:
        settings["commit.gpgsign"] = "true"
    for name, value in settings.items():
        result = _run(["git", "-C", str(repo_root), "config", name, value])
        if result.returncode != 0:
            raise ValueError(f"git config {name} failed: {result.stderr.strip()}")

    return {
        "format": "domain-memory-signing-key/v1",
        "source": source,
        "principal": principal,
        "fingerprint": digest,
        "key_file": str(key_file),
        "public_key_file": str(public_key_path(key_file)),
        "allowed_signers_file": str(allowed),
        "authorized_signers": [principal, digest],
        "portable_via": SIGNING_KEY_ENV,
        "git_config": sorted(settings),
        "signs_every_commit": sign_every_commit,
    }
