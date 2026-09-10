#!/usr/bin/env python3
"""Query GHCR for the runtime image's SBOM and scan-attestation status,
against the registry at read time rather than the build workflow's gates.
`unknown` (could not check) must never read the same as a pass.

Usage: python tools/sec009/t8-image-audit.py [--json] [--self-check]
Exit:  0 every check passed
       1 setup failed -- nothing was measured, which is not a pass
       2 a check failed, or could not be made
"""

from __future__ import annotations

import base64
import json
import os
import re
import sys
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
DOCKERFILE = os.path.join(REPO_ROOT, "infra", "images", "runtime-agent-sdk", "Dockerfile")
BASELINE_FILE = os.path.join(REPO_ROOT, "infra", "nodes", "gvisor-baseline.txt")
ALLOWLIST_FILE = os.path.join(REPO_ROOT, "infra", "egress", "allowlist.yaml")

REGISTRY = "ghcr.io"
IMAGE_REPO = "arthurc02/skillhub-runtime-agent-sdk"

SBOM_PREDICATE = "https://spdx.dev/Document/v2.3"
VULN_PREDICATE = "https://in-toto.io/attestation/vulns/v0.1"

SCAN_VALIDITY_DAYS = 30
SCAN_WARN_DAYS = 7

MANIFEST_ACCEPT = ", ".join(
    [
        "application/vnd.oci.image.index.v1+json",
        "application/vnd.oci.image.manifest.v1+json",
        "application/vnd.docker.distribution.manifest.list.v2+json",
        "application/vnd.docker.distribution.manifest.v2+json",
    ]
)


class SetupError(Exception):
    """Something the check depends on is missing, so nothing was measured."""



def _token() -> str:
    url = "https://%s/token?scope=repository:%s:pull" % (REGISTRY, IMAGE_REPO)
    try:
        with urllib.request.urlopen(url, timeout=30) as resp:
            return json.load(resp)["token"]
    except (urllib.error.URLError, KeyError, ValueError) as exc:
        raise SetupError("could not get an anonymous pull token: %s" % exc)


def _get(path: str, token: str, accept: str | None = None):
    """Return (parsed json body, response headers) for a registry path.
    Headers stay as urllib's case-insensitive Message, not a dict — GHCR's
    HTTP/2 responses arrive with lower-cased header names."""
    req = urllib.request.Request("https://%s/v2/%s/%s" % (REGISTRY, IMAGE_REPO, path))
    req.add_header("Authorization", "Bearer " + token)
    if accept:
        req.add_header("Accept", accept)
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp), resp.headers


def _blob(digest: str, token: str) -> dict:
    req = urllib.request.Request("https://%s/v2/%s/blobs/%s" % (REGISTRY, IMAGE_REPO, digest))
    req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp)



def dockerfile_version() -> str:
    """The version tag the publish step pushes. Same expression as the workflow."""
    try:
        with open(DOCKERFILE, encoding="utf-8") as fh:
            text = fh.read()
    except OSError as exc:
        raise SetupError("cannot read %s: %s" % (DOCKERFILE, exc))
    m = re.search(r"^ARG IMAGE_VERSION=(.+)$", text, re.M)
    if not m:
        raise SetupError("ARG IMAGE_VERSION not found in the Dockerfile")
    return m.group(1).strip()


def dockerfile_base_is_pinned() -> tuple[bool, str]:
    """I-02: every FROM resolves through a digest, not a tag."""
    with open(DOCKERFILE, encoding="utf-8") as fh:
        froms = re.findall(r"^FROM\s+(\S+)", fh.read(), re.M)
    if not froms:
        return False, "no FROM line found"
    unpinned = [f for f in froms if "@sha256:" not in f and not f.startswith("$")]
    if unpinned:
        return False, "unpinned base: " + ", ".join(unpinned)
    return True, "%d FROM line(s), all by digest" % len(froms)


def first_value(path: str) -> str:
    """First non-comment, non-blank line of a file."""
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if line and not line.startswith("#"):
                return line
    return ""


def pinned_ip() -> str:
    """The sandbox-tier pinned_ip from the egress allow-list, read with a
    narrow regex rather than a YAML parse so this check needs no dependency
    a freshly provisioned node might lack."""
    try:
        with open(ALLOWLIST_FILE, encoding="utf-8") as fh:
            text = fh.read()
    except OSError as exc:
        raise SetupError("cannot read %s: %s" % (ALLOWLIST_FILE, exc))
    m = re.search(r"^\s*pinned_ip:\s*(\S+)\s*$", text, re.M)
    return m.group(1) if m else ""


PASS, FAIL, UNKNOWN = "PASS", "FAIL", "UNKNOWN"


class Report:
    def __init__(self) -> None:
        self.rows: list[dict] = []

    def add(self, ident: str, name: str, status: str, detail: str) -> None:
        self.rows.append({"id": ident, "check": name, "status": status, "detail": detail})

    @property
    def ok(self) -> bool:
        return all(r["status"] == PASS for r in self.rows)

    def render(self) -> None:
        width = max(len(r["id"]) for r in self.rows)
        for r in self.rows:
            print("  %-*s  %-7s %s -- %s" % (width, r["id"], r["status"], r["check"], r["detail"]))


def grade_scan_age(finished: str | None, now: datetime) -> tuple[str, str]:
    """Judge whether a scan is recent enough to still count as evidence,
    pulled out of audit() so --self-check can drive it offline."""
    if not finished:
        return UNKNOWN, "predicate has no scan_finished_on"
    try:
        scanned = datetime.fromisoformat(finished.replace("Z", "+00:00"))
    except ValueError:
        return UNKNOWN, "scan_finished_on is not a timestamp: %r" % finished
    age = now - scanned
    expires = scanned + timedelta(days=SCAN_VALIDITY_DAYS)
    detail = "scanned %s, %d day(s) old, expires %s" % (
        finished, age.days, expires.date().isoformat())
    if age > timedelta(days=SCAN_VALIDITY_DAYS):
        return FAIL, detail + " -- EXPIRED"
    if age > timedelta(days=SCAN_VALIDITY_DAYS - SCAN_WARN_DAYS):
        detail += "  [ADR-022: within the 7-day expiry warning window]"
    return PASS, detail


def self_check() -> int:
    """Offline proof that the expiry grading can actually fail, since a real
    run stays comfortably in-date for a long time regardless."""
    now = datetime(2026, 8, 23, tzinfo=timezone.utc)

    def at(days: int) -> str:
        return (now - timedelta(days=days)).strftime("%Y-%m-%dT%H:%M:%SZ")

    cases = [
        (at(1), PASS, "a fresh scan is evidence"),
        (at(SCAN_VALIDITY_DAYS - SCAN_WARN_DAYS - 1), PASS, "outside the warning window"),
        (at(SCAN_VALIDITY_DAYS - 1), PASS, "inside the warning window, still valid"),
        (at(SCAN_VALIDITY_DAYS + 1), FAIL, "past 30 days is expired, not merely old"),
        (None, UNKNOWN, "no timestamp is not a pass"),
        ("whenever", UNKNOWN, "an unparseable timestamp is not a pass"),
    ]
    bad = 0
    for value, want, why in cases:
        got, _ = grade_scan_age(value, now)
        mark = "ok  " if got == want else "BAD "
        if got != want:
            bad += 1
        print("  %s %-24s want %-7s got %-7s  %s" % (mark, value, want, got, why))

    _, detail = grade_scan_age(at(SCAN_VALIDITY_DAYS - 1), now)
    if "warning window" not in detail:
        print("  BAD  the 7-day warning is not surfaced in the detail line")
        bad += 1

    print("self-check: %s" % ("all grading cases behave" if not bad else "%d case(s) wrong" % bad))
    return 0 if not bad else 2


def audit() -> Report:
    rep = Report()
    version = dockerfile_version()
    token = _token()

    print("image:   %s/%s:%s" % (REGISTRY, IMAGE_REPO, version))

    try:
        _, headers = _get("manifests/" + version, token, MANIFEST_ACCEPT)
        digest = headers.get("Docker-Content-Digest", "").strip()
    except urllib.error.HTTPError as exc:
        rep.add("I-01", "versioned image published", FAIL, "GET manifest -> HTTP %d" % exc.code)
        return rep
    if not digest:
        rep.add("I-01", "versioned image published", UNKNOWN, "no Docker-Content-Digest header")
        return rep
    rep.add("I-01", "versioned image published", PASS, "%s -> %s" % (version, digest))
    print("digest:  %s" % digest)

    pinned, detail = dockerfile_base_is_pinned()
    rep.add("I-02", "base pinned by digest", PASS if pinned else FAIL, detail)

    # Read by tag, not the OCI referrers API: GHCR does not serve referrers, so
    # querying it returns an empty set even when attestations exist.
    att_tag = "sha256-" + digest.split(":", 1)[1]
    try:
        index, _ = _get("manifests/" + att_tag, token, MANIFEST_ACCEPT)
    except urllib.error.HTTPError as exc:
        rep.add("I-03", "SBOM attestation", FAIL, "no attestation index (%s -> %d)" % (att_tag, exc.code))
        rep.add("I-04", "scan attestation in date", FAIL, "no attestation index")
        rep.add("I-06", "no fixable Critical/High", UNKNOWN, "no scan attestation to read")
        return rep

    found: dict[str, str] = {}
    for m in index.get("manifests", []):
        ptype = (m.get("annotations") or {}).get("dev.sigstore.bundle.predicateType")
        if ptype:
            found[ptype] = m.get("digest", "")

    if SBOM_PREDICATE in found:
        rep.add("I-03", "SBOM attestation", PASS, "SPDX 2.3 bundle %s" % found[SBOM_PREDICATE][:19])
    else:
        rep.add("I-03", "SBOM attestation", FAIL, "no %s in the index" % SBOM_PREDICATE)

    if VULN_PREDICATE not in found:
        rep.add("I-04", "scan attestation in date", FAIL, "no %s in the index" % VULN_PREDICATE)
        rep.add("I-06", "no fixable Critical/High", UNKNOWN, "no scan attestation to read")
        return rep

    try:
        bundle_manifest, _ = _get("manifests/" + found[VULN_PREDICATE], token, MANIFEST_ACCEPT)
        bundle = _blob(bundle_manifest["layers"][0]["digest"], token)
        envelope = bundle.get("dsseEnvelope") or bundle.get("dsse_envelope")
        statement = json.loads(base64.b64decode(envelope["payload"]))
    except (urllib.error.HTTPError, KeyError, ValueError, TypeError) as exc:
        rep.add("I-04", "scan attestation in date", UNKNOWN, "cannot decode the bundle: %s" % exc)
        rep.add("I-06", "no fixable Critical/High", UNKNOWN, "cannot decode the bundle")
        return rep

    subjects = {
        "sha256:" + s.get("digest", {}).get("sha256", "")
        for s in statement.get("subject", [])
    }
    if digest not in subjects:
        rep.add("I-04", "scan attestation in date", FAIL,
                "attestation subject %s != published digest" % (sorted(subjects) or ["<none>"])[0])
        rep.add("I-06", "no fixable Critical/High", UNKNOWN, "attestation is about other bytes")
        return rep

    predicate = statement.get("predicate", {})
    finished = (predicate.get("metadata") or {}).get("scan_finished_on")
    rep.add("I-04", "scan attestation in date", *grade_scan_age(finished, datetime.now(timezone.utc)))

    summary = predicate.get("summary") or {}
    fixable = summary.get("fixable_critical_high")
    if fixable is None:
        rep.add("I-06", "no fixable Critical/High", UNKNOWN, "predicate has no summary.fixable_critical_high")
    else:
        by_sev = summary.get("by_severity") or {}
        detail = "fixable Critical/High = %s (of %s findings: %s)" % (
            fixable, summary.get("total"),
            ", ".join("%s %s" % (v, k) for k, v in sorted(by_sev.items())))
        rep.add("I-06", "no fixable Critical/High", PASS if fixable == 0 else FAIL, detail)

    return rep


def preconditions() -> Report:
    """ADR-022's three batch preconditions. Any one missing => T8 unknown => fail."""
    rep = Report()

    baseline = first_value(BASELINE_FILE)
    if baseline == "unset":
        rep.add("pre-2", "gvisor-baseline.txt filled", FAIL,
                "still `unset` -- no sandbox node has been provisioned, so gate A has "
                "nothing to compare `runsc --version` against")
    elif not baseline:
        rep.add("pre-2", "gvisor-baseline.txt filled", UNKNOWN, "file has no value line")
    else:
        rep.add("pre-2", "gvisor-baseline.txt filled", PASS, baseline)

    ip = pinned_ip()
    if ip == "unset":
        rep.add("pre-3", "sandbox-tier pinned_ip filled", FAIL,
                "still `unset` -- IaC renders no accept rule, which is fail-closed: a "
                "sandbox on such a node reaches nothing at all")
    elif not ip:
        rep.add("pre-3", "sandbox-tier pinned_ip filled", UNKNOWN, "no pinned_ip key found")
    else:
        rep.add("pre-3", "sandbox-tier pinned_ip filled", PASS,
                "%s (that it is not a control-plane address is T5-7's job, not this one)" % ip)

    return rep


def main() -> int:
    if "--self-check" in sys.argv:
        return self_check()
    as_json = "--json" in sys.argv
    try:
        image = audit()
        pre = preconditions()
    except SetupError as exc:
        print("t8 image audit: setup failed, nothing was measured -- %s" % exc, file=sys.stderr)
        return 1
    except urllib.error.URLError as exc:
        print("t8 image audit: registry unreachable, nothing was measured -- %s" % exc, file=sys.stderr)
        return 1

    got = {r["id"]: r["status"] for r in image.rows}
    pre1 = PASS if all(got.get(i) == PASS for i in ("I-01", "I-03", "I-04")) else FAIL
    pre.rows.insert(0, {
        "id": "pre-1", "check": "image on GHCR with SBOM + scan",
        "status": pre1,
        "detail": "= I-01 and I-03 and I-04 above",
    })

    if as_json:
        print(json.dumps({"image_half": image.rows, "preconditions": pre.rows}, indent=2))
    else:
        print()
        print("T8 image half (Suite 1 -- no node needed):")
        image.render()
        print()
        print("SEC-009 batch preconditions (ADR-022; any FAIL => the whole batch is unknown):")
        pre.render()
        print()

    if not image.ok:
        print("t8 image half: a check failed or could not be made", file=sys.stderr)
        return 2
    if not pre.ok:
        print("t8 image half: PASS. Batch preconditions: NOT satisfied -- "
              "SEC-009 cannot start until the node exists.", file=sys.stderr)
        return 2
    print("t8 image half and all three preconditions: PASS -- NOT the SEC-009 acceptance")
    return 0


if __name__ == "__main__":
    sys.exit(main())
