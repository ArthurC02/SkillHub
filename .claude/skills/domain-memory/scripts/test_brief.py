from __future__ import annotations

import json
import unittest

from domain_registry.brief import brief_for_change, brief_for_record, to_markdown
from domain_registry.evidence import citation
from test_support import RegistryTestCase

INTERACTION = {"id": "orders-bill", "producer_context": "orders", "consumer_context": "billing", "description": "Orders asks billing to invoice."}
RULE = {"id": "order-total", "contexts": ["orders"], "statement": "An order total is non-negative."}


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
        [record] = brief["records"]
        self.assertEqual(
            ("boundary-evidence-changed", 1, ["orders"], ["src/orders/port.py"], "Own orders.", ["order-total"],
             "interactions", "orders-bill", "candidate", ["src/orders/port.py"], ["src/orders/port.py:1-1"],
             [("contexts", "billing"), ("contexts", "orders")], 2, {"citations": 2, "exists": True}),
            (brief["verdict"], brief["subject"]["changed_files"], [context["id"] for context in brief["contexts"]],
             brief["contexts"][0]["changed_files"], brief["contexts"][0]["model"]["context"]["summary"],
             [rule["id"] for rule in brief["contexts"][0]["model"]["rules"]],
             record["asset"], record["id"], record["status"], record["changed_evidence"], record["touched_evidence"],
             [(item["asset"], item["id"]) for item in record["dependents"]],
             brief["evidence"]["summary"]["stale"], brief["evidence"]["paths"]["src/orders/port.py"]),
        )
        self.assertEqual(
            ["A collaboration's cited lines changed; re-read each record below, then recite or revise it.",
             "interactions/orders-bill cites src/orders/port.py at lines that changed; recite or revise it.",
             "2 stale citations of the changed files; recite them before relying on the records."],
            brief["attention"],
        )

    def test_a_change_inside_one_context_with_nothing_cited_has_one_line_of_attention(self) -> None:
        self.two_mapped_contexts()
        self.change("src/orders/new.py")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            ("no-cross-context-change", ["The change stays inside one Context."], [], {"citations": 0, "exists": True}),
            (brief["verdict"], brief["attention"], brief["records"], brief["evidence"]["paths"]["src/orders/new.py"]),
        )

    def test_a_file_outside_every_context_that_a_record_cites_is_called_out(self) -> None:
        self.two_mapped_contexts(rules=[{**RULE, "evidence": [{"path": "README.md", "lines": {"start": 1, "end": 1}}]}])
        (self.repo / "README.md").write_text("new\n", encoding="utf-8")
        self.commit_all("readme")
        self.change("README.md")
        brief = brief_for_change(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(
            [{"path": "README.md", "cited_by": ["rules/order-total"]},
             "README.md lies outside every Context but is cited by rules/order-total; place it or move the citation."],
            [brief["outside_every_context"][0], brief["attention"][1]],
        )

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
                         "- interactions/orders-bill cites src/orders/port.py at lines that changed; recite or revise it.",
                         "### orders (candidate): Own orders.", "### interactions/orders-bill (candidate): Orders asks billing to invoice.",
                         "- contexts/orders (candidate): Own orders. via interactions/orders-bill.producer_context at depth 1", "- stale: 1"):
            self.assertIn(expected, markdown)


class RecordBriefTest(BriefTest):
    def test_a_candidate_rule_is_briefed_with_its_contexts_dependents_and_verified_citations(self) -> None:
        self.two_mapped_contexts(rules=[RULE])
        brief = brief_for_record(self.repo / "memory", self.repo, "rules", "order-total")
        self.assertEqual(
            ({"id": "order-total", "status": "candidate", "summary": "An order total is non-negative.", "asset": "rules"},
             ["orders"], [("contexts", "orders", "candidate")], 1,
             ["The record is candidate, not a reviewed constraint.",
              "1 records reach it through relations and may need the same change; they are listed."]),
            (brief["record"], [context["id"] for context in brief["contexts"]],
             [(item["asset"], item["id"], item["status"]) for item in brief["dependents"]], brief["evidence"]["summary"]["current"],
             brief["attention"]),
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
