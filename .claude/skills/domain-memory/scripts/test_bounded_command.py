import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.git import TIMEOUT_VARIABLE, GitDeadline, deadline_seconds, rerun_is_safe, run_bounded

DEADLINE_SECONDS = 6
RETURNS_WITHIN_SECONDS = 15
GRANDCHILD_SLEEP_SECONDS = 30
GONE_WITHIN_SECONDS = 5
SPAWN_AND_EXIT = (
    "import subprocess, sys; grandchild = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep({sleep})']); "
    "open(sys.argv[1], 'w').write(str(grandchild.pid))"
)
HANGING_COMMAND = """
import subprocess, sys, time
subprocess.run([sys.executable, "-c", "{spawn}", sys.argv[1]], check=True)
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
    def test_a_deadline_says_a_rerun_is_safe_only_for_a_git_command_that_writes_nothing(self) -> None:
        advice = f"; set {TIMEOUT_VARIABLE} to a larger number of seconds to wait longer"
        self.assertEqual(
            [f"Command '['git', 'ls-tree', 'HEAD']' timed out after 1 seconds; nothing was written, so it is safe to run again{advice}",
             f"Command '['git', 'config', 'core.hooksPath', 'x']' timed out after 1 seconds{advice}",
             f"Command '['ssh-keygen', '-l']' timed out after 1 seconds{advice}"],
            [str(GitDeadline(argv, 1, rerun_is_safe=rerun_is_safe(argv)))
             for argv in (["git", "ls-tree", "HEAD"], ["git", "config", "core.hooksPath", "x"], ["ssh-keygen", "-l"])],
        )

    def test_a_grandchild_whose_parent_exited_and_that_holds_the_output_open_is_stopped_at_the_deadline(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            script, pid_file = Path(temporary) / "hang.py", Path(temporary) / "grandchild.pid"
            script.write_text(
                HANGING_COMMAND.format(spawn=SPAWN_AND_EXIT.format(sleep=GRANDCHILD_SLEEP_SECONDS), sleep=GRANDCHILD_SLEEP_SECONDS),
                encoding="utf-8",
            )
            started = time.monotonic()
            with patch.dict(os.environ, {TIMEOUT_VARIABLE: str(DEADLINE_SECONDS)}), \
                    self.assertRaises(subprocess.TimeoutExpired) as stopped:
                run_bounded([sys.executable, str(script), str(pid_file)], errors="replace")
            elapsed = time.monotonic() - started
            self.assertTrue(pid_file.is_file(), f"the grandchild was not spawned within {DEADLINE_SECONDS} seconds; the deadline is shorter than two interpreter starts on this machine")
            grandchild = int(pid_file.read_text(encoding="utf-8"))
            self.assertEqual(
                (f"timed out after {DEADLINE_SECONDS} seconds; set {TIMEOUT_VARIABLE} to a larger number of "
                 "seconds to wait longer", True, True),
                (str(stopped.exception).split("' ")[-1], elapsed < RETURNS_WITHIN_SECONDS,
                 gone_within(grandchild, GONE_WITHIN_SECONDS)),
            )

    def test_a_git_read_that_times_out_says_a_rerun_is_safe(self) -> None:
        with patch.dict(os.environ, {TIMEOUT_VARIABLE: "1"}), \
                patch("domain_registry.git.subprocess.Popen") as popen,                 patch("domain_registry.git._kill_process_tree"), patch("domain_registry.git._job_holding", return_value=None):
            popen.return_value.communicate.side_effect = [subprocess.TimeoutExpired(["git"], 1), (b"", b"")]
            with self.assertRaises(subprocess.TimeoutExpired) as stopped:
                run_bounded(["git", "-C", "repo", "ls-tree", "HEAD"])
        self.assertEqual(
            "Command '['git', '-C', 'repo', 'ls-tree', 'HEAD']' timed out after 1 seconds; nothing was written, so "
            f"it is safe to run again; set {TIMEOUT_VARIABLE} to a larger number of seconds to wait longer",
            str(stopped.exception),
        )

    def test_the_deadline_is_fifteen_seconds_unless_the_variable_gives_a_positive_number(self) -> None:
        for given, expected in (
            (None, 15), ("2", 2), ("2.5", 2.5), ("0", "must be a positive number of seconds, not '0'"),
            ("-1", "must be a positive number of seconds, not '-1'"),
            ("soon", "must be a positive number of seconds, not 'soon'"),
        ):
            with self.subTest(given=given), patch.dict(os.environ, {}, clear=False):
                os.environ.pop(TIMEOUT_VARIABLE, None)
                if given is not None:
                    os.environ[TIMEOUT_VARIABLE] = given
                if isinstance(expected, str):
                    with self.assertRaises(ValueError) as refused:
                        deadline_seconds()
                    self.assertEqual(f"{TIMEOUT_VARIABLE} {expected}", str(refused.exception))
                else:
                    self.assertEqual(expected, deadline_seconds())

    def test_a_command_that_finishes_in_time_returns_its_exit_code_and_output(self) -> None:
        finished = run_bounded([sys.executable, "-c", "import sys; print('out'); sys.exit(3)"], errors="replace")
        self.assertEqual((3, "out"), (finished.returncode, finished.stdout.strip()))


if __name__ == "__main__":
    unittest.main()
