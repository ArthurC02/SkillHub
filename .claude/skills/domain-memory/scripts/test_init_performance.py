import subprocess
import unittest
from unittest.mock import patch

from test_support import RegistryTestCase

from domain_registry import sources
from domain_registry.git import GitDeadline, run_git
from domain_registry.sources import ignored_relatives, selected_source_map, source_files

ASKED = ["kept.md", "tracked.log", "hidden.log", "fresh.md", "scratch.log", "build/out.txt"]
IGNORED = {"scratch.log", "build/out.txt"}
POLICY = {
    "source_policy": {"include": ["**"], "exclude": []},
    "limits": {"max_file_count": 100, "max_file_bytes": 1000, "max_total_bytes": 10000},
}


class TrackedPrefilterTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        (self.repo / ".gitignore").write_text("*.log\nbuild/\n", encoding="utf-8")
        for name in ("kept.md", "tracked.log", "hidden.log", "fresh.md", "scratch.log", "build/out.txt"):
            (self.repo / name).parent.mkdir(parents=True, exist_ok=True)
            (self.repo / name).write_text(name, encoding="utf-8")
        self.git("add", ".gitignore", "kept.md")
        self.git("add", "-f", "tracked.log", "hidden.log")
        self.git("commit", "-qm", "fixture")
        self.git("update-index", "--skip-worktree", "hidden.log")

    def paths_sent_to_check_ignore(self, prefilter_size: int) -> tuple[set[str], list[str]]:
        with patch.object(sources, "TRACKED_PREFILTER_MIN_PATHS", prefilter_size), \
                patch("domain_registry.sources.run_git", wraps=run_git) as run:
            found = ignored_relatives(self.repo, ASKED)
        [sent] = [call.kwargs["stdin"] for call in run.call_args_list if "check-ignore" in call.args]
        return found, sent.split(chr(0))

    def test_at_the_prefilter_size_only_paths_the_index_does_not_hold_go_to_check_ignore(self) -> None:
        found, sent = self.paths_sent_to_check_ignore(len(ASKED))
        self.assertEqual(found, IGNORED)
        self.assertEqual(sent, ["hidden.log", "fresh.md", "scratch.log", "build/out.txt"])

    def test_one_path_below_the_prefilter_size_every_path_goes_to_check_ignore(self) -> None:
        found, sent = self.paths_sent_to_check_ignore(len(ASKED) + 1)
        self.assertEqual(found, IGNORED)
        self.assertEqual(sent, ASKED)

    def test_when_git_cannot_list_the_index_check_ignore_still_answers_for_every_path(self) -> None:
        def index_listing_fails(root: object, *arguments: str, **options: object) -> object:
            if "ls-files" in arguments:
                raise OSError("git vanished")
            return run_git(root, *arguments, **options)

        with patch.object(sources, "TRACKED_PREFILTER_MIN_PATHS", 1), \
                patch("domain_registry.sources.run_git", side_effect=index_listing_fails):
            self.assertEqual(ignored_relatives(self.repo, ASKED), IGNORED)

    def test_when_check_ignore_cannot_answer_nothing_is_listed_as_a_source(self) -> None:
        deadline = GitDeadline(["git", "check-ignore"], 1, rerun_is_safe=True)
        failed = subprocess.CompletedProcess(["git", "check-ignore"], 128, "", "fatal: not a git repository\n")
        for label, outcome, raised, message in (
            ("deadline", deadline, GitDeadline, "check-ignore"),
            ("git error", failed, ValueError, "^git check-ignore failed in .*, so ignored files cannot be told from sources: "
             "fatal: not a git repository$"),
        ):
            def check_ignore_answers(root: object, *arguments: str, answer: object = outcome, **options: object) -> object:
                if "check-ignore" in arguments:
                    if isinstance(answer, Exception):
                        raise answer
                    return answer
                return run_git(root, *arguments, **options)

            with self.subTest(label), patch("domain_registry.sources.run_git", side_effect=check_ignore_answers), \
                    self.assertRaisesRegex(raised, message):
                ignored_relatives(self.repo, ASKED)


class SourceWalkCountTest(RegistryTestCase):
    def test_a_source_map_under_a_policy_walks_the_selected_sources_once(self) -> None:
        (self.repo / "docs").mkdir()
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        with patch("domain_registry.sources.source_files", wraps=sources.source_files) as walk:
            source_map = selected_source_map(self.repo, [self.repo / "docs"], POLICY)
        self.assertEqual(walk.call_count, 1)
        self.assertEqual(source_map["source_policy_report"]["files"], 1)
        self.assertEqual(source_map["source_snapshots"][0]["file_count"], 1)

    def test_a_selected_file_is_the_one_source_file_it_selects(self) -> None:
        selected = self.repo / "docs" / "one.md"
        selected.parent.mkdir()
        selected.write_text("one", encoding="utf-8")
        (self.repo / "docs" / "other.md").write_text("other", encoding="utf-8")
        self.assertEqual(source_files(self.repo, [selected]), [selected.resolve()])


if __name__ == "__main__":
    unittest.main()
