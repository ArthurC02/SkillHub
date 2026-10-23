from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.attestations import (
    audit_attestations,
    carried_proposal_errors,
    commit_carries_proposal,
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
from domain_registry.updates import apply_approved_updates
from test_support import RegistryTestCase

BASE_REVISION = {"registry_digest": "sha256:" + "0" * 64, "observed_commit": None}
OTHER_REVISION = {"registry_digest": "sha256:" + "1" * 64, "observed_commit": None}
OUTSIDE_MESSAGE = "the Change Package must be inside the repository, so the attested commit can carry it"
ANOTHER_PROPOSAL_MESSAGE = (
    "Git commit carries another proposal or revision; attest a commit with this "
    "proposal_id and proposal_revision"
)
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
            "provider": "git-signed-commit",
            "commit": "a" * 40,
            "status": "approved",
            "proposal_revision": 1,
            "base_registry_revision": submitted["base_registry_revision"],
        }
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        verify_proposal(package, self.repo / "memory", self.repo)
        with patch("domain_registry.hitl.verify_git_signed_commit", return_value=[]), \
                patch("domain_registry.hitl.commit_carries_proposal", return_value=[]):
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
            "provider": "git-signed-commit",
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
             ["SCM attestation requires provider", "SCM attestation provider must be git-signed-commit"]),
            ("provider github", {**valid, "provider": "github"}, ["SCM attestation provider must be git-signed-commit"]),
            ("commit missing", without("commit"),
             ["SCM attestation requires commit", "SCM attestation requires a pinned 40-character commit"]),
            ("status missing", without("status"),
             ["SCM attestation requires status", "SCM attestation status must be approved"]),
            ("status pending", {**valid, "status": "pending"}, ["SCM attestation status must be approved"]),
            ("commit one character short", {**valid, "commit": "a" * 39},
             ["SCM attestation requires a pinned 40-character commit"]),
            ("proposal revision differs", {**valid, "proposal_revision": 2},
             ["SCM attestation proposal revision does not match"]),
            ("base registry revision differs", {**valid, "base_registry_revision": OTHER_REVISION},
             ["SCM attestation Registry revision does not match"]),
        ]
        for label, value, expected in cases:
            with self.subTest(label):
                self.assertEqual(expected, verify_scm_value(value, proposal))

    def test_an_attesting_commit_must_carry_this_proposal_revision(self) -> None:
        earlier = self.head()
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


    def test_a_signed_authorized_commit_that_changes_only_source_code_is_refused(self) -> None:
        key = self.probe_signing_key()
        commit = self.sign_a_commit_changing("src/x.py")
        self.assertEqual(
            ["Git commit does not change Domain Memory files"],
            verify_git_signed_commit(
                {"commit": commit}, self.repo, self.repo / "docs" / "domain-memory", key["authorized_signers"]
            ),
        )

    def test_a_signed_authorized_commit_that_changes_only_a_sibling_of_the_registry_directory_is_refused(self) -> None:
        key = self.probe_signing_key()
        commit = self.sign_a_commit_changing("docs/domain-memory-extra/x")
        self.assertEqual(
            ["Git commit does not change Domain Memory files"],
            verify_git_signed_commit(
                {"commit": commit}, self.repo, self.repo / "docs" / "domain-memory", key["authorized_signers"]
            ),
        )

    def test_the_attestation_audit_tells_carried_from_not_carried_from_unconfirmed(self) -> None:
        earlier = self.head()
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
        self.changed_package("remote", "applied", {"provider": "github", "commit": "e" * 40})
        self.changed_package("unattested", "applied", None)
        findings = audit_attestations(self.repo, self.repo / "changes")
        self.assertEqual(
            [("carried", "carried"), ("remote", "unconfirmed"), ("reused", "not carried"),
             ("unattested", "not carried"), ("unfetched", "unconfirmed")],
            [(finding["proposal_id"], finding["verdict"]) for finding in findings],
        )

    def test_auditing_a_changes_directory_that_does_not_exist_is_refused_and_an_empty_one_finds_nothing(self) -> None:
        with self.assertRaises(ValueError) as refused:
            audit_attestations(self.repo, self.repo / "chnages")
        (self.repo / "empty").mkdir()
        self.assertEqual(
            (f"no Change Packages directory at {self.repo / 'chnages'}; pass the directory that holds them", []),
            (str(refused.exception), audit_attestations(self.repo, self.repo / "empty")),
        )

    def test_the_attestation_audit_does_not_read_a_commit_split_across_lines(self) -> None:
        self.changed_package("unattested", "applied", None)
        added = self.commit_everything("adds the unattested package")
        self.changed_package("split", "applied", {
            "provider": "git-signed-commit", "commit": f"{added}\n{'f' * 40}",
        })
        findings = audit_attestations(self.repo, self.repo / "changes")
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
            [finding] = audit_attestations(root, root / "changes")
        self.assertEqual(finding["verdict"], "unconfirmed")

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
