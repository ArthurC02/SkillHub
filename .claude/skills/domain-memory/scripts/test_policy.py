from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.policy import REMOVED_VERIFIER, amend_policy_fields, validate_policy
from test_support import RegistryTestCase, amend_policy, stored_policy_of


class PolicyTest(RegistryTestCase):
    def test_distinct_signer_and_the_profile_fields_amend_like_the_rest(self) -> None:
        root = self.repo / "memory"
        distinct = amend_policy(root, "distinct_signer", "true", "Two people review.")
        markers = amend_policy(root, "boundary_markers", "doc.go, package-info.java", "Java packages are boundaries too.")
        stored = stored_policy_of(self.repo)
        self.assertEqual(
            (True, ["doc.go", "package-info.java"], (False, None), (None, ["doc.go", "package-info.java"])),
            (stored["review_governance"]["distinct_signer"], stored["project_profile"]["boundary_markers"],
             (distinct["from"] is True, markers["from"]), (None, markers["to"])),
        )
        with self.assertRaisesRegex(ValueError, "distinct_signer takes true or false, got 'maybe'"):
            amend_policy(root, "distinct_signer", "maybe", "Unsure.")
        self.assertEqual(True, stored_policy_of(self.repo)["review_governance"]["distinct_signer"])

    def test_each_policy_defect_is_named_and_governance_reports_only_its_first(self) -> None:
        sound = stored_policy_of(self.repo)
        self.assertEqual([], validate_policy(sound))
        signed = {"verifier": "git-signed-commit", "trigger": "git-push", "ci_requirement": "none",
                  "authorized_signers": ["SHA256:key"]}
        cases = [
            ("format", {"format": "domain-memory-policy/v2"}, "policy has an invalid format: 'domain-memory-policy/v2' is not one of domain-memory-policy/v1"),
            ("format a list", {"format": []}, "policy has an invalid format: [] is not one of domain-memory-policy/v1"),
            ("storage", {"storage_mode": "cloud"}, "policy has an invalid storage_mode: 'cloud' is not one of external, ignored, tracked"),
            ("classification", {"data_classification": "secret"}, "policy has an invalid data_classification: 'secret' is not one of confidential, internal, public, restricted"),
            ("review mode", {"review_mode": "maybe"}, "policy has an invalid review_mode: 'maybe' is not one of local-draft-only, scm-verified"),
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
            ("unknown trigger", {"review_governance": {**sound["review_governance"], "trigger": "carrier-pigeon"}},
             "policy has invalid review_governance"),
            ("unknown ci requirement",
             {"review_governance": {**sound["review_governance"], "ci_requirement": "sometimes"}},
             "policy has invalid review_governance"),
            ("removed verifier", {"review_governance": {**sound["review_governance"], "verifier": "github-pr"}}, REMOVED_VERIFIER),
            ("removed trigger", {"review_governance": {**sound["review_governance"], "trigger": "external-scm"}}, REMOVED_VERIFIER),
            ("distinct signer not a boolean", {"review_governance": {**sound["review_governance"], "distinct_signer": "yes"}},
             "review_governance.distinct_signer must be true or false, got 'yes'"),
            ("profile not an object", {"project_profile": "lib"}, "project_profile must be an object, got a string"),
            ("profile unknown key", {"project_profile": {"implementation_root": ["lib"]}},
             "project_profile.implementation_root is not a known key; did you mean implementation_roots?"),
            ("profile marker with a path", {"project_profile": {"boundary_markers": ["pkg/doc.go"]}},
             "project_profile.boundary_markers[0] must be a bare name without a path separator, got 'pkg/doc.go'"),
            ("profile root outside the repository", {"project_profile": {"implementation_roots": ["../lib"]}},
             "project_profile.implementation_roots[0] must be a relative path inside the repository, got '../lib'"),
            ("profile blank entry", {"project_profile": {"decision_roots": [" "]}}, "project_profile.decision_roots[0] is blank"),
            ("profile entry not a string", {"project_profile": {"excluded_directories": [1]}},
             "project_profile.excluded_directories[0] must be a string, got a number"),
            ("source policy is not an object", {"source_policy": "docs"},
             "policy requires selected source paths and authority"),
            ("blank selected path", {"source_policy": {**sound["source_policy"], "selected_paths": ["docs", " "]}},
             "policy requires selected source paths and authority"),
            ("blank exclude pattern", {"source_policy": {**sound["source_policy"], "exclude": [" "]}},
             "policy requires include patterns and valid exclude patterns"),
            ("no authority", {"source_policy": {**sound["source_policy"], "authority": " "}},
             "policy requires selected source paths and authority"),
            ("no sources", {"source_policy": {**sound["source_policy"], "selected_paths": []}},
             "policy requires selected source paths and authority"),
            ("no include", {"source_policy": {**sound["source_policy"], "include": []}},
             "policy requires include patterns and valid exclude patterns"),
            ("limit below one", {"limits": {**sound["limits"], "max_file_count": 0}}, "policy has invalid limits"),
            ("limit not a number", {"limits": {**sound["limits"], "max_query_results": "9"}},
             "policy has invalid limits"),
            ("limit given as true", {"limits": {**sound["limits"], "max_query_results": True}},
             "policy has invalid limits"),
        ]
        for label, change, message in cases:
            with self.subTest(label):
                self.assertEqual([message], validate_policy({**sound, **change}))
        with self.subTest("a signed-commit policy that is complete"):
            self.assertEqual([], validate_policy({**sound, "review_governance": signed}))

    def test_a_policy_broken_everywhere_names_each_defect_in_the_order_of_its_fields(self) -> None:
        broken = {
            **stored_policy_of(self.repo), "format": "v0", "storage_mode": "cloud",
            "review_governance": None, "source_policy": None, "limits": None,
        }
        self.assertEqual(
            ["policy has an invalid format: 'v0' is not one of domain-memory-policy/v1",
             "policy has an invalid storage_mode: 'cloud' is not one of external, ignored, tracked", "policy requires review_governance",
             "policy requires selected source paths and authority", "policy has invalid limits"],
            validate_policy(broken),
        )

    def test_a_key_the_policy_does_not_define_is_named_with_the_closest_one(self) -> None:
        sound = stored_policy_of(self.repo)
        self.assertEqual(
            ["policy.schema_conventions is not a known key",
             "policy.review_mod is not a known key; did you mean review_mode?"],
            validate_policy({**sound, "schema_conventions": {}, "review_mod": "scm-verified"}),
        )
        self.assertEqual([], validate_policy({**sound, "approved_command_profiles": [], "project_profile": {}}))

    def test_each_limit_accepts_one_and_refuses_zero(self) -> None:
        sound = stored_policy_of(self.repo)
        keys = ("max_file_count", "max_file_bytes", "max_total_bytes", "max_query_results")
        roomy = dict.fromkeys(keys, 5)
        for key in keys:
            with self.subTest(f"{key} of one"):
                self.assertEqual([], validate_policy({**sound, "limits": {**roomy, key: 1}}))
            with self.subTest(f"{key} of zero"):
                self.assertEqual(
                    ["policy has invalid limits"], validate_policy({**sound, "limits": {**roomy, key: 0}})
                )

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
