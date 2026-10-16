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

from domain_registry.audit import read_events as read_audit_events
from domain_registry.audit import event_digest as audit_event_digest
from domain_registry.audit import reviewing_proposals
from domain_registry.audit import anchor_error
from domain_registry.audit import verify as verify_audit
from domain_registry.evidence import (
    citation,
    digest,
    verify,
)
from domain_registry.policy import amend_policy_fields
from domain_registry.registry import (
    asset_records,
    migrate_evidence,
    verify_evidence,
)
from domain_registry.revision import (
    current_registry_revision,
    registry_digest,
    require_current_registry_revision,
    valid_commit,
    valid_digest,
    valid_registry_revision,
)
from domain_registry.sources import (
    confirm_sources,
    selected_source_map,
    verify_source_map,
    write_source_map,
)
from domain_registry.transaction import recover_interrupted_update, remove_tree, transaction_path
from domain_registry.updates import (
    apply_approved_updates,
    demote_local_reviews,
    promote_candidate,
    reconcile_pending_update,
    retract_candidate,
    upsert_candidate,
)
from domain_registry.store import (
    MANIFEST,
    document_path,
    is_concept_layout,
    read_document,
    write_document,
)
from test_support import VERIFIED, RegistryTestCase, append_audit, amend_policy, applied_by, snapshot_of_files, use_json_layout


class UpdatesTest(RegistryTestCase):
    def test_changed_registry_rejects_a_captured_revision(self) -> None:
        use_json_layout(self.repo / "memory")
        expected = current_registry_revision(self.repo / "memory", self.repo)
        value = read_document(self.repo / "memory", "contexts.json")
        value["note"] = "changed"
        write_document(self.repo / "memory", "contexts.json", value)
        with self.assertRaisesRegex(ValueError, "stale"):
            require_current_registry_revision(
                {"base_registry_revision": expected}, self.repo / "memory", self.repo
            )

    def test_a_stale_base_holds_only_while_what_the_proposal_touches_is_unchanged(self) -> None:
        use_json_layout(self.repo / "memory")
        def billing_renamed(value):
            value["contexts"][1]["name"] = "Invoicing"

        def orders_renamed(value):
            value["contexts"][0]["name"] = "Purchases"

        def rule_written(value):
            value["rules"].append({"id": "order-total", "contexts": ["orders"]})

        def context_list_annotated(value):
            value["note"] = "changed"

        cases = [
            ("an untouched record changed", "contexts.json", billing_renamed, None),
            ("a record it references changed", "contexts.json", orders_renamed, "contexts/orders"),
            ("the record it writes changed", "rules.json", rule_written, "rules/order-total"),
            ("an asset changed outside its records", "contexts.json", context_list_annotated,
             "contexts.json changed outside its records"),
        ]
        for label, name, change, refusal in cases:
            with self.subTest(label):
                self.two_contexts()
                self.seed("rules.json", [])
                self.commit_everything("base")
                proposal = {
                    "base_registry_revision": current_registry_revision(self.repo / "memory", self.repo),
                    "registry_updates": [{
                        "operation": "upsert", "asset": "rules",
                        "record": {"id": "order-total", "contexts": ["orders"]},
                    }],
                }
                self.edit_registry(name, change)
                if refusal is None:
                    self.assertEqual(
                        registry_digest(self.repo / "memory"),
                        require_current_registry_revision(proposal, self.repo / "memory", self.repo),
                    )
                else:
                    with self.assertRaisesRegex(ValueError, f"stale.*{refusal}"):
                        require_current_registry_revision(proposal, self.repo / "memory", self.repo)

    def test_a_stale_base_that_was_never_committed_cannot_be_compared(self) -> None:
        self.two_contexts()
        proposal = {
            "base_registry_revision": current_registry_revision(self.repo / "memory", self.repo),
            "registry_updates": [{"operation": "upsert", "asset": "rules", "record": {"id": "order-total"}}],
        }
        self.edit_registry("contexts.json", lambda value: value["contexts"][1].update(name="Invoicing"))
        with self.assertRaisesRegex(ValueError, "stale.*cannot be rebuilt"):
            require_current_registry_revision(proposal, self.repo / "memory", self.repo)

    def test_invalid_candidate_does_not_change_registry(self) -> None:
        before = read_document(self.repo / "memory", "vocabulary.json")
        record = self.write_record(
            "term.json",
            {
                "id": "order",
                "name": "Order",
                "definition": "A purchase.",
                "contexts": ["missing"],
            },
        )
        with self.assertRaisesRegex(ValueError, "unknown context"):
            upsert_candidate(self.repo / "memory", self.repo, "vocabulary", record)
        self.assertEqual(before, read_document(self.repo / "memory", "vocabulary.json"))

    def test_a_submitted_record_cannot_carry_its_own_approval(self) -> None:
        record = self.write_record(
            "context.json",
            {
                "id": "orders",
                "name": "Orders",
                "responsibility": "Own orders.",
                "status": "reviewed",
                "review": {
                    "proposal_id": "forged",
                    "proposal_revision": "forged",
                    "approvals": [{"role": "owner", "reviewer": "nobody"}],
                },
                "verified": [{"by": "human:nobody", "at": "2026-01-01T00:00:00Z"}],
            },
        )
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        stored = read_document(self.repo / "memory", "contexts.json")
        written = next(
            entry for entry in stored["contexts"] if entry["id"] == "orders"
        )
        self.assertEqual(
            ("candidate", False, False), (written["status"], "review" in written, "verified" in written)
        )

    def test_audit_failure_rolls_back_a_registry_swap(self) -> None:
        before = read_document(self.repo / "memory", "contexts.json")
        record = self.write_record(
            "context.json",
            {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
        )
        with (
            patch(
                "domain_registry.audit.append_locked",
                side_effect=OSError("audit unavailable"),
            ),
            self.assertRaisesRegex(OSError, "audit unavailable"),
        ):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        self.assertEqual(before, read_document(self.repo / "memory", "contexts.json"))
        self.assertFalse(
            (self.repo / "memory" / ".domain-registry-transaction.json").exists()
        )

    def test_migrate_evidence_records_how_many_references_it_upgraded(self) -> None:
        self.seed_line_reference_evidence()
        migrate_evidence(self.repo / "memory", self.repo)
        migrations = [
            event["migrated"]
            for event in read_audit_events(self.repo / "memory")
            if event["operation"] == "migrate-evidence"
        ]
        self.assertEqual([1], migrations)

    def test_a_registry_digest_is_sha256_and_64_lowercase_hex(self) -> None:
        hexes = "a" * 64
        for value, accepted in (
            (f"sha256:{hexes}", True),
            (f"sha256:{hexes[:-1]}", False),
            (f"sha256:{hexes}a", False),
            (f"sha256:{'A' * 64}", False),
            (f"sha512:{hexes}", False),
            (hexes, False),
            (hexes + "abcdefg", False),
            (None, False),
        ):
            with self.subTest(value=value):
                self.assertIs(accepted, valid_digest(value))

    def test_a_commit_is_40_lowercase_hex(self) -> None:
        for value, accepted in (
            ("f" * 40, True),
            ("f" * 39, False),
            ("f" * 41, False),
            ("F" * 40, False),
            ("g" * 40, False),
            (40, False),
            (10**39, False),
        ):
            with self.subTest(value=value):
                self.assertIs(accepted, valid_commit(value))

    def test_a_registry_revision_needs_a_valid_digest_and_an_absent_or_valid_commit(self) -> None:
        digest_value = "sha256:" + "0" * 64
        for value, accepted in (
            ({"observed_commit": None, "registry_digest": digest_value}, True),
            ({"observed_commit": "a" * 40, "registry_digest": digest_value}, True),
            ({"observed_commit": "a" * 39, "registry_digest": digest_value}, False),
            ({"observed_commit": None, "registry_digest": "sha256:0"}, False),
            ("sha256:" + "0" * 64, False),
        ):
            with self.subTest(value=value):
                self.assertIs(accepted, valid_registry_revision(value))

    def test_an_audit_chain_where_no_registry_exists_is_not_called_valid(self) -> None:
        self.assertEqual(
            ("valid", 0), tuple(verify_audit(self.repo / "memory")[key] for key in ("status", "events"))
        )
        result = verify_audit(self.repo / "nowhere")
        self.assertEqual("invalid", result["status"])
        self.assertIn("no Domain Memory Registry", result["reason"])

    def packages(self, **statuses: dict) -> Path:
        changes = self.repo / "changes"
        for name, fields in statuses.items():
            (changes / name).mkdir(parents=True)
            (changes / name / "domain-change-proposal.json").write_text(
                json.dumps({"proposal_id": name.upper(), **fields}), encoding="utf-8")
        return changes

    def test_an_applied_proposal_without_its_apply_event_makes_the_audit_invalid(self) -> None:
        root = self.repo / "memory"
        applied_by(root, "KEPT")
        changes = self.packages(kept={"status": "applied"}, lost={"status": "applied"},
                                replaced={"status": "superseded", "superseded_from_status": "applied"},
                                draft={"status": "draft"})
        self.assertEqual(
            "applied proposals have no apply-approved-updates event in the audit log: lost (LOST), replaced (REPLACED)",
            anchor_error(root, changes, None),
        )

    def test_every_applied_proposal_with_its_apply_event_anchors_the_audit(self) -> None:
        root = self.repo / "memory"
        applied_by(root, "KEPT")
        self.assertIsNone(anchor_error(root, self.packages(kept={"status": "applied"}, draft={"status": "draft"}), None))

    def test_verify_audit_checks_the_anchors_it_is_given_from_the_command_line(self) -> None:
        root = self.repo / "memory"
        changes = self.packages(lost={"status": "applied"})
        anchored = self.run_cli("verify-audit", "--registry-root", str(root), "--changes-root", str(changes))
        unresolved = self.run_cli("verify-audit", "--registry-root", str(root), "--against", "no-such-ref")
        self.assertEqual(
            (1, "applied proposals have no apply-approved-updates event in the audit log: lost (LOST)", 1,
             "ERROR: git cannot resolve no-such-ref from"),
            (anchored.returncode, json.loads(anchored.stdout)["reason"], unresolved.returncode,
             unresolved.stdout.strip().split(" from ")[0] + " from"),
        )

    def committed_log(self, count: int) -> Path:
        root = self.repo / "memory"
        for number in range(count):
            append_audit(root, {"operation": f"step-{number}"})
        self.commit_all("audit")
        return root

    def test_a_log_truncated_consistently_since_a_commit_does_not_extend_it(self) -> None:
        root = self.committed_log(3)
        events_file, manifest_file = root / "audit" / "events.jsonl", root / "audit" / "manifest.json"
        kept = events_file.read_text(encoding="utf-8").splitlines()[:2]
        events_file.write_text("".join(f"{line}\n" for line in kept), encoding="utf-8")
        manifest = json.loads(manifest_file.read_text(encoding="utf-8"))
        manifest.update(events=2, head_sha256=json.loads(kept[-1])["event_sha256"])
        manifest_file.write_text(json.dumps(manifest), encoding="utf-8")
        self.assertEqual("valid", verify_audit(root)["status"])
        self.assertRegex(anchor_error(root, None, "HEAD"), "^the audit log does not extend the one committed at HEAD [(]3 events")

    def test_a_log_rewritten_since_a_commit_does_not_extend_it(self) -> None:
        root = self.committed_log(2)
        events_file = root / "audit" / "events.jsonl"
        events_file.unlink()
        (root / "audit" / "manifest.json").unlink()
        for operation in ("other-0", "other-1", "other-2"):
            append_audit(root, {"operation": operation})
        self.assertRegex(anchor_error(root, None, "HEAD"), "^the audit log does not extend the one committed at HEAD")

    def test_a_log_grown_since_a_commit_extends_it(self) -> None:
        root = self.committed_log(2)
        append_audit(root, {"operation": "later"})
        self.assertIsNone(anchor_error(root, None, "HEAD"))

    def test_a_revision_without_a_committed_manifest_anchors_nothing(self) -> None:
        root = self.repo / "memory"
        (self.repo / "README.md").write_text("before the registry\n", encoding="utf-8")
        self.git("add", "README.md")
        self.git("commit", "-qm", "before")
        append_audit(root, {"operation": "later"})
        self.assertEqual(
            "no audit manifest committed at HEAD:memory/audit/manifest.json, so nothing anchors the log",
            anchor_error(root, None, "HEAD"),
        )

    def test_a_revision_git_cannot_resolve_is_an_error(self) -> None:
        root = self.committed_log(1)
        with self.assertRaisesRegex(ValueError, "^git cannot resolve no-such-ref from "):
            anchor_error(root, None, "no-such-ref")

    def test_audit_chain_detects_tampering(self) -> None:
        append_audit(self.repo / "memory", {"operation": "first"})
        append_audit(self.repo / "memory", {"operation": "second"})
        self.assertEqual("valid", verify_audit(self.repo / "memory")["status"])
        manifest = json.loads(
            (self.repo / "memory" / "audit" / "manifest.json").read_text(
                encoding="utf-8"
            )
        )
        events = (
            (self.repo / "memory" / "audit" / "events.jsonl")
            .read_text(encoding="utf-8")
            .splitlines()
        )
        self.assertEqual(2, manifest["events"])
        self.assertEqual(
            json.loads(events[-1])["event_sha256"], manifest["head_sha256"]
        )
        path = self.repo / "memory" / "audit" / "events.jsonl"
        path.write_text(
            path.read_text(encoding="utf-8").replace("second", "changed"),
            encoding="utf-8",
        )
        self.assertEqual("invalid", verify_audit(self.repo / "memory")["status"])

    def test_each_way_an_audit_chain_breaks_is_named(self) -> None:
        root = self.repo / "memory"
        for operation in ("first", "second", "third"):
            append_audit(root, {"operation": operation})
        events_file = root / "audit" / "events.jsonl"
        manifest_file = root / "audit" / "manifest.json"
        events = [json.loads(line) for line in events_file.read_text(encoding="utf-8").splitlines()]
        manifest = manifest_file.read_text(encoding="utf-8")

        def resigned(event):
            unsigned = {key: value for key, value in event.items() if key != "event_sha256"}
            return {**unsigned, "event_sha256": audit_event_digest(unsigned)}

        relinked = resigned({**events[1], "previous_event_sha256": "sha256:" + "0" * 64})
        cases = [
            ("renumbered", [events[0], resigned({**events[1], "sequence": 5}), events[2]], manifest,
             "audit sequence is invalid at event 2"),
            ("relinked", [events[0], relinked, events[2]], manifest, "audit chain is broken at event 2"),
            ("edited", [events[0], {**events[1], "operation": "changed"}, events[2]], manifest,
             "audit digest is invalid at event 2"),
            ("truncated", events[:2], manifest, "audit manifest does not match the event log"),
            ("miscounted", events, json.dumps({**json.loads(manifest), "events": 4}),
             "audit manifest does not match the event log"),
            ("other head", events, json.dumps({**json.loads(manifest), "head_sha256": events[1]["event_sha256"]}),
             "audit manifest does not match the event log"),
            ("unreadable manifest", events, "{", f"invalid JSON: {manifest_file}"),
            ("wrong manifest format", events, json.dumps({**json.loads(manifest), "format": "domain-memory-audit/v2"}),
             "audit manifest does not match the event log"),
            ("unreadable event", None, manifest, "audit event 2 is invalid JSON"),
            ("event that is a number", "3", manifest, "audit event 2 must be an object"),
            ("event that is a list", "[]", manifest, "audit event 2 must be an object"),
        ]
        for label, chain, manifest_text, reason in cases:
            with self.subTest(label):
                if isinstance(chain, str):
                    lines = [json.dumps(events[0]), chain, json.dumps(events[2])]
                elif chain is None:
                    lines = [json.dumps(events[0]), "{", json.dumps(events[2])]
                else:
                    lines = [json.dumps(event) for event in chain]
                events_file.write_text("\n".join(lines) + "\n", encoding="utf-8")
                manifest_file.write_text(manifest_text, encoding="utf-8")
                self.assertEqual({"status": "invalid", "reason": reason}, verify_audit(root))
        events_file.write_text("\n".join(json.dumps(event) for event in events) + "\n", encoding="utf-8")
        manifest_file.write_text(manifest, encoding="utf-8")
        self.assertEqual(("valid", 3), (verify_audit(root)["status"], verify_audit(root)["events"]))

    def test_an_upserted_citation_without_hashes_gets_the_hashes_cite_gives(self) -> None:
        self.two_line_rule()
        record = self.write_record("context.json", {
            "id": "orders", "name": "Orders", "responsibility": "Own orders.",
            "evidence": [{"path": "rule.md", "lines": {"start": 2, "end": 2}, "source_kind": "decisions"}],
        })
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        stored = asset_records(self.repo / "memory", "contexts.json")[0]["evidence"][0]
        self.assertEqual(
            {"path": "rule.md", "lines": {"start": 2, "end": 2},
             "excerpt_sha256": citation(self.repo, "rule.md", 2, 2)["excerpt_sha256"]},
            stored,
        )
        self.assertEqual("current", verify(stored, self.repo)["status"])

    def test_a_citation_nested_in_a_list_is_hashed_too(self) -> None:
        self.two_line_rule()
        record = self.write_record("context.json", {
            "id": "orders", "name": "Orders", "responsibility": "Own orders.",
            "confirmed_absences": [{"asset": "contracts", "reason": "Orders publishes nothing.",
                                    "evidence": [{"path": "rule.md", "lines": {"start": 1, "end": 1}}]}],
        })
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        stored = asset_records(self.repo / "memory", "contexts.json")[0]["confirmed_absences"][0]["evidence"][0]
        self.assertEqual(citation(self.repo, "rule.md", 1, 1)["excerpt_sha256"], stored["excerpt_sha256"])

    def test_an_upserted_citation_keeps_the_hash_it_was_given(self) -> None:
        self.two_line_rule()
        given = {"path": "rule.md", "lines": {"start": 1, "end": 1}, "excerpt_sha256": "sha256:recorded-earlier"}
        record = self.write_record("context.json", {
            "id": "orders", "name": "Orders", "responsibility": "Own orders.", "evidence": [given],
        })
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        self.assertEqual("sha256:recorded-earlier",
                         asset_records(self.repo / "memory", "contexts.json")[0]["evidence"][0]["excerpt_sha256"])

    def test_an_upserted_citation_of_a_missing_file_is_refused(self) -> None:
        record = self.write_record("context.json", {
            "id": "orders", "name": "Orders", "responsibility": "Own orders.",
            "evidence": [{"path": "gone.md", "lines": {"start": 1, "end": 1}}],
        })
        with self.assertRaisesRegex(ValueError, "no such file to cite: gone.md"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def test_audit_rejects_each_reserved_field_before_writing(self) -> None:
        root = self.repo / "memory"
        append_audit(root, {"operation": "first"})
        for field in ("sequence", "recorded_at", "previous_event_sha256", "event_sha256"):
            with self.subTest(field), self.assertRaisesRegex(ValueError, "^audit event contains reserved fields$"):
                append_audit(root, {"operation": "forged", field: "forged"})
        self.assertEqual(
            {
                "status": "valid",
                "events": 1,
                "head_sha256": json.loads(
                    (root / "audit" / "events.jsonl").read_text(encoding="utf-8")
                )["event_sha256"],
            },
            verify_audit(root),
        )

    def test_recovery_rolls_back_an_installed_transaction_without_its_audit_event(
        self,
    ) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-new"
        staging = root / ".domain-registry-stage-new"
        original = document_path(root, MANIFEST).read_bytes()
        shutil.copytree(root / "registry", backup)
        staging.mkdir()
        document_path(root, MANIFEST).write_text("{}\n", encoding="utf-8")
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
        self.assertEqual(original, document_path(root, MANIFEST).read_bytes())

    def test_recovery_rolls_back_an_installed_transaction_when_the_last_audit_event_is_another_operations(
        self,
    ) -> None:
        original = document_path(self.repo / "memory", MANIFEST).read_bytes()
        root, backup, _ = self.interrupted_transaction("installed")
        append_audit(root, {"operation": "update", "operation_id": "someone-else"})
        recover_interrupted_update(root, False)
        self.assertEqual(
            (original, False, False),
            (
                document_path(root, MANIFEST).read_bytes(),
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
        document_path(root, MANIFEST).write_text("{}\n", encoding="utf-8")
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
            "{}\n", document_path(root, MANIFEST).read_text(encoding="utf-8")
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

        with patch.object(Path, "replace", autospec=True, side_effect=refuse_installing), \
                patch("domain_registry.common.time.sleep"), self.assertRaises(PermissionError):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        self.assertEqual(
            (before, False, []),
            (read_document(self.repo / "memory", "contexts.json"),
             transaction_path(self.repo / "memory").exists(),
             sorted(path.name for path in (self.repo / "memory").glob(".domain-registry-*"))),
        )

    def test_recovery_restores_the_backup_when_a_prepared_swap_left_no_registry(self) -> None:
        original = document_path(self.repo / "memory", MANIFEST).read_bytes()
        root, backup, _ = self.interrupted_transaction("prepared")
        remove_tree(root / "registry")
        recover_interrupted_update(root, False)
        self.assertEqual(
            (original, False, False),
            (document_path(root, MANIFEST).read_bytes(), backup.exists(), transaction_path(root).exists()),
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
                document_path(root, MANIFEST).read_text(encoding="utf-8"),
                backup.exists(), staging.exists(), transaction_path(root).exists(),
            ),
        )

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
        original = document_path(self.repo / "memory", MANIFEST).read_bytes()
        root, backup, _ = self.interrupted_transaction("installed")
        remove_tree(root / "registry")
        recover_interrupted_update(root, False)
        self.assertEqual(
            (original, False, False),
            (
                document_path(root, MANIFEST).read_bytes(),
                backup.exists(), transaction_path(root).exists(),
            ),
        )

    def test_a_reviewed_record_is_not_replaced_without_superseding_its_approval(
        self,
    ) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(package, "PRO-2", "An order total may be negative.", None)
        with self.assertRaisesRegex(ValueError, "must list PRO-1 in supersedes"):
            self.approve_and_apply(package)
        self.assertEqual(
            self.stored_rule()["statement"], "An order total is non-negative."
        )

    def test_superseding_a_different_proposal_does_not_replace_a_reviewed_record(
        self,
    ) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(
            package, "PRO-2", "An order total may be negative.", "PRO-UNRELATED"
        )
        with self.assertRaisesRegex(ValueError, "must list PRO-1 in supersedes"):
            self.approve_and_apply(package)
        self.assertEqual(
            self.stored_rule()["statement"], "An order total is non-negative."
        )

    def test_superseding_the_reviewing_proposal_replaces_a_reviewed_record(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(
            package, "PRO-2", "An order total is never below zero.", "PRO-1"
        )
        self.approve_and_apply(package)
        replaced = self.stored_rule()
        self.assertEqual(replaced["statement"], "An order total is never below zero.")
        self.assertEqual(replaced["status"], "reviewed")
        self.assertEqual(
            ("PRO-2", ["human:reviewer"]),
            (reviewing_proposals(self.repo / "memory")["rules/order-total"],
             [entry["by"] for entry in replaced["verified"]]),
        )

    def test_a_reviewed_record_may_not_cite_outside_the_selected_sources(self) -> None:
        self.select_docs_as_sources()
        package = self.draft_package()
        self.cite_in_proposal(package, "README.md")
        with self.assertRaisesRegex(ValueError, "outside the confirmed sources"):
            self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), [])

    def test_a_reviewed_record_citing_a_selected_source_is_applied(self) -> None:
        self.select_docs_as_sources()
        package = self.draft_package()
        self.cite_in_proposal(package, "docs/decision.md")
        self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_applying_a_record_drops_the_source_kind_it_claims_and_verification_derives_it(
        self,
    ) -> None:
        self.select_docs_as_sources()
        package = self.draft_package()
        self.cite_in_proposal(package, "docs/decision.md")
        proposal_path = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["record"]["evidence"][0]["source_kind"] = (
            "implementation"
        )
        proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
        self.approve_and_apply(package)
        [result] = verify_evidence(self.repo / "memory", self.repo)["results"]
        self.assertEqual(
            (False, "decisions"), ("source_kind" in self.stored_rule()["evidence"][0], result["source_kind"])
        )

    def test_what_the_plugin_writes_ends_lines_with_lf_on_every_platform(self) -> None:
        package = self.draft_package()
        self.approve_and_apply(package)
        written = [
            document_path(self.repo / "memory", MANIFEST),
            self.repo / "memory" / "audit" / "events.jsonl",
            *sorted((self.repo / "memory" / "registry" / "rules").glob("*.md")),
            package / "domain-change-proposal.json",
        ]
        for path in written:
            with self.subTest(path=path.name):
                content = path.read_bytes()
                self.assertIn(b"\n", content)
                self.assertNotIn(b"\r\n", content)

    def test_a_promoted_candidate_is_applied_as_the_reviewed_record(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        [update] = self.registry_updates(package)
        self.assertEqual(("upsert", "rules"), (update["operation"], update["asset"]))
        self.assertNotIn("status", update["record"])
        self.approve_and_apply(package)
        rules = read_document(self.repo / "memory", "rules.json")
        [rule] = rules["rules"]
        self.assertEqual(("order-total", "reviewed", "An order total is non-negative."),
                         (rule["id"], rule["status"], rule["statement"]))

    def test_promoting_adds_the_contexts_the_record_names_to_the_proposal(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        proposal_file = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_file.read_text(encoding="utf-8"))
        proposal["affected_contexts"] = ["billing"]
        proposal_file.write_text(json.dumps(proposal), encoding="utf-8")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.assertEqual(["billing", "orders"], json.loads(proposal_file.read_text(encoding="utf-8"))["affected_contexts"])

    def test_promoting_again_replaces_the_entry_with_the_current_candidate(self) -> None:
        package = self.package_with_a_candidate_rule("First wording.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        record = self.write_record(
            "rule.json",
            {"id": "order-total", "contexts": ["orders"], "statement": "Second wording.", "evidence": ["README.md:1"]},
        )
        upsert_candidate(self.repo / "memory", self.repo, "rules", record)
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.assertEqual(["Second wording."], [entry["record"]["statement"] for entry in self.registry_updates(package)])

    def test_promoting_two_different_candidates_keeps_both_updates(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        second = self.write_record(
            "second-rule.json",
            {"id": "order-tax", "contexts": ["orders"], "statement": "Tax is added.", "evidence": ["README.md:1"]},
        )
        upsert_candidate(self.repo / "memory", self.repo, "rules", second)
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        promote_candidate(package, self.repo / "memory", "rules", "order-tax")
        self.assertEqual(
            ["order-total", "order-tax"], [entry["record"]["id"] for entry in self.registry_updates(package)]
        )

    def test_promoting_a_candidate_that_carries_a_review_key_strips_it(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        self.edit_registry("rules.json", lambda value: value["rules"][0].update(review={"proposal_id": "PRO-0"}))
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        [update] = self.registry_updates(package)
        self.assertNotIn("review", update["record"])

    def test_promoting_a_record_the_registry_does_not_hold_is_refused(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        with self.assertRaisesRegex(ValueError, "no rules/missing in the Registry"):
            promote_candidate(package, self.repo / "memory", "rules", "missing")

    def test_promoting_an_already_reviewed_record_is_refused(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.approve_and_apply(package)
        again = self.redraft_target()
        with self.assertRaisesRegex(ValueError, "already reviewed"):
            promote_candidate(again, self.repo / "memory", "rules", "order-total")

    def test_promoting_into_a_proposal_that_is_no_longer_a_draft_is_refused(self) -> None:
        package = self.package_with_a_candidate_rule("An order total is non-negative.")
        promote_candidate(package, self.repo / "memory", "rules", "order-total")
        self.approve_and_apply(package)
        with self.assertRaisesRegex(
            ValueError, "added while the proposal is a draft.*run supersede-proposal, then redraft-proposal"
        ):
            promote_candidate(package, self.repo / "memory", "rules", "order-total")

    def test_removing_a_reviewed_record_requires_superseding_its_approval(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "order-total", None)
        with self.assertRaisesRegex(ValueError, "must list PRO-1 in supersedes"):
            self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_superseding_the_reviewing_proposal_removes_a_reviewed_record(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "order-total", "PRO-1")
        self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), [])

    def test_removing_a_record_the_registry_does_not_hold_is_refused(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "no-such-rule", "PRO-1")
        with self.assertRaisesRegex(ValueError, "cannot remove absent record"):
            self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_an_update_naming_an_unknown_asset_is_refused_and_writes_nothing(self) -> None:
        package = self.drafted_with_updates(
            [{"operation": "upsert", "asset": "nonsense", "record": {"id": "x"}}]
        )
        with self.assertRaisesRegex(ValueError, "registry update names unknown asset 'nonsense'"):
            self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), [])

    def test_a_proposal_without_updates_is_refused(self) -> None:
        package = self.drafted_with_updates([])
        with self.assertRaisesRegex(ValueError, "approved proposal has no registry_updates"):
            self.approve_and_apply(package)

    def test_an_invalid_package_is_refused_before_anything_applies(self) -> None:
        package = self.draft_package()
        (package / "test-obligations.json").unlink()
        with self.assertRaisesRegex(ValueError, "change package is invalid"):
            apply_approved_updates(package, self.repo / "memory", self.repo)

    def test_applying_requires_an_scm_verified_policy(self) -> None:
        package = self.draft_package()
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        with self.assertRaisesRegex(ValueError, "reviewed updates require an scm-verified"):
            apply_approved_updates(package, self.repo / "memory", self.repo)

    def test_a_registry_update_that_is_neither_an_upsert_nor_a_remove_is_refused(
        self,
    ) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_removal(package, "PRO-2", "order-total", "PRO-1")
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["operation"] = "replace"
        path.write_text(json.dumps(proposal), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "must be an upsert or a remove"):
            self.approve_and_apply(package)
        self.assertEqual(self.rule_ids(), ["order-total"])

    def test_confirming_sources_is_written_into_the_audit_chain(self) -> None:
        self.committed_source_map()
        confirm_sources(self.repo / "memory", self.repo, "Arthur")
        recorded = json.loads(
            (self.repo / "memory" / "audit" / "events.jsonl")
            .read_text(encoding="utf-8")
            .splitlines()[-1]
        )
        self.assertEqual(recorded["operation"], "confirm-sources")
        self.assertEqual(recorded["confirmed_by"], "Arthur")
        self.assertEqual(recorded["selected_paths"], ["docs"])
        self.assertEqual("valid", verify_audit(self.repo / "memory")["status"])

    def test_a_file_holding_several_records_says_how_many_it_should_hold(self) -> None:
        record = self.write_record("many.json", [{"id": "orders"}, {"id": "billing"}])
        with self.assertRaisesRegex(ValueError, "One record per file; give --record-file once for each record"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def test_terms_that_name_each_other_are_upserted_together_in_one_audited_change(self) -> None:
        use_json_layout(self.repo / "memory")
        order, line = self.look_alike_terms()
        upsert_candidate(self.repo / "memory", self.repo, "vocabulary", order, line)
        self.assertEqual(
            [("order", ["order-line"]), ("order-line", ["order"])],
            [(term["id"], term["not_same_as"]) for term in asset_records(self.repo / "memory", "vocabulary.json")],
        )
        last = self.audit_lines()[-1]
        self.assertEqual(("upsert-candidate", ["order", "order-line"]), (last["operation"], last["record_ids"]))
        self.assertNotIn("record_id", last)

    def test_a_single_upsert_keeps_naming_its_record_id(self) -> None:
        self.add_context()
        last = self.audit_lines()[-1]
        self.assertEqual(("upsert-candidate", "orders"), (last["operation"], last["record_id"]))
        self.assertNotIn("record_ids", last)

    def test_one_term_naming_a_missing_look_alike_is_refused_alone(self) -> None:
        order, _ = self.look_alike_terms()
        with self.assertRaisesRegex(ValueError, "not_same_as must name other vocabulary term ids"):
            upsert_candidate(self.repo / "memory", self.repo, "vocabulary", order)

    def test_a_batch_with_one_invalid_record_writes_none_of_them(self) -> None:
        order, line = self.look_alike_terms()
        stray = self.write_record("stray.json", {"id": "stray", "name": "Stray", "contexts": ["ghost"]})
        events = len(self.audit_lines())
        with self.assertRaisesRegex(ValueError, "vocabulary/stray references unknown context"):
            upsert_candidate(self.repo / "memory", self.repo, "vocabulary", order, line, stray)
        self.assertEqual([], asset_records(self.repo / "memory", "vocabulary.json"))
        self.assertEqual(events, len(self.audit_lines()))

    def test_an_upsert_without_a_record_file_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "requires at least one record file"):
            upsert_candidate(self.repo / "memory", self.repo, "vocabulary")

    def test_a_batch_naming_one_record_twice_is_refused(self) -> None:
        order, _ = self.look_alike_terms()
        with self.assertRaisesRegex(ValueError, "names a record more than once: order"):
            upsert_candidate(self.repo / "memory", self.repo, "vocabulary", order, order)

    def test_a_change_package_offered_as_a_record_is_refused(self) -> None:
        record = self.write_record(
            "package.json", {"registry_updates": [{"id": "orders"}]}
        )
        with self.assertRaisesRegex(ValueError, "not a Change Package"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def test_a_selected_path_that_disappeared_makes_the_map_stale_and_is_named_missing(self) -> None:
        path = self.committed_source_map()
        remove_tree(self.repo / "docs")
        result = verify_source_map(self.repo, path)
        self.assertEqual(
            ("stale", [{"path": "docs", "status": "missing"}]),
            (result["status"], result["changed_sources"]),
        )

    def test_stored_evidence_records_the_kind_of_source_it_came_from(self) -> None:
        decisions = self.repo / "docs" / "adr"
        decisions.mkdir(parents=True, exist_ok=True)
        decision = decisions / "boundaries.md"
        decision.write_text("Orders owns pricing.", encoding="utf-8")
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [decisions]),
        )
        content = decision.read_bytes()
        record = self.write_record(
            "context.json",
            {
                "id": "orders",
                "name": "Orders",
                "responsibility": "Own orders.",
                "evidence": [
                    {
                        "path": "docs/adr/boundaries.md",
                        "lines": {"start": 1, "end": 1},
                        "content_sha256": digest(content),
                        "excerpt_sha256": digest(content),
                    }
                ],
            },
        )
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        stored = read_document(self.repo / "memory", "contexts.json")
        self.assertNotIn("source_kind", stored["contexts"][0]["evidence"][0])
        summary = verify_evidence(self.repo / "memory", self.repo)["summary"]
        self.assertEqual(summary["by_source_kind"], {"decisions": 1})

    def test_retracting_a_candidate_removes_it(self) -> None:
        self.two_contexts()
        retract_candidate(
            self.repo / "memory",
            self.repo,
            "contexts",
            "billing",
            "Examination found no Billing boundary in this repository.",
        )
        self.assertEqual([entry["id"] for entry in self.stored_contexts()], ["orders"])

    def test_retracting_a_candidate_records_its_reason(self) -> None:
        self.two_contexts()
        retract_candidate(
            self.repo / "memory", self.repo, "contexts", "billing", "Not a boundary."
        )
        recorded = json.loads(
            (self.repo / "memory" / "audit" / "events.jsonl")
            .read_text(encoding="utf-8")
            .splitlines()[-1]
        )
        self.assertEqual(recorded["operation"], "retract-candidate")
        self.assertEqual(recorded["record_id"], "billing")
        self.assertEqual(recorded["reason"], "Not a boundary.")

    def test_a_reviewed_record_cannot_be_retracted(self) -> None:
        use_json_layout(self.repo / "memory")
        self.two_contexts()
        value = read_document(self.repo / "memory", "contexts.json")
        value["contexts"][0]["status"] = "reviewed"
        write_document(self.repo / "memory", "contexts.json", value)
        with self.assertRaisesRegex(ValueError, "superseding proposal"):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "orders", "Changed my mind."
            )
        self.assertEqual(
            [entry["id"] for entry in self.stored_contexts()], ["orders", "billing"]
        )

    def test_a_reviewed_record_is_not_overwritten_by_an_upserted_candidate(self) -> None:
        use_json_layout(self.repo / "memory")
        self.two_contexts()
        self.edit_registry("contexts.json", lambda value: value["contexts"][0].update(status="reviewed"))
        registry_files = snapshot_of_files(self.repo / "memory" / "registry")
        record = self.write_record(
            "context.json", {"id": "orders", "name": "Renamed", "responsibility": "Own everything."}
        )
        with self.assertRaisesRegex(ValueError, "^cannot overwrite reviewed record: orders$"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        self.assertEqual(registry_files, snapshot_of_files(self.repo / "memory" / "registry"))

    def test_retracting_an_absent_record_changes_nothing(self) -> None:
        use_json_layout(self.repo / "memory")
        self.two_contexts()
        with self.assertRaisesRegex(ValueError, "no such record"):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "shipping", "Never existed."
            )
        self.assertEqual(
            [entry["id"] for entry in self.stored_contexts()], ["orders", "billing"]
        )

    def test_retracting_a_candidate_requires_a_reason(self) -> None:
        self.two_contexts()
        with self.assertRaisesRegex(ValueError, "requires the reason"):
            retract_candidate(self.repo / "memory", self.repo, "contexts", "billing", "   ")

    def test_a_referenced_candidate_cannot_be_retracted(self) -> None:
        use_json_layout(self.repo / "memory")
        self.two_contexts()
        self.seed(
            "interactions.json",
            [
                {
                    "id": "order-billed",
                    "producer_context": "orders",
                    "consumer_context": "billing",
                }
            ],
        )
        with self.assertRaisesRegex(ValueError, "references unknown context"):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "billing", "No longer wanted."
            )
        self.assertEqual(
            [entry["id"] for entry in self.stored_contexts()], ["orders", "billing"]
        )

    def demoted_contexts(self) -> dict:
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        self.two_contexts()
        value = read_document(self.repo / "memory", "contexts.json")
        value["status"] = "reviewed"
        for record in value["contexts"]:
            record["status"] = "reviewed"
            record["verified"] = VERIFIED
        write_document(self.repo / "memory", "contexts.json", value)
        demote_local_reviews(self.repo / "memory", self.repo)
        return read_document(self.repo / "memory", "contexts.json")

    def test_demoting_local_reviews_sets_a_reviewed_record_to_candidate(self) -> None:
        self.assertEqual("candidate", self.demoted_contexts()["contexts"][0]["status"])

    def test_demoting_local_reviews_sets_the_other_record_to_candidate_too(self) -> None:
        self.assertEqual("candidate", self.demoted_contexts()["contexts"][1]["status"])

    def test_demoting_local_reviews_drops_the_review_of_every_record(self) -> None:
        demoted = self.demoted_contexts()
        self.assertEqual([False, False], ["verified" in record for record in demoted["contexts"]])

    def test_demoting_local_reviews_sets_the_document_status_to_candidate(self) -> None:
        self.assertEqual("candidate", self.demoted_contexts()["status"])

    def test_demoting_local_reviews_appends_a_demote_event_to_the_audit_log(self) -> None:
        self.demoted_contexts()
        self.assertEqual("demote-local-reviews", read_audit_events(self.repo / "memory")[-1]["operation"])

    def test_demoting_is_refused_unless_the_policy_is_local_draft_only(self) -> None:
        with self.assertRaisesRegex(
            ValueError, "^review demotion is only for local-draft-only Domain Memory$"
        ):
            demote_local_reviews(self.repo / "memory", self.repo)
        self.assertEqual([], read_audit_events(self.repo / "memory"))

    def assert_registry_in_place(self, root: Path) -> None:
        self.assertTrue(is_concept_layout(root))
        self.assertTrue((root / "registry" / "contexts").is_dir())

    def test_recovery_restores_the_registry_a_crash_left_in_the_backup(self) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-abc"
        (root / "registry").replace(backup)
        transaction_path(root).write_text(
            json.dumps(
                {"backup": backup.name, "staging": ".domain-registry-stage-abc"}
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
                {"backup": backup.name, "staging": ".domain-registry-stage-abc"}
            ),
            encoding="utf-8",
        )
        recover_interrupted_update(root, False)
        self.assertFalse(backup.exists())
        self.assertFalse(transaction_path(root).exists())
        self.assert_registry_in_place(root)

    def test_recovery_discards_a_backup_once_the_registry_is_already_in_place(
        self,
    ) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-abc"
        shutil.copytree(root / "registry", backup)
        transaction_path(root).write_text(
            json.dumps(
                {"backup": backup.name, "staging": ".domain-registry-stage-abc"}
            ),
            encoding="utf-8",
        )
        recover_interrupted_update(root, False)
        self.assertFalse(backup.exists())
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
                    json.dumps({"backup": backup, "staging": staging}), encoding="utf-8"
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

    def test_amending_a_governance_field_records_what_changed_and_why(self) -> None:
        change = amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone until a reviewer exists.",
        )
        self.assertEqual(self.stored_policy()["review_mode"], "local-draft-only")
        self.assertEqual(
            (change["from"], change["to"]), ("scm-verified", "local-draft-only")
        )
        recorded = json.loads(
            (self.repo / "memory" / "audit" / "events.jsonl")
            .read_text(encoding="utf-8")
            .splitlines()[-1]
        )
        self.assertEqual(recorded["operation"], "amend-policy")
        self.assertEqual(recorded["field"], "review_mode")
        self.assertEqual(recorded["from"], "scm-verified")
        self.assertIn("reviewer", recorded["reason"])
        self.assertEqual("valid", verify_audit(self.repo / "memory")["status"])

    def test_one_amendment_moves_review_and_its_authority_together(self) -> None:
        root = self.repo / "memory"
        amend_policy(root, "review_mode", "local-draft-only", "Drafting alone.")
        recorded_before = len(self.audit_lines())
        changes = amend_policy_fields(
            root,
            [
                ("review_verifier", "github-pr"),
                ("review_trigger", "external-scm"),
                ("ci_requirement", "required"),
                ("review_mode", "scm-verified"),
            ],
            "A reviewer and CI exist now.",
        )
        policy = self.stored_policy()
        self.assertEqual("scm-verified", policy["review_mode"])
        self.assertEqual(
            ("github-pr", "external-scm", "required"),
            tuple(policy["review_governance"][key] for key in ("verifier", "trigger", "ci_requirement")),
        )
        self.assertEqual(
            [
                ("review_mode", "local-draft-only", "scm-verified"),
                ("review_verifier", "none", "github-pr"),
                ("review_trigger", "none", "external-scm"),
                ("ci_requirement", "none", "required"),
            ],
            [(change["field"], change["from"], change["to"]) for change in changes],
        )
        appended = self.audit_lines()[recorded_before:]
        self.assertEqual([change["field"] for change in changes], [event["field"] for event in appended])
        self.assertEqual("valid", verify_audit(root)["status"])

    def test_the_registry_revision_moves_when_the_policy_moves(self) -> None:
        before = registry_digest(self.repo / "memory")
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone for now.",
        )
        self.assertNotEqual(before, registry_digest(self.repo / "memory"))

    def test_a_registry_without_a_policy_still_has_a_revision(self) -> None:
        policy = self.repo / "memory" / "domain-memory-policy.json"
        with_policy = registry_digest(self.repo / "memory")
        policy.unlink()
        without_policy = registry_digest(self.repo / "memory")
        self.assertTrue(valid_digest(without_policy))
        self.assertNotEqual(with_policy, without_policy)

    def test_a_captured_revision_goes_stale_after_a_hand_edited_policy(self) -> None:
        captured = {
            "base_registry_revision": current_registry_revision(
                self.repo / "memory", self.repo
            )
        }
        require_current_registry_revision(captured, self.repo / "memory", self.repo)
        path = self.repo / "memory" / "domain-memory-policy.json"
        value = json.loads(path.read_text(encoding="utf-8"))
        value["source_policy"]["authority"] = "somebody else entirely"
        path.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "stale"):
            require_current_registry_revision(captured, self.repo / "memory", self.repo)

    def test_a_candidate_resting_on_an_unconfirmed_file_names_that_file(self) -> None:
        self.confirmed_corpus()
        (self.repo / "notes.md").write_text("Orders owns pricing.", encoding="utf-8")
        record = self.write_record(
            "context.json",
            {
                "id": "orders",
                "name": "Orders",
                "responsibility": "Own orders.",
                "evidence": [citation(self.repo, "notes.md", 1, 1)],
            },
        )
        self.assertEqual(
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record).outside,
            ["notes.md"],
        )

    def test_a_candidate_inside_the_confirmed_corpus_raises_no_reach_warning(
        self,
    ) -> None:
        self.confirmed_corpus()
        record = self.write_record(
            "context.json",
            {
                "id": "orders",
                "name": "Orders",
                "responsibility": "Own orders.",
                "evidence": [citation(self.repo, "docs/adr/boundaries.md", 2, 2)],
            },
        )
        self.assertEqual(
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record).outside, []
        )

    def test_superseding_a_list_that_names_the_reviewing_proposal_replaces_a_reviewed_record(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        self.approve_and_apply(package)
        self.redraft_package(package, "PRO-2", "An order total is never below zero.", ["PRO-0", "PRO-1"])
        self.approve_and_apply(package)
        self.assertEqual("An order total is never below zero.", self.stored_rule()["statement"])

    def test_an_update_keeps_a_registry_error_it_found_but_refuses_a_new_one(self) -> None:
        self.seed("rules.json", [{"id": "old", "contexts": ["ghost"], "statement": "Kept."}])
        self.add_context()
        self.assertEqual(["orders"], [entry["id"] for entry in asset_records(self.repo / "memory", "contexts.json")])
        record = self.write_record("rule.json", {"id": "new", "contexts": ["phantom"], "statement": "Refused."})
        with self.assertRaisesRegex(ValueError, "rules/new references unknown context"):
            upsert_candidate(self.repo / "memory", self.repo, "rules", record)

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
