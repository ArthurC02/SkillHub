from __future__ import annotations

import subprocess
import sys
import unittest
from pathlib import Path

BUILD = Path(__file__).resolve().parents[1] / "engine" / "build_binaries.py"


class EngineBinariesTest(unittest.TestCase):
    def test_the_committed_binaries_are_the_ones_built_from_the_committed_source(self) -> None:
        run = subprocess.run([sys.executable, str(BUILD), "--check"], capture_output=True, text=True, check=False)
        self.assertEqual((0, ""), (run.returncode, run.stderr))


if __name__ == "__main__":
    unittest.main()
