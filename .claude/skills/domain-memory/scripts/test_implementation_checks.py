from __future__ import annotations

import contextlib
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.cli import main
from domain_registry import counterfactual as counterfactual_module
from domain_registry.counterfactual import (
    RESTORE_DELAYS_SECONDS,
    SIDECAR_SUFFIX,
    Mutation,
    Proof,
    TestRun,
    counterfactual,
    decoded,
    verdict_of,
)
from domain_registry.gates import KINDS, quality_gates
from domain_registry.sources import is_test_file

TOOLS = Path(__file__).parent / "registry_tools.py"
RUN_TESTS = f'"{sys.executable}" -m unittest discover -s tests -t .'
RULE = """LIMIT = 200
CAP = 12


def refused(length):
    return length > LIMIT


def fee(amount):
    return min(amount, CAP)
"""
RULE_TEST = """import unittest

from rule import refused


class RefusalTest(unittest.TestCase):
    def test_the_limit_itself_is_accepted(self):
        self.assertFalse(refused(200))

    def test_one_past_the_limit_is_refused(self):
        self.assertTrue(refused(201))
"""


class RepositoryCase(unittest.TestCase):
    def setUp(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.repo = Path(temporary.name).resolve()

    def write(self, name: str, content: str = "") -> Path:
        path = self.repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content.encode("utf-8"))
        return path


class QualityGatesTest(RepositoryCase):
    def gates(self) -> list[dict[str, str]]:
        return quality_gates(self.repo)["gates"]

    def test_a_repository_with_nothing_configured_has_no_standard(self) -> None:
        self.write("src/app.py", "x = 1\n")
        result = quality_gates(self.repo)
        self.assertEqual(result["standard"], "none")
        self.assertEqual(result["gates"], [])
        self.assertEqual(result["kinds_missing"], list(KINDS))

    def test_tests_alone_are_counted_and_are_not_a_standard(self) -> None:
        self.write("tests/test_app.py", "")
        result = quality_gates(self.repo)
        self.assertEqual(result["test_files"], 1)
        self.assertEqual(result["standard"], "none")

    def test_a_pipeline_alone_is_reported_and_is_not_a_standard(self) -> None:
        self.write(".gitlab-ci.yml", "")
        result = quality_gates(self.repo)
        self.assertEqual(result["kinds_present"], ["ci"])
        self.assertEqual(result["standard"], "none")

    def test_a_configuration_file_is_a_gate_at_its_path(self) -> None:
        self.write("apps/api/ruff.toml", "")
        self.assertEqual(
            self.gates(),
            [{"kind": "lint", "tool": "ruff", "path": "apps/api/ruff.toml"}],
        )
        self.assertEqual(quality_gates(self.repo)["standard"], "configured")

    def test_a_lint_configuration_with_dependency_rules_is_also_an_architecture_gate(self) -> None:
        self.write(".golangci.yml", "linters-settings:\n  depguard:\n    rules: {}\n")
        self.assertEqual(
            [{"kind": "lint", "tool": "golangci-lint", "path": ".golangci.yml"},
             {"kind": "architecture", "tool": "depguard", "path": ".golangci.yml"}],
            self.gates(),
        )

    def test_a_lint_configuration_that_only_names_the_dependency_linter_is_not_an_architecture_gate(self) -> None:
        self.write(".golangci.yaml", "linters:\n  disable:\n    - depguard\n")
        self.assertEqual(["lint"], [entry["kind"] for entry in self.gates()])

    def git(self, *arguments: str) -> None:
        subprocess.run(["git", "-C", str(self.repo), *arguments], capture_output=True, check=True)

    def test_a_hook_git_runs_is_a_gate_and_samples_and_copies_are_not(self) -> None:
        self.git("init", "-q")
        for name in ("pre-push", "pre-push.domain-memory-existing", "pre-commit.sample"):
            self.write(f".git/hooks/{name}", "#!/bin/sh\n")
        self.assertEqual([{"kind": "hooks", "tool": "git:pre-push", "path": ".git/hooks/pre-push"}], self.gates())

    def test_hooks_are_read_where_core_hooks_path_points(self) -> None:
        self.git("init", "-q")
        self.git("config", "core.hooksPath", ".githooks")
        self.write(".githooks/pre-commit", "#!/bin/sh\n")
        self.write(".git/hooks/pre-push", "#!/bin/sh\n")
        self.assertEqual([{"kind": "hooks", "tool": "git:pre-commit", "path": ".githooks/pre-commit"}], self.gates())

    def test_a_configuration_file_is_recognised_by_the_start_of_its_name(self) -> None:
        self.write("eslint.config.mjs", "")
        self.assertEqual(
            self.gates(),
            [{"kind": "lint", "tool": "eslint", "path": "eslint.config.mjs"}],
        )

    def test_a_type_checker_alone_is_a_standard(self) -> None:
        self.write("mypy.ini", "")
        result = quality_gates(self.repo)
        self.assertEqual(result["kinds_present"], ["types"])
        self.assertEqual(result["standard"], "configured")

    def test_a_shared_manifest_counts_only_the_tools_it_configures(self) -> None:
        self.write("pyproject.toml", "[project]\nname = 'x'\n\n[tool.ruff.lint]\n")
        self.assertEqual(
            self.gates(),
            [{"kind": "lint", "tool": "ruff", "path": "pyproject.toml"}],
        )

    def test_a_shared_manifest_that_configures_no_tool_is_not_a_gate(self) -> None:
        self.write("pyproject.toml", "[project]\nname = 'x'\n")
        self.assertEqual(self.gates(), [])

    def test_a_package_script_is_a_gate(self) -> None:
        self.write("package.json", json.dumps({"scripts": {"lint": "x", "dev": "y"}}))
        self.assertEqual(
            self.gates(),
            [{"kind": "lint", "tool": "script:lint", "path": "package.json"}],
        )

    def test_a_package_file_that_cannot_be_read_is_not_a_gate(self) -> None:
        self.write("package.json", "{not json")
        self.assertEqual(self.gates(), [])

    def test_a_package_manifest_with_scripts_that_is_not_an_object_is_not_a_gate(self) -> None:
        self.write("package.json", json.dumps({"scripts": "lint"}))
        self.assertEqual(self.gates(), [])

    def test_a_package_manifest_that_is_not_an_object_is_not_a_gate(self) -> None:
        self.write("package.json", "[]")
        self.assertEqual(self.gates(), [])

    def test_a_package_manifest_that_is_not_utf8_is_not_a_gate(self) -> None:
        (self.repo / "package.json").write_bytes(
            b'{"name": "\xa4\x40", "scripts": {"lint": "x"}}'
        )
        self.assertEqual(self.gates(), [])

    def test_a_project_manifest_that_is_not_utf8_is_not_a_gate(self) -> None:
        (self.repo / "pyproject.toml").write_bytes(b"# \xa4\x40\n[tool.ruff]\n")
        self.assertEqual(self.gates(), [])

    def test_a_dependency_directory_is_not_searched(self) -> None:
        self.write("node_modules/pkg/.eslintrc.json", "")
        self.assertEqual(self.gates(), [])

    def test_a_fixture_directory_is_not_searched(self) -> None:
        for fixture in ("fixtures", "__fixtures__", "testdata"):
            self.write(f"evals/{fixture}/sample/ruff.toml", "")
        self.assertEqual(self.gates(), [])

    def test_a_directory_named_like_a_fixture_is_still_searched(self) -> None:
        self.write("src/fixtures_loader/ruff.toml", "")
        self.assertEqual(
            self.gates(),
            [{"kind": "lint", "tool": "ruff", "path": "src/fixtures_loader/ruff.toml"}],
        )

    def test_tests_inside_a_fixture_are_not_counted(self) -> None:
        self.write("tests/test_app.py", "")
        self.write("tests/fixtures/sample/test_sample.py", "")
        self.assertEqual(quality_gates(self.repo)["test_files"], 1)

    def test_a_nested_checkout_is_not_searched(self) -> None:
        self.write("checkouts/copy/.git", "gitdir: elsewhere\n")
        self.write("checkouts/copy/ruff.toml", "")
        self.assertEqual(self.gates(), [])

    def test_a_nested_checkout_whose_git_is_a_directory_is_not_searched(self) -> None:
        self.write("checkouts/copy/.git/HEAD", "ref: refs/heads/main\n")
        self.write("checkouts/copy/ruff.toml", "")
        self.assertEqual(self.gates(), [])

    def test_tests_in_a_nested_checkout_whose_git_is_a_directory_are_not_counted(self) -> None:
        self.write("tests/test_app.py", "")
        self.write("tests/copy/.git/HEAD", "ref: refs/heads/main\n")
        self.write("tests/copy/test_app.py", "")
        self.assertEqual(quality_gates(self.repo)["test_files"], 1)

    def test_tests_in_a_nested_checkout_are_not_counted(self) -> None:
        self.write("tests/test_app.py", "")
        self.write("tests/copy/.git", "gitdir: elsewhere\n")
        self.write("tests/copy/test_app.py", "")
        self.assertEqual(quality_gates(self.repo)["test_files"], 1)

    def test_the_repository_is_searched_beside_its_own_checkout_marker(self) -> None:
        self.write(".git", "gitdir: elsewhere\n")
        self.write("ruff.toml", "")
        self.assertEqual(
            self.gates(), [{"kind": "lint", "tool": "ruff", "path": "ruff.toml"}]
        )

    def test_a_missing_repository_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "not a directory"):
            quality_gates(self.repo / "absent")

    def test_test_files_are_recognised_by_name(self) -> None:
        names = {
            "test_orders.py": True,
            "orders_test.py": True,
            "orders_test.go": True,
            "orders.test.ts": True,
            "orders.spec.ts": True,
            "orders.py": False,
            "test_orders.txt": False,
            "contest.py": False,
        }
        for name, expected in names.items():
            with self.subTest(name):
                self.assertEqual(is_test_file(name), expected)


class GitIgnoredGatesTest(RepositoryCase):
    def setUp(self) -> None:
        super().setUp()
        subprocess.run(["git", "init", "-q"], cwd=self.repo, check=True)

    def ignore(self, *patterns: str) -> None:
        self.write(".gitignore", "".join(f"{pattern}\n" for pattern in patterns))

    def test_a_git_ignored_configuration_file_is_not_a_gate_and_a_git_ignored_test_is_not_counted(self) -> None:
        self.ignore("scratch/")
        self.write("scratch/ruff.toml", "")
        self.write("scratch/test_x.py", "")
        self.write("ruff.toml", "")
        self.write("test_kept.py", "")
        result = quality_gates(self.repo)
        self.assertEqual(
            [{"kind": "lint", "tool": "ruff", "path": "ruff.toml"}], result["gates"]
        )
        self.assertEqual(1, result["test_files"])

    def test_a_git_ignored_non_ascii_directory_holds_no_gate_and_no_counted_test(self) -> None:
        self.ignore("資料/")
        self.write("資料/ruff.toml", "")
        self.write("資料/test_x.py", "")
        self.write("test_kept.py", "")
        result = quality_gates(self.repo)
        self.assertEqual(([], 1), (result["gates"], result["test_files"]))

    def test_a_non_ascii_and_a_spaced_configuration_path_are_gates_beside_an_ignored_one(self) -> None:
        self.ignore("資料/")
        self.write("資料/mypy.ini", "")
        self.write("café/ruff.toml", "")
        self.write("my docs/mypy.ini", "")
        self.assertEqual(
            [
                {"kind": "lint", "tool": "ruff", "path": "café/ruff.toml"},
                {"kind": "types", "tool": "mypy", "path": "my docs/mypy.ini"},
            ],
            quality_gates(self.repo)["gates"],
        )


class CounterfactualTest(RepositoryCase):
    def setUp(self) -> None:
        super().setUp()
        self.rule = self.write("rule.py", RULE)
        self.write("tests/__init__.py")
        self.write("tests/test_rule.py", RULE_TEST)

    def break_rule(self, find: str, replace: str, **options) -> dict:
        return counterfactual(
            self.repo, Mutation(Path("rule.py"), find, replace), Proof(RUN_TESTS, **options)
        )

    def assert_untouched(self) -> None:
        self.assertEqual(self.rule.read_bytes(), RULE.encode("utf-8"))
        self.assertFalse((self.repo / ("rule.py" + SIDECAR_SUFFIX)).exists())

    def test_a_rule_with_a_test_is_killed_and_the_file_is_restored(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = 201")
        self.assertEqual(result["verdict"], "killed")
        self.assertEqual(result["line"], 1)
        self.assertIn("FAILED", result["failing_evidence"])
        self.assertTrue(result["restoration_result"]["restored"])
        self.assert_untouched()

    def test_a_rule_without_a_test_survives_and_the_file_is_restored(self) -> None:
        result = self.break_rule("CAP = 12", "CAP = 13")
        self.assertEqual(result["verdict"], "survived")
        self.assertEqual(result["line"], 2)
        self.assert_untouched()

    def test_a_run_that_does_not_finish_is_inconclusive(self) -> None:
        self.write(
            "tests/test_slow.py",
            "import time\nimport unittest\n\nfrom rule import LIMIT\n\n\n"
            "class Slow(unittest.TestCase):\n"
            "    def test_waits_when_broken(self):\n"
            "        time.sleep(0 if LIMIT == 200 else 30)\n",
        )
        result = self.break_rule("LIMIT = 200", "LIMIT = 201", timeout=5)
        self.assertEqual(result["verdict"], "inconclusive")
        self.assert_untouched()

    def test_text_that_is_absent_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "occurs 0 times"):
            self.break_rule("LIMIT = 300", "LIMIT = 301")
        self.assert_untouched()

    def test_text_that_occurs_twice_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "occurs 2 times"):
            self.break_rule("amount", "total")
        self.assert_untouched()

    def test_a_replacement_that_changes_nothing_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "is the text it replaces"):
            self.break_rule("LIMIT = 200", "LIMIT = 200")

    def test_tests_that_fail_before_anything_is_broken_are_refused(self) -> None:
        self.write(
            "tests/test_red.py",
            "import unittest\n\n\nclass Red(unittest.TestCase):\n"
            "    def test_red(self):\n        self.fail('already red')\n",
        )
        with self.assertRaisesRegex(ValueError, "prove nothing"):
            self.break_rule("LIMIT = 200", "LIMIT = 201")
        self.assert_untouched()

    def test_a_file_outside_the_repository_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "outside the repository"):
            counterfactual(
                self.repo / "tests",
                Mutation(Path("../rule.py"), "LIMIT = 200", "LIMIT = 201"),
                Proof(RUN_TESTS),
            )
        self.assert_untouched()

    def test_a_file_that_does_not_exist_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "does not exist"):
            counterfactual(
                self.repo, Mutation(Path("absent.py"), "a", "b"), Proof(RUN_TESTS)
            )

    def test_an_interrupted_earlier_run_is_refused_until_it_is_restored(self) -> None:
        sidecar = self.write("rule.py" + SIDECAR_SUFFIX, RULE)
        with self.assertRaisesRegex(ValueError, "restore from it"):
            self.break_rule("LIMIT = 200", "LIMIT = 201")
        self.assertEqual(self.rule.read_bytes(), RULE.encode("utf-8"))
        self.assertTrue(sidecar.exists())

    def test_a_copy_left_beside_another_file_is_refused_too(self) -> None:
        sidecar = self.write("other/module.py" + SIDECAR_SUFFIX, "x")
        with self.assertRaisesRegex(ValueError, "other/module.py" + SIDECAR_SUFFIX):
            self.break_rule("LIMIT = 200", "LIMIT = 201")
        self.assertEqual(self.rule.read_bytes(), RULE.encode("utf-8"))
        self.assertTrue(sidecar.exists())

    def test_a_copy_inside_an_excluded_directory_is_not_searched(self) -> None:
        self.write("node_modules/pkg/index.js" + SIDECAR_SUFFIX, "x")
        self.assertEqual(self.break_rule("LIMIT = 200", "LIMIT = 201")["verdict"], "killed")

    def break_with_failing_restores(self, failures: int) -> dict:
        real_write = counterfactual_module.write_as_newer
        calls = []

        def write(target: Path, content: bytes, seconds_ahead: int) -> None:
            calls.append(content)
            if 1 < len(calls) <= 1 + failures:
                raise OSError(22, "Invalid argument")
            real_write(target, content, seconds_ahead)

        with patch.object(counterfactual_module, "write_as_newer", write), patch.object(
            counterfactual_module.time, "sleep"
        ):
            return self.break_rule("LIMIT = 200", "LIMIT = 201")

    def test_a_restore_that_succeeds_on_its_last_retry_restores_the_file(self) -> None:
        result = self.break_with_failing_restores(len(RESTORE_DELAYS_SECONDS))
        self.assertTrue(result["restoration_result"]["restored"])
        self.assert_untouched()

    def test_a_restore_that_never_succeeds_names_the_copy_to_restore_from(self) -> None:
        with self.assertRaisesRegex(ValueError, "rule.py" + SIDECAR_SUFFIX):
            self.break_with_failing_restores(len(RESTORE_DELAYS_SECONDS) + 1)
        self.assertEqual(
            (self.repo / ("rule.py" + SIDECAR_SUFFIX)).read_bytes(), RULE.encode("utf-8")
        )

    def test_broken_code_that_does_not_build_is_inconclusive(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = (")
        self.assertEqual(("inconclusive", True), (result["verdict"], "did not build" in result["reason"]))
        self.assert_untouched()

    def test_an_expected_pattern_decides_even_when_the_code_does_not_build(self) -> None:
        self.assertEqual("killed", self.break_rule("LIMIT = 200", "LIMIT = (", expect="SyntaxError")["verdict"])

    def test_every_verdict_gives_its_reason(self) -> None:
        for find, replace, options, reason in (
            ("LIMIT = 200", "LIMIT = 201", {}, "the tests failed once the protection was broken"),
            ("LIMIT = 200", "LIMIT = 201", {"expect": "AssertionError"},
             "the tests failed with output matching --expect AssertionError"),
            ("CAP = 12", "CAP = 13", {}, "the tests still pass with the protection broken"),
        ):
            with self.subTest(reason):
                self.assertEqual(reason, self.break_rule(find, replace, **options)["reason"])

    def test_a_failure_matching_the_expected_pattern_is_killed(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = 201", expect="AssertionError")
        self.assertEqual(result["verdict"], "killed")

    def test_a_failure_not_matching_the_expected_pattern_is_inconclusive(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = (", expect="AssertionError")
        self.assertEqual(result["verdict"], "inconclusive")
        self.assertNotIn("counterfactual_check", result)
        self.assert_untouched()

    def test_an_expected_pattern_that_is_not_a_regular_expression_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "--expect is not a regular expression"):
            self.break_rule("LIMIT = 200", "LIMIT = 201", expect="(")
        self.assert_untouched()

    def test_the_command_runs_without_a_shell(self) -> None:
        with self.assertRaisesRegex(ValueError, "could not start"):
            counterfactual(
                self.repo,
                Mutation(Path("rule.py"), "LIMIT = 200", "LIMIT = 201"),
                Proof("no-such-program-for-counterfactual --version"),
            )
        self.assert_untouched()

    def test_the_command_runs_in_the_directory_cwd_names(self) -> None:
        command = f'"{sys.executable}" -m unittest discover -s . -t ..'
        result = counterfactual(
            self.repo, Mutation(Path("rule.py"), "LIMIT = 200", "LIMIT = 201"), Proof(command, cwd="tests")
        )
        self.assertEqual(result["verdict"], "killed")

    def test_a_cwd_outside_the_repository_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "--cwd must name a directory inside the repository"):
            self.break_rule("LIMIT = 200", "LIMIT = 201", cwd="..")

    def test_a_killed_rule_prints_the_check_a_change_package_records(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = 201", obligation_id="limit-refused")
        self.assertEqual(
            {
                "status": "passed",
                "obligation_id": "limit-refused",
                "mutated_protection": "rule.py:1: LIMIT = 200 -> LIMIT = 201",
                "failure_evidence": result["failing_evidence"],
                "restored": True,
            },
            result["counterfactual_check"],
        )

    def test_a_multi_line_mutation_is_shown_on_one_line(self) -> None:
        result = self.break_rule("def refused(length):\n    return length > LIMIT", "def refused(length):\n\treturn length >= LIMIT")
        self.assertEqual(
            "rule.py:5: def refused(length): return length > LIMIT -> def refused(length): return length >= LIMIT",
            result["counterfactual_check"]["mutated_protection"],
        )

    def test_the_check_names_a_placeholder_until_an_obligation_is_given(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = 201")
        self.assertEqual(result["counterfactual_check"]["obligation_id"], "<obligation-id>")

    def test_a_surviving_rule_prints_no_check(self) -> None:
        self.assertNotIn("counterfactual_check", self.break_rule("CAP = 12", "CAP = 13"))


class VerdictTest(unittest.TestCase):
    def test_a_compiler_refusal_is_inconclusive_and_an_assertion_is_killed(self) -> None:
        for output, verdict in (
            ("./purge.go:12:2: declared and not used: ready", "inconclusive"),
            ('./email.go:4:2: "strings" imported and not used', "inconclusive"),
            ("FAIL\tskillhub/identity [build failed]", "inconclusive"),
            ("--- FAIL: TestPurge (0.00s)\n    purge_test.go:40: want refusal", "killed"),
        ):
            with self.subTest(output):
                self.assertEqual(verdict, verdict_of(TestRun(1, output))[0])


class DecodedOutputTest(unittest.TestCase):
    def test_utf8_output_is_read_as_utf8(self) -> None:
        self.assertEqual(decoded("中".encode()), "中")

    def test_other_output_is_read_in_the_system_encoding(self) -> None:
        with patch.object(counterfactual_module.locale, "getpreferredencoding", return_value="cp950"):
            self.assertEqual(decoded("中".encode("cp950")), "中")


class CommandLineTest(RepositoryCase):
    def run_tool(self, *arguments: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(TOOLS), *arguments],
            capture_output=True,
            text=True,
            encoding="utf-8",
            check=False,
        )

    def counterfactual_run(self, find: str, replace: str) -> subprocess.CompletedProcess[str]:
        self.write("rule.py", RULE)
        self.write("tests/__init__.py")
        self.write("tests/test_rule.py", RULE_TEST)
        return self.run_tool(
            "counterfactual",
            "--repo-root",
            str(self.repo),
            "--file",
            "rule.py",
            "--find",
            find,
            "--replace",
            replace,
            "--test-command",
            RUN_TESTS,
        )

    def test_a_killed_rule_exits_zero(self) -> None:
        completed = self.counterfactual_run("LIMIT = 200", "LIMIT = 201")
        self.assertEqual((0, "killed"), (completed.returncode, json.loads(completed.stdout)["verdict"]))

    def test_a_surviving_rule_exits_one(self) -> None:
        completed = self.counterfactual_run("CAP = 12", "CAP = 13")
        self.assertEqual((1, "survived"), (completed.returncode, json.loads(completed.stdout)["verdict"]))

    def test_an_inconclusive_run_exits_two(self) -> None:
        self.write("rule.py", RULE)
        runs = [TestRun(0, "intact"), TestRun(None, "no result within 1 seconds")]
        arguments = [
            "counterfactual", "--repo-root", str(self.repo), "--file", "rule.py",
            "--find", "LIMIT = 200", "--replace", "LIMIT = 201", "--test-command", "unused",
        ]
        output = io.StringIO()
        with patch("domain_registry.counterfactual.run_tests", side_effect=runs), patch.object(
            sys, "argv", ["registry_tools", *arguments]
        ), contextlib.redirect_stdout(output):
            exit_code = main()
        self.assertEqual((2, "inconclusive"), (exit_code, json.loads(output.getvalue())["verdict"]))

    def test_an_error_the_console_cannot_encode_is_still_printed(self) -> None:
        completed = subprocess.run(
            [sys.executable, str(TOOLS), "counterfactual", "--repo-root", str(self.repo),
             "--file", "\U0001F600.py", "--find", "a", "--replace", "b", "--test-command", "unused"],
            capture_output=True,
            env={**os.environ, "PYTHONIOENCODING": "cp950", "PYTHONUTF8": "0"},
            check=False,
        )
        self.assertEqual(
            (1, b"ERROR: file does not exist: \\U0001f600.py"),
            (completed.returncode, completed.stdout.strip()),
        )

    def test_quality_gates_prints_its_format(self) -> None:
        completed = self.run_tool("quality-gates", "--repo-root", str(self.repo))
        self.assertEqual(completed.returncode, 0)
        self.assertEqual(
            json.loads(completed.stdout)["format"], "domain-memory-quality-gates/v1"
        )

    def test_quality_gates_prints_the_gates_it_found(self) -> None:
        self.write("ruff.toml", "")
        completed = self.run_tool("quality-gates", "--repo-root", str(self.repo))
        self.assertEqual(
            [{"kind": "lint", "tool": "ruff", "path": "ruff.toml"}],
            json.loads(completed.stdout)["gates"],
        )


if __name__ == "__main__":
    unittest.main()
