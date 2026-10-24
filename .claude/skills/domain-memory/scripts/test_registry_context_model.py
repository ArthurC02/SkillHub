from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.common import CONTEXT_RELATIONSHIPS
from domain_registry.registry import (
    boundary_analysis,
    context_model,
    coverage,
    record_by_id,
    validate,
)
from domain_registry.evidence import citation
from test_support import VERIFIED, RegistryTestCase, amend_policy, orders_context


class RegistryContextModelTest(RegistryTestCase):
    def test_an_absent_record_is_reported_as_absent_rather_than_raised(self) -> None:
        self.two_contexts()
        record = record_by_id(self.repo / "memory", "contexts", "orders")
        self.assertIsNotNone(record)
        assert record is not None
        self.assertEqual(record["name"], "Orders")
        self.assertIsNone(record_by_id(self.repo / "memory", "contexts", "shipping"))

    def test_an_unknown_context_has_no_model(self) -> None:
        self.two_contexts()
        self.assertIsNone(context_model(self.repo / "memory", "shipping"))

    def test_a_context_model_carries_the_contracts_it_consumes_as_well_as_those_it_produces(
        self,
    ) -> None:
        self.two_contexts()
        self.seed(
            "contracts.json",
            [
                {
                    "id": "invoice-api",
                    "producer_context": "billing",
                    "consumer_contexts": ["orders"],
                },
                {
                    "id": "order-api",
                    "producer_context": "orders",
                    "consumer_contexts": [],
                },
                {
                    "id": "shipping-api",
                    "producer_context": "billing",
                    "consumer_contexts": [],
                },
            ],
        )
        model = context_model(self.repo / "memory", "orders")
        self.assertIsNotNone(model)
        assert model is not None
        self.assertEqual(model["usage"], "constraint")
        self.assertEqual(
            sorted(entry["id"] for entry in model["contracts"]),
            ["invoice-api", "order-api"],
        )

    def test_a_local_draft_context_model_is_working_memory(self) -> None:
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        self.two_contexts()
        model = context_model(self.repo / "memory", "orders")
        self.assertIsNotNone(model)
        assert model is not None
        self.assertEqual(model["usage"], "working-memory")

    def test_local_draft_memory_cannot_satisfy_reviewed_validation(self) -> None:
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        self.assertEqual(
            ["local-draft-only Domain Memory cannot satisfy --require-reviewed"],
            validate(self.repo / "memory", self.repo, True),
        )

    def test_two_contexts_cannot_claim_the_same_code(self) -> None:
        self.assertEqual(
            ["contexts/orders claims sales/orders, which overlaps sales/orders claimed by billing: code belongs to one Context"],
            self.placement_errors(("orders", "sales/orders"), ("billing", "sales/orders")),
        )

    def test_a_context_cannot_claim_code_inside_another_context(self) -> None:
        self.assertEqual(
            ["contexts/sales claims sales, which overlaps sales/orders claimed by orders: code belongs to one Context"],
            self.placement_errors(("sales", "sales"), ("orders", "sales/orders")),
        )

    def test_sibling_paths_that_share_a_name_prefix_do_not_overlap(self) -> None:
        errors = self.placement_errors(("orders", "sales/order"), ("ordering", "sales/ordering"))
        self.assertEqual([error for error in errors if "overlaps" in error], [])

    def test_a_context_subdomain_is_a_closed_set(self) -> None:
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "subdomain": "strategic",
                }
            ],
        )
        self.assertEqual(
            ["contexts/orders has an unknown subdomain: 'strategic' is not one of core, generic, supporting"],
            validate(self.repo / "memory", self.repo, False),
        )

    def test_shared_kernel_is_refused_as_a_subdomain_and_pointed_to_the_interaction(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "subdomain": "shared-kernel"},
        ])
        self.assertIn(
            "contexts/orders has an unknown subdomain: 'shared-kernel' is not one of core, generic, supporting; "
            "shared-kernel is a relationship between two Contexts, recorded on their Interaction",
            validate(self.repo / "memory", self.repo, False),
        )

    def test_a_collaboration_field_outside_its_closed_set_is_refused(self) -> None:
        cases = [
            ("relationship", "interactions.json",
             [{"id": "i", "producer_context": "orders", "consumer_context": "billing", "relationship": "friends"}],
             "interactions/i has an unknown relationship: 'friends' is not one of anticorruption-layer, conformist, "
             "customer-supplier, open-host-service, partnership, published-language, shared-kernel"),
            ("separate ways on a collaboration", "interactions.json",
             [{"id": "i", "producer_context": "orders", "consumer_context": "billing", "relationship": "separate-ways"}],
             "interactions/i has an unknown relationship: 'separate-ways' is not one of anticorruption-layer, conformist, "
             "customer-supplier, open-host-service, partnership, published-language, shared-kernel; separate-ways is a "
             "decision not to collaborate, recorded as a dependency policy with policy forbidden and mode none"),
            ("mode", "dependency-policies.json",
             [{"id": "p", "from_context": "orders", "to_context": "billing", "mode": "sometimes", "policy": "allowed"}],
             "dependency-policies/p has an unknown mode: 'sometimes' is not one of asynchronous, none, "
             "synchronous-query, synchronous-write"),
            ("policy", "dependency-policies.json",
             [{"id": "p", "from_context": "orders", "to_context": "billing", "mode": "none", "policy": ["forbidden"]}],
             "dependency-policies/p has an unknown policy: ['forbidden'] is not one of allowed, forbidden"),
        ]
        orders = orders_context()
        billing = {"id": "billing", "name": "Billing", "responsibility": "Own invoices."}
        for label, name, records, message in cases:
            with self.subTest(label):
                self.seed("contexts.json", [orders, billing])
                self.seed(name, records)
                self.assertIn(message, validate(self.repo / "memory", None, False))
        self.seed("dependency-policies.json", [])
        for relationship in sorted(CONTEXT_RELATIONSHIPS):
            with self.subTest(relationship):
                self.seed("interactions.json", [
                    {"id": "i", "producer_context": "orders", "consumer_context": "billing", "relationship": relationship},
                ])
                self.assertEqual([], [error for error in validate(self.repo / "memory", None, False) if "unknown" in error])

    def test_a_known_subdomain_is_accepted(self) -> None:
        for subdomain in ("core", "supporting", "generic"):
            with self.subTest(subdomain=subdomain):
                self.seed(
                    "contexts.json",
                    [
                        {
                            "id": "orders",
                            "name": "Orders",
                            "responsibility": "Own orders.",
                            "subdomain": subdomain,
                        }
                    ],
                )
                errors = validate(self.repo / "memory", self.repo, False)
                self.assertEqual(
                    [error for error in errors if "subdomain" in error], []
                )

    def test_requirement_prefixes_must_be_a_list_of_names(self) -> None:
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "requirement_prefixes": "ORD",
                }
            ],
        )
        errors = validate(self.repo / "memory", self.repo, False)
        self.assertTrue(
            any("requirement_prefixes" in error for error in errors), errors
        )

    def test_a_registry_of_reviewed_records_satisfies_reviewed_validation(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept\n", encoding="utf-8")
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "status": "reviewed",
                    "verified": VERIFIED,
                    "evidence": [citation(self.repo, "docs/kept.md", 1, 1)],
                }
            ],
        )
        self.assertEqual([], validate(self.repo / "memory", self.repo, True))

    def test_a_record_holding_a_review_block_is_refused(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders.",
                                     "review": {"proposal_id": "P-1"}}])
        self.assertEqual(
            ["contexts/orders holds a review block; this version reads verified only"],
            validate(self.repo / "memory", None, False),
        )

    def test_with_the_repository_a_reviewed_implementation_path_over_no_file_is_refused(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "implementation_path": "src/orders",
             "status": "reviewed", "verified": VERIFIED},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices.", "implementation_path": "src/billing"},
        ])
        refusal = ("contexts/orders implementation_path src/orders lies over no file in the repository; after a move, "
                   "relocate-evidence --from <old repository path> --to <new> proposes the new path, or correct it")
        def placement(repo_root: Path | None) -> list[str]:
            return [error for error in validate(self.repo / "memory", repo_root, False) if "implementation_path" in error]

        before = (placement(self.repo), placement(None))
        (self.repo / "apps" / "src" / "orders").mkdir(parents=True)
        (self.repo / "apps" / "src" / "orders" / "port.py").write_text("v1\n", encoding="utf-8")
        self.assertEqual((([refusal], []), []), (before, placement(self.repo)))

    def test_a_given_file_listing_decides_placement_instead_of_a_repository_walk(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "implementation_path": "src/orders",
             "status": "reviewed", "verified": VERIFIED},
        ])
        (self.repo / "apps" / "src" / "orders").mkdir(parents=True)
        (self.repo / "apps" / "src" / "orders" / "port.py").write_text("v1\n", encoding="utf-8")
        def placement(listing) -> list[str]:
            return [error.split(";")[0] for error in validate(self.repo / "memory", self.repo, False, listing)
                    if "implementation_path" in error]

        self.assertEqual(([], ["contexts/orders implementation_path src/orders lies over no file in the repository"]),
                         (placement(None), placement(lambda: ["lib/billing/invoice.py"])))

    def test_two_contexts_without_registered_collaboration_say_so(self) -> None:
        self.two_contexts()
        self.assertEqual(
            boundary_analysis(self.repo / "memory", "orders", "billing")["status"],
            "no_registered_collaboration",
        )

    def test_a_registered_interaction_makes_a_boundary_known_under_the_consumer_that_depends_on_its_producer(
        self,
    ) -> None:
        self.two_contexts()
        self.seed("interactions.json", [
            {"id": "order-billed", "producer_context": "orders", "consumer_context": "billing"},
        ])
        for source, target, direction in (("billing", "orders", "source_depends_on_target"),
                                          ("orders", "billing", "target_depends_on_source")):
            with self.subTest(direction):
                analysis = boundary_analysis(self.repo / "memory", source, target)
                other = ({"source_depends_on_target", "target_depends_on_source"} - {direction}).pop()
                self.assertEqual(
                    ("known", ["order-billed"], {"interactions": [], "contracts": [], "dependencies": [], "events": []}),
                    (analysis["status"], [entry["id"] for entry in analysis[direction]["interactions"]], analysis[other]),
                )

    def test_a_boundary_lists_each_direction_so_opposite_collaborations_are_not_mixed(self) -> None:
        self.two_contexts()
        self.seed("interactions.json", [
            {"id": "purge-hook", "producer_context": "billing", "consumer_context": "orders"},
        ])
        self.seed("dependency-policies.json", [
            {"id": "billing-may-use-orders", "from_context": "billing", "to_context": "orders"},
        ])
        self.seed("events.json", [
            {"id": "order-placed", "owner_context": "orders", "consumers": ["billing"]},
        ])
        analysis = boundary_analysis(self.repo / "memory", "billing", "orders")
        self.assertEqual(
            ("known",
             {"interactions": [], "contracts": [], "dependencies": ["billing-may-use-orders"], "events": ["order-placed"]},
             {"interactions": ["purge-hook"], "contracts": [], "dependencies": [], "events": []}),
            (analysis["status"],
             {key: [entry["id"] for entry in value] for key, value in analysis["source_depends_on_target"].items()},
             {key: [entry["id"] for entry in value] for key, value in analysis["target_depends_on_source"].items()}),
        )

    def test_an_event_with_a_consumer_alone_makes_a_boundary_known(self) -> None:
        self.two_contexts()
        self.seed("events.json", [{"id": "order-placed", "owner_context": "orders", "consumers": ["billing"]}])
        self.assertEqual("known", boundary_analysis(self.repo / "memory", "orders", "billing")["status"])

    def test_a_dependency_policy_alone_is_a_permission_without_a_collaboration(self) -> None:
        self.two_contexts()
        self.seed(
            "dependency-policies.json",
            [
                {
                    "id": "orders-billing",
                    "from_context": "orders",
                    "to_context": "billing",
                }
            ],
        )
        analysis = boundary_analysis(self.repo / "memory", "orders", "billing")
        self.assertEqual(
            ("policy_only", ["orders-billing"], []),
            (analysis["status"], [entry["id"] for entry in analysis["source_depends_on_target"]["dependencies"]],
             analysis["target_depends_on_source"]["dependencies"]),
        )

    def test_coverage_reports_a_context_only_under_the_asset_it_lacks(self) -> None:
        self.seed(
            "contexts.json",
            [orders_context()],
        )
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
        gaps = coverage(self.repo / "memory")["gaps"]
        self.assertEqual(gaps["without_vocabulary"], [])
        self.assertEqual(gaps["without_aggregate"], ["orders"])
        self.assertEqual(gaps["without_rule"], ["orders"])
        self.assertEqual(gaps["without_contract"], ["orders"])
        self.assertEqual(gaps["without_event"], ["orders"])

    def test_each_defect_in_a_context_s_prefixes_or_implementation_path_is_named(self) -> None:
        prefixes = "contexts/orders requirement_prefixes must be a list of non-empty strings"
        path = "contexts/orders implementation_path must be a non-empty string"
        for label, field, value, message in (
            ("blank prefix", "requirement_prefixes", ["ORD", " "], prefixes),
            ("prefix not text", "requirement_prefixes", [7], prefixes),
            ("blank path", "implementation_path", " ", path),
            ("path not text", "implementation_path", 7, path),
        ):
            with self.subTest(label):
                self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders.", field: value}])
                self.assertEqual([message], validate(self.repo / "memory", self.repo, False))

    def test_each_defect_in_a_confirmed_absence_is_named(self) -> None:
        cases = [
            ("not a list", "rules", [], "contexts/orders confirmed_absences must be a list"),
            ("unknown asset", [{"asset": "decisions", "reason": "None."}], [],
             "contexts/orders confirms an absence of an unknown asset; use one of aggregates, contracts, events, rules, vocabulary"),
            ("not an object", ["rules"], [],
             "contexts/orders confirms an absence of an unknown asset; use one of aggregates, contracts, events, rules, vocabulary"),
            ("blank reason", [{"asset": "rules", "reason": " "}], [],
             "contexts/orders confirms an absence of rules without a reason; a confirmed absence is a finding and needs one"),
            ("contradicted by a record", [{"asset": "rules", "reason": "Orders decides nothing."}],
             [{"id": "order-total", "contexts": ["orders"], "statement": "S."}],
             "contexts/orders confirms an absence of rules, but rules records name it"),
        ]
        for label, absences, rules, message in cases:
            with self.subTest(label):
                self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders.",
                                             "confirmed_absences": absences}])
                self.seed("rules.json", rules)
                self.assertEqual([message], validate(self.repo / "memory", self.repo, False))

    def test_coverage_counts_events_and_takes_a_confirmed_absence_of_events(self) -> None:
        self.seed("contexts.json", [
            orders_context(),
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices.",
             "confirmed_absences": [{"asset": "events", "reason": "Billing publishes nothing."}]},
        ])
        self.seed("events.json", [{"id": "order-placed", "owner_context": "orders"}])
        report = coverage(self.repo / "memory")
        self.assertEqual((1, [], ["billing"]),
                         (report["events"], report["gaps"]["without_event"], report["confirmed_absent"]["without_event"]))

    def bound(self, rule_paths: list[str]) -> list[dict]:
        def cite(path: str) -> list[dict]:
            return [{"path": path, "lines": {"start": 1, "end": 1}}]

        self.seed("contexts.json", [{"id": "eval", "name": "Eval"}, {"id": "identity", "name": "Identity"}])
        self.seed("interactions.json", [{"id": "eval-identity-purge", "producer_context": "identity",
                                         "consumer_context": "eval", "evidence": cite("purge/hooks.go")}])
        self.seed("rules.json", [
            {"id": f"rule-{number}", "contexts": [context], "statement": f"Rule {number}.", "evidence": cite(path)}
            for number, (context, path) in enumerate([("eval", "purge/hooks.go"), *(("identity", path) for path in rule_paths)])
        ])
        return context_model(self.repo / "memory", "eval")["rules_bound_through_collaborations"]

    def test_a_rule_of_another_context_citing_a_collaborations_evidence_binds_this_context(self) -> None:
        self.assertEqual(
            [{"rule": "rule-1", "contexts": ["identity"], "statement": "Rule 1.", "via": ["interactions/eval-identity-purge"]}],
            self.bound(["purge/hooks.go", "identity/email.go"]),
        )

    def test_a_rule_of_another_context_citing_other_files_does_not_bind_this_context(self) -> None:
        self.assertEqual([], self.bound(["identity/email.go"]))

    def test_without_a_policy_file_the_context_model_is_working_memory(self) -> None:
        self.two_contexts()
        (self.repo / "memory" / "domain-memory-policy.json").unlink()
        model = context_model(self.repo / "memory", "orders")
        assert model is not None
        self.assertEqual(("working-memory", "no-policy"), (model["usage"], model["review_mode"]))

    def test_reviewed_validation_without_a_policy_file_cannot_be_satisfied(self) -> None:
        (self.repo / "memory" / "domain-memory-policy.json").unlink()
        self.assertEqual(
            ["local-draft-only Domain Memory cannot satisfy --require-reviewed"],
            validate(self.repo / "memory", None, True),
        )

    def test_a_context_model_gathers_each_section_from_the_records_that_name_the_context(self) -> None:
        self.two_contexts()
        self.seed("vocabulary.json", [
            {"id": "order", "name": "Order", "contexts": ["orders"]},
            {"id": "invoice", "name": "Invoice", "contexts": ["billing"]},
            {"id": "ledger-entry", "name": "Ledger Entry", "contexts": ["billing", "orders"]},
        ])
        self.seed("aggregates.json", [
            {"id": "order-aggregate", "context": "orders"}, {"id": "invoice-aggregate", "context": "billing"},
        ])
        self.seed("rules.json", [
            {"id": "order-total", "contexts": ["orders"]}, {"id": "invoice-due", "contexts": ["billing"]},
            {"id": "shared-currency", "contexts": ["billing", "orders"]},
        ])
        self.seed("interactions.json", [
            {"id": "order-billed", "producer_context": "orders", "consumer_context": "billing"},
            {"id": "invoice-paid", "producer_context": "billing", "consumer_context": "orders"},
            {"id": "invoice-archived", "producer_context": "billing", "consumer_context": "billing"},
        ])
        self.seed("events.json", [
            {"id": "order-placed", "owner_context": "orders"}, {"id": "invoice-issued", "owner_context": "billing"},
            {"id": "payment-received", "owner_context": "billing", "consumers": ["orders"]},
        ])
        self.seed("capabilities.json", [
            {"id": "place-order", "context": "orders"}, {"id": "issue-invoice", "context": "billing"},
        ])
        self.seed("value-objects.json", [
            {"id": "money", "context": "orders"}, {"id": "tax-rate", "context": "billing"},
        ])
        model = context_model(self.repo / "memory", "orders")
        assert model is not None
        expected = {
            "vocabulary": ["ledger-entry", "order"],
            "aggregates": ["order-aggregate"],
            "rules": ["order-total", "shared-currency"],
            "interactions": ["invoice-paid", "order-billed"],
            "events": ["order-placed", "payment-received"],
            "capabilities": ["place-order"],
            "value_objects": ["money"],
        }
        for section, ids in expected.items():
            with self.subTest(section):
                self.assertEqual(ids, [entry["id"] for entry in model[section]])
        self.assertEqual(
            ("orders", "constraint", "scm-verified"),
            (model["context"]["id"], model["usage"], model["review_mode"]),
        )

    def test_a_registered_contract_makes_a_boundary_known_under_the_consumer_whichever_way_it_is_asked(self) -> None:
        self.two_contexts()
        self.seed("contracts.json", [
            {"id": "order-api", "producer_context": "orders", "consumer_contexts": ["billing"]},
        ])
        asked = boundary_analysis(self.repo / "memory", "billing", "orders")
        reversed_ask = boundary_analysis(self.repo / "memory", "orders", "billing")
        self.assertEqual(
            ("known", ["order-api"], [], "known", ["order-api"]),
            (asked["status"], [entry["id"] for entry in asked["source_depends_on_target"]["contracts"]],
             asked["target_depends_on_source"]["contracts"],
             reversed_ask["status"], [entry["id"] for entry in reversed_ask["target_depends_on_source"]["contracts"]]),
        )

if __name__ == "__main__":
    unittest.main()
