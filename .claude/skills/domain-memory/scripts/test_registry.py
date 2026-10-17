from __future__ import annotations

import argparse
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.common import ASSET_KEYS, load_json
from domain_registry.contracts import validate_schema
from domain_registry.cli_commands import HANDLERS, policy_assignments
from domain_registry.cli_parser import build_parser
from domain_registry.registry import (
    boundary_analysis,
    context_model,
    coverage,
    lookup,
    migrate_evidence,
    _is_common,
    ranked_lookup,
    word_stems,
    migrate_registry,
    record_by_id,
    resolve_terms,
    validate,
    verify_evidence,
)
from domain_registry.store import read_document, write_document
from test_support import VERIFIED, RegistryTestCase, append_audit, scan_secrets, amend_policy, use_json_layout


class RegistryTest(RegistryTestCase):
    def test_lookup_honors_policy_result_limit(self) -> None:
        value = read_document(self.repo / "memory", "contexts.json")
        value["contexts"] = [
            {"id": "one", "name": "One", "responsibility": "First."},
            {"id": "two", "name": "Two", "responsibility": "Second."},
        ]
        write_document(self.repo / "memory", "contexts.json", value)
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["limits"]["max_query_results"] = 1
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        self.assertEqual(1, len(lookup(self.repo / "memory", "contexts", "")))

    def test_migrate_evidence_upgrades_existing_line_reference(self) -> None:
        self.seed_line_reference_evidence()
        self.assertEqual(1, migrate_evidence(self.repo / "memory", self.repo))
        self.assertEqual(
            1, verify_evidence(self.repo / "memory", self.repo)["summary"]["current"]
        )

    def test_each_record_and_reference_defect_is_named(self) -> None:
        use_json_layout(self.repo / "memory")
        orders = {"id": "orders", "name": "Orders", "responsibility": "Own orders."}
        billing = {"id": "billing", "name": "Billing", "responsibility": "Own invoices."}
        order_term = {"id": "order", "name": "Order", "contexts": ["orders"]}
        invoice_term = {"id": "invoice", "name": "Invoice", "contexts": ["orders"]}
        cases = [
            ("duplicate id", "contexts.json", [orders, dict(orders)], "contexts.json contains duplicate id"),
            ("blank id", "contexts.json", [orders, {**billing, "id": ""}], "contexts.json entries require non-empty id"),
            ("unknown status", "contexts.json", [{**orders, "status": "pending"}, billing],
             "contexts/orders has an invalid status"),
            ("decision review status", "decisions.json", [{"id": "d", "review_status": "bogus", "status": "candidate"}],
             "decisions/d has an invalid status"),
            ("reviewed without fields", "rules.json",
             [{"id": "r", "contexts": ["orders"], "status": "reviewed", "evidence": ["x"]}],
             "rules/r is reviewed but missing statement, verified"),
            ("reviewed without evidence", "rules.json",
             [{"id": "r", "contexts": ["orders"], "statement": "s", "verified": VERIFIED, "status": "reviewed"}],
             "rules/r is reviewed but lacks evidence"),
            ("term without contexts", "vocabulary.json", [{"id": "t", "contexts": []}], "vocabulary/t requires contexts"),
            ("synonyms not a list", "vocabulary.json", [{**order_term, "synonyms": "purchase"}],
             "vocabulary/order synonyms and not_same_as must be lists of non-empty strings"),
            ("blank synonym", "vocabulary.json", [{**order_term, "synonyms": [" "]}],
             "vocabulary/order synonyms and not_same_as must be lists of non-empty strings"),
            ("look-alikes not a list", "vocabulary.json", [{**order_term, "not_same_as": "invoice"}],
             "vocabulary/order synonyms and not_same_as must be lists of non-empty strings"),
            ("repeated synonym", "vocabulary.json", [{**order_term, "synonyms": ["Purchase", "purchase"]}],
             "vocabulary/order repeats a synonym"),
            ("own name as synonym", "vocabulary.json", [{**order_term, "synonyms": ["order"]}],
             "vocabulary/order lists its own name as a synonym"),
            ("synonym naming a term of the same context", "vocabulary.json",
             [{**order_term, "synonyms": ["Invoice"]}, invoice_term],
             "vocabulary/order synonym 'Invoice' is the name of invoice in a shared context"),
            ("unknown look-alike", "vocabulary.json", [{**order_term, "not_same_as": ["ghost"]}],
             "vocabulary/order not_same_as must name other vocabulary term ids"),
            ("itself as a look-alike", "vocabulary.json", [{**order_term, "not_same_as": ["order"]}],
             "vocabulary/order not_same_as must name other vocabulary term ids"),
            ("rule in unknown context", "rules.json", [{"id": "r", "contexts": ["ghost"]}],
             "rules/r references unknown context ghost; add the Context first"),
            ("aggregate in unknown context", "aggregates.json", [{"id": "a", "context": "ghost"}],
             "aggregates/a references unknown context ghost; add the Context first"),
            ("unknown consumer", "interactions.json",
             [{"id": "i", "producer_context": "orders", "consumer_context": "ghost"}],
             "interactions/i references unknown context ghost; add the Context first"),
            ("policy to unknown context", "dependency-policies.json",
             [{"id": "p", "from_context": "orders", "to_context": "ghost"}],
             "dependency-policies/p references unknown context ghost; add the Context first"),
            ("contract producer", "contracts.json", [{"id": "c", "producer_context": "ghost", "consumer_contexts": []}],
             "contracts/c references as producer unknown context ghost; add the Context first"),
            ("contract consumer", "contracts.json",
             [{"id": "c", "producer_context": "orders", "consumer_contexts": ["ghost"]}],
             "contracts/c references as consumer unknown context ghost; add the Context first"),
            ("unknown producer", "interactions.json",
             [{"id": "i", "producer_context": "ghost", "consumer_context": "orders"}],
             "interactions/i references unknown context ghost; add the Context first"),
            ("event in unknown context", "events.json", [{"id": "e", "owner_context": "ghost"}],
             "events/e references unknown context ghost; add the Context first"),
            ("event consumed by unknown context", "events.json",
             [{"id": "e", "owner_context": "orders", "consumers": ["billing", "ghost"]}],
             "events/e references as consumer unknown context ghost; add the Context first"),
            ("event consumers not a list", "events.json", [{"id": "e", "owner_context": "orders", "consumers": "billing"}],
             "events/e consumers must be a list of Context ids"),
            ("capability in unknown context", "capabilities.json", [{"id": "k", "context": "ghost"}],
             "capabilities/k references unknown context ghost; add the Context first"),
            ("value object in unknown context", "value-objects.json", [{"id": "v", "context": "ghost"}],
             "value-objects/v references unknown context ghost; add the Context first"),
            ("contract consumers not a list", "contracts.json",
             [{"id": "c", "producer_context": "orders", "consumer_contexts": "billing"}],
             "contracts/c consumer_contexts must be a list of Context ids"),
            ("policy between two unknown contexts", "dependency-policies.json",
             [{"id": "p", "from_context": "ghost", "to_context": "phantom"}],
             "dependency-policies/p references unknown context ghost, phantom; add the Context first"),
            ("unknown contract", "interactions.json",
             [{"id": "i", "producer_context": "orders", "consumer_context": "billing", "contract_id": "none"}],
             "interactions/i references unknown contract"),
        ]
        root = self.repo / "memory"
        pristine = {name: read_document(root, name) for name in ASSET_KEYS}
        for label, name, records, message in cases:
            with self.subTest(label):
                for asset, document in pristine.items():
                    write_document(root, asset, document)
                if name != "contexts.json":
                    self.seed("contexts.json", [orders, billing])
                self.seed(name, records)
                self.assertIn(message, validate(self.repo / "memory", None, False))

    def test_a_bound_aggregate_must_exist_and_sit_in_a_context_the_record_names(self) -> None:
        cases = [
            ("rule bound to an unknown aggregate", "rules.json",
             [{"id": "r", "contexts": ["orders"], "statement": "S.", "aggregate": "ghost"}],
             "rules/r references unknown aggregate ghost; add the Aggregate first"),
            ("rule bound to an aggregate of a context it does not name", "rules.json",
             [{"id": "r", "contexts": ["billing"], "statement": "S.", "aggregate": "order"}],
             "rules/r aggregate order belongs to context orders, which the record does not name"),
            ("event bound to an aggregate of another context", "events.json",
             [{"id": "e", "owner_context": "billing", "aggregate": "order"}],
             "events/e aggregate order belongs to context orders, which the record does not name"),
            ("event under an unknown contract", "events.json",
             [{"id": "e", "owner_context": "orders", "contract_id": "none"}],
             "events/e references unknown contract"),
        ]
        orders = {"id": "orders", "name": "Orders", "responsibility": "Own orders."}
        billing = {"id": "billing", "name": "Billing", "responsibility": "Own invoices."}
        root = self.repo / "memory"
        for label, name, records, message in cases:
            with self.subTest(label):
                self.seed("contexts.json", [orders, billing])
                self.seed("aggregates.json", [{"id": "order", "context": "orders", "root": "Order"}])
                self.seed(name, records)
                self.assertIn(message, validate(root, None, False))
        self.seed("contracts.json", [{"id": "c", "producer_context": "orders", "consumer_contexts": ["billing"]}])
        self.seed("rules.json", [{"id": "r", "contexts": ["orders"], "statement": "S.", "aggregate": "order"}])
        self.seed("events.json", [{"id": "e", "owner_context": "orders", "aggregate": "order", "contract_id": "c"}])
        self.assertEqual(
            [], [error for error in validate(root, None, False) if "aggregate" in error or "contract" in error]
        )

    def test_validate_refuses_an_unhashed_citation(self) -> None:
        self.two_line_rule()
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders.",
                                      "evidence": [{"path": "rule.md", "lines": {"start": 1, "end": 1}}]}])
        self.assertIn(
            "evidence unhashed: rule.md was cited without an excerpt hash; regenerate the citation with cite",
            validate(self.repo / "memory", self.repo, False),
        )
        self.assertEqual(1, verify_evidence(self.repo / "memory", self.repo)["summary"]["unhashed"])

    def test_contract_idempotency_states_each_part_in_text(self) -> None:
        sound = {"key": "origin key", "scope": "per Workspace", "on_replay": "same Run", "on_conflict": "refused"}
        cases = [
            ("not an object", "origin key", "contracts/c idempotency must be an object"),
            ("missing parts", {"key": "origin key"}, "contracts/c idempotency lacks scope, on_replay, on_conflict"),
            ("unknown part", {**sound, "on_retry": "x"}, "contracts/c idempotency has unknown fields on_retry"),
            ("blank part", {**sound, "scope": " "}, "contracts/c idempotency fields must be non-empty text: scope"),
            ("blank expiry", {**sound, "expiry": ""}, "contracts/c idempotency fields must be non-empty text: expiry"),
        ]
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        contract = {"id": "c", "producer_context": "orders", "consumer_contexts": []}
        for label, idempotency, message in cases:
            with self.subTest(label):
                self.seed("contracts.json", [{**contract, "idempotency": idempotency}])
                self.assertIn(message, validate(self.repo / "memory", None, False))
        with self.subTest("sound, with an expiry"):
            self.seed("contracts.json", [{**contract, "idempotency": {**sound, "expiry": "24 hours"}}])
            self.assertEqual([], validate(self.repo / "memory", None, False))

    def test_term_labels_may_reuse_a_name_from_another_context_and_point_at_a_look_alike(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
        ])
        self.seed("vocabulary.json", [
            {"id": "order", "name": "Order", "contexts": ["orders"],
             "synonyms": ["Purchase", "Invoice"], "not_same_as": ["invoice"]},
            {"id": "invoice", "name": "Invoice", "contexts": ["billing"]},
            {"id": "line", "name": "Order Line", "contexts": ["orders"]},
        ])
        self.assertEqual([], validate(self.repo / "memory", None, False))

    def test_audit_refuses_to_extend_a_corrupt_log(self) -> None:
        root = self.repo / "memory"
        append_audit(root, {"operation": "first"})
        events = root / "audit" / "events.jsonl"
        events.write_text(
            events.read_text(encoding="utf-8").replace("first", "tampered"),
            encoding="utf-8",
        )
        with self.assertRaisesRegex(ValueError, "audit log is invalid"):
            append_audit(root, {"operation": "second"})
        self.assertEqual(1, len(events.read_text(encoding="utf-8").splitlines()))

    def test_a_registry_root_one_directory_too_deep_is_named_as_such(self) -> None:
        use_json_layout(self.repo / "memory")
        inner = self.repo / "memory" / "registry"
        self.assertEqual(
            [
                f"no Domain Registry at {inner}: it holds neither registry/index.md nor registry/manifest.json; "
                f"{inner} is the registry/ directory itself, so pass the directory that contains it, {self.repo / 'memory'}"
            ],
            validate(inner, self.repo, False),
        )
        with self.assertRaisesRegex(ValueError, "points one directory too deep"):
            load_json(inner / "domain-memory-policy.json")

    def test_a_json_registry_without_a_manifest_is_still_read(self) -> None:
        root = self.repo / "memory"
        use_json_layout(root)
        contexts = read_document(root, "contexts.json")
        (root / "registry" / "manifest.json").unlink()
        self.assertEqual(contexts, read_document(root, "contexts.json"))

    def test_a_missing_file_with_nothing_nearby_gets_no_hint(self) -> None:
        missing = self.repo / "memory" / "nowhere.json"
        with self.assertRaises(ValueError) as refused:
            load_json(missing)
        self.assertEqual(f"JSON file required: {missing}", str(refused.exception))
        elsewhere = self.repo / "elsewhere"
        self.assertEqual(
            [f"no Domain Registry at {elsewhere}: it holds neither registry/index.md nor registry/manifest.json"],
            validate(elsewhere, None, False),
        )
        with self.assertRaisesRegex(ValueError, "^no Domain Registry at "):
            read_document(elsewhere, "contexts.json")
        for beside in (self.repo / "memory" / "other", elsewhere / "registry"):
            with self.subTest(beside.as_posix()):
                self.assertEqual(
                    [f"no Domain Registry at {beside}: it holds neither registry/index.md nor registry/manifest.json"],
                    validate(beside, None, False),
                )

    def test_a_concept_registry_root_one_directory_too_deep_is_named_as_such(self) -> None:
        inner = self.repo / "memory" / "registry"
        with self.assertRaisesRegex(ValueError, "is the registry/ directory itself, so pass the directory that contains it"):
            read_document(inner, "contexts.json")

    def test_json_loader_rejects_duplicate_keys(self) -> None:
        path = self.repo / "duplicate.json"
        path.write_text('{"status":"approved","status":"rejected"}\n', encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "invalid JSON"):
            load_json(path)

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
        matches = resolve_terms(
            self.repo / "memory", "Who starts the Evaluation once a Run succeeds?", None
        )
        self.assertEqual([term["id"] for term in matches], ["run"])

    def test_a_sentence_naming_no_registered_term_still_resolves_to_nothing(
        self,
    ) -> None:
        self.defined_term()
        self.assertEqual(
            resolve_terms(
                self.repo / "memory", "Who signs off on the quarterly budget?", None
            ),
            [],
        )

    def test_a_word_from_the_definition_still_resolves_the_term(self) -> None:
        self.defined_term()
        matches = resolve_terms(self.repo / "memory", "test case", None)
        self.assertEqual([(term["id"], term["match_kind"]) for term in matches], [("run", "definition")])

    def test_a_name_in_the_query_is_a_name_match(self) -> None:
        self.defined_term()
        matches = resolve_terms(self.repo / "memory", "a run", None)
        self.assertEqual([(term["id"], term["match_kind"]) for term in matches], [("run", "name")])

    def test_a_synonym_in_the_query_is_a_synonym_match(self) -> None:
        self.seed("vocabulary.json", [
            {"id": "run", "name": "Run", "definition": "One trial.", "contexts": ["trial"],
             "synonyms": ["Execution"]},
        ])
        self.allow_results(10)
        matches = resolve_terms(self.repo / "memory", "retry the execution", None)
        self.assertEqual([(term["id"], term["match_kind"]) for term in matches], [("run", "synonym")])

    def test_name_matches_come_before_synonym_and_definition_matches(self) -> None:
        self.seed("vocabulary.json", [
            {"id": "attempt", "name": "Attempt", "definition": "One execution of a Run.", "contexts": ["trial"]},
            {"id": "job", "name": "Job", "definition": "Queued work.", "contexts": ["trial"],
             "synonyms": ["Run request"]},
            {"id": "run", "name": "Run", "definition": "One trial.", "contexts": ["trial"]},
        ])
        self.allow_results(10)
        matches = resolve_terms(self.repo / "memory", "run", None)
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
            for term in resolve_terms(self.repo / "memory", "confirmed purchase", None)
        ]
        self.assertEqual(found, ["order"])

    def test_resolve_terms_matches_regardless_of_letter_case(self) -> None:
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
        self.assertEqual(len(resolve_terms(self.repo / "memory", "ORDER", None)), 1)

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
            resolve_terms(self.repo / "memory", "order", "orders")[0]["id"], "order"
        )
        self.assertEqual(resolve_terms(self.repo / "memory", "order", "billing"), [])

    def test_resolve_terms_stops_at_the_policy_result_limit(self) -> None:
        self.two_contexts()
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A purchase.",
                    "contexts": ["orders"],
                },
                {
                    "id": "order-line",
                    "name": "Order line",
                    "definition": "A purchase line.",
                    "contexts": ["orders"],
                },
            ],
        )
        self.allow_results(1)
        self.assertEqual(len(resolve_terms(self.repo / "memory", "purchase", None)), 1)

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
        errors = validate(self.repo / "memory", self.repo, True)
        self.assertTrue(any("cannot satisfy" in error for error in errors), errors)

    def test_two_contexts_cannot_claim_the_same_code(self) -> None:
        errors = self.placement_errors(("orders", "sales/orders"), ("billing", "sales/orders"))
        self.assertTrue(any("overlaps" in error for error in errors), errors)

    def test_a_context_cannot_claim_code_inside_another_context(self) -> None:
        errors = self.placement_errors(("sales", "sales"), ("orders", "sales/orders"))
        self.assertTrue(any("overlaps" in error for error in errors), errors)

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
        errors = validate(self.repo / "memory", self.repo, False)
        self.assertTrue(any("unknown subdomain" in error for error in errors), errors)

    def test_a_known_subdomain_is_accepted(self) -> None:
        for subdomain in ("core", "supporting", "generic", "shared-kernel"):
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
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "status": "reviewed",
                    "verified": VERIFIED,
                    "evidence": ["docs/kept.md:1"],
                }
            ],
        )
        self.assertEqual([], validate(self.repo / "memory", self.repo, True))

    def test_a_record_holding_a_legacy_review_block_is_sent_to_migrate_registry(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders.",
                                     "review": {"proposal_id": "P-1"}}])
        self.assertEqual(
            ["contexts/orders holds a legacy review block; run migrate-registry"],
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
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
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

    def test_coverage_counts_events_and_takes_a_confirmed_absence_of_events(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices.",
             "confirmed_absences": [{"asset": "events", "reason": "Billing publishes nothing."}]},
        ])
        self.seed("events.json", [{"id": "order-placed", "owner_context": "orders"}])
        report = coverage(self.repo / "memory")
        self.assertEqual((1, [], ["billing"]),
                         (report["events"], report["gaps"]["without_event"], report["confirmed_absent"]["without_event"]))

    def test_requiring_reviewed_records_rejects_a_candidate_registry(self) -> None:
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        self.assertEqual(validate(self.repo / "memory", None, False), [])
        self.assertEqual(
            ["contexts/orders is not reviewed"], validate(self.repo / "memory", None, True)
        )

    def test_a_non_json_contract_is_reported_unverified_instead_of_valid(self) -> None:
        path = self.repo / "openapi.yaml"
        path.write_text("openapi: 3.0.0", encoding="utf-8")
        self.assertEqual(validate_schema(path)["status"], "unverified")

    def test_a_malformed_json_contract_is_invalid(self) -> None:
        path = self.repo / "broken.json"
        path.write_text("{not json", encoding="utf-8")
        self.assertEqual(validate_schema(path)["status"], "invalid")

    def test_an_openapi_document_is_recognized_as_a_contract(self) -> None:
        path = self.repo / "public.json"
        path.write_text(json.dumps({"openapi": "3.0.0", "paths": {}}), encoding="utf-8")
        self.assertEqual(
            validate_schema(path),
            {"status": "valid", "kind": "openapi", "path": str(path)},
        )

    def test_the_secret_scanner_finds_each_kind_it_claims_to_find(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        label = "PRIVATE KEY"
        (secrets / "key.pem").write_text(
            f"-----BEGIN RSA {label}-----", encoding="utf-8"
        )
        (secrets / "ci.env").write_text("ghp_" + "a" * 30, encoding="utf-8")
        (secrets / "app.conf").write_text("api_key = " + "b" * 20, encoding="utf-8")
        (secrets / "clean.md").write_text("A token is a word.", encoding="utf-8")
        findings = scan_secrets(secrets)
        self.assertEqual({finding["kind"] for finding in findings}, {"private-key", "github-token", "generic-secret"})
        self.assertEqual(["app.conf", "ci.env", "key.pem"], sorted(finding["path"] for finding in findings))

    def test_a_secret_finding_never_carries_the_secret(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        (secrets / "ci.env").write_text("ghp_" + "c" * 30, encoding="utf-8")
        finding = scan_secrets(secrets)[0]
        self.assertEqual(sorted(finding), ["kind", "line", "path"])
        self.assertNotIn("c" * 30, json.dumps(finding))

    def test_every_parsed_command_has_a_handler_and_every_handler_a_command(self) -> None:
        subcommands = next(
            action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)
        )
        self.assertEqual(set(HANDLERS), set(subcommands.choices))

    def test_migrating_restores_a_missing_asset_without_touching_the_others(
        self,
    ) -> None:
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        use_json_layout(self.repo / "memory")
        (self.repo / "memory" / "registry" / "events.json").unlink()
        created = migrate_registry(self.repo / "memory")
        self.assertEqual(created, ["events.json"])
        record = record_by_id(self.repo / "memory", "contexts", "orders")
        self.assertIsNotNone(record)
        assert record is not None
        self.assertEqual(record["name"], "Orders")

    def test_an_examined_context_leaves_the_gap_list_but_an_unexamined_one_stays(
        self,
    ) -> None:
        self.context_declaring(
            "aggregates",
            "Invariants are thin here; a transaction script is the declared pattern.",
        )
        report = coverage(self.repo / "memory")
        self.assertEqual(report["gaps"]["without_aggregate"], ["billing"])
        self.assertEqual(report["confirmed_absent"]["without_aggregate"], ["orders"])

    def test_an_absence_confirmed_for_one_asset_does_not_answer_another(self) -> None:
        self.context_declaring("aggregates", "Invariants are thin here.")
        report = coverage(self.repo / "memory")
        self.assertIn("orders", report["gaps"]["without_rule"])
        self.assertEqual(report["confirmed_absent"]["without_rule"], [])

    def test_a_confirmed_absence_without_a_reason_is_rejected(self) -> None:
        self.context_declaring("aggregates", "   ")
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(any("without a reason" in error for error in errors), errors)

    def test_a_confirmed_absence_the_records_contradict_is_rejected(self) -> None:
        self.context_declaring(
            "rules", "No business rule is written down for this context."
        )
        self.seed(
            "rules.json",
            [
                {
                    "id": "order-total",
                    "contexts": ["orders"],
                    "statement": "An order total is non-negative.",
                }
            ],
        )
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(
            any("rules records name it" in error for error in errors), errors
        )

    def test_a_confirmed_absence_of_an_unknown_asset_names_the_ones_allowed(
        self,
    ) -> None:
        self.context_declaring("decisions", "Decisions are elsewhere.")
        errors = validate(self.repo / "memory", None, False)
        named = next(error for error in errors if "unknown asset" in error)
        for allowed in ("aggregates", "contracts", "events", "rules", "vocabulary"):
            self.assertIn(allowed, named)

    def test_the_command_line_takes_one_field_or_several_sets_but_not_both(self) -> None:
        def parsed(**values) -> argparse.Namespace:
            return argparse.Namespace(**{"field": None, "value": None, "set": [], **values})

        self.assertEqual(
            [("ci_requirement", "required")],
            policy_assignments(parsed(field="ci_requirement", value="required")),
        )
        self.assertEqual(
            [("review_mode", "scm-verified"), ("authorized_signers", "a=b,c")],
            policy_assignments(parsed(set=["review_mode=scm-verified", "authorized_signers=a=b,c"])),
        )
        for refused in (
            parsed(),
            parsed(field="ci_requirement"),
            parsed(field="ci_requirement", value="required", set=["review_mode=scm-verified"]),
            parsed(set=["review_mode"]),
        ):
            with self.subTest(refused=vars(refused)):
                with self.assertRaises(ValueError):
                    policy_assignments(refused)

    def test_a_policy_can_leave_local_draft_only_for_external_review(self) -> None:
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone until a reviewer exists.",
        )
        with self.assertRaisesRegex(ValueError, "requires the verifier"):
            amend_policy(
                self.repo / "memory",
                "review_mode",
                "scm-verified",
                "A reviewer exists now.",
            )
        self.assertEqual(self.stored_policy()["review_mode"], "local-draft-only")
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "scm-verified",
            "A reviewer exists now.",
            verifier="github-pr",
        )
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")
        self.assertEqual(
            self.stored_policy()["review_governance"]["verifier"], "github-pr"
        )

    def test_a_repository_without_ci_can_adopt_and_rotate_a_signer(self) -> None:
        root = self.repo / "memory"
        amend_policy(
            root, "authorized_signers", "SHA256:first", "Register the maintainer key."
        )
        amend_policy(root, "review_trigger", "git-push", "Signing is enforced on push.")
        amend_policy(
            root,
            "review_verifier",
            "git-signed-commit",
            "No CI reviewer exists, so the maintainer signs instead.",
        )
        governance = self.stored_policy()["review_governance"]
        self.assertEqual("git-signed-commit", governance["verifier"])
        self.assertEqual(["SHA256:first"], governance["authorized_signers"])
        amend_policy(
            root, "authorized_signers", "SHA256:second", "Rotate a compromised key."
        )
        self.assertEqual(
            ["SHA256:second"],
            self.stored_policy()["review_governance"]["authorized_signers"],
        )
        with self.assertRaisesRegex(ValueError, "amended policy is invalid"):
            amend_policy(root, "authorized_signers", "   ", "Leave nobody able to sign.")
        self.assertEqual(
            ["SHA256:second"],
            self.stored_policy()["review_governance"]["authorized_signers"],
        )

    def test_amending_without_a_reason_leaves_the_policy_alone(self) -> None:
        with self.assertRaisesRegex(ValueError, "requires a reason"):
            amend_policy(self.repo / "memory", "review_mode", "local-draft-only", "   ")
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")

    def test_an_unamendable_field_names_the_ones_that_are(self) -> None:
        with self.assertRaises(ValueError) as raised:
            amend_policy(self.repo / "memory", "limits", "0", "Loosen the limits.")
        for amendable in (
            "review_mode",
            "storage_mode",
            "data_classification",
            "source_authority",
        ):
            self.assertIn(amendable, str(raised.exception))

    def test_an_invalid_value_is_refused_before_anything_is_written(self) -> None:
        with self.assertRaisesRegex(ValueError, "amended policy is invalid"):
            amend_policy(
                self.repo / "memory", "review_mode", "whatever-i-like", "Try it."
            )
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")

    def test_verify_evidence_names_the_record_of_each_citation_and_filters_by_path(self) -> None:
        use_json_layout(self.repo / "memory")
        self.seed_two_cited_rules()
        report = verify_evidence(self.repo / "memory", self.repo, ["fact.md"])
        self.assertEqual(
            [("rules", "moved", "shifted"), ("rules", "kept", "current")],
            [(item["asset"], item["id"], item["status"]) for item in report["results"]],
        )
        self.assertEqual(3, len(verify_evidence(self.repo / "memory", self.repo)["results"]))

    def test_a_query_no_term_matches_lists_reviewed_records_that_mention_it(self) -> None:
        self.contexts_mentioning_suggestions()
        self.assertEqual(
            [{"match_kind": "mentioned", "asset": "contexts", "id": "eval", "status": "reviewed", "contexts": ["eval"],
              "summary": "Applies accepted Suggestions.", "fields": ["responsibility"]}],
            resolve_terms(self.repo / "memory", "suggestion", None),
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
             for item in resolve_terms(self.repo / "memory", "withdrawn", None)],
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
            [item["id"] for item in resolve_terms(self.repo / "memory", "takedown", None)],
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
        self.assertEqual([], resolve_terms(self.repo / "memory", "reviewed", None))

    def test_a_matching_term_hides_prose_mentions(self) -> None:
        self.contexts_mentioning_suggestions()
        self.seed("vocabulary.json", [{"id": "suggestion", "name": "Suggestion", "contexts": ["eval"]}])
        self.assertEqual(["name"], [match["match_kind"] for match in resolve_terms(self.repo / "memory", "suggestion", None)])

    def test_with_mentions_a_matching_term_comes_first_and_mentions_follow(self) -> None:
        self.contexts_mentioning_suggestions()
        self.seed("vocabulary.json", [{"id": "suggestion", "name": "Suggestion", "contexts": ["eval"],
                                       "definition": "A suggestion to change a version.", "status": "reviewed"}])
        self.assertEqual(
            [("vocabulary", "suggestion", "name"), ("contexts", "eval", "mentioned")],
            [(match.get("asset", "vocabulary"), match["id"], match["match_kind"])
             for match in resolve_terms(self.repo / "memory", "suggestion", None, with_mentions=True)],
        )

    def test_a_record_id_is_searched_word_by_word(self) -> None:
        self.allow_results(10)
        self.seed("interactions.json", [{"id": "eval-workspace-purge", "status": "reviewed"}])
        self.assertEqual(
            [("interactions", "eval-workspace-purge", ["id"])],
            [(match["asset"], match["id"], match["fields"]) for match in resolve_terms(self.repo / "memory", "purge", None)],
        )

    def test_a_list_of_text_is_searched_and_a_list_of_record_ids_is_not(self) -> None:
        self.allow_results(10)
        self.seed("vocabulary.json", [{"id": "order", "name": "Order", "examples": ["a refund request"], "status": "reviewed"}])
        self.seed("rules.json", [{"id": "gate", "contexts": ["refund"], "statement": "x", "status": "reviewed"}])
        self.assertEqual(
            [("vocabulary", "order", ["examples"])],
            [(match["asset"], match["id"], match["fields"]) for match in resolve_terms(self.repo / "memory", "refund", None)],
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
            [(match["asset"], match["id"], match["fields"]) for match in resolve_terms(self.repo / "memory", "check", None)],
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
            [(match["id"], match["fields"]) for match in resolve_terms(self.repo / "memory", "purge", None)],
        )

    def test_a_record_mentioned_in_one_defining_field_ranks_before_a_rule_mentioned_only_elsewhere(self) -> None:
        self.allow_results(10)
        self.seed("capabilities.json", [{"id": "purge", "rationale": "Purge on request.", "status": "reviewed"}])
        self.seed("rules.json", [{"id": "gate", "contexts": ["identity"], "statement": "x", "notes": "purge",
                                  "status": "reviewed"}])
        self.assertEqual(
            ["purge", "gate"],
            [match["id"] for match in resolve_terms(self.repo / "memory", "purge", None)],
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
            [(match["asset"], match["id"], match["fields"]) for match in resolve_terms(self.repo / "memory", "concurrent runs", None)],
        )

    def test_mentions_in_rules_come_before_mentions_in_other_assets(self) -> None:
        self.allow_results(10)
        self.seed("interactions.json", [{"id": "eval-workspace-purge", "status": "reviewed"}])
        self.seed("rules.json", [{"id": "purge-runs-every-step", "contexts": ["identity"], "statement": "x", "status": "reviewed"}])
        self.assertEqual(
            [("rules", "purge-runs-every-step"), ("interactions", "eval-workspace-purge")],
            [(match["asset"], match["id"]) for match in resolve_terms(self.repo / "memory", "purge", None)],
        )

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

    def test_a_query_that_matches_nothing_finds_nothing(self) -> None:
        self.two_contexts()
        self.allow_results(10)
        self.assertEqual([], lookup(self.repo / "memory", "contexts", "shipping"))

    def test_a_query_matches_regardless_of_letter_case(self) -> None:
        self.two_contexts()
        self.allow_results(10)
        found = lookup(self.repo / "memory", "contexts", "OWN INVOICES")
        self.assertEqual(["billing"], [entry["id"] for entry in found])

    def test_looking_up_an_unknown_asset_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "unknown asset: nonsense"):
            lookup(self.repo / "memory", "nonsense", "")

    def test_a_result_limit_equal_to_the_match_count_returns_every_match(self) -> None:
        use_json_layout(self.repo / "memory")
        self.two_contexts()
        self.allow_results(2)
        found = lookup(self.repo / "memory", "contexts", "")
        self.assertEqual(["orders", "billing"], [entry["id"] for entry in found])

    def test_without_a_policy_file_the_context_model_is_working_memory(self) -> None:
        self.two_contexts()
        (self.repo / "memory" / "domain-memory-policy.json").unlink()
        model = context_model(self.repo / "memory", "orders")
        assert model is not None
        self.assertEqual(("working-memory", "legacy-unverified"), (model["usage"], model["review_mode"]))

    def test_reviewed_validation_without_a_policy_file_cannot_be_satisfied(self) -> None:
        (self.repo / "memory" / "domain-memory-policy.json").unlink()
        self.assertEqual(
            ["local-draft-only Domain Memory cannot satisfy --require-reviewed"],
            validate(self.repo / "memory", None, True),
        )

    def test_a_context_model_gathers_each_section_from_the_records_that_name_the_context(self) -> None:
        use_json_layout(self.repo / "memory")
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
            "vocabulary": ["order", "ledger-entry"],
            "aggregates": ["order-aggregate"],
            "rules": ["order-total", "shared-currency"],
            "interactions": ["order-billed", "invoice-paid"],
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

    def test_each_asset_level_defect_is_named(self) -> None:
        use_json_layout(self.repo / "memory")
        registry = self.repo / "memory" / "registry"
        events = registry / "events.json"

        def break_format(value: dict) -> None:
            value["format"] = "domain-contexts/v0"

        def break_status(value: dict) -> None:
            value["status"] = "pending"

        def drop_events_from_manifest(value: dict) -> None:
            value["artifacts"] = [name for name in value["artifacts"] if name != "events.json"]

        def manifest_without_a_list(value: dict) -> None:
            value["artifacts"] = "all"

        cases = [
            ("invalid format", "contexts.json", break_format, ["contexts.json has an invalid format"]),
            ("invalid status", "contexts.json", break_status, ["contexts.json has an invalid status"]),
            ("manifest lacks an asset", "manifest.json", drop_events_from_manifest,
             ["manifest artifacts must list every standard registry asset file"]),
            ("manifest artifacts not a list", "manifest.json", manifest_without_a_list,
             ["manifest artifacts must list every standard registry asset file"]),
        ]
        pristine = {path.name: path.read_text(encoding="utf-8") for path in registry.iterdir()}
        for label, name, change, expected in cases:
            with self.subTest(label):
                for asset, text in pristine.items():
                    (registry / asset).write_text(text, encoding="utf-8")
                self.edit_registry(name, change)
                self.assertEqual(expected, validate(self.repo / "memory", None, False))
        with self.subTest("missing asset file"):
            for asset, text in pristine.items():
                (registry / asset).write_text(text, encoding="utf-8")
            events.unlink()
            self.assertEqual([f"missing asset: {events}"], validate(self.repo / "memory", None, False))


if __name__ == "__main__":
    unittest.main()
