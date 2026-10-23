from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from domain_registry.common import ASSET_KEYS, registry_dir, shown_path, write_json
from domain_registry.git import run_bounded
from domain_registry.registry import init_registry
from domain_registry.revision import registry_digest
from domain_registry.store import (
    read_record,
    MANIFEST,
    committed_documents,
    has_registry,
    read_document,
    write_document,
)
from test_support import RegistryTestCase, snapshot_of_files, orders_context

ORDER_RULE = {"id": "order-total", "statement": "An order total is never negative. It is checked on save.",
              "contexts": ["orders"], "status": "reviewed"}
REFUND_RULE = {"id": "refund-limit", "statement": "A refund never exceeds the paid amount.", "status": "candidate"}
REVIEW = {
    "proposal_id": "PRO-1",
    "proposal_revision": "sha256:r",
    "approvals": [
        {"role": "owner", "reviewer": "Ann", "decision": "approved", "approved_at": "2026-01-02T00:00:00Z"},
        {"role": "owner", "reviewer": "Bob", "decision": "rejected", "approved_at": "2026-01-03T00:00:00Z"},
    ],
}
CITED = {"path": "docs/a.md", "lines": {"start": 1, "end": 1}, "excerpt_sha256": "sha256:e"}
CITED_WITH_DERIVED_KEYS = {**CITED, "content_sha256": "sha256:c", "source_kind": "decisions"}


class WriteJsonTest(unittest.TestCase):
    def setUp(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name) / "value.json"
        self.path.write_bytes(b'{"old": true}\n')
        sleeping = patch("domain_registry.common.time.sleep")
        self.sleeps = sleeping.start()
        self.addCleanup(sleeping.stop)

    def test_a_replace_refused_for_a_moment_is_retried(self) -> None:
        replace = Path.replace
        refusals = iter([PermissionError(5, "Access is denied")])

        def refuse_once(source: Path, target: Path) -> Path:
            for refusal in refusals:
                raise refusal
            return replace(source, target)

        with patch.object(Path, "replace", autospec=True, side_effect=refuse_once):
            write_json(self.path, {"new": True})
        self.assertEqual(({"new": True}, 1), (json.loads(self.path.read_text(encoding="utf-8")), self.sleeps.call_count))

    def test_a_replace_refused_every_time_fails_and_leaves_no_temporary_file(self) -> None:
        with patch.object(Path, "replace", autospec=True, side_effect=PermissionError(5, "Access is denied")), \
                self.assertRaises(PermissionError):
            write_json(self.path, {"new": True})
        self.assertEqual(
            (["value.json"], b'{"old": true}\n'),
            (sorted(entry.name for entry in self.path.parent.iterdir()), self.path.read_bytes()),
        )


class ShownPathTest(unittest.TestCase):
    def test_a_path_is_shown_relative_to_the_repository_inside_it_and_absolute_outside_always_with_slashes(self) -> None:
        with tempfile.TemporaryDirectory() as repository, tempfile.TemporaryDirectory() as elsewhere:
            top = Path(repository)
            outside = str(Path(elsewhere, "changes", "pkg-1").resolve()).replace("\\", "/")
            cases = (
                ("inside, nested", top / "changes" / "pkg-1", top, "changes/pkg-1"),
                ("inside, through ..", top / "changes" / ".." / "changes" / "pkg-1", top, "changes/pkg-1"),
                ("the repository itself", top, top, "."),
                ("outside", Path(elsewhere) / "changes" / "pkg-1", top, outside),
                ("no repository", Path(elsewhere) / "changes" / "pkg-1", None, outside),
            )
            for label, path, root, expected in cases:
                with self.subTest(label):
                    self.assertEqual(expected, shown_path(path, root))


class ConceptLayoutTest(RegistryTestCase):
    def memory(self):
        return self.repo / "memory"

    def test_init_lays_out_a_root_index_and_one_index_per_asset(self) -> None:
        registry = registry_dir(self.memory())
        self.assertTrue(has_registry(self.memory()))
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

    def test_an_id_that_prefixes_another_reads_back_before_it_as_a_commit_lists_it(self) -> None:
        write_document(self.memory(), "rules.json", {"rules": [{"id": "order-line"}, {"id": "order"}]})
        self.assertEqual(["order", "order-line"], [rule["id"] for rule in read_document(self.memory(), "rules.json")["rules"]])

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

    def test_a_first_sentence_at_the_index_limit_is_kept_whole_and_one_past_it_is_shortened(self) -> None:
        for length, shown in ((160, "x" * 160 + "."), (161, "x" * 159 + "….")):
            with self.subTest(length=length):
                write_document(self.memory(), "rules.json", {"rules": [{"id": "long", "statement": "x" * length}]})
                line = (registry_dir(self.memory()) / "rules" / "index.md").read_text(encoding="utf-8").splitlines()[-1]
                self.assertEqual("- [long](/rules/long) — " + shown, line)

    def test_a_record_id_that_cannot_name_a_file_is_refused(self) -> None:
        for record_id in ("../escape", "a/b", "index", "", " lead", 7):
            with self.subTest(record_id=record_id), self.assertRaisesRegex(ValueError, "cannot name a concept file"):
                write_document(self.memory(), "rules.json", {"rules": [{"id": record_id}]})

    def test_two_records_with_one_id_are_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "rules holds two records with id r; a concept file holds one"):
            write_document(self.memory(), "rules.json", {"rules": [{"id": "r"}, {"id": "r", "statement": "S."}]})

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

    def test_a_directory_without_a_registry_has_none(self) -> None:
        self.assertFalse(has_registry(self.repo / "nowhere"))
        (registry_dir(self.repo / "nowhere")).mkdir(parents=True)
        write_json(registry_dir(self.repo / "nowhere") / MANIFEST, {"format": "domain-registry/v1"})
        self.assertFalse(has_registry(self.repo / "nowhere"))
        self.assertTrue(has_registry(self.memory()))


class DigestAndHistoryTest(RegistryTestCase):
    def test_an_empty_registry_keeps_the_digest_committed_packages_were_stamped_with(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "memory"
            init_registry(root)
            self.assertEqual("sha256:4a0def5bc16e39e03b939ef5f61f4dcc0c3d89f4e54f27a967c216a14720a324", registry_digest(root))

    def test_the_documents_of_a_commit_read_back(self) -> None:
        names = (MANIFEST, "rules.json", "events.json")
        write_document(self.repo / "memory", "rules.json", {"rules": [ORDER_RULE]})
        self.commit_all("concept layout")
        documents = committed_documents(self.repo, self.head(), self.repo / "memory", names)
        self.assertEqual([ORDER_RULE], documents["rules.json"]["rules"])
        self.assertEqual([], documents["events.json"]["events"])
        self.assertEqual("domain-registry/v1", documents[MANIFEST]["format"])

    def test_a_commit_without_the_registry_has_no_documents(self) -> None:
        self.assertIsNone(committed_documents(self.repo, self.head(), self.repo / "elsewhere", (MANIFEST,)))
        self.assertIsNone(committed_documents(self.repo, "f" * 40, self.repo / "memory", (MANIFEST,)))

    def test_a_commit_git_cannot_read_is_refused_naming_the_commit_and_what_git_said(self) -> None:
        commit = self.head()

        def cat_file_fails(arguments: list[str], **options: object) -> subprocess.CompletedProcess:
            if "cat-file" in arguments:
                return subprocess.CompletedProcess(arguments, 128, b"", b"fatal: object store is locked\n")
            return run_bounded(arguments, **options)

        with patch("domain_registry.git.run_bounded", side_effect=cat_file_fails), \
                self.assertRaises(ValueError) as refused:
            committed_documents(self.repo, commit, self.repo / "memory", (MANIFEST, "rules.json"))
        self.assertEqual(f"git could not read {commit}: fatal: object store is locked", str(refused.exception))

    def test_a_listed_path_git_reports_missing_is_left_out_instead_of_breaking_the_read(self) -> None:
        write_document(self.repo / "memory", "rules.json", {"rules": [ORDER_RULE]})
        self.commit_all("one rule")
        commit = self.head()

        def lists_a_ghost(arguments: list[str], **options: object) -> subprocess.CompletedProcess:
            result = run_bounded(arguments, **options)
            if "ls-tree" in arguments:
                return subprocess.CompletedProcess(arguments, 0, result.stdout + b"memory/registry/rules/ghost.md\0", b"")
            return result

        with patch("domain_registry.git.run_bounded", side_effect=lists_a_ghost):
            documents = committed_documents(self.repo, commit, self.repo / "memory", (MANIFEST, "rules.json"))
        self.assertEqual([ORDER_RULE], documents["rules.json"]["rules"])


class ReadRecordTest(RegistryTestCase):
    def test_a_record_is_read_from_its_own_concept_file_and_an_absent_or_unsafe_id_is_none(self) -> None:
        self.seed("contexts.json", [
            orders_context(),
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
        ])
        root = self.repo / "memory"
        (root / "registry" / "contexts" / "billing.md").unlink()
        self.assertEqual(
            ("Orders", None, None, None, None),
            (read_record(root, "contexts.json", "orders")["name"], read_record(root, "contexts.json", "billing"),
             read_record(root, "contexts.json", "index"), read_record(root, "contexts.json", "../orders"),
             read_record(root, "contexts.json", "orders/../orders")),
        )

if __name__ == "__main__":
    unittest.main()
