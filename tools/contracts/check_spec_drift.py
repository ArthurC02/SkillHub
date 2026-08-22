#!/usr/bin/env python3
"""Has the Agent Skills specification moved since we pinned it?

The specification has no version string, no tag and no release
(contracts/spec/SOURCE.json), so the pin is a commit SHA plus the git blob SHA
of docs/specification.mdx. Upstream can change a line and every conformance
statement this repository makes goes quietly stale.

Two questions, and they are not the same one:

  1. Does the pinned commit still serve the bytes we recorded? A mismatch means
     the pin itself is wrong or the file was rewritten under a SHA, and nothing
     downstream can be trusted until it is explained.
  2. Does the tracked branch still serve those same bytes? A mismatch is
     ordinary drift: upstream edited the specification and we have not read it.

Exit codes are distinct because the two need different responses:
  0  pinned and tracked agree
  2  drift (question 2) - read the diff, decide, re-pin
  3  the pin does not reproduce (question 1) - stop and explain
  1  a source was unreachable

An unreachable source is a failure, not a pass. "No drift" and "could not look"
must never be the same green tick; that is the whole reason this exists.

Usage:
    python tools/contracts/check_spec_drift.py            # check against main
    python tools/contracts/check_spec_drift.py --ref v2   # against another ref
"""

from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import sys
import urllib.error
import urllib.request

SOURCE = pathlib.Path(__file__).resolve().parents[2] / "contracts" / "spec" / "SOURCE.json"


def blob_sha(data: bytes) -> str:
    """The git object id of `data` as a blob, which is what SOURCE.json pins."""
    h = hashlib.sha1()
    h.update(b"blob %d\0" % len(data))
    h.update(data)
    return h.hexdigest()


def fetch(url: str) -> bytes:
    try:
        with urllib.request.urlopen(url, timeout=60) as resp:
            return resp.read()
    except (urllib.error.URLError, urllib.error.HTTPError, OSError) as e:
        print(f"unreachable: {url} ({e})", file=sys.stderr)
        raise SystemExit(1)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--ref", default="main", help="upstream ref to compare the pin against")
    args = ap.parse_args()

    src = json.loads(SOURCE.read_text(encoding="utf-8"))
    owner_repo = src["source_repository"].rstrip("/").split("github.com/", 1)[1]
    path = src["source_path"]
    pinned_commit = src["pinned"]["commit"]
    pinned_blob = src["pinned"]["blob_sha"]

    base = f"https://raw.githubusercontent.com/{owner_repo}"
    at_pin = blob_sha(fetch(f"{base}/{pinned_commit}/{path}"))
    at_ref = blob_sha(fetch(f"{base}/{args.ref}/{path}"))

    print(f"pinned  {pinned_commit[:12]} {path}: {at_pin}")
    print(f"tracked {args.ref:<12} {path}: {at_ref}")

    if at_pin != pinned_blob:
        print(
            f"\nPIN DOES NOT REPRODUCE: SOURCE.json records {pinned_blob}, the pinned "
            f"commit serves {at_pin}. Every conformance statement in this repository "
            f"names a revision that is not what we validated against. Explain this "
            f"before changing anything else.",
            file=sys.stderr,
        )
        return 3
    if at_ref != pinned_blob:
        print(
            f"\nDRIFT: {args.ref} has moved away from the pinned revision.\n"
            f"  https://github.com/{owner_repo}/commits/{args.ref}/{path}\n"
            f"Read the diff, decide whether it changes what skillpkg checks, then "
            f"re-pin contracts/spec/SOURCE.json and say so in an ADR. Drifting is not "
            f"automatically a defect - it is automatically something a person has to read.",
            file=sys.stderr,
        )
        return 2
    print("\nno drift: the pinned revision is still what upstream serves")
    return 0


if __name__ == "__main__":
    sys.exit(main())
