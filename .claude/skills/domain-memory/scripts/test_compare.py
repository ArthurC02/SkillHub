from __future__ import annotations

import json
import unittest

from domain_registry.compare import compare_registries
from test_support import RegistryTestCase

ORDERS = {"id": "orders", "name": "Orders", "implementation_path": "src/orders"}
BILLING = {"id": "billing", "name": "Billing", "implementation_path": "src/billing"}


def registry(**assets: list[dict]) -> dict:
    keys = {"vocabulary": "terms", "value-objects": "value_objects", "dependency-policies": "dependencies"}
    return {asset: {keys.get(asset, asset): records} for asset, records in assets.items()}


def asset_result(gold: dict, candidate: dict, asset: str) -> dict:
    return compare_registries(gold, candidate)["assets"][asset]


def cited(path: str) -> list[dict]:
    return [{"path": path, "lines": {"start": 1, "end": 1}}]


class ContextMatchingTest(unittest.TestCase):
    def matched(self, gold: list[dict], candidate: list[dict]) -> list[tuple[str, str, str]]:
        result = asset_result(registry(contexts=gold), registry(contexts=candidate), "contexts")
        return [(pair["candidate"], pair["gold"], pair["by"]) for pair in result["matched"]]

    def test_a_context_at_the_same_code_path_matches_whatever_it_is_called(self) -> None:
        self.assertEqual([("sales", "orders", "path")], self.matched([ORDERS], [{"id": "sales", "implementation_path": "src/orders/"}]))

    def test_a_context_inside_or_around_the_gold_path_matches_as_nested(self) -> None:
        for path in ("src/orders/core", "src"):
            with self.subTest(path):
                self.assertEqual([("x", "orders", "nested-path")], self.matched([ORDERS], [{"id": "x", "implementation_path": path}]))

    def test_a_path_written_from_another_base_matches_by_its_suffix(self) -> None:
        for path in ("app/src/orders", "orders"):
            with self.subTest(path):
                self.assertEqual([("x", "orders", "path-suffix")], self.matched([ORDERS], [{"id": "x", "implementation_path": path}]))

    def test_a_path_inside_another_base_matches_as_nested(self) -> None:
        self.assertEqual([("x", "orders", "nested-path")], self.matched([ORDERS], [{"id": "x", "implementation_path": "app/src/orders/core"}]))

    def test_a_sibling_path_that_only_shares_a_prefix_of_the_name_does_not_match(self) -> None:
        self.assertEqual([], self.matched([ORDERS], [{"id": "x", "implementation_path": "src/orders-legacy"}]))

    def test_a_context_without_a_path_matches_by_name_ignoring_case_and_punctuation(self) -> None:
        self.assertEqual([("order-ctx", "orders", "name")], self.matched([ORDERS], [{"id": "order-ctx", "name": "ORDERS"}]))

    def test_contexts_that_both_lack_a_name_and_path_do_not_match(self) -> None:
        self.assertEqual([], self.matched([{"id": "orders"}], [{"id": "sales"}]))

    def test_a_path_match_wins_over_a_name_match_for_the_same_candidate(self) -> None:
        candidate = {"id": "billing", "name": "Billing", "implementation_path": "src/orders"}
        self.assertEqual([("billing", "orders", "path")], self.matched([ORDERS, BILLING], [candidate]))

    def test_one_gold_context_is_matched_once_and_the_second_claimant_is_extra(self) -> None:
        candidates = [{"id": "a", "implementation_path": "src/orders"}, {"id": "b", "implementation_path": "src/orders"}]
        result = asset_result(registry(contexts=[ORDERS]), registry(contexts=candidates), "contexts")
        self.assertEqual((["a"], ["b"]), ([pair["candidate"] for pair in result["matched"]], result["extra"]))


class RelationMatchingTest(unittest.TestCase):
    candidate_contexts = [{"id": "sales", "implementation_path": "src/orders"}, {"id": "invoicing", "implementation_path": "src/billing"}]

    def interactions(self, producer: str, consumer: str) -> dict:
        gold = registry(contexts=[ORDERS, BILLING], interactions=[{"id": "g", "producer_context": "orders", "consumer_context": "billing"}])
        candidate = registry(contexts=self.candidate_contexts,
                             interactions=[{"id": "c", "producer_context": producer, "consumer_context": consumer}])
        return asset_result(gold, candidate, "interactions")

    def test_an_interaction_matches_when_both_ends_map_to_the_gold_contexts(self) -> None:
        self.assertEqual([{"candidate": "c", "gold": "g", "by": "path"}], self.interactions("sales", "invoicing")["matched"])

    def test_an_interaction_in_the_opposite_direction_does_not_match(self) -> None:
        self.assertEqual((["g"], ["c"]), (self.interactions("invoicing", "sales")["missed"], self.interactions("invoicing", "sales")["extra"]))

    def test_an_interaction_naming_an_unmatched_context_does_not_match(self) -> None:
        self.assertEqual([], self.interactions("sales", "shipping")["matched"])

    def test_a_dependency_policy_matches_on_its_from_and_to_contexts(self) -> None:
        gold = registry(contexts=[ORDERS, BILLING], **{"dependency-policies": [{"id": "g", "from_context": "orders", "to_context": "billing"}]})
        candidate = registry(contexts=self.candidate_contexts,
                             **{"dependency-policies": [{"id": "c", "from_context": "sales", "to_context": "invoicing"}]})
        self.assertEqual(["g"], [pair["gold"] for pair in asset_result(gold, candidate, "dependency-policies")["matched"]])


class RecordMatchingTest(unittest.TestCase):
    def rules(self, candidate_rule: dict) -> dict:
        gold = registry(contexts=[ORDERS, BILLING],
                        rules=[{"id": "order-total-is-positive", "contexts": ["orders"], "evidence": cited("src/orders/total.py")}])
        candidate = registry(contexts=[{"id": "sales", "implementation_path": "src/orders"}, BILLING], rules=[candidate_rule])
        return asset_result(gold, candidate, "rules")

    def test_a_rule_with_the_same_id_up_to_case_and_punctuation_matches_by_name(self) -> None:
        self.assertEqual("name", self.rules({"id": "Order_Total_Is_Positive"})["matched"][0]["by"])

    def test_a_rule_in_the_matching_context_citing_the_same_file_matches(self) -> None:
        rule = {"id": "totals", "contexts": ["sales"], "evidence": cited("src/orders/total.py")}
        self.assertEqual("context-and-evidence", self.rules(rule)["matched"][0]["by"])

    def test_a_legacy_path_and_line_citation_counts_as_its_file(self) -> None:
        rule = {"id": "totals", "contexts": ["sales"], "evidence": ["src/orders/total.py:4"]}
        self.assertEqual(["order-total-is-positive"], [pair["gold"] for pair in self.rules(rule)["matched"]])

    def test_a_rule_whose_cited_lines_overlap_wins_over_one_citing_elsewhere_in_the_file(self) -> None:
        gold = registry(contexts=[ORDERS], rules=[
            {"id": "purge-runs-every-step", "contexts": ["orders"], "evidence": [
                {"path": "purge.go", "lines": {"start": 47, "end": 63}}, {"path": "purge.go", "lines": {"start": 78, "end": 99}}]},
        ])
        candidate = registry(contexts=[ORDERS], rules=[
            {"id": "audited-in-one-transaction", "contexts": ["orders"], "evidence": [{"path": "purge.go", "lines": {"start": 270, "end": 278}}]},
            {"id": "refuses-an-unwired-step", "contexts": ["orders"], "evidence": [{"path": "purge.go", "lines": {"start": 78, "end": 99}}]},
        ])
        result = asset_result(gold, candidate, "rules")
        self.assertEqual(([("refuses-an-unwired-step", "purge-runs-every-step")], ["audited-in-one-transaction"]),
                         ([(pair["candidate"], pair["gold"]) for pair in result["matched"]], result["extra"]))

    def test_a_rule_citing_the_same_file_from_another_context_does_not_match(self) -> None:
        rule = {"id": "totals", "contexts": ["billing"], "evidence": cited("src/orders/total.py")}
        self.assertEqual([], self.rules(rule)["matched"])

    def test_a_rule_in_the_same_context_citing_another_file_does_not_match(self) -> None:
        rule = {"id": "totals", "contexts": ["sales"], "evidence": cited("src/orders/other.py")}
        self.assertEqual([], self.rules(rule)["matched"])

    def test_a_term_named_by_a_gold_synonym_matches(self) -> None:
        gold = registry(vocabulary=[{"id": "order", "name": "Order", "synonyms": ["Purchase"]}])
        candidate = registry(vocabulary=[{"id": "t1", "name": "purchase"}])
        self.assertEqual(["order"], [pair["gold"] for pair in asset_result(gold, candidate, "vocabulary")["matched"]])

    def test_an_aggregate_with_the_same_root_matches(self) -> None:
        gold = registry(aggregates=[{"id": "order-aggregate", "root": "Order"}])
        candidate = registry(aggregates=[{"id": "a1", "root": "order"}])
        self.assertEqual(["order-aggregate"], [pair["gold"] for pair in asset_result(gold, candidate, "aggregates")["matched"]])


class PossiblePairTest(unittest.TestCase):
    def possible(self, wording: str, contexts: list[str]) -> list[dict]:
        gold = registry(contexts=[ORDERS, BILLING],
                        rules=[{"id": "run-terminal-status-is-final", "contexts": ["orders"], "statement": "A terminal run status is final."}])
        candidate = registry(contexts=[ORDERS, BILLING],
                             rules=[{"id": "closed", "contexts": contexts, "statement": wording}])
        return asset_result(gold, candidate, "rules")["possible_pairs"]

    def test_an_unpaired_rule_worded_alike_in_a_paired_context_is_offered(self) -> None:
        self.assertEqual([{"gold": "run-terminal-status-is-final", "candidate": "closed", "similarity": 0.5}],
                         self.possible("Once a run reaches a terminal status it stays final.", ["orders"]))

    def test_a_rule_worded_alike_in_another_context_is_not_offered(self) -> None:
        self.assertEqual([], self.possible("Once a run reaches a terminal status it stays final.", ["billing"]))

    def test_a_rule_worded_differently_is_not_offered(self) -> None:
        self.assertEqual([], self.possible("Invoices are numbered in sequence.", ["orders"]))


class ScopeTest(unittest.TestCase):
    def test_a_context_scope_keeps_only_that_contexts_gold_records(self) -> None:
        gold = registry(contexts=[ORDERS, BILLING], rules=[
            {"id": "order-rule", "contexts": ["orders"]}, {"id": "bill-rule", "contexts": ["billing"]}, {"id": "loose"}])
        result = compare_registries(gold, registry(contexts=[ORDERS]), ["orders"])
        self.assertEqual((["orders"], ["order-rule"]),
                         ([pair["gold"] for pair in result["assets"]["contexts"]["matched"]], result["assets"]["rules"]["missed"]))


class ScoreTest(unittest.TestCase):
    def scores(self, gold: list[dict], candidate: list[dict]) -> tuple:
        result = asset_result(registry(contexts=gold), registry(contexts=candidate), "contexts")
        return result["precision"], result["recall"], result["f1"]

    def test_one_of_two_gold_contexts_found_with_one_extra_scores_a_half_each(self) -> None:
        self.assertEqual((0.5, 0.5, 0.5), self.scores([ORDERS, BILLING], [ORDERS, {"id": "x", "implementation_path": "lib"}]))

    def test_nothing_extracted_against_a_gold_set_scores_zero_and_leaves_precision_unknown(self) -> None:
        self.assertEqual((None, 0.0, 0.0), self.scores([ORDERS], []))

    def test_an_asset_empty_on_both_sides_has_no_score(self) -> None:
        self.assertEqual((None, None, None), self.scores([], []))

    def test_the_overall_score_counts_every_asset(self) -> None:
        gold = registry(contexts=[ORDERS], rules=[{"id": "r"}])
        candidate = registry(contexts=[ORDERS], rules=[{"id": "other"}])
        self.assertEqual({"precision": 0.5, "recall": 0.5, "f1": 0.5}, compare_registries(gold, candidate)["overall"])


class CompareCommandTest(RegistryTestCase):
    def test_the_command_scores_a_candidate_registry_against_the_gold_registry_at_a_commit(self) -> None:
        self.seed("contexts.json", [ORDERS])
        self.commit_all("gold")
        self.seed("contexts.json", [])
        candidate = self.repo / "candidate"
        self.run_cli("init", "--output", str(candidate))
        result = self.run_cli("compare-registry", "--gold-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                              "--gold-commit", "HEAD", "--candidate-root", str(candidate))
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertEqual((1, ["orders"]), (json.loads(result.stdout)["assets"]["contexts"]["gold"],
                                           json.loads(result.stdout)["assets"]["contexts"]["missed"]))

    def test_a_gold_commit_without_a_repository_root_is_refused(self) -> None:
        result = self.run_cli("compare-registry", "--gold-root", str(self.repo / "memory"), "--gold-commit", "HEAD",
                              "--candidate-root", str(self.repo / "memory"))
        self.assertEqual((1, "ERROR: compare-registry --gold-commit needs --repo-root"), (result.returncode, result.stdout.strip()))


if __name__ == "__main__":
    unittest.main()
