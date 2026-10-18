from __future__ import annotations

import argparse
import re
import unittest
from pathlib import Path

from domain_registry.cli_parser import COMMAND_GROUPS, LEGACY_REPLACEMENTS, build_parser
from test_support import RegistryTestCase

REFERENCE = Path(__file__).parents[1] / "references" / "script-api.md"
TABLE_HEADINGS = {"## Everyday commands": "everyday", "## Advanced commands": "advanced", "## Legacy commands": "legacy"}
ROW = re.compile(r"^\| [^|`]+? \| `([a-z][a-z-]*)")


def parsed_commands() -> list[str]:
    [subcommands] = [action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)]
    return list(subcommands.choices)


def documented_groups() -> dict[str, set[str]]:
    groups: dict[str, set[str]] = {}
    heading = None
    for line in REFERENCE.read_text(encoding="utf-8").splitlines():
        if line.startswith("## "):
            heading = line
        elif (row := ROW.match(line)) is not None:
            groups.setdefault(row.group(1), set()).add(TABLE_HEADINGS.get(heading, heading or "no heading"))
    return groups


class CommandGroupsTest(RegistryTestCase):
    def test_every_parsed_command_sits_in_exactly_one_group_and_every_member_is_parsed(self) -> None:
        members = [name for names in COMMAND_GROUPS.values() for name in names]
        self.assertEqual(
            ([], sorted(parsed_commands())),
            (sorted({name for name in members if members.count(name) > 1}), sorted(members)),
        )

    def test_each_command_is_documented_only_in_the_table_of_its_group(self) -> None:
        self.assertEqual(
            {name: {group} for group, names in COMMAND_GROUPS.items() for name in names}, documented_groups()
        )

    def test_the_legacy_group_is_the_commands_that_have_a_replacement(self) -> None:
        self.assertEqual(
            ("fill-harvest-package", "attest-signed-commit", "finalize-proposal", "apply-approved-updates"),
            COMMAND_GROUPS["legacy"],
        )
        self.assertEqual(
            {
                "fill-harvest-package": "harvest --spec <file>",
                "attest-signed-commit": "ship-package --commit <sha>",
                "finalize-proposal": "ship-package",
                "apply-approved-updates": "ship-package",
            },
            LEGACY_REPLACEMENTS,
        )

    def test_help_lists_the_three_groups(self) -> None:
        result = self.run_cli("--help")
        flat = " ".join(result.stdout.split())
        self.assertEqual(0, result.returncode)
        self.assertIn("Everyday commands: readiness, probe, lookup,", flat)
        self.assertIn("Advanced commands: discover-sources, init-domain-memory,", flat)
        self.assertIn(
            "Legacy commands: fill-harvest-package, attest-signed-commit, finalize-proposal, apply-approved-updates.", flat
        )

    def test_help_lists_the_commands_in_the_order_of_the_groups(self) -> None:
        result = self.run_cli("--help")
        listed = re.findall(r"^    ([a-z][a-z-]*)(?:  |$)", result.stdout.split("positional arguments:")[1], re.MULTILINE)
        self.assertEqual([name for names in COMMAND_GROUPS.values() for name in names], listed)

    def test_only_a_legacy_command_help_names_its_replacement(self) -> None:
        [subcommands] = [action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)]
        helps = {action.dest: action.help for action in subcommands._choices_actions}
        marked = {name: text for name, text in helps.items() if "(legacy:" in text}
        self.assertEqual(
            {name: f"(legacy: use {replacement})" for name, replacement in LEGACY_REPLACEMENTS.items()},
            {name: text[text.index("(legacy:"):] for name, text in marked.items()},
        )

    def test_a_legacy_command_notes_its_replacement_on_stderr_and_still_runs(self) -> None:
        result = self.run_cli(
            "apply-approved-updates", "--package-root", str(self.draft_package()),
            "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
        )
        self.assertEqual(
            (
                1,
                "ERROR: only a finalized approved proposal may apply registry updates",
                "NOTE: apply-approved-updates is legacy; use ship-package; it is removed in 1.0.0",
            ),
            (result.returncode, result.stdout.strip(), result.stderr.strip()),
        )

    def test_a_command_outside_the_legacy_group_prints_no_note(self) -> None:
        (self.repo / "changes").mkdir()
        result = self.run_cli("list-proposals", "--changes-root", str(self.repo / "changes"))
        self.assertEqual((0, "[]", ""), (result.returncode, result.stdout.strip(), result.stderr))


if __name__ == "__main__":
    unittest.main()
