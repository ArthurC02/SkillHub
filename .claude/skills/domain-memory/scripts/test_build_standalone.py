from __future__ import annotations

import re
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

    def test_bundle_carries_the_entry_point_for_a_host_without_skills(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "domain-memory-read"
            build(plugin, "domain-memory-read", output)
            entry = (output / "AGENTS.md").read_text(encoding="utf-8")
            self.assertIn("python3 scripts/registry_tools.py", entry)
            links = re.findall(r"\]\(([^)]+)\)", entry)
            self.assertIn("references/script-api.md", links)
            for link in links:
                self.assertTrue((output / link).is_file(), link)

    def test_unknown_skill_does_not_create_output(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "missing"
            with self.assertRaises(ValueError):
                build(plugin, "missing", output)
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
