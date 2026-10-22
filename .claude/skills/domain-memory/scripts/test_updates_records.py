from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.audit import read_events as read_audit_events
from domain_registry.audit import verify as verify_audit
from domain_registry.evidence import citation, digest, verify
from domain_registry.registry import asset_records, verify_evidence
from domain_registry.sources import (
    confirm_sources,
    selected_source_map,
    verify_source_map,
    write_source_map,
)
from domain_registry.transaction import remove_tree
from domain_registry.updates import demote_local_reviews, retract_candidate, upsert_candidate
from domain_registry.store import read_document, write_document
from test_support import (
    VERIFIED,
    RegistryTestCase,
    append_audit,
    amend_policy,
    snapshot_of_files,
)


class UpdatesRecordsTest(RegistryTestCase):
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
        head = json.loads((root / "audit" / "events.jsonl").read_text(encoding="utf-8"))["event_sha256"]
        self.assertEqual(
            {"status": "valid", "events": 1, "height": 1, "head_sha256": head, "heads": [head]},
            verify_audit(root),
        )

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
        self.two_contexts()
        value = read_document(self.repo / "memory", "contexts.json")
        next(context for context in value["contexts"] if context["id"] == "orders")["status"] = "reviewed"
        write_document(self.repo / "memory", "contexts.json", value)
        with self.assertRaisesRegex(ValueError, "superseding proposal"):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "orders", "Changed my mind."
            )
        self.assertEqual(
            [entry["id"] for entry in self.stored_contexts()], ["billing", "orders"]
        )

    def test_a_reviewed_record_is_not_overwritten_by_an_upserted_candidate(self) -> None:
        self.two_contexts()
        self.edit_registry("contexts.json", lambda value: next(c for c in value["contexts"] if c["id"] == "orders").update(status="reviewed"))
        registry_files = snapshot_of_files(self.repo / "memory" / "registry")
        record = self.write_record(
            "context.json", {"id": "orders", "name": "Renamed", "responsibility": "Own everything."}
        )
        with self.assertRaisesRegex(ValueError, "^cannot overwrite reviewed record: orders$"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        self.assertEqual(registry_files, snapshot_of_files(self.repo / "memory" / "registry"))

    def test_retracting_an_absent_record_changes_nothing(self) -> None:
        self.two_contexts()
        with self.assertRaisesRegex(ValueError, "no such record"):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "shipping", "Never existed."
            )
        self.assertEqual(
            [entry["id"] for entry in self.stored_contexts()], ["billing", "orders"]
        )

    def test_retracting_a_candidate_requires_a_reason(self) -> None:
        self.two_contexts()
        with self.assertRaisesRegex(ValueError, "requires the reason"):
            retract_candidate(self.repo / "memory", self.repo, "contexts", "billing", "   ")

    def test_a_referenced_candidate_cannot_be_retracted(self) -> None:
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
            [entry["id"] for entry in self.stored_contexts()], ["billing", "orders"]
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

    def test_demoting_local_reviews_sets_every_reviewed_record_to_candidate(self) -> None:
        self.assertEqual(["candidate", "candidate"], [record["status"] for record in self.demoted_contexts()["contexts"]])

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

    def test_an_update_keeps_a_registry_error_it_found_but_refuses_a_new_one(self) -> None:
        self.seed("rules.json", [{"id": "old", "contexts": ["ghost"], "statement": "Kept."}])
        self.add_context()
        self.assertEqual(["orders"], [entry["id"] for entry in asset_records(self.repo / "memory", "contexts.json")])
        record = self.write_record("rule.json", {"id": "new", "contexts": ["phantom"], "statement": "Refused."})
        with self.assertRaisesRegex(ValueError, "rules/new references unknown context"):
            upsert_candidate(self.repo / "memory", self.repo, "rules", record)


if __name__ == "__main__":
    unittest.main()
