from __future__ import annotations

import json
import shutil
import sys
import unittest
from unittest import mock

from domain_registry.changes import init_change_package, validate_change_package
from domain_registry.commands.evidence import package_overlay
from domain_registry.common import load_json, write_json
from domain_registry.counterfactual import counterfactual_suite
from domain_registry.evidence import citation
from domain_registry.harvest_run import discard_harvest, harvest, spec_errors
from domain_registry.registry import record_by_id, verify_evidence
from domain_registry.sources import selected_source_map, write_source_map
from domain_registry.updates import promote_candidate
from test_support import VERIFIED, RegistryTestCase, applied_by

RULE = "LIMIT = 200\nCAP = 12\n\n\ndef refused(length):\n    return length > LIMIT\n\n\ndef fee(amount):\n    return min(amount, CAP)\n"
RULE_TEST = (
    "import unittest\n\nfrom rule import refused\n\n\nclass RefusalTest(unittest.TestCase):\n"
    "    def test_the_limit_itself_is_accepted(self):\n        self.assertFalse(refused(200))\n\n"
    "    def test_one_past_the_limit_is_refused(self):\n        self.assertTrue(refused(201))\n"
)
RULE_ID = "a-length-past-the-limit-is-refused"
SECOND_RULE_ID = "a-length-at-the-limit-is-accepted"
PACKAGE = "memory/changes/orders-length-rule"
DATABASE_DOWN = "go test exited 1; only a passing run is recorded, and nothing was written:\nconnection refused"
RESUME = (
    f"the draft records and the package at {PACKAGE} are written; run the same harvest --spec again to resume from "
    f"submitted, or discard-harvest --package-root {PACKAGE} to undo it"
)


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

    def two_rule_spec(self) -> dict:
        spec = self.spec()
        spec["rules"].append({
            "id": SECOND_RULE_ID, "statement": "A length at the limit is accepted.", "example": "200 is accepted.",
            "enforcing": ["src/rule.py:5-6"], "test_file": "src/tests/test_rule.py", "test": "test_the_limit_itself_is_accepted",
            "check": self.check, "mutate": {"file": "src/rule.py", "find": "return length > LIMIT", "replace": "return length >= LIMIT"},
        })
        return spec

    def stale_citation(self, relative: str, start: int, end: int, old: str, new: str) -> dict:
        path = self.repo / relative
        text = path.read_text(encoding="utf-8")
        path.write_text(text.replace(old, new), encoding="utf-8")
        try:
            return citation(self.repo, relative, start, end)
        finally:
            path.write_text(text, encoding="utf-8")

    def cited_spans(self, evidence: list[dict]) -> list[tuple[str, int, int]]:
        return [(entry["path"], entry["lines"]["start"], entry["lines"]["end"]) for entry in evidence]

    def harvest_while_the_database_is_down(self, spec: dict) -> dict:
        with mock.patch("domain_registry.harvest_run.record_test_result", side_effect=ValueError(DATABASE_DOWN)):
            return harvest(self.repo / "memory", self.repo, spec, None, "verified")

    def upserts(self) -> list[dict]:
        return [event for event in self.audit_lines() if event["operation"] == "upsert-candidate" and event["asset"] == "rules"]

    def package_file(self, name: str) -> dict:
        return load_json(self.repo / PACKAGE / name)

    def set_package_status(self, status: str) -> None:
        proposal = self.package_file("domain-change-proposal.json")
        proposal["status"] = status
        write_json(self.repo / PACKAGE / "domain-change-proposal.json", proposal)

    def discard(self, reason: str = "the harvest is abandoned"):
        return self.run_cli(
            "discard-harvest", "--package-root", str(self.repo / PACKAGE), "--registry-root", str(self.repo / "memory"),
            "--repo-root", str(self.repo), "--reason", reason,
        )

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

    def test_without_an_approval_the_run_stops_tested_and_says_what_is_left(self) -> None:
        self.confirm_src()
        spec = self.spec()
        del spec["approval"]
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        proposal = load_json(self.repo / "memory" / "changes" / "orders-length-rule" / "domain-change-proposal.json")
        self.assertEqual(
            ("tested", ["no approval in the spec; record-approval, then verify-proposal"], "submitted"),
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

    def test_an_amended_rule_cites_its_lines_afresh_and_keeps_no_stale_citation_so_the_package_verifies(self) -> None:
        self.confirm_src()
        self.seed("rules.json", [{
            "id": RULE_ID, "contexts": ["orders"], "statement": "A length past the limit is refused.", "examples": ["201 is refused."],
            "status": "reviewed", "verified": VERIFIED, "evidence": [
                self.stale_citation("src/rule.py", 1, 6, "LIMIT = 200", "LIMIT = 100"),
                self.stale_citation("src/tests/test_rule.py", 10, 11, "201", "202"),
            ],
        }])
        applied_by(self.repo / "memory", "PRO-1", f"rules/{RULE_ID}")
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "verified")
        package = self.repo / "memory" / "changes" / "orders-length-rule"
        [update] = load_json(package / "domain-change-proposal.json")["registry_updates"]
        verified = verify_evidence(self.repo / "memory", self.repo, overlay=package_overlay(package))
        self.assertEqual(
            ("amended", "verified", [],
             [("src/rule.py", 1, 1), ("src/rule.py", 5, 6), ("src/tests/test_rule.py", 10, 11)], ["current"] * 3),
            (report["rules"][0]["record"], report["stopped_at"], report["errors"],
             self.cited_spans(update["record"]["evidence"]), [result["status"] for result in verified["results"]]),
        )

    def test_an_amended_rule_carries_the_spec_s_statement_and_examples_in_place_of_the_reviewed_ones(self) -> None:
        self.confirm_src()
        self.seed("rules.json", [{
            "id": RULE_ID, "contexts": ["orders"], "statement": "Long input is turned away.", "examples": ["300 is refused."],
            "status": "reviewed", "verified": VERIFIED, "evidence": ["src/rule.py:1"],
        }])
        applied_by(self.repo / "memory", "PRO-1", f"rules/{RULE_ID}")
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        [update] = load_json(self.repo / "memory" / "changes" / "orders-length-rule" / "domain-change-proposal.json")["registry_updates"]
        self.assertEqual(
            ("amended", "A length past the limit is refused.", ["201 is refused."], ["orders"], 3),
            (report["rules"][0]["record"], update["record"]["statement"], update["record"]["examples"],
             update["record"]["contexts"], len(update["record"]["evidence"])),
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
        self.assertNotIn("refresh-listing", report["errors"][0])
        self.assertIn("confirm the sources the cited paths belong to", report["next"])

    def test_a_citation_discovery_would_place_today_names_the_group_and_the_refresh_listing_to_run(self) -> None:
        self.confirm_src()
        (self.repo / "db" / "migrations").mkdir(parents=True)
        (self.repo / "db" / "migrations" / "0001_limit.sql").write_text(
            "ALTER TABLE orders ADD CONSTRAINT length_limit CHECK (length <= 200);\n", encoding="utf-8"
        )
        spec = self.spec()
        spec["rules"][0]["enforcing"] = ["db/migrations/0001_limit.sql:1-1"]
        report = harvest(self.repo / "memory", self.repo, spec, None, "draft")
        advice = "run refresh-listing --registry-root memory --repo-root . (no confirmation needed)"
        self.assertEqual(
            ("citation", None, [
                f"{RULE_ID} cites db/migrations/0001_limit.sql outside the confirmed sources; ship-package would refuse "
                "to review it, so confirm those sources first or cite others; discovery now places "
                f"db/migrations/0001_limit.sql under implementation; {advice}",
            ], f"no package was written; discovery now places a cited path, so {advice}, then run harvest again; "
               "otherwise confirm the sources the cited paths belong to, or cite others"),
            (report["stopped_at"], report["package"], report["errors"], report["next"]),
        )

    def test_a_test_the_file_does_not_define_is_named_before_any_test_runs(self) -> None:
        self.confirm_src()
        spec = self.spec()
        spec["rules"][0]["test"] = "test_nothing"
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        self.assertEqual(
            ("citation", [f"{RULE_ID}: src/tests/test_rule.py defines no Go func or Python def named test_nothing"], None),
            (report["stopped_at"], report["errors"], report["package"]),
        )

    def test_a_test_defined_in_another_file_is_refused_with_that_file_and_line(self) -> None:
        self.confirm_src()
        (self.repo / "src" / "tests" / "test_other.py").write_text("import unittest\n\n\ndef test_elsewhere():\n    pass\n", encoding="utf-8")
        spec = self.spec()
        spec["rules"][0]["test"] = "test_elsewhere"
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        self.assertEqual(
            ("citation", [f"{RULE_ID}: src/tests/test_rule.py defines no Go func or Python def named test_elsewhere; "
                          "it is defined in src/tests/test_other.py:4"], None),
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

    def test_a_test_command_failing_after_the_package_is_written_reports_the_package_and_how_to_resume(self) -> None:
        self.confirm_src()
        report = self.harvest_while_the_database_is_down(self.spec())
        self.assertEqual(
            (True, PACKAGE, False, "submitted", [DATABASE_DOWN], RESUME, "submitted"),
            (report["written"], report["package"], report["resumed"], report["stopped_at"], report["errors"], report["next"],
             self.package_file("domain-change-proposal.json")["status"]),
        )

    def test_the_command_prints_the_report_of_a_run_that_failed_after_writing_and_exits_one(self) -> None:
        self.confirm_src()
        spec_path = self.write_record("spec.json", self.spec(approval={"role": "developer", "reviewer": "domain-memory-agent"}))
        result = self.run_cli("harvest", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                              "--spec", str(spec_path), "--through", "verified")
        report = json.loads(result.stdout)
        self.assertEqual(
            (1, "", True, PACKAGE, "tested", ["the proposer cannot approve the proposal"], RESUME.replace("from submitted", "from tested")),
            (result.returncode, result.stderr, report["written"], report["package"], report["stopped_at"], report["errors"],
             report["next"]),
        )

    def test_rerunning_the_same_spec_resumes_without_writing_its_unchanged_records_again(self) -> None:
        self.confirm_src()
        spec = self.two_rule_spec()
        self.harvest_while_the_database_is_down(spec)
        upserted = len(self.upserts())
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        self.assertEqual(
            (2, 0, True, PACKAGE, ["unchanged", "unchanged"], "verified", [], "verified", 2, 1),
            (upserted, len(self.upserts()) - upserted, report["resumed"], report["package"],
             [rule["record"] for rule in report["rules"]],
             report["stopped_at"], report["errors"], self.package_file("domain-change-proposal.json")["status"],
             len(self.package_file("evidence-bundle.json")["test_results"]),
             len(self.package_file("domain-change-proposal.json")["approvals"])),
        )

    def test_rerunning_after_one_statement_changed_revises_that_record_alone(self) -> None:
        self.confirm_src()
        spec = self.two_rule_spec()
        harvest(self.repo / "memory", self.repo, spec, None, "draft")
        upserted = len(self.upserts())
        spec["rules"][0]["statement"] = "A length above the limit is refused."
        report = harvest(self.repo / "memory", self.repo, spec, None, "draft")
        updates = {update["record"]["id"]: update["record"]["statement"]
                   for update in self.package_file("domain-change-proposal.json")["registry_updates"]}
        criteria = {criterion["id"]: criterion["statement"]
                    for criterion in self.package_file("requirement-normalization.json")["acceptance_criteria"]}
        self.assertEqual(
            (["revised", "unchanged"], [RULE_ID], True, "draft", [],
             {RULE_ID: "A length above the limit is refused.", SECOND_RULE_ID: "A length at the limit is accepted."},
             {f"enforced-{RULE_ID}": "A length above the limit is refused.",
              f"enforced-{SECOND_RULE_ID}": "A length at the limit is accepted."}),
            ([rule["record"] for rule in report["rules"]], [event["record_id"] for event in self.upserts()[upserted:]],
             report["resumed"], report["stopped_at"], report["errors"], updates, criteria),
        )

    def test_rerunning_an_unchanged_draft_with_the_same_counterfactual_output_leaves_every_package_file_unwritten(self) -> None:
        self.confirm_src()
        outputs = []

        def recorded(*arguments):
            outputs.append(counterfactual_suite(*arguments))
            return outputs[-1]

        with mock.patch("domain_registry.harvest_run.counterfactual_suite", side_effect=recorded):
            harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        written = {path.name: path.stat().st_mtime_ns for path in (self.repo / PACKAGE).iterdir()}
        with mock.patch("domain_registry.harvest_run.counterfactual_suite", return_value=outputs[0]):
            report = harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        self.assertEqual(
            (True, "draft", [], 5, written),
            (report["resumed"], report["stopped_at"], report["errors"], len(written),
             {path.name: path.stat().st_mtime_ns for path in (self.repo / PACKAGE).iterdir()}),
        )

    def test_a_directory_holding_another_package_or_none_is_refused_before_anything_is_written(self) -> None:
        self.confirm_src()
        for name, prepare in (
            ("another package", lambda path: init_change_package(path, "other", "other", "someone")),
            ("a stray file", lambda path: (path.mkdir(parents=True), (path / "notes.txt").write_text("x", encoding="utf-8"))),
        ):
            with self.subTest(name):
                package = self.repo / PACKAGE
                prepare(package)
                events, record = len(self.audit_lines()), record_by_id(self.repo / "memory", "rules", RULE_ID)
                report = harvest(self.repo / "memory", self.repo, self.spec(), None, "verified")
                observed = (report["stopped_at"], report["written"], report["package"], len(self.audit_lines()),
                            record_by_id(self.repo / "memory", "rules", RULE_ID), "counterfactual" in report["rules"][0],
                            report["errors"][0].startswith(f"output directory is not empty: {PACKAGE}; ") if report["errors"] else None)
                shutil.rmtree(package)
                self.assertEqual(("package", False, None, events, record, False, True), observed)

    def test_a_finished_package_is_refused_and_not_rerun(self) -> None:
        self.confirm_src()
        harvest(self.repo / "memory", self.repo, self.spec(), None, "verified")
        for status in ("verified", "approved"):
            with self.subTest(status):
                self.set_package_status(status)
                events = len(self.audit_lines())
                report = harvest(self.repo / "memory", self.repo, self.spec(), None, "verified")
                way_out = {
                    "verified": f"to harvest again, discard-harvest --package-root {PACKAGE} --reason <why>, then run harvest again",
                    "approved": "give the spec another name to harvest it as a new package",
                }[status]
                self.assertEqual(
                    ("package", [f"the package at {PACKAGE} is {status}; a finished harvest is not re-run; {way_out}"], events, status),
                    (report["stopped_at"], report["errors"], len(self.audit_lines()),
                     self.package_file("domain-change-proposal.json")["status"]),
                )

    def test_resuming_keeps_a_recorded_passing_result_and_approval_instead_of_recording_them_again(self) -> None:
        self.confirm_src()
        with mock.patch("domain_registry.harvest_run.verify_proposal", side_effect=ValueError("the registry moved")):
            failed = harvest(self.repo / "memory", self.repo, self.spec(), None, "verified")
        [result] = self.package_file("evidence-bundle.json")["test_results"]
        [approval] = self.package_file("domain-change-proposal.json")["approvals"]
        report = harvest(self.repo / "memory", self.repo, self.spec(), None, "verified")
        self.assertEqual(
            ("tested", ["the registry moved"], "verified", [], [result], [approval]),
            (failed["stopped_at"], failed["errors"], report["stopped_at"], report["errors"],
             self.package_file("evidence-bundle.json")["test_results"], self.package_file("domain-change-proposal.json")["approvals"]),
        )

    def test_a_submitted_package_whose_rule_changed_is_refused_before_anything_is_written(self) -> None:
        self.confirm_src()
        spec = self.spec()
        self.harvest_while_the_database_is_down(spec)
        events = len(self.audit_lines())
        spec["rules"][0]["statement"] = "A length above the limit is refused."
        report = harvest(self.repo / "memory", self.repo, spec, None, "verified")
        self.assertEqual(
            ("package", [f"the package at {PACKAGE} is submitted and {RULE_ID} changed since, but a submitted package is "
                         f"frozen; discard-harvest --package-root {PACKAGE}, then run harvest again"], events, False),
            (report["stopped_at"], report["errors"], len(self.audit_lines()), report["resumed"]),
        )

    def test_discard_harvest_retracts_its_drafts_removes_the_package_and_leaves_a_reviewed_rule(self) -> None:
        self.confirm_src()
        self.seed("rules.json", [{
            "id": RULE_ID, "contexts": ["orders"], "statement": "A length past the limit is refused.",
            "status": "reviewed", "verified": VERIFIED, "evidence": ["src/rule.py:1"],
        }])
        applied_by(self.repo / "memory", "PRO-1", f"rules/{RULE_ID}")
        harvest(self.repo / "memory", self.repo, self.two_rule_spec(), None, "draft")
        result = self.discard()
        event = self.audit_lines()[-1]
        self.assertEqual(
            (0, {"operation": "discard-harvest", "package": PACKAGE, "retracted": [f"rules/{SECOND_RULE_ID}"], "kept": [],
                 "reason": "the harvest is abandoned"},
             "discard-harvest", [f"rules/{SECOND_RULE_ID}"], [], False, "reviewed", None, "retract-candidate"),
            (result.returncode, json.loads(result.stdout), event["operation"], event["retracted"], event["kept"],
             (self.repo / PACKAGE).exists(), record_by_id(self.repo / "memory", "rules", RULE_ID)["status"],
             record_by_id(self.repo / "memory", "rules", SECOND_RULE_ID), self.audit_lines()[-2]["operation"]),
        )

    def test_discard_harvest_takes_the_spec_and_finds_the_package_where_harvest_put_it(self) -> None:
        self.confirm_src()
        spec_file = self.repo / "spec.json"
        write_json(spec_file, self.spec())
        for label, changes_root in (("beside the Registry", None), ("under --changes-root", self.repo / "pkgs")):
            with self.subTest(label):
                harvest(self.repo / "memory", self.repo, self.spec(), changes_root, "draft")
                package = (changes_root or self.repo / "memory" / "changes") / "orders-length-rule"
                roots = ["--changes-root", str(changes_root)] if changes_root else []
                result = self.run_cli(
                    "discard-harvest", "--spec", str(spec_file), *roots, "--registry-root", str(self.repo / "memory"),
                    "--repo-root", str(self.repo), "--reason", "the citations ran one line long",
                )
                self.assertEqual(
                    (0, package.relative_to(self.repo).as_posix(), [f"rules/{RULE_ID}"], False),
                    (result.returncode, json.loads(result.stdout)["package"], json.loads(result.stdout)["retracted"], package.exists()),
                )

    def test_discard_harvest_wants_exactly_one_of_the_package_root_and_the_spec(self) -> None:
        spec_file = self.repo / "spec.json"
        write_json(spec_file, self.spec())
        common = ("--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo), "--reason", "undone")
        for label, chosen in (("neither", ()), ("both", ("--package-root", str(self.repo / PACKAGE), "--spec", str(spec_file)))):
            with self.subTest(label):
                result = self.run_cli("discard-harvest", *chosen, *common)
                self.assertEqual(
                    (1, "ERROR: discard-harvest takes --package-root or --spec, one of them"),
                    (result.returncode, result.stderr.strip()),
                )

    def test_discard_harvest_while_another_writer_holds_the_lock_keeps_the_package_and_writes_no_event(self) -> None:
        package = self.repo / PACKAGE
        init_change_package(package, "orders-length-rule", "orders-length-rule", "someone")
        events = len(self.audit_lines())
        (self.repo / "memory" / ".domain-registry.lock").mkdir()
        with self.assertRaisesRegex(ValueError, "^another Domain Registry update is in progress: "):
            discard_harvest(package, self.repo / "memory", self.repo, "the harvest is abandoned")
        self.assertEqual(
            (True, events), ((package / "domain-change-proposal.json").is_file(), len(self.audit_lines()))
        )

    def test_discard_harvest_keeps_a_draft_another_package_names(self) -> None:
        self.confirm_src()
        harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        other = self.repo / "memory" / "changes" / "other"
        init_change_package(other, "other", "other", "someone")
        promote_candidate(other, self.repo / "memory", "rules", RULE_ID)
        result = self.discard()
        self.assertEqual(
            (0, [], [f"rules/{RULE_ID}"], "candidate", False),
            (result.returncode, json.loads(result.stdout)["retracted"], json.loads(result.stdout)["kept"],
             record_by_id(self.repo / "memory", "rules", RULE_ID)["status"], (self.repo / PACKAGE).exists()),
        )

    def test_discard_harvest_refuses_a_second_run_and_a_finished_package(self) -> None:
        self.confirm_src()
        harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        self.set_package_status("approved")
        approved = self.discard()
        self.set_package_status("draft")
        self.discard()
        events = len(self.audit_lines())
        again = self.discard()
        self.assertEqual(
            (1, f"ERROR: the package at {PACKAGE} is approved; only a draft, submitted or verified harvest is discarded",
             1, f"ERROR: no package at {PACKAGE}", "", events),
            (approved.returncode, approved.stderr.strip(), again.returncode, again.stderr.strip(), again.stdout,
             len(self.audit_lines())),
        )

    def test_discard_harvest_undoes_a_verified_harvest_and_still_refuses_an_applied_one(self) -> None:
        self.confirm_src()
        verified = harvest(self.repo / "memory", self.repo, self.spec(), None, "verified")["stopped_at"]
        result = self.discard()
        event = self.audit_lines()[-1]
        self.assertEqual(
            ("verified", 0, ["discard-harvest", [f"rules/{RULE_ID}"]], None, False),
            (verified, result.returncode, [event["operation"], event["retracted"]],
             record_by_id(self.repo / "memory", "rules", RULE_ID), (self.repo / PACKAGE).exists()),
        )
        harvest(self.repo / "memory", self.repo, self.spec(), None, "draft")
        self.set_package_status("applied")
        applied = self.discard()
        self.assertEqual(
            (1, f"ERROR: the package at {PACKAGE} is applied; only a draft, submitted or verified harvest is discarded", True),
            (applied.returncode, applied.stderr.strip(), (self.repo / PACKAGE).exists()),
        )


if __name__ == "__main__":
    unittest.main()
