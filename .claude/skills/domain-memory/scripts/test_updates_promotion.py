from __future__ import annotations

import json
import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import validate_change_package
from domain_registry.evidence import citation
from domain_registry.hitl import submit_proposal, verify_proposal
from domain_registry.audit import reviewing_proposals
from domain_registry.audit import verify as verify_audit
from domain_registry.policy import amend_policy_fields
from domain_registry.registry import verify_evidence
from domain_registry.revision import (
    current_registry_revision,
    registry_digest,
    require_current_registry_revision,
    valid_digest,
)
from domain_registry.sources import write_source_map
from domain_registry.updates import apply_approved_updates, promote_candidate, upsert_candidate
from domain_registry.common import registry_dir
from domain_registry.store import INDEX, read_document
from test_support import VERIFIED, RegistryTestCase, amend_policy, applied_by


class UpdatesPromotionTest(RegistryTestCase):
    def test_a_reviewed_record_is_not_replaced_without_superseding_its_approval(
        self,
    ) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(package, "PRO-2", "An order total may be negative.", None)
        with self.assertRaisesRegex(ValueError, "must list PRO-1 in supersedes"):
            self.approve_and_apply(package)
        self.assertEqual(
            self.stored_rule()["statement"], "An order total is non-negative."
        )

    def recited_package(self, statement: str | None = None) -> Path:
        (self.repo / "README.md").write_text("evidence\nmoved here\n", encoding="utf-8")
        package = self.draft_package()
        proposal_path = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        record = proposal["registry_updates"][0]["record"]
        self.seed("rules.json", [{**record, "status": "reviewed", "verified": VERIFIED,
                                  "evidence": [citation(self.repo, "README.md", 1, 1)]}])
        applied_by(self.repo / "memory", "PRO-1", "rules/order-total")
        record["evidence"] = [citation(self.repo, "README.md", 2, 2)]
        if statement is not None:
            record["statement"] = statement
        proposal.update(proposal_id="PRO-2", supersedes=["PRO-1"])
        proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
        for name in ("test-obligations.json", "evidence-bundle.json"):
            path = package / name
            value = json.loads(path.read_text(encoding="utf-8"))
            value["proposal_id"] = "PRO-2"
            path.write_text(json.dumps(value), encoding="utf-8")
        requirement_path = package / "requirement-normalization.json"
        requirement = json.loads(requirement_path.read_text(encoding="utf-8"))
        requirement["required_approval_roles"] = []
        requirement_path.write_text(json.dumps(requirement), encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        return package

    def test_a_recitation_applied_without_an_approval_keeps_the_records_verified_list(self) -> None:
        package = self.recited_package()
        self.attest_and_finalize(package)
        apply_approved_updates(package, self.repo / "memory", self.repo)
        stored = self.stored_rule()
        self.assertEqual(
            (VERIFIED, "reviewed", {"start": 2, "end": 2}, "PRO-2"),
            (stored["verified"], stored["status"], stored["evidence"][0]["lines"],
             reviewing_proposals(self.repo / "memory")["rules/order-total"]),
        )

    def test_an_update_that_changes_more_than_its_citations_without_an_approval_is_refused_before_signing(self) -> None:
        package = self.recited_package("An order total is never below zero.")
        with self.assertRaisesRegex(
            ValueError,
            re.escape("once applied: rules/order-total would be reviewed by nobody; the proposal records no approval and "
                      "changes more than its citations (statement), so record-approval first"),
        ):
            verify_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual("An order total is non-negative.", self.stored_rule()["statement"])

    def test_superseding_a_different_proposal_does_not_replace_a_reviewed_record(
        self,
    ) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(
            package, "PRO-2", "An order total may be negative.", "PRO-UNRELATED"
        )
        with self.assertRaisesRegex(ValueError, "must list PRO-1 in supersedes"):
            self.approve_and_apply(package)
        self.assertEqual(
            self.stored_rule()["statement"], "An order total is non-negative."
        )

    def test_superseding_the_reviewing_proposal_replaces_a_reviewed_record(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(
            package, "PRO-2", "An order total is never below zero.", "PRO-1"
        )
        self.approve_and_apply(package)
        replaced = self.stored_rule()
        self.assertEqual(replaced["statement"], "An order total is never below zero.")
        self.assertEqual(replaced["status"], "reviewed")
        self.assertEqual(
            ("PRO-2", ["human:reviewer"]),
            (reviewing_proposals(self.repo / "memory")["rules/order-total"],
             [entry["by"] for entry in replaced["verified"]]),
        )

    def test_a_proposal_citing_outside_the_selected_sources_is_refused_at_submission(self) -> None:
        self.select_docs_as_sources()
        package = self.draft_package()
        self.cite_in_proposal(package, "README.md")
        refusal = ("rules/order-total cites README.md outside the confirmed sources, so ship-package would "
                   "refuse to review it; cite a confirmed source, or have the developer select that path")
        with self.assertRaisesRegex(ValueError, re.escape(refusal)):
            submit_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual(
            "draft", json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))["status"]
        )

    def test_a_source_deselected_after_approval_still_stops_the_apply(self) -> None:
        self.select_docs_as_sources()
        package = self.draft_package()
        self.cite_in_proposal(package, "docs/decision.md")
        self.approve_and_finalize(package)
        (self.repo / "notes").mkdir()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            {
                "selected_paths": ["notes"],
                "selection_status": "developer-confirmed",
                "source_kinds": {"notes": "decisions"},
                "source_groups": [{"kind": "decisions", "paths": ["notes"]}],
            },
        )
        with self.assertRaisesRegex(ValueError, "rules/order-total cites docs/decision.md outside the confirmed sources"):
            apply_approved_updates(package, self.repo / "memory", self.repo)
        self.assertEqual(self.rule_ids(), [])

    def test_a_reviewed_record_citing_a_selected_source_is_applied(self) -> None:
        self.select_docs_as_sources()
        package = self.draft_package()
        self.cite_in_proposal(package, "docs/decision.md")
        self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_applying_a_record_drops_the_source_kind_it_claims_and_verification_derives_it(
        self,
    ) -> None:
        self.select_docs_as_sources()
        package = self.draft_package()
        self.cite_in_proposal(package, "docs/decision.md")
        proposal_path = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["record"]["evidence"][0]["source_kind"] = (
            "implementation"
        )
        proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
        self.approve_and_apply(package)
        [result] = verify_evidence(self.repo / "memory", self.repo)["results"]
        self.assertEqual(
            (False, "decisions"), ("source_kind" in self.stored_rule()["evidence"][0], result["source_kind"])
        )

    def test_what_the_plugin_writes_ends_lines_with_lf_on_every_platform(self) -> None:
        package = self.draft_package()
        self.approve_and_apply(package)
        written = [
            (registry_dir(self.repo / "memory") / INDEX),
            self.repo / "memory" / "audit" / "events.jsonl",
            *sorted((self.repo / "memory" / "registry" / "rules").glob("*.md")),
            package / "domain-change-proposal.json",
        ]
        for path in written:
            with self.subTest(path=path.name):
                content = path.read_bytes()
                self.assertIn(b"\n", content)
                self.assertNotIn(b"\r\n", content)

    def test_a_promoted_candidate_is_applied_as_the_reviewed_record(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        [update] = self.registry_updates(package)
        self.assertEqual(("upsert", "rules"), (update["operation"], update["asset"]))
        self.assertNotIn("status", update["record"])
        self.approve_and_apply(package)
        rules = read_document(self.repo / "memory", "rules.json")
        [rule] = rules["rules"]
        self.assertEqual(("order-total", "reviewed", "An order total is non-negative."),
                         (rule["id"], rule["status"], rule["statement"]))

    def test_promoting_adds_the_contexts_the_record_names_to_the_proposal(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        proposal_file = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_file.read_text(encoding="utf-8"))
        proposal["affected_contexts"] = ["billing"]
        proposal_file.write_text(json.dumps(proposal), encoding="utf-8")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.assertEqual(["billing", "orders"], json.loads(proposal_file.read_text(encoding="utf-8"))["affected_contexts"])

    def test_promoting_again_replaces_the_entry_with_the_current_candidate(self) -> None:
        package = self.package_with_a_candidate_rule("First wording.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        record = self.write_record(
            "rule.json",
            {"id": "order-total", "contexts": ["orders"], "statement": "Second wording.", "evidence": [self.readme_citation()]},
        )
        upsert_candidate(self.repo / "memory", self.repo, "rules", record)
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.assertEqual(["Second wording."], [entry["record"]["statement"] for entry in self.registry_updates(package)])

    def test_promoting_two_different_candidates_keeps_both_updates(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        second = self.write_record(
            "second-rule.json",
            {"id": "order-tax", "contexts": ["orders"], "statement": "Tax is added.", "evidence": [self.readme_citation()]},
        )
        upsert_candidate(self.repo / "memory", self.repo, "rules", second)
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        promote_candidate(package, self.repo / "memory", "rules", "order-tax")
        self.assertEqual(
            ["order-total", "order-tax"], [entry["record"]["id"] for entry in self.registry_updates(package)]
        )

    def test_promoting_a_record_the_registry_does_not_hold_is_refused(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        with self.assertRaisesRegex(ValueError, "no rules/missing in the Registry"):
            promote_candidate(package, self.repo / "memory", "rules", "missing")

    def test_promoting_an_already_reviewed_record_is_refused(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.approve_and_apply(package)
        again = self.redraft_target()
        with self.assertRaisesRegex(ValueError, "already reviewed; .* or pass --amend"):
            promote_candidate(again, self.repo / "memory", "rules", "order-total")

    def test_amending_a_reviewed_record_copies_it_unreviewed_and_supersedes_its_reviewer(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.approve_and_apply(package)
        again = self.redraft_target()
        proposal_path = again / "domain-change-proposal.json"
        for supersedes, expected in ((None, ["PRO-1"]), ("PRO-0", ["PRO-0", "PRO-1"]), (["PRO-1"], ["PRO-1"])):
            with self.subTest(supersedes=supersedes):
                proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
                proposal_path.write_text(json.dumps({**proposal, "supersedes": supersedes}), encoding="utf-8")
                self.assertEqual("PRO-1", promote_candidate(again, self.repo / "memory", "rules", "order-total", amend=True))
                proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
                [update] = [entry for entry in proposal["registry_updates"] if entry["record"]["id"] == "order-total"]
                self.assertEqual(
                    (expected, False, False),
                    (proposal["supersedes"], "status" in update["record"], "verified" in update["record"]),
                )

    def test_amending_a_reviewed_record_no_package_reviewed_is_refused(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        self.seed("rules.json", [{"id": "order-total", "contexts": ["orders"], "statement": "x", "status": "reviewed"}])
        with self.assertRaisesRegex(ValueError, "^no applied Change Package reviewed rules/order-total in the audit log"):
            promote_candidate(package, self.repo / "memory", "rules", "order-total", amend=True)

    def test_amend_promotes_a_candidate_as_promotion_does(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        self.assertIsNone(promote_candidate(package, self.repo / "memory", "rules", "order-total", amend=True))
        self.assertEqual(["order-total"], [entry["record"]["id"] for entry in self.registry_updates(package)])

    def test_promoting_into_a_proposal_that_is_no_longer_a_draft_is_refused(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.approve_and_apply(package)
        with self.assertRaisesRegex(
            ValueError, "added while the proposal is a draft.*run supersede-proposal, then redraft-proposal"
        ):
            promote_candidate(package, self.repo / "memory", "rules", "order-total")

    def test_removing_a_reviewed_record_requires_superseding_its_approval(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "order-total", None)
        with self.assertRaisesRegex(ValueError, "must list PRO-1 in supersedes"):
            self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_superseding_the_reviewing_proposal_removes_a_reviewed_record(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "order-total", "PRO-1")
        self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), [])

    def test_removing_a_record_the_registry_does_not_hold_is_refused(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "no-such-rule", "PRO-1")
        with self.assertRaisesRegex(ValueError, "cannot remove absent record"):
            self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_a_proposal_without_updates_is_refused(self) -> None:
        package = self.drafted_with_updates([])
        with self.assertRaisesRegex(ValueError, "approved proposal has no registry_updates"):
            self.approve_and_apply(package)

    def test_an_invalid_package_is_refused_before_anything_applies(self) -> None:
        package = self.draft_package()
        (package / "test-obligations.json").unlink()
        with self.assertRaisesRegex(ValueError, "change package is invalid"):
            apply_approved_updates(package, self.repo / "memory", self.repo)

    def test_applying_requires_an_scm_verified_policy(self) -> None:
        package = self.draft_package()
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        with self.assertRaisesRegex(ValueError, "reviewed updates require an scm-verified"):
            apply_approved_updates(package, self.repo / "memory", self.repo)

    def test_a_registry_update_that_is_neither_an_upsert_nor_a_remove_is_refused(
        self,
    ) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "order-total", "PRO-1")
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["operation"] = "replace"
        path.write_text(json.dumps(proposal), encoding="utf-8")
        self.assertEqual(
            ["each registry update must be an upsert or a remove"],
            validate_change_package(package, self.repo / "memory"),
        )
        with self.assertRaisesRegex(ValueError, "^draft proposal is invalid: each registry update must be an upsert or a remove$"):
            submit_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_amending_a_governance_field_records_what_changed_and_why(self) -> None:
        change = amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone until a reviewer exists.",
        )
        self.assertEqual(self.stored_policy()["review_mode"], "local-draft-only")
        self.assertEqual(
            (change["from"], change["to"]), ("scm-verified", "local-draft-only")
        )
        recorded = json.loads(
            (self.repo / "memory" / "audit" / "events.jsonl")
            .read_text(encoding="utf-8")
            .splitlines()[-1]
        )
        self.assertEqual(recorded["operation"], "amend-policy")
        self.assertEqual(recorded["field"], "review_mode")
        self.assertEqual(recorded["from"], "scm-verified")
        self.assertIn("reviewer", recorded["reason"])
        self.assertEqual("valid", verify_audit(self.repo / "memory")["status"])

    def test_one_amendment_moves_review_and_its_authority_together(self) -> None:
        root = self.repo / "memory"
        amend_policy(root, "review_mode", "local-draft-only", "Drafting alone.")
        recorded_before = len(self.audit_lines())
        changes = amend_policy_fields(
            root,
            [
                ("review_verifier", "github-pr"),
                ("review_trigger", "external-scm"),
                ("ci_requirement", "required"),
                ("review_mode", "scm-verified"),
            ],
            "A reviewer and CI exist now.",
        )
        policy = self.stored_policy()
        self.assertEqual("scm-verified", policy["review_mode"])
        self.assertEqual(
            ("github-pr", "external-scm", "required"),
            tuple(policy["review_governance"][key] for key in ("verifier", "trigger", "ci_requirement")),
        )
        self.assertEqual(
            [
                ("review_mode", "local-draft-only", "scm-verified"),
                ("review_verifier", "none", "github-pr"),
                ("review_trigger", "none", "external-scm"),
                ("ci_requirement", "none", "required"),
            ],
            [(change["field"], change["from"], change["to"]) for change in changes],
        )
        appended = self.audit_lines()[recorded_before:]
        self.assertEqual([change["field"] for change in changes], [event["field"] for event in appended])
        self.assertEqual("valid", verify_audit(root)["status"])

    def test_the_registry_revision_moves_when_the_policy_moves(self) -> None:
        before = registry_digest(self.repo / "memory")
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone for now.",
        )
        self.assertNotEqual(before, registry_digest(self.repo / "memory"))

    def test_a_registry_without_a_policy_still_has_a_revision(self) -> None:
        policy = self.repo / "memory" / "domain-memory-policy.json"
        with_policy = registry_digest(self.repo / "memory")
        policy.unlink()
        without_policy = registry_digest(self.repo / "memory")
        self.assertTrue(valid_digest(without_policy))
        self.assertNotEqual(with_policy, without_policy)

    def test_a_captured_revision_goes_stale_after_a_hand_edited_policy(self) -> None:
        captured = {
            "base_registry_revision": current_registry_revision(
                self.repo / "memory", self.repo
            )
        }
        require_current_registry_revision(captured, self.repo / "memory", self.repo)
        path = self.repo / "memory" / "domain-memory-policy.json"
        value = json.loads(path.read_text(encoding="utf-8"))
        value["source_policy"]["authority"] = "somebody else entirely"
        path.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "stale"):
            require_current_registry_revision(captured, self.repo / "memory", self.repo)

    def test_superseding_a_list_that_names_the_reviewing_proposal_replaces_a_reviewed_record(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(package, "PRO-2", "An order total is never below zero.", ["PRO-0", "PRO-1"])
        self.approve_and_apply(package)
        self.assertEqual("An order total is never below zero.", self.stored_rule()["statement"])


if __name__ == "__main__":
    unittest.main()
