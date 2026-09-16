from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import init_change_package
from domain_registry.changes import validate_change_package
from domain_registry.hitl import finalize_proposal, record_approval, submit_proposal, supersede_proposal, verify_proposal
from domain_registry.registry import init_registry
from domain_registry.registry import migrate_registry
from domain_registry.policy import validate_policy
from domain_registry.transaction import recover_interrupted_update, transaction_path
from domain_registry.registry import lookup
from domain_registry.registry import boundary_analysis, context_model, coverage, record_by_id, resolve_terms, validate
from domain_registry.common import ASSET_KEYS
from domain_registry.contracts import validate_schema
from domain_registry.security import scan as scan_secrets
from domain_registry.registry import migrate_evidence, verify_evidence
from domain_registry.revision import current_registry_revision, require_current_registry_revision
from domain_registry.sources import confirmed_source_map, probe_sources, source_files, source_kind_for, source_policy_report, verify_source_map, write_source_map
from domain_registry.sources import test_locations as discovered_test_locations
from domain_registry.updates import apply_approved_updates, upsert_candidate
from domain_registry.attestations import verify_scm
from domain_registry.audit import append as append_audit, verify as verify_audit
from domain_registry.evidence import digest, verify


class DomainRegistryTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.repo = Path(self.temporary.name)
        init_registry(self.repo / "memory")
        (self.repo / "memory" / "domain-memory-policy.json").write_text(json.dumps({"format":"domain-memory-policy/v1","storage_mode":"tracked","data_classification":"internal","review_mode":"scm-verified","source_policy":{"selected_paths":["docs"],"include":["**"],"exclude":[],"authority":"test"},"limits":{"max_file_count":1,"max_file_bytes":1,"max_total_bytes":1,"max_query_results":1},"approved_command_profiles":["unit"]}), encoding="utf-8")
        subprocess.run(["git", "init", "-q"], cwd=self.repo, check=True)
        subprocess.run(["git", "config", "core.autocrlf", "false"], cwd=self.repo, check=True)
        subprocess.run(["git", "add", "."], cwd=self.repo, check=True)
        subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "seed"], cwd=self.repo, check=True)

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def commit_all(self, message: str) -> None:
        subprocess.run(["git", "add", "-A"], cwd=self.repo, check=True)
        subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", message], cwd=self.repo, check=True)

    def committed_source_map(self) -> Path:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        self.commit_all("sources")
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, confirmed_source_map(self.repo, [self.repo / "docs"]))
        self.commit_all("source map")
        return path

    def write_record(self, name: str, value: dict) -> Path:
        path = self.repo / name
        path.write_text(json.dumps(value), encoding="utf-8")
        return path

    def seed(self, name: str, records: list[dict]) -> None:
        path = self.repo / "memory" / "registry" / name
        value = json.loads(path.read_text(encoding="utf-8"))
        value[ASSET_KEYS[name]] = records
        path.write_text(json.dumps(value), encoding="utf-8")

    def allow_results(self, count: int) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        value = json.loads(path.read_text(encoding="utf-8"))
        value["limits"]["max_query_results"] = count
        path.write_text(json.dumps(value), encoding="utf-8")

    def two_contexts(self) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
        ])

    def add_context(self) -> None:
        record = self.write_record("context.json", {"id": "orders", "name": "Orders", "responsibility": "Own orders."})
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def test_changed_registry_rejects_a_captured_revision(self) -> None:
        expected = current_registry_revision(self.repo / "memory", self.repo)
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["note"] = "changed"
        document.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "stale"):
            require_current_registry_revision({"base_registry_revision": expected}, self.repo / "memory", self.repo)

    def test_invalid_candidate_does_not_change_registry(self) -> None:
        before = (self.repo / "memory" / "registry" / "vocabulary.json").read_text(encoding="utf-8")
        record = self.write_record("term.json", {"id": "order", "name": "Order", "definition": "A purchase.", "contexts": ["missing"]})
        with self.assertRaisesRegex(ValueError, "unknown context"):
            upsert_candidate(self.repo / "memory", self.repo, "vocabulary", record)
        self.assertEqual(before, (self.repo / "memory" / "registry" / "vocabulary.json").read_text(encoding="utf-8"))

    def test_source_snapshot_detects_drift(self) -> None:
        source = self.repo / "docs"
        source.mkdir()
        (source / "rules.md").write_text("first version\n", encoding="utf-8")
        source_map = confirmed_source_map(self.repo, [source], confirmed_by="Test Developer")
        source_map_path = self.repo / "memory" / "source-map.json"
        write_source_map(source_map_path, source_map)
        self.assertEqual("current", verify_source_map(self.repo, source_map_path)["status"])
        (source / "rules.md").write_text("revised version\n", encoding="utf-8")
        self.assertEqual("stale", verify_source_map(self.repo, source_map_path)["status"])

    def test_source_policy_excludes_files_and_enforces_limits(self) -> None:
        source = self.repo / "docs"
        source.mkdir()
        (source / "included.md").write_text("one", encoding="utf-8")
        (source / "private.md").write_text("two", encoding="utf-8")
        policy = {"source_policy": {"include": ["docs/**"], "exclude": ["docs/private.md"]}, "limits": {"max_file_count": 1, "max_file_bytes": 10, "max_total_bytes": 10}}
        report = source_policy_report(self.repo, [source], policy)
        self.assertEqual(1, report["files"])
        self.assertEqual([], report["errors"])
        source_map = confirmed_source_map(self.repo, [source], policy, confirmed_by="Test Developer")
        self.assertEqual("current", verify_source_map(self.repo, self.write_record("source-map.json", source_map), policy)["status"])
        (source / "included.md").write_text("too many bytes", encoding="utf-8")
        self.assertEqual("invalid", verify_source_map(self.repo, self.repo / "source-map.json", policy)["status"])

    def test_lookup_honors_policy_result_limit(self) -> None:
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["contexts"] = [{"id": "one", "name": "One", "responsibility": "First."}, {"id": "two", "name": "Two", "responsibility": "Second."}]
        document.write_text(json.dumps(value), encoding="utf-8")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["limits"]["max_query_results"] = 1
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        self.assertEqual(1, len(lookup(self.repo / "memory", "contexts", "")))

    def test_migrate_evidence_upgrades_existing_line_reference(self) -> None:
        (self.repo / "source.md").write_text("evidence\n", encoding="utf-8")
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["contexts"] = [{"id": "orders", "name": "Orders", "responsibility": "Own orders.", "evidence": ["source.md:1"]}]
        document.write_text(json.dumps(value), encoding="utf-8")
        self.assertEqual(1, migrate_evidence(self.repo / "memory", self.repo))
        self.assertEqual(1, verify_evidence(self.repo / "memory", self.repo)["summary"]["current"])

    def test_scm_attestation_requires_the_same_revision(self) -> None:
        proposal = {"proposal_revision": 2, "base_registry_revision": {"registry_digest": "sha256:" + "0" * 64, "observed_commit": None}}
        path = self.write_record("attestation.json", {"provider": "github", "pull_request": "1", "commit": "abc", "status": "approved", "proposal_revision": 2, "base_registry_revision": proposal["base_registry_revision"]})
        self.assertEqual([], verify_scm(path, proposal))
        value = json.loads(path.read_text(encoding="utf-8"))
        value["proposal_revision"] = 1
        path.write_text(json.dumps(value), encoding="utf-8")
        self.assertTrue(verify_scm(path, proposal))

    def test_audit_chain_detects_tampering(self) -> None:
        append_audit(self.repo / "memory", {"operation": "first"})
        append_audit(self.repo / "memory", {"operation": "second"})
        self.assertEqual("valid", verify_audit(self.repo / "memory")["status"])
        path = self.repo / "memory" / "audit" / "events.jsonl"
        path.write_text(path.read_text(encoding="utf-8").replace("second", "changed"), encoding="utf-8")
        self.assertEqual("invalid", verify_audit(self.repo / "memory")["status"])

    def test_structured_evidence_detects_source_drift(self) -> None:
        source = self.repo / "evidence.md"
        source.write_text("one\ntwo\n", encoding="utf-8")
        content = source.read_bytes()
        excerpt = "".join(content.decode("utf-8").splitlines(keepends=True)[1:2]).encode("utf-8")
        reference = {"path": "evidence.md", "lines": {"start": 2, "end": 2}, "content_sha256": digest(content), "excerpt_sha256": digest(excerpt)}
        self.assertEqual("current", verify(reference, self.repo)["status"])
        source.write_text("one\nchanged\n", encoding="utf-8")
        self.assertEqual("stale", verify(reference, self.repo)["status"])

    def draft_package(self) -> Path:
        self.add_context()
        package = self.repo / "proposal"
        init_change_package(package)
        requirement = {
            "requirement_id": "REQ-1",
            "acceptance_criteria": [{"id": "AC-1", "statement": "Orders have a total."}],
            "risk_flags": [],
            "required_approval_roles": ["domain-owner"],
        }
        proposal = {
            "proposal_id": "PRO-1",
            "proposer": "author",
            "requirement_id": "REQ-1",
            "status": "draft",
            "submitted_at": None,
            "finalized_at": None,
            "proposal_revision": 1,
            "base_registry_revision": None,
            "affected_contexts": ["orders"],
            "rule_ids": [],
            "contract_ids": [],
            "ownership_decision": {"owner_context": "orders", "invariant": "Order total is non-negative.", "evidence": []},
            "boundary_decision": {"interaction": "none", "consistency": "synchronous", "prohibited_dependencies": [], "evidence": []},
            "event_change": None,
            "contract_change": None,
            "registry_updates": [{"operation": "upsert", "asset": "rules", "record": {"id": "order-total", "contexts": ["orders"], "statement": "An order total is non-negative.", "evidence": ["README.md:1"]}}],
            "risk_flags": [],
            "open_questions": [],
            "approvals": [],
        }
        obligations = {"requirement_id": "REQ-1", "proposal_id": "PRO-1", "obligations": [{"id": "OB-1", "source_type": "acceptance-criterion", "source_id": "AC-1", "expected_outcome": "A negative total is rejected.", "check": "test command", "status": "executed"}]}
        evidence = {"requirement_id": "REQ-1", "proposal_id": "PRO-1", "registry_revision": None, "approvals": [], "scm_attestation": None, "sources": [], "test_results": [{"obligation_id": "OB-1", "status": "passed", "evidence": "test output", "command_profile":"unit","exit_code":0,"output_sha256":"sha256:" + "0" * 64,"finished_at":"2026-01-01T00:00:00Z"}], "contract_checks": [], "architecture_checks": [], "security_checks": [], "residual_risks": []}
        for name, value in (("requirement-normalization.json", requirement), ("domain-change-proposal.json", proposal), ("test-obligations.json", obligations), ("evidence-bundle.json", evidence)):
            (package / name).write_text(json.dumps(value), encoding="utf-8")
        return package

    def written_package(self, **replacements) -> Path:
        package = self.draft_package()
        for name, value in replacements.items():
            path = package / (name.replace("_", "-") + ".json")
            path.write_text(json.dumps(value), encoding="utf-8")
        return package

    def test_approved_package_applies_against_its_exact_revision(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        proposal_path = package / "domain-change-proposal.json"
        submitted = json.loads(proposal_path.read_text(encoding="utf-8"))
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["scm_attestation"] = {"provider":"test","pull_request":"1","commit":"abc","status":"approved","proposal_revision":1,"base_registry_revision":submitted["base_registry_revision"]}
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        verify_proposal(package, self.repo / "memory", self.repo)
        finalize_proposal(package, self.repo / "memory", self.repo)
        apply_approved_updates(package, self.repo / "memory", self.repo)
        rules = json.loads((self.repo / "memory" / "registry" / "rules.json").read_text(encoding="utf-8"))
        self.assertEqual("reviewed", rules["rules"][0]["status"])
        applied = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("applied", applied["status"])
        self.assertIn("registry_digest", applied["applied_registry_revision"])
        self.assertEqual([], validate_change_package(package, self.repo / "memory"))


    def test_oversized_source_names_the_file_and_the_limit(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "big.md").write_text("x" * 64, encoding="utf-8")
        policy = json.loads((self.repo / "memory" / "domain-memory-policy.json").read_text(encoding="utf-8"))
        policy["limits"] = {"max_file_count": 100, "max_file_bytes": 16, "max_total_bytes": 10000, "max_query_results": 1}
        report = source_policy_report(self.repo, [self.repo / "docs"], policy)
        self.assertEqual(report["oversized_files"], ["docs/big.md"])
        self.assertEqual(len(report["errors"]), 1)
        self.assertIn("docs/big.md", report["errors"][0])
        self.assertIn("16", report["errors"][0])

    def test_a_gitignored_file_is_not_a_source(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        (self.repo / "docs" / "build.out").write_text("generated", encoding="utf-8")
        (self.repo / ".gitignore").write_text("*.out" + chr(10), encoding="utf-8")
        found = source_files(self.repo, [self.repo / "docs"])
        self.assertEqual([path.name for path in found], ["kept.md"])

    def test_sources_stay_agent_asserted_until_a_developer_is_named(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        asserted = confirmed_source_map(self.repo, [self.repo / "docs"])
        self.assertEqual(asserted["selection_status"], "agent-asserted")
        self.assertNotIn("confirmed_by", asserted)
        confirmed = confirmed_source_map(self.repo, [self.repo / "docs"], confirmed_by="Arthur")
        self.assertEqual(confirmed["selection_status"], "developer-confirmed")
        self.assertEqual(confirmed["confirmed_by"], "Arthur")

    def test_an_agent_asserted_map_still_reports_whether_its_sources_moved(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        kept = self.repo / "docs" / "kept.md"
        kept.write_text("kept", encoding="utf-8")
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, confirmed_source_map(self.repo, [self.repo / "docs"]))
        fresh = verify_source_map(self.repo, path)
        self.assertEqual(fresh["status"], "current")
        self.assertEqual(fresh["selection_status"], "agent-asserted")
        kept.write_text("edited", encoding="utf-8")
        self.assertEqual(verify_source_map(self.repo, path)["status"], "stale")

    def test_a_map_without_snapshots_cannot_answer_the_question(self) -> None:
        path = self.write_record("source-map.json", {"selected_paths": [], "selection_status": "developer-confirmed"})
        self.assertEqual(verify_source_map(self.repo, path)["status"], "unverified")

    def test_a_clean_repo_answers_the_probe_without_hashing_anything(self) -> None:
        result = probe_sources(self.repo, self.committed_source_map())
        self.assertEqual(result["status"], "current")
        self.assertEqual(result["checked"], "git")

    def test_an_uncommitted_source_sends_the_probe_back_to_hashing(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "kept.md").write_text("edited", encoding="utf-8")
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "stale")

    def test_a_committed_edit_to_a_source_is_reported_as_stale(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "kept.md").write_text("edited", encoding="utf-8")
        self.commit_all("edit a source")
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "stale")

    def test_a_map_without_a_recorded_git_state_is_probed_by_hashing(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        source_map = confirmed_source_map(self.repo, [self.repo / "docs"])
        del source_map["git_state"]
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, source_map)
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "current")

    def nested_sources(self) -> dict:
        (self.repo / "apps" / "svc").mkdir(parents=True, exist_ok=True)
        (self.repo / "apps" / "other.go").write_text("other", encoding="utf-8")
        (self.repo / "apps" / "svc" / "doc.go").write_text("package svc", encoding="utf-8")
        return confirmed_source_map(self.repo, [self.repo / "apps", self.repo / "apps" / "svc" / "doc.go"])

    def test_a_nested_source_owns_its_files_instead_of_its_parent(self) -> None:
        source_map = self.nested_sources()
        counts = {snapshot["path"]: snapshot["file_count"] for snapshot in source_map["source_snapshots"]}
        self.assertEqual(counts["apps/svc/doc.go"], 1)
        self.assertEqual(counts["apps"], 1)

    def test_editing_a_nested_source_leaves_its_parent_current(self) -> None:
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, self.nested_sources())
        (self.repo / "apps" / "svc" / "doc.go").write_text("package svc // revised", encoding="utf-8")
        result = verify_source_map(self.repo, path)
        self.assertEqual(result["status"], "stale")
        self.assertEqual([source["path"] for source in result["changed_sources"]], ["apps/svc/doc.go"])

    def test_the_most_specific_source_decides_what_a_citation_establishes(self) -> None:
        source_map = self.nested_sources()
        self.assertEqual(source_kind_for(source_map, "apps/svc/doc.go"), "bounded_contexts")
        self.assertEqual(source_kind_for(source_map, "apps/other.go"), "implementation")

    def test_a_citation_outside_every_selected_source_is_unclassified(self) -> None:
        self.assertEqual(source_kind_for(self.nested_sources(), "infra/node.txt"), "unclassified")

    def test_an_implementation_directory_is_not_reported_as_a_test_location(self) -> None:
        (self.repo / "apps" / "svc").mkdir(parents=True, exist_ok=True)
        (self.repo / "apps" / "svc" / "thing_test.go").write_text("x", encoding="utf-8")
        (self.repo / "tests").mkdir(exist_ok=True)
        (self.repo / "tests" / "e2e_test.go").write_text("x", encoding="utf-8")
        self.assertEqual(discovered_test_locations(self.repo), ["tests"])

    def test_stored_evidence_records_the_kind_of_source_it_came_from(self) -> None:
        decisions = self.repo / "docs" / "adr"
        decisions.mkdir(parents=True, exist_ok=True)
        decision = decisions / "boundaries.md"
        decision.write_text("Orders owns pricing.", encoding="utf-8")
        write_source_map(self.repo / "memory" / "source-map.json", confirmed_source_map(self.repo, [decisions]))
        content = decision.read_bytes()
        record = self.write_record("context.json", {
            "id": "orders", "name": "Orders", "responsibility": "Own orders.",
            "evidence": [{"path": "docs/adr/boundaries.md", "lines": {"start": 1, "end": 1},
                          "content_sha256": digest(content), "excerpt_sha256": digest(content)}],
        })
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        stored = json.loads((self.repo / "memory" / "registry" / "contexts.json").read_text(encoding="utf-8"))
        self.assertEqual(stored["contexts"][0]["evidence"][0]["source_kind"], "decisions")

    def test_a_probe_of_an_uninitialized_repository_says_so(self) -> None:
        result = probe_sources(self.repo, self.repo / "nowhere" / "source-map.json")
        self.assertEqual(result["status"], "absent")

    def test_resolve_terms_reads_the_definition_and_not_the_rest_of_the_record(self) -> None:
        self.two_contexts()
        self.seed("vocabulary.json", [
            {"id": "order", "name": "Order", "definition": "A confirmed purchase.", "contexts": ["orders"]},
            {"id": "invoice", "name": "Invoice", "definition": "A demand for payment.", "contexts": ["billing"],
             "note": "A confirmed purchase becomes this."},
        ])
        self.allow_results(10)
        found = [term["id"] for term in resolve_terms(self.repo / "memory", "confirmed purchase", None)]
        self.assertEqual(found, ["order"])

    def test_resolve_terms_matches_regardless_of_letter_case(self) -> None:
        self.two_contexts()
        self.seed("vocabulary.json", [{"id": "order", "name": "Order", "definition": "A purchase.", "contexts": ["orders"]}])
        self.assertEqual(len(resolve_terms(self.repo / "memory", "ORDER", None)), 1)

    def test_resolve_terms_excludes_a_term_that_does_not_belong_to_the_named_context(self) -> None:
        self.two_contexts()
        self.seed("vocabulary.json", [{"id": "order", "name": "Order", "definition": "A purchase.", "contexts": ["orders"]}])
        self.assertEqual(resolve_terms(self.repo / "memory", "order", "orders")[0]["id"], "order")
        self.assertEqual(resolve_terms(self.repo / "memory", "order", "billing"), [])

    def test_resolve_terms_stops_at_the_policy_result_limit(self) -> None:
        self.two_contexts()
        self.seed("vocabulary.json", [
            {"id": "order", "name": "Order", "definition": "A purchase.", "contexts": ["orders"]},
            {"id": "order-line", "name": "Order line", "definition": "A purchase line.", "contexts": ["orders"]},
        ])
        self.allow_results(1)
        self.assertEqual(len(resolve_terms(self.repo / "memory", "purchase", None)), 1)

    def test_an_absent_record_is_reported_as_absent_rather_than_raised(self) -> None:
        self.two_contexts()
        self.assertEqual(record_by_id(self.repo / "memory", "contexts", "orders")["name"], "Orders")
        self.assertIsNone(record_by_id(self.repo / "memory", "contexts", "shipping"))

    def test_an_unknown_context_has_no_model(self) -> None:
        self.two_contexts()
        self.assertIsNone(context_model(self.repo / "memory", "shipping"))

    def test_a_context_model_carries_the_contracts_it_consumes_as_well_as_those_it_produces(self) -> None:
        self.two_contexts()
        self.seed("contracts.json", [
            {"id": "invoice-api", "producer_context": "billing", "consumer_contexts": ["orders"]},
            {"id": "order-api", "producer_context": "orders", "consumer_contexts": []},
            {"id": "shipping-api", "producer_context": "billing", "consumer_contexts": []},
        ])
        model = context_model(self.repo / "memory", "orders")
        self.assertEqual(sorted(entry["id"] for entry in model["contracts"]), ["invoice-api", "order-api"])

    def test_two_contexts_without_registered_collaboration_say_so(self) -> None:
        self.two_contexts()
        self.assertEqual(boundary_analysis(self.repo / "memory", "orders", "billing")["status"], "no_registered_collaboration")

    def test_a_registered_interaction_makes_a_boundary_known(self) -> None:
        self.two_contexts()
        self.seed("interactions.json", [{"id": "order-billed", "producer_context": "orders", "consumer_context": "billing"}])
        self.assertEqual(boundary_analysis(self.repo / "memory", "orders", "billing")["status"], "known")

    def test_a_registered_interaction_does_not_make_the_reverse_direction_known(self) -> None:
        self.two_contexts()
        self.seed("interactions.json", [{"id": "order-billed", "producer_context": "orders", "consumer_context": "billing"}])
        self.assertEqual(boundary_analysis(self.repo / "memory", "billing", "orders")["status"], "no_registered_collaboration")

    def test_a_dependency_policy_alone_does_not_register_a_collaboration(self) -> None:
        self.two_contexts()
        self.seed("dependency-policies.json", [{"id": "orders-billing", "from_context": "orders", "to_context": "billing"}])
        analysis = boundary_analysis(self.repo / "memory", "orders", "billing")
        self.assertEqual(len(analysis["dependencies"]), 1)
        self.assertEqual(analysis["status"], "no_registered_collaboration")

    def test_coverage_reports_a_context_only_under_the_asset_it_lacks(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.seed("vocabulary.json", [{"id": "order", "name": "Order", "definition": "A purchase.", "contexts": ["orders"]}])
        gaps = coverage(self.repo / "memory")["gaps"]
        self.assertEqual(gaps["without_vocabulary"], [])
        self.assertEqual(gaps["without_aggregate"], ["orders"])
        self.assertEqual(gaps["without_rule"], ["orders"])
        self.assertEqual(gaps["without_contract"], ["orders"])

    def test_requiring_reviewed_records_rejects_a_candidate_registry(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.assertEqual(validate(self.repo / "memory", None, False), [])
        errors = validate(self.repo / "memory", None, True)
        self.assertTrue(any("contexts.json:orders is not reviewed" in error for error in errors))

    def test_a_reviewed_record_without_evidence_is_rejected(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders.",
                                     "status": "reviewed", "review": {"proposal_id": "p-1"}}])
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(any("contexts.json:orders is reviewed but lacks evidence" in error for error in errors))

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
        self.assertEqual(validate_schema(path), {"status": "valid", "kind": "openapi", "path": str(path)})

    def test_the_secret_scanner_finds_each_kind_it_claims_to_find(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        (secrets / "key.pem").write_text("-----BEGIN RSA PRIVATE KEY-----", encoding="utf-8")
        (secrets / "ci.env").write_text("ghp_" + "a" * 30, encoding="utf-8")
        (secrets / "app.conf").write_text("api_key = " + "b" * 20, encoding="utf-8")
        (secrets / "clean.md").write_text("A token is a word.", encoding="utf-8")
        found = {finding["kind"] for finding in scan_secrets(secrets)}
        self.assertEqual(found, {"private-key", "github-token", "generic-secret"})

    def test_a_secret_finding_never_carries_the_secret(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        (secrets / "ci.env").write_text("ghp_" + "c" * 30, encoding="utf-8")
        finding = scan_secrets(secrets)[0]
        self.assertEqual(sorted(finding), ["kind", "line", "path"])
        self.assertNotIn("c" * 30, json.dumps(finding))


    def run_cli(self, *arguments: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", *arguments],
            cwd=Path(__file__).parent, capture_output=True, text=True,
        )

    def test_the_module_entry_point_runs_the_command_instead_of_exiting_quietly(self) -> None:
        result = self.run_cli("coverage", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["contexts"], 0)

    def test_initializing_into_an_occupied_destination_is_refused(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "rules.md").write_text("rules", encoding="utf-8")
        result = self.run_cli(
            "init-domain-memory", "--repo-root", str(self.repo), "--output", str(self.repo / "memory"),
            "--source", str(self.repo / "docs"), "--storage-mode", "ignored",
            "--data-classification", "internal", "--review-mode", "local-draft-only",
            "--source-authority", "test",
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("not empty", result.stdout)

    def test_initializing_a_fresh_destination_writes_the_registry_and_its_policy(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "rules.md").write_text("rules", encoding="utf-8")
        destination = self.repo / "domain-memory"
        result = self.run_cli(
            "init-domain-memory", "--repo-root", str(self.repo), "--output", str(destination),
            "--source", str(self.repo / "docs"), "--storage-mode", "ignored",
            "--data-classification", "internal", "--review-mode", "local-draft-only",
            "--source-authority", "test",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((destination / "source-map.json").is_file())
        self.assertTrue((destination / "domain-memory-policy.json").is_file())
        self.assertTrue((destination / "registry" / "contexts.json").is_file())

    def test_a_policy_is_rejected_one_step_past_every_limit(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        valid = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual(validate_policy(valid), [])
        for key in ("max_file_count", "max_file_bytes", "max_total_bytes", "max_query_results"):
            broken = json.loads(json.dumps(valid))
            broken["limits"][key] = 0
            self.assertEqual(validate_policy(broken), ["policy has invalid limits"], key)

    def test_a_policy_without_a_named_source_authority_is_rejected(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        broken = json.loads(path.read_text(encoding="utf-8"))
        broken["source_policy"]["authority"] = "   "
        self.assertIn("policy requires selected source paths and authority", validate_policy(broken))

    def test_an_unknown_storage_mode_is_rejected(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        broken = json.loads(path.read_text(encoding="utf-8"))
        broken["storage_mode"] = "somewhere-else"
        self.assertIn("policy has an invalid storage_mode", validate_policy(broken))

    def test_migrating_restores_a_missing_asset_without_touching_the_others(self) -> None:
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        (self.repo / "memory" / "registry" / "events.json").unlink()
        created = migrate_registry(self.repo / "memory")
        self.assertEqual(created, ["events.json"])
        self.assertEqual(record_by_id(self.repo / "memory", "contexts", "orders")["name"], "Orders")

    def test_recovery_restores_the_registry_a_crash_left_in_the_backup(self) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-abc"
        (root / "registry").replace(backup)
        transaction_path(root).write_text(json.dumps({"backup": backup.name, "staging": ".domain-registry-stage-abc"}), encoding="utf-8")
        recover_interrupted_update(root, False)
        self.assertTrue((root / "registry" / "contexts.json").is_file())
        self.assertFalse(transaction_path(root).exists())

    def test_recovery_discards_a_backup_once_the_registry_is_already_in_place(self) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-abc"
        shutil.copytree(root / "registry", backup)
        transaction_path(root).write_text(json.dumps({"backup": backup.name, "staging": ".domain-registry-stage-abc"}), encoding="utf-8")
        recover_interrupted_update(root, False)
        self.assertFalse(backup.exists())
        self.assertTrue((root / "registry" / "contexts.json").is_file())

    def test_a_recovery_journal_pointing_outside_the_registry_is_refused(self) -> None:
        root = self.repo / "memory"
        transaction_path(root).write_text(json.dumps({"backup": "../.domain-registry-backup-abc", "staging": ".domain-registry-stage-abc"}), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "unsafe path"):
            recover_interrupted_update(root, False)

    def test_a_malformed_recovery_journal_is_refused(self) -> None:
        root = self.repo / "memory"
        transaction_path(root).write_text(json.dumps({"backup": 7}), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "malformed"):
            recover_interrupted_update(root, False)

    def test_recovery_refuses_to_break_a_live_writer_lock_without_force(self) -> None:
        root = self.repo / "memory"
        (root / ".domain-registry.lock").mkdir()
        with self.assertRaisesRegex(ValueError, "confirm the writer stopped"):
            recover_interrupted_update(root, False)
        recover_interrupted_update(root, True)
        self.assertFalse((root / ".domain-registry.lock").exists())


    def test_an_obligation_source_type_error_names_the_types_that_are_allowed(self) -> None:
        package = self.written_package(test_obligations={
            "requirement_id": "REQ-1", "proposal_id": "PRO-1",
            "obligations": [{"id": "OB-1", "source_type": "architecture", "source_id": "AC-1",
                             "expected_outcome": "A negative total is rejected.", "check": "test command", "status": "executed"}],
        })
        errors = validate_change_package(package, self.repo / "memory")
        named = next(error for error in errors if error.startswith("test obligation OB-1"))
        self.assertIn("'architecture'", named)
        for allowed in ("acceptance-criterion", "rule", "contract", "invariant"):
            self.assertIn(allowed, named)

    def test_an_incomplete_source_id_is_reported_separately_from_an_unknown_source_type(self) -> None:
        package = self.written_package(test_obligations={
            "requirement_id": "REQ-1", "proposal_id": "PRO-1",
            "obligations": [{"id": "OB-1", "source_type": "acceptance-criterion", "source_id": "<stable-id>",
                             "expected_outcome": "A negative total is rejected.", "check": "test command", "status": "executed"}],
        })
        errors = validate_change_package(package, self.repo / "memory")
        named = next(error for error in errors if error.startswith("test obligation OB-1"))
        self.assertIn("incomplete source_id", named)
        self.assertNotIn("source_type", named)

    def test_an_uncovered_acceptance_criterion_is_told_what_to_add(self) -> None:
        package = self.written_package(test_obligations={
            "requirement_id": "REQ-1", "proposal_id": "PRO-1",
            "obligations": [{"id": "OB-1", "source_type": "invariant", "source_id": "PRO-1",
                             "expected_outcome": "A negative total is rejected.", "check": "test command", "status": "executed"}],
        })
        errors = validate_change_package(package, self.repo / "memory")
        named = next(error for error in errors if error.startswith("acceptance criterion AC-1"))
        self.assertIn("source_type is acceptance-criterion", named)
        self.assertIn("source_id is AC-1", named)


    def context_declaring(self, asset: str, reason: str) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.",
             "confirmed_absences": [{"asset": asset, "reason": reason}]},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
        ])

    def test_an_examined_context_leaves_the_gap_list_but_an_unexamined_one_stays(self) -> None:
        self.context_declaring("aggregates", "Invariants are thin here; a transaction script is the declared pattern.")
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
        self.context_declaring("rules", "No business rule is written down for this context.")
        self.seed("rules.json", [{"id": "order-total", "contexts": ["orders"], "statement": "An order total is non-negative."}])
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(any("rules records name it" in error for error in errors), errors)

    def test_a_confirmed_absence_of_an_unknown_asset_names_the_ones_allowed(self) -> None:
        self.context_declaring("events", "Events are elsewhere.")
        errors = validate(self.repo / "memory", None, False)
        named = next(error for error in errors if "unknown asset" in error)
        for allowed in ("aggregates", "contracts", "rules", "vocabulary"):
            self.assertIn(allowed, named)


    def submitted_package(self) -> Path:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        return package

    def proposal_in(self, package: Path) -> dict:
        return json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))

    def test_superseding_keeps_the_status_the_proposal_died_in(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "The event catalogue shows the collaboration runs the other way.", None)
        proposal = self.proposal_in(package)
        self.assertEqual(proposal["status"], "superseded")
        self.assertEqual(proposal["superseded_from_status"], "submitted")
        self.assertIn("other way", proposal["superseded_reason"])

    def test_a_superseded_proposal_can_name_what_replaced_it(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "Replaced by a proposal with the corrected direction.", "PRO-2")
        self.assertEqual(self.proposal_in(package)["superseded_by"], "PRO-2")

    def test_superseding_without_a_reason_is_refused(self) -> None:
        package = self.submitted_package()
        with self.assertRaisesRegex(ValueError, "requires a reason"):
            supersede_proposal(package, "   ", None)
        self.assertEqual(self.proposal_in(package)["status"], "submitted")

    def test_a_superseded_proposal_cannot_be_superseded_again(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "Overtaken by the event catalogue.", None)
        with self.assertRaisesRegex(ValueError, "create a new draft instead"):
            supersede_proposal(package, "Overtaken twice.", None)

    def test_a_superseded_proposal_no_longer_accepts_approval(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "Overtaken by the event catalogue.", None)
        with self.assertRaisesRegex(ValueError, "only for a submitted proposal"):
            record_approval(package, "domain-owner", "reviewer", "entire proposal", None)


if __name__ == "__main__":
    unittest.main()
