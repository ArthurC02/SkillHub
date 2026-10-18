from __future__ import annotations

import json
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

from domain_registry.common import load_json
from domain_registry.hitl import record_approval, submit_proposal, verify_proposal
from domain_registry.registry import record_by_id
from domain_registry.ship import ship_package
from domain_registry.signing import SIGNING_KEY_ENV, init_signing_key
from test_support import RegistryTestCase


class ShipPackageTest(RegistryTestCase):
    def verified_package_under_signed_governance(self) -> tuple[Path, str]:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            key = init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["review_governance"].update(verifier="git-signed-commit", trigger="git-push", authorized_signers=[key["fingerprint"]])
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        unsigned = self.commit_everything("writes the policy")
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        verify_proposal(package, self.repo / "memory", self.repo)
        return package, unsigned

    def test_the_signed_commit_carrying_the_package_lets_it_attest_finalize_and_apply_in_one_call(self) -> None:
        package, _ = self.verified_package_under_signed_governance()
        signed = self.commit_everything("adds the Change Package", signed=True)
        report = ship_package(package, self.repo / "memory", self.repo, signed[:12])
        evidence = load_json(package / "evidence-bundle.json")
        self.assertEqual(
            (["attest-signed-commit", "finalize-proposal", "apply-approved-updates"], [], "applied", signed, "proposal", signed, "reviewed"),
            (report["completed"], report["errors"], report["status"], report["commit"], report["package"],
             evidence["scm_attestation"]["commit"], record_by_id(self.repo / "memory", "rules", "order-total")["status"]),
        )
        self.assertIn("sign a commit holding the package and the Registry", report["next"])

    def test_shipping_an_applied_package_again_with_the_same_commit_writes_nothing_and_succeeds(self) -> None:
        package, _ = self.verified_package_under_signed_governance()
        signed = self.commit_everything("adds the Change Package", signed=True)
        ship_package(package, self.repo / "memory", self.repo, signed)
        events_before = (self.repo / "memory" / "audit" / "events.jsonl").read_text(encoding="utf-8")
        evidence_before = (package / "evidence-bundle.json").read_bytes()
        again = ship_package(package, self.repo / "memory", self.repo, "HEAD")
        without_commit = ship_package(package, self.repo / "memory", self.repo, None)
        self.assertEqual(
            ([], [], "applied", "already applied; nothing was written", [], [], "already applied; nothing was written",
             events_before, evidence_before,
             "nothing to do here; if the applied Registry change is not committed yet, sign a commit holding it, then push"),
            (again["completed"], again["errors"], again["status"], again["note"], without_commit["completed"], without_commit["errors"],
             without_commit["note"], (self.repo / "memory" / "audit" / "events.jsonl").read_text(encoding="utf-8"),
             (package / "evidence-bundle.json").read_bytes(), again["next"]),
        )

    def test_shipping_an_applied_package_with_another_commit_is_refused_and_writes_nothing(self) -> None:
        package, unsigned = self.verified_package_under_signed_governance()
        signed = self.commit_everything("adds the Change Package", signed=True)
        ship_package(package, self.repo / "memory", self.repo, signed)
        evidence_before = (package / "evidence-bundle.json").read_bytes()
        report = ship_package(package, self.repo / "memory", self.repo, unsigned[:9])
        self.assertEqual(
            ([], [f"attest-signed-commit: the proposal is applied with commit {signed}; {unsigned} is another commit, so nothing was written"],
             "applied", evidence_before),
            (report["completed"], report["errors"], report["status"], (package / "evidence-bundle.json").read_bytes()),
        )
        self.assertNotIn("note", report)

    def test_an_approved_package_whose_apply_failed_is_applied_on_the_next_run_without_attesting_again(self) -> None:
        package, _ = self.verified_package_under_signed_governance()
        signed = self.commit_everything("adds the Change Package", signed=True)
        with patch("domain_registry.ship.apply_approved_updates", side_effect=ValueError("the disk was full")):
            first = ship_package(package, self.repo / "memory", self.repo, signed)
        second = ship_package(package, self.repo / "memory", self.repo, signed)
        self.assertEqual(
            ((["attest-signed-commit", "finalize-proposal"], ["apply-approved-updates: the disk was full"], "approved"),
             (["apply-approved-updates"], [], "applied")),
            ((first["completed"], first["errors"], first["status"]), (second["completed"], second["errors"], second["status"])),
        )
        self.assertNotIn("commit", second)

    def test_an_approved_package_is_not_applied_under_another_commit(self) -> None:
        package, unsigned = self.verified_package_under_signed_governance()
        signed = self.commit_everything("adds the Change Package", signed=True)
        with patch("domain_registry.ship.apply_approved_updates", side_effect=ValueError("the disk was full")):
            ship_package(package, self.repo / "memory", self.repo, signed)
        report = ship_package(package, self.repo / "memory", self.repo, unsigned)
        self.assertEqual(
            ([], 1, "approved", None),
            (report["completed"], len(report["errors"]), report["status"], record_by_id(self.repo / "memory", "rules", "order-total")),
        )
        self.assertTrue(report["errors"][0].startswith(f"attest-signed-commit: the proposal is approved with commit {signed}; {unsigned} is another commit"))

    def test_an_unsigned_commit_stops_before_finalizing_and_leaves_the_proposal_verified(self) -> None:
        package, unsigned = self.verified_package_under_signed_governance()
        report = ship_package(package, self.repo / "memory", self.repo, unsigned)
        self.assertEqual(
            ([], 1, "verified", None),
            (report["completed"], len(report["errors"]), report["status"], record_by_id(self.repo / "memory", "rules", "order-total")),
        )
        self.assertTrue(report["errors"][0].startswith("attest-signed-commit: the commit cannot attest this proposal: Git commit signature is invalid"))
        self.assertNotIn("commit", report)
        self.assertIn("ship-package --commit", report["next"])

    def test_without_a_commit_the_pull_request_verifier_needs_its_attestation_written_first(self) -> None:
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        verify_proposal(package, self.repo / "memory", self.repo)
        report = ship_package(package, self.repo / "memory", self.repo, None)
        self.assertEqual(([], 1, "verified"), (report["completed"], len(report["errors"]), report["status"]))
        self.assertTrue(report["errors"][0].startswith("finalize-proposal: finalizing a proposal requires SCM attestation: after the pull request merges"))

    def test_a_proposal_that_is_not_verified_is_named_with_its_status(self) -> None:
        package = self.draft_package()
        report = ship_package(package, self.repo / "memory", self.repo, None)
        self.assertEqual(
            (["finalize-proposal: only a verified proposal may be finalized"], "draft", "the proposal is draft; ship-package works from a verified proposal"),
            (report["errors"], report["status"], report["next"]),
        )

    def test_the_command_prints_the_report_and_exits_one_when_a_step_fails(self) -> None:
        package, unsigned = self.verified_package_under_signed_governance()
        failed = self.run_cli("ship-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
                              "--repo-root", str(self.repo), "--commit", unsigned)
        signed = self.commit_everything("adds the Change Package", signed=True)
        passed = self.run_cli("ship-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
                              "--repo-root", str(self.repo), "--commit", signed)
        self.assertEqual(
            ((1, "verified"), (0, "applied", signed)),
            ((failed.returncode, json.loads(failed.stdout)["status"]),
             (passed.returncode, json.loads(passed.stdout)["status"], json.loads(passed.stdout)["commit"])),
        )
        self.assertEqual(signed, subprocess.run(["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True).stdout.strip())


if __name__ == "__main__":
    unittest.main()
