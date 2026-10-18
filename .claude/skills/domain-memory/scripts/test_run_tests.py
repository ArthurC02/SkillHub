from __future__ import annotations

import contextlib
import io
import sys
import tempfile
import unittest
from pathlib import Path

import run_tests

PASSING = "import unittest\n\n\nclass PassingTest(unittest.TestCase):\n    def test_passes(self):\n        self.assertTrue(True)\n"
FAILING = "import unittest\n\n\nclass FailingTest(unittest.TestCase):\n    def test_fails(self):\n        self.assertEqual(1, 2)\n"


class RunTestsTest(unittest.TestCase):
    def run_in(self, modules: dict[str, str], *arguments: str) -> tuple[int, str]:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name, source in modules.items():
                (root / f"{name}.py").write_text(source, encoding="utf-8")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = run_tests.main(["--root", str(root), "--python", sys.executable, *arguments])
        return code, output.getvalue()

    def test_every_module_beside_the_script_runs_and_a_failing_one_fails_the_run_with_its_output(self) -> None:
        code, output = self.run_in({"test_one": PASSING, "test_two": FAILING, "helper": FAILING}, "--jobs", "2")
        lines = output.splitlines()
        self.assertEqual((1, "ok   test_one", "FAIL test_two", True, True), (
            code, lines[0][:13], lines[1][:13], "AssertionError: 1 != 2" in output,
            lines[-1].startswith("2 modules in ") and lines[-1].endswith("with 2 jobs; failed: test_two"),
        ))

    def test_a_module_holding_no_tests_is_skipped_rather_than_failed(self) -> None:
        code, output = self.run_in({"test_one": PASSING, "test_support": "import unittest\n"}, "--jobs", "2")
        lines = output.splitlines()
        self.assertEqual((0, "skip test_support", True), (code, lines[1][:17], lines[-1].endswith("failed: none")))

    def test_named_modules_run_alone_and_a_green_run_exits_zero(self) -> None:
        code, output = self.run_in({"test_one": PASSING, "test_two": FAILING}, "test_one")
        self.assertEqual((0, True, True), (code, "test_two" not in output.split("failed:")[0], output.rstrip().endswith("failed: none")))


if __name__ == "__main__":
    unittest.main()
