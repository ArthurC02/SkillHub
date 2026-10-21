from __future__ import annotations

import json
import shutil
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import missing_approval_roles, redraft_change_package, validate_change_package
from domain_registry.hitl import (
    ObligationRun,
    finalize_proposal,
    missing_attestation_message,
    rebase_proposal,
    record_test_result,
    record_approval,
    submit_proposal,
    supersede_proposal,
    verify_proposal,
)
from domain_registry.common import iso_timestamp, write_json
from domain_registry.revision import (
    StaleBaseError,
    base_registry_revision,
    registry_digest,
    require_current_registry_revision,
)
from domain_registry.store import read_document
from domain_registry.updates import apply_approved_updates, promote_candidate, upsert_candidate
from test_support import RegistryTestCase


class ReviewLifecycleTest(RegistryTestCase):
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
            redraft, self.repo / "memory", self.repo,
            ObligationRun("OB-1", self.declare_check(redraft, self.run_writing("ok")), "unit"), 60,
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
        with self.assertRaisesRegex(ValueError, "output directory is not empty: "):
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
            "proposal base revision is stale; rebase-proposal, then obtain fresh approval: "
            "rules/order-total, which this proposal writes, changed since it was reviewed",
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
            "proposal base revision is stale; rebase-proposal, then obtain fresh approval: "
            "rules/order-total, which this proposal writes, changed since it was reviewed",
            str(refused.exception),
        )
        self.assertEqual("verified", self.proposal_in(package)["status"])

    def break_registry_updates(self, package: Path) -> Path:
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["registry_updates"] = "not a list"
        path.write_text(json.dumps(proposal), encoding="utf-8")
        return path

    def test_a_verification_the_package_fails_writes_nothing(self) -> None:
        package = self.draft_package()
        self.submit_and_approve_after_committing(package)
        path = self.break_registry_updates(package)
        before = path.read_bytes()
        with patch("domain_registry.hitl.write_json", wraps=write_json) as written, self.assertRaisesRegex(
            ValueError, "^verified proposal is invalid: domain-change-proposal.json registry_updates must be a list$"
        ):
            verify_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual(([], before), (written.call_args_list, path.read_bytes()))

    def test_a_finalization_the_package_fails_writes_the_proposal_once_as_submitted_again(self) -> None:
        package = self.draft_package()
        self.submit_and_approve_after_committing(package)
        self.attest_by_pull_request(package)
        verify_proposal(package, self.repo / "memory", self.repo)
        path = self.break_registry_updates(package)
        with patch("domain_registry.hitl.verify_external_scm", return_value=[]), patch(
            "domain_registry.hitl.github_commit_carries_proposal", return_value=[]
        ), patch("domain_registry.hitl.write_json", wraps=write_json) as written, self.assertRaisesRegex(
            ValueError,
            "^proposal lacks required approvals or traceability, and is submitted again so it can be corrected, "
            "then verified and finalized again: domain-change-proposal.json registry_updates must be a list$",
        ):
            finalize_proposal(package, self.repo / "memory", self.repo)
        proposal = self.proposal_in(package)
        self.assertEqual(
            ([path], [("submitted", False)], "submitted", False),
            ([call.args[0] for call in written.call_args_list],
             [(call.args[1]["status"], "finalized_at" in call.args[1]) for call in written.call_args_list],
             proposal["status"], "finalized_at" in proposal),
        )

    def test_a_stale_base_is_refused_as_a_stale_base_error_that_is_still_a_value_error(self) -> None:
        package = self.draft_package()
        self.submit_and_approve_after_committing(package)
        self.another_apply_rewrites_the_proposed_rule()
        with self.assertRaises(StaleBaseError) as refused:
            require_current_registry_revision(self.proposal_in(package), self.repo / "memory", self.repo)
        self.assertIsInstance(refused.exception, ValueError)
        self.assertEqual(
            "proposal base revision is stale; rebase-proposal, then obtain fresh approval: "
            "rules/order-total, which this proposal writes, changed since it was reviewed",
            str(refused.exception),
        )

    def test_a_committed_policy_that_is_not_json_is_refused_by_commit_and_path(self) -> None:
        policy = self.repo / "memory" / "domain-memory-policy.json"
        intact = policy.read_bytes()
        policy.write_text("{not json", encoding="utf-8")
        commit = self.commit_everything("broken policy")
        policy.write_bytes(intact)
        with self.assertRaisesRegex(ValueError, f"^{commit}:memory/domain-memory-policy.json is not valid JSON$"):
            base_registry_revision(self.repo / "memory", self.repo)

    def test_rebasing_a_proposal_whose_base_is_unreadable_is_refused_and_writes_nothing(self) -> None:
        package = self.submitted_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["base_registry_revision"] = None
        path.write_text(json.dumps(proposal), encoding="utf-8")
        written = path.read_bytes()
        with self.assertRaisesRegex(ValueError, "^proposal requires a Git commit and Registry digest base revision$"):
            rebase_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual(written, path.read_bytes())

    def attest_by_pull_request(self, package: Path) -> None:
        proposal = self.proposal_in(package)
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["scm_attestation"] = {
            "provider": "github", "pull_request": "https://github.example/repo/pull/1",
            "checks_url": "https://github.example/repo/actions/runs/1", "commit": "a" * 40, "status": "approved",
            "proposal_revision": proposal["proposal_revision"], "base_registry_revision": proposal["base_registry_revision"],
        }
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")

    def ship_by_pull_request(self, package: Path) -> None:
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        self.attest_by_pull_request(package)
        verify_proposal(package, self.repo / "memory", self.repo)
        with patch("domain_registry.hitl.verify_external_scm", return_value=[]), \
                patch("domain_registry.hitl.github_commit_carries_proposal", return_value=[]):
            finalize_proposal(package, self.repo / "memory", self.repo)
        apply_approved_updates(package, self.repo / "memory", self.repo)

    def drafted_beside(self, package: Path, proposal_id: str, rule_id: str) -> None:
        for name in ("domain-change-proposal.json", "test-obligations.json", "evidence-bundle.json"):
            document = json.loads((package / name).read_text(encoding="utf-8"))
            document["proposal_id"] = proposal_id
            if name == "domain-change-proposal.json":
                document["registry_updates"] = []
            (package / name).write_text(json.dumps(document), encoding="utf-8")
        record = self.write_record(
            f"{rule_id}.json",
            {"id": rule_id, "contexts": ["orders"], "statement": f"The rule {rule_id} holds.", "evidence": [self.readme_citation()]},
        )
        upsert_candidate(self.repo / "memory", self.repo, "rules", record)
        promote_candidate(package, self.repo / "memory", "rules", rule_id)

    def test_two_packages_submitted_over_each_others_uncommitted_drafts_both_apply(self) -> None:
        first = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.commit_everything("context")
        second = self.repo / "second"
        shutil.copytree(first, second)
        self.drafted_beside(first, "PRO-1", "order-total")
        self.drafted_beside(second, "PRO-2", "order-count")
        for package in (first, second):
            submit_proposal(package, self.repo / "memory", self.repo)
        self.commit_everything("both packages and their drafts")
        for package in (first, second):
            self.ship_by_pull_request(package)
        rules = read_document(self.repo / "memory", "rules.json")["rules"]
        self.assertEqual(
            [("order-count", "reviewed"), ("order-total", "reviewed")],
            sorted((rule["id"], rule["status"]) for rule in rules),
        )
        self.assertEqual(["applied", "applied"], [self.proposal_in(package)["status"] for package in (first, second)])

    def test_rebasing_a_stale_verified_proposal_restamps_it_and_retires_its_approval_and_attestation(self) -> None:
        package = self.draft_package()
        self.submit_and_approve_after_committing(package)
        submitted_base = self.proposal_in(package)["base_registry_revision"]
        self.attest_by_pull_request(package)
        verify_proposal(package, self.repo / "memory", self.repo)
        tested = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))["test_results"]
        self.another_apply_rewrites_the_proposed_rule()
        expected_base = {
            "observed_commit": subprocess.run(
                ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True
            ).stdout.strip(),
            "registry_digest": registry_digest(self.repo / "memory"),
            "committed_digest": submitted_base["registry_digest"],
        }
        self.assertEqual(
            {"package": "proposal", "status": "submitted", "proposal_revision": 2, "base_registry_revision": expected_base,
             "approvals_retired": 1, "attestation_removed": True,
             "next": "record-approval for each required role, verify-proposal, sign a commit holding the package, "
                     "then ship-package --commit <that commit>"},
            rebase_proposal(package, self.repo / "memory", self.repo),
        )
        proposal = self.proposal_in(package)
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        requirement = json.loads((package / "requirement-normalization.json").read_text(encoding="utf-8"))
        self.assertEqual(
            ("submitted", 2, expected_base, None, [1], expected_base, None, tested, ["domain-owner"]),
            (proposal["status"], proposal["proposal_revision"], proposal["base_registry_revision"], proposal["verified_at"],
             [approval["proposal_revision"] for approval in proposal["approvals"]], evidence["registry_revision"],
             evidence["scm_attestation"], evidence["test_results"], missing_approval_roles(requirement, proposal)),
        )
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        verify_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual("verified", self.proposal_in(package)["status"])

    def test_a_proposal_outside_review_or_with_a_current_base_is_not_rebased(self) -> None:
        package = self.draft_package()
        self.commit_everything("draft")
        submit_proposal(package, self.repo / "memory", self.repo)
        path = package / "domain-change-proposal.json"
        submitted = path.read_bytes()
        self.commit_everything("a commit on top that touches nothing the proposal names")
        self.edit_registry("contexts.json", lambda value: value["contexts"].append({"id": "billing", "name": "Billing"}))
        current = rebase_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual(
            ("the proposal base is current; nothing was written", "submitted", 1, submitted),
            (current["note"], current["status"], current["proposal_revision"], path.read_bytes()),
        )
        for status, refusal in (
            ("draft", "a draft proposal is not rebased; submit-proposal stamps its base"),
            ("applied", "an applied proposal is not rebased; it is applied; supersede it and open a new draft"),
            ("superseded", "a superseded proposal is not rebased; it is retired; redraft-proposal opens a new draft from it"),
        ):
            with self.subTest(status):
                proposal = json.loads(submitted)
                proposal["status"] = status
                path.write_text(json.dumps(proposal), encoding="utf-8")
                self.edit_registry("contexts.json", lambda value, status=status: value["contexts"][0].update(name=f"Orders {status}"))
                written = path.read_bytes()
                with self.assertRaisesRegex(ValueError, f"^{refusal}$"):
                    rebase_proposal(package, self.repo / "memory", self.repo)
                self.assertEqual(written, path.read_bytes())

    def test_a_stale_approved_proposal_is_rebased_back_to_submitted(self) -> None:
        package = self.submitted_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal.update(status="approved", finalized_at="2026-10-19T00:00:00Z")
        path.write_text(json.dumps(proposal), encoding="utf-8")
        self.edit_registry("contexts.json", lambda value: value["contexts"][0].update(name="Purchases"))
        printed = rebase_proposal(package, self.repo / "memory", self.repo)
        rebased = self.proposal_in(package)
        self.assertEqual(
            ("submitted", 2, None, "submitted"),
            (printed["status"], rebased["proposal_revision"], rebased["finalized_at"], rebased["status"]),
        )

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
        self.assertIn("run ship-package --commit", signed)
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


if __name__ == "__main__":
    unittest.main()
