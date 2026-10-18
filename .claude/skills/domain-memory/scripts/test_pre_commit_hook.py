from __future__ import annotations

import os
import shutil
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.evidence import citation
from domain_registry.git_hooks import install_pre_commit_hook
from test_support import RegistryTestCase

TOOL = Path(__file__).resolve().parent / "registry_tools.py"
REAL_PYTHON = Path(sys.executable).as_posix()
REFUSED_STALE = "domain-memory pre-commit: a staged file no longer matches what the Registry cites from it"
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
        self.assertIn("memory='memory'\nexisting='.git/hooks/pre-commit.domain-memory-existing'\n", content)

    def test_the_hook_installs_where_a_configured_hooks_path_points(self) -> None:
        (self.repo / ".githooks").mkdir()
        self.git("config", "core.hooksPath", ".githooks")
        hook = install_pre_commit_hook(self.repo / "memory", self.repo, self.repo / "registry_tools.py")
        content = hook.read_text(encoding="utf-8")
        self.assertEqual(self.repo / ".githooks" / "pre-commit", hook)
        self.assertFalse((self.repo / ".git" / "hooks" / "pre-commit").exists())
        self.assertNotIn(self.repo.resolve().as_posix(), content)
        self.assertIn("tool='registry_tools.py'\nmemory='memory'\nexisting='.githooks/pre-commit.domain-memory-existing'\n",
                      content)

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

    def test_a_staged_file_whose_cited_lines_changed_stops_the_commit_naming_the_record(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "changed\ntwo\n")
        result = self.commit_hook(hook)
        self.assertEqual(1, result.returncode, result.stdout + result.stderr)
        for expected in ('"id": "cited-rule"', '"path": "src/a.py"', '"status": "stale"', REFUSED_STALE):
            self.assertIn(expected, result.stderr)

    def test_a_staged_file_whose_cited_lines_are_unchanged_commits(self) -> None:
        self.cited_source()
        hook = self.install()
        self.stage("src/a.py", "one\nchanged\n")
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

    def test_a_commit_staging_only_a_deletion_needs_no_python(self) -> None:
        self.cited_source()
        hook = self.install()
        self.git("rm", "-q", "src/a.py")
        result = self.commit_hook(hook, self.shims("exit 1"))
        self.assertEqual((0, ""), (result.returncode, result.stderr))

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
