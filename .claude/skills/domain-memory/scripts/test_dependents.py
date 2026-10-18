from __future__ import annotations

import json
import unittest

from domain_registry.dependents import dependents
from test_support import RegistryTestCase


class DependentsTest(RegistryTestCase):
    def coupled_registry(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
        ])
        self.seed("aggregates.json", [{"id": "order", "context": "orders", "root": "Order"}])
        self.seed("rules.json", [
            {"id": "total", "contexts": ["orders"], "aggregate": "order", "statement": "A total is never negative."},
            {"id": "unbound", "contexts": ["orders"], "statement": "Another rule of the Context."},
        ])
        self.seed("contracts.json", [{"id": "orders-v1", "producer_context": "orders", "consumer_contexts": ["billing"]}])
        self.seed("events.json", [{
            "id": "order-placed", "owner_context": "orders", "aggregate": "order", "contract_id": "orders-v1",
            "consumers": ["billing"], "meaning": "An order was placed.",
        }])
        self.seed("interactions.json", [
            {"id": "orders-billing", "producer_context": "orders", "consumer_context": "billing", "contract_id": "orders-v1"},
        ])

    def test_every_relation_is_followed_in_both_directions_without_crossing_a_context(self) -> None:
        self.coupled_registry()
        reached = dependents(self.repo / "memory", "aggregates", "order")["reached"]
        self.assertEqual(
            [("contexts", "orders", 1, "aggregates/order.context"),
             ("events", "order-placed", 1, "events/order-placed.aggregate"),
             ("rules", "total", 1, "rules/total.aggregate"),
             ("contexts", "billing", 2, "events/order-placed.consumers"),
             ("contracts", "orders-v1", 2, "events/order-placed.contract_id"),
             ("interactions", "orders-billing", 3, "interactions/orders-billing.contract_id")],
            [(item["asset"], item["id"], item["depth"], item["through"]) for item in reached],
        )
        self.assertEqual("A total is never negative.", reached[2]["summary"])

    def test_an_absent_record_is_refused_by_name(self) -> None:
        self.coupled_registry()
        with self.assertRaises(ValueError) as refused:
            dependents(self.repo / "memory", "aggregates", "ghost")
        self.assertEqual("aggregates record not found: ghost", str(refused.exception))

    def test_the_command_prints_the_walk(self) -> None:
        self.coupled_registry()
        shown = self.run_cli("dependents", "--registry-root", str(self.repo / "memory"), "--asset", "rules", "--id", "total")
        self.assertEqual(
            (0, ["aggregates/order", "contexts/orders", "events/order-placed", "contexts/billing", "contracts/orders-v1",
                 "interactions/orders-billing"]),
            (shown.returncode, [f"{item['asset']}/{item['id']}" for item in json.loads(shown.stdout)["reached"]]),
        )

    def test_the_command_names_the_ids_that_resemble_an_unknown_one(self) -> None:
        self.coupled_registry()
        shown = self.run_cli("dependents", "--registry-root", str(self.repo / "memory"), "--asset", "rules", "--id", "tot")
        self.assertEqual(
            (1, "ERROR: rules record not found: tot; did you mean total?"),
            (shown.returncode, shown.stderr.strip()),
        )


if __name__ == "__main__":
    unittest.main()
