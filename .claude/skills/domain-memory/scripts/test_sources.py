from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.common import load_json
from domain_registry.evidence import digest
from domain_registry.impact import boundary_impact
from domain_registry.policy import validate_policy
from domain_registry.readiness import assess_readiness
from domain_registry.registry import (
    migrate_evidence,
    verify_evidence,
)
from domain_registry.security import scan_report
from domain_registry.sources import (
    confirm_sources,
    discover_sources,
    probe_sources,
    refine_sources,
    selected_source_map,
    source_files,
    source_kind_for,
    source_policy_report,
    verify_source_map,
    write_source_map,
)
from domain_registry.sources import discover_ci_tools
from domain_registry.sources import source_snapshots
from test_support import RegistryTestCase, stored_policy_of, use_json_layout


class SourcesTest(RegistryTestCase):
    def test_a_repository_inside_a_directory_named_build_still_has_sources(self) -> None:
        with tempfile.TemporaryDirectory() as parent:
            root = Path(parent) / "build" / "repo"
            (root / "docs").mkdir(parents=True)
            (root / "docs" / "rules.md").write_text("a rule\n", encoding="utf-8")
            [snapshot] = source_snapshots(root, ["docs"])
        self.assertEqual(snapshot["file_count"], 1)

    def test_a_dependency_directory_inside_a_selected_source_is_skipped(self) -> None:
        (self.repo / "docs" / "node_modules").mkdir(parents=True)
        (self.repo / "docs" / "rules.md").write_text("a rule\n", encoding="utf-8")
        (self.repo / "docs" / "node_modules" / "readme.md").write_text("x\n", encoding="utf-8")
        [snapshot] = source_snapshots(self.repo, ["docs"])
        self.assertEqual(snapshot["file_count"], 1)

    def test_editing_a_selected_source_is_content_drift_not_selection_drift(
        self,
    ) -> None:
        source_map_path = self.selected_docs()
        (self.repo / "docs" / "rules.md").write_text(
            "revised version\n", encoding="utf-8"
        )
        result = verify_source_map(self.repo, source_map_path)
        self.assertEqual("current", result["status"])
        self.assertEqual([], result["changed_sources"])
        self.assertEqual(
            ["docs"], [entry["path"] for entry in result["content_changed"]]
        )

    def test_a_discovered_file_that_is_gone_is_listed_without_moving_the_selection(self) -> None:
        (self.repo / "tests").mkdir()
        (self.repo / "tests" / "test_kept.py").write_text("kept\n", encoding="utf-8")
        (self.repo / "tests" / "test_gone.py").write_text("gone\n", encoding="utf-8")
        source_map_path = self.selected_docs()
        (self.repo / "tests" / "test_gone.py").unlink()
        result = verify_source_map(self.repo, source_map_path)
        self.assertEqual(
            ("current", ["tests/test_gone.py"]),
            (result["status"], result["discovered_but_gone"]),
        )

    def test_a_file_added_under_a_selected_source_moves_the_selection(self) -> None:
        source_map_path = self.selected_docs()
        (self.repo / "docs" / "extra.md").write_text("second\n", encoding="utf-8")
        result = verify_source_map(self.repo, source_map_path)
        self.assertEqual("stale", result["status"])
        self.assertEqual(
            ["docs"], [entry["path"] for entry in result["changed_sources"]]
        )
        self.assertEqual([], result["content_changed"])

    def test_a_file_removed_from_a_selected_source_moves_the_selection(self) -> None:
        source_map_path = self.selected_docs()
        (self.repo / "docs" / "rules.md").unlink()
        result = verify_source_map(self.repo, source_map_path)
        self.assertEqual("stale", result["status"])
        self.assertEqual(
            ["docs"], [entry["path"] for entry in result["changed_sources"]]
        )

    def test_a_snapshot_without_a_listing_digest_falls_back_to_the_file_count(
        self,
    ) -> None:
        source_map_path = self.selected_docs()
        source_map = json.loads(source_map_path.read_text(encoding="utf-8"))
        for snapshot in source_map["source_snapshots"]:
            del snapshot["listing_sha256"]
        write_source_map(source_map_path, source_map)
        untouched = verify_source_map(self.repo, source_map_path)
        self.assertEqual("current", untouched["status"])
        self.assertEqual([], untouched["content_changed"])
        (self.repo / "docs" / "rules.md").write_text("revised\n", encoding="utf-8")
        self.assertEqual(
            "current", verify_source_map(self.repo, source_map_path)["status"]
        )
        (self.repo / "docs" / "extra.md").write_text("second\n", encoding="utf-8")
        self.assertEqual(
            "stale", verify_source_map(self.repo, source_map_path)["status"]
        )

    def test_a_renamed_file_moves_the_selection_when_the_listing_is_recorded(
        self,
    ) -> None:
        source_map_path = self.selected_docs()
        (self.repo / "docs" / "rules.md").rename(self.repo / "docs" / "renamed.md")
        result = verify_source_map(self.repo, source_map_path)
        self.assertEqual("stale", result["status"])
        self.assertEqual(
            ["docs"], [entry["path"] for entry in result["changed_sources"]]
        )

    def test_refining_sources_replaces_the_corpus_and_requires_confirmation(self) -> None:
        source = self.repo / "fact.md"
        source.write_text("x", encoding="utf-8")
        policy = stored_policy_of(self.repo)
        result = refine_sources(self.repo / "memory", self.repo, [source], policy)
        self.assertEqual(result["selected_paths"], ["fact.md"])
        self.assertEqual(result["selection_status"], "agent-asserted")
        self.assertEqual(stored_policy_of(self.repo)["source_policy"]["selected_paths"], ["fact.md"])

    def test_a_source_map_with_mismatched_snapshot_paths_is_unverified(self) -> None:
        source = self.repo / "docs"
        source.mkdir()
        (source / "rules.md").write_text("rules\n", encoding="utf-8")
        source_map = selected_source_map(self.repo, [source])
        source_map["source_snapshots"][0]["path"] = "other"
        path = self.write_record("source-map.json", source_map)
        self.assertEqual("unverified", verify_source_map(self.repo, path)["status"])

    def test_source_policy_excludes_files_and_enforces_limits(self) -> None:
        source = self.repo / "docs"
        source.mkdir()
        (source / "included.md").write_text("one", encoding="utf-8")
        (source / "private.md").write_text("two", encoding="utf-8")
        policy = {
            "source_policy": {"include": ["docs/**"], "exclude": ["docs/private.md"]},
            "limits": {
                "max_file_count": 1,
                "max_file_bytes": 10,
                "max_total_bytes": 10,
            },
        }
        report = source_policy_report(self.repo, [source], policy)
        self.assertEqual(1, report["files"])
        self.assertEqual([], report["errors"])
        source_map = selected_source_map(self.repo, [source], policy)
        self.assertEqual(
            "current",
            verify_source_map(
                self.repo, self.write_record("source-map.json", source_map), policy
            )["status"],
        )
        (source / "included.md").write_text("too many bytes", encoding="utf-8")
        self.assertEqual(
            "invalid",
            verify_source_map(self.repo, self.repo / "source-map.json", policy)[
                "status"
            ],
        )

    def test_migrated_evidence_is_classified_by_the_source_map_when_verified(self) -> None:
        self.seed_line_reference_evidence()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            {"selected_paths": ["source.md"],
             "source_groups": [{"kind": "decisions", "paths": ["source.md"]}]},
        )
        migrate_evidence(self.repo / "memory", self.repo)
        [result] = verify_evidence(self.repo / "memory", self.repo)["results"]
        self.assertEqual("decisions", result["source_kind"])

    def test_a_change_across_two_contexts_without_a_collaboration_is_unregistered(self) -> None:
        self.two_mapped_contexts()
        impact = self.change("src/orders/port.py", "src/billing/invoice.py")
        self.assertEqual("unregistered-co-change", impact["verdict"])
        self.assertEqual({"billing": ["src/billing/invoice.py"], "orders": ["src/orders/port.py"]},
                         impact["contexts_touched"])
        self.assertEqual([{"contexts": ["billing", "orders"], "collaborations": []}], impact["co_changed_pairs"])

    def test_a_change_across_two_collaborating_contexts_names_the_collaboration(self) -> None:
        self.two_mapped_contexts(dependency_policies=[
            {"id": "orders-may-use-billing", "from_context": "orders", "to_context": "billing", "status": "reviewed"},
        ])
        impact = self.change("src/orders/port.py", "src/billing/invoice.py")
        self.assertEqual("registered-co-change", impact["verdict"])
        self.assertEqual(
            [{"asset": "dependency-policies", "id": "orders-may-use-billing", "status": "reviewed"}],
            impact["co_changed_pairs"][0]["collaborations"],
        )

    def test_a_contract_between_two_contexts_registers_their_collaboration(self) -> None:
        self.two_mapped_contexts(contracts=[
            {"id": "invoice-v1", "producer_context": "billing", "consumer_contexts": ["orders"]},
        ])
        impact = self.change("src/orders/port.py", "src/billing/invoice.py")
        self.assertEqual(
            ("registered-co-change", [{"asset": "contracts", "id": "invoice-v1", "status": "candidate"}]),
            (impact["verdict"], impact["co_changed_pairs"][0]["collaborations"]),
        )

    def test_a_changed_file_cited_by_a_boundary_record_is_named(self) -> None:
        self.two_mapped_contexts(interactions=[
            {"id": "orders-bill", "producer_context": "orders", "consumer_context": "billing",
             "evidence": [{"path": "src/orders/port.py", "lines": {"start": 1, "end": 1}}]},
        ])
        impact = self.change("src/orders/port.py")
        self.assertEqual("boundary-evidence-changed", impact["verdict"])
        self.assertEqual(
            [{"asset": "interactions", "id": "orders-bill", "status": "candidate", "changed_evidence": ["src/orders/port.py"]}],
            impact["boundary_evidence_changed"],
        )

    def test_a_changed_file_whose_cited_lines_only_moved_is_not_changed_evidence(self) -> None:
        self.two_mapped_contexts(interactions=[
            {"id": "orders-bill", "producer_context": "orders", "consumer_context": "billing",
             "evidence": [{"path": "src/orders/port.py", "lines": {"start": 1, "end": 1}, "excerpt_sha256": digest(b"v1\n")}]},
        ])
        (self.repo / "src/orders/port.py").write_bytes(b"header\nv1\n")
        impact = boundary_impact(self.repo / "memory", self.repo, "HEAD")
        self.assertEqual(("no-cross-context-change", []), (impact["verdict"], impact["boundary_evidence_changed"]))

    def test_a_changed_file_whose_cited_lines_changed_is_changed_evidence(self) -> None:
        self.two_mapped_contexts(interactions=[
            {"id": "orders-bill", "producer_context": "orders", "consumer_context": "billing",
             "evidence": [{"path": "src/orders/port.py", "lines": {"start": 1, "end": 1}, "excerpt_sha256": digest(b"v1\n")}]},
        ])
        self.assertEqual("boundary-evidence-changed", self.change("src/orders/port.py")["verdict"])

    def touched(self, cited: str, start: int, appended: bytes, prepended: bytes = b"") -> list[dict]:
        self.two_mapped_contexts(interactions=[
            {"id": "orders-bill", "producer_context": "orders", "consumer_context": "billing",
             "evidence": [{"path": cited, "lines": {"start": start, "end": start}}]},
        ])
        target = self.repo / cited
        target.write_bytes(prepended + (target.read_bytes() if target.exists() else b"") + appended)
        return boundary_impact(self.repo / "memory", self.repo, "HEAD")["boundary_evidence_touched"]

    def test_a_cited_line_the_change_added_is_touched_evidence(self) -> None:
        self.assertEqual(
            [{"asset": "interactions", "id": "orders-bill", "status": "candidate",
              "touched_evidence": ["src/orders/port.py:2-2"]}],
            self.touched("src/orders/port.py", 2, b"added\n"),
        )

    def test_a_cited_line_before_the_changed_lines_is_not_touched(self) -> None:
        self.assertEqual([], self.touched("src/orders/port.py", 1, b"added\n"))

    def test_a_cited_line_after_the_changed_lines_is_not_touched(self) -> None:
        self.assertEqual([], self.touched("src/orders/port.py", 2, b"", prepended=b"added\n"))

    def test_a_cited_line_of_an_untracked_file_is_touched(self) -> None:
        self.assertEqual(["src/orders/new.py:1-1"], self.touched("src/orders/new.py", 1, b"new\n")[0]["touched_evidence"])

    def test_a_change_inside_one_context_crosses_no_boundary(self) -> None:
        self.two_mapped_contexts()
        impact = self.change("src/orders/port.py", "README.md")
        self.assertEqual(("no-cross-context-change", 2, []), (impact["verdict"], impact["changed_files"], impact["co_changed_pairs"]))
        self.assertEqual(["README.md"], impact["outside_every_context"])

    def test_a_changed_file_outside_every_context_is_named_with_the_records_citing_it(self) -> None:
        cited = [{"path": "contracts/api.yaml", "lines": {"start": 1, "end": 1}}]
        inside = [{"path": "src/orders/port.py", "lines": {"start": 1, "end": 1}}]
        self.two_mapped_contexts(rules=[{"id": "r", "evidence": cited}, {"id": "inside", "evidence": inside}],
                                 contracts=[{"id": "api", "producer_context": "orders", "evidence": cited}])
        impact = self.change("contracts/api.yaml", "docs/note.md", "src/orders/port.py")
        self.assertEqual(
            (3, 1, [{"path": "contracts/api.yaml", "cited_by": ["contracts/api", "rules/r"]}]),
            (impact["changed_files"], impact["mapped_files"], impact["cited_outside_contexts"]),
        )

    def test_an_untracked_file_and_a_nested_path_count_but_a_sibling_prefix_does_not(self) -> None:
        self.two_mapped_contexts()
        impact = self.change("lib/src/billing/new.py", "src/orders_archive/old.py", "x/src/orders")
        self.assertEqual({"billing": ["lib/src/billing/new.py"]}, impact["contexts_touched"])

    def test_an_unknown_base_is_an_error_not_a_quiet_result(self) -> None:
        self.two_mapped_contexts()
        with self.assertRaisesRegex(ValueError, "git could not list the changes since no-such-revision"):
            boundary_impact(self.repo / "memory", self.repo, "no-such-revision")

    def test_oversized_source_names_the_file_and_the_limit(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "big.md").write_text("x" * 64, encoding="utf-8")
        policy = json.loads(
            (self.repo / "memory" / "domain-memory-policy.json").read_text(
                encoding="utf-8"
            )
        )
        policy["limits"] = {
            "max_file_count": 100,
            "max_file_bytes": 16,
            "max_total_bytes": 10000,
            "max_query_results": 1,
        }
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

    def test_a_freshly_selected_source_map_is_only_ever_agent_asserted(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        asserted = selected_source_map(self.repo, [self.repo / "docs"])
        self.assertEqual(asserted["selection_status"], "agent-asserted")
        self.assertNotIn("confirmed_by", asserted)

    def test_confirming_records_who_chose_the_sources_and_when(self) -> None:
        path = self.committed_source_map()
        confirmed = confirm_sources(self.repo / "memory", self.repo, "Arthur")
        self.assertEqual(confirmed["selection_status"], "developer-confirmed")
        self.assertEqual(confirmed["confirmed_by"], "Arthur")
        self.assertTrue(confirmed["confirmed_at"].endswith("Z"))
        self.assertEqual(
            json.loads(path.read_text(encoding="utf-8"))["selection_status"],
            "developer-confirmed",
        )

    def test_confirming_without_a_named_developer_leaves_the_map_agent_asserted(
        self,
    ) -> None:
        path = self.committed_source_map()
        for empty in ("", "   ", "<identity>"):
            with self.assertRaisesRegex(ValueError, "developer"):
                confirm_sources(self.repo / "memory", self.repo, empty)
        self.assertEqual(
            json.loads(path.read_text(encoding="utf-8"))["selection_status"],
            "agent-asserted",
        )

    def test_sources_that_moved_cannot_be_confirmed(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "appeared.md").write_text(
            "added after the developer looked", encoding="utf-8"
        )
        with self.assertRaisesRegex(ValueError, "no longer there"):
            confirm_sources(self.repo / "memory", self.repo, "Arthur")
        self.assertEqual(
            json.loads(path.read_text(encoding="utf-8"))["selection_status"],
            "agent-asserted",
        )

    def test_a_source_map_the_policy_rejects_is_not_reported_as_moved(self) -> None:
        self.committed_source_map()
        self.permit_sources()
        policy = stored_policy_of(self.repo)
        policy["limits"]["max_file_bytes"] = 1
        with self.assertRaisesRegex(
            ValueError, "does not verify.*over max_file_bytes 1: docs/kept.md"
        ):
            confirm_sources(self.repo / "memory", self.repo, "Arthur", policy)

    def test_the_instruction_files_group_says_it_is_not_a_statement_of_the_domain(
        self,
    ) -> None:
        (self.repo / "AGENTS.md").write_text("How agents work here.", encoding="utf-8")
        group = next(
            g
            for g in discover_sources(self.repo)["source_groups"]
            if g["kind"] == "repository_instructions"
        )
        self.assertEqual(group["paths"], ["AGENTS.md"])
        self.assertIn("not a statement of the business domain", group["authority"])
        self.assertIn("loses its evidence", group["caution"])

    def test_discovery_lists_both_instruction_file_names(self) -> None:
        (self.repo / "AGENTS.md").write_text("How agents work here.", encoding="utf-8")
        (self.repo / "web").mkdir()
        (self.repo / "web" / "CLAUDE.md").write_text("@AGENTS.md", encoding="utf-8")
        self.assertEqual(
            self.discovery_groups("repository_instructions"),
            ["AGENTS.md", "web/CLAUDE.md"],
        )

    def test_a_package_doc_outside_the_implementation_roots_is_not_a_boundary(
        self,
    ) -> None:
        for directory in ("apps/core", "tools/devctl"):
            (self.repo / directory).mkdir(parents=True)
            (self.repo / directory / "doc.go").write_text("package x\n", encoding="utf-8")
        self.assertEqual(self.discovery_groups("bounded_contexts"), ["apps/core/doc.go"])

    def test_discovery_gives_no_file_a_role_by_its_project_specific_name(self) -> None:
        guidance = self.repo / "docs" / "development" / "platform-context-map.md"
        guidance.parent.mkdir(parents=True)
        guidance.write_text("Contexts and their owners.", encoding="utf-8")

        listed = [
            path
            for group in discover_sources(self.repo)["source_groups"]
            for path in group["paths"]
        ]
        self.assertNotIn("docs/development/platform-context-map.md", listed)

    def test_discovery_leaves_out_what_the_repository_ignores(self) -> None:
        self.seed_generated_cache()
        (self.repo / ".gitignore").write_text(
            "/.agents/\napps/.cache/\n", encoding="utf-8"
        )

        self.assertEqual(self.discovery_groups("repository_instructions"), ["AGENTS.md"])
        self.assertEqual(self.discovery_groups("bounded_contexts"), ["apps/core/doc.go"])
        self.assertEqual(self.discovery_groups("tests"), ["apps/core/core_test.go"])

    def test_discovery_outside_a_git_repository_keeps_every_file_it_finds(self) -> None:
        with tempfile.TemporaryDirectory() as plain:
            root = Path(plain)
            (root / "AGENTS.md").write_text("How agents work here.", encoding="utf-8")
            (root / ".agents" / "skills").mkdir(parents=True)
            (root / ".agents" / "skills" / "AGENTS.md").write_text(
                "A copy this host generated.", encoding="utf-8"
            )
            (root / ".gitignore").write_text("/.agents/\n", encoding="utf-8")

            paths = next(
                group
                for group in discover_sources(root)["source_groups"]
                if group["kind"] == "repository_instructions"
            )["paths"]
            self.assertEqual(paths, [".agents/skills/AGENTS.md", "AGENTS.md"])

    def test_an_agent_asserted_map_still_reports_whether_its_sources_moved(
        self,
    ) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        kept = self.repo / "docs" / "kept.md"
        kept.write_text("kept", encoding="utf-8")
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, selected_source_map(self.repo, [self.repo / "docs"]))
        fresh = verify_source_map(self.repo, path)
        self.assertEqual(fresh["status"], "current")
        self.assertEqual(fresh["selection_status"], "agent-asserted")
        kept.unlink()
        self.assertEqual(verify_source_map(self.repo, path)["status"], "stale")

    def test_each_structural_problem_in_a_source_map_gets_its_own_verdict_and_reason(self) -> None:
        path = self.committed_source_map()
        sound = json.loads(path.read_text(encoding="utf-8"))
        policy = stored_policy_of(self.repo)

        def escaping(value):
            value["selected_paths"] = ["../outside"]
            value["source_snapshots"][0]["path"] = "../outside"

        cases = [
            ("no snapshots", lambda value: value.pop("source_snapshots"), None,
             "unverified", "source map has no snapshots"),
            ("a snapshot that is not an object", lambda value: value["source_snapshots"].append(7), None,
             "unverified", "source map has an invalid snapshot"),
            ("a path selected twice", lambda value: value["selected_paths"].append("docs"), None,
             "unverified", "source map snapshots do not match selected_paths"),
            ("a path outside the repository", escaping, None,
             "invalid", "source path escapes repository: ../outside"),
            ("a policy selecting other paths", lambda value: None, ["other"],
             "invalid", "policy selected_paths do not match the source map"),
        ]
        for label, change, policy_paths, status, reason in cases:
            with self.subTest(label):
                value = json.loads(json.dumps(sound))
                change(value)
                path.write_text(json.dumps(value), encoding="utf-8")
                chosen = None
                if policy_paths is not None:
                    chosen = json.loads(json.dumps(policy))
                    chosen["source_policy"]["selected_paths"] = policy_paths
                result = verify_source_map(self.repo, path, chosen)
                self.assertEqual((status, reason), (result["status"], result["reason"]))

    def test_a_clean_repo_answers_the_probe_without_hashing_anything(self) -> None:
        result = probe_sources(self.repo, self.committed_source_map())
        self.assertEqual(result["status"], "current")
        self.assertEqual(result["checked"], "git")

    def test_probe_rechecks_policy_limits_before_using_the_git_fast_path(self) -> None:
        path = self.committed_source_map()
        self.permit_sources()
        policy = stored_policy_of(self.repo)
        policy["limits"]["max_file_bytes"] = 1
        result = probe_sources(self.repo, path, policy)
        self.assertEqual(
            ("invalid", "over max_file_bytes 1: docs/kept.md"),
            (result["status"], result["reason"]),
        )

    def test_probe_rejects_policy_source_paths_that_differ_from_the_map(self) -> None:
        path = self.committed_source_map()
        policy = stored_policy_of(self.repo)
        policy["source_policy"]["selected_paths"] = ["other"]
        result = probe_sources(self.repo, path, policy)
        self.assertEqual(result["status"], "invalid")
        self.assertEqual(
            "invalid", verify_source_map(self.repo, path, policy)["status"]
        )

    def test_the_probe_answers_absent_invalid_and_hashed_cases_without_git(self) -> None:
        missing = self.repo / "memory" / "no-map.json"
        self.assertEqual({"status": "absent", "reason": f"no source map at {missing}"}, probe_sources(self.repo, missing))
        path = self.committed_source_map()
        policy = stored_policy_of(self.repo)
        with self.subTest("an invalid policy"):
            result = probe_sources(self.repo, path, {**policy, "storage_mode": "cloud"})
            self.assertEqual(("invalid", "policy has an invalid storage_mode"), (result["status"], result["reason"]))
        with self.subTest("a policy report that cannot be built"), \
                patch("domain_registry.sources.source_policy_report", side_effect=OSError("disk gone")):
            result = probe_sources(self.repo, path, policy)
            self.assertEqual(("invalid", "disk gone"), (result["status"], result["reason"]))
        with self.subTest("a map recorded with dirty sources"):
            value = json.loads(path.read_text(encoding="utf-8"))
            value["git_state"]["dirty_sources"] = ["docs/kept.md"]
            path.write_text(json.dumps(value), encoding="utf-8")
            self.assertEqual(("current", "hash"), tuple(probe_sources(self.repo, path)[key] for key in ("status", "checked")))

    def test_a_committed_source_change_sends_the_probe_back_to_hashing(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "kept.md").write_text("kept, then edited", encoding="utf-8")
        self.commit_all("edit a source")
        result = probe_sources(self.repo, path)
        self.assertEqual(
            ("hash", "current", [{"path": "docs", "status": "content-changed"}]),
            (result["checked"], result["status"], result["content_changed"]),
        )

    def test_an_uncommitted_source_sends_the_probe_back_to_hashing(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "kept.md").write_text("edited", encoding="utf-8")
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "current")
        self.assertEqual(
            ["docs"], [entry["path"] for entry in result["content_changed"]]
        )

    def test_a_committed_file_added_to_a_source_is_reported_as_stale(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "added.md").write_text("added", encoding="utf-8")
        self.commit_all("add a source file")
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "stale")

    def test_a_map_without_a_recorded_git_state_is_probed_by_hashing(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        source_map = selected_source_map(self.repo, [self.repo / "docs"])
        del source_map["git_state"]
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, source_map)
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "current")

    def test_a_nested_source_owns_its_files_instead_of_its_parent(self) -> None:
        source_map = self.nested_sources()
        counts = {
            snapshot["path"]: snapshot["file_count"]
            for snapshot in source_map["source_snapshots"]
        }
        self.assertEqual(counts["apps/svc/doc.go"], 1)
        self.assertEqual(counts["apps"], 1)

    def test_editing_a_nested_source_leaves_its_parent_alone(self) -> None:
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, self.nested_sources())
        (self.repo / "apps" / "svc" / "doc.go").write_text(
            "package svc // revised", encoding="utf-8"
        )
        result = verify_source_map(self.repo, path)
        self.assertEqual(result["status"], "current")
        self.assertEqual(
            [source["path"] for source in result["content_changed"]],
            ["apps/svc/doc.go"],
        )

    def test_a_file_added_beside_a_nested_source_moves_only_its_parent(self) -> None:
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, self.nested_sources())
        (self.repo / "apps" / "svc" / "extra.go").write_text(
            "package svc", encoding="utf-8"
        )
        result = verify_source_map(self.repo, path)
        self.assertEqual(result["status"], "stale")
        self.assertEqual(
            [source["path"] for source in result["changed_sources"]], ["apps"]
        )

    def test_the_most_specific_source_decides_what_a_citation_establishes(self) -> None:
        source_map = self.nested_sources()
        self.assertEqual(
            source_kind_for(source_map, "apps/svc/doc.go"), "bounded_contexts"
        )
        self.assertEqual(source_kind_for(source_map, "apps/other.go"), "implementation")

    def test_a_citation_outside_every_selected_source_is_unclassified(self) -> None:
        self.assertEqual(
            source_kind_for(self.nested_sources(), "infra/node.txt"), "unclassified"
        )

    def test_a_selected_file_inherits_its_discovered_parent_kind(self) -> None:
        source_map = {
            "source_kinds": {"apps/svc/preflight.go": "unclassified"},
            "source_groups": [{"kind": "implementation", "paths": ["apps"]}],
        }
        self.assertEqual(
            source_kind_for(source_map, "apps/svc/preflight.go"), "implementation"
        )

    def test_a_citation_inside_a_selected_path_no_group_classifies_is_selected(self) -> None:
        source_map = {"source_kinds": {"db": "unclassified"}, "source_groups": []}
        self.assertEqual(
            ("selected", "unclassified"),
            (source_kind_for(source_map, "db/migrations/0002.sql"), source_kind_for(source_map, "infra/x.tf")),
        )

    def test_a_path_two_groups_list_takes_the_kind_of_the_first(self) -> None:
        source_map = {
            "source_groups": [
                {"kind": "bounded_contexts", "paths": ["apps/svc"]},
                {"kind": "implementation", "paths": ["apps/svc"]},
            ],
        }
        self.assertEqual(
            source_kind_for(source_map, "apps/svc/doc.go"), "bounded_contexts"
        )

    def write_files(self, *names: str) -> None:
        for name in names:
            path = self.repo / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("x", encoding="utf-8")

    def ignore(self, *patterns: str) -> None:
        (self.repo / ".gitignore").write_text(
            "".join(f"{pattern}\n" for pattern in patterns), encoding="utf-8"
        )

    def test_discovery_lists_nested_test_files_and_leaves_out_a_git_ignored_one(self) -> None:
        self.write_files(
            "apps/svc/thing_test.go",
            "apps/svc/thing.go",
            "apps/svc/deep/inner_test.go",
            "apps/svc/skipped_test.go",
        )
        self.ignore("skipped_test.go")
        self.assertEqual(
            ["apps/svc/deep/inner_test.go", "apps/svc/thing_test.go"],
            self.discovery_groups("tests"),
        )

    def test_nested_test_files_do_not_reclassify_the_implementation_root(self) -> None:
        self.write_files("apps/svc/deep/inner_test.go", "apps/svc/thing.go")
        self.assertEqual(["apps"], self.discovery_groups("implementation"))

    def test_discovery_leaves_out_instruction_files_in_a_git_ignored_non_ascii_directory(self) -> None:
        self.write_files("AGENTS.md", "資料/AGENTS.md")
        self.ignore("資料/")
        self.assertEqual(["AGENTS.md"], self.discovery_groups("repository_instructions"))

    def test_discovery_leaves_out_a_package_doc_in_a_git_ignored_non_ascii_directory(self) -> None:
        self.write_files("apps/x/doc.go", "apps/x/資料/doc.go")
        self.ignore("資料/")
        self.assertEqual(["apps/x/doc.go"], self.discovery_groups("bounded_contexts"))

    def test_discovery_leaves_out_a_test_file_in_a_git_ignored_non_ascii_directory(self) -> None:
        self.write_files("apps/x/kept_test.go", "apps/x/資料/ignored_test.go")
        self.ignore("資料/")
        self.assertEqual(["apps/x/kept_test.go"], self.discovery_groups("tests"))

    def test_discovery_keeps_a_file_whose_name_contains_an_accented_letter(self) -> None:
        self.write_files("apps/x/café_test.go", "資料/ignored_test.go")
        self.ignore("資料/")
        self.assertEqual(["apps/x/café_test.go"], self.discovery_groups("tests"))

    def test_discovery_keeps_a_file_whose_name_contains_a_space(self) -> None:
        self.write_files("apps/x/my space_test.go", "資料/ignored_test.go")
        self.ignore("資料/")
        self.assertEqual(["apps/x/my space_test.go"], self.discovery_groups("tests"))

    def test_discovery_keeps_an_instruction_file_in_a_directory_with_an_accented_letter_and_a_space(self) -> None:
        self.write_files("café/AGENTS.md", "my docs/CLAUDE.md", "資料/AGENTS.md")
        self.ignore("資料/")
        self.assertEqual(
            ["café/AGENTS.md", "my docs/CLAUDE.md"],
            self.discovery_groups("repository_instructions"),
        )

    def test_the_dirty_state_of_a_non_ascii_source_is_read_back_as_git_wrote_it(self) -> None:
        self.write_files("資料/a.md")
        self.commit_all("non-ascii source")
        (self.repo / "資料" / "a.md").write_text("changed", encoding="utf-8")
        source_map = selected_source_map(self.repo, [self.repo / "資料"])
        self.assertEqual([" M 資料/a.md"], source_map["git_state"]["dirty_sources"])

    def test_discovery_skips_a_nested_checkout_whose_git_is_a_directory(self) -> None:
        self.write_files("AGENTS.md", "copy/.git/HEAD", "copy/AGENTS.md")
        self.assertEqual(["AGENTS.md"], self.discovery_groups("repository_instructions"))

    def test_discovery_skips_a_nested_checkout_whose_git_is_a_file(self) -> None:
        self.write_files("AGENTS.md", "copy/.git", "copy/AGENTS.md")
        self.assertEqual(["AGENTS.md"], self.discovery_groups("repository_instructions"))

    def test_discovery_does_not_descend_into_an_excluded_directory(self) -> None:
        self.write_files("kept/test_kept.py")
        excluded = [
            ".git", ".hg", ".svn", ".devctl", "node_modules",
            ".venv", "vendor", "dist", "build", "generated",
        ]
        for name in excluded:
            with self.subTest(name):
                self.write_files(f"{name}/test_inside.py")
                self.assertEqual(["kept/test_kept.py"], self.discovery_groups("tests"))

    def test_the_source_policy_report_accepts_each_limit_at_its_value_and_refuses_one_past_it(self) -> None:
        five = {f"f{index}.md": 2 for index in range(1, 6)}
        rows = [
            ("a file not matching include is not counted", {"a.md": 3, "b.txt": 3}, ["docs/*.md"], {}, 1, []),
            ("file count at its limit", {"a.md": 1, "b.md": 1}, ["**"], {"max_file_count": 2}, 2, []),
            ("file count one past its limit", {"a.md": 1, "b.md": 1, "c.md": 1}, ["**"], {"max_file_count": 2}, 3,
             ["selected sources hold 3 files, over max_file_count 2"]),
            ("file bytes at its limit", {"a.md": 4}, ["**"], {"max_file_bytes": 4}, 1, []),
            ("file bytes one past its limit", {"a.md": 5}, ["**"], {"max_file_bytes": 4}, 1,
             ["over max_file_bytes 4: docs/a.md"]),
            ("total bytes at its limit", {"a.md": 3, "b.md": 3}, ["**"], {"max_total_bytes": 6}, 2, []),
            ("total bytes one past its limit", {"a.md": 3, "b.md": 3}, ["**"], {"max_total_bytes": 5}, 2,
             ["selected sources hold 6 bytes, over max_total_bytes 5"]),
            ("five oversized files are all named", five, ["**"], {"max_file_bytes": 1}, 5,
             ["over max_file_bytes 1: docs/f1.md, docs/f2.md, docs/f3.md, docs/f4.md, docs/f5.md"]),
            ("a sixth oversized file is counted", {**five, "f6.md": 2}, ["**"], {"max_file_bytes": 1}, 6,
             ["over max_file_bytes 1: docs/f1.md, docs/f2.md, docs/f3.md, docs/f4.md, docs/f5.md, and 1 more"]),
            ("a seventh oversized file is counted", {**five, "f6.md": 2, "f7.md": 2}, ["**"], {"max_file_bytes": 1}, 7,
             ["over max_file_bytes 1: docs/f1.md, docs/f2.md, docs/f3.md, docs/f4.md, docs/f5.md, and 2 more"]),
        ]
        for label, sizes, include, limits, files, errors in rows:
            with self.subTest(label), tempfile.TemporaryDirectory() as parent:
                root = Path(parent).resolve()
                (root / "docs").mkdir()
                for name, size in sizes.items():
                    (root / "docs" / name).write_bytes(b"x" * size)
                policy = {
                    "source_policy": {"include": include, "exclude": []},
                    "limits": {"max_file_count": 100, "max_file_bytes": 1000, "max_total_bytes": 10000} | limits,
                }
                report = source_policy_report(root, [root / "docs"], policy)
                self.assertEqual((files, errors), (report["files"], report["errors"]))

    def test_ci_discovery_offers_candidates_without_selecting_governance(self) -> None:
        (self.repo / ".github" / "workflows").mkdir(parents=True)
        self.assertEqual(["github-actions"], discover_ci_tools(self.repo))
        discovered = discover_sources(self.repo)
        self.assertEqual("github-pr", discovered["governance_candidates"]["recommended_verifier"])
        self.assertEqual("discovered", discovered["selection_status"])

    def test_discovery_only_recommends_governance_a_policy_accepts(self) -> None:
        def policy_from(candidates: dict) -> dict:
            value = json.loads(
                (self.repo / "memory" / "domain-memory-policy.json").read_text(
                    encoding="utf-8"
                )
            )
            value["review_mode"] = "scm-verified"
            value["review_governance"] = {
                "verifier": candidates["recommended_verifier"],
                "trigger": candidates["recommended_trigger"],
                "ci_requirement": "optional",
                "authorized_signers": ["ABCD"],
            }
            return value

        with self.subTest("no CI discovered"):
            candidates = discover_sources(self.repo)["governance_candidates"]
            self.assertEqual([], validate_policy(policy_from(candidates)))

        (self.repo / ".github" / "workflows").mkdir(parents=True)
        with self.subTest("CI discovered"):
            candidates = discover_sources(self.repo)["governance_candidates"]
            self.assertEqual([], validate_policy(policy_from(candidates)))

    def test_a_secret_scan_reports_what_it_read_and_why_it_skipped_the_rest(self) -> None:
        secrets = self.repo / "leaky"
        (secrets / "node_modules").mkdir(parents=True)
        (secrets / "node_modules" / "vendored.env").write_bytes(b"ghp_" + b"e" * 30)
        (secrets / "clean.md").write_bytes(b"one\n")
        (secrets / "ci.env").write_bytes(b"first\nghp_" + b"f" * 30 + b"\n")
        report = scan_report(secrets)
        self.assertEqual(
            ("complete", [{"path": "ci.env", "line": 2, "kind": "github-token"}], 2, 4 + 6 + 35, 0),
            (report["status"], report["findings"], report["scanned_files"], report["scanned_bytes"],
             report["skipped_count"]),
        )
        with self.subTest("symlink and unreadable"), \
                patch.object(Path, "is_symlink", lambda path: path.name == "clean.md"), \
                patch.object(Path, "open", side_effect=OSError("denied")):
            report = scan_report(secrets)
            self.assertEqual(
                ("incomplete", [{"path": "ci.env", "reason": "unreadable"}, {"path": "clean.md", "reason": "symlink"}]),
                (report["status"], sorted(report["skipped"], key=lambda skip: skip["path"])),
            )
        failing = MagicMock()
        failing.__enter__.return_value = MagicMock(__iter__=MagicMock(side_effect=OSError("device lost")))
        with self.subTest("a file that fails while it is read"), patch.object(Path, "open", return_value=failing):
            report = scan_report(secrets)
            self.assertEqual(
                ("incomplete", {"unreadable"}), (report["status"], {skip["reason"] for skip in report["skipped"]})
            )
        with self.subTest("a file of exactly the per-file limit"), \
                patch("domain_registry.security.MAX_FILE_BYTES", 41):
            self.assertEqual("complete", scan_report(secrets)["status"])
        with self.subTest("one byte over the per-file limit"), \
                patch("domain_registry.security.MAX_FILE_BYTES", 40):
            self.assertEqual(
                [{"path": "ci.env", "reason": "file-too-large"}], scan_report(secrets)["skipped"]
            )
        with self.subTest("the byte budget is inclusive"), patch("domain_registry.security.MAX_SCAN_BYTES", 45):
            self.assertEqual("complete", scan_report(secrets)["status"])
        with self.subTest("one byte over the budget"), patch("domain_registry.security.MAX_SCAN_BYTES", 44):
            self.assertEqual("resource-limit", scan_report(secrets)["skipped"][0]["reason"])
        with self.subTest("the scan stops at the first file past the budget"), \
                patch("domain_registry.security.MAX_SCAN_FILES", 0):
            report = scan_report(secrets)
            self.assertEqual(
                ("incomplete", 1, 0),
                (report["status"], report["skipped_count"], report["scanned_files"]),
            )
        with self.subTest("reported skips are capped but all are counted"), \
                patch("domain_registry.security.MAX_REPORTED_SKIPS", 1), \
                patch("domain_registry.security.MAX_FILE_BYTES", 0):
            report = scan_report(secrets)
            self.assertEqual((1, 2), (len(report["skipped"]), report["skipped_count"]))

    def test_secret_scan_reports_oversized_files_as_incomplete(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        with (secrets / "large.log").open("wb") as output:
            output.truncate(1_048_577)
        report = scan_report(secrets)
        self.assertEqual("incomplete", report["status"])
        self.assertEqual("file-too-large", report["skipped"][0]["reason"])

    def test_readiness_calls_a_repository_without_product_sources_empty(self) -> None:
        result = assess_readiness(self.repo)
        self.assertEqual(result["state"], "empty")
        self.assertEqual(result["next_capability"], "discover")

    def test_readiness_calls_documented_work_without_implementation_greenfield(
        self,
    ) -> None:
        (self.repo / "docs" / "requirements").mkdir(parents=True)
        result = assess_readiness(self.repo)
        self.assertEqual(result["state"], "greenfield")
        self.assertEqual(result["next_capability"], "design")

    def test_readiness_calls_existing_implementation_brownfield(self) -> None:
        implementation = self.repo / "apps" / "orders"
        implementation.mkdir(parents=True)
        (implementation / "main.py").write_text("pass\n", encoding="utf-8")
        result = assess_readiness(self.repo)
        self.assertEqual(result["state"], "brownfield")
        self.assertEqual(result["next_capability"], "read-and-maintain")

    def test_readiness_reports_the_verifier_a_policy_configures_instead_of_recommending_one(self) -> None:
        policy_file = self.repo / "memory" / "domain-memory-policy.json"
        policy = stored_policy_of(self.repo)
        policy["review_governance"].update(verifier="git-signed-commit", trigger="git-push")
        policy_file.write_text(json.dumps(policy), encoding="utf-8")
        self.assertEqual(
            ({"ci_tools": [], "configured_verifier": "git-signed-commit", "configured_trigger": "git-push"},
             {"ci_tools": [], "recommended_verifier": "git-signed-commit", "recommended_trigger": "git-push"}),
            (assess_readiness(self.repo, self.repo / "memory")["governance_candidates"],
             assess_readiness(self.repo)["governance_candidates"]),
        )

    def test_readiness_calls_an_unverifiable_memory_without_sources_dead(self) -> None:
        memory = self.repo / "orphaned-memory"
        memory.mkdir()
        result = assess_readiness(self.repo, memory)
        self.assertEqual(result["state"], "dead")
        self.assertEqual(result["confidence"], "medium")
        self.assertEqual("recover-or-reconfirm", result["next_capability"])
        self.assertTrue(result["blocks"])

    def test_readiness_calls_a_repository_whose_only_test_files_are_git_ignored_empty(self) -> None:
        self.write_files("ignored_test.go")
        self.ignore("*_test.go")
        result = assess_readiness(self.repo)
        self.assertEqual(("empty", "discover"), (result["state"], result["next_capability"]))

    def test_readiness_sends_a_stale_source_map_back_to_reconfirmation(self) -> None:
        self.permit_sources()
        self.write_files("apps/orders/main.py")
        source_map_path = self.selected_docs()
        self.assertEqual(["docs"], load_json(source_map_path)["selected_paths"])
        self.write_files("docs/extra.md")
        result = assess_readiness(self.repo, self.repo / "memory")
        self.assertEqual(
            ("brownfield", "recover-or-reconfirm", {"kind": "registry", "status": "stale"}),
            (result["state"], result["next_capability"], result["signals"][-2]),
        )
        self.assertEqual(
            ["Domain Memory sources require review before they can guide implementation."],
            result["blocks"],
        )

    def test_readiness_reports_a_reviewed_registry_without_records_as_reviewed_empty(self) -> None:
        use_json_layout(self.repo / "memory")
        self.write_files("apps/orders/main.py")
        (self.repo / "docs").mkdir()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs"], stored_policy_of(self.repo)),
        )
        self.edit_registry("manifest.json", lambda manifest: manifest.update(status="reviewed"))
        result = assess_readiness(self.repo, self.repo / "memory")
        self.assertEqual(
            ({"kind": "registry", "status": "reviewed-empty"}, {"kind": "registry_records", "count": 0}, []),
            (result["signals"][-2], result["signals"][-1], result["blocks"]),
        )

    def test_readiness_on_a_root_one_level_too_deep_names_the_likely_root(self) -> None:
        self.committed_source_map()
        result = assess_readiness(self.repo, self.repo / "memory" / "registry")
        self.assertEqual(len(result["blocks"]), 1)
        self.assertIn("points one directory too deep", result["blocks"][0])

    def test_readiness_on_a_memory_without_a_source_map_gives_no_root_hint(self) -> None:
        result = assess_readiness(self.repo, self.repo / "memory")
        self.assertEqual(result["blocks"], ["Domain Memory exists without a source map."])

    def test_readiness_routes_a_valid_registry_with_domain_records_to_greenfield(
        self,
    ) -> None:
        use_json_layout(self.repo / "memory")
        docs = self.repo / "docs"
        docs.mkdir()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [docs]),
        )
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        result = assess_readiness(self.repo, self.repo / "memory")
        self.assertEqual(result["state"], "greenfield")
        self.assertEqual(
            result["signals"][-1], {"kind": "registry_records", "count": 1}
        )

    def test_readiness_reports_an_empty_working_memory_as_current(self) -> None:
        use_json_layout(self.repo / "memory")
        (self.repo / "docs").mkdir()
        implementation = self.repo / "apps" / "orders"
        implementation.mkdir(parents=True)
        (implementation / "main.py").write_text("pass\n", encoding="utf-8")
        self.commit_all("implementation")
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(
                self.repo,
                [self.repo / "docs"],
                stored_policy_of(self.repo),
            ),
        )
        self.commit_all("source map")
        result = assess_readiness(self.repo, self.repo / "memory")

        self.assertEqual(("brownfield", "extract"), (result["state"], result["next_capability"]))
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.commit_all("one record")
        self.assertEqual("read-and-maintain", assess_readiness(self.repo, self.repo / "memory")["next_capability"])
        self.assertEqual(
            {"kind": "registry", "status": "current"}, result["signals"][-2]
        )
        self.assertEqual([], result["blocks"])

    def test_readiness_does_not_treat_a_corrupt_registry_as_empty(self) -> None:
        docs = self.repo / "domain-notes"
        docs.mkdir()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [docs]),
        )
        use_json_layout(self.repo / "memory")
        (self.repo / "memory" / "registry" / "contexts.json").write_text(
            '{"contexts": "broken"}', encoding="utf-8"
        )
        result = assess_readiness(self.repo, self.repo / "memory")
        self.assertEqual(result["state"], "dead")
        self.assertEqual(result["signals"][-2]["status"], "invalid")


if __name__ == "__main__":
    unittest.main()
