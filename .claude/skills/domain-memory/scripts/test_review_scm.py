from __future__ import annotations

import base64
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
    verify_scm_value,
)
from domain_registry.hitl import (
    finalize_proposal,
    record_approval,
    submit_proposal,
    verify_proposal,
)
from domain_registry.revision import registry_digest
from domain_registry.signing import SIGNING_KEY_ENV, init_signing_key
from domain_registry.updates import apply_approved_updates
from test_support import RegistryTestCase

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

    def serve(url: str, _token: str) -> tuple[object, None]:
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

    def serve(url: str, _token: str) -> tuple[object, str | None]:
        requested.append(url)
        return pages[url]

    return serve, requested


class ReviewScmTest(RegistryTestCase):
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

    def test_the_latest_review_of_each_reviewer_decides_the_approval(self) -> None:
        attestation = {"provider": "github", "pull_request": PULL_REQUEST_URL, "commit": "a" * 40}
        approved = {"user": {"login": "reviewer"}, "state": "APPROVED"}
        changes = {"user": {"login": "reviewer"}, "state": "CHANGES_REQUESTED"}
        for label, reviews, expected in (
            ("changes requested, then approved", [changes, approved], []),
            ("approved, then changes requested", [approved, changes], ["SCM pull request has no current approval"]),
        ):
            serve, _ = github_serving(
                {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}}, reviews,
                {"check_runs": [{"status": "completed", "conclusion": "success"}]},
            )
            with (
                self.subTest(label),
                patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}),
                patch("domain_registry.attestations.github_page", side_effect=serve),
            ):
                self.assertEqual(expected, verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"))

    def test_external_scm_verification_requires_merged_approved_green_github_pr(self) -> None:
        attestation = {"provider": "github", "pull_request": PULL_REQUEST_URL, "commit": "a" * 40}
        serve, requested = github_serving(
            {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}},
            [{"user": {"login": "reviewer"}, "state": "APPROVED"}],
            {"check_runs": [{"status": "completed", "conclusion": "success"}]},
        )
        with (
            patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}),
            patch("domain_registry.attestations.github_page", side_effect=serve),
        ):
            self.assertEqual([], verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"))
        self.assertEqual(
            [PULL_ENDPOINT, PULL_ENDPOINT + "/reviews", CHECK_RUNS_ENDPOINT], requested
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
            ("null head", ({**merged, "head": None}, approved, green),
             "SCM pull request head does not match the attested commit"),
            ("no reviews", (merged, [], green), "SCM pull request has no current approval"),
            ("approval by a deleted account", (merged, [{"user": None, "state": "APPROVED"}], green),
             "SCM pull request has no current approval"),
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
        with (
            patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}),
            patch("domain_registry.attestations.github_page", side_effect=serve),
        ):
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

    def test_auditing_a_changes_directory_that_does_not_exist_is_refused_and_an_empty_one_finds_nothing(self) -> None:
        with self.assertRaises(ValueError) as refused:
            audit_attestations(self.repo, self.repo / "chnages", "GITHUB_TOKEN")
        (self.repo / "empty").mkdir()
        self.assertEqual(
            (f"no Change Packages directory at {self.repo / 'chnages'}; pass the directory that holds them", []),
            (str(refused.exception), audit_attestations(self.repo, self.repo / "empty", "GITHUB_TOKEN")),
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
        with (
            patch("domain_registry.hitl.verify_external_scm", return_value=[]),
            patch("domain_registry.hitl.github_commit_carries_proposal", return_value=absent),
            self.assertRaisesRegex(ValueError, "not externally verified: Git commit does not carry"),
        ):
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


if __name__ == "__main__":
    unittest.main()
