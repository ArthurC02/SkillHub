from __future__ import annotations

import json
import subprocess
import unittest
from pathlib import Path

from test_support import RegistryTestCase

KNOWN_VALUES_TAIL = (
    'is not one of the known values synchronous, eventual; it is accepted as written, '
    "but a reader looking for those values misses it."
)


class ConsistencyNoteTest(RegistryTestCase):
    def validated_registry(self, consistency: str) -> subprocess.CompletedProcess:
        self.two_contexts()
        self.seed("interactions.json", [{
            "id": "order-billed", "producer_context": "orders", "consumer_context": "billing",
            "consistency": consistency, "delivery": "outbox event",
        }])
        return self.run_cli("validate", "--registry-root", str(self.repo / "memory"))

    def validated_package(self, boundary_consistency: str, interaction_consistency: str | None = None) -> subprocess.CompletedProcess:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["boundary_decision"]["consistency"] = boundary_consistency
        if interaction_consistency is not None:
            proposal["registry_updates"].append({"operation": "upsert", "asset": "interactions", "record": {
                "id": "order-placed", "producer_context": "orders", "consumer_context": "orders",
                "consistency": interaction_consistency, "delivery": "in process", "evidence": ["README.md:1"],
            }})
        path.write_text(json.dumps(proposal), encoding="utf-8")
        return self.run_cli(
            "validate-change-package", "--package-root", str(package), "--registry-root", str(self.repo / "memory")
        )

    def test_a_registry_interaction_with_a_known_consistency_prints_no_note(self) -> None:
        for value in ("synchronous", "eventual"):
            with self.subTest(value=value):
                result = self.validated_registry(value)
                self.assertEqual((0, "Registry is valid.", ""), (result.returncode, result.stdout.strip(), result.stderr))

    def test_a_registry_interaction_with_an_unknown_consistency_is_named_and_still_valid(self) -> None:
        result = self.validated_registry("synchronous request")
        self.assertEqual(
            (0, "Registry is valid.", f'NOTE: interactions/order-billed consistency "synchronous request" {KNOWN_VALUES_TAIL}'),
            (result.returncode, result.stdout.strip(), result.stderr.strip()),
        )

    def test_a_consistency_differing_only_in_case_is_unknown(self) -> None:
        result = self.validated_registry("Eventual")
        self.assertEqual(
            f'NOTE: interactions/order-billed consistency "Eventual" {KNOWN_VALUES_TAIL}', result.stderr.strip()
        )

    def test_a_package_with_a_known_boundary_consistency_prints_no_note(self) -> None:
        result = self.validated_package("eventual")
        self.assertEqual((0, "Change package is valid.", ""), (result.returncode, result.stdout.strip(), result.stderr))

    def test_a_package_whose_boundary_consistency_is_unknown_is_named_and_still_valid(self) -> None:
        result = self.validated_package("synchronous read-only")
        self.assertEqual(
            (0, "Change package is valid.", f'NOTE: proposal boundary_decision consistency "synchronous read-only" {KNOWN_VALUES_TAIL}'),
            (result.returncode, result.stdout.strip(), result.stderr.strip()),
        )

    def test_a_package_interaction_update_with_an_unknown_consistency_is_named(self) -> None:
        result = self.validated_package("synchronous", "at once")
        self.assertEqual(
            (0, f'NOTE: proposal interactions/order-placed consistency "at once" {KNOWN_VALUES_TAIL}'),
            (result.returncode, result.stderr.strip()),
        )

    def test_the_schema_page_names_the_known_values_and_leaves_kind_and_delivery_unchecked(self) -> None:
        page = (Path(__file__).parents[1] / "references" / "registry-schema.md").read_text(encoding="utf-8")
        self.assertIn("is `synchronous` when the consumer sees the change", page)
        self.assertIn("or `eventual` when it arrives later", page)
        self.assertIn("A Contract's `kind` and an Interaction's `delivery` stay free prose and are not checked", page)


if __name__ == "__main__":
    unittest.main()
