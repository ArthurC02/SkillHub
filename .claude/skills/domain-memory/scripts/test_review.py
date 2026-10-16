from __future__ import annotations

import base64
import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from urllib.error import HTTPError

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.attestations import (
    audit_attestations,
    carried_proposal_errors,
    commit_carries_proposal,
    github_api_path,
    github_commit_carries_proposal,
    next_page_url,
    verify_external_scm,
    verify_git_signed_commit,
    verify_scm,
    verify_scm_value,
)
from domain_registry.changes import (
    redraft_change_package,
    validate_change_package,
)
from domain_registry.hitl import (
    ObligationRun,
    attest_signed_commit,
    finalize_proposal,
    missing_attestation_message,
    record_test_result,
    record_approval,
    submit_proposal,
    supersede_proposal,
    verify_proposal,
)
from domain_registry.common import iso_timestamp
from domain_registry.git_hooks import governance_readiness
from domain_registry.revision import (
    registry_digest,
)
from domain_registry.store import read_document
from domain_registry.signing import SIGNING_KEY_ENV, init_signing_key
from domain_registry.updates import (
    apply_approved_updates,
)
from test_support import RegistryTestCase, amend_policy

BASE_REVISION = {"registry_digest": "sha256:" + "0" * 64, "observed_commit": None}
OTHER_REVISION = {"registry_digest": "sha256:" + "1" * 64, "observed_commit": None}
OUTSIDE_MESSAGE = "the Change Package must be inside the repository, so the attested commit can carry it"
ANOTHER_PROPOSAL_MESSAGE = (
    "Git commit carries another proposal or revision; attest a commit with this "
    "proposal_id and proposal_revision"
)
PULL_REQUEST_URL = "https://github.com/acme/skills/pull/7"
PULL_ENDPOINT = "https://api.github.com/repos/acme/skills/pulls/7"
CHECK_RUNS_ENDPOINT = f"https://api.github.com/repos/acme/skills/commits/{'a' * 40}/check-runs"


def github_serving(pull: object, reviews: object, checks: object):
    requested: list[str] = []

    def serve(url: str, token: str) -> tuple[object, None]:
        endpoint = url.partition("?")[0]
        requested.append(endpoint)
        if endpoint.endswith("/reviews"):
            served = reviews
        elif endpoint.endswith("/check-runs"):
            served = checks
        else:
            served = pull
        if isinstance(served, Exception):
            raise served
        return served, None

    return serve, requested


def github_paging(pages: dict[str, tuple[object, str | None]]):
    requested: list[str] = []

    def serve(url: str, token: str) -> tuple[object, str | None]:
        requested.append(url)
        return pages[url]

    return serve, requested


class ReviewTest(RegistryTestCase):
    def test_an_unrelated_apply_does_not_void_a_submitted_proposal(self) -> None:
        package = self.draft_package()
        self.commit_everything("draft")
        submit_proposal(package, self.repo / "memory", self.repo)
        submitted = self.proposal_in(package)
        self.edit_registry(
            "contexts.json",
            lambda value: value["contexts"].append(
                {"id": "billing", "name": "Billing", "responsibility": "Own invoices."}
            ),
        )
        self.commit_everything("another proposal applied")
        current = registry_digest(self.repo / "memory")
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["scm_attestation"] = {
            "provider": "github",
            "pull_request": "https://github.example/repo/pull/1",
            "checks_url": "https://github.example/repo/actions/runs/1",
            "commit": "a" * 40,
            "status": "approved",
            "proposal_revision": 1,
            "base_registry_revision": submitted["base_registry_revision"],
        }
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        verify_proposal(package, self.repo / "memory", self.repo)
        with patch("domain_registry.hitl.verify_external_scm", return_value=[]), \
                patch("domain_registry.hitl.github_commit_carries_proposal", return_value=[]):
            finalize_proposal(package, self.repo / "memory", self.repo)
        apply_approved_updates(package, self.repo / "memory", self.repo)
        self.assertEqual("applied", self.proposal_in(package)["status"])
        events = (self.repo / "memory" / "audit" / "events.jsonl").read_text(encoding="utf-8")
        applied = json.loads(events.splitlines()[-1])
        self.assertEqual(
            (current, submitted["base_registry_revision"]),
            (applied["from_revision"]["registry_digest"], applied["proposal_base_revision"]),
        )

    def test_scm_attestation_requires_the_same_revision(self) -> None:
        proposal = {
            "proposal_revision": 2,
            "base_registry_revision": {
                "registry_digest": "sha256:" + "0" * 64,
                "observed_commit": None,
            },
        }
        path = self.write_record(
            "attestation.json",
            {
                "provider": "github",
                "pull_request": "https://github.example/repo/pull/1",
                "checks_url": "https://github.example/repo/actions/runs/1",
                "commit": "a" * 40,
                "status": "approved",
                "proposal_revision": 2,
                "base_registry_revision": proposal["base_registry_revision"],
            },
        )
        self.assertEqual([], verify_scm(path, proposal))
        value = json.loads(path.read_text(encoding="utf-8"))
        value["proposal_revision"] = 1
        path.write_text(json.dumps(value), encoding="utf-8")
        self.assertEqual(["SCM attestation proposal revision does not match"], verify_scm(path, proposal))

    def test_each_scm_attestation_defect_is_named(self) -> None:
        valid = {
            "provider": "github",
            "pull_request": "https://github.example/repo/pull/1",
            "checks_url": "https://github.example/repo/actions/runs/1",
            "commit": "a" * 40,
            "status": "approved",
            "proposal_revision": 1,
            "base_registry_revision": BASE_REVISION,
        }
        proposal = {"proposal_revision": 1, "base_registry_revision": BASE_REVISION}

        def without(key: str) -> dict:
            return {name: value for name, value in valid.items() if name != key}

        cases = [
            ("complete", valid, []),
            ("provider missing", without("provider"),
             ["SCM attestation requires provider",
              "SCM attestation provider must be github or git-signed-commit"]),
            ("commit missing", without("commit"),
             ["SCM attestation requires commit", "SCM attestation requires a pinned 40-character commit"]),
            ("status missing", without("status"),
             ["SCM attestation requires status", "SCM attestation status must be approved"]),
            ("status pending", {**valid, "status": "pending"}, ["SCM attestation status must be approved"]),
            ("pull request over http", {**valid, "pull_request": "http://github.example/repo/pull/1"},
             ["SCM attestation requires an external pull_request URL"]),
            ("pull request missing", without("pull_request"),
             ["SCM attestation requires an external pull_request URL"]),
            ("checks url over ftp", {**valid, "checks_url": "ftp://github.example/runs/1"},
             ["SCM attestation requires an external checks_url URL"]),
            ("commit one character short", {**valid, "commit": "a" * 39},
             ["SCM attestation requires a pinned 40-character commit"]),
            ("commit one character long", {**valid, "commit": "a" * 41},
             ["SCM attestation requires a pinned 40-character commit"]),
            ("commit in capitals", {**valid, "commit": "A" * 40},
             ["SCM attestation requires a pinned 40-character commit"]),
            ("commit not hex", {**valid, "commit": "g" * 40},
             ["SCM attestation requires a pinned 40-character commit"]),
            ("proposal revision differs", {**valid, "proposal_revision": 2},
             ["SCM attestation proposal revision does not match"]),
            ("base registry revision differs", {**valid, "base_registry_revision": OTHER_REVISION},
             ["SCM attestation Registry revision does not match"]),
        ]
        for label, value, expected in cases:
            with self.subTest(label):
                self.assertEqual(expected, verify_scm_value(value, proposal))

    def test_a_github_pull_request_url_maps_to_its_api_path(self) -> None:
        self.assertEqual("repos/acme/skills/pulls/7", github_api_path(PULL_REQUEST_URL))

    def test_each_malformed_github_pull_request_url_is_named(self) -> None:
        not_github = "SCM attestation pull_request must be a GitHub pull request URL"
        not_one = "SCM attestation pull_request must identify one pull request"
        cases = [
            ("other host", "https://gitlab.com/acme/skills/pull/7", not_github),
            ("plain http", "http://github.com/acme/skills/pull/7", not_github),
            ("one segment short", "https://github.com/acme/skills/pull", not_one),
            ("one segment long", "https://github.com/acme/skills/pull/7/files", not_one),
            ("an issue, not a pull request", "https://github.com/acme/skills/issues/7", not_one),
            ("number not numeric", "https://github.com/acme/skills/pull/seven", not_one),
        ]
        for label, url, message in cases:
            with self.subTest(label):
                with self.assertRaises(ValueError) as refused:
                    github_api_path(url)
                self.assertEqual(message, str(refused.exception))

    def test_scm_attestation_rejects_a_local_claim_of_review(self) -> None:
        proposal = {"proposal_revision": 1, "base_registry_revision": {"registry_digest": "sha256:" + "0" * 64, "observed_commit": None}}
        path = self.write_record("attestation.json", {"provider": "git", "pull_request": "https://host.example/pull/1", "checks_url": "https://host.example/checks/1", "commit": "a" * 40, "status": "approved", "proposal_revision": 1, "base_registry_revision": proposal["base_registry_revision"]})
        errors = verify_scm(path, proposal)
        self.assertTrue(
            any("must be github or git-signed-commit" in error for error in errors), errors
        )

    def test_external_scm_verification_requires_merged_approved_green_github_pr(self) -> None:
        attestation = {"provider": "github", "pull_request": PULL_REQUEST_URL, "commit": "a" * 40}
        serve, requested = github_serving(
            {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}},
            [{"user": {"login": "reviewer"}, "state": "APPROVED"}],
            {"check_runs": [{"status": "completed", "conclusion": "success"}]},
        )
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            with patch("domain_registry.attestations.github_page", side_effect=serve):
                self.assertEqual([], verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"))
        self.assertEqual(
            [PULL_ENDPOINT, PULL_ENDPOINT + "/reviews", CHECK_RUNS_ENDPOINT], requested
        )

    def test_external_scm_verification_rejects_unsuccessful_checks(self) -> None:
        attestation = {"provider": "github", "pull_request": PULL_REQUEST_URL, "commit": "a" * 40}
        serve, _ = github_serving(
            {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}},
            [{"user": {"login": "reviewer"}, "state": "APPROVED"}],
            {"check_runs": [{"status": "completed", "conclusion": "failure"}]},
        )
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            with patch("domain_registry.attestations.github_page", side_effect=serve):
                self.assertEqual(
                    ["SCM attested commit has incomplete or unsuccessful checks"],
                    verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"),
                )

    def test_each_external_scm_defect_is_named(self) -> None:
        attestation = {"provider": "github", "pull_request": PULL_REQUEST_URL, "commit": "a" * 40}
        merged = {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}}
        approved = [{"user": {"login": "reviewer"}, "state": "APPROVED"}]
        green = {"check_runs": [{"status": "completed", "conclusion": "success"}]}
        cases = [
            ("open", ({**merged, "state": "open"}, approved, green), "SCM pull request is not merged"),
            ("unmerged close", ({**merged, "merged_at": None}, approved, green), "SCM pull request is not merged"),
            ("other head", ({**merged, "head": {"sha": "b" * 40}}, approved, green),
             "SCM pull request head does not match the attested commit"),
            ("no reviews", (merged, [], green), "SCM pull request has no current approval"),
            ("approval withdrawn",
             (merged, [*approved, {"user": {"login": "reviewer"}, "state": "CHANGES_REQUESTED"}, 7], green),
             "SCM pull request has no current approval"),
            ("no checks", (merged, approved, {"check_runs": []}), "SCM attested commit has no check runs"),
            ("running check", (merged, approved, {"check_runs": [{"status": "in_progress", "conclusion": None}]}),
             "SCM attested commit has incomplete or unsuccessful checks"),
            ("unreachable", (OSError("network down"), None, None), "SCM governance verification failed: network down"),
        ]
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            for label, responses, message in cases:
                serve, _ = github_serving(*responses)
                with self.subTest(label), patch("domain_registry.attestations.github_page", side_effect=serve):
                    self.assertEqual([message], verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"))
            with self.subTest("another provider"):
                self.assertEqual(
                    ["SCM governance verification currently supports GitHub only"],
                    verify_external_scm({**attestation, "provider": "gitlab"}, "DOMAIN_MEMORY_SCM_TOKEN"),
                )
        with self.subTest("no token"), patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": ""}):
            self.assertEqual(
                ["SCM governance verification requires DOMAIN_MEMORY_SCM_TOKEN"],
                verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"),
            )

    def test_reviews_and_check_runs_are_read_past_the_first_page(self) -> None:
        attestation = {"provider": "github", "pull_request": PULL_REQUEST_URL, "commit": "a" * 40}
        reviews = PULL_ENDPOINT + "/reviews?per_page=100"
        checks = CHECK_RUNS_ENDPOINT + "?per_page=100"
        approved = [{"user": {"login": "reviewer"}, "state": "APPROVED"}]
        withdrawn = [{"user": {"login": "reviewer"}, "state": "CHANGES_REQUESTED"}]
        green = {"check_runs": [{"status": "completed", "conclusion": "success"}]}
        red = {"check_runs": [{"status": "completed", "conclusion": "failure"}]}
        merged = ({"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}}, None)
        cases = [
            ("approval withdrawn on page two", {
                PULL_ENDPOINT: merged,
                reviews: (approved, reviews + "&page=2"),
                reviews + "&page=2": (withdrawn, None),
                checks: (green, None),
            }, "SCM pull request has no current approval"),
            ("failed check on page two", {
                PULL_ENDPOINT: merged,
                reviews: (approved, None),
                checks: (green, checks + "&page=2"),
                checks + "&page=2": (red, None),
            }, "SCM attested commit has incomplete or unsuccessful checks"),
            ("a page that is not a list", {
                PULL_ENDPOINT: merged,
                reviews: ({"message": "rate limited"}, None),
            }, f"SCM governance verification failed: the provider returned no list at {PULL_ENDPOINT}/reviews"),
        ]
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            for label, pages, message in cases:
                serve, requested = github_paging(pages)
                with self.subTest(label), patch("domain_registry.attestations.github_page", side_effect=serve):
                    self.assertEqual([message], verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"))
                    self.assertEqual(list(pages), requested)

    def test_the_next_page_is_taken_from_the_link_header(self) -> None:
        cases = [
            ("next and last", '<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=5>; rel="last"',
             "https://api.github.com/x?page=2"),
            ("next listed after prev", '<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?page=3>; rel="next"',
             "https://api.github.com/x?page=3"),
            ("last page", '<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?page=1>; rel="first"', None),
            ("no header", None, None),
        ]
        for label, header, expected in cases:
            with self.subTest(label):
                self.assertEqual(expected, next_page_url(header))

    def test_external_scm_verification_can_make_ci_optional(self) -> None:
        attestation = {"provider": "github", "pull_request": PULL_REQUEST_URL, "commit": "a" * 40}
        serve, requested = github_serving(
            {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}},
            [{"user": {"login": "reviewer"}, "state": "APPROVED"}],
            OSError("check-runs must not be requested"),
        )
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            with patch("domain_registry.attestations.github_page", side_effect=serve):
                self.assertEqual([], verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN", False))
        self.assertEqual([PULL_ENDPOINT, PULL_ENDPOINT + "/reviews"], requested)

    def test_an_attesting_commit_must_carry_this_proposal_revision(self) -> None:
        earlier = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True
        ).stdout.strip()
        package = self.draft_package()
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        carrying = self.commit_everything("adds the Change Package")
        self.assertEqual([], commit_carries_proposal(self.repo, carrying, package, proposal))
        [missing] = commit_carries_proposal(self.repo, earlier, package, proposal)
        self.assertIn("does not carry this Change Package at proposal", missing)
        [other] = commit_carries_proposal(
            self.repo, carrying, package, {**proposal, "proposal_revision": 2}
        )
        self.assertIn("another proposal or revision", other)

    def test_each_committed_proposal_that_is_not_this_one_is_named(self) -> None:
        proposal = {"proposal_id": "PRO-1", "proposal_revision": 1}
        cases = [
            ("same identity", json.dumps(proposal), []),
            ("different proposal id", json.dumps({**proposal, "proposal_id": "PRO-2"}), [ANOTHER_PROPOSAL_MESSAGE]),
            ("different revision", json.dumps({**proposal, "proposal_revision": 2}), [ANOTHER_PROPOSAL_MESSAGE]),
            ("a list, not an object", "[1, 2]", [ANOTHER_PROPOSAL_MESSAGE]),
            ("a string, not an object", '"PRO-1"', [ANOTHER_PROPOSAL_MESSAGE]),
            ("unparseable", "{not json", ["Git commit carries an unreadable domain-change-proposal.json"]),
        ]
        for label, committed_text, expected in cases:
            with self.subTest(label):
                self.assertEqual(expected, carried_proposal_errors(committed_text, "proposal", proposal))

    def test_a_package_outside_the_repository_cannot_be_carried_by_a_commit(self) -> None:
        proposal = {"proposal_id": "PRO-1", "proposal_revision": 1}
        with tempfile.TemporaryDirectory() as elsewhere:
            self.assertEqual(
                [OUTSIDE_MESSAGE],
                commit_carries_proposal(self.repo, "HEAD", Path(elsewhere), proposal),
            )

    def test_a_package_outside_the_repository_cannot_be_carried_by_a_pull_request(self) -> None:
        proposal = {"proposal_id": "PRO-1", "proposal_revision": 1}
        attestation = {"pull_request": PULL_REQUEST_URL, "commit": "c" * 40}
        with tempfile.TemporaryDirectory() as elsewhere, patch.dict("os.environ", {"GITHUB_TOKEN": ""}):
            self.assertEqual(
                [OUTSIDE_MESSAGE],
                github_commit_carries_proposal(attestation, "GITHUB_TOKEN", self.repo, Path(elsewhere), proposal),
            )

    def test_a_signed_authorized_commit_that_changes_only_source_code_is_refused(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            key = init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        commit = self.sign_a_commit_changing("src/x.py")
        self.assertEqual(
            ["Git commit does not change Domain Memory files"],
            verify_git_signed_commit(
                {"commit": commit}, self.repo, self.repo / "docs" / "domain-memory", key["authorized_signers"]
            ),
        )

    def test_a_signed_authorized_commit_that_changes_only_a_sibling_of_the_registry_directory_is_refused(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            key = init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        commit = self.sign_a_commit_changing("docs/domain-memory-extra/x")
        self.assertEqual(
            ["Git commit does not change Domain Memory files"],
            verify_git_signed_commit(
                {"commit": commit}, self.repo, self.repo / "docs" / "domain-memory", key["authorized_signers"]
            ),
        )

    def sign_a_commit_changing(self, relative: str) -> str:
        path = self.repo / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(relative, encoding="utf-8")
        subprocess.run(["git", "add", relative], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "-c", "user.name=Probe", "-c", "user.email=probe@example.com",
             "commit", "-qS", "-m", f"change {relative}"],
            cwd=self.repo, check=True,
        )
        return subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True,
        ).stdout.strip()

    def test_attesting_a_signed_commit_that_carries_the_package_lets_it_finalize(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            key = init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["review_governance"].update(
            verifier="git-signed-commit", trigger="git-push", authorized_signers=[key["fingerprint"]]
        )
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        unsigned = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True
        ).stdout.strip()
        signed_before = self.commit_everything("signs the policy before the package", signed=True)
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        with self.assertRaisesRegex(ValueError, "cannot attest this proposal: Git commit signature is invalid"):
            attest_signed_commit(package, self.repo / "memory", self.repo, unsigned)
        with self.assertRaisesRegex(ValueError, "does not carry this Change Package"):
            attest_signed_commit(package, self.repo / "memory", self.repo, signed_before)
        signed = self.commit_everything("adds the Change Package", signed=True)
        self.assertEqual(signed, attest_signed_commit(package, self.repo / "memory", self.repo, signed[:12]))
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual(
            {"provider": "git-signed-commit", "commit": signed, "status": "approved",
             "proposal_revision": 1, "base_registry_revision": proposal["base_registry_revision"]},
            evidence["scm_attestation"],
        )
        verify_proposal(package, self.repo / "memory", self.repo)
        finalize_proposal(package, self.repo / "memory", self.repo)
        finalized = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("approved", finalized["status"])

    def test_attesting_needs_the_signed_commit_verifier_and_a_submitted_proposal(self) -> None:
        package = self.draft_package()
        with self.assertRaisesRegex(ValueError, "this policy uses github-pr"):
            attest_signed_commit(package, self.repo / "memory", self.repo, "HEAD")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["review_governance"].update(
            verifier="git-signed-commit", trigger="git-push", authorized_signers=["probe@example.com"]
        )
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "submit the proposal first"):
            attest_signed_commit(package, self.repo / "memory", self.repo, "HEAD")

    def test_a_passing_run_replaces_the_obligation_result_with_its_output_digest(self) -> None:
        package = self.draft_package()
        record_test_result(
            package, self.repo / "memory", self.repo,
            ObligationRun("OB-1", self.run_writing("ok"), "unit"), 60,
        )
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual(
            ("passed", 0, "unit", "sha256:" + hashlib.sha256(b"ok").hexdigest()),
            (result["status"], result["exit_code"], result["command_profile"], result["output_sha256"]),
        )

    def test_a_recorded_command_passes_shell_characters_through_unchanged(self) -> None:
        package = self.draft_package()
        echo = "import sys; sys.stdout.write(sys.argv[1])"
        record_test_result(
            package, self.repo / "memory", self.repo,
            ObligationRun("OB-1", f'"{sys.executable}" -c "{echo}" ^(A|B)$', "unit"), 60,
        )
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual("sha256:" + hashlib.sha256(b"^(A|B)$").hexdigest(), result["output_sha256"])

    def test_a_command_that_cannot_start_records_nothing(self) -> None:
        package = self.draft_package()
        before = (package / "evidence-bundle.json").read_bytes()
        with self.assertRaisesRegex(ValueError, "could not start: .*runs as written, with no shell and no aliases"):
            record_test_result(
                package, self.repo / "memory", self.repo,
                ObligationRun("OB-1", "no-such-program-for-domain-memory --version", "unit"), 60,
            )
        self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())

    def test_a_policy_without_approved_profiles_accepts_any_profile(self) -> None:
        package = self.draft_package()
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["approved_command_profiles"] = []
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        record_test_result(
            package, self.repo / "memory", self.repo,
            ObligationRun("OB-1", self.run_writing("ok"), "release"), 60,
        )
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual("release", result["command_profile"])

    def test_a_failing_run_and_an_unknown_obligation_or_profile_record_nothing(self) -> None:
        package = self.draft_package()
        before = (package / "evidence-bundle.json").read_bytes()
        cases = [
            ("OB-1", self.run_writing("broken", 3), "unit", "exited 3; only a passing run is recorded"),
            ("OB-9", self.run_writing("ok"), "unit", "no obligation OB-9"),
            ("OB-1", self.run_writing("ok"), "release", "release is not approved"),
        ]
        for obligation, command, profile, refusal in cases:
            with self.subTest(refusal=refusal):
                with self.assertRaisesRegex(ValueError, refusal):
                    record_test_result(
                        package, self.repo / "memory", self.repo,
                        ObligationRun(obligation, command, profile), 60,
                    )
                self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())

    def test_the_attestation_audit_tells_carried_from_not_carried_from_unconfirmed(self) -> None:
        earlier = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True
        ).stdout.strip()
        self.changed_package("carried", "applied", None)
        self.changed_package("draft", "draft", None)
        carrying = self.commit_everything("adds the carried package")
        evidence_path = self.repo / "changes" / "carried" / "evidence-bundle.json"
        evidence_path.write_text(
            json.dumps({"scm_attestation": {"provider": "git-signed-commit", "commit": carrying}}),
            encoding="utf-8",
        )
        self.changed_package("reused", "approved", {"provider": "git-signed-commit", "commit": earlier})
        self.changed_package("unfetched", "applied", {"provider": "git-signed-commit", "commit": "f" * 40})
        self.changed_package("remote", "applied", {
            "provider": "github", "commit": "e" * 40,
            "pull_request": "https://github.com/acme/shop/pull/7",
        })
        self.changed_package("unattested", "applied", None)
        with patch.dict("os.environ", {"GITHUB_TOKEN": ""}):
            findings = audit_attestations(self.repo, self.repo / "changes", "GITHUB_TOKEN")
        self.assertEqual(
            [("carried", "carried"), ("remote", "unconfirmed"), ("reused", "not carried"),
             ("unattested", "not carried"), ("unfetched", "unconfirmed")],
            [(finding["proposal_id"], finding["verdict"]) for finding in findings],
        )

    def test_the_attestation_audit_does_not_read_a_commit_split_across_lines(self) -> None:
        self.changed_package("unattested", "applied", None)
        added = self.commit_everything("adds the unattested package")
        self.changed_package("split", "applied", {
            "provider": "git-signed-commit", "commit": f"{added}\n{'f' * 40}",
        })
        findings = audit_attestations(self.repo, self.repo / "changes", "GITHUB_TOKEN")
        self.assertEqual(
            [("split", "unconfirmed"), ("unattested", "not carried")],
            [(finding["proposal_id"], finding["verdict"]) for finding in findings],
        )

    def test_the_attestation_audit_outside_a_git_repository_confirms_nothing(self) -> None:
        with tempfile.TemporaryDirectory() as plain:
            root = Path(plain)
            package = root / "changes" / "attested"
            package.mkdir(parents=True)
            (package / "domain-change-proposal.json").write_text(
                json.dumps({"proposal_id": "attested", "status": "applied"}), encoding="utf-8"
            )
            (package / "evidence-bundle.json").write_text(
                json.dumps({"scm_attestation": {"provider": "git-signed-commit", "commit": "a" * 40}}),
                encoding="utf-8",
            )
            [finding] = audit_attestations(root, root / "changes", "GITHUB_TOKEN")
        self.assertEqual(finding["verdict"], "unconfirmed")

    def test_finalizing_refuses_a_merged_pull_request_that_does_not_carry_the_package(self) -> None:
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        submitted = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["scm_attestation"] = {
            "provider": "github", "pull_request": "https://github.com/acme/shop/pull/7",
            "checks_url": "https://github.com/acme/shop/actions/runs/1", "commit": "a" * 40,
            "status": "approved", "proposal_revision": 1,
            "base_registry_revision": submitted["base_registry_revision"],
        }
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        verify_proposal(package, self.repo / "memory", self.repo)
        absent = ["Git commit does not carry this Change Package at proposal"]
        with patch("domain_registry.hitl.verify_external_scm", return_value=[]), \
                patch("domain_registry.hitl.github_commit_carries_proposal", return_value=absent):
            with self.assertRaisesRegex(ValueError, "not externally verified: Git commit does not carry"):
                finalize_proposal(package, self.repo / "memory", self.repo)
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("verified", proposal["status"])

    def test_a_github_pull_request_head_must_carry_this_proposal_revision(self) -> None:
        package = self.draft_package()
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        attestation = {"pull_request": "https://github.com/acme/shop/pull/7", "commit": "c" * 40}

        def served(document: dict) -> dict:
            encoded = base64.b64encode(json.dumps(document).encode("utf-8")).decode("ascii")
            return {"encoding": "base64", "content": encoded}

        def not_found(*_: object) -> None:
            raise HTTPError("url", 404, "Not Found", {}, None)

        def unavailable(*_: object) -> None:
            raise HTTPError("url", 503, "Service Unavailable", {}, None)

        cases = [
            ("carried", lambda *_: served(proposal), None),
            ("other revision", lambda *_: served({**proposal, "proposal_revision": 2}), "another proposal or revision"),
            ("absent", not_found, "does not carry this Change Package at proposal"),
            ("unavailable", unavailable, "verification failed: HTTP Error 503"),
        ]
        for name, provider, refusal in cases:
            with self.subTest(name=name), patch.dict("os.environ", {"GITHUB_TOKEN": "t"}), \
                    patch("domain_registry.attestations.github_json", side_effect=provider) as fetch:
                errors = github_commit_carries_proposal(
                    attestation, "GITHUB_TOKEN", self.repo, package, proposal
                )
                self.assertEqual(
                    "https://api.github.com/repos/acme/shop/contents/proposal/domain-change-proposal.json"
                    f"?ref={'c' * 40}",
                    fetch.call_args.args[0],
                )
                if refusal is None:
                    self.assertEqual([], errors)
                else:
                    [error] = errors
                    self.assertIn(refusal, error)
        with patch.dict("os.environ", {"GITHUB_TOKEN": ""}):
            self.assertEqual(
                ["SCM governance verification requires GITHUB_TOKEN"],
                github_commit_carries_proposal(attestation, "GITHUB_TOKEN", self.repo, package, proposal),
            )

    def test_finalizing_refuses_an_attested_commit_that_predates_the_change_package(self) -> None:
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["review_governance"].update(
            verifier="git-signed-commit", trigger="git-push", authorized_signers=["probe@example.com"]
        )
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        earlier = self.commit_everything("an earlier Domain Memory commit")
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))

        def attest(commit: str) -> None:
            evidence_path = package / "evidence-bundle.json"
            evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
            evidence["scm_attestation"] = {
                "provider": "git-signed-commit", "commit": commit, "status": "approved",
                "proposal_revision": 1, "base_registry_revision": proposal["base_registry_revision"],
            }
            evidence_path.write_text(json.dumps(evidence), encoding="utf-8")

        attest(earlier)
        verify_proposal(package, self.repo / "memory", self.repo)
        with patch("domain_registry.hitl.verify_git_signed_commit", return_value=[]):
            with self.assertRaisesRegex(ValueError, "does not carry this Change Package"):
                finalize_proposal(package, self.repo / "memory", self.repo)
            attest(self.commit_everything("adds the verified Change Package"))
            finalize_proposal(package, self.repo / "memory", self.repo)
        finalized = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("approved", finalized["status"])

    def test_a_generated_signing_key_signs_a_commit_git_can_verify(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            result = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        self.assertEqual("generated", result["source"])
        self.assertTrue(result["fingerprint"].startswith("SHA256:"))
        self.assertEqual(
            ["probe@example.com", result["fingerprint"]], result["authorized_signers"]
        )
        commit = self.sign_a_domain_memory_commit("signed by the generated key")
        for authorized in (["probe@example.com"], [result["fingerprint"]]):
            with self.subTest(authorized=authorized[0]):
                self.assertEqual(
                    [],
                    verify_git_signed_commit(
                        {"commit": commit}, self.repo, self.repo / "docs" / "domain-memory",
                        authorized,
                    ),
                )

    def test_a_rejected_commit_says_whether_it_lost_its_signature_or_fails_to_verify(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        registry_root = self.repo / "docs" / "domain-memory"
        signed = self.sign_a_domain_memory_commit("signed")
        (registry_root / "note.txt").write_text("unsigned", encoding="utf-8")
        unsigned = self.commit_everything("unsigned")
        [lost] = verify_git_signed_commit({"commit": unsigned}, self.repo, registry_root, ["probe@example.com"])
        self.assertIn(f"Git commit signature is invalid: {unsigned} carries no signature", lost)
        self.assertIn("git pull --no-rebase", lost)
        subprocess.run(["git", "config", "--unset", "gpg.ssh.allowedSignersFile"], cwd=self.repo, check=True)
        [failing] = verify_git_signed_commit({"commit": signed}, self.repo, registry_root, ["probe@example.com"])
        self.assertRegex(
            failing,
            rf"^Git commit signature is invalid: {signed} is signed, but the signature does not verify \(.*allowedSignersFile.*\)$",
        )

    def test_a_signature_program_that_cannot_run_is_reported_as_not_checked(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        signed = self.sign_a_domain_memory_commit("signed")
        subprocess.run(["git", "config", "gpg.ssh.program", "no-such-keygen"], cwd=self.repo, check=True)
        [unchecked] = verify_git_signed_commit(
            {"commit": signed}, self.repo, self.repo / "docs" / "domain-memory", ["probe@example.com"]
        )
        self.assertRegex(
            unchecked,
            r"^Git commit signature was not checked: cannot (spawn|run) no-such-keygen: No such file or directory\. "
            r"Install the signing program, or point gpg\.ssh\.program or gpg\.program at it, and verify again$",
        )

    def test_the_same_key_rebuilds_from_the_environment(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            first = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        material = (self.repo / "keys" / "signing-key").read_text(encoding="utf-8")
        elsewhere = self.repo / "elsewhere"
        subprocess.run(["git", "init", "-q", str(elsewhere)], check=True)
        with patch.dict("os.environ", {SIGNING_KEY_ENV: material}):
            second = init_signing_key(
                elsewhere, "probe@example.com", elsewhere / "keys" / "signing-key"
            )
        self.assertEqual("environment", second["source"])
        self.assertEqual(first["fingerprint"], second["fingerprint"])

    def test_key_material_with_crlf_line_endings_and_no_final_newline_rebuilds(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            first = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        material = (self.repo / "keys" / "signing-key").read_text(encoding="utf-8")
        pasted = material.rstrip("\n").replace("\n", "\r\n")
        elsewhere = self.repo / "elsewhere"
        subprocess.run(["git", "init", "-q", str(elsewhere)], check=True)
        with patch.dict("os.environ", {SIGNING_KEY_ENV: pasted}):
            second = init_signing_key(
                elsewhere, "probe@example.com", elsewhere / "keys" / "signing-key"
            )
        self.assertEqual("environment", second["source"])
        self.assertEqual(first["fingerprint"], second["fingerprint"])

    def test_unreadable_key_material_leaves_no_key_behind(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        label = "PRIVATE KEY"
        material = f"-----BEGIN OPENSSH {label}-----\nnot a key\n"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: material}):
            with self.assertRaisesRegex(ValueError, "readable OpenSSH private key"):
                init_signing_key(self.repo, "probe@example.com", key_file)
        self.assertFalse(key_file.exists())

    def test_the_reported_result_never_carries_private_key_material(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            result = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        self.assertNotIn("PRIVATE KEY", json.dumps(result))

    def test_no_line_of_the_private_key_body_appears_in_the_reported_result(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            result = init_signing_key(self.repo, "probe@example.com", key_file)
        body = [
            line for line in key_file.read_text(encoding="utf-8").splitlines()
            if line and not line.startswith("-----")
        ]
        serialized = json.dumps(result)
        self.assertTrue(body)
        self.assertEqual([], [line for line in body if line in serialized])

    def test_a_second_call_reuses_the_existing_key(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            first = init_signing_key(self.repo, "probe@example.com", key_file)
            second = init_signing_key(self.repo, "probe@example.com", key_file)
        self.assertEqual(
            ("generated", "reused", first["fingerprint"]),
            (first["source"], second["source"], second["fingerprint"]),
        )

    def test_a_blank_principal_is_refused_and_creates_no_key(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        for principal in ("", "   ", None):
            with self.subTest(principal=principal), patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
                with self.assertRaises(ValueError) as refused:
                    init_signing_key(self.repo, principal, key_file)
                self.assertEqual(
                    "a signing key needs a principal: the identity an approval is attributed to",
                    str(refused.exception),
                )
                self.assertFalse(key_file.exists())

    def test_force_without_the_environment_key_is_refused_and_leaves_the_key_untouched(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            init_signing_key(self.repo, "probe@example.com", key_file)
            before = key_file.read_bytes()
            with self.assertRaises(ValueError) as refused:
                init_signing_key(self.repo, "probe@example.com", key_file, force=True)
        self.assertEqual(
            "--force replaces a key from DOMAIN_MEMORY_SIGNING_KEY, and DOMAIN_MEMORY_SIGNING_KEY "
            f"is not set; delete {key_file} yourself if you mean to discard it",
            str(refused.exception),
        )
        self.assertEqual(before, key_file.read_bytes())

    def test_an_ssh_signed_commit_is_authorized_by_principal_or_fingerprint(self) -> None:
        attestation = {"commit": "a" * 40}
        signed = subprocess.CompletedProcess(
            [],
            0,
            'Good "git" signature for arthur@example.com with ED25519 key '
            "SHA256:HMt6j4Woy7Beu896pq7/ApyBUR9qPWi/JJXlPBOVvnk\n",
            "",
        )
        changed = subprocess.CompletedProcess(
            [], 0, "docs/domain-memory/registry/rules.json\n", ""
        )
        registry_root = self.repo / "docs" / "domain-memory"
        for authorized in (
            ["arthur@example.com"],
            ["SHA256:HMt6j4Woy7Beu896pq7/ApyBUR9qPWi/JJXlPBOVvnk"],
        ):
            with self.subTest(authorized=authorized[0]):
                with patch(
                    "domain_registry.attestations.subprocess.run",
                    side_effect=[signed, changed],
                ):
                    self.assertEqual(
                        [],
                        verify_git_signed_commit(
                            attestation, self.repo, registry_root, authorized
                        ),
                    )
        with patch(
            "domain_registry.attestations.subprocess.run", side_effect=[signed, changed]
        ):
            self.assertEqual(
                ["Git commit signer is not authorized by Domain Memory policy"],
                verify_git_signed_commit(
                    attestation, self.repo, registry_root, ["someone.else@example.com"]
                ),
            )

    def test_a_raw_gnupg_signer_is_accepted_when_listed_and_refused_when_not(self) -> None:
        attestation = {"commit": "a" * 40}
        verified = subprocess.CompletedProcess(
            [], 0, "[GNUPG:] VALIDSIG ABCD", ""
        )
        changed = subprocess.CompletedProcess([], 0, "docs/domain-memory/registry/rules.json\n", "")
        with patch("domain_registry.attestations.subprocess.run", side_effect=[verified, changed]):
            self.assertEqual(
                [],
                verify_git_signed_commit(
                    attestation, self.repo, self.repo / "docs" / "domain-memory", ["ABCD"]
                ),
            )
        with patch("domain_registry.attestations.subprocess.run", return_value=verified):
            self.assertEqual(
                ["Git commit signer is not authorized by Domain Memory policy"],
                verify_git_signed_commit(
                    attestation, self.repo, self.repo / "docs" / "domain-memory", ["OTHER"]
                ),
            )

    def test_a_new_branch_skips_commits_the_remote_already_has(self) -> None:
        hook, _ = self.push_fixture()
        head = self.commit_file("elsewhere/readme.txt")
        result = self.run_pre_push(
            hook, f"refs/heads/feature {head} refs/heads/feature {'0' * 40}\n"
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

    def test_a_new_branch_still_verifies_its_own_unsigned_memory_commit(self) -> None:
        hook, _ = self.push_fixture()
        head = self.commit_file("memory/registry/extra.json")
        result = self.run_pre_push(
            hook, f"refs/heads/feature {head} refs/heads/feature {'0' * 40}\n"
        )
        self.assertNotEqual(0, result.returncode)
        self.assertIn("Git commit signature is invalid", result.stdout + result.stderr)

    def test_the_pre_push_hook_runs_python3_where_python_does_not_work(self) -> None:
        result = self.pushed_with(self.interpreters_on_path(python=False, python3=True))
        self.assertIn("Git commit signature is invalid", result.stdout + result.stderr)

    def test_the_pre_push_hook_runs_python_where_python3_does_not_work(self) -> None:
        result = self.pushed_with(self.interpreters_on_path(python=True, python3=False))
        self.assertIn("Git commit signature is invalid", result.stdout + result.stderr)

    def test_a_push_without_memory_commits_needs_no_interpreter(self) -> None:
        environment = self.interpreters_on_path(python=False, python3=False)
        hook, _ = self.push_fixture()
        head = self.commit_file("elsewhere/readme.txt")
        result = subprocess.run(
            ["sh", str(hook), "origin", "unused"], cwd=self.repo, env=environment,
            input=f"refs/heads/feature {head} refs/heads/feature {'0' * 40}\n",
            capture_output=True, text=True, check=False,
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

    def test_the_pre_push_hook_names_a_missing_interpreter(self) -> None:
        result = self.pushed_with(self.interpreters_on_path(python=False, python3=False))
        self.assertEqual(1, result.returncode)
        self.assertIn("no Python 3.10 or later on PATH as python3 or python", result.stderr)

    def test_an_existing_branch_checks_only_the_range_after_the_remote_tip(self) -> None:
        hook, remote_tip = self.push_fixture()
        head = self.commit_file("elsewhere/readme.txt")
        result = self.run_pre_push(
            hook, f"refs/heads/main {head} refs/heads/main {remote_tip}\n"
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

    def test_local_working_memory_needs_no_governance_setup(self) -> None:
        amend_policy(self.repo / "memory", "review_mode", "local-draft-only", "Drafting only.")
        self.assertEqual(
            {"status": "working-memory", "blocks": []},
            governance_readiness(self.repo / "memory", self.repo),
        )

    def test_git_attestation_does_not_require_a_pull_request(self) -> None:
        proposal = {"proposal_revision": 1, "base_registry_revision": {"registry_digest": "sha256:" + "0" * 64, "observed_commit": None}}
        path = self.write_record("attestation.json", {"provider": "git-signed-commit", "commit": "a" * 40, "status": "approved", "proposal_revision": 1, "base_registry_revision": proposal["base_registry_revision"]})
        self.assertEqual([], verify_scm(path, proposal))

    def test_without_any_implementation_path_the_impact_is_unknown(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.commit_all("unmapped")
        self.assertEqual("unknown", self.change("src/orders/port.py")["verdict"])

    def test_a_superseded_proposal_redrafts_into_a_package_that_applies(self) -> None:
        stuck = self.draft_package()
        submit_proposal(stuck, self.repo / "memory", self.repo)
        supersede_proposal(stuck, "verified before its approval was recorded", None)
        redraft = self.repo / "redraft"
        redraft_change_package(stuck, redraft, "PRO-2")
        proposal = json.loads((redraft / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual(
            ("draft", None, [], None),
            (proposal["status"], proposal["submitted_at"], proposal["approvals"],
             proposal["base_registry_revision"]),
        )
        self.assertFalse([key for key in proposal if key.startswith("superseded")])
        self.assertEqual((["PRO-1"], True), (proposal["supersedes"], iso_timestamp(proposal["redrafted_at"])))
        for name in ("domain-change-proposal.json", "test-obligations.json", "evidence-bundle.json"):
            with self.subTest(name=name):
                document = json.loads((redraft / name).read_text(encoding="utf-8"))
                self.assertEqual("PRO-2", document["proposal_id"])
        evidence_path = redraft / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        self.assertEqual((None, []), (evidence["counterfactual_check"], evidence["test_results"]))
        stuck_evidence = json.loads((stuck / "evidence-bundle.json").read_text(encoding="utf-8"))
        evidence["counterfactual_check"] = stuck_evidence["counterfactual_check"]
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        record_test_result(
            redraft, self.repo / "memory", self.repo, ObligationRun("OB-1", self.run_writing("ok"), "unit"), 60
        )
        self.approve_and_apply(redraft)
        self.assertEqual(["order-total"], self.rule_ids())

    def test_redrafting_refuses_a_live_proposal_and_a_used_output(self) -> None:
        package = self.draft_package()
        with self.assertRaisesRegex(ValueError, "only a superseded proposal is redrafted"):
            redraft_change_package(package, self.repo / "redraft", "PRO-2")
        supersede_proposal(package, "replaced", None)
        (self.repo / "used").mkdir()
        (self.repo / "used" / "note.txt").write_text("kept", encoding="utf-8")
        with self.assertRaisesRegex(FileExistsError, "output directory is not empty: "):
            redraft_change_package(package, self.repo / "used", "PRO-2")
        self.assertEqual(["note.txt"], [entry.name for entry in (self.repo / "used").iterdir()])
        self.assertEqual("kept", (self.repo / "used" / "note.txt").read_text(encoding="utf-8"))

    def test_redrafting_under_an_id_a_sibling_package_uses_is_refused_and_writes_nothing(self) -> None:
        package = self.draft_package()
        supersede_proposal(package, "replaced", None)
        with self.assertRaisesRegex(ValueError, "^proposal_id PRO-1 is already used by proposal; choose another$"):
            redraft_change_package(package, self.repo / "redraft", "PRO-1")
        self.assertFalse((self.repo / "redraft").exists())

    def test_a_redraft_that_fails_halfway_leaves_the_output_as_it_found_it(self) -> None:
        package = self.draft_package()
        supersede_proposal(package, "replaced", None)
        (self.repo / "empty").mkdir()
        for output, existed in ((self.repo / "redraft", False), (self.repo / "empty", True)):
            with self.subTest(existed=existed):
                with patch("domain_registry.changes._reset_review_state", side_effect=PermissionError(5, "Access is denied")), \
                        self.assertRaises(PermissionError):
                    redraft_change_package(package, output, "PRO-2")
                self.assertEqual(([] if existed else None), sorted(output.iterdir()) if output.exists() else None)

    def test_redrafting_with_a_blank_proposal_id_is_refused_and_writes_nothing(self) -> None:
        package = self.draft_package()
        supersede_proposal(package, "replaced", None)
        for blank in ("", "   "):
            with self.subTest(blank=repr(blank)):
                with self.assertRaisesRegex(ValueError, "^proposal_id must not be blank$"):
                    redraft_change_package(package, self.repo / "redraft", blank)
                self.assertFalse((self.repo / "redraft").exists())

    def test_redrafting_an_applied_and_superseded_proposal_resets_every_review_state_field(self) -> None:
        applied = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(applied)
        source = self.proposal_in(applied)
        source_evidence = json.loads((applied / "evidence-bundle.json").read_text(encoding="utf-8"))
        self.assertEqual(
            (True, True, True, True, True),
            (bool(source["approvals"]), source["verified_at"] is not None, source["finalized_at"] is not None,
             source["applied_at"] is not None, source_evidence["scm_attestation"] is not None),
        )
        supersede_proposal(applied, "replaced by a corrected proposal", "PRO-2")
        redraft = self.repo / "redraft"
        redraft_change_package(applied, redraft, "PRO-2")
        proposal = self.proposal_in(redraft)
        evidence = json.loads((redraft / "evidence-bundle.json").read_text(encoding="utf-8"))
        self.assertEqual(
            {"status": "draft", "approvals": [], "verified_at": None, "finalized_at": None,
             "applied_at": None, "scm_attestation": None, "has_applied_registry_revision": False},
            {"status": proposal["status"], "approvals": proposal["approvals"],
             "verified_at": proposal["verified_at"], "finalized_at": proposal["finalized_at"],
             "applied_at": proposal["applied_at"], "scm_attestation": evidence["scm_attestation"],
             "has_applied_registry_revision": "applied_registry_revision" in proposal},
        )

    def test_a_submitted_package_ends_lines_with_lf_on_every_platform(self) -> None:
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        for name in ("domain-change-proposal.json", "evidence-bundle.json"):
            with self.subTest(name=name):
                content = (package / name).read_bytes()
                self.assertIn(b"\n", content)
                self.assertNotIn(b"\r\n", content)

    def test_only_a_finalized_proposal_applies(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        with self.assertRaisesRegex(ValueError, "only a finalized approved proposal may apply"):
            apply_approved_updates(package, self.repo / "memory", self.repo)

    def test_a_finalized_proposal_superseded_before_it_applied_does_not_apply(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_finalize(package)
        supersede_proposal(package, "A later proposal records this rule differently.", None)
        with self.assertRaisesRegex(ValueError, "only a finalized approved proposal may apply"):
            apply_approved_updates(package, self.repo / "memory", self.repo)
        self.assertEqual(self.rule_ids(), [])

    def test_verifying_before_the_required_approval_names_the_role_and_keeps_it_submitted(self) -> None:
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        with self.assertRaisesRegex(ValueError, "run record-approval for each of: domain-owner$"):
            verify_proposal(package, self.repo / "memory", self.repo)
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("submitted", proposal["status"])
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        verify_proposal(package, self.repo / "memory", self.repo)
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("verified", proposal["status"])

    def test_the_proposer_cannot_approve_their_own_proposal(self) -> None:
        package = self.submitted_package()
        with self.assertRaisesRegex(ValueError, "^the proposer cannot approve the proposal$"):
            record_approval(package, "domain-owner", "author", "entire proposal", None)
        self.assertEqual([], self.proposal_in(package)["approvals"])

    def test_a_blank_role_reviewer_or_scope_is_refused(self) -> None:
        package = self.submitted_package()
        cases = [
            ("blank role", ("   ", "reviewer", "entire proposal")),
            ("blank reviewer", ("domain-owner", "", "entire proposal")),
            ("blank scope", ("domain-owner", "reviewer", "  ")),
        ]
        for label, (role, reviewer, scope) in cases:
            with self.subTest(label):
                with self.assertRaisesRegex(ValueError, "^approval requires role, reviewer, and scope$"):
                    record_approval(package, role, reviewer, scope, None)
                self.assertEqual([], self.proposal_in(package)["approvals"])

    def test_an_approval_time_that_is_not_iso_is_refused(self) -> None:
        package = self.submitted_package()
        for label, approved_at in (("words", "yesterday"), ("impossible date", "2026-13-45T00:00:00Z")):
            with self.subTest(label):
                with self.assertRaisesRegex(ValueError, "^approved_at must be an ISO-8601 timestamp$"):
                    record_approval(package, "domain-owner", "reviewer", "entire proposal", approved_at)
                self.assertEqual([], self.proposal_in(package)["approvals"])

    def test_the_same_role_cannot_approve_the_same_revision_twice(self) -> None:
        package = self.submitted_package()
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        recorded = self.proposal_in(package)["approvals"]
        with self.assertRaisesRegex(ValueError, "^the role already approved this proposal revision$"):
            record_approval(package, "domain-owner", "second-reviewer", "entire proposal", None)
        self.assertEqual(recorded, self.proposal_in(package)["approvals"])

    def test_the_same_role_can_approve_a_later_revision(self) -> None:
        package = self.submitted_package()
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["proposal_revision"] = 2
        path.write_text(json.dumps(proposal), encoding="utf-8")
        record_approval(package, "domain-owner", "second-reviewer", "entire proposal", None)
        self.assertEqual(
            [("reviewer", 1), ("second-reviewer", 2)],
            [(entry["reviewer"], entry["proposal_revision"]) for entry in self.proposal_in(package)["approvals"]],
        )

    def submit_and_approve_after_committing(self, package: Path) -> None:
        self.commit_everything("draft")
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)

    def another_apply_rewrites_the_proposed_rule(self) -> None:
        self.edit_registry(
            "rules.json",
            lambda value: value["rules"].append(
                {"id": "order-total", "contexts": ["orders"], "statement": "Another statement.", "evidence": []}
            ),
        )

    def test_verifying_after_another_apply_changed_a_record_the_proposal_names_is_stale(self) -> None:
        package = self.draft_package()
        self.submit_and_approve_after_committing(package)
        self.another_apply_rewrites_the_proposed_rule()
        with self.assertRaises(ValueError) as refused:
            verify_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual(
            "proposal base revision is stale; rebase and obtain fresh approval: "
            "rules/order-total, which this proposal touches, changed",
            str(refused.exception),
        )
        self.assertEqual("submitted", self.proposal_in(package)["status"])

    def test_finalizing_after_another_apply_changed_a_record_the_proposal_names_is_stale(self) -> None:
        package = self.draft_package()
        self.submit_and_approve_after_committing(package)
        verify_proposal(package, self.repo / "memory", self.repo)
        self.another_apply_rewrites_the_proposed_rule()
        with self.assertRaises(ValueError) as refused:
            finalize_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual(
            "proposal base revision is stale; rebase and obtain fresh approval: "
            "rules/order-total, which this proposal touches, changed",
            str(refused.exception),
        )
        self.assertEqual("verified", self.proposal_in(package)["status"])

    def test_an_approval_of_another_revision_does_not_satisfy_verification(self) -> None:
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["approvals"][0]["proposal_revision"] = 0
        path.write_text(json.dumps(proposal), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "each of: domain-owner$"):
            verify_proposal(package, self.repo / "memory", self.repo)

    def test_previewing_keeps_the_base_revision_a_submitted_proposal_already_has(self) -> None:
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        self.assertEqual(
            ["an approved or applied proposal requires SCM attestation"],
            validate_change_package(package, self.repo / "memory", "approved"),
        )

    def test_finalizing_without_scm_attestation_names_the_fields_and_the_step(self) -> None:
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        verify_proposal(package, self.repo / "memory", self.repo)
        with self.assertRaises(ValueError) as refused:
            finalize_proposal(package, self.repo / "memory", self.repo)
        message = str(refused.exception)
        for expected in (
            "after the pull request merges",
            'add scm_attestation to evidence-bundle.json with provider "github"',
            "pull_request and checks_url",
            "base_registry_revision copied from domain-change-proposal.json",
            "verify-scm-attestation",
        ):
            with self.subTest(expected=expected):
                self.assertIn(expected, message)
        proposal = json.loads(
            (package / "domain-change-proposal.json").read_text(encoding="utf-8")
        )
        self.assertEqual("verified", proposal["status"])

    def test_the_missing_attestation_message_follows_the_policy_verifier(self) -> None:
        signed = missing_attestation_message("git-signed-commit")
        self.assertIn("after the signed commit that adds this Change Package and changes the Domain Memory files exists", signed)
        self.assertIn('provider "git-signed-commit"', signed)
        self.assertIn("run attest-signed-commit --commit", signed)
        self.assertNotIn("pull_request", signed)
        self.assertIn("names no review verifier", missing_attestation_message("none"))

    def test_approved_package_applies_against_its_exact_revision(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        proposal_path = package / "domain-change-proposal.json"
        submitted = json.loads(proposal_path.read_text(encoding="utf-8"))
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["scm_attestation"] = {
            "provider": "github",
            "pull_request": "https://github.example/repo/pull/1",
            "checks_url": "https://github.example/repo/actions/runs/1",
            "commit": "a" * 40,
            "status": "approved",
            "proposal_revision": 1,
            "base_registry_revision": submitted["base_registry_revision"],
        }
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        verify_proposal(package, self.repo / "memory", self.repo)
        with patch("domain_registry.hitl.verify_external_scm", return_value=[]) as verify_scm, \
                patch("domain_registry.hitl.github_commit_carries_proposal", return_value=[]):
            finalize_proposal(package, self.repo / "memory", self.repo)
        verify_scm.assert_called_once_with(evidence["scm_attestation"], "GITHUB_TOKEN", True)
        digest_before_apply = registry_digest(self.repo / "memory")
        apply_approved_updates(package, self.repo / "memory", self.repo)
        rules = read_document(self.repo / "memory", "rules.json")
        self.assertEqual("reviewed", rules["rules"][0]["status"])
        applied = json.loads(
            (package / "domain-change-proposal.json").read_text(encoding="utf-8")
        )
        self.assertEqual("applied", applied["status"])
        head = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True
        ).stdout.strip()
        self.assertEqual(
            {"observed_commit": head, "registry_digest": registry_digest(self.repo / "memory")},
            applied["applied_registry_revision"],
        )
        self.assertNotEqual(digest_before_apply, applied["applied_registry_revision"]["registry_digest"])
        self.assertFalse(
            (self.repo / "memory" / ".domain-registry-reconciliation.json").exists()
        )
        self.assertEqual([], validate_change_package(package, self.repo / "memory"))

    def test_superseding_keeps_the_status_the_proposal_died_in(self) -> None:
        package = self.submitted_package()
        supersede_proposal(
            package,
            "The event catalogue shows the collaboration runs the other way.",
            None,
        )
        proposal = self.proposal_in(package)
        self.assertEqual(proposal["status"], "superseded")
        self.assertEqual(proposal["superseded_from_status"], "submitted")
        self.assertIn("other way", proposal["superseded_reason"])

    def test_a_superseded_proposal_can_name_what_replaced_it(self) -> None:
        package = self.submitted_package()
        supersede_proposal(
            package, "Replaced by a proposal with the corrected direction.", "PRO-2"
        )
        self.assertEqual(self.proposal_in(package)["superseded_by"], "PRO-2")

    def test_superseding_without_a_reason_is_refused(self) -> None:
        package = self.submitted_package()
        with self.assertRaisesRegex(ValueError, "requires a reason"):
            supersede_proposal(package, "   ", None)
        self.assertEqual(self.proposal_in(package)["status"], "submitted")

    def test_a_superseded_proposal_cannot_be_superseded_again(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "Overtaken by the event catalogue.", None)
        with self.assertRaisesRegex(ValueError, "create a new draft instead"):
            supersede_proposal(package, "Overtaken twice.", None)

    def test_a_superseded_proposal_no_longer_accepts_approval(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "Overtaken by the event catalogue.", None)
        with self.assertRaisesRegex(ValueError, "only for a submitted proposal"):
            record_approval(
                package, "domain-owner", "reviewer", "entire proposal", None
            )

    def test_a_test_run_in_a_subdirectory_records_where_it_ran(self) -> None:
        package = self.draft_package()
        (self.repo / "apps" / "web").mkdir(parents=True)
        command = self.run_writing("ok").replace(
            "sys.stdout.write('ok')", "import os; sys.stdout.write(os.path.basename(os.getcwd()))"
        )
        record_test_result(
            package, self.repo / "memory", self.repo, ObligationRun("OB-1", command, "unit", cwd="apps/web"), 60
        )
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual(
            ("apps/web", "sha256:" + hashlib.sha256(b"web").hexdigest()),
            (result["working_directory"], result["output_sha256"]),
        )

    def test_a_test_run_refuses_a_directory_outside_the_repository_or_missing(self) -> None:
        package = self.draft_package()
        before = (package / "evidence-bundle.json").read_bytes()
        for cwd in ("..", "missing"):
            with self.subTest(cwd=cwd), self.assertRaisesRegex(ValueError, "--cwd must name a directory inside"):
                record_test_result(
                    package, self.repo / "memory", self.repo,
                    ObligationRun("OB-1", self.run_writing("ok"), "unit", cwd=cwd), 60,
                )
        self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())


if __name__ == "__main__":
    unittest.main()
