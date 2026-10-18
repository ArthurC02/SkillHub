from __future__ import annotations

import json
import unittest

from domain_registry.common import ASSET_KEYS
from domain_registry.evidence import citation
from domain_registry.harvest import harvest_candidates
from domain_registry.registry import asset_records
from domain_registry.store import read_document, write_document
from test_support import VERIFIED, RegistryTestCase, applied_by, init_registry

ORDERS = {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "implementation_path": "src/orders"}


class HarvestTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        (self.repo / "src" / "orders").mkdir(parents=True)
        (self.repo / "src" / "orders" / "total.py").write_text("total\nlimit\n", encoding="utf-8")
        self.seed("contexts.json", [ORDERS])
        self.seed("rules.json", [{"id": "kept", "contexts": ["orders"], "statement": "Totals are positive.",
                                  "status": "reviewed", "verified": VERIFIED,
                                  "evidence": [citation(self.repo, "src/orders/total.py", 1, 1)]}])
        applied_by(self.repo / "memory", "PRO-1", "rules/kept")
        self.scratch = self.repo / "scratch"
        init_registry(self.scratch)

    def extract(self, **assets: list[dict]) -> None:
        for asset, records in assets.items():
            name = f"{asset}.json"
            document = read_document(self.scratch, name)
            document[ASSET_KEYS[name]] = records
            write_document(self.scratch, name, document)

    def harvest(self, context_map: dict[str, str] | None = None) -> dict:
        return harvest_candidates(self.repo / "memory", self.repo, self.scratch, context_map or {})

    def test_records_of_a_context_paired_by_path_are_harvested_under_the_gold_context(self) -> None:
        self.extract(
            contexts=[{"id": "sales", "name": "Sales", "responsibility": "Sell.", "implementation_path": "src/orders"}],
            rules=[{"id": "limit", "contexts": ["sales"], "statement": "Orders have a limit.",
                    "evidence": [citation(self.repo, "src/orders/total.py", 2, 2)]}],
        )
        report = self.harvest()
        [rule] = [rule for rule in asset_records(self.repo / "memory", "rules.json") if rule["id"] == "limit"]
        self.assertEqual(
            ({"sales": "orders"}, ["rules/limit"], 1, ["orders"], "candidate", ["orders"]),
            (report["context_map"], report["harvested"], report["created"], rule["contexts"], rule["status"],
             [context["id"] for context in asset_records(self.repo / "memory", "contexts.json")]),
        )

    def test_a_single_context_field_is_renamed_too(self) -> None:
        self.extract(
            contexts=[{"id": "sales", "name": "Sales", "responsibility": "Sell.", "implementation_path": "src/orders"}],
            events=[{"id": "order-placed", "name": "Order placed", "owner_context": "sales"}],
        )
        self.harvest()
        self.assertEqual(["orders"], [event["owner_context"] for event in asset_records(self.repo / "memory", "events.json")])

    def test_a_context_no_gold_context_pairs_with_is_harvested_as_a_candidate(self) -> None:
        self.extract(contexts=[{"id": "billing", "name": "Billing", "responsibility": "Bill.", "implementation_path": "src/billing"}])
        report = self.harvest()
        self.assertEqual((["contexts/billing"], {}), (report["harvested"], report["context_map"]))

    def test_harvesting_again_updates_the_candidates_an_earlier_harvest_wrote(self) -> None:
        billing = {"id": "billing", "name": "Billing", "responsibility": "Bill.", "implementation_path": "src/billing"}
        self.extract(contexts=[billing])
        self.harvest()
        self.extract(contexts=[{**billing, "responsibility": "Bill customers."}])
        report = self.harvest()
        self.assertEqual(
            ((["contexts/billing"], 0, 1), "Bill customers."),
            ((report["harvested"], report["created"], report["updated"]),
             next(context["responsibility"] for context in asset_records(self.repo / "memory", "contexts.json")
                  if context["id"] == "billing")),
        )

    def test_a_given_pairing_renames_a_context_the_search_could_not_pair(self) -> None:
        self.extract(
            contexts=[{"id": "commerce", "name": "Commerce", "responsibility": "Trade."}],
            rules=[{"id": "limit", "contexts": ["commerce"], "statement": "Orders have a limit."}],
        )
        report = self.harvest({"commerce": "orders"})
        self.assertEqual((["rules/limit"], ["orders"]), (
            report["harvested"],
            next(rule["contexts"] for rule in asset_records(self.repo / "memory", "rules.json") if rule["id"] == "limit"),
        ))

    def test_a_record_that_is_reviewed_under_the_same_id_is_skipped_not_overwritten(self) -> None:
        self.extract(rules=[{"id": "kept", "contexts": ["orders"], "statement": "Rewritten.",
                             "evidence": [citation(self.repo, "src/orders/total.py", 1, 1)]}])
        report = self.harvest()
        self.assertEqual(
            ([], ["rules/kept"], "Totals are positive."),
            (report["harvested"], report["skipped_reviewed"],
             next(rule["statement"] for rule in asset_records(self.repo / "memory", "rules.json") if rule["id"] == "kept")),
        )

    def test_a_harvested_record_citing_more_than_a_gold_records_evidence_is_written_and_named(self) -> None:
        self.extract(rules=[{"id": "positive", "contexts": ["orders"], "statement": "Totals are positive.",
                             "evidence": [citation(self.repo, "src/orders/total.py", 1, 1),
                                          citation(self.repo, "src/orders/total.py", 2, 2)]}])
        report = self.harvest()
        self.assertEqual((["rules/positive"], ["rules/kept"]), (report["harvested"], report["possible_duplicates"]))

    def copy_of_the_reviewed_rule(self) -> None:
        self.extract(rules=[{"id": "positive", "contexts": ["orders"], "statement": "Totals are positive.",
                             "evidence": [citation(self.repo, "src/orders/total.py", 1, 1)]}])

    def test_a_record_citing_exactly_a_reviewed_records_evidence_is_skipped(self) -> None:
        self.copy_of_the_reviewed_rule()
        report = self.harvest()
        self.assertEqual(
            ([], [{"record": "rules/positive", "duplicates": "rules/kept"}], ["kept"]),
            (report["harvested"], report["skipped_duplicate"],
             [rule["id"] for rule in asset_records(self.repo / "memory", "rules.json")]),
        )

    def test_a_copy_of_a_reviewed_record_is_written_when_duplicates_are_kept(self) -> None:
        self.copy_of_the_reviewed_rule()
        report = harvest_candidates(self.repo / "memory", self.repo, self.scratch, {}, keep_duplicates=True)
        self.assertEqual((["rules/positive"], []), (report["harvested"], report["skipped_duplicate"]))

    def test_every_context_pair_is_listed_with_how_it_paired(self) -> None:
        self.extract(contexts=[ORDERS, {"id": "commerce", "name": "Commerce", "responsibility": "Trade."}])
        self.assertEqual(
            [{"candidate": "commerce", "gold": "orders", "by": "given"},
             {"candidate": "orders", "gold": "orders", "by": "path"}],
            self.harvest({"commerce": "orders"})["context_pairs"],
        )

    def test_a_context_sharing_an_id_at_another_path_is_named(self) -> None:
        for path, collisions in (("src/orders/", []), ("src/shipping", ["orders"])):
            with self.subTest(path):
                self.extract(contexts=[{"id": "orders", "name": "Shipping", "responsibility": "Ship.", "implementation_path": path}])
                self.assertEqual(collisions, self.harvest()["context_id_collisions"])

    def test_the_command_refuses_a_pairing_not_written_candidate_equals_gold(self) -> None:
        result = self.run_cli("harvest-candidates", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                              "--from-root", str(self.scratch), "--context-map", "commerce")
        self.assertEqual((1, "ERROR: --context-map takes CANDIDATE=GOLD, got commerce"), (result.returncode, result.stderr.strip()))

    def test_the_command_prints_what_it_harvested(self) -> None:
        self.extract(contexts=[{"id": "billing", "name": "Billing", "responsibility": "Bill."}])
        result = self.run_cli("harvest-candidates", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                              "--from-root", str(self.scratch))
        self.assertEqual((0, ["contexts/billing"]), (result.returncode, json.loads(result.stdout)["harvested"]))


if __name__ == "__main__":
    unittest.main()
