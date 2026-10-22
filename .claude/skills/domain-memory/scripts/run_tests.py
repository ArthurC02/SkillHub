from __future__ import annotations

import argparse
import os
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from domain_registry.git import TIMEOUT_VARIABLE

HERE = Path(__file__).resolve().parent
SUMMARY_TAIL = 3
GIT_TIMEOUT_SECONDS_FOR_A_LOADED_RUNNER = "60"


def test_modules(root: Path, chosen: list[str]) -> list[str]:
    if chosen:
        return chosen
    return sorted(path.stem for path in root.glob("test_*.py"))


FAILED, PASSED, NO_TESTS = "FAIL", "ok  ", "skip"
NO_TESTS_EXIT_CODES = (0, 5)


def status_of(code: int, output: str) -> str:
    if "Ran 0 tests" in output and code in NO_TESTS_EXIT_CODES:
        return NO_TESTS
    return PASSED if code == 0 else FAILED


def run_module(root: Path, python: str, module: str) -> tuple[str, int, str, float]:
    started = time.monotonic()
    completed = subprocess.run(
        [python, "-m", "unittest", "-q", module], cwd=root, capture_output=True, text=True, encoding="utf-8", errors="replace",
        check=False, env={TIMEOUT_VARIABLE: GIT_TIMEOUT_SECONDS_FOR_A_LOADED_RUNNER, **os.environ},
    )
    return module, completed.returncode, completed.stdout + completed.stderr, time.monotonic() - started


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Run each test module in its own interpreter, several at a time.")
    parser.add_argument("modules", nargs="*", help="test modules to run; every test_*.py beside this script when none is named")
    parser.add_argument("--jobs", type=int, default=os.cpu_count() or 1)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--root", type=Path, default=HERE)
    args = parser.parse_args(argv)
    root = args.root.resolve()
    modules = test_modules(root, args.modules)
    if not modules:
        print(f"no test_*.py under {root}; nothing ran")
        return 1
    started = time.monotonic()
    with ThreadPoolExecutor(max_workers=max(1, args.jobs)) as pool:
        results = list(pool.map(lambda module: run_module(root, args.python, module), modules))
    failed = []
    for module, code, output, seconds in results:
        tail = " ".join(line for line in output.strip().splitlines()[-SUMMARY_TAIL:] if line.startswith(("Ran ", "OK", "FAILED")))
        status = status_of(code, output)
        print(f"{status} {module} {seconds:.1f}s {tail}")
        if status == FAILED:
            failed.append(module)
            print(output, end="" if output.endswith("\n") else "\n")
    print(f"{len(modules)} modules in {time.monotonic() - started:.1f}s with {args.jobs} jobs; failed: {', '.join(failed) or 'none'}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
