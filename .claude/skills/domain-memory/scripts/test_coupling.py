from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from domain_registry.coupling import CouplingLimits, CouplingReading, change_coupling, module_of
from test_support import RegistryTestCase

LOW = CouplingLimits(min_revisions=2, min_shared=2, min_degree=0, max_changeset=30)


class ModuleOfTest(unittest.TestCase):
    def test_a_file_belongs_to_the_directory_depth_levels_below_its_root(self) -> None:
        self.assertEqual("src/orders/core", module_of("src/orders/core/deep/total.py", ["src"], 2))

    def test_a_file_shallower_than_the_depth_belongs_to_its_own_directory(self) -> None:
        self.assertEqual("src/orders", module_of("src/orders/total.py", ["src"], 2))

    def test_a_file_directly_in_the_root_belongs_to_no_module(self) -> None:
        self.assertIsNone(module_of("src/main.py", ["src"], 2))

    def test_a_file_outside_every_root_belongs_to_no_module(self) -> None:
        self.assertIsNone(module_of("docs/orders/a.md", ["src"], 2))

    def test_a_root_name_that_only_prefixes_a_directory_does_not_own_it(self) -> None:
        self.assertIsNone(module_of("src-legacy/orders/a.py", ["src"], 1))

    def test_the_repository_root_owns_every_nested_file(self) -> None:
        self.assertEqual("src/orders", module_of("src/orders/a.py", ["."], 2))

    def test_the_first_root_that_holds_the_file_decides(self) -> None:
        self.assertEqual("src/orders", module_of("src/orders/core/a.py", ["src", "src/orders"], 1))


class ChangeCouplingTest(RegistryTestCase):
    def change(self, *paths: str, subject: str = "change") -> None:
        for path in paths:
            target = self.repo / path
            target.parent.mkdir(parents=True, exist_ok=True)
            with target.open("a", encoding="utf-8") as handle:
                handle.write("x\n")
        self.git("add", *paths)
        self.git("commit", "-qm", subject)

    def pairs(self, limits: CouplingLimits = LOW) -> list[dict]:
        return change_coupling(self.repo, ["src"], 1, limits)["pairs"]

    def test_the_degree_is_shared_commits_over_the_average_revisions_of_the_two(self) -> None:
        for _ in range(4):
            self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py")
        self.change("src/a/x.py")
        self.assertEqual(
            [{"modules": ["src/a", "src/b"], "shared": 4, "degree": 80, "subjects": ["change"] * 4}], self.pairs()
        )

    def test_a_pair_lists_the_subjects_of_its_newest_shared_commits_up_to_ten(self) -> None:
        for number in range(11):
            self.change("src/a/x.py", "src/b/y.py", subject=f"step {number}")
        self.assertEqual([f"step {number}" for number in range(10, 0, -1)], self.pairs()[0]["subjects"])

    def test_a_commit_whose_subject_matches_an_exclusion_is_not_counted(self) -> None:
        before = change_coupling(self.repo, ["src"], 1, LOW, CouplingReading(exclude_subjects=("^chore",)))
        for _ in range(2):
            self.change("src/a/x.py", "src/b/y.py", subject="chore: sweep comments")
            self.change("src/a/x.py", "src/b/y.py")
        result = change_coupling(self.repo, ["src"], 1, LOW, CouplingReading(exclude_subjects=("^chore",)))
        self.assertEqual(
            (2, [{"modules": ["src/a", "src/b"], "shared": 2, "degree": 100, "subjects": ["change", "change"]}]),
            (result["excluded_commits"] - before["excluded_commits"], result["pairs"]),
        )

    def test_an_exclusion_that_is_not_a_regular_expression_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "--exclude-subject is not a regular expression"):
            change_coupling(self.repo, ["src"], 1, LOW, CouplingReading(exclude_subjects=("(",)))

    def test_a_module_is_named_by_the_context_whose_longest_path_it_lies_in(self) -> None:
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py", "src/b/y.py")
        reading = CouplingReading(context_paths={"alpha": "src/a", "wide": "src"})
        self.assertEqual(
            [["alpha", "wide"]],
            [pair["contexts"] for pair in change_coupling(self.repo, ["src"], 1, LOW, reading)["pairs"]],
        )

    def test_a_module_outside_every_context_is_named_by_none(self) -> None:
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py", "src/b/y.py")
        reading = CouplingReading(context_paths={"beta": "b"})
        self.assertEqual(
            [[None, "beta"]],
            [pair["contexts"] for pair in change_coupling(self.repo, ["src"], 1, LOW, reading)["pairs"]],
        )

    def test_without_contexts_a_pair_names_none(self) -> None:
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py", "src/b/y.py")
        self.assertNotIn("contexts", self.pairs()[0])

    def test_a_pair_sharing_the_minimum_number_of_commits_is_kept_and_one_fewer_is_dropped(self) -> None:
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py", "src/c/z.py")
        self.change("src/c/z.py")
        self.assertEqual([["src/a", "src/b"]], [pair["modules"] for pair in self.pairs()])

    def test_a_pair_is_dropped_when_either_module_is_below_the_minimum_revisions(self) -> None:
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py")
        self.assertEqual([], self.pairs(CouplingLimits(min_revisions=3, min_shared=2, min_degree=0, max_changeset=30)))
        self.assertEqual(1, len(self.pairs(CouplingLimits(min_revisions=2, min_shared=2, min_degree=0, max_changeset=30))))

    def test_a_pair_at_the_minimum_degree_is_kept_and_below_it_is_dropped(self) -> None:
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py", "src/b/y.py")
        self.change("src/a/x.py")
        self.change("src/a/x.py")
        self.assertEqual(67, self.pairs()[0]["degree"])
        self.assertEqual(1, len(self.pairs(CouplingLimits(min_revisions=2, min_shared=2, min_degree=67, max_changeset=30))))
        self.assertEqual([], self.pairs(CouplingLimits(min_revisions=2, min_shared=2, min_degree=68, max_changeset=30)))

    def test_a_commit_at_the_changeset_limit_counts_and_one_file_over_it_is_skipped(self) -> None:
        limits = CouplingLimits(min_revisions=1, min_shared=1, min_degree=0, max_changeset=3)
        before = change_coupling(self.repo, ["src"], 1, limits)
        self.change("src/a/x.py", "src/b/y.py", "src/c/z.py")
        self.change("src/a/x.py", "src/b/y.py", "src/c/z.py", "src/d/w.py")
        result = change_coupling(self.repo, ["src"], 1, limits)
        self.assertEqual(
            (2, 1),
            (result["commits"] - before["commits"], result["skipped_large_commits"] - before["skipped_large_commits"]),
        )
        self.assertEqual([1, 1, 1], [pair["shared"] for pair in result["pairs"]])

    def test_a_module_that_no_longer_exists_is_left_out(self) -> None:
        self.change("src/a/x.py", "src/gone/y.py")
        self.change("src/a/x.py", "src/gone/y.py")
        self.git("rm", "-rq", "src/gone")
        self.git("commit", "-qm", "remove")
        result = change_coupling(self.repo, ["src"], 1, LOW)
        self.assertEqual((["src/a"], []), ([module["module"] for module in result["modules"]], result["pairs"]))

    def test_a_directory_that_is_not_a_repository_is_named(self) -> None:
        with tempfile.TemporaryDirectory() as empty, self.assertRaisesRegex(ValueError, "git could not read the history: "):
            change_coupling(Path(empty), ["src"], 1, LOW)

    def test_the_command_prints_the_coupling_and_refuses_a_depth_below_one(self) -> None:
        self.change("src/a/x.py", "src/b/y.py")
        result = self.run_cli("change-coupling", "--repo-root", str(self.repo), "--root", "src", "--depth", "1",
                              "--min-revisions", "1", "--min-shared", "1")
        self.assertEqual([["src/a", "src/b"]], [pair["modules"] for pair in json.loads(result.stdout)["pairs"]])
        refused = self.run_cli("change-coupling", "--repo-root", str(self.repo), "--depth", "0")
        self.assertEqual((1, "ERROR: change-coupling --depth must be at least 1"), (refused.returncode, refused.stdout.strip()))


if __name__ == "__main__":
    unittest.main()
