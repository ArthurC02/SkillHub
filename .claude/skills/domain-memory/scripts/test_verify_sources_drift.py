from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.readiness import assess_readiness
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
                f"run refresh-listing --registry-root {source_map_path.parent.as_posix()} "
                f"--repo-root {self.repo.as_posix()} (no confirmation needed)",
            ),
            (
                result.returncode,
                report["status"],
                report["discovered_but_gone"],
                report["discovered_but_unlisted"],
                result.stderr.strip(),
            ),
        )

    def test_a_listed_path_discovery_no_longer_places_counts_as_gone_while_its_file_remains(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        (self.repo / "notes").mkdir()
        (self.repo / "notes" / "README.md").write_text("notes\n", encoding="utf-8")
        stored = json.loads(source_map_path.read_text(encoding="utf-8"))
        next(group for group in stored["source_groups"] if group["kind"] == "tests")["paths"].append("notes/README.md")
        write_source_map(source_map_path, stored)
        result = self.verify(source_map_path)
        report = json.loads(result.stdout)
        self.assertEqual(
            ("discovery_drifted", ["notes/README.md"], [], True,
             "NOTE: discovery drifted: 1 listed path gone, 0 matching paths unlisted; "
             f"run refresh-listing --registry-root {source_map_path.parent.as_posix()} "
             f"--repo-root {self.repo.as_posix()} (no confirmation needed)"),
            (report["status"], report["discovered_but_gone"], report["discovered_but_unlisted"],
             (self.repo / "notes" / "README.md").is_file(), result.stderr.strip()),
        )

    def test_readiness_reports_listing_drift_as_a_signal_and_a_note_without_blocking(self) -> None:
        self.permit_sources()
        source_map_path = self.map_with_tests("test_kept.py")
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        assessed = assess_readiness(self.repo, self.repo / "memory")
        result = self.run_cli("readiness", "--repo-root", str(self.repo), "--registry-root", str(self.repo / "memory"))
        self.assertEqual(
            ([], {"kind": "listing", "status": "drifted", "discovered_but_gone": [], "discovered_but_unlisted": ["tests/test_new.py"]},
             0, "NOTE: discovery drifted: 0 listed paths gone, 1 matching path unlisted; "
                f"run refresh-listing --registry-root {source_map_path.parent.as_posix()} "
                f"--repo-root {self.repo.as_posix()} (no confirmation needed)"),
            (assessed["blocks"], next(s for s in assessed["signals"] if s["kind"] == "listing"),
             result.returncode, result.stderr.strip()),
        )

    def test_readiness_reports_a_current_listing_silently(self) -> None:
        self.permit_sources()
        self.map_with_tests("test_kept.py")
        assessed = assess_readiness(self.repo, self.repo / "memory")
        result = self.run_cli("readiness", "--repo-root", str(self.repo), "--registry-root", str(self.repo / "memory"))
        self.assertEqual(
            ({"kind": "listing", "status": "current", "discovered_but_gone": [], "discovered_but_unlisted": []}, ""),
            (next(s for s in assessed["signals"] if s["kind"] == "listing"), result.stderr.strip()),
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
                f"run refresh-listing --registry-root {source_map_path.parent.as_posix()} "
                f"--repo-root {self.repo.as_posix()} (no confirmation needed)",
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
                f"run refresh-listing --registry-root {source_map_path.parent.as_posix()} "
                f"--repo-root {self.repo.as_posix()} (no confirmation needed)",
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
                f"run refresh-listing --registry-root {source_map_path.parent.as_posix()} "
                f"--repo-root {self.repo.as_posix()} (no confirmation needed)",
            ),
            (result.returncode, json.loads(result.stdout)["status"], result.stderr.strip()),
        )

    def test_an_edit_inside_a_selected_source_beside_drift_is_named_in_the_note_as_a_citation_matter(
        self,
    ) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        (self.repo / "docs" / "rules.md").write_text("revised\n", encoding="utf-8")
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        result = self.verify(source_map_path)
        self.assertEqual(
            (
                0,
                "discovery_drifted",
                "NOTE: discovery drifted: 0 listed paths gone, 1 matching path unlisted; "
                "1 selected source changed inside; verify-evidence checks the citations; "
                f"run refresh-listing --registry-root {source_map_path.parent.as_posix()} "
                f"--repo-root {self.repo.as_posix()} (no confirmation needed)",
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
            (
                1,
                "stale",
                ["tests/test_new.py"],
                "NOTE: discovery drifted: 0 listed paths gone, 1 matching path unlisted; "
                "the selection is stale, so run refresh-sources, then confirm-sources",
            ),
            (
                result.returncode,
                report["status"],
                report["discovered_but_unlisted"],
                result.stderr.strip(),
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

    def refresh_listing(self):
        self.permit_sources()
        return self.run_cli(
            "refresh-listing", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo)
        )

    def stored_map(self) -> dict:
        return json.loads((self.repo / "memory" / "source-map.json").read_text(encoding="utf-8"))

    def recorded_operations(self) -> list[str]:
        log = self.repo / "memory" / "audit" / "events.jsonl"
        return [event["operation"] for event in self.audit_lines()] if log.is_file() else []

    def without_listing(self, source_map: dict) -> dict:
        return {key: value for key, value in source_map.items() if key not in ("source_groups", "missing_groups")}

    def test_refresh_listing_replaces_only_the_listing_and_leaves_the_confirmation_standing(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py", "test_gone.py")
        confirmed = self.stored_map() | {"selection_status": "developer-confirmed", "confirmed_by": "Ada", "confirmed_at": "2026-01-01T00:00:00Z"}
        write_source_map(source_map_path, confirmed)
        (self.repo / "tests" / "test_gone.py").unlink()
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        result = self.refresh_listing()
        after = self.stored_map()
        tests_group = next(group for group in after["source_groups"] if group["kind"] == "tests")
        self.assertEqual(
            (
                0,
                "Listing refreshed: +1 -1 discovered paths\nadded: tests/test_new.py\nremoved: tests/test_gone.py",
                self.without_listing(confirmed),
                ["tests/test_kept.py", "tests/test_new.py"],
                {"operation": "refresh-listing", "added": ["tests/test_new.py"], "removed": ["tests/test_gone.py"]},
            ),
            (
                result.returncode,
                result.stdout.strip(),
                self.without_listing(after),
                tests_group["paths"],
                {key: self.audit_lines()[-1][key] for key in ("operation", "added", "removed")},
            ),
        )

    def test_a_map_refreshed_by_refresh_listing_verifies_current_without_a_note(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py", "test_gone.py")
        (self.repo / "tests" / "test_gone.py").unlink()
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        self.refresh_listing()
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

    def test_a_single_added_path_is_counted_in_the_singular_and_fills_the_group_it_was_missing(self) -> None:
        self.selected_docs()
        before = self.stored_map()
        (self.repo / "tests").mkdir()
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        result = self.refresh_listing()
        after = self.stored_map()
        self.assertEqual(
            (0, "Listing refreshed: +1 -0 discovered path\nadded: tests/test_new.py", True, False, self.without_listing(before)),
            (
                result.returncode,
                result.stdout.strip(),
                "tests" in before["missing_groups"],
                "tests" in after["missing_groups"],
                self.without_listing(after),
            ),
        )

    def test_refresh_listing_with_no_drift_writes_nothing(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        bytes_before = source_map_path.read_bytes()
        mtime_before = source_map_path.stat().st_mtime_ns
        operations_before = self.recorded_operations()
        result = self.refresh_listing()
        self.assertEqual(
            (0, "Listing already current", bytes_before, mtime_before, operations_before),
            (
                result.returncode,
                result.stdout.strip(),
                source_map_path.read_bytes(),
                source_map_path.stat().st_mtime_ns,
                self.recorded_operations(),
            ),
        )

    def test_refresh_listing_refuses_a_stale_selection_and_writes_nothing(self) -> None:
        source_map_path = self.map_with_tests("test_kept.py")
        (self.repo / "docs" / "extra.md").write_text("second\n", encoding="utf-8")
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        bytes_before = source_map_path.read_bytes()
        operations_before = self.recorded_operations()
        result = self.refresh_listing()
        self.assertEqual(
            (
                1,
                "ERROR: the selection is stale: docs; run refresh-sources, then confirm-sources",
                "",
                bytes_before,
                operations_before,
            ),
            (
                result.returncode,
                result.stderr.strip(),
                result.stdout.strip(),
                source_map_path.read_bytes(),
                self.recorded_operations(),
            ),
        )

    def test_refresh_listing_refuses_an_unverified_selection_and_writes_nothing(self) -> None:
        self.map_with_tests("test_kept.py")
        source_map_path = self.repo / "memory" / "source-map.json"
        write_source_map(source_map_path, {"source_groups": [{"kind": "tests", "paths": ["tests/test_kept.py"]}]})
        (self.repo / "tests" / "test_new.py").write_text("new\n", encoding="utf-8")
        bytes_before = source_map_path.read_bytes()
        operations_before = self.recorded_operations()
        result = self.refresh_listing()
        self.assertEqual(
            (
                1,
                "ERROR: the selection is unverified: source map has no snapshots; "
                "run refresh-sources, then confirm-sources",
                bytes_before,
                operations_before,
            ),
            (result.returncode, result.stderr.strip(), source_map_path.read_bytes(), self.recorded_operations()),
        )


if __name__ == "__main__":
    unittest.main()
