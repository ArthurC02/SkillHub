from __future__ import annotations

import argparse
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.cli_commands import policy_assignments
from domain_registry.registry import (
    coverage,
    migrate_registry,
    record_by_id,
    validate,
    verify_evidence,
)
from test_support import RegistryTestCase, amend_policy, use_json_layout


class RegistryPolicyTest(RegistryTestCase):
    def test_migrating_restores_a_missing_asset_without_touching_the_others(
        self,
    ) -> None:
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        use_json_layout(self.repo / "memory")
        (self.repo / "memory" / "registry" / "events.json").unlink()
        created = migrate_registry(self.repo / "memory")
        self.assertEqual(created, ["events.json"])
        record = record_by_id(self.repo / "memory", "contexts", "orders")
        self.assertIsNotNone(record)
        assert record is not None
        self.assertEqual(record["name"], "Orders")

    def test_an_examined_context_leaves_the_gap_list_but_an_unexamined_one_stays(
        self,
    ) -> None:
        self.context_declaring(
            "aggregates",
            "Invariants are thin here; a transaction script is the declared pattern.",
        )
        report = coverage(self.repo / "memory")
        self.assertEqual(report["gaps"]["without_aggregate"], ["billing"])
        self.assertEqual(report["confirmed_absent"]["without_aggregate"], ["orders"])

    def test_an_absence_confirmed_for_one_asset_does_not_answer_another(self) -> None:
        self.context_declaring("aggregates", "Invariants are thin here.")
        report = coverage(self.repo / "memory")
        self.assertIn("orders", report["gaps"]["without_rule"])
        self.assertEqual(report["confirmed_absent"]["without_rule"], [])

    def test_a_confirmed_absence_without_a_reason_is_rejected(self) -> None:
        self.context_declaring("aggregates", "   ")
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(any("without a reason" in error for error in errors), errors)

    def test_a_confirmed_absence_the_records_contradict_is_rejected(self) -> None:
        self.context_declaring(
            "rules", "No business rule is written down for this context."
        )
        self.seed(
            "rules.json",
            [
                {
                    "id": "order-total",
                    "contexts": ["orders"],
                    "statement": "An order total is non-negative.",
                }
            ],
        )
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(
            any("rules records name it" in error for error in errors), errors
        )

    def test_a_confirmed_absence_of_an_unknown_asset_names_the_ones_allowed(
        self,
    ) -> None:
        self.context_declaring("decisions", "Decisions are elsewhere.")
        errors = validate(self.repo / "memory", None, False)
        named = next(error for error in errors if "unknown asset" in error)
        for allowed in ("aggregates", "contracts", "events", "rules", "vocabulary"):
            self.assertIn(allowed, named)

    def test_the_command_line_takes_one_field_or_several_sets_but_not_both(self) -> None:
        def parsed(**values) -> argparse.Namespace:
            return argparse.Namespace(**{"field": None, "value": None, "set": [], **values})

        self.assertEqual(
            [("ci_requirement", "required")],
            policy_assignments(parsed(field="ci_requirement", value="required")),
        )
        self.assertEqual(
            [("review_mode", "scm-verified"), ("authorized_signers", "a=b,c")],
            policy_assignments(parsed(set=["review_mode=scm-verified", "authorized_signers=a=b,c"])),
        )
        for refused in (
            parsed(),
            parsed(field="ci_requirement"),
            parsed(field="ci_requirement", value="required", set=["review_mode=scm-verified"]),
            parsed(set=["review_mode"]),
        ):
            with self.subTest(refused=vars(refused)), self.assertRaises(ValueError):
                policy_assignments(refused)

    def test_a_policy_can_leave_local_draft_only_for_external_review(self) -> None:
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone until a reviewer exists.",
        )
        with self.assertRaisesRegex(ValueError, "requires the verifier"):
            amend_policy(
                self.repo / "memory",
                "review_mode",
                "scm-verified",
                "A reviewer exists now.",
            )
        self.assertEqual(self.stored_policy()["review_mode"], "local-draft-only")
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "scm-verified",
            "A reviewer exists now.",
            verifier="github-pr",
        )
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")
        self.assertEqual(
            self.stored_policy()["review_governance"]["verifier"], "github-pr"
        )

    def test_a_repository_without_ci_can_adopt_and_rotate_a_signer(self) -> None:
        root = self.repo / "memory"
        amend_policy(
            root, "authorized_signers", "SHA256:first", "Register the maintainer key."
        )
        amend_policy(root, "review_trigger", "git-push", "Signing is enforced on push.")
        amend_policy(
            root,
            "review_verifier",
            "git-signed-commit",
            "No CI reviewer exists, so the maintainer signs instead.",
        )
        governance = self.stored_policy()["review_governance"]
        self.assertEqual("git-signed-commit", governance["verifier"])
        self.assertEqual(["SHA256:first"], governance["authorized_signers"])
        amend_policy(
            root, "authorized_signers", "SHA256:second", "Rotate a compromised key."
        )
        self.assertEqual(
            ["SHA256:second"],
            self.stored_policy()["review_governance"]["authorized_signers"],
        )
        with self.assertRaisesRegex(ValueError, "amended policy is invalid"):
            amend_policy(root, "authorized_signers", "   ", "Leave nobody able to sign.")
        self.assertEqual(
            ["SHA256:second"],
            self.stored_policy()["review_governance"]["authorized_signers"],
        )

    def test_amending_without_a_reason_leaves_the_policy_alone(self) -> None:
        with self.assertRaisesRegex(ValueError, "requires a reason"):
            amend_policy(self.repo / "memory", "review_mode", "local-draft-only", "   ")
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")

    def test_an_unamendable_field_names_the_ones_that_are(self) -> None:
        with self.assertRaises(ValueError) as raised:
            amend_policy(self.repo / "memory", "limits", "0", "Loosen the limits.")
        for amendable in (
            "review_mode",
            "storage_mode",
            "data_classification",
            "source_authority",
        ):
            self.assertIn(amendable, str(raised.exception))

    def test_an_invalid_value_is_refused_before_anything_is_written(self) -> None:
        with self.assertRaisesRegex(ValueError, "amended policy is invalid"):
            amend_policy(
                self.repo / "memory", "review_mode", "whatever-i-like", "Try it."
            )
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")

    def test_an_initialization_whose_policy_is_refused_leaves_no_directory_so_the_corrected_run_succeeds(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "rules.md").write_text("rules", encoding="utf-8")
        destination = self.repo / "domain-memory"
        arguments = [
            "init-domain-memory", "--repo-root", str(self.repo), "--output", str(destination),
            "--source", str(self.repo / "docs"), "--storage-mode", "ignored",
            "--data-classification", "internal", "--review-mode", "local-draft-only", "--source-authority", "test",
        ]
        refused = self.run_cli(*arguments, "--review-verifier", "github-pr")
        self.assertEqual(
            (1, "ERROR: invalid policy: local-draft-only policy must not select a review verifier"),
            (refused.returncode, refused.stderr.strip()),
        )
        self.assertFalse(destination.exists())
        corrected = self.run_cli(*arguments)
        self.assertEqual(0, corrected.returncode, corrected.stderr)
        self.assertTrue((destination / "domain-memory-policy.json").is_file())

    def test_verify_evidence_names_the_record_of_each_citation_and_filters_by_path(self) -> None:
        use_json_layout(self.repo / "memory")
        self.seed_two_cited_rules()
        report = verify_evidence(self.repo / "memory", self.repo, ["fact.md"])
        self.assertEqual(
            [("rules", "moved", "shifted"), ("rules", "kept", "current")],
            [(item["asset"], item["id"], item["status"]) for item in report["results"]],
        )
        self.assertEqual(3, len(verify_evidence(self.repo / "memory", self.repo)["results"]))


if __name__ == "__main__":
    unittest.main()
