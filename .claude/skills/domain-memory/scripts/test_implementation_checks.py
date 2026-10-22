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
    OUTPUT_TAIL,
    FailingBaselineError,
    RESTORE_DELAYS_SECONDS,
    SIDECAR_SUFFIX,
    Mutation,
    Proof,
    TestRun,
    counterfactual,
    counterfactual_suite,
    decoded,
    failing_tests,
    failure_evidence,
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

    def test_a_pipeline_is_a_gate_at_its_path(self) -> None:
        self.write(".gitlab-ci.yml", "")
        self.assertEqual([{"kind": "ci", "tool": "gitlab-ci", "path": ".gitlab-ci.yml"}], self.gates())

    def test_a_go_module_with_tests_beneath_it_is_a_test_gate(self) -> None:
        self.write("apps/api/go.mod", "module api\n")
        self.write("apps/api/internal/run_test.go", "package internal\n")
        self.write("apps/cli/go.mod", "module cli\n")
        self.write("apps/cli-tools/x_test.go", "package x\n")
        self.write("apps/cli/testdata/y_test.go", "package y\n")
        self.write("testdata/go.mod", "module fixture\n")
        self.write("testdata/a_test.go", "package a\n")
        self.assertEqual([{"kind": "tests", "tool": "go-test", "path": "apps/api/go.mod"}], self.gates())

    def test_a_root_go_module_owns_every_test(self) -> None:
        self.write("go.mod", "module root\n")
        self.write("pkg/a_test.go", "package a\n")
        self.assertEqual([{"kind": "tests", "tool": "go-test", "path": "go.mod"}], self.gates())

    def test_taskfile_tasks_named_for_a_check_are_gates(self) -> None:
        self.write("Taskfile.yml", (
            "version: '3'\nvars:\n  test: x\ntasks:\n  build:\n    cmd: go build\n  lint:\n    cmd: x\n"
            "  'test:go':\n    cmds:\n      - go test ./...\n  typecheck:\n    cmd: x\nincludes:\n  lint-more: x\n"
        ))
        self.assertEqual(
            [{"kind": "lint", "tool": "task:lint", "path": "Taskfile.yml"},
             {"kind": "tests", "tool": "task:test:go", "path": "Taskfile.yml"},
             {"kind": "types", "tool": "task:typecheck", "path": "Taskfile.yml"}],
            self.gates(),
        )

    def test_a_taskfile_without_tasks_is_no_gate(self) -> None:
        self.write("Taskfile.yml", "version: '3'\nvars:\n  test: x\n")
        self.assertEqual([], self.gates())

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

    def test_a_hook_the_repository_ships_but_git_does_not_run_is_a_gate_not_enabled(self) -> None:
        self.git("init", "-q")
        self.write(".githooks/pre-push", "#!/bin/sh\n")
        self.write(".githooks/pre-push.domain-memory-existing", "#!/bin/sh\n")
        self.write("scripts/update", "")
        self.assertEqual(
            [{"kind": "hooks", "tool": "git:pre-push", "path": ".githooks/pre-push", "enabled": False}], self.gates()
        )

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
        self.assertIn("FAILED", result["failure_evidence"])
        self.assertEqual(["tests.test_rule.RefusalTest.test_one_past_the_limit_is_refused"], result["failing_tests"])
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

    def test_a_suite_judges_each_mutation_and_restores_the_file_between_them(self) -> None:
        suite = {"test_command": RUN_TESTS, "mutations": [
            {"file": "rule.py", "find": "LIMIT = 200", "replace": "LIMIT = 201", "obligation_id": "OB-1"},
            {"file": "rule.py", "find": "CAP = 12", "replace": "CAP = 13"},
            {"file": "rule.py", "find": "nope", "replace": "x"},
            "not an object",
            {"file": "rule.py", "find": "CAP"},
        ]}
        result = counterfactual_suite(self.repo, suite)
        self.assertEqual(
            (["killed", "survived", "error", "error", "error"],
             {"killed": 1, "survived": 1, "inconclusive": 0, "error": 3}, "OB-1", 3, 4),
            ([entry["verdict"] for entry in result["results"]], result["summary"],
             result["results"][0]["counterfactual_check"]["obligation_id"],
             result["results"][2]["entry"], result["results"][3]["entry"]),
        )
        self.assertIn("occurs 0 times", result["results"][2]["reason"])
        self.assertEqual(
            ({"obligation_id": "OB-1", "entry": 1, "file": "rule.py", "test_command": RUN_TESTS},
             {"entry": 3, "file": "rule.py", "test_command": RUN_TESTS}),
            ({key: result["results"][0][key] for key in ("obligation_id", "entry", "file", "test_command")},
             {key: result["results"][2][key] for key in ("entry", "file", "test_command")}),
        )
        self.assertIn("mutations[4] must be an object", result["results"][3]["reason"])
        self.assertIn("mutations[5] needs replace", result["results"][4]["reason"])
        self.assert_untouched()

    def test_a_suite_runs_the_intact_tests_once_per_command(self) -> None:
        runs = [TestRun(0, "Ran 2 tests in 0.1s\n\nOK"), TestRun(1, "FAIL: a"), TestRun(1, "FAIL: b")]
        suite = {"test_command": "unused", "mutations": [
            {"file": "rule.py", "find": "LIMIT = 200", "replace": "LIMIT = 201"},
            {"file": "rule.py", "find": "CAP = 12", "replace": "CAP = 13"},
        ]}
        with patch.object(counterfactual_module, "run_tests", side_effect=runs) as run_tests:
            result = counterfactual_suite(self.repo, suite)
        self.assertEqual((["killed", "killed"], 3), ([entry["verdict"] for entry in result["results"]], run_tests.call_count))
        self.assert_untouched()

    def test_a_suite_without_mutations_is_refused(self) -> None:
        for suite in ({}, {"mutations": []}, [], {"mutations": "x"}):
            with self.subTest(suite), self.assertRaisesRegex(ValueError, "mutations is a non-empty list"):
                counterfactual_suite(self.repo, suite)

    def test_a_passing_run_in_which_no_test_ran_proves_nothing(self) -> None:
        intact = TestRun(0, "Ran 0 tests in 0.000s\n\nOK")
        with (
            patch.object(counterfactual_module, "run_tests", side_effect=[intact]),
            self.assertRaisesRegex(ValueError, "^the tests pass before anything is broken, but unittest ran no test"),
        ):
            self.break_rule("LIMIT = 200", "LIMIT = 201")
        with patch.object(counterfactual_module, "run_tests", side_effect=[TestRun(0, "Ran 1 test in 0.0s\n\nOK"), intact]):
            result = self.break_rule("LIMIT = 200", "LIMIT = 201")
        self.assertEqual(
            ("inconclusive", "the tests passed on the broken file, but unittest ran no test that was not skipped; "
                             "a run that judges nothing kills nothing"),
            (result["verdict"], result["reason"]),
        )
        self.assert_untouched()

    def test_text_that_is_absent_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "occurs 0 times"):
            self.break_rule("LIMIT = 300", "LIMIT = 301")
        self.assert_untouched()

    def test_text_that_occurs_twice_is_refused(self) -> None:
        with self.assertRaisesRegex(
            ValueError,
            "^the text to break must occur once in rule.py, and it occurs 2 times, at lines 9, 10; "
            "widen --find to text that occurs once$",
        ):
            self.break_rule("amount", "total")
        with self.assertRaisesRegex(ValueError, "occurs 2 times, at lines 1;"):
            self.break_rule("0", "5")
        self.assert_untouched()

    def test_the_lines_of_a_repeated_text_are_listed_up_to_ten(self) -> None:
        for count, listed in ((10, "1, 2, 3, 4, 5, 6, 7, 8, 9, 10;"), (11, "1, 2, 3, 4, 5, 6, 7, 8, 9, 10 and 1 more;")):
            with self.subTest(count):
                self.write("many.py", "x\n" * count)
                with self.assertRaisesRegex(ValueError, f"at lines {listed}"):
                    counterfactual(self.repo, Mutation(Path("many.py"), "x", "y"), Proof(RUN_TESTS))

    def test_a_replacement_that_changes_nothing_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "is the text it replaces"):
            self.break_rule("LIMIT = 200", "LIMIT = 200")

    def test_tests_that_fail_before_anything_is_broken_are_refused_with_what_failed_and_the_whole_log(self) -> None:
        self.write(
            "tests/test_red.py",
            "import unittest\n\n\nclass Red(unittest.TestCase):\n"
            "    def test_red(self):\n        self.fail('already red' + 'x' * 3000)\n",
        )
        with self.assertRaisesRegex(FailingBaselineError, "prove nothing; the whole output is in ") as refused:
            self.break_rule("LIMIT = 200", "LIMIT = 201")
        self.assert_untouched()
        details = refused.exception.details
        log = Path(details["log_path"])
        self.addCleanup(log.unlink)
        self.assertEqual(
            (["tests.test_red.Red.test_red"], True, True, str(log)),
            (details["failing_tests"], "FAIL: test_red" in details["failure_evidence"],
             "already red" in log.read_text(encoding="utf-8"), str(refused.exception).rpartition(" ")[2]),
        )

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

    def test_broken_code_that_does_not_build_is_inconclusive_whatever_expect_matches(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = (", expect="SyntaxError")
        self.assertEqual(
            ("inconclusive", False, True),
            (result["verdict"], "counterfactual_check" in result, "whatever --expect matches" in result["reason"]),
        )

    def test_every_verdict_gives_its_reason(self) -> None:
        for find, replace, options, reason in (
            ("LIMIT = 200", "LIMIT = 201", {}, "the tests failed once the protection was broken"),
            ("LIMIT = 200", "LIMIT = 201", {"expect": "AssertionError"},
             "the tests failed with output matching --expect AssertionError"),
            ("CAP = 12", "CAP = 13", {}, "the tests still pass with this change; either a test is missing, "
             "or the change does not break the protection (an equivalent mutation); if the test command selects "
             "only some tests, run it over the whole suite, where a kill names the protecting test in failing_tests"),
        ):
            with self.subTest(reason):
                self.assertEqual(reason, self.break_rule(find, replace, **options)["reason"])

    def test_a_failure_not_matching_the_expected_pattern_is_inconclusive(self) -> None:
        result = self.break_rule("LIMIT = 200", "LIMIT = 201", expect="KeyError")
        self.assertEqual(result["verdict"], "inconclusive")
        self.assertNotIn("counterfactual_check", result)
        self.assert_untouched()

    def test_an_expected_pattern_that_is_a_verdict_word_is_refused(self) -> None:
        for expect in ("FAIL", " passed ", "Killed", "error"):
            with self.subTest(expect), self.assertRaisesRegex(ValueError, "^--expect is a regular expression .* not a verdict"):
                self.break_rule("LIMIT = 200", "LIMIT = 201", expect=expect)
        self.assertEqual("killed", self.break_rule("LIMIT = 200", "LIMIT = 201", expect="FAIL|Error")["verdict"])
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
                "failure_evidence": result["failure_evidence"],
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


class FailingTestsTest(unittest.TestCase):
    def test_each_runner_names_the_tests_that_failed_once_each_in_order(self) -> None:
        for runner, output, names in (
            ("go -v", "=== RUN   TestA\n--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/sub (0.00s)\n--- PASS: TestB (0.00s)\n"
                      "--- FAIL: TestA (0.00s)\nFAIL\n", ["TestA"]),
            ("go -json", '{"Action":"fail","Package":"p","Test":"TestA/sub","Elapsed":0}\n'
                         '{"Action":"fail","Package":"p","Test":"TestA","Elapsed":0}\n'
                         '{"Action":"pass","Package":"p","Test":"TestB","Elapsed":0}\n'
                         '{"Action":"fail","Package":"p","Elapsed":0}\n', ["TestA"]),
            ("pytest summary", "FAILED tests/test_a.py::test_one - assert 1 == 2\nERROR tests/test_a.py::test_two\n",
             ["tests/test_a.py::test_one", "tests/test_a.py::test_two"]),
            ("pytest -v", "tests/test_a.py::test_one FAILED  [ 50%]\ntests/test_a.py::test_two PASSED  [100%]\n",
             ["tests/test_a.py::test_one"]),
            ("unittest 3.11", "FAIL: test_one (tests.test_a.ATest.test_one)\nERROR: test_two (tests.test_a.ATest.test_two)\n",
             ["tests.test_a.ATest.test_one", "tests.test_a.ATest.test_two"]),
            ("unittest 3.10", "FAIL: test_one (tests.test_a.ATest)\n", ["tests.test_a.ATest.test_one"]),
            ("unrecognised", "error: something broke\nFAIL\n", []),
        ):
            with self.subTest(runner):
                self.assertEqual(names, failing_tests(output))


class FailureEvidenceTest(unittest.TestCase):
    def test_failure_lines_before_the_tail_come_first(self) -> None:
        frames = "".join(f"frame {number}\n" for number in range(400))
        output = "=== RUN   TestPurge\n--- FAIL: TestPurge (0.00s)\npanic: nil pointer\n" + frames
        self.assertEqual("--- FAIL: TestPurge (0.00s)\npanic: nil pointer\n...\n" + output[-OUTPUT_TAIL:], failure_evidence(output))

    def test_the_lines_indented_under_a_failure_are_kept_with_it(self) -> None:
        failures = (
            "--- FAIL: TestPurge (0.00s)\n"
            "    --- FAIL: TestPurge/eval (0.00s)\n"
            "        purge_steps_test.go:54: purge without eval = <nil>, want a refusal naming it\n"
        )
        output = failures + "FAIL\n" + "x" * OUTPUT_TAIL
        self.assertEqual(failures + "...\n" + output[-OUTPUT_TAIL:], failure_evidence(output))

    def test_at_most_three_indented_lines_follow_each_failure_until_a_line_not_indented_under_it(self) -> None:
        details = "".join(f"    detail {number}\n" for number in range(5))
        output = ("--- FAIL: TestPurge (0.00s)\n" + details + "--- FAIL: TestRun (0.00s)\n    run detail\nFAIL\n    unrelated\n"
                  + "x" * OUTPUT_TAIL)
        self.assertEqual(
            ["--- FAIL: TestPurge (0.00s)", "    detail 0", "    detail 1", "    detail 2", "--- FAIL: TestRun (0.00s)",
             "    run detail", "..."],
            failure_evidence(output).splitlines()[:7],
        )

    def test_output_that_fits_is_kept_whole(self) -> None:
        output = "--- FAIL: TestPurge (0.00s)\n" + "x" * (OUTPUT_TAIL - 30)
        self.assertEqual(output, failure_evidence(output))


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

    def test_without_a_suite_the_mutation_options_are_required(self) -> None:
        self.write("rule.py", RULE)
        completed = self.run_tool("counterfactual", "--repo-root", str(self.repo), "--file", "rule.py")
        self.assertEqual(
            (3, "error", "counterfactual needs --find, --replace, --test-command, or --suite <json file> for several mutations"),
            (completed.returncode, json.loads(completed.stdout)["verdict"], json.loads(completed.stdout)["reason"]),
        )

    def test_a_suite_exits_with_its_worst_verdict(self) -> None:
        self.write("rule.py", RULE)
        self.write("tests/__init__.py")
        self.write("tests/test_rule.py", RULE_TEST)
        suite = self.write("suite.json", json.dumps({"test_command": RUN_TESTS, "mutations": [
            {"file": "rule.py", "find": "LIMIT = 200", "replace": "LIMIT = 201"},
            {"file": "rule.py", "find": "CAP = 12", "replace": "CAP = 13"},
        ]}))
        completed = self.run_tool("counterfactual", "--repo-root", str(self.repo), "--suite", str(suite))
        self.assertEqual(
            (1, {"killed": 1, "survived": 1, "inconclusive": 0, "error": 0}),
            (completed.returncode, json.loads(completed.stdout)["summary"]),
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

    def test_an_error_on_a_console_in_another_code_page_is_printed_in_utf8(self) -> None:
        completed = subprocess.run(
            [sys.executable, str(TOOLS), "counterfactual", "--repo-root", str(self.repo),
             "--file", "\U0001F600.py", "--find", "a", "--replace", "b", "--test-command", "unused"],
            capture_output=True,
            env={**os.environ, "PYTHONIOENCODING": "cp950", "PYTHONUTF8": "0"},
            check=False,
        )
        self.assertEqual(
            (3, "file does not exist: \U0001F600.py"),
            (completed.returncode, json.loads(completed.stdout.decode("utf-8"))["reason"]),
        )

    def test_a_counterfactual_usage_mistake_is_a_json_error_and_other_commands_keep_exit_two(self) -> None:
        for arguments, reason in (
            (("--repo-root", str(self.repo), "--file", "rule.py", "--find", "a", "--replace", "b", "--test-command", "x",
              "--bogus"), "unrecognized arguments: --bogus"),
            (("--repo-root", str(self.repo)), "counterfactual needs --file, --find, --replace, --test-command"),
        ):
            with self.subTest(reason):
                completed = self.run_tool("counterfactual", *arguments)
                result = json.loads(completed.stdout)
                self.assertEqual((3, "error", True), (completed.returncode, result["verdict"], reason in result["reason"]))
        other = self.run_tool("quality-gates", "--repo-root", str(self.repo), "--bogus")
        self.assertEqual(
            (2, "", True, True),
            (other.returncode, other.stdout, other.stderr.startswith("usage: "),
             other.stderr.rstrip().endswith("error: unrecognized arguments: --bogus")),
        )

    def test_a_failing_baseline_is_a_json_error_with_its_own_exit_code(self) -> None:
        self.write(
            "tests/test_red.py",
            "import unittest\n\n\nclass Red(unittest.TestCase):\n    def test_red(self):\n        self.fail('red')\n",
        )
        completed = self.counterfactual_run("CAP = 12", "CAP = 13")
        result = json.loads(completed.stdout)
        self.addCleanup(Path(result["log_path"]).unlink)
        self.assertEqual(
            (3, "domain-memory-counterfactual/v1", "error", ["tests.test_red.Red.test_red"], True),
            (completed.returncode, result["format"], result["verdict"], result["failing_tests"],
             "FAIL: test_red" in result["failure_evidence"]),
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
