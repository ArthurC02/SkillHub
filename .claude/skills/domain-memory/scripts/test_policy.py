from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.policy import amend_policy_fields, validate_policy
from test_support import RegistryTestCase, amend_policy, stored_policy_of


class PolicyTest(RegistryTestCase):
    def test_each_policy_defect_is_named_and_governance_reports_only_its_first(self) -> None:
        sound = stored_policy_of(self.repo)
        self.assertEqual([], validate_policy(sound))
        signed = {"verifier": "git-signed-commit", "trigger": "git-push", "ci_requirement": "none",
                  "authorized_signers": ["SHA256:key"]}
        cases = [
            ("format", {"format": "domain-memory-policy/v2"}, "policy has an invalid format"),
            ("storage", {"storage_mode": "cloud"}, "policy has an invalid storage_mode"),
            ("classification", {"data_classification": "secret"}, "policy has an invalid data_classification"),
            ("review mode", {"review_mode": "maybe"}, "policy has an invalid review_mode"),
            ("no governance", {"review_governance": None}, "policy requires review_governance"),
            ("unknown verifier", {"review_governance": {**sound["review_governance"], "verifier": "fax"}},
             "policy has invalid review_governance"),
            ("blank signer", {"review_governance": {**signed, "authorized_signers": [" "]}},
             "policy has invalid review_governance"),
            ("local with verifier", {"review_mode": "local-draft-only"},
             "local-draft-only policy must not select a review verifier"),
            ("scm without verifier",
             {"review_governance": {**sound["review_governance"], "verifier": "none", "trigger": "none"}},
             "scm-verified policy requires a review verifier"),
            ("signed without signers", {"review_governance": {**signed, "authorized_signers": []}},
             "git-signed-commit policy requires authorized_signers"),
            ("signed without push", {"review_governance": {**signed, "trigger": "git-commit"}},
             "git-signed-commit policy requires git-push enforcement"),
            ("signed without signers or push",
             {"review_governance": {**signed, "authorized_signers": [], "trigger": "git-commit"}},
             "git-signed-commit policy requires authorized_signers"),
            ("local with an unsigned verifier",
             {"review_mode": "local-draft-only", "review_governance": {**signed, "authorized_signers": []}},
             "local-draft-only policy must not select a review verifier"),
            ("no authority", {"source_policy": {**sound["source_policy"], "authority": " "}},
             "policy requires selected source paths and authority"),
            ("no sources", {"source_policy": {**sound["source_policy"], "selected_paths": []}},
             "policy requires selected source paths and authority"),
            ("no include", {"source_policy": {**sound["source_policy"], "include": []}},
             "policy requires include patterns and valid exclude patterns"),
            ("limit below one", {"limits": {**sound["limits"], "max_file_count": 0}}, "policy has invalid limits"),
            ("limit not a number", {"limits": {**sound["limits"], "max_query_results": "9"}},
             "policy has invalid limits"),
        ]
        for label, change, message in cases:
            with self.subTest(label):
                self.assertEqual([message], validate_policy({**sound, **change}))
        with self.subTest("a limit of one"):
            self.assertEqual([], validate_policy({**sound, "limits": {**sound["limits"], "max_file_count": 1}}))
        with self.subTest("a signed-commit policy that is complete"):
            self.assertEqual([], validate_policy({**sound, "review_governance": signed}))

    def test_a_policy_is_rejected_one_step_past_every_limit(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        valid = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual(validate_policy(valid), [])
        for key in (
            "max_file_count",
            "max_file_bytes",
            "max_total_bytes",
            "max_query_results",
        ):
            broken = json.loads(json.dumps(valid))
            broken["limits"][key] = 0
            self.assertEqual(
                validate_policy(broken), ["policy has invalid limits"], key
            )

    def test_a_policy_without_a_named_source_authority_is_rejected(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        broken = json.loads(path.read_text(encoding="utf-8"))
        broken["source_policy"]["authority"] = "   "
        self.assertIn(
            "policy requires selected source paths and authority",
            validate_policy(broken),
        )

    def test_an_unknown_storage_mode_is_rejected(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        broken = json.loads(path.read_text(encoding="utf-8"))
        broken["storage_mode"] = "somewhere-else"
        self.assertIn("policy has an invalid storage_mode", validate_policy(broken))

    def test_an_amendment_whose_final_policy_is_invalid_writes_nothing(self) -> None:
        root = self.repo / "memory"
        amend_policy(root, "review_mode", "local-draft-only", "Drafting alone.")
        policy_before = (root / "domain-memory-policy.json").read_bytes()
        recorded_before = len(self.audit_lines())
        with self.assertRaisesRegex(ValueError, "amended policy is invalid"):
            amend_policy_fields(
                root,
                [("review_mode", "scm-verified"), ("review_verifier", "none")],
                "A reviewer exists now.",
            )
        self.assertEqual(policy_before, (root / "domain-memory-policy.json").read_bytes())
        self.assertEqual(recorded_before, len(self.audit_lines()))

    def test_an_amendment_naming_a_field_twice_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "ci_requirement is named more than once"):
            amend_policy_fields(
                self.repo / "memory",
                [("ci_requirement", "required"), ("ci_requirement", "none")],
                "Undecided.",
            )


if __name__ == "__main__":
    unittest.main()
