from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.audit import event_digest as audit_event_digest
from domain_registry.audit import anchor_error
from domain_registry.audit import verify as verify_audit
from domain_registry.revision import (
    base_registry_revision,
    current_registry_revision,
    registry_digest,
    require_current_registry_revision,
    valid_commit,
    valid_digest,
    valid_registry_revision,
)
from domain_registry.registry import init_registry
from domain_registry.updates import upsert_candidate
from domain_registry.store import read_document
from test_support import RegistryTestCase, append_audit, applied_by

DRAFTED_RULE = {"id": "order-total", "contexts": ["orders"], "statement": "A total."}


class UpdatesAuditTest(RegistryTestCase):
    def test_a_stale_base_holds_only_while_what_the_proposal_touches_is_unchanged(self) -> None:
        def billing_renamed(value):
            next(context for context in value["contexts"] if context["id"] == "billing")["name"] = "Invoicing"

        def orders_renamed(value):
            next(context for context in value["contexts"] if context["id"] == "orders")["name"] = "Purchases"

        def rule_written(value):
            value["rules"].append({"id": "order-total", "contexts": ["billing"]})

        def rule_written_as_its_draft(value):
            value["rules"].append({"id": "order-total", "contexts": ["orders"], "status": "candidate"})

        cases = [
            ("an untouched record changed", "contexts.json", billing_renamed, None),
            ("a record it references changed", "contexts.json", orders_renamed, "contexts/orders"),
            ("the record it writes changed", "rules.json", rule_written,
             "rules/order-total, which this proposal writes, changed since it was reviewed"),
            ("the record it writes is present as its own draft", "rules.json", rule_written_as_its_draft, None),
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

    def test_a_removed_record_that_reappears_as_the_draft_is_stale(self) -> None:
        self.two_contexts()
        self.seed("rules.json", [])
        self.commit_everything("base")
        draft = {"id": "order-total", "contexts": ["orders"]}
        proposal = {
            "base_registry_revision": current_registry_revision(self.repo / "memory", self.repo),
            "registry_updates": [{"operation": "remove", "asset": "rules", "id": "order-total", "record": draft}],
        }
        self.edit_registry("rules.json", lambda value: value["rules"].append({**draft, "status": "candidate"}))
        with self.assertRaisesRegex(ValueError, "stale.*rules/order-total, which this proposal touches, changed$"):
            require_current_registry_revision(proposal, self.repo / "memory", self.repo)

    def test_a_decision_draft_matches_its_record_apart_from_its_review_status(self) -> None:
        decision = {"id": "one-store", "statement": "Use one store.", "status": "accepted"}
        for label, drafted, refusal in (
            ("the draft is the record", decision, None),
            ("the draft decided otherwise", {**decision, "status": "rejected"},
             "decisions/one-store, which this proposal writes, changed since it was reviewed"),
        ):
            with self.subTest(label):
                self.seed("decisions.json", [])
                self.commit_everything("base")
                proposal = {
                    "base_registry_revision": current_registry_revision(self.repo / "memory", self.repo),
                    "registry_updates": [{"operation": "upsert", "asset": "decisions", "record": decision}],
                }
                self.edit_registry(
                    "decisions.json", lambda value, drafted=drafted: value["decisions"].append({**drafted, "review_status": "candidate"})
                )
                if refusal is None:
                    self.assertEqual(
                        registry_digest(self.repo / "memory"),
                        require_current_registry_revision(proposal, self.repo / "memory", self.repo),
                    )
                else:
                    with self.assertRaisesRegex(ValueError, f"stale.*{refusal}$"):
                        require_current_registry_revision(proposal, self.repo / "memory", self.repo)

    def test_the_base_stamps_the_tree_and_the_registry_its_commit_holds(self) -> None:
        self.commit_everything("base")
        committed = registry_digest(self.repo / "memory")
        head = self.git("rev-parse", "HEAD")
        self.add_context()
        tree = registry_digest(self.repo / "memory")
        self.assertNotEqual(committed, tree)
        self.assertEqual(
            {"observed_commit": head, "registry_digest": tree, "committed_digest": committed},
            base_registry_revision(self.repo / "memory", self.repo),
        )

    def test_without_a_commit_or_a_committed_registry_the_base_stamps_the_tree_alone(self) -> None:
        other = self.repo / "other"
        init_registry(other)
        self.assertEqual(
            {"observed_commit": self.git("rev-parse", "HEAD"), "registry_digest": registry_digest(other)},
            base_registry_revision(other, self.repo),
            "a commit that holds no Registry",
        )
        fresh = self.repo / "fresh"
        fresh.mkdir()
        subprocess.run(["git", "init", "-q"], cwd=fresh, check=True)
        init_registry(fresh / "memory")
        self.assertEqual(
            {"observed_commit": None, "registry_digest": registry_digest(fresh / "memory")},
            base_registry_revision(fresh / "memory", fresh),
            "a repository with no commit",
        )

    def test_a_base_submitted_from_uncommitted_drafts_is_rebuilt_from_its_commit(self) -> None:
        def rename_billing(value):
            next(context for context in value["contexts"] if context["id"] == "billing")["name"] = "Invoicing"

        self.two_contexts()
        self.commit_everything("base")
        self.add_record_draft("order-total")
        proposal = {
            "base_registry_revision": base_registry_revision(self.repo / "memory", self.repo),
            "registry_updates": [{"operation": "upsert", "asset": "rules", "record": DRAFTED_RULE}],
        }
        self.edit_registry("contexts.json", rename_billing)
        self.assertEqual(
            registry_digest(self.repo / "memory"),
            require_current_registry_revision(proposal, self.repo / "memory", self.repo),
        )
        proposal["base_registry_revision"]["committed_digest"] = "sha256:" + "0" * 64
        commit = proposal["base_registry_revision"]["observed_commit"]
        with self.assertRaises(ValueError) as refused:
            require_current_registry_revision(proposal, self.repo / "memory", self.repo)
        self.assertEqual(
            "proposal base revision is stale; rebase-proposal, then obtain fresh approval: the Registry committed at "
            f"{commit[:12]} does not reproduce the recorded base digest, so the records this proposal touches cannot "
            "be compared; rebase-proposal restamps the proposal on the current Registry",
            str(refused.exception),
        )

    def test_a_base_stamped_on_a_commit_before_the_registry_was_committed_names_that_commit(self) -> None:
        self.git("rm", "-r", "-q", "--cached", "memory")
        self.git("commit", "-qm", "untracks the registry")
        before = self.git("rev-parse", "HEAD")
        self.git("add", "memory")
        self.git("commit", "-qm", "tracks the registry again")
        proposal = {
            "base_registry_revision": {"observed_commit": before, "registry_digest": "sha256:" + "1" * 64},
            "registry_updates": [{"operation": "upsert", "asset": "rules", "record": DRAFTED_RULE}],
        }
        with self.assertRaises(ValueError) as refused:
            require_current_registry_revision(proposal, self.repo / "memory", self.repo)
        self.assertEqual(
            f"proposal base revision is stale; rebase-proposal, then obtain fresh approval: commit {before[:12]} does "
            "not hold the Registry at memory/registry, so the records this proposal touches cannot be compared; "
            "rebase-proposal restamps the proposal on the current Registry",
            str(refused.exception),
        )

    def add_record_draft(self, record_id: str) -> None:
        record = self.write_record("rule.json", {**DRAFTED_RULE, "id": record_id})
        upsert_candidate(self.repo / "memory", self.repo, "rules", record)

    def test_a_stale_base_that_was_never_committed_cannot_be_compared(self) -> None:
        self.two_contexts()
        proposal = {
            "base_registry_revision": current_registry_revision(self.repo / "memory", self.repo),
            "registry_updates": [{"operation": "upsert", "asset": "rules", "record": {"id": "order-total"}}],
        }
        self.edit_registry("contexts.json", lambda value: value["contexts"][1].update(name="Invoicing"))
        with self.assertRaisesRegex(ValueError, "stale.*does not reproduce the recorded base digest"):
            require_current_registry_revision(proposal, self.repo / "memory", self.repo)

    def test_a_submitted_record_cannot_carry_its_own_approval(self) -> None:
        record = self.write_record(
            "context.json",
            {
                "id": "orders",
                "name": "Orders",
                "responsibility": "Own orders.",
                "status": "reviewed",
                "verified": [{"by": "human:nobody", "at": "2026-01-01T00:00:00Z"}],
            },
        )
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        stored = read_document(self.repo / "memory", "contexts.json")
        written = next(
            entry for entry in stored["contexts"] if entry["id"] == "orders"
        )
        self.assertEqual(
            ("candidate", False), (written["status"], "verified" in written)
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
        self.assertIn("no Domain Registry at ", result["reason"])

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
             unresolved.stderr.strip().split(" from ")[0] + " from"),
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
        manifest.update(events=2, head_sha256=json.loads(kept[-1])["event_sha256"], heads=[json.loads(kept[-1])["event_sha256"]])
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


if __name__ == "__main__":
    unittest.main()
