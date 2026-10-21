from __future__ import annotations

import json
import shutil
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.audit import anchor_error, event_digest, parents_of, read_events, reconcile
from domain_registry.audit import verify as verify_audit
from domain_registry.common import load_json
from test_support import RegistryTestCase, append_audit

MARKED = "<<<<<<< HEAD\n{ours}=======\n{theirs}>>>>>>> theirs\n"


class AuditMergeTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.memory = self.repo / "memory"
        append_audit(self.memory, {"operation": "seed", "note": "shared"})

    def events_file(self, root: Path | None = None) -> Path:
        return (root or self.memory) / "audit" / "events.jsonl"

    def fork(self) -> Path:
        other = self.repo / "memory-other"
        shutil.copytree(self.memory, other)
        append_audit(self.memory, {"operation": "ours", "note": "a"})
        append_audit(other, {"operation": "theirs", "note": "b"})
        return other

    def merged_with_markers(self, other: Path) -> None:
        shared = self.events_file().read_text(encoding="utf-8").splitlines(keepends=True)[:-1]
        ours = self.events_file().read_text(encoding="utf-8").splitlines(keepends=True)[-1]
        theirs = self.events_file(other).read_text(encoding="utf-8").splitlines(keepends=True)[-1]
        self.events_file().write_text("".join(shared) + MARKED.format(ours=ours, theirs=theirs), encoding="utf-8")
        manifest = self.memory / "audit" / "manifest.json"
        manifest.write_text(
            MARKED.format(ours=manifest.read_text(encoding="utf-8"), theirs=(other / "audit" / "manifest.json").read_text(encoding="utf-8")),
            encoding="utf-8",
        )

    def merged_by_union(self, other: Path) -> None:
        theirs = self.events_file(other).read_text(encoding="utf-8").splitlines(keepends=True)[-1]
        with self.events_file().open("a", encoding="utf-8", newline="\n") as output:
            output.write(theirs)

    def test_two_clones_that_appended_on_the_same_base_are_joined_after_a_conflicted_merge(self) -> None:
        other = self.fork()
        self.merged_with_markers(other)
        before = verify_audit(self.memory)
        result = reconcile(self.memory)
        after = verify_audit(self.memory)
        events = read_events(self.memory)
        ours, theirs = (event["event_sha256"] for event in events[1:3])
        self.assertEqual(
            (
                {"status": "invalid", "reason": f"conflict markers at event 2; run reconcile-audit --registry-root {self.memory.resolve().as_posix()}"},
                {"events": 4, "conflict_markers_removed": 3, "duplicates_dropped": 0, "joined": sorted([ours, theirs]), "written": True},
                ("valid", 4, 3, 1),
                ("reconcile-audit", sorted([ours, theirs]), 3, sorted([ours, theirs])),
                {"ours", "theirs"},
            ),
            (
                before,
                result,
                (after["status"], after["events"], after["height"], len(after["heads"])),
                (events[-1]["operation"], parents_of(events[-1]), events[-1]["sequence"], events[-1]["joined"]),
                {events[1]["operation"], events[2]["operation"]},
            ),
        )

    def test_a_log_merged_by_union_verifies_with_two_heads_and_the_next_append_joins_them(self) -> None:
        other = self.fork()
        self.merged_by_union(other)
        events_before = read_events(self.memory)
        verified = verify_audit(self.memory)
        self.memory.joinpath("audit", "manifest.json").unlink()
        append_audit(self.memory, {"operation": "later"})
        appended = read_events(self.memory)[-1]
        self.assertEqual(
            (
                "invalid", "audit manifest does not match the event log",
                sorted(event["event_sha256"] for event in events_before[1:]), 3,
                [appended["event_sha256"]], "valid",
            ),
            (
                verified["status"], verified["reason"],
                parents_of(appended), appended["sequence"],
                load_json(self.memory / "audit" / "manifest.json")["heads"], verify_audit(self.memory)["status"],
            ),
        )

    def test_reconcile_leaves_a_log_with_one_head_untouched(self) -> None:
        append_audit(self.memory, {"operation": "more"})
        before = (self.events_file().read_bytes(), (self.memory / "audit" / "manifest.json").read_bytes())
        result = reconcile(self.memory)
        after = (self.events_file().read_bytes(), (self.memory / "audit" / "manifest.json").read_bytes())
        self.assertEqual(
            ({"events": 2, "conflict_markers_removed": 0, "duplicates_dropped": 0, "joined": [], "written": False}, before),
            (result, after),
        )

    def test_reconcile_drops_an_event_the_union_merge_repeated(self) -> None:
        last = self.events_file().read_text(encoding="utf-8").splitlines(keepends=True)[-1]
        with self.events_file().open("a", encoding="utf-8", newline="\n") as output:
            output.write(last)
        repeated = verify_audit(self.memory)
        result = reconcile(self.memory)
        self.assertEqual(
            ({"status": "invalid", "reason": "audit event 2 repeats event 1"}, 1, 1, "valid"),
            (repeated, result["duplicates_dropped"], result["events"], verify_audit(self.memory)["status"]),
        )

    def test_reconcile_refuses_a_log_whose_parent_is_missing_rather_than_papering_over_it(self) -> None:
        append_audit(self.memory, {"operation": "more"})
        lines = self.events_file().read_text(encoding="utf-8").splitlines(keepends=True)
        self.events_file().write_text(lines[-1], encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "^audit chain is broken at event 1$"):
            reconcile(self.memory)

    def test_a_join_event_must_name_present_parents_and_sit_one_above_the_highest(self) -> None:
        other = self.fork()
        self.merged_by_union(other)
        reconcile(self.memory)
        events = read_events(self.memory)
        join = events[-1]

        def resigned(event: dict) -> dict:
            unsigned = {key: value for key, value in event.items() if key != "event_sha256"}
            return {**unsigned, "event_sha256": event_digest(unsigned)}

        cases = (
            ("one parent gone", events[:2] + [events[3]], "audit chain is broken at event 3"),
            ("height off by one", events[:3] + [resigned({**join, "sequence": 4})], "audit sequence is invalid at event 4"),
            ("parents listed out of order", events[:3] + [resigned({**join, "previous_event_sha256": list(reversed(join["previous_event_sha256"]))})], None),
        )
        for name, log, reason in cases:
            with self.subTest(name):
                self.events_file().write_text("".join(json.dumps(event, sort_keys=True) + "\n" for event in log), encoding="utf-8")
                (self.memory / "audit" / "manifest.json").unlink(missing_ok=True)
                result = verify_audit(self.memory)
                self.assertEqual(("invalid", reason) if reason else ("valid", None), (result["status"], result.get("reason")))

    def test_a_committed_manifest_anchors_a_log_that_was_merged_and_joined_since(self) -> None:
        self.commit_all("before the fork")
        other = self.fork()
        self.merged_by_union(other)
        reconcile(self.memory)
        extends = anchor_error(self.memory, None, "HEAD")
        lines = self.events_file().read_text(encoding="utf-8").splitlines(keepends=True)
        self.events_file().write_text("".join(lines[1:]), encoding="utf-8")
        self.assertEqual(
            (None, True),
            (extends, str(anchor_error(self.memory, None, "HEAD")).startswith("the audit log does not extend the one committed at HEAD (1 events")),
        )

    def test_the_reconcile_command_reports_the_join_and_exits_zero(self) -> None:
        other = self.fork()
        self.merged_with_markers(other)
        result = self.run_cli("reconcile-audit", "--registry-root", str(self.memory))
        report = json.loads(result.stdout)
        self.assertEqual(
            (0, 2, True, "NOTE: 2 audit heads joined; commit audit/events.jsonl and audit/manifest.json"),
            (result.returncode, len(report["joined"]), report["written"], result.stderr.strip()),
        )

    def test_the_refresh_indexes_command_lists_what_it_rebuilt_and_nothing_on_a_second_run(self) -> None:
        index = self.memory / "registry" / "contexts" / "index.md"
        index.write_text("# Contexts\n", encoding="utf-8")
        first = self.run_cli("refresh-indexes", "--registry-root", str(self.memory))
        second = self.run_cli("refresh-indexes", "--registry-root", str(self.memory))
        self.assertEqual(
            ((0, {"refreshed": ["registry/contexts/index.md"]}), (0, {"refreshed": []})),
            ((first.returncode, json.loads(first.stdout)), (second.returncode, json.loads(second.stdout))),
        )

    def test_a_root_without_a_registry_is_named_in_posix_form_with_the_registry_markers_it_lacks(self) -> None:
        nowhere = self.repo / "nowhere"
        expected = (
            f"no Domain Registry at {nowhere.resolve().as_posix()}: "
            "it holds no registry/index.md; "
        )
        with self.assertRaises(ValueError) as refused:
            reconcile(nowhere)
        self.assertEqual(
            ("invalid", True, True),
            (verify_audit(nowhere)["status"], verify_audit(nowhere)["reason"].startswith(expected), str(refused.exception).startswith(expected)),
        )

    def test_a_linear_log_written_before_the_dag_still_verifies(self) -> None:
        append_audit(self.memory, {"operation": "second"})
        manifest = self.memory / "audit" / "manifest.json"
        stored = load_json(manifest)
        del stored["heads"]
        manifest.write_text(json.dumps(stored), encoding="utf-8")
        result = verify_audit(self.memory)
        self.assertEqual(
            ("valid", 2, 2, [stored["head_sha256"]], stored["head_sha256"]),
            (result["status"], result["events"], result["height"], result["heads"], result["head_sha256"]),
        )


if __name__ == "__main__":
    unittest.main()
