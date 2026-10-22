from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.registry import coverage, verify_evidence
from domain_registry.search import _is_common, every_resolved_term, ranked_lookup
from test_support import RegistryTestCase, lookup, word_stems


class RegistryLookupTest(RegistryTestCase):
    def test_coverage_names_a_reviewed_record_with_no_evidence(self) -> None:
        self.select_docs_as_sources()
        self.reviewed_rule([])
        self.assertEqual(
            [{"asset": "rules", "id": "order-total", "reason": "no evidence"}],
            coverage(self.repo / "memory")["evidence_gaps"],
        )

    def test_coverage_says_nothing_about_a_candidate_without_evidence(self) -> None:
        self.select_docs_as_sources()
        self.reviewed_rule([])
        self.seed(
            "rules.json",
            [
                {
                    "id": "order-total",
                    "contexts": ["orders"],
                    "statement": "An order total is non-negative.",
                    "evidence": [],
                    "status": "candidate",
                }
            ],
        )
        self.assertEqual([], coverage(self.repo / "memory")["evidence_gaps"])

    def test_a_requirement_sentence_resolves_the_terms_it_names(self) -> None:
        self.defined_term()
        matches = every_resolved_term(
            self.repo / "memory", "Who starts the Evaluation once a Run succeeds?", None
        )
        self.assertEqual([term["id"] for term in matches], ["run"])

    def test_a_sentence_naming_no_registered_term_still_resolves_to_nothing(
        self,
    ) -> None:
        self.defined_term()
        self.assertEqual(
            every_resolved_term(
                self.repo / "memory", "Who signs off on the quarterly budget?", None
            ),
            [],
        )

    def test_a_word_from_the_definition_still_resolves_the_term(self) -> None:
        self.defined_term()
        matches = every_resolved_term(self.repo / "memory", "test case", None)
        self.assertEqual([(term["id"], term["match_kind"]) for term in matches], [("run", "definition")])

    def test_a_name_in_the_query_is_a_name_match(self) -> None:
        self.defined_term()
        matches = every_resolved_term(self.repo / "memory", "a run", None)
        self.assertEqual([(term["id"], term["match_kind"]) for term in matches], [("run", "name")])

    def test_a_synonym_in_the_query_is_a_synonym_match(self) -> None:
        self.seed("vocabulary.json", [
            {"id": "run", "name": "Run", "definition": "One trial.", "contexts": ["trial"],
             "synonyms": ["Execution"]},
        ])
        self.allow_results(10)
        matches = every_resolved_term(self.repo / "memory", "retry the execution", None)
        self.assertEqual([(term["id"], term["match_kind"]) for term in matches], [("run", "synonym")])

    def test_name_matches_come_before_synonym_and_definition_matches(self) -> None:
        self.seed("vocabulary.json", [
            {"id": "attempt", "name": "Attempt", "definition": "One execution of a Run.", "contexts": ["trial"]},
            {"id": "job", "name": "Job", "definition": "Queued work.", "contexts": ["trial"],
             "synonyms": ["Run request"]},
            {"id": "run", "name": "Run", "definition": "One trial.", "contexts": ["trial"]},
        ])
        self.allow_results(10)
        matches = every_resolved_term(self.repo / "memory", "run", None)
        self.assertEqual(
            [(term["id"], term["match_kind"]) for term in matches],
            [("run", "name"), ("job", "synonym"), ("attempt", "definition")],
        )

    def test_resolve_terms_reads_the_definition_and_not_the_rest_of_the_record(
        self,
    ) -> None:
        self.two_contexts()
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A confirmed purchase.",
                    "contexts": ["orders"],
                },
                {
                    "id": "invoice",
                    "name": "Invoice",
                    "definition": "A demand for payment.",
                    "contexts": ["billing"],
                    "note": "A confirmed purchase becomes this.",
                },
            ],
        )
        self.allow_results(10)
        found = [
            term["id"]
            for term in every_resolved_term(self.repo / "memory", "confirmed purchase", None)
        ]
        self.assertEqual(found, ["order"])

    def test_resolve_terms_excludes_a_term_that_does_not_belong_to_the_named_context(
        self,
    ) -> None:
        self.two_contexts()
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A purchase.",
                    "contexts": ["orders"],
                }
            ],
        )
        self.assertEqual(
            every_resolved_term(self.repo / "memory", "order", "orders")[0]["id"], "order"
        )
        self.assertEqual(every_resolved_term(self.repo / "memory", "order", "billing"), [])

    def test_a_query_no_term_matches_lists_reviewed_records_that_mention_it(self) -> None:
        self.contexts_mentioning_suggestions()
        self.assertEqual(
            [{"match_kind": "mentioned", "asset": "contexts", "id": "eval", "status": "reviewed", "contexts": ["eval"],
              "summary": "Applies accepted Suggestions.", "fields": ["responsibility"]}],
            every_resolved_term(self.repo / "memory", "suggestion", None),
        )

    def test_a_mention_names_its_contexts_and_the_first_sentence_of_its_statement(self) -> None:
        self.allow_results(10)
        self.seed("rules.json", [{"id": "gone", "contexts": ["catalog", "search"], "status": "reviewed",
                                  "statement": "A withdrawn skill answers gone. Search drops it too."}])
        self.seed("interactions.json", [{"id": "notice", "producer_context": "catalog", "consumer_context": "billing",
                                         "description": "Withdrawn skills stop billing", "status": "reviewed"}])
        self.seed("dependency-policies.json", [{"id": "lookup", "from_context": "catalog", "to_context": "registry",
                                                "rationale": "Withdrawn skills are looked up here.", "status": "reviewed"}])
        self.seed("events.json", [{"id": "withdrawn", "owner_context": "catalog", "consumers": ["search", "billing"],
                                   "meaning": "A skill was withdrawn.", "status": "reviewed"}])
        self.assertEqual(
            [("rules", ["catalog", "search"], "A withdrawn skill answers gone."),
             ("events", ["catalog", "search", "billing"], "A skill was withdrawn."),
             ("interactions", ["catalog", "billing"], "Withdrawn skills stop billing"),
             ("dependency-policies", ["catalog", "registry"], "Withdrawn skills are looked up here.")],
            [(item["asset"], item["contexts"], item["summary"])
             for item in every_resolved_term(self.repo / "memory", "withdrawn", None)],
        )

    def test_one_word_finds_its_spaced_hyphenated_and_camel_case_forms(self) -> None:
        self.allow_results(10)
        self.seed("rules.json", [
            {"id": f"r{number}", "contexts": ["catalog"], "status": "reviewed", "statement": statement}
            for number, statement in enumerate([
                "A taken down skill answers gone.", "TakedownAt is set once.", "A take-down hides the skill.",
                "Takedowns are audited.", "A skill taken over keeps its history.", "Down time is reported.",
            ])
        ])
        self.assertEqual(
            ["r0", "r1", "r2", "r3"],
            [item["id"] for item in every_resolved_term(self.repo / "memory", "takedown", None)],
        )

    def test_lookup_finds_every_word_through_its_spaced_form(self) -> None:
        self.seed("rules.json", [
            {"id": "gone", "contexts": ["catalog"], "statement": "A taken down skill answers gone."},
            {"id": "kept", "contexts": ["catalog"], "statement": "A skill keeps its history."},
        ])
        self.assertEqual((["gone"], "all-words"), self.found("takedown answers"))

    def test_a_participle_in_en_and_a_plural_are_forms_of_their_word(self) -> None:
        self.assertEqual(
            (word_stems("take"), word_stems("token"), word_stems("open")),
            (word_stems("taken"), word_stems("tokens"), {"open"}),
        )

    def test_a_mention_search_ignores_the_status_field(self) -> None:
        self.contexts_mentioning_suggestions()
        self.assertEqual([], every_resolved_term(self.repo / "memory", "reviewed", None))

    def test_a_matching_term_hides_prose_mentions(self) -> None:
        self.contexts_mentioning_suggestions()
        self.seed("vocabulary.json", [{"id": "suggestion", "name": "Suggestion", "contexts": ["eval"]}])
        self.assertEqual(["name"], [match["match_kind"] for match in every_resolved_term(self.repo / "memory", "suggestion", None)])

    def test_with_mentions_a_matching_term_comes_first_and_mentions_follow(self) -> None:
        self.contexts_mentioning_suggestions()
        self.seed("vocabulary.json", [{"id": "suggestion", "name": "Suggestion", "contexts": ["eval"],
                                       "definition": "A suggestion to change a version.", "status": "reviewed"}])
        self.assertEqual(
            [("vocabulary", "suggestion", "name"), ("contexts", "eval", "mentioned")],
            [(match.get("asset", "vocabulary"), match["id"], match["match_kind"])
             for match in every_resolved_term(self.repo / "memory", "suggestion", None, with_mentions=True)],
        )

    def test_a_record_id_is_searched_word_by_word(self) -> None:
        self.allow_results(10)
        self.seed("interactions.json", [{"id": "eval-workspace-purge", "status": "reviewed"}])
        self.assertEqual(
            [("interactions", "eval-workspace-purge", ["id"])],
            [(match["asset"], match["id"], match["fields"]) for match in every_resolved_term(self.repo / "memory", "purge", None)],
        )

    def test_a_list_of_text_is_searched_and_a_list_of_record_ids_is_not(self) -> None:
        self.allow_results(10)
        self.seed("vocabulary.json", [{"id": "order", "name": "Order", "examples": ["a refund request"], "status": "reviewed"}])
        self.seed("rules.json", [{"id": "gate", "contexts": ["refund"], "statement": "x", "status": "reviewed"}])
        self.assertEqual(
            [("vocabulary", "order", ["examples"])],
            [(match["asset"], match["id"], match["fields"]) for match in every_resolved_term(self.repo / "memory", "refund", None)],
        )

    def test_verify_evidence_reports_each_requested_path_and_whether_it_exists(self) -> None:
        self.seed_two_cited_rules()
        report = verify_evidence(self.repo / "memory", self.repo, ["./other.md", "fact.md", "typo.md"])
        self.assertEqual(
            {"other.md": {"citations": 1, "exists": True}, "fact.md": {"citations": 2, "exists": True},
             "typo.md": {"citations": 0, "exists": False}},
            report["paths"],
        )
        self.assertNotIn("paths", verify_evidence(self.repo / "memory", self.repo))

    def test_mentions_match_whole_words_and_list_defining_fields_first(self) -> None:
        self.allow_results(10)
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Owns checkpoints.", "status": "reviewed"}])
        self.seed("capabilities.json", [{"id": "audit", "rationale": "Runs checks.", "status": "reviewed"}])
        self.seed("value-objects.json", [{"id": "gauge", "description": "A precheck value.", "status": "reviewed"}])
        self.seed("rules.json", [{"id": "gate", "contexts": ["orders"], "statement": "A check gates release.", "status": "reviewed"}])
        self.assertEqual(
            [("rules", "gate", ["statement"]), ("capabilities", "audit", ["rationale"])],
            [(match["asset"], match["id"], match["fields"]) for match in every_resolved_term(self.repo / "memory", "check", None)],
        )

    def runs_rules(self) -> None:
        self.allow_results(10)
        self.seed("rules.json", [
            {"id": "one-trial", "contexts": ["run"], "statement": "A workspace runs one trial at a time; concurrency is refused.",
             "status": "reviewed"},
            {"id": "licensed", "contexts": ["packaging"], "statement": "A package carries its license.", "status": "reviewed"},
            {"id": "taken-down", "contexts": ["packaging"], "statement": "A skill taken down is not served.", "status": "reviewed"},
        ])

    def found(self, query: str) -> tuple[list[str], str]:
        entries, how = ranked_lookup(self.repo / "memory", "rules", query)
        return [entry["id"] for entry in entries], how

    def test_a_query_whose_words_appear_apart_and_in_another_form_finds_the_record(self) -> None:
        self.runs_rules()
        for query, expected in (("concurrent runs", ["one-trial"]), ("licensing", ["licensed"])):
            with self.subTest(query):
                self.assertEqual((expected, "all-words"), self.found(query))

    def test_a_query_no_record_holds_whole_lists_the_records_holding_most_of_its_words(self) -> None:
        self.runs_rules()
        self.assertEqual((["taken-down", "licensed"], "some-words"), self.found("package taken down download"))

    def test_a_query_word_like_after_does_not_count_as_shared(self) -> None:
        self.allow_results(10)
        self.seed("rules.json", [
            {"id": "taken-down", "contexts": ["packaging"], "statement": "A skill taken down is not served.", "status": "reviewed"},
            {"id": "halt", "contexts": ["run"], "statement": "Dispatch halts after an incident.", "status": "reviewed"},
        ])
        self.assertEqual((["taken-down"], "some-words"), self.found("download after taken"))

    def test_a_record_mentioned_in_several_fields_is_listed_once_naming_each_field(self) -> None:
        self.allow_results(10)
        self.seed("rules.json", [{"id": "purge-runs-every-step", "contexts": ["identity"],
                                  "statement": "A purge runs every step.", "status": "reviewed"}])
        self.assertEqual(
            [("purge-runs-every-step", ["id", "statement"])],
            [(match["id"], match["fields"]) for match in every_resolved_term(self.repo / "memory", "purge", None)],
        )

    def test_a_record_mentioned_in_one_defining_field_ranks_before_a_rule_mentioned_only_elsewhere(self) -> None:
        self.allow_results(10)
        self.seed("capabilities.json", [{"id": "purge", "rationale": "Purge on request.", "status": "reviewed"}])
        self.seed("rules.json", [{"id": "gate", "contexts": ["identity"], "statement": "x", "notes": "purge",
                                  "status": "reviewed"}])
        self.assertEqual(
            ["purge", "gate"],
            [match["id"] for match in every_resolved_term(self.repo / "memory", "purge", None)],
        )

    def test_a_query_sharing_only_a_word_many_records_hold_lists_nothing_and_says_so(self) -> None:
        self.allow_results(10)
        self.seed("rules.json", [
            {"id": f"rule-{number}", "contexts": ["run"], "statement": text, "status": "reviewed"}
            for number, text in enumerate(["A skill is served.", "A skill is packaged.", "A skill is rated.", "A run ends."])
        ])
        self.assertEqual(([], "common-words"), self.found("unpublish a skill"))

    def test_a_word_is_common_from_three_holders_above_a_quarter_of_the_records(self) -> None:
        self.assertEqual(
            [True, False, False, True],
            [_is_common(holders, records) for holders, records in ((3, 11), (3, 12), (2, 7), (3, 3))],
        )

    def test_a_query_matching_a_phrase_lists_only_records_holding_the_phrase(self) -> None:
        self.runs_rules()
        self.assertEqual((["taken-down"], "phrase"), self.found("taken down"))

    def test_a_query_none_of_whose_words_appear_says_so(self) -> None:
        self.runs_rules()
        self.assertEqual(([], "none"), self.found("invoices"))

    def test_a_query_of_stop_words_only_finds_nothing_and_says_so(self) -> None:
        self.runs_rules()
        self.assertEqual(([], "none"), self.found("who does what"))

    def test_the_result_limit_holds_for_records_holding_every_word_and_some_words(self) -> None:
        self.allow_results(1)
        self.seed("rules.json", [
            {"id": "a-daily", "contexts": ["shipping"], "statement": "Orders ship daily.", "status": "reviewed"},
            {"id": "b-weekly", "contexts": ["shipping"], "statement": "Orders ship weekly.", "status": "reviewed"},
        ])
        for query, how in (("shipped order", "all-words"), ("shipped order invoices", "some-words")):
            with self.subTest(query):
                self.assertEqual((["a-daily"], how), self.found(query))

    def test_word_stems_join_forms_of_a_word_but_not_a_longer_word(self) -> None:
        self.assertEqual(
            (word_stems("concurrency"), word_stems("licensing"), word_stems("deleted"), word_stems("runs"), {"precheck"}),
            (word_stems("concurrent"), word_stems("license"), word_stems("deletion"), word_stems("run"), word_stems("precheck")),
        )

    def test_word_stems_join_a_noun_in_ure_with_its_verb_forms(self) -> None:
        self.assertEqual(
            [word_stems("exposed"), word_stems("failed"), word_stems("measure"), word_stems("measures")],
            [word_stems("exposure"), word_stems("failure"), word_stems("measured"), word_stems("measured")],
        )

    def test_question_words_are_not_searched(self) -> None:
        self.assertEqual({"see"}, word_stems("who can see what it does, how and when that is"))

    def test_word_stems_keep_a_short_word_whole(self) -> None:
        self.assertEqual({"ring", "bed"}, word_stems("ring bed"))

    def test_a_multi_word_query_finds_a_rule_mentioning_every_word(self) -> None:
        self.runs_rules()
        self.seed("interactions.json", [{"id": "run-retry", "status": "reviewed"}])
        self.assertEqual(
            [("rules", "one-trial", ["statement"])],
            [(match["asset"], match["id"], match["fields"]) for match in every_resolved_term(self.repo / "memory", "concurrent runs", None)],
        )

    def test_a_query_matches_regardless_of_letter_case(self) -> None:
        self.two_contexts()
        self.allow_results(10)
        found = lookup(self.repo / "memory", "contexts", "OWN INVOICES")
        self.assertEqual(["billing"], [entry["id"] for entry in found])

    def test_looking_up_an_unknown_asset_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "unknown asset: nonsense"):
            lookup(self.repo / "memory", "nonsense", "")

    def test_a_result_limit_equal_to_the_match_count_returns_every_match(self) -> None:
        self.two_contexts()
        self.allow_results(2)
        found = lookup(self.repo / "memory", "contexts", "")
        self.assertEqual(["billing", "orders"], [entry["id"] for entry in found])


if __name__ == "__main__":
    unittest.main()
