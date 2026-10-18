import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.git import run_bounded

DEADLINE_SECONDS = 2
RETURNS_WITHIN_SECONDS = 10
GRANDCHILD_SLEEP_SECONDS = 12
GONE_WITHIN_SECONDS = 5
HANGING_COMMAND = """
import subprocess, sys, time
grandchild = subprocess.Popen([sys.executable, "-c", "import time; time.sleep({sleep})"])
with open(sys.argv[1], "w") as pid_file:
    pid_file.write(str(grandchild.pid))
time.sleep({sleep})
"""


def alive(pid: int) -> bool:
    if os.name == "nt":
        listed = subprocess.run(
            ["tasklist", "/FI", f"PID eq {pid}", "/NH"], capture_output=True, text=True, check=False
        ).stdout
        return str(pid) in listed.split()
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    return True


def gone_within(pid: int, seconds: float) -> bool:
    deadline = time.monotonic() + seconds
    while alive(pid):
        if time.monotonic() > deadline:
            return False
    return True


class BoundedCommandTest(unittest.TestCase):
    def test_a_command_whose_grandchild_holds_the_output_open_is_stopped_with_its_whole_tree_at_the_deadline(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            script, pid_file = Path(temporary) / "hang.py", Path(temporary) / "grandchild.pid"
            script.write_text(HANGING_COMMAND.format(sleep=GRANDCHILD_SLEEP_SECONDS), encoding="utf-8")
            started = time.monotonic()
            with patch("domain_registry.git.GIT_COMMAND_TIMEOUT_SECONDS", DEADLINE_SECONDS), \
                    self.assertRaises(subprocess.TimeoutExpired) as stopped:
                run_bounded([sys.executable, str(script), str(pid_file)], errors="replace")
            elapsed = time.monotonic() - started
            grandchild = int(pid_file.read_text(encoding="utf-8"))
            self.assertEqual(
                (f"timed out after {DEADLINE_SECONDS} seconds", True, True),
                (str(stopped.exception).split("' ")[-1], elapsed < RETURNS_WITHIN_SECONDS,
                 gone_within(grandchild, GONE_WITHIN_SECONDS)),
            )

    def test_a_command_that_finishes_in_time_returns_its_exit_code_and_output(self) -> None:
        finished = run_bounded([sys.executable, "-c", "import sys; print('out'); sys.exit(3)"], errors="replace")
        self.assertEqual((3, "out"), (finished.returncode, finished.stdout.strip()))


if __name__ == "__main__":
    unittest.main()
