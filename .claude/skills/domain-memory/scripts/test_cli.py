from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import validate_change_package
from domain_registry.cli_parser import build_parser
from domain_registry.git_hooks import governance_readiness, install_pre_push_hook
from domain_registry.store import is_concept_layout, write_document
from test_support import RegistryTestCase, amend_policy, stored_policy_of


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
        self.assertIn("cannot remove a source that is not selected: fact.md", result.stderr)
        self.assertEqual(before, (self.repo / "memory" / "source-map.json").read_bytes())

    def test_refining_takes_a_whole_selection_or_changes_to_it_but_not_both(self) -> None:
        self.selection_with_two_sources()
        for label, arguments in (("both", ("--source", "fact.md", "--add", "adr.md")), ("neither", ())):
            with self.subTest(label):
                result = self.refine(*arguments)
                self.assertEqual(1, result.returncode)
                self.assertIn("with --source, or change it with --add and --remove, not both", result.stderr)

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
            'existing="$(git rev-parse --git-path hooks)/pre-push.domain-memory-existing" || exit 1\n', content
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
        self.assertIn("ERROR", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_a_malformed_json_argument_reports_an_error_instead_of_a_traceback(
        self,
    ) -> None:
        malformed = self.repo / "malformed.json"
        malformed.write_text("{", encoding="utf-8")
        result = self.run_cli("validate-policy", "--policy", str(malformed))
        self.assertEqual(result.returncode, 1)
        self.assertIn("ERROR", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def file_bytes(self) -> dict[str, bytes]:
        return {
            path.relative_to(self.repo).as_posix(): path.read_bytes()
            for path in self.repo.rglob("*")
            if path.is_file()
        }

    def test_every_command_says_in_one_line_what_it_is_for_as_the_reference_does(self) -> None:
        reference = (Path(__file__).parents[1] / "references" / "script-api.md").read_text(encoding="utf-8")
        tasks: dict[str, str] = {}
        for task, command in re.findall(r"^\| ([^|`]+?) \| `([a-z][a-z-]*)", reference, re.MULTILINE):
            tasks.setdefault(command, task.strip())
        [subcommands] = [action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)]
        helps = {choice.dest: choice.help for choice in subcommands._choices_actions}
        self.assertEqual((65, tasks), (len(helps), helps))

    def test_readiness_command_returns_json_without_writing_and_assesses_the_one_domain_memory_it_finds(self) -> None:
        before = self.file_bytes()
        result = self.run_cli("readiness", "--repo-root", str(self.repo))
        after = self.file_bytes()
        report = json.loads(result.stdout)
        self.assertEqual(
            (0, "dead", "memory", ["memory"], before,
             "NOTE: assessing the Domain Memory at memory, the one this repository holds; pass --registry-root to assess another."),
            (result.returncode, report["state"], report["registry_root"], report["domain_memories"], after, result.stderr.strip()),
        )

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
        self.assertIn("not empty", result.stderr)

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


if __name__ == "__main__":
    unittest.main()
