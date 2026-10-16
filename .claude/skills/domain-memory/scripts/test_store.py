from __future__ import annotations

import json
import subprocess
import unittest

from domain_registry.audit import read_events
from domain_registry.common import ASSET_KEYS, registry_dir, write_json
from domain_registry.registry import migrate_registry
from domain_registry.revision import registry_digest
from domain_registry.store import (
    MANIFEST,
    committed_documents,
    has_document,
    has_registry,
    is_concept_layout,
    logical_documents,
    read_document,
    write_document,
)
from test_support import RegistryTestCase, snapshot_of_files, use_json_layout

ORDER_RULE = {"id": "order-total", "statement": "An order total is never negative. It is checked on save.",
              "contexts": ["orders"], "status": "reviewed"}
REFUND_RULE = {"id": "refund-limit", "statement": "A refund never exceeds the paid amount.", "status": "candidate"}


class ConceptLayoutTest(RegistryTestCase):
    def memory(self):
        return self.repo / "memory"

    def test_init_lays_out_a_root_index_and_one_index_per_asset(self) -> None:
        registry = registry_dir(self.memory())
        self.assertTrue(is_concept_layout(self.memory()))
        root_index = (registry / "index.md").read_text(encoding="utf-8")
        self.assertTrue(root_index.startswith('---\nokf_version: "0.2"\n---\n# Domain Registry\n'))
        self.assertIn("- [Business Rules](/rules/index.md)\n", root_index)
        for name in ASSET_KEYS:
            with self.subTest(name):
                self.assertTrue((registry / name.removesuffix(".json") / "index.md").is_file())
                self.assertEqual([], read_document(self.memory(), name)[ASSET_KEYS[name]])

    def test_each_record_is_one_concept_file_and_the_asset_reads_back_in_id_order(self) -> None:
        write_document(self.memory(), "rules.json", {"rules": [REFUND_RULE, ORDER_RULE]})
        rules = registry_dir(self.memory()) / "rules"
        self.assertEqual(["index.md", "order-total.md", "refund-limit.md"], sorted(p.name for p in rules.iterdir()))
        self.assertEqual(
            {"format": "domain-rules/v1", "status": "candidate", "rules": [ORDER_RULE, REFUND_RULE]},
            read_document(self.memory(), "rules.json"),
        )

    def test_a_record_left_out_of_a_write_loses_its_concept_file(self) -> None:
        write_document(self.memory(), "rules.json", {"rules": [REFUND_RULE, ORDER_RULE]})
        write_document(self.memory(), "rules.json", {"rules": [ORDER_RULE]})
        self.assertFalse((registry_dir(self.memory()) / "rules" / "refund-limit.md").exists())
        self.assertEqual([ORDER_RULE], read_document(self.memory(), "rules.json")["rules"])

    def test_the_asset_index_lists_each_record_with_its_first_sentence(self) -> None:
        context = {"id": "orders", "name": "Orders", "responsibility": "Own orders. Nothing else."}
        write_document(self.memory(), "contexts.json", {"contexts": [context]})
        write_document(self.memory(), "rules.json", {"rules": [REFUND_RULE, {"id": "bare"}]})
        registry = registry_dir(self.memory())
        self.assertEqual(
            "# Bounded Contexts\n\n- [Orders](/contexts/orders) — Own orders.\n",
            (registry / "contexts" / "index.md").read_text(encoding="utf-8"),
        )
        self.assertEqual(
            "# Business Rules\n\n- [bare](/rules/bare)\n"
            "- [refund-limit](/rules/refund-limit) — A refund never exceeds the paid amount.\n",
            (registry / "rules" / "index.md").read_text(encoding="utf-8"),
        )

    def test_a_long_first_sentence_is_shortened_in_the_index(self) -> None:
        write_document(self.memory(), "rules.json", {"rules": [{"id": "long", "statement": "x" * 200}]})
        line = (registry_dir(self.memory()) / "rules" / "index.md").read_text(encoding="utf-8").splitlines()[-1]
        self.assertEqual("- [long](/rules/long) — " + "x" * 159 + "….", line)

    def test_a_record_id_that_cannot_name_a_file_is_refused(self) -> None:
        for record_id in ("../escape", "a/b", "index", "", " lead", 7):
            with self.subTest(record_id=record_id), self.assertRaisesRegex(ValueError, "cannot name a concept file"):
                write_document(self.memory(), "rules.json", {"rules": [{"id": record_id}]})

    def test_two_records_with_one_id_are_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "rules holds two records with id r; a concept file holds one"):
            write_document(self.memory(), "rules.json", {"rules": [{"id": "r"}, {"id": "r", "statement": "S."}]})

    def test_an_asset_level_status_moves_into_records_that_have_none(self) -> None:
        write_document(self.memory(), "rules.json", {"status": "reviewed", "rules": [{"id": "a"}, {"id": "b", "status": "candidate"}]})
        self.assertEqual(
            [{"id": "a", "status": "reviewed"}, {"id": "b", "status": "candidate"}],
            read_document(self.memory(), "rules.json")["rules"],
        )

    def test_an_unreadable_concept_file_is_named(self) -> None:
        (registry_dir(self.memory()) / "rules" / "broken.md").write_text("no frontmatter\n", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, r"rules/broken\.md: a concept document starts with a --- frontmatter line"):
            read_document(self.memory(), "rules.json")

    def test_the_manifest_is_implied_by_the_layout_and_writing_it_changes_nothing(self) -> None:
        before = snapshot_of_files(registry_dir(self.memory()))
        write_document(self.memory(), MANIFEST, {"format": "other"})
        self.assertEqual(before, snapshot_of_files(registry_dir(self.memory())))
        self.assertEqual(
            {"format": "domain-registry/v1", "status": "candidate", "artifacts": list(ASSET_KEYS)},
            read_document(self.memory(), MANIFEST),
        )
        self.assertTrue(has_document(self.memory(), "events.json"))

    def test_a_directory_without_a_registry_has_none(self) -> None:
        self.assertFalse(has_registry(self.repo / "nowhere"))
        self.assertTrue(has_registry(self.memory()))


class MigrationTest(RegistryTestCase):
    def legacy_registry(self, rules: list[dict]) -> None:
        write_document(self.repo / "memory", "rules.json", {"rules": rules})
        use_json_layout(self.repo / "memory")

    def test_migration_moves_a_json_registry_into_concept_files_unchanged(self) -> None:
        self.legacy_registry([REFUND_RULE, ORDER_RULE])
        before = logical_documents(self.repo / "memory")
        self.assertEqual([], migrate_registry(self.repo / "memory"))
        self.assertTrue(is_concept_layout(self.repo / "memory"))
        self.assertFalse((registry_dir(self.repo / "memory") / "rules.json").exists())
        self.assertEqual(before, logical_documents(self.repo / "memory"))
        self.assertEqual("migrate-registry", read_events(self.repo / "memory")[-1]["operation"])

    def test_migrating_a_concept_registry_changes_nothing(self) -> None:
        before = snapshot_of_files(self.repo / "memory")
        self.assertEqual([], migrate_registry(self.repo / "memory"))
        self.assertEqual(before, snapshot_of_files(self.repo / "memory"))

    def test_a_missing_asset_file_is_created_before_the_move(self) -> None:
        self.legacy_registry([ORDER_RULE])
        (registry_dir(self.repo / "memory") / "events.json").unlink()
        self.assertEqual(["events.json"], migrate_registry(self.repo / "memory"))
        self.assertEqual([], read_document(self.repo / "memory", "events.json")["events"])

    def test_a_registry_the_concept_layout_cannot_hold_is_left_as_it_was(self) -> None:
        self.legacy_registry([ORDER_RULE])
        rules = registry_dir(self.repo / "memory") / "rules.json"
        write_json(rules, {**json.loads(rules.read_text(encoding="utf-8")), "note": "kept only in JSON"})
        before = snapshot_of_files(registry_dir(self.repo / "memory"))
        with self.assertRaisesRegex(ValueError, "would not hold the Registry unchanged"):
            migrate_registry(self.repo / "memory")
        self.assertEqual(before, snapshot_of_files(registry_dir(self.repo / "memory")))


class DigestAndHistoryTest(RegistryTestCase):
    def test_the_digest_does_not_depend_on_record_order(self) -> None:
        self.legacy_registry_with([REFUND_RULE, ORDER_RULE])
        first = registry_digest(self.repo / "memory")
        self.legacy_registry_with([ORDER_RULE, REFUND_RULE])
        self.assertEqual(first, registry_digest(self.repo / "memory"))

    def legacy_registry_with(self, rules: list[dict]) -> None:
        if not (registry_dir(self.repo / "memory") / MANIFEST).exists():
            use_json_layout(self.repo / "memory")
        write_json(registry_dir(self.repo / "memory") / "rules.json", {"format": "domain-rules/v1", "status": "candidate", "rules": rules})

    def test_the_documents_of_a_commit_read_back_in_either_layout(self) -> None:
        names = (MANIFEST, "rules.json", "events.json")
        write_document(self.repo / "memory", "rules.json", {"rules": [ORDER_RULE]})
        self.commit_all("concept layout")
        concept = self.head()
        use_json_layout(self.repo / "memory")
        self.commit_all("json layout")
        legacy = self.head()
        for label, commit in (("concept", concept), ("json", legacy)):
            with self.subTest(label):
                documents = committed_documents(self.repo, commit, self.repo / "memory", names)
                self.assertEqual([ORDER_RULE], documents["rules.json"]["rules"])
                self.assertEqual([], documents["events.json"]["events"])
                self.assertEqual("domain-registry/v1", documents[MANIFEST]["format"])

    def test_a_commit_without_the_registry_has_no_documents(self) -> None:
        self.assertIsNone(committed_documents(self.repo, self.head(), self.repo / "elsewhere", (MANIFEST,)))
        self.assertIsNone(committed_documents(self.repo, "f" * 40, self.repo / "memory", (MANIFEST,)))

    def head(self) -> str:
        return subprocess.run(["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True).stdout.strip()


if __name__ == "__main__":
    unittest.main()
