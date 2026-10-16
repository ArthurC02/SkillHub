from __future__ import annotations

import hashlib
import json
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import (
    validate_change_package,
)
from domain_registry.evidence import (
    citation,
    digest,
)
from domain_registry.git_hooks import governance_readiness, install_pre_push_hook
from domain_registry.registry import (
    asset_records,
)
from domain_registry.store import is_concept_layout, write_document
from domain_registry.sources import (
    probe_sources,
    selected_source_map,
    write_source_map,
)
from test_support import RegistryTestCase, amend_policy, stored_policy_of, use_json_layout


class CliTest(RegistryTestCase):
    def test_adding_a_source_keeps_the_rest_of_the_selection(self) -> None:
        self.selection_with_two_sources()
        result = self.refine("--add", "fact.md", "--add", "adr.md")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(["adr.md", "docs", "fact.md"], stored_policy_of(self.repo)["source_policy"]["selected_paths"])

    def test_removing_a_source_keeps_the_rest_of_the_selection(self) -> None:
        self.selection_with_two_sources()
        self.refine("--add", "fact.md")
        result = self.refine("--remove", "docs")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(["fact.md"], stored_policy_of(self.repo)["source_policy"]["selected_paths"])

    def test_removing_a_source_that_is_not_selected_changes_nothing(self) -> None:
        self.selection_with_two_sources()
        before = (self.repo / "memory" / "source-map.json").read_bytes()
        result = self.refine("--remove", "fact.md")
        self.assertEqual(1, result.returncode)
        self.assertIn("cannot remove a source that is not selected: fact.md", result.stdout)
        self.assertEqual(before, (self.repo / "memory" / "source-map.json").read_bytes())

    def test_refining_takes_a_whole_selection_or_changes_to_it_but_not_both(self) -> None:
        self.selection_with_two_sources()
        for label, arguments in (("both", ("--source", "fact.md", "--add", "adr.md")), ("neither", ())):
            with self.subTest(label):
                result = self.refine(*arguments)
                self.assertEqual(1, result.returncode)
                self.assertIn("with --source, or change it with --add and --remove, not both", result.stdout)

    def test_the_command_line_records_the_run_it_was_given(self) -> None:
        package = self.draft_package()
        completed = subprocess.run(
            [
                sys.executable, str(Path(__file__).resolve().parent / "registry_tools.py"),
                "record-test-result", "--package-root", str(package),
                "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                "--obligation", "OB-1", "--command", self.run_writing("cli"),
                "--command-profile", "unit",
            ],
            capture_output=True, text=True, check=False,
        )
        self.assertEqual(0, completed.returncode, completed.stderr)
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual("sha256:" + hashlib.sha256(b"cli").hexdigest(), result["output_sha256"])

    def test_pre_push_hook_checks_each_domain_memory_commit(self) -> None:
        (self.repo / ".git" / "hooks").mkdir(parents=True, exist_ok=True)
        existing = self.repo / ".git" / "hooks" / "pre-push"
        existing.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        hook = install_pre_push_hook(
            self.repo / "docs" / "domain-memory", self.repo, self.repo / "registry_tools.py"
        )
        content = hook.read_text(encoding="utf-8")
        self.assertIn("git rev-list $range -- \"$memory\"", content)
        self.assertIn("verify-git-governance", content)
        self.assertEqual(
            "#!/bin/sh\nexit 0\n",
            (self.repo / ".git" / "hooks" / "pre-push.domain-memory-existing").read_text(encoding="utf-8"),
        )

    def test_the_hook_installs_where_a_configured_hooks_path_points(self) -> None:
        amend_policy(
            self.repo / "memory", "authorized_signers", "SHA256:AAA", "Register a key."
        )
        amend_policy(
            self.repo / "memory", "review_trigger", "git-push", "Enforce on push."
        )
        amend_policy(
            self.repo / "memory",
            "review_verifier",
            "git-signed-commit",
            "The maintainer signs.",
        )
        (self.repo / ".githooks").mkdir()
        subprocess.run(
            ["git", "config", "core.hooksPath", ".githooks"], cwd=self.repo, check=True
        )
        self.assertEqual(
            ["Git HITL pre-push hook is not installed on this machine"],
            governance_readiness(self.repo / "memory", self.repo)["blocks"],
        )
        hook = install_pre_push_hook(
            self.repo / "memory", self.repo, self.repo / "registry_tools.py"
        )
        self.assertEqual(self.repo / ".githooks" / "pre-push", hook)
        self.assertFalse((self.repo / ".git" / "hooks" / "pre-push").exists())
        self.assertEqual(
            [], governance_readiness(self.repo / "memory", self.repo)["blocks"]
        )

    def test_the_generated_hook_carries_no_machine_specific_path(self) -> None:
        (self.repo / ".githooks").mkdir()
        subprocess.run(
            ["git", "config", "core.hooksPath", ".githooks"], cwd=self.repo, check=True
        )
        (self.repo / ".githooks" / "pre-push").write_text(
            "#!/bin/sh\nexit 0\n", encoding="utf-8"
        )
        hook = install_pre_push_hook(
            self.repo / "memory", self.repo, self.repo / "registry_tools.py"
        )
        content = hook.read_bytes().decode("utf-8")
        self.assertNotIn("\r", content)
        self.assertNotIn(self.repo.resolve().as_posix(), content)
        self.assertIn("tool='registry_tools.py'", content)
        self.assertIn(
            "existing='.githooks/pre-push.domain-memory-existing'", content
        )

    def test_an_existing_hook_reading_stdin_does_not_disarm_the_check(self) -> None:
        amend_policy(
            self.repo / "memory", "authorized_signers", "SHA256:NOBODY", "A key."
        )
        amend_policy(self.repo / "memory", "review_trigger", "git-push", "On push.")
        amend_policy(
            self.repo / "memory",
            "review_verifier",
            "git-signed-commit",
            "The maintainer signs.",
        )
        hooks = self.repo / ".git" / "hooks"
        hooks.mkdir(parents=True, exist_ok=True)
        with open(hooks / "pre-push", "w", encoding="utf-8", newline="\n") as handle:
            handle.write("#!/bin/sh\ncat >/dev/null\nexit 0\n")
        (hooks / "pre-push").chmod(0o755)
        hook = install_pre_push_hook(
            self.repo / "memory",
            self.repo,
            Path(__file__).resolve().parent / "registry_tools.py",
        )
        write_document(
            self.repo / "memory", "rules.json",
            {"format": "domain-rules/v1", "status": "candidate", "rules": []},
        )
        subprocess.run(["git", "add", "memory"], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "-c", "user.name=T", "-c", "user.email=t@example.com",
             "-c", "commit.gpgsign=false", "commit", "-qm", "unsigned domain memory change"],
            cwd=self.repo, check=True,
        )
        head = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo,
            capture_output=True, text=True, check=True,
        ).stdout.strip()
        parent = subprocess.run(
            ["git", "rev-parse", "HEAD~1"], cwd=self.repo,
            capture_output=True, text=True, check=True,
        ).stdout.strip()
        result = subprocess.run(
            ["sh", str(hook), "origin", "https://example.com/repo.git"],
            cwd=self.repo,
            input=f"refs/heads/main {head} refs/heads/main {parent}\n",
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(0, result.returncode, result.stdout + result.stderr)
        self.assertIn(
            f"Git commit signature is invalid: {head} carries no signature",
            result.stdout + result.stderr,
        )

    def test_a_chained_hook_that_is_not_executable_stops_the_push(self) -> None:
        hooks = self.repo / ".git" / "hooks"
        hooks.mkdir(parents=True, exist_ok=True)
        with open(hooks / "pre-push", "w", encoding="utf-8", newline="\n") as handle:
            handle.write("exit 0\n")
        (hooks / "pre-push").chmod(0o644)
        hook = install_pre_push_hook(
            self.repo / "memory", self.repo, Path(__file__).resolve().parent / "registry_tools.py"
        )
        result = subprocess.run(
            ["sh", str(hook), "origin", "https://example.com/repo.git"],
            cwd=self.repo, input="", capture_output=True, text=True, check=False,
        )
        self.assertEqual(1, result.returncode, result.stdout + result.stderr)
        self.assertIn(
            "domain-memory pre-push: the chained hook .git/hooks/pre-push.domain-memory-existing is not "
            "executable, so it would not run; make it executable (chmod +x) or delete it",
            result.stderr,
        )

    def test_boundary_impact_runs_from_the_command_line(self) -> None:
        self.two_mapped_contexts()
        (self.repo / "src/billing/invoice.py").write_text("v2\n", encoding="utf-8")
        result = self.run_cli("boundary-impact", "--registry-root", str(self.repo / "memory"),
                              "--repo-root", str(self.repo), "--base", "HEAD")
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertEqual("no-cross-context-change", json.loads(result.stdout)["verdict"])

    def test_a_package_started_with_its_identifiers_carries_them_in_every_file(self) -> None:
        package = self.repo / "named"
        result = self.run_cli(
            "init-change-package", "--output", str(package),
            "--requirement-id", "REQ-7", "--proposal-id", "PRO-7",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        documents = self.package_documents(package)
        self.assertEqual(
            {
                "requirement-normalization.json": ("REQ-7", None),
                "domain-change-proposal.json": ("REQ-7", "PRO-7"),
                "test-obligations.json": ("REQ-7", "PRO-7"),
                "evidence-bundle.json": ("REQ-7", "PRO-7"),
            },
            {
                name: (document.get("requirement_id"), document.get("proposal_id"))
                for name, document in documents.items()
            },
        )
        errors = "; ".join(validate_change_package(package, self.repo / "memory", "submitted"))
        for gone in ("must reference the requirement_id", "must reference the proposal_id",
                     "requires a completed requirement_id", "requires a completed proposal_id"):
            with self.subTest(gone=gone):
                self.assertNotIn(gone, errors)

    def test_initialization_does_not_take_a_developer_identity(self) -> None:
        result = self.run_cli(
            "init-domain-memory",
            "--repo-root",
            str(self.repo),
            "--output",
            str(self.repo / "elsewhere"),
            "--source",
            str(self.repo / "docs"),
            "--storage-mode",
            "ignored",
            "--data-classification",
            "internal",
            "--review-mode",
            "local-draft-only",
            "--source-authority",
            "test",
            "--confirmed-by",
            "Arthur",
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("--confirmed-by", result.stderr)
        self.assertFalse((self.repo / "elsewhere").exists())

    def test_the_confirm_command_moves_the_map_through_the_command_line(self) -> None:
        self.committed_source_map()
        self.permit_sources()
        result = self.run_cli(
            "confirm-sources",
            "--registry-root",
            str(self.repo / "memory"),
            "--repo-root",
            str(self.repo),
            "--confirmed-by",
            "Arthur",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Arthur", result.stdout)
        stored = json.loads(
            (self.repo / "memory" / "source-map.json").read_text(encoding="utf-8")
        )
        self.assertEqual(stored["selection_status"], "developer-confirmed")

    def test_the_confirm_command_reports_a_refusal_instead_of_a_traceback(self) -> None:
        self.committed_source_map()
        self.permit_sources()
        result = self.run_cli(
            "confirm-sources",
            "--registry-root",
            str(self.repo / "memory"),
            "--repo-root",
            str(self.repo),
            "--confirmed-by",
            "<identity>",
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("ERROR", result.stdout)
        self.assertNotIn("Traceback", result.stderr)

    def test_a_malformed_json_argument_reports_an_error_instead_of_a_traceback(
        self,
    ) -> None:
        malformed = self.repo / "malformed.json"
        malformed.write_text("{", encoding="utf-8")
        result = self.run_cli("validate-policy", "--policy", str(malformed))
        self.assertEqual(result.returncode, 1)
        self.assertIn("ERROR", result.stdout)
        self.assertNotIn("Traceback", result.stderr)

    def test_resolve_terms_answers_each_repeated_query_in_order(self) -> None:
        self.defined_term()
        result = self.run_cli(
            "resolve-terms", "--registry-root", str(self.repo / "memory"),
            "--query", "test case", "--query", "budget", "--query", "a run",
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(
            [("test case", "run", "definition"), ("a run", "run", "name")],
            [(match["query"], match["id"], match["match_kind"]) for match in json.loads(result.stdout)],
        )

    def test_upsert_candidate_takes_record_file_more_than_once_from_the_command_line(self) -> None:
        use_json_layout(self.repo / "memory")
        order, line = self.look_alike_terms()
        result = self.run_cli(
            "upsert-candidate", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "vocabulary", "--record-file", str(order), "--record-file", str(line),
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(["order", "order-line"],
                         [term["id"] for term in asset_records(self.repo / "memory", "vocabulary.json")])

    def test_the_probe_on_a_root_one_level_too_deep_names_the_likely_root(self) -> None:
        self.committed_source_map()
        deeper = self.repo / "memory" / "registry"
        result = probe_sources(self.repo, deeper / "source-map.json")
        self.assertEqual("absent", result["status"])
        self.assertIn(f"{self.repo / 'memory' / 'source-map.json'} exists", result["hint"])
        summary = self.run_cli("probe", "--repo-root", str(self.repo), "--registry-root", str(deeper))
        self.assertEqual(2, summary.returncode)
        self.assertIn("points one directory too deep", summary.stdout)
        self.assertNotIn("Nothing is initialized here", summary.stdout)

    def test_a_file_system_error_ends_in_one_error_line_instead_of_a_traceback(self) -> None:
        occupied = self.repo / "occupied.txt"
        occupied.write_text("not a directory\n", encoding="utf-8")
        result = self.run_cli("init-change-package", "--output", str(occupied))
        self.assertEqual(1, result.returncode)
        self.assertNotIn("Traceback", result.stdout + result.stderr)
        self.assertTrue(result.stdout.startswith("ERROR: "), result.stdout)

    def test_export_prints_every_asset_document_by_asset_name(self) -> None:
        rule = {"id": "order-total", "statement": "An order total is never negative.", "status": "candidate"}
        self.seed("rules.json", [rule])
        result = self.run_cli("export", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(0, result.returncode, result.stdout)
        exported = json.loads(result.stdout)
        self.assertEqual(
            {"contexts", "vocabulary", "aggregates", "rules", "contracts", "interactions", "decisions",
             "events", "capabilities", "value-objects", "dependency-policies"},
            set(exported),
        )
        self.assertEqual([rule], exported["rules"]["rules"])
        self.assertEqual("domain-rules/v1", exported["rules"]["format"])

    def test_the_module_entry_point_runs_the_command_instead_of_exiting_quietly(
        self,
    ) -> None:
        result = self.run_cli("coverage", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["contexts"], 0)

    def file_bytes(self) -> dict[str, bytes]:
        return {
            path.relative_to(self.repo).as_posix(): path.read_bytes()
            for path in self.repo.rglob("*")
            if path.is_file()
        }

    def test_readiness_command_returns_json_without_writing(self) -> None:
        before = self.file_bytes()
        result = self.run_cli("readiness", "--repo-root", str(self.repo))
        after = self.file_bytes()
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["state"], "empty")
        self.assertEqual(before, after)

    def test_initializing_into_an_occupied_destination_is_refused(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "rules.md").write_text("rules", encoding="utf-8")
        result = self.run_cli(
            "init-domain-memory",
            "--repo-root",
            str(self.repo),
            "--output",
            str(self.repo / "memory"),
            "--source",
            str(self.repo / "docs"),
            "--storage-mode",
            "ignored",
            "--data-classification",
            "internal",
            "--review-mode",
            "local-draft-only",
            "--source-authority",
            "test",
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("not empty", result.stdout)

    def test_initializing_a_fresh_destination_writes_the_registry_and_its_policy(
        self,
    ) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "rules.md").write_text("rules", encoding="utf-8")
        destination = self.repo / "domain-memory"
        result = self.run_cli(
            "init-domain-memory",
            "--repo-root",
            str(self.repo),
            "--output",
            str(destination),
            "--source",
            str(self.repo / "docs"),
            "--storage-mode",
            "ignored",
            "--data-classification",
            "internal",
            "--review-mode",
            "local-draft-only",
            "--source-authority",
            "test",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((destination / "source-map.json").is_file())
        self.assertTrue(is_concept_layout(destination))
        self.assertTrue((destination / "registry" / "contexts").is_dir())
        policy = json.loads((destination / "domain-memory-policy.json").read_text(encoding="utf-8"))
        self.assertEqual(
            ("ignored", "internal", "local-draft-only", "test", ["docs"]),
            (
                policy["storage_mode"],
                policy["data_classification"],
                policy["review_mode"],
                policy["source_policy"]["authority"],
                policy["source_policy"]["selected_paths"],
            ),
        )

    def test_the_cite_command_reports_the_kind_of_source_the_lines_came_from(
        self,
    ) -> None:
        self.cited_file()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr"]),
        )
        result = self.run_cli(
            "cite",
            "--repo-root",
            str(self.repo),
            "--path",
            "docs/adr/boundaries.md",
            "--start",
            "2",
            "--end",
            "2",
            "--registry-root",
            str(self.repo / "memory"),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["source_kind"], "decisions")
        self.assertEqual("", result.stderr)

    def test_citing_outside_the_confirmed_sources_says_review_will_refuse_it(self) -> None:
        self.cited_file()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr"]),
        )
        (self.repo / "db").mkdir()
        (self.repo / "db" / "queries.sql").write_text("select 1;\n", encoding="utf-8")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "db/queries.sql",
            "--start", "1", "--end", "1", "--registry-root", str(self.repo / "memory"),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        reference = json.loads(result.stdout)
        self.assertEqual("unclassified", reference["source_kind"])
        self.assertEqual({"path", "lines", "content_sha256", "excerpt_sha256", "source_kind"}, set(reference))
        self.assertIn("db/queries.sql is outside the confirmed sources", result.stderr)
        self.assertIn("apply-approved-updates refuses to review", result.stderr)

    def test_the_upsert_command_says_review_will_refuse_an_unconfirmed_file(self) -> None:
        self.confirmed_corpus()
        (self.repo / "notes.md").write_text("Orders owns pricing.", encoding="utf-8")
        record = self.write_record(
            "context.json",
            {
                "id": "orders",
                "name": "Orders",
                "responsibility": "Own orders.",
                "evidence": [citation(self.repo, "notes.md", 1, 1)],
            },
        )
        result = self.run_cli(
            "upsert-candidate", "--registry-root", str(self.repo / "memory"),
            "--repo-root", str(self.repo), "--asset", "contexts", "--record-file", str(record),
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("files the confirmed source map does not cover: notes.md", result.stdout)
        self.assertIn("apply-approved-updates refuses to review", result.stdout)

    def test_recovery_reports_that_nothing_was_interrupted_or_pending(self) -> None:
        result = self.run_cli(
            "recover-registry-update", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo)
        )
        self.assertEqual(
            (0, ["No interrupted Registry update.", "No pending apply to reconcile."]),
            (result.returncode, result.stdout.splitlines()),
        )

    def test_recovery_without_a_repo_root_says_a_pending_apply_remains(self) -> None:
        self.pending_marker("superseded", applied=False)
        result = self.run_cli("recover-registry-update", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(1, result.returncode)
        self.assertIn("ERROR: a pending apply remains; rerun with --repo-root", result.stdout)
        self.assertTrue(self.marker().exists())

    def test_recovery_with_a_repo_root_reconciles_a_pending_apply(self) -> None:
        self.pending_marker("superseded", applied=False)
        result = self.run_cli(
            "recover-registry-update", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo)
        )
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertIn("Pending apply reconciled: PRO-1 was never applied", result.stdout)
        self.assertFalse(self.marker().exists())

    def test_reciting_from_the_command_line_prints_the_update(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--lines", "3",
        )
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertEqual(["PRO-1"], json.loads(result.stdout)["supersedes"])
        self.assertEqual("", result.stderr)

    def test_reciting_onto_an_earlier_output_file_merges_into_it(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        earlier = self.repo / "earlier.json"
        earlier.write_text(json.dumps({"registry_updates": [], "supersedes": ["PRO-9"], "candidates": []}), encoding="utf-8")
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--lines", "3", "--onto", str(earlier),
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        self.assertEqual(["PRO-1", "PRO-9"], json.loads(result.stdout)["supersedes"])

    def test_reciting_without_lines_re_cites_the_replaced_lines_of_the_same_file(self) -> None:
        self.cited_rule("reviewed", (1, 1), (2, 2))
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--replace", "src/a.py:2-2",
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        evidence = json.loads(result.stdout)["registry_updates"][0]["record"]["evidence"]
        one = digest((self.repo / "src" / "a.py").read_bytes().splitlines(keepends=True)[1])
        self.assertEqual(
            [({"start": 1, "end": 1}, one), ({"start": 2, "end": 2}, one)],
            [(reference["lines"], reference["excerpt_sha256"]) for reference in evidence],
        )

    def test_reciting_without_lines_is_refused_unless_replace_names_lines_of_the_same_file(self) -> None:
        self.cited_rule("reviewed", (1, 1), (2, 2))
        (self.repo / "src" / "b.py").write_text("one\n", encoding="utf-8")
        for name, replace in (
            ("no replace", ()),
            ("replace without lines", ("--replace", "src/a.py")),
            ("replace in another file", ("--replace", "src/b.py:1-1")),
        ):
            with self.subTest(name):
                result = self.run_cli(
                    "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                    "--asset", "rules", "--id", "rule", "--path", "src/a.py", *replace,
                )
                self.assertEqual(1, result.returncode, result.stdout)
                self.assertIn("recite needs --lines, unless --replace names lines of the same file", result.stdout)

    def test_reciting_lines_that_repeat_in_the_file_says_to_widen_them(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        (self.repo / "src" / "a.py").write_text("zero\none\ntwo\ntwo\n", encoding="utf-8")
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--lines", "3",
        )
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertIn("the text of src/a.py:3-3 also starts at line 4", result.stderr)
        self.assertIn("Widen the citation to the enclosing block", result.stderr)

    def test_citing_lines_that_repeat_in_the_file_says_where_else_they_start(self) -> None:
        (self.repo / "fact.md").write_text("same\nother\nsame\nsame\n", encoding="utf-8")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "fact.md", "--start", "3", "--end", "3",
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual({"start": 3, "end": 3}, json.loads(result.stdout)["lines"])
        self.assertIn("the text of fact.md:3-3 also starts at line 1, 4", result.stderr)

    def test_a_package_started_without_identifiers_names_the_flags_still_blank(self) -> None:
        result = self.run_cli("init-change-package", "--output", str(self.repo / "blank"))
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("Still blank: --requirement-id, --proposal-id, --proposer;", result.stdout)

    def test_a_package_started_with_every_identifier_prints_nothing(self) -> None:
        result = self.run_cli(
            "init-change-package", "--output", str(self.repo / "full"),
            "--requirement-id", "REQ-1", "--proposal-id", "PRO-1", "--proposer", "dev",
        )
        self.assertEqual((0, ""), (result.returncode, result.stdout))

    def test_a_shifted_citation_does_not_fail_verify_evidence(self) -> None:
        self.seed_two_cited_rules()
        result = self.run_cli("verify-evidence", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        self.assertEqual((0, 1), (result.returncode, json.loads(result.stdout)["summary"]["shifted"]))


if __name__ == "__main__":
    unittest.main()
