from __future__ import annotations

import json
import os
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import init_change_package, validate_change_package
from domain_registry.evidence import citation
from domain_registry.harvest_package import harvest_package, write_harvest_package
from domain_registry.hitl import record_approval
from test_support import RegistryTestCase, applied_by


class CliProposalsTest(RegistryTestCase):
    def test_get_record_names_the_obligations_that_prove_a_reviewed_rule(self) -> None:
        package = self.draft_package()
        for name, change in (
            ("test-obligations.json",
             lambda value: value["obligations"][0].update(also_covers=[{"source_type": "rule", "source_id": "order-total"}])),
            ("domain-change-proposal.json", lambda value: value.update(rule_ids=["order-total"])),
            ("evidence-bundle.json", lambda value: value["test_results"][0].update(working_directory="apps/platform")),
        ):
            value = json.loads((package / name).read_text(encoding="utf-8"))
            change(value)
            (package / name).write_text(json.dumps(value), encoding="utf-8")
        self.approve_and_apply(package)
        other = self.repo / "other"
        other.mkdir()
        (other / "domain-change-proposal.json").write_text(json.dumps({"proposal_id": "PRO-2"}), encoding="utf-8")
        (other / "test-obligations.json").write_bytes((package / "test-obligations.json").read_bytes())
        elsewhere = self.repo / "elsewhere"
        elsewhere.mkdir()
        memory = str(self.repo / "memory")
        shown, unseen = (
            self.run_cli("get-record", "--registry-root", memory, "--asset", "rule", "--id", "order-total", "--changes-root", str(root))
            for root in (self.repo, elsewhere)
        )
        self.assertEqual(
            [{"proposal_id": "PRO-1", "package": "proposal", "obligation_id": "OB-1", "check": "test command",
              "working_directory": "apps/platform"}],
            json.loads(shown.stdout)["proven_by"],
        )
        self.assertEqual(
            (False, f"NOTE: the Change Package of PRO-1, which reviewed rules/order-total, is not under {elsewhere.resolve()}, "
                    "so the tests that prove it are unknown."),
            ("proven_by" in json.loads(unseen.stdout), unseen.stderr.strip()),
        )
        recitation = self.repo / "recitation"
        recitation.mkdir()
        (recitation / "domain-change-proposal.json").write_text(json.dumps({"proposal_id": "PRO-3"}), encoding="utf-8")
        applied_by(self.repo / "memory", "PRO-3", "rules/order-total")
        recited = self.run_cli("get-record", "--registry-root", memory, "--asset", "rules", "--id", "order-total",
                               "--changes-root", str(self.repo))
        self.assertEqual(
            (["PRO-1"], "NOTE: rules/order-total is proven by PRO-1; PRO-3, applied since, changed it without a test "
                        "obligation, as a re-citation does."),
            ([entry["proposal_id"] for entry in json.loads(recited.stdout)["proven_by"]], recited.stderr.strip()),
        )
        coverage = ("coverage", "--registry-root", memory, "--changes-root", str(self.repo), "--repo-root", str(self.repo))
        self.assertEqual(
            [{"context": "orders", "reviewed_rules": 1, "candidate_rules": 0, "proven": ["order-total"], "unproven": [],
              "unknown": []}],
            json.loads(self.run_cli(*coverage).stdout)["rules_by_context"],
        )
        applied_by(self.repo / "memory", "PRO-4", "rules/order-total")
        unknown = json.loads(self.run_cli(*coverage).stdout)
        self.assertEqual(
            (["order-total"], {"reviewed_rules": 1, "candidate_rules": 0, "proven": 0, "unproven": 0, "unknown": 1,
                               "contexts_without_reviewed_rules": 0}),
            (unknown["rules_by_context"][0]["unknown"], unknown["rule_totals"]),
        )

    def test_promote_candidate_adds_the_citations_it_is_given_once(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        (self.repo / "total_test.py").write_text("def test_total():\n    assert True\n", encoding="utf-8")
        promote = ("promote-candidate", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
                   "--asset", "rules", "--id", "order-total")
        for alone in (("--cite", "README.md:1-1"), ("--cite-function", "total_test.py:test_total")):
            with self.subTest(alone):
                self.assertEqual("ERROR: --cite and --cite-function need --repo-root", self.run_cli(*promote, *alone).stderr.strip())
        cited = ("--repo-root", str(self.repo), "--cite", "README.md:1-1", "--cite", "README.md:1-1",
                 "--cite-function", "total_test.py:test_total")
        self.assertEqual(0, self.run_cli(*promote, *cited).returncode)
        [update] = self.registry_updates(package)
        self.assertEqual(
            ["README.md:1", ("README.md", 1, 1), ("total_test.py", 1, 2)],
            [entry if isinstance(entry, str) else (entry["path"], entry["lines"]["start"], entry["lines"]["end"])
             for entry in update["record"]["evidence"]],
        )

    def harvest_spec(self, **changes: object) -> Path:
        check = "go test -v -run ^TestTotal$ ./orders/"
        entry = {
            "check": f"{check}\n",
            "counterfactual": {
                "verdict": "killed", "test_command": check,
                "counterfactual_check": {"status": "passed", "obligation_id": "<obligation-id>",
                                         "mutated_protection": "total.go:3: >= 0 -> >= -1",
                                         "failure_evidence": "--- FAIL: TestTotal", "restored": True},
            },
        }
        spec = {
            "requirement": {"intent": "Record the enforced order rules."},
            "implementation_design": {"decision": "Record the rule as reviewed.", "ignored": "x"},
            "rules": {"order-total": {**entry, **changes}},
        }
        path = self.repo / "spec.json"
        path.write_text(json.dumps(spec), encoding="utf-8")
        return path

    def fill(self, package: Path, spec: Path, *, write: bool = True) -> dict:
        documents = harvest_package(package, json.loads(spec.read_text(encoding="utf-8")))
        if write:
            write_harvest_package(package, documents)
        return documents

    def fill_refusal(self, package: Path, spec: Path, *, write: bool = True) -> str:
        with self.assertRaises(ValueError) as refused:
            self.fill(package, spec, write=write)
        return str(refused.exception)

    def test_filling_a_harvest_package_maps_each_promoted_rule_to_a_criterion_an_obligation_and_its_counterfactual(self) -> None:
        package = self.draft_package()
        proposal_path = package / "domain-change-proposal.json"
        proposal_path.write_text(
            json.dumps({**json.loads(proposal_path.read_text(encoding="utf-8")), "affected_contexts": []}), encoding="utf-8"
        )
        evidence_path = package / "evidence-bundle.json"
        evidence_path.write_text(
            json.dumps({**json.loads(evidence_path.read_text(encoding="utf-8")), "test_results": []}), encoding="utf-8"
        )
        before = {path.name: path.read_bytes() for path in package.iterdir()}
        dry = self.fill(package, self.harvest_spec(), write=False)
        self.assertEqual(before, {path.name: path.read_bytes() for path in package.iterdir()})
        self.fill(package, self.harvest_spec())
        written = {path.name: json.loads(path.read_text(encoding="utf-8")) for path in package.glob("*.json")}
        self.assertEqual(dry, {name: written[name] for name in dry})
        requirement, obligations = written["requirement-normalization.json"], written["test-obligations.json"]
        proposal, evidence = written["domain-change-proposal.json"], written["evidence-bundle.json"]
        self.assertEqual(
            ("Record the enforced order rules.", [{"id": "enforced-order-total", "statement": "An order total is non-negative."}],
             ["README.md"]),
            (requirement["intent"], requirement["acceptance_criteria"], requirement["sources"]),
        )
        self.assertEqual(
            [{"id": "rule-order-total", "source_type": "acceptance-criterion", "source_id": "enforced-order-total",
              "also_covers": [{"source_type": "rule", "source_id": "order-total"}],
              "expected_outcome": "An order total is non-negative.", "level": "unit", "status": "planned",
              "check": "go test -v -run ^TestTotal$ ./orders/"}],
            obligations["obligations"],
        )
        self.assertEqual(
            (["orders"], ["order-total"], ["rule-order-total"], "Record the rule as reviewed.", False),
            (proposal["affected_contexts"], proposal["rule_ids"], proposal["implementation_design"]["proof_obligations"],
             proposal["implementation_design"]["decision"], "ignored" in proposal["implementation_design"]),
        )
        self.assertEqual(
            (["README.md"], ["rule-order-total"], []),
            (evidence["sources"], [check["obligation_id"] for check in evidence["counterfactual_check"]],
             validate_change_package(package, None)),
        )

    def test_filling_a_harvest_package_refuses_a_rule_it_cannot_show_proven_and_a_dry_fill_refuses_alike(self) -> None:
        package = self.draft_package()
        killed = json.loads(self.harvest_spec().read_text(encoding="utf-8"))["rules"]["order-total"]["counterfactual"]
        for label, changes, refusal in (
            ("no check", {"check": " "}, "rules.order-total: give the check that runs the rule's test"),
            ("only the check object", {"counterfactual": killed["counterfactual_check"]},
             "rules.order-total: counterfactual must be the whole output of counterfactual"),
            ("survived", {"counterfactual": {**killed, "verdict": "survived"}},
             "rules.order-total: the counterfactual is survived, not killed"),
            ("another command", {"counterfactual": {**killed, "test_command": "go test -v ./..."}},
             "rules.order-total: the counterfactual ran 'go test -v ./...', not the check"),
            ("another obligation", {"counterfactual": {**killed, "counterfactual_check": {
                **killed["counterfactual_check"], "obligation_id": "rule-other"}}},
             "rules.order-total: the counterfactual names obligation rule-other, not rule-order-total"),
        ):
            spec = self.harvest_spec(**changes)
            for write in (True, False):
                with self.subTest(label=label, write=write):
                    self.assertIn(refusal, self.fill_refusal(package, spec, write=write))
        spec = json.loads(self.harvest_spec().read_text(encoding="utf-8"))
        spec["rules"]["extra-rule"] = spec["rules"].pop("order-total")
        (self.repo / "spec.json").write_text(json.dumps(spec), encoding="utf-8")
        self.assertEqual(
            "the harvest spec cannot fill this package: rules.order-total is missing; "
            "rules.extra-rule names a rule the package does not promote",
            self.fill_refusal(package, self.repo / "spec.json"),
        )

    def test_filling_a_harvest_package_fills_only_a_draft_that_promotes_a_rule(self) -> None:
        package = self.draft_package()
        proposal_path = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        for status, updates, refusal in (
            ("submitted", proposal["registry_updates"], "the package is submitted; harvest --spec fills a draft"),
            ("draft", [], "the package promotes no rule; run promote-candidate for each harvested rule first"),
        ):
            with self.subTest(refusal):
                proposal_path.write_text(json.dumps({**proposal, "status": status, "registry_updates": updates}), encoding="utf-8")
                self.assertEqual(refusal, self.fill_refusal(package, self.harvest_spec()))

    def test_get_record_looks_for_packages_beside_the_registry_only_for_rules_and_contracts(self) -> None:
        self.seed("rules.json", [{"id": "r", "contexts": ["orders"], "statement": "x"}])
        self.seed("contexts.json", [{"id": "orders", "name": "Orders"}])
        memory = self.repo / "memory"
        note = "NOTE: rules/r was not reviewed through an applied Change Package, so no test obligation proves it yet."
        rule, context = (("--asset", "rules", "--id", "r"), ("--asset", "contexts", "--id", "orders"))
        before = self.run_cli("get-record", "--registry-root", str(memory), *rule).stderr.strip()
        (memory / "changes").mkdir()
        self.assertEqual(
            ("", note, ""),
            (before, self.run_cli("get-record", "--registry-root", str(memory), *rule).stderr.strip(),
             self.run_cli("get-record", "--registry-root", str(memory), *context, "--changes-root", str(memory / "changes")).stderr.strip()),
        )

    def test_coverage_counts_reviewed_and_candidate_rules_per_context_gaps_first(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders"}, {"id": "billing", "name": "Billing"},
                                    {"id": "ops", "name": "Ops"}])
        self.seed("rules.json", [
            {"id": "a", "contexts": ["orders"], "statement": "x", "status": "reviewed"},
            {"id": "b", "contexts": ["orders"], "statement": "x"},
            {"id": "c", "contexts": ["orders", "billing"], "statement": "x", "status": "reviewed"},
            {"id": "d", "contexts": ["ghost"], "statement": "x"},
        ])
        memory = str(self.repo / "memory")
        bare = self.run_cli("coverage", "--registry-root", memory)
        self.assertEqual(
            ([{"context": "ghost", "reviewed_rules": 0, "candidate_rules": 1},
              {"context": "ops", "reviewed_rules": 0, "candidate_rules": 0},
              {"context": "billing", "reviewed_rules": 1, "candidate_rules": 0},
              {"context": "orders", "reviewed_rules": 2, "candidate_rules": 1}],
             f"NOTE: no Change Packages at {(self.repo / 'memory').resolve() / 'changes'}; pass --changes-root to count "
             "the rules a test proves."),
            (json.loads(bare.stdout)["rules_by_context"], bare.stderr.strip()),
        )
        self.assertEqual(
            {"reviewed_rules": 2, "candidate_rules": 2, "contexts_without_reviewed_rules": 2},
            json.loads(bare.stdout)["rule_totals"],
        )
        (self.repo / "memory" / "changes").mkdir()
        proved = json.loads(self.run_cli("coverage", "--registry-root", memory).stdout)
        self.assertEqual(
            ([("ghost", [], []), ("ops", [], []), ("billing", [], ["c"]), ("orders", [], ["a", "c"])],
             {"reviewed_rules": 2, "candidate_rules": 2, "proven": 0, "unproven": 2, "unknown": 0,
              "contexts_without_reviewed_rules": 2}),
            ([(row["context"], row["proven"], row["unproven"]) for row in proved["rules_by_context"]],
             proved["rule_totals"]),
        )

    def test_validate_notes_a_candidate_implementation_path_over_no_file_instead_of_refusing_it(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "implementation_path": "src/orders"},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices.", "implementation_path": "src/billing"},
        ])
        validated = self.run_cli("validate", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        self.assertEqual(
            (0, "Registry is valid.",
             "NOTE: contexts/billing implementation_path src/billing, contexts/orders implementation_path src/orders lie "
             "over no file in the repository; a candidate may name code to come, and ship-package refuses to "
             "review it until code is there or the path is corrected."),
            (validated.returncode, validated.stdout.strip(), validated.stderr.strip()),
        )

    def test_relocate_evidence_notes_when_nothing_under_from_needs_moving(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        registry = ("--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        in_place = self.run_cli("relocate-evidence", *registry, "--from", "src", "--to", "lib")
        nothing = self.run_cli("relocate-evidence", *registry, "--from", "trial/batch", "--to", "trial/batchy")
        self.assertEqual(
            ((0, "NOTE: 1 citation under src is still in place and no Context ends with it; nothing to move."),
             (0, "NOTE: no record cites a path under trial/batch and no Context ends with it; nothing to move. "
                 "Pass the path as the repository holds it, such as src/billing/orders for a Context at billing/orders.")),
            ((in_place.returncode, in_place.stderr.strip()), (nothing.returncode, nothing.stderr.strip())),
        )

    def test_relocate_evidence_notes_a_context_placed_under_a_to_that_holds_no_file(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "implementation_path": "src"},
        ])
        registry = ("--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        typo = self.run_cli("relocate-evidence", *registry, "--from", "src", "--to", "lib")
        self.assertEqual(
            (0, "NOTE: --to lib lies over no file in the repository; the Context path proposed for orders would be "
                "refused at review. Check --to for a typo."),
            (typo.returncode, typo.stderr.strip()),
        )

    def test_validate_change_package_notes_a_counterfactual_that_mutates_a_file_the_rule_does_not_cite(self) -> None:
        package = self.draft_package()
        (self.repo / "src").mkdir()
        (self.repo / "src" / "a.py").write_text("total >= 0\n", encoding="utf-8")
        proposal_path = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["record"]["evidence"] = [citation(self.repo, "src/a.py", 1, 1)]
        proposal["rule_ids"] = ["order-total"]
        proposal["implementation_design"]["proof_obligations"].append("OB-2")
        proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
        obligations_path = package / "test-obligations.json"
        obligations = json.loads(obligations_path.read_text(encoding="utf-8"))
        obligations["obligations"].append({
            **obligations["obligations"][0], "id": "OB-2", "source_type": "rule", "source_id": "order-total",
        })
        obligations_path.write_text(json.dumps(obligations), encoding="utf-8")
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["counterfactual_check"]["obligation_id"] = "OB-2"
        validate = ("validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"))
        noted = {}
        for label, mutated in (("elsewhere", "gen/query.sql:3: = $2 -> IS NOT NULL"), ("cited", "src/a.py:1: >= 0 -> >= -1")):
            evidence["counterfactual_check"]["mutated_protection"] = mutated
            evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
            shown = self.run_cli(*validate)
            noted[label] = (shown.returncode, shown.stderr.strip())
        self.assertEqual(
            {"elsewhere": (0, "NOTE: the counterfactual proving order-total mutates gen/query.sql, which the rule does not cite. "
                              "That proves the rule only when the mutated line lies on the execution path of a cited line, "
                              "as the query a cited call runs does; otherwise mutate a cited line."),
             "cited": (0, "")},
            noted,
        )

    def test_validate_names_the_approved_packages_left_unapplied(self) -> None:
        changes = self.repo / "memory" / "changes"
        for name, status in (("a", "approved"), ("b", "applied"), ("c", "approved"), ("d", "verified")):
            (changes / name).mkdir(parents=True)
            (changes / name / "domain-change-proposal.json").write_text(
                json.dumps({"proposal_id": f"PRO-{name}", "status": status}), encoding="utf-8"
            )
        validate = ("validate", "--registry-root", str(self.repo / "memory"))
        validated = self.run_cli(*validate)
        self.assertEqual(
            (0, "Registry is valid.", "NOTE: 2 approved Change Packages not applied: PRO-a, PRO-c. Apply each with ship-package; retire one that cannot be applied with supersede-proposal, and redraft-proposal carries its work into a new draft."),
            (validated.returncode, validated.stdout.strip(), validated.stderr.strip()),
        )
        (changes / "c" / "domain-change-proposal.json").write_text(
            json.dumps({"proposal_id": "PRO-c", "status": "superseded"}), encoding="utf-8"
        )
        self.assertEqual("NOTE: 1 approved Change Package not applied: PRO-a. Apply each with ship-package; retire one that cannot be applied with supersede-proposal, and redraft-proposal carries its work into a new draft.", self.run_cli(*validate).stderr.strip())
        (changes / "a" / "domain-change-proposal.json").write_text(
            json.dumps({"proposal_id": "PRO-a", "status": "applied"}), encoding="utf-8"
        )
        self.assertEqual("", self.run_cli(*validate).stderr.strip())

    def test_a_replacement_no_package_carries_is_noted_until_one_does(self) -> None:
        changes = self.repo / "memory" / "changes"
        for name, proposal in (("a", {"proposal_id": "PRO-a", "status": "superseded", "superseded_by": "PRO-x"}),
                               ("b", {"proposal_id": "PRO-b", "status": "superseded", "superseded_by": "PRO-a"})):
            (changes / name).mkdir(parents=True)
            (changes / name / "domain-change-proposal.json").write_text(json.dumps(proposal), encoding="utf-8")
        validate = ("validate", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(
            "NOTE: 1 superseded Change Package names a replacement no package carries: PRO-a (by PRO-x). "
            "Create it with redraft-proposal --proposal-id <id>.",
            self.run_cli(*validate).stderr.strip(),
        )
        (changes / "x").mkdir()
        (changes / "x" / "domain-change-proposal.json").write_text(
            json.dumps({"proposal_id": "PRO-x", "status": "draft"}), encoding="utf-8"
        )
        self.assertEqual("", self.run_cli(*validate).stderr.strip())
        init_change_package(changes / "y")
        retire = ("supersede-proposal", "--package-root", str(changes / "y"), "--reason", "replaced", "--superseded-by")
        self.assertEqual("", self.run_cli(*retire, "PRO-x").stderr.strip())

    def test_supersede_and_redraft_take_the_lifecycle_flags_and_say_what_replaces_the_proposal(self) -> None:
        lifecycle = ("--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        for name, replacement, said in (
            ("a", (), "Proposal superseded. Its record stays; create a new draft for the replacement."),
            ("b", ("--superseded-by", "PRO-2"), "Proposal superseded by PRO-2. Its record stays."),
        ):
            with self.subTest(said):
                init_change_package(self.repo / name)
                retired = self.run_cli("supersede-proposal", "--package-root", str(self.repo / name), "--reason",
                                       "replaced", *replacement, *lifecycle)
                self.assertEqual((0, said), (retired.returncode, retired.stdout.strip()))
        self.assertTrue(retired.stderr.startswith(f"NOTE: no package beside {self.repo / 'b'} carries PRO-2 yet;"))
        redrafted = self.run_cli("redraft-proposal", "--package-root", str(self.repo / "a"), "--output",
                                 str(self.repo / "c"), "--proposal-id", "PRO-3", *lifecycle)
        self.assertEqual(0, redrafted.returncode, redrafted.stdout)

    def test_a_shifted_citation_does_not_fail_verify_evidence(self) -> None:
        self.seed_two_cited_rules()
        result = self.run_cli("verify-evidence", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        self.assertEqual((0, 1), (result.returncode, json.loads(result.stdout)["summary"]["shifted"]))

    def test_list_proposals_reports_each_package_with_its_status_and_updates(self) -> None:
        changes = self.repo / "changes"
        for name, fields in (
            ("a", {"proposal_id": "a", "status": "draft", "registry_updates": []}),
            ("b", {"proposal_id": "b", "status": "applied", "submitted_at": "2026-10-01T00:00:00Z",
                   "applied_at": "2026-10-02T00:00:00Z", "registry_updates": [{}], "supersedes": ["a"]}),
            ("c", {"proposal_id": "c", "status": "superseded", "submitted_at": "2026-10-01T00:00:00Z",
                   "superseded_at": "2026-10-03T00:00:00Z", "superseded_by": "b"}),
            ("d", {"proposal_id": "d", "status": "superseded", "superseded_at": "2026-10-04T00:00:00Z",
                   "superseded_by": "nosuch"}),
            ("e", {"proposal_id": "e", "status": "draft", "redrafted_at": "2026-10-05T00:00:00Z", "supersedes": ["a", "gone"]}),
        ):
            (changes / name).mkdir(parents=True)
            (changes / name / "domain-change-proposal.json").write_text(json.dumps(fields), encoding="utf-8")
        (changes / "not-a-package").mkdir()
        result = self.run_cli("list-proposals", "--changes-root", str(changes))
        self.assertEqual(
            [{"package": "a", "proposal_id": "a", "status": "draft", "last_changed_at": None, "registry_updates": 0, "supersedes": []},
             {"package": "b", "proposal_id": "b", "status": "applied", "last_changed_at": "2026-10-02T00:00:00Z",
              "registry_updates": 1, "supersedes": ["a"]},
             {"package": "c", "proposal_id": "c", "status": "superseded", "last_changed_at": "2026-10-03T00:00:00Z",
              "registry_updates": 0, "supersedes": [], "superseded_by": "b"},
             {"package": "d", "proposal_id": "d", "status": "superseded", "last_changed_at": "2026-10-04T00:00:00Z",
              "registry_updates": 0, "supersedes": [], "superseded_by": "nosuch", "superseded_by_unknown": True},
             {"package": "e", "proposal_id": "e", "status": "draft", "last_changed_at": "2026-10-05T00:00:00Z",
              "registry_updates": 0, "supersedes": ["a", "gone"], "supersedes_unknown": ["gone"]}],
            json.loads(result.stdout),
        )

    def test_list_proposals_flags_packages_sharing_a_proposal_id(self) -> None:
        changes = self.repo / "changes"
        for name, proposal_id in (("a", "same"), ("b", "same"), ("c", "other")):
            (changes / name).mkdir(parents=True)
            (changes / name / "domain-change-proposal.json").write_text(
                json.dumps({"proposal_id": proposal_id, "status": "draft"}), encoding="utf-8")
        rows = json.loads(self.run_cli("list-proposals", "--changes-root", str(changes)).stdout)
        self.assertEqual([("a", True), ("b", True), ("c", False)],
                         [(row["package"], row.get("duplicate_proposal_id", False)) for row in rows])

    def validated_package(self, *, updates: bool) -> list[str]:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        if not updates:
            proposal["registry_updates"] = []
        path.write_text(json.dumps(proposal), encoding="utf-8")
        result = self.run_cli("validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"))
        return result.stdout.splitlines()

    def test_a_valid_package_that_changes_no_record_says_so(self) -> None:
        self.assertEqual(
            ["Change package is valid.", "NOTE: this package changes no Registry record; add the updates it is for, or supersede it."],
            self.validated_package(updates=False),
        )

    def test_a_preview_before_approval_says_approvals_are_not_asked_for(self) -> None:
        package = self.draft_package()
        result = self.run_cli(
            "validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
            "--as-status", "submitted",
        )
        self.assertEqual(
            "Change package has everything submitted requires, apart from the approvals, which are given only once it is submitted.",
            result.stdout.strip(),
        )

    def test_a_preview_of_an_approved_package_does_not_say_approvals_are_missing(self) -> None:
        package = self.submitted_package()
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        result = self.run_cli(
            "validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
            "--as-status", "submitted",
        )
        self.assertEqual("Change package has everything submitted requires.", result.stdout.strip())

    def test_json_piped_from_a_console_in_another_code_page_is_utf8(self) -> None:
        self.seed("contexts.json", [{"id": "testlab", "name": "Testlab", "responsibility": "Owns drafts\uff0ctheir prompts."}])
        result = subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", "get-context", "--registry-root", str(self.repo / "memory"),
             "--id", "testlab"],
            cwd=Path(__file__).parent,
            capture_output=True,
            env={**os.environ, "PYTHONIOENCODING": "cp950"},
            check=False,
        )
        self.assertEqual("Owns drafts\uff0ctheir prompts.", json.loads(result.stdout.decode("utf-8"))["context"]["responsibility"])

    def test_a_valid_package_with_updates_prints_only_its_verdict(self) -> None:
        self.assertEqual(["Change package is valid."], self.validated_package(updates=True))

    def test_a_shifted_citation_fails_verify_evidence_when_strict(self) -> None:
        self.seed_two_cited_rules()
        result = self.run_cli(
            "verify-evidence", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo), "--strict"
        )
        self.assertEqual(
            (1, "verify-evidence failed: 1 shifted citations; run recite on each to cite its current lines"),
            (result.returncode, result.stderr.strip()),
        )

    def test_strict_passes_when_every_citation_is_current(self) -> None:
        self.seed_two_cited_rules()
        result = self.run_cli(
            "verify-evidence", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--strict", "--path", "other.md",
        )
        self.assertEqual((0, {"current": 1, "shifted": 0}),
                         (result.returncode, {key: json.loads(result.stdout)["summary"][key] for key in ("current", "shifted")}))

    def test_rebase_proposal_prints_the_restamped_package_and_a_refusal_goes_to_stderr(self) -> None:
        package = self.submitted_package()
        arguments = ("rebase-proposal", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
                     "--repo-root", str(self.repo))
        current = self.run_cli(*arguments)
        self.edit_registry("contexts.json", lambda value: value["contexts"][0].update(name="Purchases"))
        rebased = self.run_cli(*arguments)
        printed = json.loads(rebased.stdout)
        self.assertEqual(
            (1, "ERROR: the proposal base is current; nothing to rebase", "",
             0, ("proposal", "submitted", 2, 0, False), self.proposal_in(package)["base_registry_revision"]),
            (current.returncode, current.stderr.strip(), current.stdout,
             rebased.returncode, (printed["package"], printed["status"], printed["proposal_revision"],
                                  printed["approvals_retired"], printed["attestation_removed"]),
             printed["base_registry_revision"]),
        )

    def test_verify_proposal_run_again_on_a_verified_package_says_so_and_exits_zero(self) -> None:
        package = self.submitted_package()
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        arguments = ("verify-proposal", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
                     "--repo-root", str(self.repo))
        first = self.run_cli(*arguments)
        again = self.run_cli(*arguments)
        self.assertEqual(
            (0, "Proposal test evidence verified.", 0, "already verified at revision 1; nothing was written", "", "verified"),
            (first.returncode, first.stdout.strip(), again.returncode, again.stdout.strip(), again.stderr,
             self.proposal_in(package)["status"]),
        )
        path = package / "domain-change-proposal.json"
        path.write_text(json.dumps({**self.proposal_in(package), "status": "applied"}), encoding="utf-8")
        shipped = self.run_cli(*arguments)
        self.assertEqual(
            (0, "already applied at revision 1; nothing was written", ""),
            (shipped.returncode, shipped.stdout.strip(), shipped.stderr),
        )


if __name__ == "__main__":
    unittest.main()
