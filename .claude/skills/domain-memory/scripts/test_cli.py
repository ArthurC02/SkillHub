from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
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
from domain_registry.cli_parser import build_parser
from domain_registry.git_hooks import governance_readiness, install_pre_push_hook
from domain_registry.hitl import record_approval
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

    def test_the_command_line_runs_the_obligation_check_when_given_no_command(self) -> None:
        package = self.draft_package()
        check = self.declare_check(package, self.run_writing("cli"))
        completed = subprocess.run(
            [
                sys.executable, str(Path(__file__).resolve().parent / "registry_tools.py"),
                "record-test-result", "--package-root", str(package),
                "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                "--obligation", "OB-1", "--command-profile", "unit",
            ],
            capture_output=True, text=True, check=False,
        )
        self.assertEqual(0, completed.returncode, completed.stderr)
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual((check, "sha256:" + hashlib.sha256(b"cli").hexdigest()), (result["evidence"], result["output_sha256"]))

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
        self.assertEqual((0, "2 candidate records written: 2 created, 0 updated."), (result.returncode, result.stdout.strip()))
        self.assertEqual(["order", "order-line"],
                         [term["id"] for term in asset_records(self.repo / "memory", "vocabulary.json")])

    def test_upsert_candidate_names_a_record_file_that_does_not_exist(self) -> None:
        missing = self.repo / "absent.json"
        result = self.run_cli(
            "upsert-candidate", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--record-file", str(missing),
        )
        self.assertEqual((1, f"ERROR: no such record file: {missing.resolve()}"), (result.returncode, result.stdout.strip()))

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

    def test_export_at_a_commit_prints_what_that_commit_holds(self) -> None:
        committed = {"id": "order-total", "statement": "Committed.", "status": "candidate"}
        self.seed("rules.json", [committed])
        self.commit_all("rule")
        self.seed("rules.json", [{**committed, "statement": "Only in the working tree."}])
        result = self.run_cli("export", "--registry-root", str(self.repo / "memory"),
                              "--repo-root", str(self.repo), "--commit", "HEAD")
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertEqual([committed], json.loads(result.stdout)["rules"]["rules"])

    def test_each_export_at_a_commit_that_cannot_be_read_is_named(self) -> None:
        root = str(self.repo / "memory")
        cases = [
            ("no repository root", ["--commit", "HEAD"], "ERROR: export --commit needs --repo-root"),
            ("unknown commit", ["--repo-root", str(self.repo), "--commit", "f" * 40],
             f"ERROR: no Domain Memory Registry at {'f' * 40}"),
        ]
        for label, extra, message in cases:
            with self.subTest(label):
                result = self.run_cli("export", "--registry-root", root, *extra)
                self.assertEqual(1, result.returncode)
                self.assertEqual(message, result.stdout.strip())

    def test_a_committed_json_asset_that_is_not_json_is_named(self) -> None:
        use_json_layout(self.repo / "memory")
        (self.repo / "memory" / "registry" / "rules.json").write_text("not json", encoding="utf-8")
        self.commit_all("broken")
        result = self.run_cli("export", "--registry-root", str(self.repo / "memory"),
                              "--repo-root", str(self.repo), "--commit", "HEAD")
        self.assertEqual(1, result.returncode)
        self.assertTrue(result.stdout.startswith("ERROR: memory/registry/rules.json is not JSON: "), result.stdout)

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

    def upsert_term(self, term: dict) -> subprocess.CompletedProcess:
        self.add_context()
        path = self.repo / "term.json"
        path.write_text(json.dumps(term), encoding="utf-8")
        return self.run_cli("upsert-candidate", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                            "--asset", "vocabulary", "--record-file", str(path))

    def test_upsert_candidate_says_whether_it_created_or_updated_the_record(self) -> None:
        term = {"id": "order", "name": "Order", "contexts": ["orders"]}
        self.assertEqual(["Candidate record created.", "Candidate record updated."],
                         [self.upsert_term(term).stdout.strip() for _ in range(2)])

    def test_upsert_candidate_notes_a_record_whose_excerpts_are_within_another_records(self) -> None:
        (self.repo / "fact.md").write_text("orders\nlines\ntotals\n", encoding="utf-8")
        first, second, third = (citation(self.repo, "fact.md", line, line) for line in (1, 2, 3))
        order = {"id": "order", "name": "Order", "contexts": ["orders"], "evidence": [first]}
        again = [self.upsert_term(order).stderr.strip() for _ in range(2)]
        wider = self.upsert_term({"id": "purchase", "name": "Purchase", "contexts": ["orders"], "evidence": [first, second]})
        overlapping = self.upsert_term({"id": "line", "name": "Line", "contexts": ["orders"], "evidence": [second, third]})
        self.assertEqual(
            (["", ""], "NOTE: its cited excerpts are all among those of vocabulary/order, or theirs among its; "
             "it may be a duplicate of that record.", ""),
            (again, wider.stderr.strip(), overlapping.stderr.strip()),
        )

    def lookup_note(self, query: str) -> str:
        self.allow_results(10)
        self.seed("rules.json", [{"id": "one-trial", "contexts": ["run"], "statement": "A workspace runs one trial.",
                                  "status": "reviewed"}])
        return self.run_cli("lookup", "--registry-root", str(self.repo / "memory"), "--asset", "rules",
                            "--query", query).stderr.strip()

    def test_lookup_notes_a_query_only_partly_held(self) -> None:
        self.assertEqual('NOTE: no rules record holds every word of "workspace concurrency"; these hold some of them, '
                         "most first.", self.lookup_note("workspace concurrency"))

    def test_lookup_notes_that_finding_nothing_is_not_proof_of_no_constraint(self) -> None:
        self.assertRegex(self.lookup_note("invoices"), '^NOTE: no rules record mentions "invoices" or any of its words. '
                         "That is not proof that no constraint applies")

    def test_lookup_notes_a_query_sharing_only_common_words(self) -> None:
        self.allow_results(10)
        self.seed("rules.json", [{"id": f"r{number}", "contexts": ["run"], "statement": "A skill is served.", "status": "reviewed"}
                                 for number in range(3)])
        result = self.run_cli("lookup", "--registry-root", str(self.repo / "memory"), "--asset", "rules", "--query", "unpublish skill")
        self.assertEqual(
            ([], 'NOTE: the rules records share with "unpublish skill" only words many of them hold, so none is listed. '
                 "That is not proof that no constraint applies: read get-context for the Context you are changing, or "
                 "look up another asset."),
            (json.loads(result.stdout), result.stderr.strip()),
        )

    def test_lookup_finding_every_word_adds_no_note(self) -> None:
        self.assertEqual("", self.lookup_note("trial workspace"))

    def resolve_notes(self, limit: int, *, term: bool) -> str:
        self.allow_results(limit)
        if term:
            self.seed("vocabulary.json", [{"id": "purge", "name": "Purge", "contexts": ["identity"], "status": "reviewed"}])
        self.seed("rules.json", [{"id": "purge-runs-every-step", "contexts": ["identity"], "statement": "x", "status": "reviewed"}])
        self.seed("interactions.json", [{"id": "eval-workspace-purge", "status": "reviewed"}])
        return self.run_cli("resolve-terms", "--registry-root", str(self.repo / "memory"), "--query", "purge").stderr.strip()

    def test_resolve_terms_notes_mentions_cut_by_the_result_limit(self) -> None:
        self.assertEqual('NOTE: "purge" has 2 matches; the first 1 are shown, rules and contracts first among mentions.',
                         self.resolve_notes(1, term=False))

    def test_resolve_terms_holding_exactly_the_result_limit_adds_no_note(self) -> None:
        self.assertEqual("", self.resolve_notes(2, term=False))

    def test_resolve_terms_notes_rules_that_a_matching_term_hides(self) -> None:
        self.assertEqual('NOTE: 1 reviewed rules or contracts mention "purge"; pass --with-mentions to list them.',
                         self.resolve_notes(10, term=True))

    def test_get_context_summary_gives_one_line_per_record_without_evidence(self) -> None:
        self.seed("contexts.json", [{"id": "run", "name": "Run", "responsibility": "Runs trials.", "implementation_path": "src/run"}])
        self.seed("rules.json", [{"id": "one-trial", "contexts": ["run"], "statement": "One trial at a time.", "status": "reviewed",
                                  "evidence": [{"path": "run.go", "lines": {"start": 1, "end": 1}}]}])
        summary = json.loads(self.run_cli("get-context", "--registry-root", str(self.repo / "memory"), "--id", "run",
                                          "--summary").stdout)
        self.assertEqual(
            ({"id": "run", "status": None, "summary": "Runs trials.", "implementation_path": "src/run"},
             [{"id": "one-trial", "status": "reviewed", "summary": "One trial at a time."}]),
            (summary["context"], summary["rules"]),
        )

    def test_an_unknown_id_names_the_nearest_known_ones(self) -> None:
        self.seed("contexts.json", [{"id": "eval", "name": "Eval"}, {"id": "evaluation-batch", "name": "Batch"}, {"id": "run", "name": "Run"}])
        self.seed("rules.json", [{"id": "one-live-account-per-email", "contexts": ["eval"], "statement": "x"}])
        memory = str(self.repo / "memory")
        self.assertEqual(
            ["ERROR: context not found: evals; did you mean eval?",
             "ERROR: rules record not found: one-live-account-per-mail; did you mean one-live-account-per-email?",
             "ERROR: context not found: billing",
             "ERROR: rules record not found: one-live; did you mean one-live-account-per-email?"],
            [self.run_cli("get-context", "--registry-root", memory, "--id", "evals").stdout.strip(),
             self.run_cli("get-record", "--registry-root", memory, "--asset", "rules", "--id", "one-live-account-per-mail").stdout.strip(),
             self.run_cli("get-context", "--registry-root", memory, "--id", "billing").stdout.strip(),
             self.run_cli("get-record", "--registry-root", memory, "--asset", "rules", "--id", "one-live").stdout.strip()],
        )

    def test_an_asset_named_in_the_singular_is_the_plural_asset(self) -> None:
        self.seed("rules.json", [{"id": "r", "contexts": ["orders"], "statement": "x"}])
        self.seed("dependency-policies.json", [{"id": "p", "from_context": "a", "to_context": "b"}])
        memory = str(self.repo / "memory")
        self.assertEqual(
            ["r", "p"],
            [json.loads(self.run_cli("get-record", "--registry-root", memory, "--asset", asset, "--id", identifier).stdout)["id"]
             for asset, identifier in (("rule", "r"), ("dependency-policy", "p"))],
        )

    def test_an_empty_query_is_a_usage_error(self) -> None:
        memory = str(self.repo / "memory")
        for command in (("resolve-terms", "--query", " "), ("lookup", "--asset", "rules", "--query", "")):
            with self.subTest(command[0]):
                result = self.run_cli(command[0], "--registry-root", memory, *command[1:])
                self.assertEqual((2, True), (result.returncode, result.stderr.strip().endswith("the query is empty")))

    def test_every_command_says_in_one_line_what_it_is_for_as_the_reference_does(self) -> None:
        reference = (Path(__file__).parents[1] / "references" / "script-api.md").read_text(encoding="utf-8")
        tasks: dict[str, str] = {}
        for task, command in re.findall(r"^\| ([^|`]+?) \| `([a-z][a-z-]*)", reference, re.MULTILINE):
            tasks.setdefault(command, task.strip())
        [subcommands] = [action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)]
        helps = {choice.dest: choice.help for choice in subcommands._choices_actions}
        self.assertEqual((57, tasks), (len(helps), helps))

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
        self.assertIn("and test, the source authority, has not said they are the right corpus", result.stdout)

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

    def test_citing_a_selected_path_no_discovery_group_classifies_is_selected_and_silent(self) -> None:
        self.cited_file()
        (self.repo / "db").mkdir()
        (self.repo / "db" / "queries.sql").write_text("select 1;\n", encoding="utf-8")
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr", self.repo / "db"]),
        )
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "db/queries.sql",
            "--start", "1", "--end", "1", "--registry-root", str(self.repo / "memory"),
        )
        self.assertEqual(("selected", ""), (json.loads(result.stdout)["source_kind"], result.stderr))

    def test_cite_shows_the_first_and_last_cited_lines_when_asked(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\nthree\n")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "notes.md", "--start", "1", "--end", "3", "--show",
        )
        self.assertEqual(["notes.md:1: one", "notes.md: ... 1 lines not shown", "notes.md:3: three"], result.stderr.splitlines())

    def test_cite_shows_two_adjacent_lines_without_an_elision(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\n")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "notes.md", "--lines", "1-2", "--show",
        )
        self.assertEqual(["notes.md:1: one", "notes.md:2: two"], result.stderr.splitlines())

    def cite(self, *range_options: str) -> subprocess.CompletedProcess:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\nthree\n")
        return self.run_cli("cite", "--repo-root", str(self.repo), "--path", "notes.md", *range_options)

    def test_cite_takes_lines_as_recite_does(self) -> None:
        self.assertEqual(json.loads(self.cite("--start", "2", "--end", "3").stdout), json.loads(self.cite("--lines", "2-3").stdout))

    def test_cite_refuses_both_line_forms_or_neither(self) -> None:
        for options in (("--lines", "1-2", "--start", "1", "--end", "2"), ("--lines", "1-2", "--start", "1"), ("--start", "1"), ()):
            with self.subTest(options):
                self.assertEqual("ERROR: cite takes --lines START-END, or --start and --end",
                                 self.cite(*options).stdout.strip())

    def test_output_lines_end_in_a_line_feed_on_every_platform(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\nthree\n")
        result = subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", "cite", "--repo-root", str(self.repo), "--path", "notes.md",
             "--lines", "1-3", "--show"],
            cwd=Path(__file__).parent, capture_output=True, check=False,
        )
        self.assertEqual((False, False, True), (b"\r\n" in result.stdout, b"\r\n" in result.stderr, b"\n" in result.stderr))

    def test_cite_shows_a_one_line_citation_once(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\n")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "notes.md", "--start", "2", "--end", "2", "--show",
        )
        self.assertEqual(["notes.md:2: two"], result.stderr.splitlines())

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
        self.assertIn("files the confirmed source map does not cover: notes.md", result.stderr)
        self.assertIn("apply-approved-updates refuses to review", result.stderr)
        self.assertEqual("Candidate record created.", result.stdout.strip())

    def upsert_with(self, **fields: object) -> subprocess.CompletedProcess:
        record = self.write_record("context.json", {
            "id": "orders", "name": "Orders", "responsibility": "Own orders.",
            "evidence": [citation(self.repo, "notes.md", 1, 1)], **fields,
        })
        return self.run_cli("upsert-candidate", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                            "--asset", "contexts", "--record-file", str(record))

    def test_upsert_says_when_it_drops_a_review_status_the_record_carried(self) -> None:
        note = ("NOTE: contexts/orders carried a review status, stored as candidate: a record becomes reviewed only "
                "through an approved Change Package.")
        self.confirmed_corpus()
        (self.repo / "notes.md").write_text("Orders owns pricing.", encoding="utf-8")
        for label, fields, expected in (
            ("reviewed", {"status": "reviewed"}, True),
            ("verified", {"verified": True}, True),
            ("candidate", {"status": "candidate"}, False),
            ("no status", {}, False),
        ):
            with self.subTest(label):
                result = self.upsert_with(**fields)
                self.assertEqual((0, expected), (result.returncode, note in result.stderr))

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

    def test_list_proposals_reports_each_package_with_its_status_and_updates(self) -> None:
        changes = self.repo / "changes"
        for name, fields in (
            ("a", {"proposal_id": "a", "status": "draft", "registry_updates": []}),
            ("b", {"proposal_id": "b", "status": "applied", "submitted_at": "2026-10-01T00:00:00Z",
                   "applied_at": "2026-10-02T00:00:00Z", "registry_updates": [{}], "supersedes": ["a"]}),
            ("c", {"proposal_id": "c", "status": "superseded", "submitted_at": "2026-10-01T00:00:00Z",
                   "superseded_at": "2026-10-03T00:00:00Z", "superseded_by": "b"}),
            ("d", {"proposal_id": "d", "status": "superseded", "superseded_at": "2026-10-04T00:00:00Z",
                   "superseded_by": "nosuch"}),
            ("e", {"proposal_id": "e", "status": "draft", "redrafted_at": "2026-10-05T00:00:00Z", "supersedes": ["a", "gone"]}),
        ):
            (changes / name).mkdir(parents=True)
            (changes / name / "domain-change-proposal.json").write_text(json.dumps(fields), encoding="utf-8")
        (changes / "not-a-package").mkdir()
        result = self.run_cli("list-proposals", "--changes-root", str(changes))
        self.assertEqual(
            [{"package": "a", "proposal_id": "a", "status": "draft", "last_changed_at": None, "registry_updates": 0, "supersedes": []},
             {"package": "b", "proposal_id": "b", "status": "applied", "last_changed_at": "2026-10-02T00:00:00Z",
              "registry_updates": 1, "supersedes": ["a"]},
             {"package": "c", "proposal_id": "c", "status": "superseded", "last_changed_at": "2026-10-03T00:00:00Z",
              "registry_updates": 0, "supersedes": [], "superseded_by": "b"},
             {"package": "d", "proposal_id": "d", "status": "superseded", "last_changed_at": "2026-10-04T00:00:00Z",
              "registry_updates": 0, "supersedes": [], "superseded_by": "nosuch", "superseded_by_unknown": True},
             {"package": "e", "proposal_id": "e", "status": "draft", "last_changed_at": "2026-10-05T00:00:00Z",
              "registry_updates": 0, "supersedes": ["a", "gone"], "supersedes_unknown": ["gone"]}],
            json.loads(result.stdout),
        )

    def test_list_proposals_flags_packages_sharing_a_proposal_id(self) -> None:
        changes = self.repo / "changes"
        for name, proposal_id in (("a", "same"), ("b", "same"), ("c", "other")):
            (changes / name).mkdir(parents=True)
            (changes / name / "domain-change-proposal.json").write_text(
                json.dumps({"proposal_id": proposal_id, "status": "draft"}), encoding="utf-8")
        rows = json.loads(self.run_cli("list-proposals", "--changes-root", str(changes)).stdout)
        self.assertEqual([("a", True), ("b", True), ("c", False)],
                         [(row["package"], row.get("duplicate_proposal_id", False)) for row in rows])

    def validated_package(self, *, updates: bool) -> list[str]:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        if not updates:
            proposal["registry_updates"] = []
        path.write_text(json.dumps(proposal), encoding="utf-8")
        result = self.run_cli("validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"))
        return result.stdout.splitlines()

    def test_a_valid_package_that_changes_no_record_says_so(self) -> None:
        self.assertEqual(
            ["Change package is valid.", "NOTE: this package changes no Registry record; add the updates it is for, or supersede it."],
            self.validated_package(updates=False),
        )

    def test_a_preview_before_approval_says_approvals_are_not_asked_for(self) -> None:
        package = self.draft_package()
        result = self.run_cli(
            "validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
            "--as-status", "submitted",
        )
        self.assertEqual(
            "Change package has everything submitted requires, apart from the approvals, which are given only once it is submitted.",
            result.stdout.strip(),
        )

    def test_a_preview_of_an_approved_package_does_not_say_approvals_are_missing(self) -> None:
        package = self.submitted_package()
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        result = self.run_cli(
            "validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory"),
            "--as-status", "submitted",
        )
        self.assertEqual("Change package has everything submitted requires.", result.stdout.strip())

    def test_json_piped_from_a_console_in_another_code_page_is_utf8(self) -> None:
        self.seed("contexts.json", [{"id": "testlab", "name": "Testlab", "responsibility": "Owns drafts\uff0ctheir prompts."}])
        result = subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", "get-context", "--registry-root", str(self.repo / "memory"),
             "--id", "testlab"],
            cwd=Path(__file__).parent,
            capture_output=True,
            env={**os.environ, "PYTHONIOENCODING": "cp950"},
            check=False,
        )
        self.assertEqual("Owns drafts\uff0ctheir prompts.", json.loads(result.stdout.decode("utf-8"))["context"]["responsibility"])

    def test_a_valid_package_with_updates_prints_only_its_verdict(self) -> None:
        self.assertEqual(["Change package is valid."], self.validated_package(updates=True))

    def test_a_shifted_citation_fails_verify_evidence_when_strict(self) -> None:
        self.seed_two_cited_rules()
        result = self.run_cli(
            "verify-evidence", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo), "--strict"
        )
        self.assertEqual(
            (1, "verify-evidence failed: 1 shifted citations; run recite on each to cite its current lines"),
            (result.returncode, result.stderr.strip()),
        )

    def test_strict_passes_when_every_citation_is_current(self) -> None:
        self.seed_two_cited_rules()
        result = self.run_cli(
            "verify-evidence", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--strict", "--path", "other.md",
        )
        self.assertEqual((0, {"current": 1, "shifted": 0}),
                         (result.returncode, {key: json.loads(result.stdout)["summary"][key] for key in ("current", "shifted")}))


if __name__ == "__main__":
    unittest.main()
