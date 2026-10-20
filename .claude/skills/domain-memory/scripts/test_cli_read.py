from __future__ import annotations

import contextlib
import io
import json
import shlex
import shutil
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.cli import main
from domain_registry.common import DOMAIN_MEMORY_HINT
from domain_registry.locate import domain_memory_hint
from domain_registry.evidence import citation
from domain_registry.registry import asset_records
from domain_registry.sources import probe_sources, shown_argument
from test_support import RegistryTestCase, use_json_layout


class CliReadTest(RegistryTestCase):
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
        self.assertEqual((1, f"ERROR: no such record file: {missing.resolve()}"), (result.returncode, result.stderr.strip()))

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

    def test_the_probe_of_a_moved_selection_names_the_commands_that_settle_it(self) -> None:
        self.selection_with_two_sources()
        (self.repo / "docs" / "added.md").write_text("added", encoding="utf-8")
        root = self.repo / "memory"
        summary = self.run_cli("probe", "--repo-root", str(self.repo), "--registry-root", str(root))
        self.assertEqual(
            (1, "Domain Memory at memory: stale. 1 selected sources gained, lost or renamed files (docs); "
                "run refresh-sources, then have a developer run confirm-sources."),
            (summary.returncode, summary.stdout.strip()),
        )

    def test_the_probe_reports_discovery_drift_as_verify_sources_does_and_still_exits_zero(self) -> None:
        self.selection_with_two_sources()
        (self.repo / "tests").mkdir()
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        root = self.repo / "memory"
        summary = self.run_cli("probe", "--repo-root", str(self.repo), "--registry-root", str(root))
        self.assertEqual(
            (0, "Domain Memory at memory: current, agent-asserted, established by git; discovery drifted: "
                f"0 listed paths gone, 1 matching path unlisted; run refresh-listing --registry-root {root.as_posix()} "
                f"--repo-root {self.repo.as_posix()} (no confirmation needed)."),
            (summary.returncode, summary.stdout.strip()),
        )
        advised = summary.stdout.strip().split("; run ")[-1].removesuffix(" (no confirmation needed).")
        refreshed = self.run_cli(*shlex.split(advised))
        after = self.run_cli("probe", "--repo-root", str(self.repo), "--registry-root", str(root))
        self.assertEqual(
            (0, 0, "Domain Memory at memory: current, agent-asserted, established by git."),
            (refreshed.returncode, after.returncode, after.stdout.strip()),
        )

    def test_a_registry_root_beside_the_real_one_is_refused_with_the_real_one_named_and_nothing_created(self) -> None:
        self.committed_source_map()
        typo = self.repo / "memory" / "typo"
        for given, shown in ((typo, "memory/typo"), (self.repo, ".")):
            with self.subTest(shown):
                result = self.run_cli("refresh-listing", "--registry-root", str(given), "--repo-root", str(self.repo))
                self.assertEqual(
                    (1, f"ERROR: no Domain Memory at {shown}: {DOMAIN_MEMORY_HINT}; did you mean memory?\n", False),
                    (result.returncode, result.stderr, typo.exists()),
                )

    def test_the_probe_of_a_root_with_no_domain_memory_nearby_says_to_check_the_option_before_initializing(self) -> None:
        bare = self.repo / "bare"
        bare.mkdir()
        nowhere = bare / "nowhere"
        summary = self.run_cli("probe", "--repo-root", str(bare), "--registry-root", str(nowhere))
        self.assertEqual(
            (2, f"Domain Memory at nowhere: none; {DOMAIN_MEMORY_HINT}."),
            (summary.returncode, summary.stdout.strip()),
        )

    def test_the_probe_of_a_typo_beside_the_registry_echoes_both_paths_alike_and_ends_on_the_question(self) -> None:
        self.committed_source_map()
        typo = self.repo / "memry"
        summary = self.run_cli("probe", "--repo-root", str(self.repo), "--registry-root", str(typo))
        self.assertEqual(
            (2, f"Domain Memory at memry: none; {DOMAIN_MEMORY_HINT}; did you mean memory?"),
            (summary.returncode, summary.stdout.strip()),
        )

    def test_a_sibling_typo_and_the_repository_root_are_pointed_at_the_one_registry_the_repository_holds(self) -> None:
        self.committed_source_map()
        for given, shown in ((self.repo / "memry", "memry"), (self.repo, ".")):
            with self.subTest(shown):
                result = self.run_cli("refresh-listing", "--registry-root", str(given), "--repo-root", str(self.repo))
                self.assertEqual(
                    (1, f"ERROR: no Domain Memory at {shown}: {DOMAIN_MEMORY_HINT}; did you mean memory?\n"),
                    (result.returncode, result.stderr),
                )

    def test_an_unrelated_file_named_source_map_json_is_not_a_domain_memory(self) -> None:
        self.committed_source_map()
        (self.repo / "web" / "build").mkdir(parents=True)
        (self.repo / "web" / "build" / "source-map.json").write_text('{"version": 3, "sources": ["app.ts"]}\n', encoding="utf-8")
        (self.repo / "web" / "notes").mkdir()
        (self.repo / "web" / "notes" / "source-map.json").write_text("not json\n", encoding="utf-8")
        (self.repo / "web" / "other").mkdir()
        (self.repo / "web" / "other" / "source-map.json").write_text('{"format": "something-else/v1"}\n', encoding="utf-8")
        result = self.run_cli("refresh-listing", "--registry-root", str(self.repo), "--repo-root", str(self.repo))
        self.assertEqual(
            (1, f"ERROR: no Domain Memory at .: {DOMAIN_MEMORY_HINT}; did you mean memory?\n"),
            (result.returncode, result.stderr),
        )

    def test_outside_git_a_registry_directory_counts_only_when_it_holds_a_manifest_or_an_index(self) -> None:
        for name, files in (("bare", ("registry/notes.txt",)), ("indexed", ("registry/index.md",)), ("manifested", ("registry/manifest.json",))):
            for relative in files:
                (self.repo / "places" / name / relative).parent.mkdir(parents=True, exist_ok=True)
                (self.repo / "places" / name / relative).write_text("{}", encoding="utf-8")
        places = self.repo.resolve() / "places"
        self.assertEqual(
            f"{DOMAIN_MEMORY_HINT}; Domain Memories in this repository: "
            f"{(places / 'indexed').as_posix()}, {(places / 'manifested').as_posix()}",
            domain_memory_hint(places / "memry"),
        )

    def test_a_repository_with_several_registries_lists_them_including_one_two_levels_down(self) -> None:
        self.committed_source_map()
        deeper = self.repo / "docs" / "dm"
        deeper.mkdir(parents=True)
        shutil.copy(self.repo / "memory" / "source-map.json", deeper / "source-map.json")
        (self.repo / "docs" / "stray").mkdir()
        shutil.copy(self.repo / "memory" / "source-map.json", self.repo / "docs" / "stray" / "foo-source-map.json")
        result = self.run_cli("refresh-listing", "--registry-root", str(self.repo), "--repo-root", str(self.repo))
        self.assertEqual(
            (1, f"ERROR: no Domain Memory at .: {DOMAIN_MEMORY_HINT}; Domain Memories in this repository: docs/dm, memory\n"),
            (result.returncode, result.stderr),
        )

    def test_an_advised_path_is_posix_and_quoted_only_when_it_holds_a_space(self) -> None:
        self.assertEqual(
            ('"docs/with space/memory"', "docs/plain/memory"),
            (shown_argument(Path("docs", "with space", "memory")), shown_argument(Path("docs", "plain", "memory"))),
        )

    def test_the_probe_of_a_selection_changed_only_inside_stays_current_and_points_at_verify_evidence(self) -> None:
        self.selection_with_two_sources()
        (self.repo / "docs" / "kept.md").write_text("edited", encoding="utf-8")
        root = self.repo / "memory"
        summary = self.run_cli("probe", "--repo-root", str(self.repo), "--registry-root", str(root))
        self.assertEqual(
            (0, "Domain Memory at memory: current, agent-asserted, established by hash; "
                "1 selected sources changed inside (docs); verify-evidence checks the citations."),
            (summary.returncode, summary.stdout.strip()),
        )

    def test_a_file_system_error_ends_in_one_error_line_instead_of_a_traceback(self) -> None:
        occupied = self.repo / "occupied.txt"
        occupied.write_text("not a directory\n", encoding="utf-8")
        result = self.run_cli("init-change-package", "--output", str(occupied))
        self.assertEqual(1, result.returncode)
        self.assertNotIn("Traceback", result.stdout + result.stderr)
        self.assertTrue(result.stderr.startswith("ERROR: "), result.stderr)

    def test_init_change_package_into_a_directory_that_holds_a_file_is_refused_and_keeps_the_file(self) -> None:
        occupied = self.repo / "occupied"
        occupied.mkdir()
        (occupied / "note.txt").write_text("kept\n", encoding="utf-8")
        result = self.run_cli("init-change-package", "--output", str(occupied))
        self.assertEqual(
            (1, "", f"ERROR: output directory is not empty: {occupied.resolve()}\n", ["note.txt"]),
            (result.returncode, result.stdout, result.stderr, [entry.name for entry in occupied.iterdir()]),
        )

    def test_a_git_command_that_times_out_ends_in_one_error_line_instead_of_a_traceback(self) -> None:
        arguments = [
            "export", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--commit", "HEAD",
        ]
        stdout, stderr = io.StringIO(), io.StringIO()
        with patch(
            "domain_registry.git.run_bounded", side_effect=subprocess.TimeoutExpired(["git", "ls-tree"], 15)
        ), patch.object(sys, "argv", ["registry_tools", *arguments]), contextlib.redirect_stdout(
            stdout
        ), contextlib.redirect_stderr(stderr):
            exit_code = main()
        self.assertEqual(1, exit_code)
        self.assertEqual("", stdout.getvalue())
        self.assertEqual(
            "ERROR: Command '['git', 'ls-tree']' timed out after 15 seconds\n", stderr.getvalue()
        )

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
                self.assertEqual(message, result.stderr.strip())

    def test_a_committed_json_asset_that_is_not_json_is_named(self) -> None:
        use_json_layout(self.repo / "memory")
        (self.repo / "memory" / "registry" / "rules.json").write_text("not json", encoding="utf-8")
        self.commit_all("broken")
        result = self.run_cli("export", "--registry-root", str(self.repo / "memory"),
                              "--repo-root", str(self.repo), "--commit", "HEAD")
        self.assertEqual(1, result.returncode)
        self.assertTrue(result.stderr.startswith("ERROR: memory/registry/rules.json is not JSON: "), result.stderr)

    def test_the_module_entry_point_runs_the_command_instead_of_exiting_quietly(
        self,
    ) -> None:
        result = self.run_cli("coverage", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["contexts"], 0)

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
            [self.run_cli("get-context", "--registry-root", memory, "--id", "evals").stderr.strip(),
             self.run_cli("get-record", "--registry-root", memory, "--asset", "rules", "--id", "one-live-account-per-mail").stderr.strip(),
             self.run_cli("get-context", "--registry-root", memory, "--id", "billing").stderr.strip(),
             self.run_cli("get-record", "--registry-root", memory, "--asset", "rules", "--id", "one-live").stderr.strip()],
        )

    def test_a_refusal_prints_its_error_on_stderr_and_nothing_on_stdout(self) -> None:
        self.seed("rules.json", [{"id": "order-total", "contexts": ["orders"], "statement": "x"}])
        result = self.run_cli("get-record", "--registry-root", str(self.repo / "memory"), "--asset", "rules", "--id", "nothing")
        self.assertEqual(
            (1, "", "ERROR: rules record not found: nothing\n"), (result.returncode, result.stdout, result.stderr)
        )

    def test_get_record_says_a_candidate_has_no_proving_obligation_yet(self) -> None:
        self.seed("rules.json", [{"id": "r", "contexts": ["orders"], "statement": "x"}])
        shown = self.run_cli("get-record", "--registry-root", str(self.repo / "memory"), "--asset", "rules", "--id", "r",
                             "--changes-root", str(self.repo))
        self.assertEqual(
            (False, "NOTE: rules/r was not reviewed through an applied Change Package, so no test obligation proves it yet."),
            ("proven_by" in json.loads(shown.stdout), shown.stderr.strip()),
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

    def test_a_registry_command_accepts_the_repo_root_flag_it_does_not_use(self) -> None:
        memory = str(self.repo / "memory")
        with_flag = self.run_cli("coverage", "--registry-root", memory, "--repo-root", str(self.repo))
        for command in (
            ("get-context", "--id", "orders"), ("get-record", "--asset", "contexts", "--id", "orders"),
            ("lookup", "--asset", "rules", "--query", "a"), ("resolve-terms", "--query", "a"),
            ("dependents", "--asset", "contexts", "--id", "orders"), ("verify-audit",), ("reconcile-audit",), ("refresh-indexes",),
        ):
            with self.subTest(command[0]):
                result = self.run_cli(command[0], "--registry-root", memory, "--repo-root", str(self.repo), *command[1:])
                self.assertEqual((0, False), (with_flag.returncode, "unrecognized arguments" in result.stderr))

    def test_an_argument_a_command_does_not_take_is_refused_with_that_commands_usage(self) -> None:
        result = self.run_cli("get-context", "--registry-root", str(self.repo / "memory"), "--id", "orders", "--summery")
        usage, error = result.stderr.rstrip().rsplit("\n", 1)
        flat = " ".join(usage.split())
        self.assertEqual(
            (2, True, True, True, False),
            (
                result.returncode,
                flat.startswith("usage: ") and " get-context [-h]" in flat,
                "--id ID [--summary]" in flat,
                error.endswith(" get-context: error: unrecognized arguments: --summery"),
                "[-h] command" in usage,
            ),
        )

    def test_an_empty_query_is_a_usage_error(self) -> None:
        memory = str(self.repo / "memory")
        for command in (("resolve-terms", "--query", " "), ("lookup", "--asset", "rules", "--query", "")):
            with self.subTest(command[0]):
                result = self.run_cli(command[0], "--registry-root", memory, *command[1:])
                self.assertEqual((2, True), (result.returncode, result.stderr.strip().endswith("the query is empty")))


if __name__ == "__main__":
    unittest.main()
