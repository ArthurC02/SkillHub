from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.evidence import citation
from domain_registry.git_hooks import EMPTY_SHA, install_pre_commit_hook, install_pre_push_hook
from test_support import RegistryTestCase

TOOL = Path(__file__).resolve().parent / "registry_tools.py"
REAL_PYTHON = Path(sys.executable).as_posix()
REFUSED_STALE = "domain-memory pre-commit: a staged file no longer matches what the Registry cites from it"
FULL_REPORT_OF_A = (
    f"{REFUSED_STALE}; details: verify-evidence --registry-root memory --repo-root . --staged "
    "--path src/a.py prints the full report, as this hook does when DOMAIN_MEMORY_HOOK_VERBOSE is set"
)
STALE_FIRST_LINE_OF_A = (
    "rules/cited-rule: src/a.py:1-1 changed; check that its statement still holds; recite only if it does, "
    "otherwise amend it"
)
MISSING_LINE_OF_A = (
    "rules/cited-rule: src/a.py is not in this commit; relocate-evidence if it moved, "
    "otherwise amend or retire the record\n"
)
REFUSAL_OF_A =f"{STALE_FIRST_LINE_OF_A}\nverify-evidence failed: 1 stale citation\n{FULL_REPORT_OF_A}\n"
EXISTING_FROM_GIT = '="$(git rev-parse --git-path hooks)/pre-commit.domain-memory-existing" || exit 1\n'
NO_PYTHON = "no Python 3.10 or later on PATH as python3 or python"
NOTE = "NOTE: domain-memory pre-commit:"
UNMAPPED_NOTE = (
    f"{NOTE} boundary-impact against HEAD reports unknown; "
    "run boundary-impact --base HEAD to see the Contexts this change touches"
)


class PreCommitHookTest(RegistryTestCase):
    def install(self) -> Path:
        (self.repo / ".git" / "hooks").mkdir(parents=True, exist_ok=True)
        return install_pre_commit_hook(self.repo / "memory", self.repo, TOOL)

    def shims(self, python_body: str, git_body: str | None = None) -> dict[str, str]:
        shims = self.repo.parent / f"{self.repo.name}-shims"
        shims.mkdir()
        self.addCleanup(shutil.rmtree, shims)
        bodies = {"python": python_body, "python3": python_body}
        if git_body is not None:
            bodies["git"] = git_body
        for name, body in bodies.items():
            shim = shims / name
            shim.write_bytes(f"#!/bin/sh\n{body}\n".encode())
            shim.chmod(0o755)
        return {**os.environ, "PATH": str(shims) + os.pathsep + os.environ["PATH"]}

    def working_python(self) -> dict[str, str]:
        return self.shims(f'exec "{REAL_PYTHON}" "$@"')

    def commit_hook(self, hook: Path, environment: dict[str, str] | None = None) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["sh", str(hook)], cwd=self.repo, env=environment or self.working_python(),
            capture_output=True, text=True, encoding="utf-8", check=False,
        )

    def stage(self, relative: str, text: str) -> None:
        path = self.repo / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        self.git("add", relative)

    def rule_citing_first_line_of(self, relative: str) -> None:
        self.seed("rules.json", [{
            "id": "cited-rule", "contexts": ["orders"], "statement": "S.",
            "evidence": [citation(self.repo, relative, 1, 1)],
        }])
        self.commit_all("cite")

    def cited_source(self) -> None:
        (self.repo / "src").mkdir()
        (self.repo / "src" / "a.py").write_text("one\ntwo\n", encoding="utf-8")
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.rule_citing_first_line_of("src/a.py")

    def test_the_hook_installs_where_git_runs_hooks_with_relative_paths_only(self) -> None:
        hook = self.install()
        content = hook.read_bytes().decode("utf-8")
        self.assertEqual(self.repo / ".git" / "hooks" / "pre-commit", hook)
        self.assertEqual("#!/bin/sh\n# domain-memory-pre-commit\n", content[: content.index("tool=")])
        self.assertNotIn("\r", content)
        self.assertIn(f"memory='memory'\nexisting{EXISTING_FROM_GIT}", content)

    def test_the_hook_installs_where_a_configured_hooks_path_points(self) -> None:
        (self.repo / ".githooks").mkdir()
        self.git("config", "core.hooksPath", ".githooks")
        hook = install_pre_commit_hook(self.repo / "memory", self.repo, self.repo / "registry_tools.py")
        content = hook.read_text(encoding="utf-8")
        self.assertEqual(self.repo / ".githooks" / "pre-commit", hook)
        self.assertFalse((self.repo / ".git" / "hooks" / "pre-commit").exists())
        self.assertNotIn(self.repo.resolve().as_posix(), content)
        self.assertIn(f"tool='registry_tools.py'\nmemory='memory'\nexisting{EXISTING_FROM_GIT}", content)

    @unittest.skipUnless(os.name == "posix", "only a POSIX file system records the executable bit")
    def test_the_installed_hook_is_executable(self) -> None:
        self.assertEqual(0o111, self.install().stat().st_mode & 0o111)

    def test_the_command_line_installs_the_hook(self) -> None:
        (self.repo / ".git" / "hooks").mkdir(parents=True, exist_ok=True)
        result = self.run_cli("install-pre-commit-hook", "--registry-root", str(self.repo / "memory"),
                              "--repo-root", str(self.repo))
        hook = self.repo / ".git" / "hooks" / "pre-commit"
        self.assertEqual((0, f"Installed Domain Memory pre-commit hook: {hook.resolve()}\n"),
                         (result.returncode, result.stdout))
        self.assertIn("# domain-memory-pre-commit\n", hook.read_text(encoding="utf-8"))

    def test_a_staged_file_whose_cited_lines_changed_stops_the_commit_with_one_line_per_record_and_no_report(
        self,
    ) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "changed\ntwo\n")
        result = self.commit_hook(hook)
        self.assertEqual((1, REFUSAL_OF_A), (result.returncode, result.stderr))

    def test_the_verbose_variable_adds_the_full_report_after_the_record_lines(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "changed\ntwo\n")
        result = self.commit_hook(hook, {**self.working_python(), "DOMAIN_MEMORY_HOOK_VERBOSE": "1"})
        lines = result.stderr.splitlines()
        self.assertEqual(
            (1, STALE_FIRST_LINE_OF_A, "verify-evidence failed: 1 stale citation", "{", FULL_REPORT_OF_A),
            (result.returncode, lines[0], lines[1], lines[2], lines[-1]),
        )
        self.assertEqual(
            ("rules", "cited-rule", "stale"),
            tuple(json.loads("\n".join(lines[2:-1]))["results"][0][key] for key in ("asset", "id", "status")),
        )

    def test_a_refusal_the_check_prints_as_an_error_is_shown_without_the_verbose_variable(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "changed\ntwo\n")
        environment = self.shims(
            f'test "$2" = verify-evidence && {{ echo "ERROR: index unreadable" >&2; exit 1; }}\nexec "{REAL_PYTHON}" "$@"'
        )
        result = self.commit_hook(hook, environment)
        self.assertEqual((1, f"ERROR: index unreadable\n{FULL_REPORT_OF_A}\n"), (result.returncode, result.stderr))

    def test_a_weakened_line_staged_while_the_disk_holds_the_original_stops_the_commit(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "changed\ntwo\n")
        (self.repo / "src" / "a.py").write_text("one\ntwo\n", encoding="utf-8")
        result = self.commit_hook(hook)
        self.assertEqual((1, REFUSAL_OF_A), (result.returncode, result.stderr))

    def test_an_intact_line_staged_while_the_disk_holds_a_weakened_one_commits(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "one\nadded\n")
        (self.repo / "src" / "a.py").write_text("changed\nadded\n", encoding="utf-8")
        result = self.commit_hook(hook)
        self.assertEqual((0, f"{UNMAPPED_NOTE}\n"), (result.returncode, result.stderr))

    def test_a_stale_citation_of_a_file_left_unstaged_does_not_stop_the_commit(self) -> None:
        self.cited_source()
        (self.repo / "src" / "b.py").write_text("kept\n", encoding="utf-8")
        self.seed("rules.json", [{
            "id": "two-files", "contexts": ["orders"], "statement": "S.",
            "evidence": [citation(self.repo, "src/a.py", 1, 1), citation(self.repo, "src/b.py", 1, 1)],
        }])
        self.commit_all("cite both")
        hook = self.install()
        (self.repo / "src" / "a.py").write_text("changed\ntwo\n", encoding="utf-8")
        self.stage("src/b.py", "kept\nadded\n")
        result = self.commit_hook(hook)
        self.assertEqual((0, f"{UNMAPPED_NOTE}\n"), (result.returncode, result.stderr))

    def test_a_staged_listing_that_fails_stops_the_commit(self) -> None:
        hook = self.install()
        self.stage("notes/uncited.md", "anything\n")
        real_git = Path(shutil.which("git")).as_posix()
        result = self.commit_hook(hook, self.shims("exit 1", f'test "$3" = diff && exit 128\nexec "{real_git}" "$@"'))
        self.assertEqual((1, ""), (result.returncode, result.stderr))

    def test_a_commit_staging_only_uncited_files_needs_no_python(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("notes/uncited.md", "anything\n")
        result = self.commit_hook(hook, self.shims("exit 1"))
        self.assertEqual((0, ""), (result.returncode, result.stderr))

    def test_deleting_a_cited_file_stops_the_commit_and_names_the_record_and_path(self) -> None:
        self.cited_source()
        hook = self.install()
        self.git("rm", "-q", "src/a.py")
        result = self.commit_hook(hook)
        self.assertEqual((1, MISSING_LINE_OF_A + f"verify-evidence failed: 1 missing citation\n{FULL_REPORT_OF_A}\n"),
                         (result.returncode, result.stderr))

    def test_renaming_a_cited_file_stops_the_commit_and_names_the_relocation(self) -> None:
        self.cited_source()
        hook = self.install()
        self.git("mv", "src/a.py", "src/b.py")
        result = self.commit_hook(hook)
        self.assertEqual(
            (1, MISSING_LINE_OF_A + "verify-evidence failed: 1 missing citation\n"
                "src/a.py is renamed to src/b.py in this commit; run relocate-evidence --from src/a.py --to src/b.py\n"
                f"{REFUSED_STALE}; details: verify-evidence --registry-root memory --repo-root . --staged "
                "--path src/a.py --path src/b.py prints the full report, as this hook does when "
                "DOMAIN_MEMORY_HOOK_VERBOSE is set\n"),
            (result.returncode, result.stderr),
        )

    def test_the_rename_hint_does_not_depend_on_the_diff_renames_setting(self) -> None:
        self.cited_source()
        hook = self.install()
        self.git("config", "diff.renames", "false")
        self.git("mv", "src/a.py", "src/b.py")
        result = self.commit_hook(hook)
        self.assertEqual(
            (1, MISSING_LINE_OF_A + "verify-evidence failed: 1 missing citation\n"
                "src/a.py is renamed to src/b.py in this commit; run relocate-evidence --from src/a.py --to src/b.py\n"
                f"{REFUSED_STALE}; details: verify-evidence --registry-root memory --repo-root . --staged "
                "--path src/a.py --path src/b.py prints the full report, as this hook does when "
                "DOMAIN_MEMORY_HOOK_VERBOSE is set\n"),
            (result.returncode, result.stderr),
        )

    def test_a_rename_commits_once_relocate_evidence_has_moved_the_citations(self) -> None:
        self.cited_source()
        hook = self.install()
        self.git("mv", "src/a.py", "src/b.py")
        relocated = self.run_cli("relocate-evidence", "--registry-root", str(self.repo / "memory"),
                                 "--repo-root", str(self.repo), "--from", "src/a.py", "--to", "src/b.py")
        proposal = json.loads(relocated.stdout)
        self.seed("rules.json", [item["record"] for item in proposal["registry_updates"] + proposal["candidates"]])
        result = self.commit_hook(hook)
        self.assertEqual((0, f"{UNMAPPED_NOTE}\n"), (result.returncode, result.stderr))

    def test_a_cited_file_staged_where_no_python_works_stops_the_commit(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "one\nchanged\n")
        result = self.commit_hook(hook, self.shims("exit 1"))
        self.assertEqual((1, f"domain-memory pre-commit: {NO_PYTHON}\n"), (result.returncode, result.stderr))

    def test_a_registry_search_that_fails_stops_the_commit(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("notes/uncited.md", "anything\n")
        real_git = Path(shutil.which("git")).as_posix()
        environment = self.shims("exit 1", f'test "$1" = grep && exit 128\nexec "{real_git}" "$@"')
        result = self.commit_hook(hook, environment)
        self.assertEqual(
            (1, "domain-memory pre-commit: could not search the Registry for the staged files\n"),
            (result.returncode, result.stderr),
        )

    def test_a_staged_file_named_only_in_prose_needs_no_python(self) -> None:
        self.cited_source()
        (self.repo / "memory" / "changes" / "old").mkdir(parents=True)
        (self.repo / "memory" / "changes" / "old" / "requirement-normalization.json").write_text(
            '{"text": "Update README.md and src/a.py.", "path": "README.md"}\n', encoding="utf-8")
        self.seed("rules.json", [{
            "id": "cited-rule", "contexts": ["orders"], "statement": "README.md says so.",
            "evidence": [citation(self.repo, "src/a.py", 1, 1)],
        }])
        self.commit_all("prose")
        hook = self.install()
        self.stage("README.md", "anything\n")
        result = self.commit_hook(hook, self.shims("exit 1"))
        self.assertEqual((0, ""), (result.returncode, result.stderr))

    def test_a_staged_file_whose_path_begins_a_cited_path_needs_no_python(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a", "anything\n")
        result = self.commit_hook(hook, self.shims("exit 1"))
        self.assertEqual((0, ""), (result.returncode, result.stderr))

    def assert_staging_needs_python(self, staged: str) -> None:
        self.commit_all("arranged")
        hook = self.install()
        self.stage(staged, "one\nchanged\n")
        result = self.commit_hook(hook, self.shims("exit 1"))
        self.assertEqual((1, f"domain-memory pre-commit: {NO_PYTHON}\n"), (result.returncode, result.stderr))

    def mixed_with_a_plain_text_citation(self, relative: str) -> None:
        self.cited_source()
        (self.repo / relative).parent.mkdir(parents=True, exist_ok=True)
        (self.repo / relative).write_text("one\ntwo\n", encoding="utf-8")
        self.seed("rules.json", [{
            "id": "mixed", "contexts": ["orders"], "statement": "S.",
            "evidence": [f"{relative}:2", citation(self.repo, relative, 1, 1)],
        }])

    def test_a_file_named_in_the_source_map_needs_python(self) -> None:
        self.cited_source()
        (self.repo / "memory" / "source-map.json").write_text(
            '{"sources": [{"path": "docs/selected.md"}]}\n', encoding="utf-8")
        self.assert_staging_needs_python("docs/selected.md")

    def test_a_plain_path_kept_beside_a_plain_text_citation_needs_python(self) -> None:
        self.mixed_with_a_plain_text_citation("src/b.py")
        self.assert_staging_needs_python("src/b.py")

    def test_a_quoted_path_kept_beside_a_plain_text_citation_needs_python(self) -> None:
        self.mixed_with_a_plain_text_citation(".hidden/b.py")
        self.assert_staging_needs_python(".hidden/b.py")

    def mapped_contexts_with_a_cited_port(self) -> None:
        self.two_mapped_contexts()
        self.rule_citing_first_line_of("src/orders/port.py")

    def test_a_change_across_two_contexts_commits_with_the_boundary_note(self) -> None:
        self.mapped_contexts_with_a_cited_port()
        hook = self.install()
        self.stage("src/orders/port.py", "v1\nadded\n")
        self.stage("src/billing/invoice.py", "v2\n")
        result = self.commit_hook(hook)
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(
            f"{NOTE} boundary-impact against HEAD reports unregistered-co-change; "
            "run boundary-impact --base HEAD to see the Contexts this change touches\n",
            result.stderr,
        )

    def test_a_change_inside_one_context_commits_without_a_note(self) -> None:
        self.mapped_contexts_with_a_cited_port()
        hook = self.install()
        self.stage("src/orders/port.py", "v1\nadded\n")
        result = self.commit_hook(hook)
        self.assertEqual((0, ""), (result.returncode, result.stderr))

    def test_the_first_commit_runs_no_boundary_check(self) -> None:
        self.cited_source()
        hook = self.install()
        self.git("update-ref", "-d", "HEAD")
        result = self.commit_hook(hook)
        self.assertEqual((0, ""), (result.returncode, result.stderr))

    def boundary_impact_replaced_by(self, behaviour: str) -> dict[str, str]:
        return self.shims(f'test "$2" = boundary-impact && {{ {behaviour}; }}\nexec "{REAL_PYTHON}" "$@"')

    def test_a_boundary_check_that_fails_notes_it_and_commits(self) -> None:
        self.mapped_contexts_with_a_cited_port()
        hook = self.install()
        self.stage("src/orders/port.py", "v1\nadded\n")
        result = self.commit_hook(hook, self.boundary_impact_replaced_by("exit 1"))
        self.assertEqual(
            (0, f"{NOTE} boundary-impact against HEAD did not run; the commit goes ahead\n"),
            (result.returncode, result.stderr),
        )

    def test_a_boundary_check_without_a_verdict_notes_that_it_has_none(self) -> None:
        self.mapped_contexts_with_a_cited_port()
        hook = self.install()
        self.stage("src/orders/port.py", "v1\nadded\n")
        result = self.commit_hook(hook, self.boundary_impact_replaced_by("echo '{}'; exit 0"))
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn(f"{NOTE} boundary-impact against HEAD reports no verdict;", result.stderr)

    def existing_hook(self, text: str) -> Path:
        hooks = self.repo / ".git" / "hooks"
        hooks.mkdir(parents=True, exist_ok=True)
        with open(hooks / "pre-commit", "w", encoding="utf-8", newline="\n") as handle:
            handle.write(text)
        (hooks / "pre-commit").chmod(0o755)
        return hooks / "pre-commit.domain-memory-existing"

    def test_an_existing_hook_is_kept_and_runs_before_the_citation_check(self) -> None:
        self.cited_source()
        backup = self.existing_hook("#!/bin/sh\necho existing-hook-refused >&2\nexit 1\n")
        hook = self.install()
        self.stage("src/a.py", "changed\ntwo\n")
        result = self.commit_hook(hook)
        self.assertEqual((1, "existing-hook-refused\n"), (result.returncode, result.stderr))
        self.assertEqual("#!/bin/sh\necho existing-hook-refused >&2\nexit 1\n", backup.read_text(encoding="utf-8"))

    def test_an_existing_hook_that_passes_lets_the_citation_check_run(self) -> None:
        self.cited_source()
        self.existing_hook("#!/bin/sh\nexit 0\n")
        hook = self.install()
        self.stage("src/a.py", "changed\ntwo\n")
        result = self.commit_hook(hook)
        self.assertEqual(1, result.returncode)
        self.assertIn(REFUSED_STALE, result.stderr)

    def linked_worktree(self) -> Path:
        linked = self.repo.parent / f"{self.repo.name}-linked"
        self.addCleanup(shutil.rmtree, linked, ignore_errors=True)
        self.git("worktree", "add", "-q", str(linked))
        return linked

    def test_an_existing_hook_refusing_in_a_linked_worktree_stops_the_commit(self) -> None:
        self.existing_hook("#!/bin/sh\necho existing-hook-refused >&2\nexit 1\n")
        hook = self.install()
        linked = self.linked_worktree()
        result = subprocess.run(
            ["sh", str(hook)], cwd=linked, env=self.working_python(),
            capture_output=True, text=True, encoding="utf-8", check=False,
        )
        self.assertEqual((1, "existing-hook-refused\n"), (result.returncode, result.stderr))

    def test_an_existing_pre_push_hook_refusing_in_a_linked_worktree_stops_the_push(self) -> None:
        hooks = self.repo / ".git" / "hooks"
        hooks.mkdir(parents=True, exist_ok=True)
        with open(hooks / "pre-push", "w", encoding="utf-8", newline="\n") as handle:
            handle.write("#!/bin/sh\necho existing-push-refused >&2\nexit 1\n")
        (hooks / "pre-push").chmod(0o755)
        hook = install_pre_push_hook(self.repo / "memory", self.repo, TOOL)
        linked = self.linked_worktree()
        result = subprocess.run(
            ["sh", str(hook), "origin", "https://example.com/repo.git"], cwd=linked, input="",
            capture_output=True, text=True, encoding="utf-8", check=False,
        )
        self.assertEqual((1, "existing-push-refused\n"), (result.returncode, result.stderr))

    def test_the_pre_push_hook_takes_the_git_timeout_from_git_config_unless_the_environment_sets_it(self) -> None:
        (self.repo / ".git" / "hooks").mkdir(parents=True, exist_ok=True)
        hook = install_pre_push_hook(self.repo / "memory", self.repo, TOOL)
        self.stage("memory/registry/rules.json", "{}\n")
        subprocess.run(
            ["git", "-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false",
             "commit", "-qm", "registry change"], cwd=self.repo, check=True,
        )
        head = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True,
        ).stdout.strip()
        shims = self.shims('case $1 in -c) exit 0 ;; esac\necho "timeout=$DOMAIN_MEMORY_GIT_TIMEOUT" >&2\nexit 1')
        for label, configured, environment, expected in (
            ("config only", "42", {}, "timeout=42\n"),
            ("environment wins", "42", {"DOMAIN_MEMORY_GIT_TIMEOUT": "7"}, "timeout=7\n"),
            ("neither", None, {}, "timeout=\n"),
        ):
            with self.subTest(label):
                subprocess.run(["git", "config", "--unset", "domain-memory.git-timeout"], cwd=self.repo, check=False)
                if configured is not None:
                    subprocess.run(["git", "config", "domain-memory.git-timeout", configured], cwd=self.repo, check=True)
                inherited = {name: value for name, value in shims.items() if name != "DOMAIN_MEMORY_GIT_TIMEOUT"}
                result = subprocess.run(
                    ["sh", str(hook), "origin", "https://example.com/repo.git"], cwd=self.repo,
                    input=f"refs/heads/main {head} refs/heads/main {EMPTY_SHA}\n",
                    env={**inherited, **environment}, capture_output=True, text=True, encoding="utf-8", check=False,
                )
                self.assertEqual((1, expected), (result.returncode, result.stderr))

    def test_a_hooks_directory_git_cannot_name_stops_the_commit(self) -> None:
        hook = self.install()
        self.stage("notes/uncited.md", "anything\n")
        real_git = Path(shutil.which("git")).as_posix()
        result = self.commit_hook(hook, self.shims("exit 1", f'test "$1" = rev-parse && exit 128\nexec "{real_git}" "$@"'))
        self.assertEqual((1, ""), (result.returncode, result.stderr))

    def test_a_chained_hook_that_is_not_executable_stops_the_commit(self) -> None:
        hooks = self.repo / ".git" / "hooks"
        hooks.mkdir(parents=True, exist_ok=True)
        with open(hooks / "pre-commit", "w", encoding="utf-8", newline="\n") as handle:
            handle.write("exit 0\n")
        (hooks / "pre-commit").chmod(0o644)
        result = self.commit_hook(self.install())
        self.assertEqual(
            (1, "domain-memory pre-commit: the chained hook .git/hooks/pre-commit.domain-memory-existing is not "
                "executable, so it would not run; make it executable (chmod +x) or delete it\n"),
            (result.returncode, result.stderr),
        )

    def test_installing_again_replaces_only_its_own_hook(self) -> None:
        backup = self.existing_hook("#!/bin/sh\nexit 0\n")
        self.install()
        hook = self.install()
        self.assertEqual("#!/bin/sh\nexit 0\n", backup.read_text(encoding="utf-8"))
        self.assertIn("# domain-memory-pre-commit\n", hook.read_text(encoding="utf-8"))

    def test_a_foreign_hook_is_refused_when_a_backup_already_exists(self) -> None:
        self.existing_hook("#!/bin/sh\nexit 0\n")
        self.install()
        (self.repo / ".git" / "hooks" / "pre-commit").write_text("#!/bin/sh\necho other\n", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "^existing Domain Memory pre-commit hook backup already exists$"):
            self.install()


if __name__ == "__main__":
    unittest.main()
