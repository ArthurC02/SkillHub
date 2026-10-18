from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.sources import (
    selected_source_map,
    verify_against_discovery,
    write_source_map,
)
from test_support import RegistryTestCase


class VerifySourcesDriftTest(RegistryTestCase):
    def map_with_tests(self, *names: str) -> Path:
        (self.repo / "tests").mkdir(exist_ok=True)
        for name in names:
            (self.repo / "tests" / name).write_text(f"{name}\n", encoding="utf-8")
        return self.selected_docs()

    def verify(self, source_map_path: Path):
        return self.run_cli(
            "verify-sources", "--repo-root", str(self.repo), "--source-map", str(source_map_path)
        )

    def test_a_listed_path_that_is_gone_and_a_new_match_report_discovery_drifted_with_a_singular_note(
        self,
    ) -> None:
        source_map_path = self.map_with_tests("test_kept.py", "test_gone.py")
        (self.repo / "tests" / "test_gone.py").unlink()
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        result = self.verify(source_map_path)
        report = json.loads(result.stdout)
        self.assertEqual(
            (
                0,
                "discovery_drifted",
                ["tests/test_gone.py"],
                ["tests/test_new.py"],
                "NOTE: discovery drifted: 1 listed path gone, 1 matching path unlisted; "
                "run refresh-sources, then confirm-sources",
            ),
            (
                result.returncode,
                report["status"],
                report["discovered_but_gone"],
                report["discovered_but_unlisted"],
                result.stderr.strip(),
            ),
        )

    def test_several_gone_and_unlisted_paths_are_counted_in_the_plural_and_listed_sorted(
        self,
    ) -> None:
        source_map_path = self.map_with_tests("test_a_gone.py", "test_b_gone.py")
        (self.repo / "tests" / "test_a_gone.py").unlink()
        (self.repo / "tests" / "test_b_gone.py").unlink()
        (self.repo / "tests" / "test_z_new.py").write_text("z\n", encoding="utf-8")
        (self.repo / "tests" / "test_y_new.py").write_text("y\n", encoding="utf-8")
        result = self.verify(source_map_path)
        report = json.loads(result.stdout)
        self.assertEqual(
            (
                ["tests/test_y_new.py", "tests/test_z_new.py"],
                "NOTE: discovery drifted: 2 listed paths gone, 2 matching paths unlisted; "
                "run refresh-sources, then confirm-sources",
            ),
            (report["discovered_but_unlisted"], result.stderr.strip()),
        )

    def test_only_an_unlisted_match_drifts_discovery_and_counts_zero_gone(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        result = self.verify(source_map_path)
        self.assertEqual(
            (
                0,
                "discovery_drifted",
                "NOTE: discovery drifted: 0 listed paths gone, 1 matching path unlisted; "
                "run refresh-sources, then confirm-sources",
            ),
            (result.returncode, json.loads(result.stdout)["status"], result.stderr.strip()),
        )

    def test_only_a_gone_path_drifts_discovery_and_counts_zero_unlisted(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py", "test_gone.py")
        (self.repo / "tests" / "test_gone.py").unlink()
        result = self.verify(source_map_path)
        self.assertEqual(
            (
                0,
                "discovery_drifted",
                "NOTE: discovery drifted: 1 listed path gone, 0 matching paths unlisted; "
                "run refresh-sources, then confirm-sources",
            ),
            (result.returncode, json.loads(result.stdout)["status"], result.stderr.strip()),
        )

    def test_a_map_that_matches_discovery_is_current_with_empty_lists_and_no_note(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        result = self.verify(source_map_path)
        report = json.loads(result.stdout)
        self.assertEqual(
            (0, "current", [], [], ""),
            (
                result.returncode,
                report["status"],
                report["discovered_but_gone"],
                report["discovered_but_unlisted"],
                result.stderr.strip(),
            ),
        )

    def test_an_edit_inside_a_listed_file_is_content_drift_alone_and_leaves_discovery_current(
        self,
    ) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        (self.repo / "docs" / "rules.md").write_text("revised\n", encoding="utf-8")
        result = self.verify(source_map_path)
        report = json.loads(result.stdout)
        self.assertEqual(
            (0, "current", ["docs"], [], ""),
            (
                result.returncode,
                report["status"],
                [entry["path"] for entry in report["content_changed"]],
                report["discovered_but_unlisted"],
                result.stderr.strip(),
            ),
        )

    def test_a_moved_selection_stays_stale_while_the_note_still_names_the_unlisted_path(
        self,
    ) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        (self.repo / "docs" / "extra.md").write_text("second\n", encoding="utf-8")
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        result = self.verify(source_map_path)
        report = json.loads(result.stdout)
        self.assertEqual(
            (1, "stale", ["tests/test_new.py"], True),
            (
                result.returncode,
                report["status"],
                report["discovered_but_unlisted"],
                "1 matching path unlisted" in result.stderr,
            ),
        )

    def test_a_map_without_snapshots_stays_unverified_and_gains_no_discovery_fields(self) -> None:
        self.map_with_tests("test_kept.py")
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, {"source_groups": [{"kind": "tests", "paths": ["tests/test_kept.py"]}]})
        result = verify_against_discovery(self.repo, path)
        self.assertEqual(
            ("unverified", False),
            (result["status"], "discovered_but_unlisted" in result),
        )

    def refresh(self):
        return self.run_cli(
            "refresh-sources", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo)
        )

    def test_refresh_names_the_one_selected_source_whose_confirmation_it_reset(self) -> None:
        self.committed_source_map()
        self.permit_sources()
        result = self.refresh()
        self.assertEqual(
            (0, "Selection reset to agent-asserted; confirm this 1 selected source: docs"),
            (result.returncode, result.stdout.strip()),
        )

    def test_refresh_names_every_selected_source_in_the_plural(self) -> None:
        self.committed_source_map()
        self.permit_sources()
        (self.repo / "extra.md").write_text("extra\n", encoding="utf-8")
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs", self.repo / "extra.md"]),
        )
        result = self.refresh()
        self.assertEqual(
            (0, "Selection reset to agent-asserted; confirm these 2 selected sources: docs, extra.md"),
            (result.returncode, result.stdout.strip()),
        )


if __name__ == "__main__":
    unittest.main()
