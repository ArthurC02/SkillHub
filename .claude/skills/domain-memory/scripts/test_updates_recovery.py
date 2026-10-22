from __future__ import annotations

import json
import os
import shutil
import stat
import sys
import unittest
from pathlib import Path
from typing import Any
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.revision import registry_digest
from domain_registry.transaction import mutate_registry, recover_interrupted_update, remove_tree, transaction_path
from domain_registry.updates import (
    apply_approved_updates,
    reconcile_pending_update,
    upsert_candidate,
)
from domain_registry.common import MAX_JSON_BYTES, registry_dir
from domain_registry.store import INDEX, has_registry, read_document, write_document
from test_support import RegistryTestCase, append_audit


class UpdatesRecoveryTest(RegistryTestCase):
    def test_recovery_rolls_back_an_installed_transaction_without_its_audit_event(
        self,
    ) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-new"
        staging = root / ".domain-registry-stage-new"
        original = (registry_dir(root) / INDEX).read_bytes()
        shutil.copytree(root / "registry", backup)
        staging.mkdir()
        (registry_dir(root) / INDEX).write_text("{}\n", encoding="utf-8")
        transaction_path(root).write_text(
            json.dumps(
                {
                    "format": "domain-registry-transaction/v2",
                    "operation_id": "new",
                    "phase": "installed",
                    "backup": backup.name,
                    "staging": staging.name,
                }
            ),
            encoding="utf-8",
        )
        recover_interrupted_update(root, False)
        self.assertEqual(original, (registry_dir(root) / INDEX).read_bytes())

    def test_recovery_rolls_back_an_installed_transaction_when_the_last_audit_event_is_another_operations(
        self,
    ) -> None:
        original = (registry_dir(self.repo / "memory") / INDEX).read_bytes()
        root, backup, _ = self.interrupted_transaction("installed")
        append_audit(root, {"operation": "update", "operation_id": "someone-else"})
        recover_interrupted_update(root, False)
        self.assertEqual(
            (original, False, False),
            (
                (registry_dir(root) / INDEX).read_bytes(),
                backup.exists(), transaction_path(root).exists(),
            ),
        )

    def test_recovery_keeps_an_installed_transaction_whose_audit_event_is_present(
        self,
    ) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-audited"
        staging = root / ".domain-registry-stage-audited"
        shutil.copytree(root / "registry", backup)
        staging.mkdir()
        (registry_dir(root) / INDEX).write_text("{}\n", encoding="utf-8")
        append_audit(root, {"operation": "update", "operation_id": "audited"})
        transaction_path(root).write_text(
            json.dumps(
                {
                    "format": "domain-registry-transaction/v2",
                    "operation_id": "audited",
                    "phase": "installed",
                    "backup": backup.name,
                    "staging": staging.name,
                }
            ),
            encoding="utf-8",
        )
        recover_interrupted_update(root, False)
        self.assertEqual(
            "{}\n", (registry_dir(root) / INDEX).read_text(encoding="utf-8")
        )
        self.assertFalse(backup.exists())

    def test_recovery_refuses_an_unrecoverable_installed_transaction(self) -> None:
        root = self.repo / "memory"
        staging = root / ".domain-registry-stage-unrecoverable"
        staging.mkdir()
        transaction_path(root).write_text(
            json.dumps(
                {
                    "format": "domain-registry-transaction/v2",
                    "operation_id": "unrecoverable",
                    "phase": "installed",
                    "backup": ".domain-registry-backup-missing",
                    "staging": staging.name,
                }
            ),
            encoding="utf-8",
        )
        with self.assertRaisesRegex(ValueError, "manual reconciliation"):
            recover_interrupted_update(root, False)

    def test_a_registry_swap_refused_by_windows_restores_the_registry_in_place(self) -> None:
        before = read_document(self.repo / "memory", "contexts.json")
        record = self.write_record("context.json", {"id": "orders", "name": "Orders", "responsibility": "Own orders."})
        replace = Path.replace

        def refuse_installing(source: Path, target: Path) -> Path:
            if source.name == "registry" and source.parent.name.startswith(".domain-registry-stage-"):
                raise PermissionError(5, "Access is denied")
            return replace(source, target)

        self.assert_a_refused_swap_changes_nothing(before, record, refuse_installing)

    def test_a_registry_that_cannot_be_moved_aside_leaves_no_interrupted_transaction(self) -> None:
        before = read_document(self.repo / "memory", "contexts.json")
        record = self.write_record("context.json", {"id": "orders", "name": "Orders", "responsibility": "Own orders."})
        replace = Path.replace

        def refuse_moving_aside(source: Path, target: Path) -> Path:
            if source.name == "registry" and target.name.startswith(".domain-registry-backup-"):
                raise PermissionError(5, "Access is denied")
            return replace(source, target)

        self.assert_a_refused_swap_changes_nothing(before, record, refuse_moving_aside)

    def assert_a_refused_swap_changes_nothing(self, before: dict, record: Path, refuse) -> None:
        with patch.object(Path, "replace", autospec=True, side_effect=refuse), \
                patch("domain_registry.common.time.sleep"), \
                self.assertRaisesRegex(ValueError, r"^another process is holding .*registry or a file in it open "
                                       r"\(error 5\), so the update could not be swapped; nothing was changed"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        self.assertEqual(
            (before, False, []),
            (read_document(self.repo / "memory", "contexts.json"),
             transaction_path(self.repo / "memory").exists(),
             sorted(path.name for path in (self.repo / "memory").glob(".domain-registry-*"))),
        )

    def test_rolling_back_an_installed_transaction_leaves_no_staging_or_backup_directory(self) -> None:
        original = (registry_dir(self.repo / "memory") / INDEX).read_bytes()
        root, _, _ = self.interrupted_transaction("installed")
        recover_interrupted_update(root, False)
        self.assertEqual(
            (original, [], False),
            ((registry_dir(root) / INDEX).read_bytes(), sorted(path.name for path in root.glob(".domain-registry-*")),
             transaction_path(root).exists()),
        )

    def test_an_installed_transaction_whose_staging_directory_is_gone_needs_reconciliation(self) -> None:
        root, _, staging = self.interrupted_transaction("installed")
        staging.rmdir()
        with self.assertRaisesRegex(ValueError, "registry transaction requires manual reconciliation"):
            recover_interrupted_update(root, False)

    def test_an_update_that_fails_at_any_step_leaves_the_registry_as_it_was_and_nothing_behind(self) -> None:
        def write_a_context(staging: Path) -> None:
            write_document(staging, "contexts.json", {"contexts": [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}]})

        def break_it(_staging: Path) -> None:
            raise ValueError("mutation failed")

        def unknown_context(staging: Path) -> None:
            write_document(staging, "rules.json", {"rules": [{"id": "r", "contexts": ["nowhere"], "statement": "S."}]})

        def refuse_the_audit() -> dict:
            raise ValueError("audit refused")

        for label, mutate, audit_event, message in (
            ("the mutation raises", break_it, None, "^mutation failed$"),
            ("the result is invalid", unknown_context, None, "^registry update is invalid: .*unknown context nowhere; add the Context first$"),
            ("the audit event cannot be written", write_a_context, refuse_the_audit, "^audit refused$"),
        ):
            with self.subTest(label):
                root = self.repo / "memory"
                before = registry_digest(root)
                with self.assertRaisesRegex(ValueError, message):
                    mutate_registry(root, self.repo, mutate, audit_event=audit_event)
                self.assertEqual(
                    (before, [], False),
                    (registry_digest(root), sorted(path.name for path in root.glob(".domain-registry-*")),
                     transaction_path(root).exists()),
                )

    def test_a_successful_update_writes_its_change_and_leaves_nothing_behind(self) -> None:
        root = self.repo / "memory"
        mutate_registry(
            root, self.repo,
            lambda staging: write_document(
                staging, "contexts.json", {"contexts": [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}]}
            ),
        )
        self.assertEqual(
            (["orders"], [], False),
            ([context["id"] for context in read_document(root, "contexts.json")["contexts"]],
             sorted(path.name for path in root.glob(".domain-registry-*")), transaction_path(root).exists()),
        )

    def test_an_update_expecting_a_revision_the_registry_has_left_is_refused_and_writes_nothing(self) -> None:
        root = self.repo / "memory"
        index = registry_dir(root) / INDEX
        before = index.read_bytes()
        applied = []
        with self.assertRaisesRegex(ValueError, "^registry revision changed before update; run ship-package again$"):
            mutate_registry(root, self.repo, applied.append, expected_digest="sha256:" + "0" * 64)
        mutate_registry(root, self.repo, applied.append, expected_digest=registry_digest(root))
        self.assertEqual(
            (before, 1, [], False),
            (index.read_bytes(), len(applied), sorted(path.name for path in root.glob(".domain-registry-*")),
             transaction_path(root).exists()),
        )

    def test_recovery_restores_the_backup_when_a_prepared_swap_left_no_registry(self) -> None:
        original = (registry_dir(self.repo / "memory") / INDEX).read_bytes()
        root, backup, _ = self.interrupted_transaction("prepared")
        remove_tree(root / "registry")
        recover_interrupted_update(root, False)
        self.assertEqual(
            (original, False, False),
            ((registry_dir(root) / INDEX).read_bytes(), backup.exists(), transaction_path(root).exists()),
        )

    def test_recovery_discards_a_prepared_transaction_and_keeps_the_registry(self) -> None:
        self.assert_recovery_discards_and_keeps_the_registry("prepared")

    def test_recovery_discards_an_audited_transaction_and_keeps_the_registry(self) -> None:
        self.assert_recovery_discards_and_keeps_the_registry("audited")

    def assert_recovery_discards_and_keeps_the_registry(self, phase: str) -> None:
        root, backup, staging = self.interrupted_transaction(phase)
        recover_interrupted_update(root, False)
        self.assertEqual(
            ("{}\n", False, False, False),
            (
                (registry_dir(root) / INDEX).read_text(encoding="utf-8"),
                backup.exists(), staging.exists(), transaction_path(root).exists(),
            ),
        )

    def test_a_journal_that_is_not_json_too_large_or_not_an_object_is_refused_by_name(self) -> None:
        for label, content, message in (
            ("not JSON", "{not json", "registry recovery journal is not JSON"),
            ("a list", "[]", "registry recovery journal is malformed"),
            ("too large", "[" + " " * MAX_JSON_BYTES + "]", "registry recovery journal is too large"),
        ):
            with self.subTest(label):
                self.tearDown()
                self.setUp()
                root, backup, staging = self.interrupted_transaction("installed")
                transaction_path(root).write_text(content, encoding="utf-8")
                with self.assertRaisesRegex(ValueError, f"^{message}$"):
                    recover_interrupted_update(root, False)
                self.assertEqual((True, True, True), (backup.exists(), staging.exists(), transaction_path(root).exists()))

    def test_a_journal_without_a_phase_or_an_operation_id_is_malformed(self) -> None:
        for missing in ("phase", "operation_id"):
            with self.subTest(missing):
                self.tearDown()
                self.setUp()
                root, _, _ = self.interrupted_transaction("installed")
                journal = json.loads(transaction_path(root).read_text(encoding="utf-8"))
                del journal[missing]
                transaction_path(root).write_text(json.dumps(journal), encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "^registry recovery journal is malformed$"):
                    recover_interrupted_update(root, False)

    def test_recovery_refuses_a_transaction_marked_for_reconciliation_or_in_an_unknown_phase(self) -> None:
        for phase, refusal in (
            ("reconciliation-required", "manual reconciliation"), ("halfway", "invalid phase"),
        ):
            with self.subTest(phase):
                root, backup, _ = self.interrupted_transaction(phase)
                with self.assertRaisesRegex(ValueError, refusal):
                    recover_interrupted_update(root, False)
                self.assertEqual((True, True), (backup.exists(), transaction_path(root).exists()))
                remove_tree(backup)
                remove_tree(root / ".domain-registry-stage-op")
                transaction_path(root).unlink()

    def test_recovery_restores_the_backup_when_an_unaudited_install_left_no_registry(self) -> None:
        original = (registry_dir(self.repo / "memory") / INDEX).read_bytes()
        root, backup, _ = self.interrupted_transaction("installed")
        remove_tree(root / "registry")
        recover_interrupted_update(root, False)
        self.assertEqual(
            (original, False, False),
            (
                (registry_dir(root) / INDEX).read_bytes(),
                backup.exists(), transaction_path(root).exists(),
            ),
        )

    def assert_registry_in_place(self, root: Path) -> None:
        self.assertTrue(has_registry(root))
        self.assertTrue((root / "registry" / "contexts").is_dir())

    def test_recovery_restores_the_registry_a_crash_left_in_the_backup(self) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-abc"
        (root / "registry").replace(backup)
        transaction_path(root).write_text(
            json.dumps(
                {"format": "domain-registry-transaction/v2", "operation_id": "abc", "phase": "prepared",
                 "backup": backup.name, "staging": ".domain-registry-stage-abc"}
            ),
            encoding="utf-8",
        )
        recover_interrupted_update(root, False)
        self.assert_registry_in_place(root)
        self.assertFalse(transaction_path(root).exists())

    def test_recovery_discards_a_backup_whose_folders_are_read_only(self) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-abc"
        shutil.copytree(root / "registry", backup)
        read_only = stat.S_IREAD | stat.S_IEXEC
        for path in [*backup.iterdir(), backup]:
            os.chmod(path, read_only)
        transaction_path(root).write_text(
            json.dumps(
                {"format": "domain-registry-transaction/v2", "operation_id": "abc", "phase": "prepared",
                 "backup": backup.name, "staging": ".domain-registry-stage-abc"}
            ),
            encoding="utf-8",
        )
        recover_interrupted_update(root, False)
        self.assertFalse(backup.exists())
        self.assertFalse(transaction_path(root).exists())
        self.assert_registry_in_place(root)

    def test_a_recovery_journal_naming_an_unsafe_path_is_refused(self) -> None:
        root = self.repo / "memory"
        cases = [
            ("backup climbing out", "../.domain-registry-backup-abc", ".domain-registry-stage-abc"),
            ("backup nested below a folder", ".domain-registry-backup-a/b", ".domain-registry-stage-abc"),
            ("backup naming the registry", "registry", ".domain-registry-stage-abc"),
            ("staging naming the registry", ".domain-registry-backup-abc", "registry"),
            ("staging named like a backup", ".domain-registry-backup-abc", ".domain-registry-backup-x"),
            ("backup named like a staging", ".domain-registry-stage-x", ".domain-registry-stage-abc"),
        ]
        for label, backup, staging in cases:
            with self.subTest(label):
                transaction_path(root).write_text(
                    json.dumps({"format": "domain-registry-transaction/v2", "operation_id": "abc", "phase": "prepared",
                                "backup": backup, "staging": staging}), encoding="utf-8"
                )
                with self.assertRaisesRegex(ValueError, "^registry recovery journal contains an unsafe path$"):
                    recover_interrupted_update(root, False)
                self.assert_registry_in_place(root)

    def test_a_malformed_recovery_journal_is_refused(self) -> None:
        root = self.repo / "memory"
        transaction_path(root).write_text(json.dumps({"backup": 7}), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "malformed"):
            recover_interrupted_update(root, False)

    def test_recovery_refuses_to_break_a_live_writer_lock_without_force(self) -> None:
        root = self.repo / "memory"
        (root / ".domain-registry.lock").mkdir()
        with self.assertRaisesRegex(ValueError, "confirm the writer stopped"):
            recover_interrupted_update(root, False)
        recover_interrupted_update(root, True)
        self.assertFalse((root / ".domain-registry.lock").exists())

    def test_an_apply_that_fails_before_committing_leaves_no_marker(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.redraft_removal(package, "PRO-1", "missing-rule", None)
        with self.assertRaisesRegex(ValueError, "cannot remove absent record"):
            self.approve_and_apply(package)
        self.assertFalse(self.marker().exists())

    def test_an_apply_that_fails_after_its_audit_event_keeps_the_marker(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_finalize(package)

        def committed_then_failed(root: Path, *_args: Any, **_kwargs: Any) -> None:
            append_audit(root, {"operation": "apply-approved-updates", "proposal_id": "PRO-1"})
            raise OSError("disk went away")

        with patch("domain_registry.updates.mutate_registry", side_effect=committed_then_failed), \
                self.assertRaises(OSError):
            apply_approved_updates(package, self.repo / "memory", self.repo)
        self.assertTrue(self.marker().exists())

    def test_reconciling_clears_the_marker_of_a_proposal_that_was_never_applied(self) -> None:
        for status in ("approved", "superseded"):
            with self.subTest(status=status):
                self.tearDown()
                self.setUp()
                proposal_path = self.pending_marker(status, applied=False)
                outcome = reconcile_pending_update(self.repo / "memory", self.repo)
                self.assertIn("PRO-1 was never applied", outcome)
                self.assertFalse(self.marker().exists())
                self.assertEqual(status, json.loads(proposal_path.read_text(encoding="utf-8"))["status"])

    def test_reconciling_marks_applied_when_the_apply_event_is_not_the_latest(self) -> None:
        proposal_path = self.pending_marker("approved", applied=True)
        outcome = reconcile_pending_update(self.repo / "memory", self.repo)
        self.assertIn("now marked applied", outcome)
        self.assertEqual("applied", json.loads(proposal_path.read_text(encoding="utf-8"))["status"])
        self.assertFalse(self.marker().exists())

    def test_reconciling_refuses_an_applied_update_whose_proposal_was_superseded(self) -> None:
        self.pending_marker("superseded", applied=True)
        with self.assertRaisesRegex(ValueError, "records an apply of PRO-1, but its proposal is superseded"):
            reconcile_pending_update(self.repo / "memory", self.repo)
        self.assertTrue(self.marker().exists())

    def test_reconciling_refuses_a_malformed_marker_and_keeps_it(self) -> None:
        for label, marker in (
            ("package is not a string", {"package": 5, "proposal_id": "PRO-1"}),
            ("proposal id is missing", {"package": "proposal"}),
            ("proposal id is a placeholder", {"package": "proposal", "proposal_id": "<pending>"}),
        ):
            with self.subTest(label):
                self.marker().write_text(json.dumps(marker), encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "^registry reconciliation marker is malformed$"):
                    reconcile_pending_update(self.repo / "memory", self.repo)
                self.assertTrue(self.marker().exists())

    def test_reconciling_refuses_a_marker_pointing_outside_the_repository_and_keeps_it(self) -> None:
        self.marker().write_text(
            json.dumps({"package": "../outside", "proposal_id": "PRO-1"}), encoding="utf-8"
        )
        with self.assertRaisesRegex(
            ValueError, "^registry reconciliation marker points outside the repository$"
        ):
            reconcile_pending_update(self.repo / "memory", self.repo)
        self.assertTrue(self.marker().exists())

    def test_reconciling_refuses_a_marker_that_names_another_proposal_and_keeps_it(self) -> None:
        proposal_path = self.pending_marker("approved", applied=True)
        self.marker().write_text(
            json.dumps({"package": "proposal", "proposal_id": "PRO-2"}), encoding="utf-8"
        )
        with self.assertRaisesRegex(
            ValueError, "^registry reconciliation marker does not match its proposal$"
        ):
            reconcile_pending_update(self.repo / "memory", self.repo)
        self.assertEqual(
            ("approved", True),
            (json.loads(proposal_path.read_text(encoding="utf-8"))["status"], self.marker().exists()),
        )

    def test_reconciling_a_proposal_already_marked_applied_only_clears_the_marker(self) -> None:
        proposal_path = self.pending_marker("applied", applied=False)
        outcome = reconcile_pending_update(self.repo / "memory", self.repo)
        self.assertEqual("PRO-1 was already marked applied", outcome)
        self.assertEqual(
            ("applied", False),
            (json.loads(proposal_path.read_text(encoding="utf-8"))["status"], self.marker().exists()),
        )


if __name__ == "__main__":
    unittest.main()
