from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import _requirement_and_proposal_errors as requirement_and_proposal_errors
from domain_registry.changes import (
    implementation_design_errors,
    init_change_package,
    validate_change_package,
)
from domain_registry.common import RISK_FLAGS
from domain_registry.evidence import (
    citation,
)
from test_support import RegistryTestCase


class ChangesTest(RegistryTestCase):
    def test_each_implementation_design_defect_is_named(self) -> None:
        design = {
            "domain_forces": ["force"], "decision": "decide", "invariants_preserved": ["kept"],
            "rejected_alternatives": ["other"], "proof_obligations": ["OB-1"], "counterfactual_check": "break it",
        }
        material = {"change_classification": "material", "implementation_design": design}
        cases = [
            ("complete", material, []),
            ("unknown classification", {**material, "change_classification": "minor"},
             ["domain-change-proposal.json has an invalid change_classification"]),
            ("material without design", {"change_classification": "material"},
             ["a material proposal requires implementation_design with domain forces, decision, invariants, "
              "alternatives, proof obligations, and counterfactual check"]),
            ("routine without design", {"change_classification": "routine"}, []),
            ("blank force", {**material, "implementation_design": {**design, "domain_forces": [""]}},
             ["implementation_design requires a valid domain_forces list"]),
            ("no invariants", {**material, "implementation_design": {**design, "invariants_preserved": "kept"}},
             ["implementation_design requires a valid invariants_preserved list"]),
            ("no alternatives", {**material, "implementation_design": {**design, "rejected_alternatives": None}},
             ["implementation_design requires a valid rejected_alternatives list"]),
            ("no decision", {**material, "implementation_design": {**design, "decision": " "}},
             ["implementation_design requires a completed decision"]),
            ("no obligation ids", {**material, "implementation_design": {**design, "proof_obligations": "OB-1"}},
             ["implementation_design requires proof obligation IDs"]),
            ("unknown obligation", {**material, "implementation_design": {**design, "proof_obligations": ["OB-9"]}},
             ["implementation_design references an unknown proof obligation"]),
            ("no counterfactual", {**material, "implementation_design": {**design, "counterfactual_check": ""}},
             ["implementation_design requires a counterfactual check"]),
            ("material without obligations", {**material, "implementation_design": {**design, "proof_obligations": []}},
             ["a material proposal requires at least one proof obligation"]),
            ("routine without obligations",
             {"change_classification": "routine", "implementation_design": {**design, "proof_obligations": []}}, []),
        ]
        for label, proposal, expected in cases:
            with self.subTest(label):
                self.assertEqual(expected, implementation_design_errors(proposal, {"OB-1"}))

    def test_each_requirement_and_proposal_defect_is_named(self) -> None:
        requirement = {"acceptance_criteria": [{"id": "AC-1", "statement": "Totals hold."}], "risk_flags": []}
        proposal = {"status": "draft", "proposal_revision": 1, "affected_contexts": ["orders"], "risk_flags": []}
        flagged = sorted(RISK_FLAGS)[0]
        cases = [
            ("complete", {}, {}, []),
            ("no criteria", {"acceptance_criteria": []}, {},
             ["requirement-normalization.json requires acceptance_criteria"]),
            ("blank criterion", {"acceptance_criteria": [{"id": "AC-1", "statement": ""}]}, {},
             ["each acceptance criterion requires a completed id and statement"]),
            ("unknown requirement flag", {"risk_flags": ["comet"]}, {},
             ["requirement-normalization.json has an unknown risk flag",
              "domain-change-proposal.json must retain requirement risk flags"]),
            ("unknown proposal flag", {}, {"risk_flags": ["comet"]},
             ["domain-change-proposal.json has an unknown risk flag"]),
            ("dropped flag", {"risk_flags": [flagged]}, {},
             ["domain-change-proposal.json must retain requirement risk flags"]),
            ("kept flag", {"risk_flags": [flagged]}, {"risk_flags": [flagged]}, []),
            ("status", {}, {"status": "shipped"}, ["domain-change-proposal.json has an invalid status"]),
            ("revision zero", {}, {"proposal_revision": 0},
             ["domain-change-proposal.json requires a positive proposal_revision"]),
            ("no contexts", {}, {"affected_contexts": []}, ["domain-change-proposal.json requires affected_contexts"]),
            ("rule ids", {}, {"rule_ids": [""]}, ["domain-change-proposal.json has invalid rule_ids"]),
            ("contract ids", {}, {"contract_ids": "c"}, ["domain-change-proposal.json has invalid contract_ids"]),
        ]
        for label, requirement_change, proposal_change, expected in cases:
            with self.subTest(label):
                errors, criteria = requirement_and_proposal_errors(
                    {**requirement, **requirement_change}, {**proposal, **proposal_change}
                )
                self.assertEqual(expected, errors)
        self.assertEqual(
            {"AC-1", "AC-2"},
            requirement_and_proposal_errors(
                {**requirement, "acceptance_criteria": [
                    {"id": "AC-1", "statement": "a"}, 7, {"id": "AC-2", "statement": "b"}]},
                proposal,
            )[1],
        )

    def test_a_package_started_without_identifiers_is_the_plain_template(self) -> None:
        package = self.repo / "plain"
        init_change_package(package)
        templates = Path(__file__).resolve().parent.parent / "templates"
        for path in package.iterdir():
            with self.subTest(file=path.name):
                self.assertEqual((templates / path.name).read_bytes(), path.read_bytes())

    def test_a_blank_identifier_is_refused_before_anything_is_written(self) -> None:
        package = self.repo / "blank"
        with self.assertRaisesRegex(ValueError, "proposal_id must not be blank"):
            init_change_package(package, "REQ-7", " ")
        with self.assertRaisesRegex(ValueError, "proposer must not be blank"):
            init_change_package(package, "REQ-7", "PRO-7", " ")
        self.assertFalse(package.exists())

    def test_a_package_started_with_a_proposer_is_not_asked_for_one(self) -> None:
        package = self.repo / "named"
        init_change_package(package, "REQ-7", "PRO-7", "agent-1")
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("agent-1", proposal["proposer"])
        self.assertNotIn(
            "domain-change-proposal.json requires a completed proposer",
            validate_change_package(package, self.repo / "memory"),
        )

    def test_a_fresh_package_is_not_asked_for_test_results_before_they_exist(self) -> None:
        package = self.repo / "fresh"
        init_change_package(package)
        errors = validate_change_package(package, self.repo / "memory", "submitted")
        self.assertNotIn("each test result requires a passed obligation_id and evidence", errors)
        self.assertNotIn("a test result references an unknown obligation", errors)
        self.assertNotIn("an approval requires role, reviewer, approved_at, and scope", errors)

    def test_previewing_approved_lists_every_missing_piece_of_that_stage_at_once(self) -> None:
        package = self.draft_package()
        errors = validate_change_package(package, self.repo / "memory", "approved")
        self.assertEqual(
            [
                "an approved or applied proposal lacks required approvals: domain-owner",
                "an approved or applied proposal requires SCM attestation",
            ],
            errors,
        )
        self.assertEqual([], validate_change_package(package, self.repo / "memory"))

    def test_each_approval_and_lifecycle_defect_is_named(self) -> None:
        package = self.draft_package()
        files = {
            name: json.loads((package / name).read_text(encoding="utf-8"))
            for name in ("requirement-normalization.json", "domain-change-proposal.json", "evidence-bundle.json")
        }
        revision = {"observed_commit": None, "registry_digest": "sha256:" + "1" * 64}
        other = {"observed_commit": None, "registry_digest": "sha256:" + "2" * 64}
        complete = {
            "role": "domain-owner", "reviewer": "reviewer", "decision": "approved",
            "approved_at": "2026-10-10T00:00:00Z", "scope": "entire proposal", "proposal_revision": 1,
        }
        cases = [
            ("roles", {"requirement-normalization.json": {"required_approval_roles": [""]}},
             "requirement-normalization.json has invalid required_approval_roles"),
            ("approvals", {"domain-change-proposal.json": {"approvals": "all"}},
             "domain-change-proposal.json has invalid approvals"),
            ("entry", {"domain-change-proposal.json": {"approvals": [7]}},
             "domain-change-proposal.json has invalid approval entry"),
            ("fields", {"domain-change-proposal.json": {"approvals": [{**complete, "scope": ""}]}},
             "an approval requires role, reviewer, approved_at, and scope"),
            ("binding", {"domain-change-proposal.json": {"approvals": [{**complete, "base_registry_revision": other}]}},
             "an approval must bind to the proposal base_registry_revision"),
            ("base", {"domain-change-proposal.json": {"status": "submitted"}},
             "a submitted, approved, or applied proposal requires a Registry digest base revision"),
            ("retained", {"domain-change-proposal.json": {"status": "submitted", "base_registry_revision": revision},
                          "evidence-bundle.json": {"registry_revision": other}},
             "evidence-bundle.json must retain the proposal base_registry_revision"),
            ("missing role", {"domain-change-proposal.json": {"status": "approved"}},
             "an approved or applied proposal lacks required approvals: domain-owner"),
            ("verified_at", {"domain-change-proposal.json": {"status": "verified"}},
             "a verified proposal requires verified_at"),
            ("applied_at", {"domain-change-proposal.json": {"status": "applied"}},
             "an applied proposal requires applied_at"),
            ("applied revision", {"domain-change-proposal.json": {"status": "applied"}},
             "an applied proposal requires applied_registry_revision"),
        ]
        for label, changes, message in cases:
            with self.subTest(label):
                for name, value in files.items():
                    (package / name).write_text(json.dumps({**value, **changes.get(name, {})}), encoding="utf-8")
                self.assertIn(message, validate_change_package(package, self.repo / "memory"))
        other_revision = {**complete, "scope": "", "proposal_revision": 2}
        for name, value in files.items():
            changed = {"approvals": [other_revision]} if name == "domain-change-proposal.json" else {}
            (package / name).write_text(json.dumps({**value, **changed}), encoding="utf-8")
        self.assertNotIn(
            "an approval requires role, reviewer, approved_at, and scope",
            validate_change_package(package, self.repo / "memory"),
        )

    def test_previewing_does_not_ask_for_fields_the_stage_commands_write(self) -> None:
        package = self.draft_package()
        for status in ("submitted", "verified", "applied"):
            with self.subTest(status=status):
                errors = "; ".join(
                    validate_change_package(package, self.repo / "memory", status)
                )
                for written_by_the_command in (
                    "Registry digest base revision",
                    "retain the proposal base_registry_revision",
                    "requires verified_at",
                    "requires applied_at",
                    "requires applied_registry_revision",
                ):
                    self.assertNotIn(written_by_the_command, errors)

    def test_material_proposal_requires_implementation_design(self) -> None:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal.pop("implementation_design")
        path.write_text(json.dumps(proposal), encoding="utf-8")
        errors = validate_change_package(package, self.repo / "memory")
        self.assertTrue(
            any("requires implementation_design" in error for error in errors)
        )

    def test_proof_obligation_must_exist_in_test_obligations(self) -> None:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["implementation_design"]["proof_obligations"] = ["OB-404"]
        path.write_text(json.dumps(proposal), encoding="utf-8")
        errors = validate_change_package(package, self.repo / "memory")
        self.assertIn(
            "implementation_design references an unknown proof obligation", errors
        )

    def test_verified_material_proposal_requires_counterfactual_evidence(self) -> None:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["status"] = "verified"
        proposal["verified_at"] = "2026-01-01T00:00:00Z"
        proposal["base_registry_revision"] = {
            "observed_commit": None,
            "registry_digest": "sha256:" + "0" * 64,
        }
        path.write_text(json.dumps(proposal), encoding="utf-8")
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["registry_revision"] = proposal["base_registry_revision"]
        evidence["counterfactual_check"] = None
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        errors = validate_change_package(package, self.repo / "memory")
        self.assertIn(
            "a material verified proposal requires counterfactual_check evidence",
            errors,
        )

    def test_an_obligation_source_type_error_names_the_types_that_are_allowed(
        self,
    ) -> None:
        package = self.written_package(
            test_obligations={
                "requirement_id": "REQ-1",
                "proposal_id": "PRO-1",
                "obligations": [
                    {
                        "id": "OB-1",
                        "source_type": "architecture",
                        "source_id": "AC-1",
                        "expected_outcome": "A negative total is rejected.",
                        "check": "test command",
                        "status": "executed",
                    }
                ],
            }
        )
        errors = validate_change_package(package, self.repo / "memory")
        named = next(
            error for error in errors if error.startswith("test obligation OB-1")
        )
        self.assertIn("'architecture'", named)
        for allowed in ("acceptance-criterion", "rule", "contract", "invariant"):
            self.assertIn(allowed, named)

    def test_an_incomplete_source_id_is_reported_separately_from_an_unknown_source_type(
        self,
    ) -> None:
        package = self.written_package(
            test_obligations={
                "requirement_id": "REQ-1",
                "proposal_id": "PRO-1",
                "obligations": [
                    {
                        "id": "OB-1",
                        "source_type": "acceptance-criterion",
                        "source_id": "<stable-id>",
                        "expected_outcome": "A negative total is rejected.",
                        "check": "test command",
                        "status": "executed",
                    }
                ],
            }
        )
        errors = validate_change_package(package, self.repo / "memory")
        named = next(
            error for error in errors if error.startswith("test obligation OB-1")
        )
        self.assertIn("incomplete source_id", named)
        self.assertNotIn("source_type", named)

    def test_an_uncovered_acceptance_criterion_is_told_what_to_add(self) -> None:
        package = self.written_package(
            test_obligations={
                "requirement_id": "REQ-1",
                "proposal_id": "PRO-1",
                "obligations": [
                    {
                        "id": "OB-1",
                        "source_type": "invariant",
                        "source_id": "PRO-1",
                        "expected_outcome": "A negative total is rejected.",
                        "check": "test command",
                        "status": "executed",
                    }
                ],
            }
        )
        errors = validate_change_package(package, self.repo / "memory")
        named = next(
            error for error in errors if error.startswith("acceptance criterion AC-1")
        )
        self.assertIn("source_type is acceptance-criterion", named)
        self.assertIn("source_id is AC-1", named)

    def test_a_not_applicable_counterfactual_is_refused_when_a_rule_changes(self) -> None:
        verdicts = self.counterfactual_verdicts({"status": "not_applicable", "reason": "only renames a term"})
        self.assertEqual(1, len(verdicts), verdicts)
        self.assertIn("may be not_applicable only when the proposal changes no rule", verdicts[0])

    def test_a_not_applicable_counterfactual_needs_a_reason_when_no_behavior_changes(self) -> None:
        self.assertEqual(
            ["a not_applicable counterfactual_check requires a reason"],
            self.counterfactual_verdicts({"status": "not_applicable"}, updates=self.vocabulary_update()),
        )

    def test_a_reasoned_not_applicable_counterfactual_passes_when_no_behavior_changes(self) -> None:
        check = {"status": "not_applicable", "reason": "only renames a term"}
        self.assertEqual([], self.counterfactual_verdicts(check, updates=self.vocabulary_update()))

    def test_a_counterfactual_must_target_a_planned_proof_obligation(self) -> None:
        self.assertIn(self.UNPLANNED_MESSAGE, self.counterfactual_verdicts(self.UNPLANNED_CHECK))

    def test_an_applied_package_is_not_held_to_the_planned_proof_obligation_rule(self) -> None:
        self.assertNotIn(self.UNPLANNED_MESSAGE, self.counterfactual_verdicts(self.UNPLANNED_CHECK, status="applied"))

    def test_supersedes_takes_one_id_or_a_list_of_distinct_ids(self) -> None:
        package = self.draft_package()
        requirement = json.loads((package / "requirement-normalization.json").read_text(encoding="utf-8"))
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        message = "domain-change-proposal.json supersedes must be a proposal id or a list of distinct proposal ids"
        for value, refused in (
            ("PRO-0", False), (["PRO-0"], False), (["PRO-0", "PRO-1"], False),
            ([], True), (["PRO-0", "PRO-0"], True), (["PRO-0", ""], True), (7, True),
        ):
            with self.subTest(supersedes=value):
                errors, _ = requirement_and_proposal_errors(requirement, proposal | {"supersedes": value})
                self.assertEqual(refused, message in errors)

    def test_ids_the_package_itself_adds_count_as_known(self) -> None:
        shipping = {"id": "shipping", "name": "Shipping", "responsibility": "Own parcels."}
        rule = {"id": "order-total", "contexts": ["orders"], "statement": "An order total is non-negative."}
        errors = self.proposal_reference_errors(
            "draft",
            affected_contexts=["orders", "shipping"],
            rule_ids=["order-total"],
            registry_updates=[
                {"operation": "upsert", "asset": "contexts", "record": shipping},
                {"operation": "upsert", "asset": "rules", "record": rule},
            ],
        )
        self.assertFalse([error for error in errors if "unknown registry" in error], errors)

    def test_an_id_neither_registered_nor_added_by_the_package_is_unknown(self) -> None:
        errors = self.proposal_reference_errors("draft", affected_contexts=["orders", "ghost"])
        self.assertIn("domain-change-proposal.json references an unknown registry context", errors)

    def test_an_applied_package_may_name_an_id_the_registry_later_retired(self) -> None:
        errors = self.proposal_reference_errors("applied", contract_ids=["retired-v1"])
        self.assertNotIn("domain-change-proposal.json references an unknown registry contract", errors)

    def test_a_draft_reports_what_its_updates_would_break_once_applied(self) -> None:
        rule = {"id": "order-total", "contexts": ["ghost"], "statement": "An order total is non-negative."}
        errors = self.proposal_reference_errors(
            "draft", registry_updates=[{"operation": "upsert", "asset": "rules", "record": rule}]
        )
        self.assertIn("once applied: rules.json:order-total references unknown context", errors)

    def test_a_draft_does_not_repeat_an_error_the_registry_already_has(self) -> None:
        self.seed("rules.json", [{"id": "old", "contexts": ["ghost"], "statement": "Kept."}])
        errors = self.proposal_reference_errors("draft")
        self.assertFalse([error for error in errors if error.startswith("once applied:")], errors)

    def test_with_a_repo_root_a_draft_must_cite_current_evidence_in_its_updates(self) -> None:
        source = self.repo / "fact.md"
        source.write_text("fact\n", encoding="utf-8")
        package = self.updated_rule_citing(citation(self.repo, "fact.md", 1, 1))
        source.write_text("changed\n", encoding="utf-8")
        message = (
            "registry update rules/order-total cites evidence that is stale: the cited excerpt is no longer "
            "in fact.md; check that the fact still holds, then recite it"
        )
        self.assertIn(message, validate_change_package(package, self.repo / "memory", repo_root=self.repo))
        self.assertNotIn(message, validate_change_package(package, self.repo / "memory"))

    def test_with_a_repo_root_current_evidence_in_the_updates_passes(self) -> None:
        (self.repo / "fact.md").write_text("fact\n", encoding="utf-8")
        package = self.updated_rule_citing(citation(self.repo, "fact.md", 1, 1))
        errors = validate_change_package(package, self.repo / "memory", repo_root=self.repo)
        self.assertFalse([error for error in errors if "cites evidence" in error], errors)

    def test_validation_names_a_list_key_given_as_an_asset_and_the_asset_that_holds_it(self) -> None:
        [error] = self.update_errors([{"operation": "upsert", "asset": "dependencies", "record": {"id": "x"}}])
        self.assertIn("registry update names unknown asset 'dependencies'; valid assets: aggregates,", error)
        self.assertIn("'dependencies' is the list inside dependency-policies.json, whose asset is 'dependency-policies'", error)

    def test_validation_names_an_unknown_asset_without_a_holder_hint(self) -> None:
        [error] = self.update_errors([{"operation": "remove", "asset": "nonsense", "id": "x"}])
        self.assertIn("registry update names unknown asset 'nonsense'; valid assets:", error)
        self.assertNotIn("is the list inside", error)

    def test_validation_names_the_asset_of_an_update_without_a_record(self) -> None:
        self.assertEqual(
            ["registry update for rules needs a record with an id, or a remove with an id"],
            self.update_errors([{"operation": "upsert", "asset": "rules"}]),
        )

    def test_validation_refuses_registry_updates_that_are_not_a_list(self) -> None:
        package = self.drafted_with_updates({"operation": "upsert"})
        self.assertIn(
            "domain-change-proposal.json registry_updates must be a list",
            validate_change_package(package, self.repo / "memory"),
        )


if __name__ == "__main__":
    unittest.main()
