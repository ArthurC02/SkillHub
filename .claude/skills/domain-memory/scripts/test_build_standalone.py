from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from build_standalone import build


class BuildStandaloneTest(unittest.TestCase):
    def test_bundle_contains_one_skill_and_all_relative_resources(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "domain-memory-read"
            build(plugin, "domain-memory-read", output)
            skill = (output / "SKILL.md").read_text(encoding="utf-8")
            self.assertIn("(references/script-api.md)", skill)
            self.assertNotIn("../../references/", skill)
            self.assertTrue((output / "references/script-api.md").is_file())
            self.assertTrue(
                (output / "templates/domain-change-proposal.json").is_file()
            )
            self.assertTrue((output / "scripts/registry_tools.py").is_file())
            self.assertFalse((output / "scripts/test_domain_registry.py").exists())

    def test_unknown_skill_does_not_create_output(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "missing"
            with self.assertRaises(ValueError):
                build(plugin, "missing", output)
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
