from __future__ import annotations

import json
import sys
import unittest

from domain_registry.changes import validate_change_package
from domain_registry.common import load_json
from domain_registry.evidence import citation
from domain_registry.harvest_run import harvest, spec_errors
from domain_registry.registry import record_by_id
from domain_registry.sources import selected_source_map, write_source_map
from test_support import VERIFIED, RegistryTestCase, applied_by

RULE = "LIMIT = 200\nCAP = 12\n\n\ndef refused(length):\n    return length > LIMIT\n\n\ndef fee(amount):\n    return min(amount, CAP)\n"
RULE_TEST = (
    "import unittest\n\nfrom rule import refused\n\n\nclass RefusalTest(unittest.TestCase):\n"
    "    def test_the_limit_itself_is_accepted(self):\n        self.assertFalse(refused(200))\n\n"
    "    def test_one_past_the_limit_is_refused(self):\n        self.assertTrue(refused(201))\n"
)
RULE_ID = "a-length-past-the-limit-is-refused"


class HarvestTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.add_context()
        (self.repo / "src" / "tests").mkdir(parents=True)
        (self.repo / "src" / "rule.py").write_text(RULE, encoding="utf-8")
        (self.repo / "src" / "tests" / "__init__.py").write_text("", encoding="utf-8")
        (self.repo / "src" / "tests" / "test_rule.py").write_text(RULE_TEST, encoding="utf-8")
        self.check = f'"{sys.executable}" -m unittest discover -s src/tests -t src'

    def confirm_src(self) -> None:
        write_source_map(self.repo / "memory" / "source-map.json", selected_source_map(self.repo, [self.repo / "src"]))

    def spec(self, **changes: object) -> dict:
        rule = {
            "id": RULE_ID, "statement": "A length past the limit is refused.", "example": "201 is refused.",
            "enforcing": [["src/rule.py", 1, 1], "src/rule.py:5-6"],
            "test_file": "src/tests/test_rule.py", "test": "test_one_past_the_limit_is_refused",
            "check": self.check, "command_profile": "unit", "level": "unit",
            "mutate": {"file": "src/rule.py", "find": "LIMIT = 200", "replace": "LIMIT = 201"},
        }
        return {
            "name": "orders-length-rule", "context": "orders", "proposer": "domain-memory-agent",
            "intent": "Record the length limit the code enforces.", "outcome": "get-context orders lists the rule as reviewed.",
            "scope": "Record one orders rule as reviewed",
            "approval": {"role": "developer", "reviewer": "reviewer"},
            "ownership": {"owner_context": "orders", "invariant": "Only orders decides the limit.", "evidence": []},
            "boundary": {"interaction": "none", "consistency": "synchronous", "prohibited_dependencies": [], "evidence": []},
            "design": {"domain_forces": ["The limit is enforced and tested."], "decision": "Record it.",
                       "invariants_preserved": ["The limit holds."], "rejected_alternatives": ["Leave it unrecorded."]},
            "rules": [rule], **changes,
        }

    def test_a_killed_counterfactual_leads_to_a_candidate_and_a_draft_that_passes_the_verified_preview(self) -> None:
        self.confirm_src()
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        package = self.repo / "memory" / "changes" / "orders-length-rule"
        candidate = record_by_id(self.repo / "memory", "rules", RULE_ID)
        proposal = load_json(package / "domain-change-proposal.json")
        self.assertEqual(
            ("draft", [], [{"id": RULE_ID, "counterfactual": "killed", "obligation": f"rule-{RULE_ID}", "record": "created"}],
             "memory/changes/orders-length-rule", "candidate", ["orders"], 3, "draft", [RULE_ID], []),
            (report["stopped_at"], report["errors"],
             [{key: value for key, value in rule.items() if key != "test_citation"} for rule in report["rules"]],
             report["package"], candidate["status"], candidate["contexts"], len(candidate["evidence"]),
             proposal["status"], proposal["rule_ids"], validate_change_package(package, self.repo / "memory", "submitted", self.repo)),
        )
        self.assertEqual(
            ("src/tests/test_rule.py", 10, 11),
            (report["rules"][0]["test_citation"]["path"], report["rules"][0]["test_citation"]["lines"]["start"],
             report["rules"][0]["test_citation"]["lines"]["end"]),
        )
        self.assertIn("harvest --through verified", report["next"])

    def test_through_verified_submits_records_the_test_results_approves_and_verifies(self) -> None:
        self.confirm_src()
        report = harvest(self.repo / "memory", self.repo, self.spec(command_profile="bogus"), None, "verified")
        package = self.repo / "memory" / "changes" / "orders-length-rule"
        proposal = load_json(package / "domain-change-proposal.json")
        evidence = load_json(package / "evidence-bundle.json")
        self.assertEqual(
            ("verified", [], "verified", [f"rule-{RULE_ID}"], ["developer"], 1),
            (report["stopped_at"], report["errors"], proposal["status"],
             [result["obligation_id"] for result in evidence["test_results"]],
             [approval["role"] for approval in proposal["approvals"]], len(evidence["counterfactual_check"])),
        )
        self.assertIn("ship-package --commit", report["next"])

    def test_without_an_approval_the_run_stops_submitted_and_says_what_is_left(self) -> None:
        self.confirm_src()
        spec = self.spec()
        del spec["approval"]
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        proposal = load_json(self.repo / "memory" / "changes" / "orders-length-rule" / "domain-change-proposal.json")
        self.assertEqual(
            ("submitted", ["no approval in the spec; record-approval, then verify-proposal"], "submitted"),
            (report["stopped_at"], report["errors"], proposal["status"]),
        )

    def test_a_draft_the_preview_refuses_is_left_written_with_what_it_names(self) -> None:
        self.confirm_src()
        spec = self.spec()
        spec["design"] = {"domain_forces": [], "decision": "Record it.", "invariants_preserved": [], "rejected_alternatives": []}
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        proposal = load_json(self.repo / "memory" / "changes" / "orders-length-rule" / "domain-change-proposal.json")
        self.assertEqual(("draft", "draft", True), (report["stopped_at"], proposal["status"], len(report["errors"]) > 0))
        self.assertTrue(all(error.startswith("once submitted: ") for error in report["errors"]), report["errors"])
        self.assertIn("fix what the preview names", report["next"])

    def test_a_surviving_counterfactual_writes_nothing(self) -> None:
        self.confirm_src()
        spec = self.spec()
        spec["rules"][0]["mutate"] = {"file": "src/rule.py", "find": "CAP = 12", "replace": "CAP = 13"}
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        self.assertEqual(
            ("counterfactual", "survived", 1, False, None, None),
            (report["stopped_at"], report["rules"][0]["counterfactual"], len(report["errors"]),
             (self.repo / "memory" / "changes" / "orders-length-rule").exists(),
             record_by_id(self.repo / "memory", "rules", RULE_ID), report["package"]),
        )
        self.assertTrue(report["errors"][0].startswith(f"{RULE_ID}: the counterfactual is survived:"))

    def test_a_reviewed_rule_is_amended_with_its_test_citation_and_supersedes_its_reviewer(self) -> None:
        self.confirm_src()
        self.seed("rules.json", [{
            "id": RULE_ID, "contexts": ["orders"], "statement": "A length past the limit is refused.",
            "status": "reviewed", "verified": VERIFIED, "evidence": ["src/rule.py:1"],
        }])
        applied_by(self.repo / "memory", "PRO-1", f"rules/{RULE_ID}")
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        proposal = load_json(self.repo / "memory" / "changes" / "orders-length-rule" / "domain-change-proposal.json")
        [update] = proposal["registry_updates"]
        self.assertEqual(
            ("amended", ["PRO-1"], "src/tests/test_rule.py", "draft"),
            (report["rules"][0]["record"], proposal["supersedes"], update["record"]["evidence"][-1]["path"], report["stopped_at"]),
        )

    def test_evidence_outside_every_discovered_source_is_refused_by_path_before_any_test_runs(self) -> None:
        self.confirm_src()
        (self.repo / "notes.txt").write_text("the limit is 200", encoding="utf-8")
        spec = self.spec()
        spec["rules"][0]["enforcing"] = ["notes.txt:1-1"]
        report = harvest(self.repo / "memory", self.repo, spec, None, "draft")
        self.assertEqual(
            ("citation", None, [{"id": RULE_ID, "obligation": f"rule-{RULE_ID}"}], 1, False, None),
            (report["stopped_at"], report["package"], report["rules"], len(report["errors"]),
             (self.repo / "memory" / "changes" / "orders-length-rule").exists(), record_by_id(self.repo / "memory", "rules", RULE_ID)),
        )
        self.assertTrue(report["errors"][0].startswith(f"{RULE_ID} cites notes.txt outside the confirmed sources"))
        self.assertIn("confirm the sources the cited paths belong to", report["next"])

    def test_a_test_the_file_does_not_define_is_named_before_any_test_runs(self) -> None:
        self.confirm_src()
        spec = self.spec()
        spec["rules"][0]["test"] = "test_nothing"
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        self.assertEqual(
            ("citation", [f"{RULE_ID}: src/tests/test_rule.py defines no Go func or Python def named test_nothing"], None),
            (report["stopped_at"], report["errors"], report["package"]),
        )

    def test_a_reviewed_rule_with_the_same_spans_but_another_example_is_amended(self) -> None:
        self.confirm_src()
        self.seed("rules.json", [{
            "id": RULE_ID, "contexts": ["orders"], "statement": "A length past the limit is refused.", "examples": ["200 is accepted."],
            "status": "reviewed", "verified": VERIFIED, "evidence": [
                citation(self.repo, "src/rule.py", 1, 1), citation(self.repo, "src/rule.py", 5, 6), citation(self.repo, "src/tests/test_rule.py", 10, 11),
            ],
        }])
        applied_by(self.repo / "memory", "PRO-1", f"rules/{RULE_ID}")
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        self.assertEqual(("amended", []), (report["rules"][0]["record"], report["errors"]))

    def test_a_reviewed_rule_with_the_same_example_but_fewer_cited_spans_is_amended(self) -> None:
        self.confirm_src()
        self.seed("rules.json", [{
            "id": RULE_ID, "contexts": ["orders"], "statement": "A length past the limit is refused.", "examples": ["201 is refused."],
            "status": "reviewed", "verified": VERIFIED, "evidence": [
                citation(self.repo, "src/rule.py", 1, 1), citation(self.repo, "src/tests/test_rule.py", 10, 11),
            ],
        }])
        applied_by(self.repo / "memory", "PRO-1", f"rules/{RULE_ID}")
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        self.assertEqual(("amended", []), (report["rules"][0]["record"], report["errors"]))

    def test_a_reviewed_rule_whose_content_is_unchanged_is_reported_reproven(self) -> None:
        self.confirm_src()
        self.seed("rules.json", [{
            "id": RULE_ID, "contexts": ["orders"], "statement": "A length past the limit is refused.", "examples": ["201 is refused."],
            "status": "reviewed", "verified": VERIFIED, "evidence": [
                citation(self.repo, "src/rule.py", 1, 1), citation(self.repo, "src/rule.py", 5, 6), citation(self.repo, "src/tests/test_rule.py", 10, 11),
            ],
        }])
        applied_by(self.repo / "memory", "PRO-1", f"rules/{RULE_ID}")
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        proposal = load_json(self.repo / "memory" / "changes" / "orders-length-rule" / "domain-change-proposal.json")
        self.assertEqual(
            ("reproven", ["PRO-1"], [], "draft"),
            (report["rules"][0]["record"], proposal["supersedes"], report["errors"], report["stopped_at"]),
        )

    def test_a_spec_with_gaps_names_each_gap(self) -> None:
        self.assertEqual(["the spec must be a JSON object"], spec_errors([]))
        self.assertEqual(
            ["name must be text", "context must be text", "proposer must be text", "intent must be text", "outcome must be text",
             "scope must be text", "ownership must be an object", "boundary must be an object", "design must be an object",
             "rules must be a non-empty list"],
            spec_errors({}),
        )
        spec = self.spec(approval={"role": "developer"}, rules=[{"id": "r", "enforcing": [["x", 1]], "mutate": {}, "level": 3}, 7, {**self.spec()["rules"][0], "id": ""}])
        self.assertEqual(
            ["approval must be an object with role and reviewer",
             "rule r: statement must be text", "rule r: test_file must be text", "rule r: test must be text",
             "rule r: check must be text",
             "rule r: enforcing must be a non-empty list of [path, start, end] or path:start-end",
             "rule r: mutate must be an object with file, find and replace", "rule r: level must be text",
             "rule 2: must be an object", "rule 3: id must be text"],
            spec_errors(spec),
        )
        report = harvest(self.repo / "memory", self.repo, {}, None, "draft")
        self.assertEqual(
            ("spec", "name must be text", None, [], "no package was written; fix what the spec errors name, then run harvest again"),
            (report["stopped_at"], report["errors"][0], report["package"], report["rules"], report["next"]),
        )

    def test_the_command_prints_the_report_and_exits_one_when_a_stage_fails(self) -> None:
        self.confirm_src()
        spec = self.spec()
        spec["rules"][0]["mutate"] = {"file": "src/rule.py", "find": "CAP = 12", "replace": "CAP = 13"}
        spec_path = self.repo / "spec.json"
        spec_path.write_text(json.dumps(spec), encoding="utf-8")
        failed = self.run_cli("harvest", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                              "--spec", str(spec_path))
        spec["rules"][0]["mutate"] = {"file": "src/rule.py", "find": "LIMIT = 200", "replace": "LIMIT = 201"}
        spec_path.write_text(json.dumps(spec), encoding="utf-8")
        passed = self.run_cli("harvest", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                              "--spec", str(spec_path), "--through", "verified")
        self.assertEqual(
            ((1, "counterfactual"), (0, "verified")),
            ((failed.returncode, json.loads(failed.stdout)["stopped_at"]), (passed.returncode, json.loads(passed.stdout)["stopped_at"])),
        )


if __name__ == "__main__":
    unittest.main()
