from __future__ import annotations

import json
import shlex
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.changes import validate_change_package
from domain_registry.evidence import citation
from domain_registry.okf import contexts_named_by
from domain_registry.recitation_package import recitation_package
from domain_registry.relocate import Recitation, recite
from test_support import RegistryTestCase

IDENTIFIERS = ("rq-recite", "pr-recite", "probe")


class RecitationPackageTest(RegistryTestCase):
    def recitation(self) -> dict:
        self.cited_rule("reviewed", (2, 2))
        return recite(self.repo / "memory", self.repo, Recitation("rules", "rule", "src/a.py", "3"))

    def read(self, output: Path, name: str) -> dict:
        return json.loads((output / name).read_text(encoding="utf-8"))

    def test_a_recitation_becomes_a_routine_package_ready_to_submit(self) -> None:
        recitation = self.recitation()
        output = self.repo / "changes" / "recite"
        self.assertEqual([], recitation_package(output, self.repo / "memory", recitation, IDENTIFIERS))
        self.assertEqual(
            [], validate_change_package(output, self.repo / "memory", "submitted", self.repo)
        )
        proposal = self.read(output, "domain-change-proposal.json")
        self.assertEqual(
            ("pr-recite", "probe", "rq-recite"),
            (proposal["proposal_id"], proposal["proposer"], proposal["requirement_id"]),
        )
        requirement = self.read(output, "requirement-normalization.json")
        self.assertEqual(("rq-recite", "probe"), (requirement["requirement_id"], requirement["actor"]))
        self.assertEqual(
            ("routine", ["orders"], ["PRO-1"], recitation["registry_updates"], None),
            (proposal["change_classification"], proposal["affected_contexts"], proposal["supersedes"],
             proposal["registry_updates"], proposal["implementation_design"]),
        )
        [obligation] = self.read(output, "test-obligations.json")["obligations"]
        self.assertEqual(
            ("citations-current", ["verify-evidence", "--registry-root", "memory", "--repo-root", ".", "--path", "src/a.py"]),
            (obligation["source_id"], shlex.split(obligation["check"])[2:]),
        )
        self.assertEqual(["src/a.py"], requirement["sources"])

    def test_a_recitation_that_is_more_than_a_recitation_is_refused(self) -> None:
        recitation = self.recitation()
        update = recitation["registry_updates"][0]
        cases = [
            ("a changed statement", {**recitation, "registry_updates": [
                {**update, "record": {**update["record"], "statement": "Other."}}]},
             "rules/rule changes more than its citations"),
            ("a removal", {**recitation, "registry_updates": [{"operation": "remove", "asset": "rules", "id": "rule"}]},
             "rules/rule is removed, which is more than a re-citation"),
            ("a record the Registry lacks", {**recitation, "registry_updates": [
                {**update, "record": {**update["record"], "id": "absent"}}]},
             "rules/absent changes more than its citations"),
            ("candidates", {**recitation, "candidates": [{"asset": "rules", "record": update["record"]}]},
             "carries candidates"),
            ("unmatched citations", {**recitation, "unmatched": [{"path": "src/a.py"}]},
             "still has unmatched citations"),
            ("no updates", {**recitation, "registry_updates": []}, "has no registry_updates"),
        ]
        for name, value, reason in cases:
            with self.subTest(name):
                output = self.repo / "changes" / name.replace(" ", "-")
                with self.assertRaisesRegex(ValueError, reason):
                    recitation_package(output, self.repo / "memory", value, IDENTIFIERS)
                self.assertFalse(output.exists())

    def test_a_record_that_names_no_context_leaves_affected_contexts_for_the_proposer(self) -> None:
        recitation = self.recitation()
        update = recitation["registry_updates"][0]
        self.seed("rules.json", [{**update["record"], "contexts": [], "review": {"proposal_id": "PRO-1"}}])
        recitation["registry_updates"] = [{**update, "record": {**update["record"], "contexts": []}}]
        output = self.repo / "changes" / "recite"
        blank = recitation_package(output, self.repo / "memory", recitation, IDENTIFIERS)
        self.assertEqual(["affected_contexts: none of the updated records names a Context"], blank)
        self.assertEqual([], self.read(output, "domain-change-proposal.json")["affected_contexts"])

    def test_a_record_citing_several_paths_lists_each_path_once_in_the_source_and_the_check(self) -> None:
        (self.repo / "src").mkdir()
        (self.repo / "src" / "a.py").write_text("one\ntwo\nthree\n", encoding="utf-8")
        (self.repo / "src" / "b.py").write_text("four\nfive\n", encoding="utf-8")
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        rule = {
            "id": "rule", "contexts": ["orders"], "statement": "S.", "status": "reviewed",
            "evidence": [
                citation(self.repo, "src/b.py", 1, 1),
                citation(self.repo, "src/a.py", 1, 1),
                citation(self.repo, "src/a.py", 3, 3),
            ],
            "review": {"proposal_id": "PRO-1"},
        }
        self.seed("rules.json", [rule])
        recitation = {"registry_updates": [{"operation": "upsert", "asset": "rules", "record": rule}]}
        output = self.repo / "changes" / "recite"
        self.assertEqual([], recitation_package(output, self.repo / "memory", recitation, IDENTIFIERS))
        self.assertEqual(["src/a.py", "src/b.py"], self.read(output, "requirement-normalization.json")["sources"])
        [obligation] = self.read(output, "test-obligations.json")["obligations"]
        arguments = shlex.split(obligation["check"])
        self.assertEqual(
            ["verify-evidence", "--registry-root", "memory", "--repo-root", ".", "--path", "src/a.py", "--path", "src/b.py"],
            arguments[2:],
        )
        self.assertTrue(arguments[1].endswith("scripts/registry_tools.py"))

    def test_the_check_runs_as_written_and_passes_once_the_citations_are_current(self) -> None:
        (self.repo / "README.md").write_bytes(b"fact\n")
        rule = {
            "id": "r", "contexts": ["orders"], "statement": "S.",
            "evidence": [citation(self.repo, "README.md", 1, 1)],
        }
        self.seed("rules.json", [rule])
        output = self.repo / "changes" / "recite"
        recitation_package(output, self.repo / "memory", {"registry_updates": [{"operation": "upsert", "asset": "rules", "record": rule}]}, IDENTIFIERS)
        [obligation] = self.read(output, "test-obligations.json")["obligations"]
        arguments = shlex.split(obligation["check"])
        completed = subprocess.run([sys.executable, *arguments[1:]], cwd=self.repo, capture_output=True, check=False)
        self.assertEqual(0, completed.returncode, completed.stdout + completed.stderr)

    def test_each_record_shape_names_its_contexts(self) -> None:
        cases = [
            ("rules", {"id": "r", "contexts": ["a", "b"]}, {"a", "b"}),
            ("aggregates", {"id": "g", "context": "a"}, {"a"}),
            ("events", {"id": "e", "owner_context": "a"}, {"a"}),
            ("interactions", {"id": "i", "producer_context": "a", "consumer_context": "b"}, {"a", "b"}),
            ("contracts", {"id": "c", "producer_context": "a", "consumer_contexts": ["b", "c"]}, {"a", "b", "c"}),
            ("dependency-policies", {"id": "d", "from_context": "a", "to_context": "b"}, {"a", "b"}),
            ("contexts", {"id": "orders"}, {"orders"}),
            ("decisions", {"id": "x", "statement": "S."}, set()),
        ]
        for asset, record, expected in cases:
            with self.subTest(asset):
                self.assertEqual(expected, contexts_named_by(asset, record))

    def test_the_command_writes_the_package_only_with_every_identifier_and_the_registry(self) -> None:
        recitation_file = self.repo / "recitation.json"
        recitation_file.write_text(json.dumps(self.recitation()), encoding="utf-8")
        base = ["init-change-package", "--output", str(self.repo / "changes" / "recite"),
                "--recitation", str(recitation_file)]
        complete = {"--registry-root": str(self.repo / "memory"), "--requirement-id": "rq",
                    "--proposal-id": "pr", "--proposer": "probe"}
        refusal = (
            "--recitation needs --registry-root, --requirement-id, --proposal-id and --proposer, "
            "so the package is complete when it is written"
        )
        for omitted in complete:
            with self.subTest(omitted=omitted):
                given = [item for flag, value in complete.items() if flag != omitted for item in (flag, value)]
                refused = self.run_cli(*base, *given)
                self.assertEqual(1, refused.returncode, refused.stdout)
                self.assertIn(refusal, refused.stdout)
                self.assertFalse((self.repo / "changes").exists())
        created = self.run_cli(*base, *[item for pair in complete.items() for item in pair])
        self.assertEqual(0, created.returncode, created.stdout + created.stderr)
        self.assertIn("a routine re-citation package", created.stdout)
        self.assertEqual(
            "rq", self.read(self.repo / "changes" / "recite", "domain-change-proposal.json")["requirement_id"]
        )


if __name__ == "__main__":
    unittest.main()
