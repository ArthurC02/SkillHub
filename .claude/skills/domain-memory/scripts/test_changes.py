from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import _changes_behavior as changes_behavior
from domain_registry.changes import _obligation_errors as obligation_errors
from domain_registry.changes import _requirement_and_proposal_errors as requirement_and_proposal_errors
from domain_registry.changes import (
    shifted_citation_notes,
    EVIDENCE_FILE,
    PROPOSAL_FILE,
    as_if_moved_to,
    implementation_design_errors,
    init_change_package,
    missing_approval_roles,
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
            ("invariants not a list", {**material, "implementation_design": {**design, "invariants_preserved": "kept"}},
             ["implementation_design requires a valid invariants_preserved list"]),
            ("alternatives not a list", {**material, "implementation_design": {**design, "rejected_alternatives": None}},
             ["implementation_design requires a valid rejected_alternatives list"]),
            ("routine with an incomplete design",
             {"change_classification": "routine", "implementation_design": {**design, "decision": " "}},
             ["implementation_design requires a completed decision"]),
            ("no decision", {**material, "implementation_design": {**design, "decision": " "}},
             ["implementation_design requires a completed decision"]),
            ("proof obligations not a list", {**material, "implementation_design": {**design, "proof_obligations": "OB-1"}},
             ["implementation_design requires proof obligation IDs"]),
            ("unknown obligation", {**material, "implementation_design": {**design, "proof_obligations": ["OB-9"]}},
             ["implementation_design references an unknown proof obligation"]),
            ("no counterfactual", {**material, "implementation_design": {**design, "counterfactual_check": ""}},
             ["implementation_design requires a counterfactual check"]),
            ("material without obligations", {**material, "implementation_design": {**design, "proof_obligations": []}},
             ["a material proposal requires at least one proof obligation"]),
            ("material without forces", {**material, "implementation_design": {**design, "domain_forces": []}},
             ["a material proposal requires at least one entry in domain_forces"]),
            ("material without invariants", {**material, "implementation_design": {**design, "invariants_preserved": []}},
             ["a material proposal requires at least one entry in invariants_preserved"]),
            ("material without alternatives", {**material, "implementation_design": {**design, "rejected_alternatives": []}},
             ["a material proposal requires at least one entry in rejected_alternatives"]),
            ("routine with empty lists", {"change_classification": "routine", "implementation_design": {
                **design, "domain_forces": [], "invariants_preserved": [], "rejected_alternatives": []}}, []),
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
        self.assertEqual(
            [
                "domain-change-proposal.json",
                "draft-pr.md",
                "evidence-bundle.json",
                "requirement-normalization.json",
                "test-obligations.json",
            ],
            sorted(path.name for path in package.iterdir()),
        )
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

    def test_a_proposal_id_a_sibling_package_uses_is_refused_before_anything_is_written(self) -> None:
        init_change_package(self.repo / "first", "REQ-7", "PRO-7")
        with self.assertRaisesRegex(ValueError, "^proposal_id PRO-7 is already used by first; choose another$"):
            init_change_package(self.repo / "second", "REQ-7", "PRO-7")
        self.assertFalse((self.repo / "second").exists())

    def test_a_package_that_fails_halfway_is_removed(self) -> None:
        package = self.repo / "half"
        with patch("domain_registry.changes._fill_identifiers", side_effect=PermissionError(5, "Access is denied")), \
                self.assertRaises(PermissionError):
            init_change_package(package, "REQ-7", "PRO-7")
        self.assertFalse(package.exists())

    def test_a_package_started_with_a_proposer_is_not_asked_for_one(self) -> None:
        package = self.repo / "named"
        init_change_package(package, "REQ-7", "PRO-7", "agent-1")
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("agent-1", proposal["proposer"])
        self.assertEqual(
            [
                "each acceptance criterion requires a completed id and statement",
                "domain-change-proposal.json requires affected_contexts",
                "each test obligation requires a completed id",
                "a material proposal requires at least one entry in domain_forces",
                "a material proposal requires at least one entry in invariants_preserved",
                "a material proposal requires at least one entry in rejected_alternatives",
                "implementation_design requires a completed decision",
                "implementation_design requires a counterfactual check",
                "a material proposal requires at least one proof obligation",
            ],
            validate_change_package(package, self.repo / "memory"),
        )

    def test_a_fresh_package_lists_only_what_the_template_leaves_blank(self) -> None:
        package = self.repo / "fresh"
        init_change_package(package)
        blank_template = [
            "requirement-normalization.json requires a completed requirement_id",
            "domain-change-proposal.json requires a completed proposal_id",
            "domain-change-proposal.json requires a completed proposer",
            "domain-change-proposal.json must reference the requirement_id",
            "test-obligations.json must reference the requirement_id",
            "evidence-bundle.json must reference the requirement_id",
            "test-obligations.json must reference the proposal_id",
            "evidence-bundle.json must reference the proposal_id",
            "each acceptance criterion requires a completed id and statement",
            "domain-change-proposal.json requires affected_contexts",
            "each test obligation requires a completed id",
            "a material proposal requires at least one entry in domain_forces",
            "a material proposal requires at least one entry in invariants_preserved",
            "a material proposal requires at least one entry in rejected_alternatives",
            "implementation_design requires a completed decision",
            "implementation_design requires a counterfactual check",
            "a material proposal requires at least one proof obligation",
        ]
        for status in ("submitted", "draft"):
            with self.subTest(status=status):
                self.assertEqual(blank_template, validate_change_package(package, self.repo / "memory", status))

    def test_previewing_approved_lists_every_missing_piece_of_that_stage_at_once(self) -> None:
        package = self.draft_package()
        errors = validate_change_package(package, self.repo / "memory", "approved")
        self.assertEqual(
            [
                "an approved or applied proposal lacks required approvals: domain-owner",
                "an approved or applied proposal requires SCM attestation",
                "once applied: rules/order-total would be reviewed by nobody; the proposal records no approval and the "
                "Registry holds no verified rules/order-total, so record-approval first",
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
            ("blank role", {"domain-change-proposal.json": {"approvals": [{**complete, "role": ""}]}},
             "an approval requires role, reviewer, approved_at, and scope"),
            ("blank reviewer", {"domain-change-proposal.json": {"approvals": [{**complete, "reviewer": " "}]}},
             "an approval requires role, reviewer, approved_at, and scope"),
            ("blank approved_at", {"domain-change-proposal.json": {"approvals": [{**complete, "approved_at": ""}]}},
             "an approval requires role, reviewer, approved_at, and scope"),
            ("binding", {"domain-change-proposal.json": {"approvals": [{**complete, "base_registry_revision": other}]}},
             "an approval must bind to the proposal base_registry_revision"),
            ("rejected approval leaves the role missing",
             {"domain-change-proposal.json": {"status": "approved", "approvals": [{**complete, "decision": "rejected"}]}},
             "an approved or applied proposal lacks required approvals: domain-owner"),
            ("stale-revision approval leaves the role missing",
             {"domain-change-proposal.json": {"status": "approved", "approvals": [{**complete, "proposal_revision": 2}]}},
             "an approved or applied proposal lacks required approvals: domain-owner"),
            ("unbound approval leaves the role missing",
             {"domain-change-proposal.json": {
                 "status": "approved", "approvals": [{**complete, "base_registry_revision": other}]}},
             "an approved or applied proposal lacks required approvals: domain-owner"),
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
        self.assertIn(
            "a material proposal requires implementation_design with domain forces, decision, invariants, "
            "alternatives, proof obligations, and counterfactual check",
            errors,
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
        self.assertIn("once applied: rules/order-total references unknown context ghost; add the Context first", errors)

    def decision_evidence_errors(self, *evidence: object) -> list[str]:
        (self.repo / "src").mkdir(exist_ok=True)
        (self.repo / "src" / "owner.py").write_text("x\n", encoding="utf-8")
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["boundary_decision"]["evidence"] = list(evidence)
        path.write_text(json.dumps(proposal), encoding="utf-8")
        errors = validate_change_package(package, self.repo / "memory", repo_root=self.repo)
        return [error for error in errors if error.startswith("boundary_decision evidence")]

    def test_decision_evidence_naming_a_missing_file_is_refused(self) -> None:
        self.assertEqual(
            ["boundary_decision evidence names src/gone.py, which is not a file in the repository"] * 2,
            self.decision_evidence_errors("src/gone.py:6-45", {"path": "src/gone.py", "lines": {"start": 1, "end": 1}}),
        )

    def test_decision_evidence_naming_a_file_that_exists_or_written_as_prose_passes(self) -> None:
        self.assertEqual([], self.decision_evidence_errors("src/owner.py:1", "src/owner.py", "Billing owns invoices.", "ADR-12"))

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

    def test_each_test_obligation_defect_is_named(self) -> None:
        good = {
            "id": "OB-1", "source_type": "acceptance-criterion", "source_id": "AC-1",
            "expected_outcome": "Totals hold.", "check": "run it", "status": "executed",
        }
        rule_obligation = {**good, "id": "OB-2", "source_type": "rule", "source_id": "order-total"}
        contract_obligation = {**good, "id": "OB-3", "source_type": "contract", "source_id": "invoice-api"}
        without_id = {key: value for key, value in good.items() if key != "id"}
        cases = [
            ("covered", [good], [], [], [], {"OB-1"}),
            ("empty list", [], [], [], ["test-obligations.json requires obligations"], set()),
            ("not an object", [good, 7], [], [], ["each test obligation requires a completed id"], {"OB-1"}),
            ("missing id", [good, without_id], [], [], ["each test obligation requires a completed id"], {"OB-1"}),
            ("blank id", [good, {**good, "id": " "}], [], [], ["each test obligation requires a completed id"],
             {"OB-1"}),
            ("duplicate id", [good, dict(good)], [], [], ["test-obligations.json contains duplicate obligation id"],
             {"OB-1"}),
            ("blank expected_outcome", [{**good, "expected_outcome": " "}], [], [],
             ["test obligation OB-1 requires expected_outcome and check"], {"OB-1"}),
            ("blank check", [{**good, "check": ""}], [], [],
             ["test obligation OB-1 requires expected_outcome and check"], {"OB-1"}),
            ("invalid status", [{**good, "status": "done"}], [], [],
             ["test obligation OB-1 has an invalid status"], {"OB-1"}),
            ("rule lacks an obligation", [good], ["order-total"], [],
             ["rule order-total lacks a test obligation"], {"OB-1"}),
            ("rule named only by an invariant obligation",
             [good, {**rule_obligation, "source_type": "invariant"}], ["order-total"], [],
             ["rule order-total lacks a test obligation"], {"OB-1", "OB-2"}),
            ("rule covered", [good, rule_obligation], ["order-total"], [], [], {"OB-1", "OB-2"}),
            ("contract lacks an obligation", [good], [], ["invoice-api"],
             ["contract invoice-api lacks a test obligation"], {"OB-1"}),
            ("contract covered", [good, contract_obligation], [], ["invoice-api"], [], {"OB-1", "OB-3"}),
            ("rule and contract covered by the criterion's obligation",
             [{**good, "also_covers": [{"source_type": "rule", "source_id": "order-total"},
                                       {"source_type": "contract", "source_id": "invoice-api"}]}],
             ["order-total"], ["invoice-api"], [], {"OB-1"}),
            ("criterion covered only through also_covers",
             [{**rule_obligation, "also_covers": [{"source_type": "acceptance-criterion", "source_id": "AC-1"}]}],
             ["order-total"], [], [], {"OB-2"}),
            ("also_covers not a list", [{**good, "also_covers": "order-total"}], ["order-total"], [],
             ["test obligation OB-1 has also_covers that is not a list of sources", "rule order-total lacks a test obligation"],
             {"OB-1"}),
            ("also_covers naming a rule the proposal does not name",
             [{**good, "also_covers": [{"source_type": "rule", "source_id": "no-such-rule"}]}], ["order-total"], [],
             ["test obligation OB-1 lists rule no-such-rule in also_covers, but the proposal names no such rule; "
              "also_covers counts only toward the acceptance criteria and the rule_ids and contract_ids the proposal names",
              "rule order-total lacks a test obligation"], {"OB-1"}),
            ("also_covers naming a criterion the requirement lacks",
             [{**good, "also_covers": [{"source_type": "acceptance-criterion", "source_id": "AC-9"}]}], [], [],
             ["test obligation OB-1 lists acceptance-criterion AC-9 in also_covers, but the proposal names no such "
              "acceptance-criterion; also_covers counts only toward the acceptance criteria and the rule_ids and "
              "contract_ids the proposal names"], {"OB-1"}),
            ("also_covers naming an invariant", [{**good, "also_covers": [{"source_type": "invariant", "source_id": "totals-hold"}]}],
             [], [], [], {"OB-1"}),
            ("also_covers a number", [{**good, "also_covers": 7}], [], [],
             ["test obligation OB-1 has also_covers that is not a list of sources"], {"OB-1"}),
            ("also_covers holding a bare id", [{**good, "also_covers": ["order-total"]}], [], [],
             ["test obligation OB-1 has also_covers that is not a list of sources"], {"OB-1"}),
            ("also_covers source of an unknown type", [{**good, "also_covers": [{"source_type": "test", "source_id": "x"}]}],
             [], [],
             ["test obligation OB-1 has source_type 'test'; a source_type names what the obligation is derived from, "
              "not the kind of check, and must be one of acceptance-criterion, contract, invariant, rule"], {"OB-1"}),
            ("also_covers source without an id", [{**good, "also_covers": [{"source_type": "rule"}]}], [], [],
             ["test obligation OB-1 has an incomplete source_id: None"], {"OB-1"}),
        ]
        for label, records, rule_ids, contract_ids, expected_errors, expected_ids in cases:
            with self.subTest(label):
                self.assertEqual(
                    (expected_errors, expected_ids),
                    obligation_errors({"obligations": records}, {"AC-1"}, rule_ids, contract_ids),
                )
        for label, document in (("no obligations key", {}), ("obligations not a list", {"obligations": "OB-1"})):
            with self.subTest(label):
                self.assertEqual(
                    (["test-obligations.json requires obligations"], set()),
                    obligation_errors(document, {"AC-1"}, [], []),
                )

    def test_only_a_complete_current_bound_approval_supplies_its_role(self) -> None:
        base = {"observed_commit": None, "registry_digest": "sha256:" + "2" * 64}
        complete = {
            "role": "domain-owner", "reviewer": "reviewer", "decision": "approved",
            "approved_at": "2026-10-10T00:00:00Z", "scope": "entire proposal", "proposal_revision": 1,
        }
        requirement = {"required_approval_roles": ["domain-owner"]}
        cases = [
            ("complete", [complete], []),
            ("no approvals", [], ["domain-owner"]),
            ("approvals not a list", "all", ["domain-owner"]),
            ("not an object", [7], ["domain-owner"]),
            ("rejected", [{**complete, "decision": "rejected"}], ["domain-owner"]),
            ("stale revision", [{**complete, "proposal_revision": 2}], ["domain-owner"]),
            ("unbound", [{**complete, "base_registry_revision": base}], ["domain-owner"]),
            ("other role", [{**complete, "role": "auditor"}], ["domain-owner"]),
            ("blank role", [{**complete, "role": " "}], ["domain-owner"]),
            ("blank reviewer", [{**complete, "reviewer": ""}], ["domain-owner"]),
            ("blank approved_at", [{**complete, "approved_at": ""}], ["domain-owner"]),
            ("blank scope", [{**complete, "scope": " "}], ["domain-owner"]),
        ]
        for label, approvals, expected in cases:
            with self.subTest(label):
                proposal = {"proposal_revision": 1, "base_registry_revision": None, "approvals": approvals}
                self.assertEqual(expected, missing_approval_roles(requirement, proposal))
        with self.subTest("two roles, one approved"):
            proposal = {"proposal_revision": 1, "base_registry_revision": None, "approvals": [complete]}
            self.assertEqual(
                ["security"],
                missing_approval_roles({"required_approval_roles": ["security", "domain-owner"]}, proposal),
            )
        with self.subTest("required roles not a list"):
            proposal = {"proposal_revision": 1, "base_registry_revision": None, "approvals": []}
            self.assertEqual([], missing_approval_roles({"required_approval_roles": "domain-owner"}, proposal))

    def test_each_source_of_a_behavior_change_is_recognised(self) -> None:
        cases = [
            ("rule ids", {"rule_ids": ["order-total"]}, True),
            ("contract ids", {"contract_ids": ["invoice-api"]}, True),
            ("rules update", {"registry_updates": [{"asset": "rules"}]}, True),
            ("contracts update", {"registry_updates": [{"asset": "contracts"}]}, True),
            ("aggregates update", {"registry_updates": [{"asset": "aggregates"}]}, True),
            ("vocabulary update", {"registry_updates": [{"asset": "vocabulary"}]}, False),
            ("no sources", {"rule_ids": [], "contract_ids": [], "registry_updates": []}, False),
            ("updates not a list", {"registry_updates": "rules"}, False),
            ("update not an object", {"registry_updates": ["rules"]}, False),
        ]
        for label, proposal, expected in cases:
            with self.subTest(label):
                self.assertIs(expected, changes_behavior(proposal))

    def verdicts_in(self, package: Path, check: dict | None, status: str, updates: list | None = None) -> list[str]:
        proposal_path = package / PROPOSAL_FILE
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        proposal["status"] = status
        if updates is not None:
            proposal["registry_updates"] = updates
        proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
        evidence_path = package / EVIDENCE_FILE
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["counterfactual_check"] = check
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        return [error for error in validate_change_package(package, None) if "counterfactual" in error]

    def test_each_counterfactual_defect_is_named(self) -> None:
        passed = {
            "status": "passed", "obligation_id": "OB-1", "mutated_protection": "Remove the check.",
            "failure_evidence": "OB-1 failed.", "restored": True,
        }
        requires = (
            "counterfactual_check requires passed status, a known obligation_id, mutated_protection, "
            "failure_evidence, and restored=true"
        )
        cases = [
            ("complete", "verified", passed, None, []),
            ("no check recorded", "verified", None, None,
             ["a material verified proposal requires counterfactual_check evidence"]),
            ("status not passed", "verified", {**passed, "status": "failed"}, None, [requires]),
            ("not restored", "verified", {**passed, "restored": False}, None, [requires]),
            ("restored is not a boolean", "verified", {**passed, "restored": "true"}, None, [requires]),
            ("unknown obligation_id", "applied", {**passed, "obligation_id": "OB-9"}, None, [requires]),
            ("blank mutated_protection", "verified", {**passed, "mutated_protection": " "}, None, [requires]),
            ("blank failure_evidence", "verified", {**passed, "failure_evidence": ""}, None, [requires]),
            ("not_applicable with a blank reason", "verified", {"status": "not_applicable", "reason": " "},
             self.vocabulary_update(), ["a not_applicable counterfactual_check requires a reason"]),
            ("a list of complete checks", "verified", [passed], None, []),
            ("a list with an incomplete check", "verified", [{**passed, "restored": False}], None,
             [requires]),
            ("a list whose second check targets an unplanned obligation", "verified",
             [passed, {**passed, "obligation_id": "OB-9"}], None, [self.UNPLANNED_MESSAGE]),
            ("a list targeting one obligation twice", "verified", [passed, passed], None,
             ["each counterfactual_check in a list targets a different obligation"]),
            ("an empty list", "verified", [], None,
             ["a material verified proposal requires counterfactual_check evidence"]),
            ("a list holding a non-object", "verified", [passed, "OB-1 failed."], None,
             ["a material verified proposal requires counterfactual_check evidence"]),
            ("superseded is not asked for one", "superseded", self.UNPLANNED_CHECK, None, []),
            ("rejected is not asked for one", "rejected", self.UNPLANNED_CHECK, None, []),
        ]
        package = self.draft_package()
        for label, status, check, updates, expected in cases:
            with self.subTest(label):
                self.assertEqual(expected, self.verdicts_in(package, check, status, updates))

    def test_shifted_citations_are_a_note_per_record_not_an_error(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        (self.repo / "NOTES.md").write_text("another fact\n", encoding="utf-8")
        proposal_path = package / PROPOSAL_FILE
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["record"]["evidence"] = [
            citation(self.repo, "README.md", 1, 1), citation(self.repo, "NOTES.md", 1, 1),
        ]
        proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
        (self.repo / "README.md").write_text("moved down\nevidence\n", encoding="utf-8")
        (self.repo / "NOTES.md").write_text("moved down\nanother fact\n", encoding="utf-8")
        self.assertEqual(
            ([], ["NOTE: registry update rules/order-total cites 2 citations whose lines are unchanged but now sit elsewhere in "
                  "NOTES.md, README.md; this passes every check and needs no Change Package of its own; when the record is "
                  "next touched, relocate-evidence re-cites them"]),
            (validate_change_package(package, self.repo / "memory", None, self.repo),
             shifted_citation_notes(proposal, self.repo)),
        )

    def test_a_routine_package_at_verified_needs_no_counterfactual(self) -> None:
        package = self.draft_package()
        revision = {"observed_commit": None, "registry_digest": "sha256:" + "0" * 64}
        proposal_path = package / PROPOSAL_FILE
        evidence_path = package / EVIDENCE_FILE
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence.update({"registry_revision": revision, "counterfactual_check": None})
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        nobody = ("once applied: rules/order-total would be reviewed by nobody; the proposal records no approval and the "
                  "Registry holds no verified rules/order-total, so record-approval first")
        cases = [
            ("material", ["a material verified proposal requires counterfactual_check evidence", nobody]),
            ("routine", [nobody]),
        ]
        for classification, expected in cases:
            with self.subTest(classification):
                proposal.update({
                    "change_classification": classification, "status": "verified",
                    "verified_at": "2026-01-01T00:00:00Z", "base_registry_revision": revision,
                })
                proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
                self.assertEqual(expected, validate_change_package(package, self.repo / "memory"))

    def test_only_a_settled_package_may_name_an_id_the_registry_later_retired(self) -> None:
        package = self.draft_package()
        proposal_path = package / PROPOSAL_FILE
        obligations_path = package / "test-obligations.json"
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        obligations = json.loads(obligations_path.read_text(encoding="utf-8"))
        obligations["obligations"].append({
            "id": "OB-2", "source_type": "contract", "source_id": "retired-v1",
            "expected_outcome": "The retired contract still holds.", "check": "test command", "status": "executed",
        })
        obligations_path.write_text(json.dumps(obligations), encoding="utf-8")
        cases = [
            ("superseded", []),
            ("rejected", ["domain-change-proposal.json references an unknown registry contract"]),
            ("draft", ["domain-change-proposal.json references an unknown registry contract"]),
        ]
        for status, expected in cases:
            with self.subTest(status):
                proposal.update({"status": status, "contract_ids": ["retired-v1"]})
                proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
                self.assertEqual(expected, validate_change_package(package, self.repo / "memory"))

    def test_previewing_keeps_a_base_revision_and_timestamps_that_are_already_real(self) -> None:
        real = {"observed_commit": None, "registry_digest": "sha256:" + "1" * 64}
        proposal = {
            "status": "draft", "base_registry_revision": real, "verified_at": "2026-01-01T00:00:00Z",
            "applied_at": "2026-01-02T00:00:00Z", "applied_registry_revision": real,
        }
        for status in ("submitted", "verified", "applied"):
            with self.subTest(status):
                moved = as_if_moved_to(status, {PROPOSAL_FILE: proposal, EVIDENCE_FILE: {"registry_revision": real}})
                self.assertEqual({**proposal, "status": status}, moved[PROPOSAL_FILE])
                self.assertEqual({"registry_revision": real}, moved[EVIDENCE_FILE])

    def test_previewing_replaces_a_base_revision_and_timestamps_that_are_unusable(self) -> None:
        stand_in = {"observed_commit": None, "registry_digest": "sha256:" + "0" * 64}
        proposal = {
            "status": "draft", "base_registry_revision": {"registry_digest": "sha256:short"},
            "verified_at": "yesterday", "applied_at": None, "applied_registry_revision": None,
        }
        cases = [
            ("verified", {
                "status": "verified", "base_registry_revision": stand_in, "verified_at": "1970-01-01T00:00:00Z",
                "applied_at": None, "applied_registry_revision": None,
            }),
            ("applied", {
                "status": "applied", "base_registry_revision": stand_in, "verified_at": "yesterday",
                "applied_at": "1970-01-01T00:00:00Z", "applied_registry_revision": stand_in,
            }),
        ]
        for status, expected in cases:
            with self.subTest(status):
                moved = as_if_moved_to(status, {PROPOSAL_FILE: proposal, EVIDENCE_FILE: {"registry_revision": None}})
                self.assertEqual(expected, moved[PROPOSAL_FILE])
                self.assertEqual({"registry_revision": stand_in}, moved[EVIDENCE_FILE])

    def test_validation_refuses_registry_updates_that_are_not_a_list(self) -> None:
        package = self.drafted_with_updates({"operation": "upsert"})
        self.assertIn(
            "domain-change-proposal.json registry_updates must be a list",
            validate_change_package(package, self.repo / "memory"),
        )


if __name__ == "__main__":
    unittest.main()
