from __future__ import annotations

import argparse
import re
import unittest
from pathlib import Path

from domain_registry.cli_parser import (
    COMMAND_GROUPS,
    _add_evidence_commands,
    _add_governance_commands,
    _add_lifecycle_commands,
    _add_read_commands,
    build_parser,
)
from domain_registry.commands import evidence, governance, lifecycle, read
from test_support import RegistryTestCase

THEMES = {
    "read": (read, _add_read_commands),
    "evidence": (evidence, _add_evidence_commands),
    "lifecycle": (lifecycle, _add_lifecycle_commands),
    "governance": (governance, _add_governance_commands),
}

REFERENCE = Path(__file__).parents[1] / "references" / "script-api.md"
TABLE_HEADINGS = {"## Everyday commands": "everyday", "## Advanced commands": "advanced"}
ROW = re.compile(r"^\| [^|`]+? \| `([a-z][a-z-]*)")


def parsed_commands() -> list[str]:
    [subcommands] = [action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)]
    return list(subcommands.choices)


def registered_by(add_commands) -> list[str]:
    subcommands = argparse.ArgumentParser().add_subparsers()
    add_commands(subcommands)
    return sorted(subcommands.choices)


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

    def test_each_parser_theme_registers_exactly_the_commands_its_handler_module_holds(self) -> None:
        self.assertEqual(
            {theme: sorted(module.HANDLERS) for theme, (module, _) in THEMES.items()},
            {theme: registered_by(add_commands) for theme, (_, add_commands) in THEMES.items()},
        )

    def test_every_parsed_command_has_a_handler_in_exactly_one_theme_module(self) -> None:
        held = [command for module, _ in THEMES.values() for command in module.HANDLERS]
        self.assertEqual(
            ([], sorted(parsed_commands())),
            (sorted({command for command in held if held.count(command) > 1}), sorted(held)),
        )

    def test_each_hook_installing_command_points_the_hook_at_the_entry_script(self) -> None:
        (self.repo / ".git" / "hooks").mkdir(parents=True, exist_ok=True)
        entry_script = (Path(__file__).resolve().parent / "registry_tools.py").as_posix()
        for command, hook_name in (("install-git-hitl-hook", "pre-push"), ("install-pre-commit-hook", "pre-commit")):
            result = self.run_cli(command, "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
            hook = self.repo / ".git" / "hooks" / hook_name
            self.assertEqual(
                (0, f"tool='{entry_script}'"),
                (result.returncode, hook.read_text(encoding="utf-8").splitlines()[2]),
            )

    def test_each_command_is_documented_only_in_the_table_of_its_group(self) -> None:
        self.assertEqual(
            {name: {group} for group, names in COMMAND_GROUPS.items() for name in names}, documented_groups()
        )

    def test_help_lists_the_two_groups_and_says_nothing_of_legacy_commands(self) -> None:
        result = self.run_cli("--help")
        flat = " ".join(result.stdout.split())
        self.assertEqual(
            (0, ["everyday", "advanced"], True, True, False),
            (
                result.returncode,
                list(COMMAND_GROUPS),
                "Everyday commands: readiness, probe, lookup," in flat,
                "Advanced commands: discover-sources, init-domain-memory," in flat,
                "legacy command" in flat.lower(),
            ),
        )

    def test_help_lists_the_commands_in_the_order_of_the_groups(self) -> None:
        result = self.run_cli("--help")
        listed = re.findall(r"^    ([a-z][a-z-]*)(?:  |$)", result.stdout.split("positional arguments:")[1], re.MULTILINE)
        self.assertEqual([name for names in COMMAND_GROUPS.values() for name in names], listed)

    def test_a_removed_legacy_command_is_an_invalid_choice_and_prints_no_note(self) -> None:
        for command in ("fill-harvest-package", "attest-signed-commit", "finalize-proposal", "apply-approved-updates"):
            with self.subTest(command):
                result = self.run_cli(command, "--package-root", str(self.repo))
                self.assertEqual(
                    (2, "", True, False),
                    (
                        result.returncode,
                        result.stdout,
                        f"invalid choice: '{command}'" in result.stderr,
                        "NOTE:" in result.stderr,
                    ),
                )

    def test_a_command_prints_no_note_on_stderr_when_it_has_nothing_to_note(self) -> None:
        (self.repo / "changes").mkdir()
        result = self.run_cli("list-proposals", "--changes-root", str(self.repo / "changes"))
        self.assertEqual((0, "[]", ""), (result.returncode, result.stdout.strip(), result.stderr))


if __name__ == "__main__":
    unittest.main()
