from __future__ import annotations

import sys
import unittest
from pathlib import Path
from typing import Any
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.evidence import (
    all_references,
    citation,
    classify_all,
    digest,
    source_map_for,
    verify,
)
from domain_registry.relocate import Recitation, recite, relocate_evidence
from domain_registry.registry import (
    coverage,
    validate,
)
from domain_registry.store import read_document, write_document
from test_support import VERIFIED, RegistryTestCase, applied_by


class EvidenceTest(RegistryTestCase):
    def test_a_malformed_source_map_is_rejected_instead_of_treated_as_empty(
        self,
    ) -> None:
        path = self.repo / "memory" / "source-map.json"
        path.write_text("{not json", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "invalid JSON"):
            source_map_for(self.repo / "memory")

    def test_each_evidence_defect_gets_its_verdict(self) -> None:
        source = self.repo / "fact.md"
        source.write_text("one\ntwo\nthree\n", encoding="utf-8")
        cited = citation(self.repo, "fact.md", 2, 3)
        cases = [
            ("plain text", "fact.md:2", {"status": "invalid", "reason": "evidence must be an object"}),
            ("not an object", 7, {"status": "invalid", "reason": "evidence must be an object"}),
            ("no lines", {"path": "fact.md"}, {"status": "invalid", "reason": "evidence requires path and lines"}),
            ("line zero", {**cited, "lines": {"start": 0, "end": 1}},
             {"status": "invalid", "reason": "evidence line range is invalid"}),
            ("end before start", {**cited, "lines": {"start": 3, "end": 2}},
             {"status": "invalid", "reason": "evidence line range is invalid"}),
            ("outside", {**cited, "path": "../fact.md"},
             {"status": "invalid", "reason": "evidence path escapes repository"}),
            ("absent", {**cited, "path": "gone.md"}, {"status": "missing", "path": "gone.md"}),
            ("past the end without a digest", {"path": "fact.md", "lines": {"start": 3, "end": 4}},
             {"cited_lines": {"start": 3, "end": 4}, "status": "invalid",
              "reason": "evidence line range exceeds source", "path": "fact.md"}),
            ("past the end, excerpt earlier in the file", {**cited, "lines": {"start": 3, "end": 4}},
             {"cited_lines": {"start": 3, "end": 4}, "status": "shifted", "path": "fact.md",
              "lines": {"start": 2, "end": 3},
              "reason": "the cited lines are unchanged but now sit elsewhere in fact.md; this passes "
              "every check and needs no Change Package of its own; when one next touches the record, "
              "relocate-evidence --from fact.md --to fact.md re-cites them and prints the supersedes"}),
            ("past the end, excerpt gone", {**cited, "lines": {"start": 3, "end": 5}},
             {"cited_lines": {"start": 3, "end": 5}, "status": "stale", "path": "fact.md",
              "reason": "the cited excerpt is no longer in fact.md; check that the fact still holds, then recite it"}),
            ("last line", {**cited, "lines": {"start": 3, "end": 3}},
             {"cited_lines": {"start": 3, "end": 3}, "status": "stale", "path": "fact.md",
              "reason": "the cited excerpt is no longer in fact.md; check that the fact still holds, then recite it"}),
        ]
        for label, reference, expected in cases:
            with self.subTest(label):
                self.assertEqual(expected, verify(reference, self.repo))
        self.assertEqual("current", verify(cited, self.repo)["status"])
        with self.subTest("oversized"), patch("domain_registry.evidence.MAX_EVIDENCE_BYTES", 4):
            self.assertEqual(
                {"status": "invalid", "reason": "evidence source exceeds 4 bytes", "path": "fact.md"},
                verify(cited, self.repo),
            )

    def test_a_citation_without_an_excerpt_hash_is_unhashed_not_stale(self) -> None:
        self.two_line_rule()
        result = verify({"path": "rule.md", "lines": {"start": 1, "end": 1}}, self.repo)
        self.assertEqual(
            {"cited_lines": {"start": 1, "end": 1}, "status": "unhashed", "path": "rule.md",
             "reason": "rule.md was cited without an excerpt hash; regenerate the citation with cite"},
            result,
        )

    def test_an_unhashed_citation_ending_on_the_last_line_is_unhashed(self) -> None:
        self.two_line_rule()
        result = verify({"path": "rule.md", "lines": {"start": 2, "end": 2}}, self.repo)
        self.assertEqual("unhashed", result["status"])

    def test_an_unhashed_citation_ending_one_line_past_the_last_is_invalid(self) -> None:
        self.two_line_rule()
        result = verify({"path": "rule.md", "lines": {"start": 3, "end": 3}}, self.repo)
        self.assertEqual(
            ("invalid", "evidence line range exceeds source"), (result["status"], result["reason"])
        )

    def test_a_line_number_that_is_not_an_integer_is_an_invalid_range(self) -> None:
        self.two_line_rule()
        cases = [
            ("a string start", {"start": "1", "end": 1}),
            ("a missing start", {"start": None, "end": 1}),
            ("a string end", {"start": 1, "end": "1"}),
            ("a missing end", {"start": 1, "end": None}),
        ]
        for label, lines in cases:
            with self.subTest(label):
                self.assertEqual(
                    {"status": "invalid", "reason": "evidence line range is invalid"},
                    verify({"path": "rule.md", "lines": lines}, self.repo),
                )

    def eight_byte_rule(self) -> None:
        (self.repo / "rule.md").write_bytes(b"one\ntwo\n")

    def test_a_source_of_exactly_the_evidence_size_limit_is_accepted(self) -> None:
        self.eight_byte_rule()
        reference = citation(self.repo, "rule.md", 1, 1)
        with patch("domain_registry.evidence.MAX_EVIDENCE_BYTES", 8):
            self.assertEqual("current", verify(reference, self.repo)["status"])

    def test_a_source_one_byte_over_the_evidence_size_limit_is_refused(self) -> None:
        self.eight_byte_rule()
        reference = citation(self.repo, "rule.md", 1, 1)
        with patch("domain_registry.evidence.MAX_EVIDENCE_BYTES", 7):
            self.assertEqual(
                {"status": "invalid", "reason": "evidence source exceeds 7 bytes", "path": "rule.md"},
                verify(reference, self.repo),
            )

    def test_a_citation_of_a_source_of_exactly_the_evidence_size_limit_is_built(self) -> None:
        self.eight_byte_rule()
        with patch("domain_registry.evidence.MAX_EVIDENCE_BYTES", 8):
            self.assertEqual({"start": 1, "end": 2}, citation(self.repo, "rule.md", 1, 2)["lines"])

    def test_a_citation_of_a_source_one_byte_over_the_evidence_size_limit_is_refused(self) -> None:
        self.eight_byte_rule()
        with patch("domain_registry.evidence.MAX_EVIDENCE_BYTES", 7), self.assertRaisesRegex(
            ValueError, "exceeds 7 bytes"
        ):
            citation(self.repo, "rule.md", 1, 2)

    def test_evidence_classification_accepts_nesting_at_the_depth_limit(self) -> None:
        value: dict[str, Any] = {}
        nested: dict[str, Any] = value
        for _ in range(100):
            child: dict[str, Any] = {}
            nested["child"] = child
            nested = child
        self.assertEqual(value, classify_all(value, {}))

    def evidence_below_nesting(self, levels: int) -> dict[str, Any]:
        value: dict[str, Any] = {}
        nested: dict[str, Any] = value
        for _ in range(levels):
            child: dict[str, Any] = {}
            nested["child"] = child
            nested = child
        nested["evidence"] = ["deepest"]
        return value

    def test_listing_references_accepts_a_reference_at_the_depth_limit(self) -> None:
        self.assertEqual(["deepest"], all_references(self.evidence_below_nesting(98)))

    def test_listing_references_rejects_a_reference_one_level_past_the_depth_limit(self) -> None:
        with self.assertRaisesRegex(ValueError, "nesting depth 100"):
            all_references(self.evidence_below_nesting(99))

    def test_evidence_classification_rejects_excessive_nesting(self) -> None:
        value: dict[str, Any] = {}
        nested: dict[str, Any] = value
        for _ in range(101):
            child: dict[str, Any] = {}
            nested["child"] = child
            nested = child
        with self.assertRaisesRegex(ValueError, "nesting depth"):
            classify_all(value, {})

    def test_evidence_classification_rejects_excessive_list_nesting(self) -> None:
        value: list[Any] = []
        nested = value
        for _ in range(101):
            child: list[Any] = []
            nested.append(child)
            nested = child
        with self.assertRaisesRegex(ValueError, "nesting depth"):
            classify_all(value, {})

    def test_structured_evidence_detects_source_drift(self) -> None:
        source = self.repo / "evidence.md"
        source.write_bytes(b"one\ntwo\n")
        reference = {
            "path": "evidence.md",
            "lines": {"start": 2, "end": 2},
            "content_sha256": "sha256:c3f9c8c283a2b1f2f1896f27a01cbe3cddc0c9d93f752e4639035a0f5b36f6e8",
            "excerpt_sha256": "sha256:27dd8ed44a83ff94d557f9fd0412ed5a8cbca69ea04922d88c01184a07300a5a",
        }
        self.assertEqual("current", verify(reference, self.repo)["status"])
        source.write_text("one\nchanged\n", encoding="utf-8")
        self.assertEqual("stale", verify(reference, self.repo)["status"])

    def test_structured_evidence_allows_changes_outside_the_cited_excerpt(self) -> None:
        source = self.repo / "evidence.md"
        source.write_text("before\nfact\nafter\n", encoding="utf-8")
        reference = citation(self.repo, "evidence.md", 2, 2)

        source.write_text("changed\nfact\nafter\n", encoding="utf-8")

        self.assertEqual("current", verify(reference, self.repo)["status"])

    def test_coverage_names_a_reviewed_record_citing_outside_the_sources(self) -> None:
        self.select_docs_as_sources()
        self.reviewed_rule([citation(self.repo, "README.md", 1, 1)])
        self.assertEqual(
            [
                {
                    "asset": "rules",
                    "id": "order-total",
                    "reason": "outside the selected sources",
                    "paths": ["README.md"],
                }
            ],
            coverage(self.repo / "memory")["evidence_gaps"],
        )

    def test_coverage_says_nothing_about_a_record_citing_a_selected_source(
        self,
    ) -> None:
        self.select_docs_as_sources()
        self.reviewed_rule([citation(self.repo, "docs/decision.md", 1, 1)])
        self.assertEqual([], coverage(self.repo / "memory")["evidence_gaps"])

    def test_a_reviewed_internal_interaction_does_not_require_a_contract(self) -> None:
        self.two_contexts()
        source = self.repo / "fact.md"
        source.write_text("Orders notify Billing.", encoding="utf-8")
        value = read_document(self.repo / "memory", "interactions.json")
        value["status"] = "reviewed"
        value["interactions"] = [{"id": "order-billed", "producer_context": "orders", "consumer_context": "billing", "consistency": "eventual", "delivery": "outbox event", "evidence": [citation(self.repo, "fact.md", 1, 1)], "verified": VERIFIED}]
        write_document(self.repo / "memory", "interactions.json", value)
        self.assertEqual(validate(self.repo / "memory", self.repo, False), [])

    def test_citation_refuses_an_oversized_source(self) -> None:
        source = self.repo / "large.md"
        with source.open("wb") as output:
            output.truncate(16 * 1024 * 1024 + 1)
        with self.assertRaisesRegex(ValueError, "exceeds"):
            citation(self.repo, "large.md", 1, 1)

    def test_a_built_citation_verifies_as_current(self) -> None:
        self.cited_file()
        reference = citation(self.repo, "docs/adr/boundaries.md", 2, 3)
        self.assertEqual(verify(reference, self.repo)["status"], "current")

    def test_a_citation_covers_only_the_lines_it_names(self) -> None:
        path = self.cited_file()
        reference = citation(self.repo, "docs/adr/boundaries.md", 2, 3)
        whole_file = digest(path.read_bytes())
        self.assertEqual(reference["content_sha256"], whole_file)
        self.assertNotEqual(reference["excerpt_sha256"], whole_file)
        hand_written = reference | {"excerpt_sha256": whole_file}
        self.assertEqual(verify(hand_written, self.repo)["status"], "stale")

    def test_the_last_line_can_be_cited_but_the_one_after_it_cannot(self) -> None:
        self.cited_file()
        self.assertEqual(
            citation(self.repo, "docs/adr/boundaries.md", 4, 4)["lines"]["end"], 4
        )
        with self.assertRaisesRegex(ValueError, "has 4 lines"):
            citation(self.repo, "docs/adr/boundaries.md", 4, 5)

    def test_a_citation_starting_before_the_first_line_is_refused(self) -> None:
        self.cited_file()
        self.assertEqual(
            citation(self.repo, "docs/adr/boundaries.md", 1, 1)["lines"]["start"], 1
        )
        with self.assertRaisesRegex(ValueError, "1 <= start <= end"):
            citation(self.repo, "docs/adr/boundaries.md", 0, 1)

    def test_a_citation_ending_before_it_starts_is_refused(self) -> None:
        self.cited_file()
        with self.assertRaisesRegex(ValueError, "1 <= start <= end"):
            citation(self.repo, "docs/adr/boundaries.md", 3, 2)

    def test_a_citation_reaching_outside_the_repository_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "escapes the repository"):
            citation(self.repo, "../outside.md", 1, 1)

    def test_citing_a_file_that_is_not_there_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "no such file to cite"):
            citation(self.repo, "docs/adr/missing.md", 1, 1)

    def test_relocating_a_directory_recites_evidence_where_the_excerpt_now_sits(self) -> None:
        self.evidence_moved("one\ntwo\nthree\n", "lib/a.py", "zero\none\ntwo\nthree\n")
        moved = relocate_evidence(self.repo / "memory", self.repo, "src", "lib")
        [update] = moved["registry_updates"]
        [reference] = update["record"]["evidence"]
        self.assertEqual(
            ("rules", "reviewed-rule", "lib/a.py", {"start": 3, "end": 3}, "current"),
            (update["asset"], update["record"]["id"], reference["path"], reference["lines"],
             verify(reference, self.repo)["status"]),
        )
        self.assertEqual((False, False), ("review" in update["record"], "verified" in update["record"]))
        self.assertEqual(["PRO-1"], moved["supersedes"])
        self.assertEqual(["candidate-rule"], [candidate["record"]["id"] for candidate in moved["candidates"]])
        self.assertEqual([], moved["unmatched"])

    def test_relocating_a_directory_moves_the_implementation_path_of_the_contexts_it_holds(self) -> None:
        self.evidence_moved("one\ntwo\nthree\n", "lib/a.py", "zero\none\ntwo\nthree\n")
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "implementation_path": "src",
             "status": "reviewed", "verified": VERIFIED},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices.", "implementation_path": "src/billing"},
            {"id": "shipping", "name": "Shipping", "responsibility": "Ship.", "implementation_path": "srcs/ship"},
        ])
        applied_by(self.repo / "memory", "PRO-2", "contexts/orders")
        moved = relocate_evidence(self.repo / "memory", self.repo, "src", "lib")
        self.assertEqual(
            ([("contexts", "orders", "lib"), ("rules", "reviewed-rule", None)], ["PRO-1", "PRO-2"],
             [("contexts", "billing", "lib/billing"), ("rules", "candidate-rule", None)]),
            ([(item["asset"], item["record"]["id"], item["record"].get("implementation_path"))
              for item in moved["registry_updates"]],
             moved["supersedes"],
             [(item["asset"], item["record"]["id"], item["record"].get("implementation_path"))
              for item in moved["candidates"]]),
        )

    def test_relocating_by_repository_path_moves_the_context_whose_implementation_path_ends_it(self) -> None:
        self.evidence_moved("one\ntwo\n", "lib/a.py", "one\ntwo\n")
        self.seed("contexts.json", [
            {"id": "batch", "name": "Batch", "responsibility": "Run batches.", "implementation_path": "trial/batch"},
            {"id": "trial", "name": "Trial", "responsibility": "Try.", "implementation_path": "internal/trial"},
            {"id": "other", "name": "Other", "responsibility": "Else.", "implementation_path": "batch"},
        ])
        moved = relocate_evidence(
            self.repo / "memory", self.repo, "apps/platform/internal/trial/batch", "apps/platform/internal/trial/batchy"
        )
        self.assertEqual(
            ([], [("contexts", "batch", "trial/batchy"), ("contexts", "other", "batchy")]),
            (moved["registry_updates"],
             [(item["asset"], item["record"]["id"], item["record"]["implementation_path"])
              for item in moved["candidates"]]),
        )

    def test_relocating_counts_the_citations_under_from_that_stayed_in_their_own_file(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        moved = relocate_evidence(self.repo / "memory", self.repo, "src", "lib")
        self.assertEqual(
            (1, [], [], []),
            (moved["citations_in_place"], moved["registry_updates"], moved["candidates"], moved["unmatched"]),
        )

    def test_relocating_a_single_file_recites_its_evidence(self) -> None:
        self.evidence_moved("one\ntwo\n", "lib/b.py", "two\n")
        moved = relocate_evidence(self.repo / "memory", self.repo, "src/a.py", "lib/b.py")
        [update] = moved["registry_updates"]
        self.assertEqual(
            {"path": "lib/b.py", "lines": {"start": 1, "end": 1}},
            {key: update["record"]["evidence"][0][key] for key in ("path", "lines")},
        )

    def test_relocating_leaves_a_citation_unmatched_when_its_excerpt_is_absent_or_repeated(self) -> None:
        for text, found in (("one\nother\n", 0), ("two\ntwo\n", 2)):
            with self.subTest(found=found):
                self.tearDown()
                self.setUp()
                self.evidence_moved("one\ntwo\n", "lib/a.py", text)
                moved = relocate_evidence(self.repo / "memory", self.repo, "src", "lib")
                self.assertEqual(([], []), (moved["registry_updates"], moved["candidates"]))
                self.assertEqual(
                    [("candidate-rule", found), ("reviewed-rule", found)],
                    [(problem["id"], problem["found"]) for problem in moved["unmatched"]],
                )

    def test_splitting_a_file_moves_only_the_citations_whose_lines_left_it(self) -> None:
        for kept_text, kept_status in (("keep\n", "current"), ("header\nkeep\n", "shifted")):
            with self.subTest(kept_status):
                self.tearDown()
                self.setUp()
                (self.repo / "src").mkdir()
                (self.repo / "src" / "a.py").write_text("keep\nmove\n", encoding="utf-8")
                self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
                self.seed("rules.json", [
                    {"id": "kept", "contexts": ["orders"], "statement": "S.", "evidence": [citation(self.repo, "src/a.py", 1, 1)]},
                    {"id": "moved", "contexts": ["orders"], "statement": "S.", "evidence": [citation(self.repo, "src/a.py", 2, 2)]},
                ])
                (self.repo / "src" / "a.py").write_text(kept_text, encoding="utf-8")
                (self.repo / "src" / "b.py").write_text("move\n", encoding="utf-8")
                moved = relocate_evidence(self.repo / "memory", self.repo, "src/a.py", "src/b.py")
                self.assertEqual(
                    ([("moved", "src/b.py")], [], []),
                    ([(candidate["record"]["id"], candidate["record"]["evidence"][0]["path"]) for candidate in moved["candidates"]],
                     moved["registry_updates"], moved["unmatched"]),
                )

    def test_relocating_ignores_a_path_that_only_shares_the_prefix(self) -> None:
        self.evidence_moved("one\ntwo\n", "srcx/a.py", "one\ntwo\n")
        moved = relocate_evidence(self.repo / "memory", self.repo, "sr", "srcx")
        self.assertEqual(([], [], []), (moved["registry_updates"], moved["candidates"], moved["unmatched"]))

    def test_reciting_a_reviewed_record_prints_its_update_and_the_proposal_to_supersede(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        result = recite(self.repo / "memory", self.repo, Recitation("rules", "rule", "src/a.py", "3", None))
        [update] = result["registry_updates"]
        [reference] = update["record"]["evidence"]
        self.assertEqual(
            ({"start": 3, "end": 3}, "current", ["PRO-1"], [], False),
            (reference["lines"], verify(reference, self.repo)["status"], result["supersedes"],
             result["candidates"], "review" in update["record"]),
        )

    def test_reciting_a_candidate_prints_it_as_a_candidate(self) -> None:
        self.cited_rule("candidate", (2, 2))
        result = recite(self.repo / "memory", self.repo, Recitation("rules", "rule", "src/a.py", "3-4", None))
        self.assertEqual(([], []), (result["registry_updates"], result["supersedes"]))
        self.assertEqual({"start": 3, "end": 4}, result["candidates"][0]["record"]["evidence"][0]["lines"])

    def test_reciting_one_of_several_citations_of_a_file_needs_replace(self) -> None:
        self.cited_rule("reviewed", (1, 1), (2, 3))
        with self.assertRaisesRegex(ValueError, "has 2 citations of src/a.py.*It cites: src/a.py:1-1, src/a.py:2-3"):
            recite(self.repo / "memory", self.repo, Recitation("rules", "rule", "src/a.py", "3-4", None))
        result = recite(self.repo / "memory", self.repo, Recitation("rules", "rule", "src/a.py", "3-4", "src/a.py:2-3"))
        self.assertEqual(
            [{"start": 1, "end": 1}, {"start": 3, "end": 4}],
            [reference["lines"] for reference in result["registry_updates"][0]["record"]["evidence"]],
        )

    def shifted_and_stale_in_one_record(self) -> dict:
        (self.repo / "fact.md").write_text("fact\na\nold\n", encoding="utf-8")
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.seed("rules.json", [
            {"id": "both", "contexts": ["orders"], "statement": "S.", "status": "reviewed",
             "evidence": [citation(self.repo, "fact.md", 1, 1), citation(self.repo, "fact.md", 3, 3)],
             "verified": VERIFIED},
            {"id": "shifted", "contexts": ["orders"], "statement": "S.", "status": "reviewed",
             "evidence": [citation(self.repo, "fact.md", 2, 2)], "verified": VERIFIED},
        ])
        applied_by(self.repo / "memory", "PRO-1", "rules/both")
        applied_by(self.repo / "memory", "PRO-2", "rules/shifted")
        (self.repo / "fact.md").write_text("x\nfact\na\nnew\n", encoding="utf-8")
        return relocate_evidence(self.repo / "memory", self.repo, "fact.md", "fact.md")

    def test_reciting_onto_a_relocation_keeps_its_re_citations_in_one_update(self) -> None:
        moved = self.shifted_and_stale_in_one_record()
        self.assertEqual([("both", {"start": 3, "end": 3})], [(p["id"], p["lines"]) for p in moved["unmatched"]])
        result = recite(self.repo / "memory", self.repo,
                        Recitation("rules", "both", "fact.md", "4", "fact.md:3-3", moved))
        self.assertEqual(
            {"both": [{"start": 2, "end": 2}, {"start": 4, "end": 4}], "shifted": [{"start": 3, "end": 3}]},
            {update["record"]["id"]: [reference["lines"] for reference in update["record"]["evidence"]]
             for update in result["registry_updates"]},
        )
        self.assertEqual((2, ["PRO-1", "PRO-2"], []),
                         (len(result["registry_updates"]), result["supersedes"], result["unmatched"]))

    def test_reciting_onto_output_that_lacks_the_record_starts_from_the_registry(self) -> None:
        moved = self.shifted_and_stale_in_one_record()
        moved["registry_updates"] = [u for u in moved["registry_updates"] if u["record"]["id"] != "both"]
        result = recite(self.repo / "memory", self.repo,
                        Recitation("rules", "both", "fact.md", "4", "fact.md:3-3", moved))
        both = next(u["record"] for u in result["registry_updates"] if u["record"]["id"] == "both")
        self.assertEqual([{"start": 1, "end": 1}, {"start": 4, "end": 4}], [r["lines"] for r in both["evidence"]])
        self.assertEqual(["both", "shifted"], sorted(u["record"]["id"] for u in result["registry_updates"]))

    def test_reciting_onto_output_leaves_a_same_id_record_of_another_asset_alone(self) -> None:
        moved = self.shifted_and_stale_in_one_record()
        namesake = {"operation": "upsert", "asset": "contexts", "record": {"id": "both", "evidence": []}}
        moved["registry_updates"].insert(0, namesake)
        result = recite(self.repo / "memory", self.repo,
                        Recitation("rules", "both", "fact.md", "4", "fact.md:3-3", moved))
        self.assertEqual(namesake, result["registry_updates"][0])
        self.assertEqual(["contexts", "rules", "rules"], [u["asset"] for u in result["registry_updates"]])

    def test_reciting_refuses_an_unknown_record_and_a_malformed_range(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        for record_id, lines, message in (("ghost", "3", "no rules record ghost"), ("rule", "3-x", "lines must be")):
            with self.subTest(message=message), self.assertRaisesRegex(ValueError, message):
                recite(self.repo / "memory", self.repo, Recitation("rules", record_id, "src/a.py", lines, None))

    def test_cited_lines_pushed_down_by_an_insertion_are_shifted_to_where_they_now_sit(self) -> None:
        result = self.citation_after_edit("a\nfact\n", 2, "new\na\nfact\n")
        self.assertEqual(("shifted", {"start": 3, "end": 3}), (result["status"], result["lines"]))
        spanning = self.citation_after_edit("a\nfact\n", 1, "new\na\nfact\n", last=2)
        self.assertEqual(("shifted", {"start": 2, "end": 3}), (spanning["status"], spanning["lines"]))
        self.assertIn("relocate-evidence --from fact.md --to fact.md", result["reason"])

    def test_cited_lines_that_now_appear_twice_or_not_at_all_are_stale(self) -> None:
        for after in ("fact\nnew\nfact\n", "a\nchanged\n"):
            with self.subTest(after=after):
                self.assertEqual("stale", self.citation_after_edit("a\nfact\n", 2, after)["status"])

    def test_relocating_a_file_onto_itself_recites_only_its_shifted_citations(self) -> None:
        self.seed_two_cited_rules()
        moved = relocate_evidence(self.repo / "memory", self.repo, "fact.md", "fact.md")
        self.assertEqual(
            [("moved", {"start": 3, "end": 3})],
            [(update["record"]["id"], update["record"]["evidence"][0]["lines"]) for update in moved["registry_updates"]],
        )
        self.assertEqual(["PRO-1"], moved["supersedes"])

    def test_verify_reports_the_cited_lines_beside_the_lines_it_found(self) -> None:
        result = self.citation_after_edit("a\nfact\n", 2, "new\na\nfact\n")
        self.assertEqual(
            ({"start": 2, "end": 2}, {"start": 3, "end": 3}), (result["cited_lines"], result["lines"])
        )

    def test_a_repeated_excerpt_is_stale_and_says_so(self) -> None:
        result = self.citation_after_edit("a\nfact\n", 2, "fact\nnew\nfact\n")
        self.assertEqual(
            "the cited excerpt is repeated in fact.md; check that the fact still holds, then recite it. "
            "Widen the citation to the enclosing block, so its lines occur once in the file.",
            result["reason"],
        )

    def test_an_unmatched_relocation_says_why(self) -> None:
        for text, reason in (
            ("one\nother\n", "the cited excerpt occurs 0 times in lib/a.py; its content changed, so check the fact and recite it"),
            ("two\ntwo\n", "the cited excerpt occurs 2 times in lib/a.py; recite it with the lines that hold the fact. "
             "Widen the citation to the enclosing block, so its lines occur once in the file."),
        ):
            with self.subTest(reason=reason):
                self.tearDown()
                self.setUp()
                self.evidence_moved("one\ntwo\n", "lib/a.py", text)
                moved = relocate_evidence(self.repo / "memory", self.repo, "src", "lib")
                self.assertEqual({reason}, {problem["reason"] for problem in moved["unmatched"]})

    def test_relocating_a_directory_onto_itself_recites_every_shifted_citation_under_it(self) -> None:
        self.seed_two_cited_rules()
        (self.repo / "other.md").write_text("new\nother\n", encoding="utf-8")
        moved = relocate_evidence(self.repo / "memory", self.repo, ".", ".")
        self.assertEqual(["PRO-1", "PRO-2"], moved["supersedes"])
        self.assertEqual(
            {"moved": [{"start": 3, "end": 3}], "kept": [{"start": 1, "end": 1}, {"start": 2, "end": 2}]},
            {update["record"]["id"]: [reference["lines"] for reference in update["record"]["evidence"]]
             for update in moved["registry_updates"]},
        )


if __name__ == "__main__":
    unittest.main()
