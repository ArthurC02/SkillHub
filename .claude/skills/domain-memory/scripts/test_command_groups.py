from __future__ import annotations

import argparse
import re
import shlex
import unittest
from pathlib import Path

from domain_registry.cli_parser import (
    COMMAND_GROUPS,
    UsageError,
    _add_evidence_commands,
    _add_governance_commands,
    _add_lifecycle_commands,
    _add_read_commands,
    build_parser,
)
from domain_registry.cli_commands import COMMANDS_BY_MODULE
from domain_registry.commands import evidence, governance, lifecycle, read
from test_support import RegistryTestCase

THEMES = {
    "read": (read, _add_read_commands),
    "evidence": (evidence, _add_evidence_commands),
    "lifecycle": (lifecycle, _add_lifecycle_commands),
    "governance": (governance, _add_governance_commands),
}

PLUGIN = Path(__file__).parents[1]
REFERENCE = PLUGIN / "references" / "script-api.md"
LINK = re.compile(r"\]\(([^)]+)\)")
SNIPPET = re.compile(r"`([a-z][a-z-]* [^`]*--[^`]*)`")
PLACEHOLDER = re.compile(r"<[^>]*>")
ROOTS = ("--registry-root", "x", "--repo-root", "x")
TABLE_HEADINGS = {"## Everyday commands": "everyday", "## Advanced commands": "advanced"}
ROW = re.compile(r"^### `([a-z][a-z-]*)`")


def parsed_commands() -> list[str]:
    [subcommands] = [action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)]
    return list(subcommands.choices)


def registered_by(add_commands) -> list[str]:
    subcommands = argparse.ArgumentParser().add_subparsers()
    add_commands(subcommands)
    return sorted(subcommands.choices)


def pages() -> list[Path]:
    return [PLUGIN / "SKILL.md", PLUGIN / "AGENTS.md", *sorted(PLUGIN.glob("skills/*/SKILL.md")), *sorted(PLUGIN.glob("references/*.md"))]


def local_targets(page: Path) -> list[str]:
    links = LINK.findall(page.read_text(encoding="utf-8"))
    kept = [link for link in links if not link.startswith(("http://", "https://", "/"))]
    return [target for target in (link.split("#", 1)[0] for link in kept) if target]


def code_rules(page: Path) -> list[str]:
    section = page.read_text(encoding="utf-8").split("## When you are the one writing the code", 1)[1].split("\n## ", 1)[0]
    return re.findall(r"^\d+\. .+$", section, re.MULTILINE)


def common_task_invocations() -> list[str]:
    section = (PLUGIN / "AGENTS.md").read_text(encoding="utf-8").split("## Common tasks", 1)[1].split("\n## ", 1)[0]
    names = set(parsed_commands())
    return [snippet for snippet in SNIPPET.findall(section) if snippet.split()[0] in names]


def invocation_argv(snippet: str) -> list[str]:
    [subcommands] = [action for action in build_parser()._actions if isinstance(action, argparse._SubParsersAction)]
    tokens = shlex.split(PLACEHOLDER.sub("x", snippet).replace("[", "").replace("]", ""))
    choices = {flag: action.choices for action in subcommands.choices[tokens[0]]._actions for flag in action.option_strings if action.choices}
    for index, token in enumerate(tokens[:-1]):
        if token in choices and tokens[index + 1] == "x":
            tokens[index + 1] = sorted(choices[token])[0]
    return tokens


def parse_refusal(argv: list[str]) -> str | None:
    try:
        build_parser().parse_args(argv)
    except UsageError as refusal:
        return str(refusal).splitlines()[-1]
    return None


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

    def test_the_dispatch_table_names_each_command_under_the_module_that_holds_its_handler(self) -> None:
        self.assertEqual(
            {theme: sorted(module.HANDLERS) for theme, (module, _) in THEMES.items()},
            {theme: sorted(commands) for theme, commands in COMMANDS_BY_MODULE.items()},
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

    def test_each_command_is_documented_only_under_the_heading_of_its_group(self) -> None:
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
                "Everyday commands: " in flat,
                "Advanced commands: " in flat,
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


class PagesTest(unittest.TestCase):
    def test_every_page_links_only_to_files_the_plugin_holds(self) -> None:
        broken = [(page.relative_to(PLUGIN).as_posix(), target) for page in pages() for target in local_targets(page)
                  if not (page.parent / target).is_file()]
        self.assertEqual([], broken)

    def test_every_reference_is_linked_from_a_page_that_chooses_work(self) -> None:
        linked = {(page.parent / target).resolve() for page in pages() for target in local_targets(page)}
        self.assertEqual([], [path.name for path in sorted(PLUGIN.glob("references/*.md")) if path.resolve() not in linked])

    def test_every_invocation_in_the_common_tasks_table_parses_with_the_roots_the_convention_leaves_out(self) -> None:
        invocations = common_task_invocations()
        self.assertGreater(len(invocations), 10)
        refused = {}
        for snippet in invocations:
            argv = invocation_argv(snippet)
            variants = [argv + list(ROOTS), argv + list(ROOTS[:2]), argv + list(ROOTS[2:]), argv]
            refusals = [parse_refusal(variant) for variant in variants]
            if all(refusals):
                refused[snippet] = refusals[0]
        self.assertEqual({}, refused)

    def test_the_code_rules_live_on_the_entry_page_alone_and_the_hygiene_page_points_there(self) -> None:
        hygiene = (PLUGIN / "skills/domain-memory-implementation-hygiene/SKILL.md").read_text(encoding="utf-8")
        self.assertEqual(
            (7, True, False),
            (len(code_rules(PLUGIN / "AGENTS.md")), "AGENTS.md#when-you-are-the-one-writing-the-code" in hygiene,
             "## When you are the one writing the code" in hygiene),
        )


if __name__ == "__main__":
    unittest.main()
