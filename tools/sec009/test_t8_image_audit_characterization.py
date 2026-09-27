#!/usr/bin/env python3
"""Characterization of the image audit's grading, with the registry, the
Dockerfile and the clock replaced by fakes: nothing reaches the network."""

import base64
import contextlib
import importlib.util
import io
import json
import sys
import urllib.error
from datetime import datetime, timezone
from pathlib import Path
from unittest import mock

_spec = importlib.util.spec_from_file_location("t8_image_audit", Path(__file__).with_name("t8-image-audit.py"))
audit = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(audit)

DIGEST = "sha256:" + "d" * 64
OTHER = "sha256:" + "e" * 64
ATT_TAG = "sha256-" + "d" * 64
VERSION = "2026.08-12"
NOW = datetime(2026, 8, 23, tzinfo=timezone.utc)


class FixedClock(datetime):
    @classmethod
    def now(cls, tz=None):
        return NOW


def missing(path):
    return urllib.error.HTTPError("https://ghcr.io/" + path, 404, "Not Found", {}, None)


def statement(subject=DIGEST, finished="2026-08-20T10:00:00Z", summary=None):
    body = {"subject": [{"digest": {"sha256": subject.split(":", 1)[1]}}],
            "predicate": {"metadata": {"scan_finished_on": finished}}}
    if summary is not None:
        body["predicate"]["summary"] = summary
    return {"dsseEnvelope": {"payload": base64.b64encode(json.dumps(body).encode()).decode()}}


def registry(manifest=None, index=None, bundle=None, bundle_error=False):
    def get(path, token, accept=None):
        assert token == "tok" and accept == audit.MANIFEST_ACCEPT, (token, accept)
        if path == "manifests/" + VERSION or path == "manifests/" + DIGEST:
            if manifest is None:
                raise missing(path)
            return {}, manifest
        if path == "manifests/" + ATT_TAG:
            if index is None:
                raise missing(path)
            return index, {}
        if path == "manifests/sha256:vuln":
            if bundle_error:
                raise missing(path)
            return {"layers": [{"digest": "sha256:layer"}]}, {}
        raise AssertionError("unexpected registry path " + path)

    def blob(digest, token):
        assert digest == "sha256:layer"
        return bundle
    return get, blob


def run(get, blob, node_ref="", pinned=(True, "1 FROM line(s), all by digest")):
    err = io.StringIO()
    env = {"SKILLHUB_SANDBOX_IMAGE": node_ref} if node_ref else {}
    with mock.patch.object(audit, "_token", lambda: "tok"), \
            mock.patch.object(audit, "_get", get), mock.patch.object(audit, "_blob", blob), \
            mock.patch.object(audit, "dockerfile_version", lambda: VERSION), \
            mock.patch.object(audit, "dockerfile_base_is_pinned", lambda: pinned), \
            mock.patch.object(audit, "datetime", FixedClock), \
            mock.patch.dict(audit.os.environ, env, clear=False), \
            contextlib.redirect_stderr(err):
        if not node_ref:
            audit.os.environ.pop("SKILLHUB_SANDBOX_IMAGE", None)
        rep = audit.audit()
    return [(r["id"], r["status"], r["detail"]) + ((r["expires_in_days"],) if "expires_in_days" in r else ())
            for r in rep.rows], err.getvalue()


INDEX = {"manifests": [
    {"digest": "sha256:sbom", "annotations": {"dev.sigstore.bundle.predicateType": audit.SBOM_PREDICATE}},
    {"digest": "sha256:vuln", "annotations": {"dev.sigstore.bundle.predicateType": audit.VULN_PREDICATE}},
    {"digest": "sha256:none", "annotations": None},
]}
HEADERS = {"Docker-Content-Digest": " " + DIGEST + " "}
I01 = ("I-01", audit.PASS, f"{VERSION} -> {DIGEST}")
I02 = ("I-02", audit.PASS, "1 FROM line(s), all by digest")
I03 = ("I-03", audit.PASS, "SPDX 2.3 bundle sha256:sbom")
IMAGE_LINE = f"image:   ghcr.io/{audit.IMAGE_REPO}:{VERSION}\n"


def test_a_node_running_another_tag_stops_at_i01():
    rows, err = run(*registry(), node_ref=f"ghcr.io/{audit.IMAGE_REPO}:2026.08-11@{DIGEST}")
    assert rows == [("I-01", audit.FAIL, "the node runs tag 2026.08-11 but its checkout's Dockerfile "
                     f"declares IMAGE_VERSION {VERSION}, so the node and its release disagree about "
                     "which image it should run")], rows
    assert err == ""


def test_an_unpublished_manifest_fails_and_a_missing_digest_is_unknown():
    rows, err = run(*registry())
    assert rows == [("I-01", audit.FAIL, f"GET manifest {VERSION} -> HTTP 404")], rows
    assert err == IMAGE_LINE
    rows, _ = run(*registry(manifest={}))
    assert rows == [("I-01", audit.UNKNOWN, "no Docker-Content-Digest header")], rows


def test_no_attestation_index_fails_both_attestations():
    rows, err = run(*registry(manifest=HEADERS), pinned=(False, "unpinned base: node:22"))
    assert rows == [I01, ("I-02", audit.FAIL, "unpinned base: node:22"),
                    ("I-03", audit.FAIL, f"no attestation index ({ATT_TAG} -> 404)"),
                    ("I-04", audit.FAIL, "no attestation index"),
                    ("I-06", audit.UNKNOWN, "no scan attestation to read")], rows
    assert err == IMAGE_LINE + f"digest:  {DIGEST}\n"


def test_an_index_without_either_predicate_fails_both():
    rows, _ = run(*registry(manifest=HEADERS, index={"manifests": [{"digest": "x"}]}))
    assert rows[2:] == [("I-03", audit.FAIL, f"no {audit.SBOM_PREDICATE} in the index"),
                        ("I-04", audit.FAIL, f"no {audit.VULN_PREDICATE} in the index"),
                        ("I-06", audit.UNKNOWN, "no scan attestation to read")], rows


def test_an_undecodable_bundle_is_unknown_not_a_pass():
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle={"other": 1}))
    assert rows[2:] == [I03, ("I-04", audit.UNKNOWN, "cannot decode the bundle: 'NoneType' object is "
                                                     "not subscriptable"),
                        ("I-06", audit.UNKNOWN, "cannot decode the bundle")], rows
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle_error=True))
    assert rows[3] == ("I-04", audit.UNKNOWN, "cannot decode the bundle: HTTP Error 404: Not Found"), rows


def test_an_attestation_about_other_bytes_fails():
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle=statement(subject=OTHER)))
    assert rows[2:] == [I03, ("I-04", audit.FAIL, f"attestation subject {OTHER} != published digest"),
                        ("I-06", audit.UNKNOWN, "attestation is about other bytes")], rows


def test_a_fresh_scan_is_graded_and_fixable_findings_decide_i06():
    summary = {"fixable_critical_high": 2, "total": 9, "by_severity": {"high": 2, "critical": 0, "low": 7}}
    node_ref = f"ghcr.io/{audit.IMAGE_REPO}:{VERSION}@{DIGEST}"
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle=statement(summary=summary)), node_ref=node_ref)
    assert rows == [
        ("I-01", audit.PASS, f"{VERSION} -> {DIGEST} (the digest this node runs)"), I02, I03,
        ("I-04", audit.PASS, "scanned 2026-08-20T10:00:00Z, 2 day(s) old, expires 2026-09-19", 27),
        ("I-06", audit.FAIL, "fixable Critical/High = 2 (of 9 findings: 0 critical, 2 high, 7 low)"),
    ], rows
    clean = dict(summary, fixable_critical_high=0)
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle=statement(summary=clean)))
    assert rows[-1][:2] == ("I-06", audit.PASS), rows
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle=statement(summary={})))
    assert rows[-1] == ("I-06", audit.UNKNOWN, "predicate has no summary.fixable_critical_high"), rows


def test_an_expired_scan_fails_and_keeps_its_negative_expiry():
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX,
                            bundle=statement(finished="2026-07-01T00:00:00Z", summary={})))
    assert rows[3] == ("I-04", audit.FAIL, "scanned 2026-07-01T00:00:00Z, 53 day(s) old, expires "
                       "2026-07-31 -- EXPIRED", -23), rows


if __name__ == "__main__":
    failed = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"ok   {name}")
            except AssertionError as exc:
                failed += 1
                print(f"FAIL {name}: {exc}")
    sys.exit(1 if failed else 0)
