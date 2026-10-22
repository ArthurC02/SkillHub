from __future__ import annotations

import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry import sources
from domain_registry.changes import validate_change_package
from domain_registry.git import GitDeadline, run_git
from domain_registry.locate import domain_memories_in
from domain_registry.recitation_package import recitation_package
from domain_registry.relocate import Recitation, recite
from domain_registry.revision import git_commit
from domain_registry.security import scan_report
from domain_registry.sources import git_state, ignored_relatives
from test_support import RegistryTestCase

DEADLINE = GitDeadline(["git", "ls-files"], 1, rerun_is_safe=True)
VANISHED = OSError("git vanished")


class GitDeadlineSurfacesTest(RegistryTestCase):
    def test_the_domain_memories_git_lists_stop_at_a_deadline_and_name_the_reason_without_git(self) -> None:
        with self.subTest("deadline"), patch("domain_registry.locate.run_git", side_effect=DEADLINE), \
                self.assertRaises(GitDeadline):
            domain_memories_in(self.repo)
        with self.subTest("no git"), patch("domain_registry.locate.run_git", side_effect=VANISHED):
            self.assertEqual("git vanished", domain_memories_in(self.repo))

    def test_the_secret_scan_stops_at_a_deadline_and_walks_the_same_files_without_git(self) -> None:
        listed_by_git = scan_report(self.repo)
        with self.subTest("deadline"), patch("domain_registry.security.run_git", side_effect=DEADLINE), \
                self.assertRaises(GitDeadline):
            scan_report(self.repo)
        with self.subTest("no git"), patch("domain_registry.security.run_git", side_effect=VANISHED):
            walked = scan_report(self.repo)
        self.assertEqual(
            ("complete", listed_by_git["scanned_files"], listed_by_git["scanned_bytes"]),
            (walked["status"], walked["scanned_files"], walked["scanned_bytes"]),
        )

    def test_the_observed_commit_stops_at_a_deadline_and_is_none_without_git(self) -> None:
        with self.subTest("deadline"), patch("domain_registry.revision.run_git", side_effect=DEADLINE), \
                self.assertRaises(GitDeadline):
            git_commit(self.repo)
        with self.subTest("no git"), patch("domain_registry.revision.run_git", side_effect=VANISHED):
            self.assertIsNone(git_commit(self.repo))

    def test_the_git_state_of_the_sources_stops_at_a_deadline_and_is_empty_without_git(self) -> None:
        with self.subTest("deadline"), patch("domain_registry.sources.run_git", side_effect=DEADLINE), \
                self.assertRaises(GitDeadline):
            git_state(self.repo, ["memory"])
        with self.subTest("no git"), patch("domain_registry.sources.run_git", side_effect=VANISHED):
            self.assertEqual({}, git_state(self.repo, ["memory"]))

    def test_the_untracked_prefilter_stops_at_a_deadline_instead_of_asking_check_ignore(self) -> None:
        def index_listing_times_out(root: object, *arguments: str, **options: object) -> object:
            if "ls-files" in arguments:
                raise DEADLINE
            return run_git(root, *arguments, **options)

        with patch.object(sources, "TRACKED_PREFILTER_MIN_PATHS", 1), \
                patch("domain_registry.sources.run_git", side_effect=index_listing_times_out), \
                self.assertRaises(GitDeadline):
            ignored_relatives(self.repo, ["memory/registry/index.md"])

    def test_a_recitation_package_stops_at_a_deadline_writing_nothing_and_is_still_written_without_git(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        recitation = recite(self.repo / "memory", self.repo, Recitation("rules", "rule", "src/a.py", "3"))
        identifiers = ("rq-recite", "pr-recite", "probe")
        with self.subTest("deadline"), patch("domain_registry.recitation_package.toplevel", side_effect=DEADLINE), \
                self.assertRaises(GitDeadline):
            recitation_package(self.repo / "changes" / "timed-out", self.repo / "memory", recitation, identifiers)
        self.assertFalse((self.repo / "changes" / "timed-out").exists())
        with self.subTest("no git"), patch("domain_registry.recitation_package.toplevel", side_effect=VANISHED):
            output = self.repo / "changes" / "recite"
            self.assertEqual(
                ([], []),
                (recitation_package(output, self.repo / "memory", recitation, identifiers),
                 validate_change_package(output, self.repo / "memory", "submitted", self.repo)),
            )


if __name__ == "__main__":
    unittest.main()
