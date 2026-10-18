from __future__ import annotations

import json
import subprocess
import unittest

from domain_registry.brief import brief_for_change, brief_for_record, to_markdown
from domain_registry.evidence import citation
from domain_registry.sources import confirm_sources, refresh_sources, selected_source_map, write_source_map
from test_support import RegistryTestCase, applied_by

INTERACTION = {"id": "orders-bill", "producer_context": "orders", "consumer_context": "billing", "description": "Orders asks billing to invoice."}
RULE = {"id": "order-total", "contexts": ["orders"], "statement": "An order total is non-negative."}
SECOND_RULE = {"id": "order-min", "contexts": ["orders"], "statement": "An order has at least one line."}
CHECK = "python -m unittest test_orders"


class BriefTest(RegistryTestCase):
    def two_mapped_contexts(self, **collaborations: list[dict]) -> None:
        super().two_mapped_contexts()
        for name, records in collaborations.items():
            self.seed(f"{name.replace('_', '-')}.json", [
                {**record, "evidence": [citation(self.repo, "src/orders/port.py", 1, 1)]} if "evidence" not in record else record
                for record in records
            ])
        if collaborations:
            self.commit_all("cited")


class ChangeBriefTest(BriefTest):
    def test_a_change_to_cited_boundary_lines_names_the_record_its_dependents_and_the_stale_citations(self) -> None:
        self.two_mapped_contexts(interactions=[INTERACTION], rules=[RULE])
        self.change("src/orders/port.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        record, rule = brief["records"]
        self.assertEqual(
            ("boundary-evidence-changed", 1, ["orders"], ["src/orders/port.py"], "Own orders.", ["order-total"],
             "interactions", "orders-bill", "candidate", ["src/orders/port.py"], ["src/orders/port.py:1-1"],
             [("contexts", "billing"), ("contexts", "orders")],
             ("rules", "order-total", ["src/orders/port.py:1-1"], ["src/orders/port.py"], None, None, False),
             2, {"citations": 2, "exists": True}, []),
            (brief["verdict"], brief["subject"]["changed_files"], [context["id"] for context in brief["contexts"]],
             brief["contexts"][0]["changed_files"], brief["contexts"][0]["model"]["context"]["summary"],
             [rule["id"] for rule in brief["contexts"][0]["model"]["rules"]],
             record["asset"], record["id"], record["status"], record["changed_evidence"], record["touched_evidence"],
             [(item["asset"], item["id"]) for item in record["dependents"]],
             (rule["asset"], rule["id"], rule["touched_evidence"], rule["changed_evidence"], rule["check"], rule["cwd"], "check" in record),
             brief["evidence"]["summary"]["stale"], brief["evidence"]["paths"]["src/orders/port.py"], brief["registry_changes"]),
        )
        self.assertEqual(
            ["A collaboration's cited lines changed; re-read each record below, and recite only what the code still states.",
             "interactions/orders-bill cites src/orders/port.py at lines that changed; check that its statement still holds before reciting: "
             "Orders asks billing to invoice.",
             "rules/order-total cites src/orders/port.py at lines that changed; check that its statement still holds before reciting: "
             "An order total is non-negative.",
             "1 rule cites lines this change touched; re-run their checks, which get-record names.",
             "2 stale citations of the changed files; recite only a record whose statement still holds."],
            brief["attention"],
        )

    def test_a_rule_citing_a_test_file_added_in_the_range_is_listed_with_the_lines_the_change_touched(self) -> None:
        super().two_mapped_contexts()
        (self.repo / "src/orders/port_test.py").write_text("def test_limit():\n    assert True\n", encoding="utf-8")
        self.seed("rules.json", [{**RULE, "evidence": [citation(self.repo, "src/orders/port.py", 1, 1), citation(self.repo, "src/orders/port_test.py", 1, 2)]}])
        self.commit_all("adds the test and the rule")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD~1")
        [rule] = brief["records"]
        self.assertEqual(
            ("no-cross-context-change", "rules", "order-total", ["src/orders/port_test.py:1-2"], 1,
             ["The change stays inside one Context.",
              "rules/order-total cites src/orders/port_test.py:1-2, which the change overlaps; re-read it.",
              "1 rule cites lines this change touched; re-run their checks, which get-record names.",
              "rules: 1 record changed in this range, without a reviewing proposal."]),
            (brief["verdict"], rule["asset"], rule["id"], rule["touched_evidence"], brief["evidence"]["summary"]["current"], brief["attention"]),
        )

    def test_a_change_inside_one_context_with_nothing_cited_has_one_line_of_attention(self) -> None:
        self.two_mapped_contexts()
        self.change("src/orders/new.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ("no-cross-context-change", ["The change stays inside one Context."], [], {"citations": 0, "exists": True}, {"changed": False}),
            (brief["verdict"], brief["attention"], brief["records"], brief["evidence"]["paths"]["src/orders/new.py"], brief["source_map"]),
        )

    def test_a_file_outside_every_context_of_unknown_kind_that_a_record_cites_is_called_out(self) -> None:
        self.two_mapped_contexts(rules=[{**RULE, "evidence": [{"path": "README.md", "lines": {"start": 1, "end": 1}}]}])
        (self.repo / "README.md").write_text("new\n", encoding="utf-8")
        self.commit_all("readme")
        self.change("README.md")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual([{"path": "README.md", "cited_by": ["rules/order-total"], "cited_as": None}], brief["outside_every_context"])
        self.assertIn("README.md lies outside every Context but is cited by rules/order-total; place it or move the citation.", brief["attention"])

    def cited_outside_every_context(self, path: str) -> dict:
        self.two_mapped_contexts(rules=[{**RULE, "evidence": [{"path": path, "lines": {"start": 1, "end": 1}}]}])
        (self.repo / path).parent.mkdir(parents=True, exist_ok=True)
        (self.repo / path).write_text("v1\n", encoding="utf-8")
        self.committed_source_map()
        self.change(path)
        return brief_for_change(self.repo / "memory", self.repo, "HEAD")

    def test_a_file_outside_every_context_cited_as_enforcing_evidence_is_called_out_with_its_kind(self) -> None:
        brief = self.cited_outside_every_context("src/shared/money.py")
        self.assertEqual(
            [{"path": "src/shared/money.py", "cited_by": ["rules/order-total"], "cited_as": "implementation"}], brief["outside_every_context"]
        )
        self.assertIn(
            "src/shared/money.py lies outside every Context but is cited by rules/order-total; place it or move the citation.", brief["attention"]
        )

    def test_a_test_file_outside_every_context_cited_only_as_proof_is_listed_without_an_action(self) -> None:
        brief = self.cited_outside_every_context("api/server_test.py")
        self.assertEqual(
            ([{"path": "api/server_test.py", "cited_by": ["rules/order-total"], "cited_as": "tests"}], []),
            (brief["outside_every_context"], [line for line in brief["attention"] if "outside every Context" in line]),
        )

    def test_a_source_map_refreshed_and_confirmed_in_the_range_names_who_confirmed_it_and_the_discovered_files(self) -> None:
        self.two_mapped_contexts()
        (self.repo / "tests").mkdir()
        (self.repo / "tests/test_old.py").write_text("old\n", encoding="utf-8")
        self.committed_source_map()
        (self.repo / "tests/test_old.py").unlink()
        for name in ("test_new.py", "test_more.py"):
            (self.repo / "tests" / name).write_text("new\n", encoding="utf-8")
        refresh_sources(self.repo / "memory", self.repo)
        confirm_sources(self.repo / "memory", self.repo, "human:ann")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        line = "source map re-confirmed in this range by human:ann (+2 -1 discovered files)"
        self.assertEqual(
            ({"changed": True, "confirmed_by": "human:ann", "confirmation": "developer-confirmed", "discovered_added": 2, "discovered_removed": 1},
             line),
            (brief["source_map"], brief["attention"][-1]),
        )
        self.assertIn(f"## Source map\n\n- {line}\n", to_markdown(brief))

    def test_a_source_map_written_in_the_range_without_confirmation_says_it_is_not_confirmed(self) -> None:
        self.two_mapped_contexts()
        (self.repo / "docs").mkdir()
        (self.repo / "docs/kept.md").write_text("kept", encoding="utf-8")
        write_source_map(self.repo / "memory" / "source-map.json", selected_source_map(self.repo, [self.repo / "docs"]))
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ({"changed": True, "confirmed_by": None, "confirmation": "agent-asserted", "discovered_added": 1, "discovered_removed": 0},
             "source map changed in this range and is agent-asserted, not confirmed (+1 -0 discovered file)"),
            (brief["source_map"], brief["attention"][-1]),
        )

    def test_a_source_map_removed_in_the_range_is_said_to_be_removed(self) -> None:
        self.two_mapped_contexts()
        self.committed_source_map().unlink()
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ({"changed": True, "confirmed_by": None, "confirmation": "removed", "discovered_added": 0, "discovered_removed": 1},
             "source map changed in this range and is removed, not confirmed (+0 -1 discovered file)"),
            (brief["source_map"], brief["attention"][-1]),
        )

    def test_a_source_map_unchanged_in_the_range_adds_no_line_and_no_markdown_section(self) -> None:
        self.two_mapped_contexts()
        self.committed_source_map()
        self.change("src/orders/new.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(({"changed": False}, ["The change stays inside one Context."]), (brief["source_map"], brief["attention"]))
        self.assertNotIn("## Source map", to_markdown(brief))

    def test_without_implementation_paths_the_brief_says_files_cannot_be_mapped(self) -> None:
        self.add_context()
        self.commit_all("unmapped")
        self.change("src/orders/port.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ("unknown", "No Context records an implementation_path, so the changed files cannot be mapped to Contexts.",
             {"citations": 0, "exists": True}, []),
            (brief["verdict"], brief["attention"][0], brief["evidence"]["paths"]["src/orders/port.py"], brief["contexts"]),
        )

    def test_the_markdown_brief_carries_the_verdict_the_attention_and_each_record(self) -> None:
        self.two_mapped_contexts(interactions=[INTERACTION])
        self.change("src/orders/port.py")
        markdown = to_markdown(brief_for_change(self.repo / "memory", self.repo, "HEAD"))
        for expected in ("# Change brief: since HEAD", "**Verdict:** `boundary-evidence-changed`",
                         "- interactions/orders-bill cites src/orders/port.py at lines that changed; check that its statement still holds before reciting: Orders asks billing to invoice.",
                         "### orders (candidate): Own orders.", "### interactions/orders-bill (candidate): Orders asks billing to invoice.",
                         "- contexts/orders (candidate): Own orders. via interactions/orders-bill.producer_context at depth 1", "- stale: 1"):
            self.assertIn(expected, markdown)


    def prove(self, rule_id: str) -> None:
        package = self.repo / "memory" / "changes" / "pro-1"
        package.mkdir(parents=True)
        for name, value in (
            ("domain-change-proposal.json", {"proposal_id": "PRO-1"}),
            ("test-obligations.json", {"obligations": [{"id": "OB-1", "source_type": "rule", "source_id": rule_id, "check": CHECK}]}),
            ("evidence-bundle.json", {"test_results": [{"obligation_id": "OB-1", "working_directory": "src/orders"}]}),
        ):
            (package / name).write_text(json.dumps(value), encoding="utf-8")
        applied_by(self.repo / "memory", "PRO-1", f"rules/{rule_id}")
        self.commit_all("proved")

    def test_one_rule_and_one_stale_citation_are_counted_in_the_singular(self) -> None:
        self.two_mapped_contexts(rules=[RULE])
        self.change("src/orders/port.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ["The change stays inside one Context.",
             "rules/order-total cites src/orders/port.py at lines that changed; check that its statement still holds before reciting: "
             "An order total is non-negative.",
             "1 rule cites lines this change touched; re-run their checks, which get-record names.",
             "1 stale citation of the changed files; recite only a record whose statement still holds."],
            brief["attention"],
        )

    def test_a_rule_proved_by_a_package_beside_the_registry_carries_its_check_and_says_to_run_it(self) -> None:
        self.two_mapped_contexts(rules=[RULE])
        self.prove("order-total")
        self.change("src/orders/port.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        [rule] = brief["records"]
        self.assertEqual(
            (CHECK, "src/orders", "1 rule cites lines this change touched; run the checks listed on them."),
            (rule["check"], rule["cwd"], brief["attention"][2]),
        )

    def test_when_one_of_two_rules_has_no_proving_package_its_check_is_null_and_get_record_is_named(self) -> None:
        self.two_mapped_contexts(rules=[RULE, SECOND_RULE])
        self.prove("order-total")
        self.change("src/orders/port.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ([("order-min", None, None), ("order-total", CHECK, "src/orders")],
             ["2 rules cite lines this change touched; re-run their checks, which get-record names.",
              "2 stale citations of the changed files; recite only a record whose statement still holds."]),
            ([(rule["id"], rule["check"], rule["cwd"]) for rule in brief["records"]], brief["attention"][3:]),
        )

    def test_records_added_amended_and_removed_in_the_range_name_the_proposal_that_last_reviewed_them(self) -> None:
        kept = {"id": "kept", "contexts": ["orders"], "statement": "Kept.", "evidence": []}
        self.two_mapped_contexts(rules=[
            kept, {"id": "amended", "contexts": ["orders"], "statement": "Before.", "evidence": []},
            {"id": "removed", "contexts": ["orders"], "statement": "Removed.", "evidence": []},
        ])
        self.seed("rules.json", [
            kept, {"id": "amended", "contexts": ["orders"], "statement": "After.", "evidence": []},
            {"id": "added", "contexts": ["orders"], "statement": "Added.", "status": "reviewed", "evidence": []},
        ])
        applied_by(self.repo / "memory", "PRO-1", "rules/amended", "rules/added")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ([{"asset": "rules", "id": "added", "change": "added", "status": "reviewed", "summary": "Added.", "through": "PRO-1"},
              {"asset": "rules", "id": "amended", "change": "amended", "status": "candidate", "summary": "After.", "through": "PRO-1"},
              {"asset": "rules", "id": "removed", "change": "removed", "status": "candidate", "summary": "Removed.", "through": None}],
             ["rules: 2 records changed in this range, last reviewed through PRO-1.",
              "rules: 1 record changed in this range, without a reviewing proposal."]),
            (brief["registry_changes"], brief["attention"][-2:]),
        )

    def test_a_registry_absent_at_the_base_lists_every_record_as_added(self) -> None:
        super().two_mapped_contexts()
        (self.repo / ".git" / "info" / "exclude").write_text("memory/\n", encoding="utf-8")
        subprocess.run(["git", "rm", "-r", "-q", "--cached", "memory"], cwd=self.repo, check=True)
        self.commit_all("untracks the registry")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ([("contexts", "billing", "added", "candidate", "Own invoices.", None),
              ("contexts", "orders", "added", "candidate", "Own orders.", None)],
             "contexts: 2 records changed in this range, without a reviewing proposal."),
            ([(change["asset"], change["id"], change["change"], change["status"], change["summary"], change["through"])
              for change in brief["registry_changes"]], brief["attention"][-1]),
        )

    def test_the_markdown_brief_lists_registry_changes_and_the_check_on_each_rule(self) -> None:
        self.two_mapped_contexts(rules=[RULE, SECOND_RULE])
        self.prove("order-total")
        self.change("src/orders/port.py")
        markdown = to_markdown(brief_for_change(self.repo / "memory", self.repo, "HEAD~2"))
        for expected in ("## Registry changes\n\n"
                         "- added without a reviewing proposal: rules/order-min (candidate): An order has at least one line.\n"
                         "- added through PRO-1: rules/order-total (candidate): An order total is non-negative.\n",
                         f"Check: `{CHECK}` in `src/orders`", "Check: none known; get-record names what proves it"):
            self.assertIn(expected, markdown)
        self.assertIn("## Registry changes\n\n- none\n", to_markdown(brief_for_change(self.repo / "memory", self.repo, "HEAD")))


class RecordBriefTest(BriefTest):
    def test_a_candidate_rule_is_briefed_with_its_contexts_dependents_and_verified_citations(self) -> None:
        self.two_mapped_contexts(rules=[RULE])
        brief = brief_for_record(self.repo / "memory", self.repo, "rules", "order-total")
        self.assertEqual(
            ({"id": "order-total", "status": "candidate", "summary": "An order total is non-negative.", "asset": "rules"},
             ["orders"], [("contexts", "orders", "candidate")], 1,
             ["The record is candidate, not a reviewed constraint.",
              "1 record reaches it through relations and may need the same change; they are listed."]),
            (brief["record"], [context["id"] for context in brief["contexts"]],
             [(item["asset"], item["id"], item["status"]) for item in brief["dependents"]], brief["evidence"]["summary"]["current"],
             brief["attention"]),
        )

    def test_two_records_reaching_a_rule_and_one_stale_citation_are_counted_by_number(self) -> None:
        self.two_mapped_contexts(rules=[{**RULE, "contexts": ["orders", "billing"]}])
        self.change("src/orders/port.py")
        brief = brief_for_record(self.repo / "memory", self.repo, "rules", "order-total")
        self.assertEqual(
            ["The record is candidate, not a reviewed constraint.",
             "2 records reach it through relations and may need the same change; they are listed.",
             "1 stale citation; recite them before relying on the record."],
            brief["attention"],
        )

    def test_a_record_that_cites_nothing_is_said_to_be_unverifiable(self) -> None:
        self.two_mapped_contexts(rules=[{"id": "bare", "contexts": [], "statement": "Bare.", "evidence": []}])
        brief = brief_for_record(self.repo / "memory", self.repo, "rules", "bare")
        self.assertEqual(
            (None, [], ["The record is candidate, not a reviewed constraint.", "The record cites nothing; it cannot be verified against the code."]),
            (brief["evidence"], brief["contexts"], brief["attention"]),
        )

    def test_a_record_the_registry_lacks_is_refused_by_name(self) -> None:
        self.two_mapped_contexts()
        with self.assertRaisesRegex(ValueError, "^rules record not found: nothing$"):
            brief_for_record(self.repo / "memory", self.repo, "rules", "nothing")

    def test_the_command_takes_a_base_or_an_asset_and_id_and_prints_json_or_markdown(self) -> None:
        self.two_mapped_contexts(rules=[RULE])
        roots = ("--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        neither = self.run_cli("brief", *roots)
        both = self.run_cli("brief", *roots, "--base", "HEAD", "--asset", "rules", "--id", "order-total")
        as_json = self.run_cli("brief", *roots, "--asset", "rules", "--id", "order-total")
        as_markdown = self.run_cli("brief", *roots, "--base", "HEAD", "--markdown")
        self.assertEqual(
            ((1, "ERROR: brief needs --base <revision>, or --asset <asset> --id <id>, not both"), 1,
             (0, "domain-memory-brief/v1", {"asset": "rules", "id": "order-total"}), (0, "# Change brief: since HEAD")),
            ((neither.returncode, neither.stdout.strip()), both.returncode,
             (as_json.returncode, json.loads(as_json.stdout)["format"], json.loads(as_json.stdout)["subject"]),
             (as_markdown.returncode, as_markdown.stdout.splitlines()[0])),
        )


if __name__ == "__main__":
    unittest.main()
