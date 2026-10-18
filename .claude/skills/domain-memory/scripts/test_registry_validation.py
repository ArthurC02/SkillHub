from __future__ import annotations

import argparse
import json
import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.common import ASSET_KEYS, load_json
from domain_registry.contracts import validate_schema
from domain_registry.cli_commands import HANDLERS
from domain_registry.cli_parser import build_parser
from domain_registry.registry import lookup, migrate_evidence, validate, verify_evidence
from domain_registry.store import read_document, write_document
from test_support import VERIFIED, RegistryTestCase, append_audit, scan_secrets, use_json_layout


class RegistryValidationTest(RegistryTestCase):
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

    def test_json_loader_refuses_nesting_deeper_than_the_parser_can_follow_naming_the_file(self) -> None:
        path = self.repo / "nested.json"
        path.write_text("[" * 200_000, encoding="utf-8")
        with self.assertRaisesRegex(ValueError, f"^invalid JSON: {re.escape(str(path))}$"):
            load_json(path)

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


if __name__ == "__main__":
    unittest.main()
