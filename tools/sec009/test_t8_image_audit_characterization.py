#!/usr/bin/env python3
"""Characterization of the image audit's grading, with the registry, the
Dockerfile and the clock replaced by fakes: nothing reaches the network."""

import base64
import contextlib
import importlib.util
import io
import json
import subprocess
import sys
import tempfile
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
    body = {"predicateType": "https://in-toto.io/attestation/vulns/v0.1",
            "subject": [{"digest": {"sha256": subject.split(":", 1)[1]}}],
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
        layers = {"manifests/sha256:vuln": "sha256:layer", "manifests/sha256:sbom": "sha256:sbom-layer"}
        if path in layers:
            if bundle_error and path == "manifests/sha256:vuln":
                raise missing(path)
            return {"layers": [{"digest": layers[path]}]}, {}
        raise AssertionError("unexpected registry path " + path)

    def blob(digest, token):
        if digest == "sha256:sbom-layer":
            body = {"predicateType": "https://spdx.dev/Document/v2.3",
                    "subject": [{"digest": {"sha256": "d" * 64}}], "predicate": {}}
            return {"dsseEnvelope": {"payload": base64.b64encode(json.dumps(body).encode()).decode()}}
        assert digest == "sha256:layer"
        return bundle
    return get, blob


def verified_cli(args, **kwargs):
    assert args[:3] == ["gh", "attestation", "verify"], args
    assert args[3] == "oci://ghcr.io/arthurc02/skillhub-runtime-agent-sdk@" + DIGEST, args
    assert args[args.index("--repo") + 1] == "ArthurC02/SkillHub", args
    assert args[args.index("--signer-workflow") + 1] == "ArthurC02/SkillHub/.github/workflows/runtime-image.yml", args
    assert args[args.index("--format") + 1] == "json", args
    assert kwargs["timeout"] == 60 and kwargs["stderr"] == subprocess.DEVNULL
    bundle = json.loads(Path(args[args.index("--bundle") + 1]).read_text(encoding="utf-8"))
    payload = json.loads(base64.b64decode(bundle["dsseEnvelope"]["payload"]))
    assert args[args.index("--predicate-type") + 1] == payload["predicateType"]
    kwargs["stdout"].write(json.dumps([{"verificationResult": {"statement": payload}}]).encode())
    return subprocess.CompletedProcess(args, 0)


def run(get, blob, node_ref="", pinned=(True, "1 FROM line(s), all by digest"), verify=verified_cli):
    err = io.StringIO()
    env = {"SKILLHUB_SANDBOX_IMAGE": node_ref} if node_ref else {}
    with mock.patch.object(audit, "_token", lambda: "tok"), \
            mock.patch.object(audit, "_get", get), mock.patch.object(audit, "_blob", blob), \
            mock.patch.object(audit, "dockerfile_version", lambda: VERSION), \
            mock.patch.object(audit, "dockerfile_base_is_pinned", lambda: pinned), \
            mock.patch.object(audit, "datetime", FixedClock), \
            mock.patch("subprocess.run", side_effect=verify), \
            mock.patch.dict(audit.os.environ, env, clear=False), \
            contextlib.redirect_stderr(err):
        if not node_ref:
            audit.os.environ.pop("SKILLHUB_SANDBOX_IMAGE", None)
        rep = audit.audit()
    return [(r["id"], r["status"], r["detail"]) + ((r["expires_in_days"],) if "expires_in_days" in r else ())
            for r in rep.rows], err.getvalue()


def test_an_unsigned_bundle_cannot_supply_any_passing_attestation_check():
    def rejected(args, **kwargs):
        verified_cli(args, **kwargs)
        return subprocess.CompletedProcess(args, 1)
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX,
                           bundle=statement(summary={"fixable_critical_high": 0})), verify=rejected)
    assert [(row[0], row[1]) for row in rows[2:]] == [
        ("I-03", "UNKNOWN"), ("I-04", "UNKNOWN"), ("I-06", "UNKNOWN")], rows


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
    assert rows[2:] == [I03, ("I-04", audit.UNKNOWN, "could not verify the scan bundle"),
                        ("I-06", audit.UNKNOWN, "no verified scan statement")], rows
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle_error=True))
    assert rows[3] == ("I-04", audit.UNKNOWN, "could not verify the scan bundle"), rows


def test_an_attestation_about_other_bytes_cannot_supply_scan_evidence():
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle=statement(subject=OTHER)))
    assert rows[2:] == [I03, ("I-04", audit.UNKNOWN, "could not verify the scan bundle"),
                        ("I-06", audit.UNKNOWN, "no verified scan statement")], rows


def cli_output(args, kwargs, value):
    kwargs["stdout"].seek(0)
    kwargs["stdout"].truncate()
    kwargs["stdout"].write(value)
    return subprocess.CompletedProcess(args, 0)


def test_missing_or_timed_out_verification_is_unknown_and_never_discloses_stderr():
    for failure in [FileNotFoundError("private path"), subprocess.TimeoutExpired("gh", 60)]:
        def unavailable(args, **kwargs):
            raise failure
        rows, err = run(*registry(manifest=HEADERS, index=INDEX,
                                 bundle=statement(summary={"fixable_critical_high": 0})), verify=unavailable)
        assert [(row[0], row[1]) for row in rows[2:]] == [
            ("I-03", "UNKNOWN"), ("I-04", "UNKNOWN"), ("I-06", "UNKNOWN")], rows
        assert "private path" not in str(rows) + err


def test_malformed_verifier_output_does_not_become_evidence():
    cases = [b"not json", b"{}", b"[]", b"[{},{}]", b"[{}]",
             b'[{"verificationResult":{"statement":null}}]']
    for body in cases:
        def malformed(args, **kwargs):
            return cli_output(args, kwargs, body)
        rows, _ = run(*registry(manifest=HEADERS, index=INDEX,
                               bundle=statement(summary={"fixable_critical_high": 0})), verify=malformed)
        assert [(row[0], row[1]) for row in rows[2:]] == [
            ("I-03", "UNKNOWN"), ("I-04", "UNKNOWN"), ("I-06", "UNKNOWN")], (body, rows)


def test_the_scan_judgment_uses_verified_output_instead_of_the_raw_bundle():
    def different(args, **kwargs):
        result = verified_cli(args, **kwargs)
        if args[args.index("--predicate-type") + 1] == "https://in-toto.io/attestation/vulns/v0.1":
            kwargs["stdout"].seek(0)
            value = json.load(kwargs["stdout"])
            value[0]["verificationResult"]["statement"]["predicate"]["summary"]["fixable_critical_high"] = 2
            return cli_output(args, kwargs, json.dumps(value).encode())
        return result
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX,
                           bundle=statement(summary={"fixable_critical_high": 0})), verify=different)
    assert rows[-1][:2] == ("I-06", "FAIL"), rows
    assert "fixable Critical/High = 2" in rows[-1][2], rows


def test_the_verifier_output_must_bind_the_predicate_and_subject():
    for field in ["predicateType", "subject", "predicate"]:
        def wrong(args, **kwargs):
            verified_cli(args, **kwargs)
            kwargs["stdout"].seek(0)
            value = json.load(kwargs["stdout"])
            value[0]["verificationResult"]["statement"][field] = None
            return cli_output(args, kwargs, json.dumps(value).encode())
        rows, _ = run(*registry(manifest=HEADERS, index=INDEX,
                               bundle=statement(summary={"fixable_critical_high": 0})), verify=wrong)
        assert [(row[0], row[1]) for row in rows[2:]] == [
            ("I-03", "UNKNOWN"), ("I-04", "UNKNOWN"), ("I-06", "UNKNOWN")], (field, rows)


def test_verification_output_at_the_limit_is_read_but_one_more_byte_is_refused():
    for size, expected in [(16777216, "PASS"), (16777217, "UNKNOWN")]:
        def sized(args, **kwargs):
            verified_cli(args, **kwargs)
            kwargs["stdout"].seek(0)
            value = kwargs["stdout"].read()
            return cli_output(args, kwargs, value + b" " * (size - len(value)))
        rows, _ = run(*registry(manifest=HEADERS, index=INDEX,
                               bundle=statement(summary={"fixable_critical_high": 0})), verify=sized)
        assert [(row[0], row[1]) for row in rows[2:]] == [
            ("I-03", expected), ("I-04", expected), ("I-06", expected)], (size, rows)


def test_bundle_download_at_the_limit_is_read_but_one_more_byte_is_refused():
    for size, allowed in [(16777216, True), (16777217, False)]:
        response = io.BytesIO(b"{}" + b" " * (size - 2))
        with mock.patch.object(audit.urllib.request, "urlopen", return_value=response):
            try:
                value = audit._blob("sha256:fixture", "test-token")
            except ValueError:
                assert not allowed, "a bundle exactly at the limit must be readable"
            else:
                assert allowed and value == {}, "an oversized bundle must be refused"


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


def test_a_scan_expires_at_thirty_days_with_one_microsecond_on_either_side():
    cases = [
        ("2026-07-24T00:00:00.000001Z", "PASS", 0),
        ("2026-07-24T00:00:00Z", "FAIL", 0),
        ("2026-07-23T23:59:59.999999Z", "FAIL", -1),
        ("2026-08-23T00:00:00Z", "PASS", 30),
        ("2026-08-23T01:00:00+01:00", "PASS", 30),
    ]
    for finished, status, days in cases:
        assert audit.grade_scan_age(finished, NOW)[0] == status, finished
        assert audit.scan_expires_in_days(finished, NOW) == days, finished


def test_a_future_naive_or_non_string_scan_time_is_unknown_without_an_expiry():
    for finished in ["2026-08-23T00:00:00.000001Z", "2026-08-23T00:00:00", "2026-08-23",
                     "whenever", None, "", False, True, 0, 1, [], {}]:
        assert audit.grade_scan_age(finished, NOW)[0] == "UNKNOWN", finished
        assert audit.scan_expires_in_days(finished, NOW) is None, finished


def test_the_seven_day_warning_starts_at_the_boundary():
    for finished, warning in [("2026-07-31T00:00:00.000001Z", False),
                              ("2026-07-31T00:00:00Z", True),
                              ("2026-07-30T23:59:59.999999Z", True)]:
        status, detail = audit.grade_scan_age(finished, NOW)
        assert status == "PASS" and ("warning window" in detail) == warning, (finished, detail)


def test_only_a_nonnegative_integer_can_be_a_fixable_finding_count():
    for value, expected in [(False, "UNKNOWN"), (True, "UNKNOWN"), (0.0, "UNKNOWN"),
                            (-1, "UNKNOWN"), ("0", "UNKNOWN"), ([], "UNKNOWN"),
                            ({}, "UNKNOWN"), (0, "PASS"), (1, "FAIL")]:
        rows, _ = run(*registry(manifest=HEADERS, index=INDEX,
                               bundle=statement(summary={"fixable_critical_high": value})))
        assert rows[-1][:2] == ("I-06", expected), (value, rows)


def test_malformed_scan_metadata_or_summary_stays_unknown_without_crashing():
    for field, check in [("metadata", "I-04"), ("summary", "I-06")]:
        for value in [None, {}, [1], True, 1, "private malformed value"]:
            bundle = statement(summary={"fixable_critical_high": 0})
            body = json.loads(base64.b64decode(bundle["dsseEnvelope"]["payload"]))
            body["predicate"][field] = value
            bundle["dsseEnvelope"]["payload"] = base64.b64encode(json.dumps(body).encode()).decode()
            rows, err = run(*registry(manifest=HEADERS, index=INDEX, bundle=bundle))
            assert next(row[1] for row in rows if row[0] == check) == "UNKNOWN", (field, value, rows)
            assert "private malformed value" not in str(rows) + err


def test_an_unreadable_optional_severity_breakdown_does_not_replace_the_verified_count():
    for value in [None, [], [1], True, 1, "private malformed value"]:
        rows, err = run(*registry(manifest=HEADERS, index=INDEX,
                                 bundle=statement(summary={"fixable_critical_high": 0, "by_severity": value})))
        assert rows[-1][:2] == ("I-06", "PASS"), (value, rows)
        assert "private malformed value" not in str(rows) + err


def test_a_verified_matching_subject_does_not_crash_on_an_unrelated_malformed_subject():
    bundle = statement(summary={"fixable_critical_high": 0})
    body = json.loads(base64.b64decode(bundle["dsseEnvelope"]["payload"]))
    body["subject"].extend([None, {"digest": None}])
    bundle["dsseEnvelope"]["payload"] = base64.b64encode(json.dumps(body).encode()).decode()
    rows, _ = run(*registry(manifest=HEADERS, index=INDEX, bundle=bundle))
    assert rows[3][1] == "PASS" and rows[-1][1] == "PASS", rows


def test_first_value_skips_comments_and_blank_lines():
    with tempfile.TemporaryDirectory() as folder:
        path = Path(folder) / "value.txt"
        path.write_text("# a comment\n\n  \n1.2.3\nignored-second-line\n", encoding="utf-8")
        assert audit.first_value(str(path)) == "1.2.3"


def test_first_value_of_an_all_comment_file_is_empty():
    with tempfile.TemporaryDirectory() as folder:
        path = Path(folder) / "value.txt"
        path.write_text("# only comments\n# more\n", encoding="utf-8")
        assert audit.first_value(str(path)) == ""


def test_dockerfile_version_wraps_a_missing_file_and_keeps_its_cause():
    with mock.patch.object(audit, "DOCKERFILE", "/no/such/dockerfile"):
        try:
            audit.dockerfile_version()
        except audit.SetupError as exc:
            assert "cannot read" in str(exc), exc
            assert isinstance(exc.__cause__, OSError)
        else:
            raise AssertionError("a missing Dockerfile must raise SetupError")


def test_pinned_ip_wraps_a_missing_allowlist_and_keeps_its_cause():
    with mock.patch.object(audit, "ALLOWLIST_FILE", "/no/such/allowlist.yaml"):
        try:
            audit.pinned_ip()
        except audit.SetupError as exc:
            assert "cannot read" in str(exc), exc
            assert isinstance(exc.__cause__, OSError)
        else:
            raise AssertionError("a missing allow-list must raise SetupError")


def test_token_wraps_a_network_failure_and_keeps_its_cause():
    with mock.patch.object(
        audit.urllib.request, "urlopen",
        side_effect=urllib.error.URLError("no route to host"),
    ):
        try:
            audit._token()
        except audit.SetupError as exc:
            assert "could not get an anonymous pull token" in str(exc), exc
            assert isinstance(exc.__cause__, urllib.error.URLError)
        else:
            raise AssertionError("a network failure must raise SetupError")


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
