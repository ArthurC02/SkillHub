from __future__ import annotations

import fnmatch
import re
import tempfile
import unittest
from pathlib import Path

from build_standalone import build

LINK = re.compile(r"\]\(([^)]+)\)")


def local_targets(page: str) -> list[str]:
    links = LINK.findall(page)
    kept = [link for link in links if not link.startswith(("http://", "https://"))]
    return [target for target in (link.split("#", 1)[0] for link in kept) if target]


class BuildStandaloneTest(unittest.TestCase):
    def test_link_targets_drop_anchors_external_links_and_same_page_anchors(self) -> None:
        page = "[a](x.md#section) [b](https://example.com/y) [c](http://example.com/z) [d](#top) [e](w.md)"
        self.assertEqual(["x.md", "w.md"], local_targets(page))

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
            self.assertTrue((plugin / "scripts/test_support.py").is_file())
            self.assertEqual([], [path.name for path in (output / "scripts").glob("test_*.py")])

    def test_the_scripts_tree_carries_no_builder_cache_or_test_at_any_depth(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "domain-memory-read"
            build(plugin, "domain-memory-read", output)
            scripts = output / "scripts"
            self.assertTrue((scripts / "domain_registry" / "common.py").is_file())
            excluded = [
                path.relative_to(scripts).as_posix()
                for path in scripts.rglob("*")
                if path.name in ("build_standalone.py", "__pycache__") or fnmatch.fnmatch(path.name, "test_*.py")
            ]
            self.assertEqual([], excluded)

    def test_a_non_empty_output_directory_is_refused_and_left_untouched(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "domain-memory-read"
            output.mkdir()
            (output / "kept.txt").write_text("kept", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "^output directory is not empty: " + re.escape(str(output)) + "$"):
                build(plugin, "domain-memory-read", output)
            self.assertEqual(["kept.txt"], [entry.name for entry in output.iterdir()])
            self.assertEqual("kept", (output / "kept.txt").read_text(encoding="utf-8"))

    def test_an_existing_empty_output_directory_is_filled(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "domain-memory-read"
            output.mkdir()
            build(plugin, "domain-memory-read", output)
            self.assertTrue((output / "SKILL.md").is_file())

    def test_bundle_carries_the_entry_point_for_a_host_without_skills(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "domain-memory-read"
            build(plugin, "domain-memory-read", output)
            entry = (output / "AGENTS.md").read_text(encoding="utf-8")
            self.assertIn("python3 scripts/registry_tools.py", entry)
            self.assertIn("references/script-api.md", LINK.findall(entry))
            targets = local_targets(entry)
            self.assertTrue(targets)
            for target in targets:
                self.assertTrue((output / target).is_file(), target)

    def test_every_capability_bundle_carries_what_its_page_links_to(self) -> None:
        plugin = Path(__file__).parents[1]
        capabilities = sorted(p.name for p in (plugin / "skills").iterdir())
        self.assertIn("domain-memory-implementation-hygiene", capabilities)
        for capability in capabilities:
            with self.subTest(capability), tempfile.TemporaryDirectory() as temporary:
                output = Path(temporary) / capability
                build(plugin, capability, output)
                page = (output / "SKILL.md").read_text(encoding="utf-8")
                targets = local_targets(page)
                self.assertTrue(targets)
                for target in targets:
                    self.assertTrue((output / target).is_file(), target)

    def test_the_code_rules_read_the_same_on_the_entry_and_the_hygiene_page(self) -> None:
        plugin = Path(__file__).parents[1]

        def code_rules(page: Path) -> list[str]:
            text = page.read_text(encoding="utf-8")
            section = text.split("## When you are the one writing the code", 1)[1]
            section = section.split("\n## ", 1)[0]
            return re.findall(r"^\d+\. .+$", section, re.MULTILINE)

        entry = code_rules(plugin / "AGENTS.md")
        hygiene = code_rules(
            plugin / "skills/domain-memory-implementation-hygiene/SKILL.md"
        )
        self.assertEqual(len(entry), 6)
        self.assertEqual(entry, hygiene)

    def test_unknown_skill_does_not_create_output(self) -> None:
        plugin = Path(__file__).parents[1]
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "missing"
            with self.assertRaises(ValueError):
                build(plugin, "missing", output)
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
