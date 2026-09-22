from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.attestations import verify_external_scm, verify_git_signed_commit, verify_scm
from domain_registry.audit import append as append_audit
from domain_registry.audit import verify as verify_audit
from domain_registry.changes import init_change_package, validate_change_package
from domain_registry.common import ASSET_KEYS, load_json
from domain_registry.contracts import validate_schema
from domain_registry.evidence import (
    citation,
    classify_all,
    digest,
    source_map_for,
    verify,
)
from domain_registry.hitl import (
    finalize_proposal,
    record_approval,
    submit_proposal,
    supersede_proposal,
    verify_proposal,
)
from domain_registry.git_hooks import governance_readiness, install_pre_push_hook
from domain_registry.policy import amend_policy, validate_policy
from domain_registry.readiness import assess_readiness
from domain_registry.registry import (
    boundary_analysis,
    context_model,
    coverage,
    init_registry,
    lookup,
    migrate_evidence,
    migrate_registry,
    record_by_id,
    resolve_terms,
    validate,
    verify_evidence,
)
from domain_registry.revision import (
    current_registry_revision,
    registry_digest,
    require_current_registry_revision,
)
from domain_registry.security import scan as scan_secrets
from domain_registry.security import scan_report
from domain_registry.signing import SIGNING_KEY_ENV, init_signing_key
from domain_registry.sources import (
    confirm_sources,
    discover_sources,
    probe_sources,
    refine_sources,
    selected_source_map,
    source_files,
    source_kind_for,
    source_policy_report,
    verify_source_map,
    write_source_map,
)
from domain_registry.sources import discover_ci_tools
from domain_registry.sources import test_locations as discovered_test_locations
from domain_registry.transaction import recover_interrupted_update, transaction_path
from domain_registry.updates import (
    apply_approved_updates,
    demote_local_reviews,
    reconcile_pending_update,
    retract_candidate,
    upsert_candidate,
)


def stored_policy_of(repo: Path) -> dict:
    return json.loads(
        (repo / "memory" / "domain-memory-policy.json").read_text(encoding="utf-8")
    )


class DomainRegistryTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.repo = Path(self.temporary.name)
        init_registry(self.repo / "memory")
        (self.repo / "memory" / "domain-memory-policy.json").write_text(
            json.dumps(
                {
                    "format": "domain-memory-policy/v1",
                    "storage_mode": "tracked",
                    "data_classification": "internal",
                    "review_mode": "scm-verified",
                    "review_governance": {
                        "verifier": "github-pr",
                        "trigger": "external-scm",
                        "ci_requirement": "required",
                        "authorized_signers": [],
                    },
                    "source_policy": {
                        "selected_paths": ["docs"],
                        "include": ["**"],
                        "exclude": [],
                        "authority": "test",
                    },
                    "limits": {
                        "max_file_count": 1,
                        "max_file_bytes": 1,
                        "max_total_bytes": 1,
                        "max_query_results": 1,
                    },
                    "approved_command_profiles": ["unit"],
                }
            ),
            encoding="utf-8",
        )
        subprocess.run(["git", "init", "-q"], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "config", "core.autocrlf", "false"], cwd=self.repo, check=True
        )
        subprocess.run(["git", "add", "."], cwd=self.repo, check=True)
        subprocess.run(
            [
                "git",
                "-c",
                "user.name=Test",
                "-c",
                "user.email=test@example.com",
                "commit",
                "-qm",
                "seed",
            ],
            cwd=self.repo,
            check=True,
        )

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def commit_all(self, message: str) -> None:
        subprocess.run(["git", "add", "-A"], cwd=self.repo, check=True)
        subprocess.run(
            [
                "git",
                "-c",
                "user.name=Test",
                "-c",
                "user.email=test@example.com",
                "commit",
                "-qm",
                message,
            ],
            cwd=self.repo,
            check=True,
        )

    def committed_source_map(self) -> Path:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        self.commit_all("sources")
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, selected_source_map(self.repo, [self.repo / "docs"]))
        self.commit_all("source map")
        return path

    def write_record(self, name: str, value: Any) -> Path:
        path = self.repo / name
        path.write_text(json.dumps(value), encoding="utf-8")
        return path

    def seed(self, name: str, records: list[dict]) -> None:
        path = self.repo / "memory" / "registry" / name
        value = json.loads(path.read_text(encoding="utf-8"))
        value[ASSET_KEYS[name]] = records
        path.write_text(json.dumps(value), encoding="utf-8")

    def allow_results(self, count: int) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        value = json.loads(path.read_text(encoding="utf-8"))
        value["limits"]["max_query_results"] = count
        path.write_text(json.dumps(value), encoding="utf-8")

    def two_contexts(self) -> None:
        self.seed(
            "contexts.json",
            [
                {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
                {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
            ],
        )

    def add_context(self) -> None:
        record = self.write_record(
            "context.json",
            {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
        )
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def test_changed_registry_rejects_a_captured_revision(self) -> None:
        expected = current_registry_revision(self.repo / "memory", self.repo)
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["note"] = "changed"
        document.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "stale"):
            require_current_registry_revision(
                {"base_registry_revision": expected}, self.repo / "memory", self.repo
            )

    def test_invalid_candidate_does_not_change_registry(self) -> None:
        before = (self.repo / "memory" / "registry" / "vocabulary.json").read_text(
            encoding="utf-8"
        )
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
        self.assertEqual(
            before,
            (self.repo / "memory" / "registry" / "vocabulary.json").read_text(
                encoding="utf-8"
            ),
        )

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
            },
        )
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)
        stored = json.loads(
            (self.repo / "memory" / "registry" / "contexts.json").read_text(
                encoding="utf-8"
            )
        )
        written = next(
            entry for entry in stored["contexts"] if entry["id"] == "orders"
        )
        self.assertEqual(written["status"], "candidate")
        self.assertNotIn("review", written)

    def test_audit_failure_rolls_back_a_registry_swap(self) -> None:
        before = (self.repo / "memory" / "registry" / "contexts.json").read_text(
            encoding="utf-8"
        )
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
        self.assertEqual(
            before,
            (self.repo / "memory" / "registry" / "contexts.json").read_text(
                encoding="utf-8"
            ),
        )
        self.assertFalse(
            (self.repo / "memory" / ".domain-registry-transaction.json").exists()
        )

    def test_source_snapshot_detects_drift(self) -> None:
        source = self.repo / "docs"
        source.mkdir()
        (source / "rules.md").write_text("first version\n", encoding="utf-8")
        source_map = selected_source_map(self.repo, [source])
        source_map_path = self.repo / "memory" / "source-map.json"
        write_source_map(source_map_path, source_map)
        self.assertEqual(
            "current", verify_source_map(self.repo, source_map_path)["status"]
        )
        (source / "rules.md").write_text("revised version\n", encoding="utf-8")
        self.assertEqual(
            "stale", verify_source_map(self.repo, source_map_path)["status"]
        )

    def test_source_verification_rejects_a_path_outside_the_repository(self) -> None:
        source_map = {
            "selected_paths": ["../outside"],
            "source_snapshots": [{"path": "../outside", "file_count": 0}],
        }
        path = self.repo / "source-map.json"
        path.write_text(json.dumps(source_map), encoding="utf-8")
        result = verify_source_map(self.repo, path)
        self.assertEqual("invalid", result["status"])
        self.assertIn("escapes repository", result["reason"])

    def test_refining_sources_replaces_the_corpus_and_requires_confirmation(self) -> None:
        source = self.repo / "fact.md"
        source.write_text("x", encoding="utf-8")
        policy = stored_policy_of(self.repo)
        result = refine_sources(self.repo / "memory", self.repo, [source], policy)
        self.assertEqual(result["selected_paths"], ["fact.md"])
        self.assertEqual(result["selection_status"], "agent-asserted")
        self.assertEqual(stored_policy_of(self.repo)["source_policy"]["selected_paths"], ["fact.md"])

    def test_a_source_map_with_mismatched_snapshot_paths_is_unverified(self) -> None:
        source = self.repo / "docs"
        source.mkdir()
        (source / "rules.md").write_text("rules\n", encoding="utf-8")
        source_map = selected_source_map(self.repo, [source])
        source_map["source_snapshots"][0]["path"] = "other"
        path = self.write_record("source-map.json", source_map)
        self.assertEqual("unverified", verify_source_map(self.repo, path)["status"])

    def test_a_malformed_source_map_is_rejected_instead_of_treated_as_empty(
        self,
    ) -> None:
        path = self.repo / "memory" / "source-map.json"
        path.write_text("{not json", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "invalid JSON"):
            source_map_for(self.repo / "memory")

    def test_reconciliation_marks_a_committed_proposal_applied(self) -> None:
        package = self.repo / "proposal"
        package.mkdir()
        proposal_path = package / "domain-change-proposal.json"
        proposal_path.write_text(
            json.dumps({"proposal_id": "PRO-1", "status": "approved"}), encoding="utf-8"
        )
        (self.repo / "memory" / ".domain-registry-reconciliation.json").write_text(
            json.dumps({"package": "proposal", "proposal_id": "PRO-1"}),
            encoding="utf-8",
        )
        append_audit(
            self.repo / "memory",
            {"operation": "apply-approved-updates", "proposal_id": "PRO-1"},
        )
        reconcile_pending_update(self.repo / "memory", self.repo)
        applied = json.loads(proposal_path.read_text(encoding="utf-8"))
        self.assertEqual("applied", applied["status"])
        self.assertFalse(
            (self.repo / "memory" / ".domain-registry-reconciliation.json").exists()
        )

    def test_source_policy_excludes_files_and_enforces_limits(self) -> None:
        source = self.repo / "docs"
        source.mkdir()
        (source / "included.md").write_text("one", encoding="utf-8")
        (source / "private.md").write_text("two", encoding="utf-8")
        policy = {
            "source_policy": {"include": ["docs/**"], "exclude": ["docs/private.md"]},
            "limits": {
                "max_file_count": 1,
                "max_file_bytes": 10,
                "max_total_bytes": 10,
            },
        }
        report = source_policy_report(self.repo, [source], policy)
        self.assertEqual(1, report["files"])
        self.assertEqual([], report["errors"])
        source_map = selected_source_map(self.repo, [source], policy)
        self.assertEqual(
            "current",
            verify_source_map(
                self.repo, self.write_record("source-map.json", source_map), policy
            )["status"],
        )
        (source / "included.md").write_text("too many bytes", encoding="utf-8")
        self.assertEqual(
            "invalid",
            verify_source_map(self.repo, self.repo / "source-map.json", policy)[
                "status"
            ],
        )

    def test_lookup_honors_policy_result_limit(self) -> None:
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["contexts"] = [
            {"id": "one", "name": "One", "responsibility": "First."},
            {"id": "two", "name": "Two", "responsibility": "Second."},
        ]
        document.write_text(json.dumps(value), encoding="utf-8")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["limits"]["max_query_results"] = 1
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        self.assertEqual(1, len(lookup(self.repo / "memory", "contexts", "")))

    def test_migrate_evidence_upgrades_existing_line_reference(self) -> None:
        (self.repo / "source.md").write_text("evidence\n", encoding="utf-8")
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["contexts"] = [
            {
                "id": "orders",
                "name": "Orders",
                "responsibility": "Own orders.",
                "evidence": ["source.md:1"],
            }
        ]
        document.write_text(json.dumps(value), encoding="utf-8")
        self.assertEqual(1, migrate_evidence(self.repo / "memory", self.repo))
        self.assertEqual(
            1, verify_evidence(self.repo / "memory", self.repo)["summary"]["current"]
        )

    def test_scm_attestation_requires_the_same_revision(self) -> None:
        proposal = {
            "proposal_revision": 2,
            "base_registry_revision": {
                "registry_digest": "sha256:" + "0" * 64,
                "observed_commit": None,
            },
        }
        path = self.write_record(
            "attestation.json",
            {
                "provider": "github",
                "pull_request": "https://github.example/repo/pull/1",
                "checks_url": "https://github.example/repo/actions/runs/1",
                "commit": "a" * 40,
                "status": "approved",
                "proposal_revision": 2,
                "base_registry_revision": proposal["base_registry_revision"],
            },
        )
        self.assertEqual([], verify_scm(path, proposal))
        value = json.loads(path.read_text(encoding="utf-8"))
        value["proposal_revision"] = 1
        path.write_text(json.dumps(value), encoding="utf-8")
        self.assertTrue(verify_scm(path, proposal))

    def test_scm_attestation_rejects_a_local_claim_of_review(self) -> None:
        proposal = {"proposal_revision": 1, "base_registry_revision": {"registry_digest": "sha256:" + "0" * 64, "observed_commit": None}}
        path = self.write_record("attestation.json", {"provider": "git", "pull_request": "https://host.example/pull/1", "checks_url": "https://host.example/checks/1", "commit": "a" * 40, "status": "approved", "proposal_revision": 1, "base_registry_revision": proposal["base_registry_revision"]})
        errors = verify_scm(path, proposal)
        self.assertTrue(
            any("must be github or git-signed-commit" in error for error in errors), errors
        )

    def test_external_scm_verification_requires_merged_approved_green_github_pr(self) -> None:
        attestation = {
            "provider": "github",
            "pull_request": "https://github.com/acme/skills/pull/7",
            "commit": "a" * 40,
        }
        responses = [
            {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}},
            [{"user": {"login": "reviewer"}, "state": "APPROVED"}],
            {"check_runs": [{"status": "completed", "conclusion": "success"}]},
        ]
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            with patch("domain_registry.attestations.github_json", side_effect=responses):
                self.assertEqual([], verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"))

    def test_external_scm_verification_rejects_unsuccessful_checks(self) -> None:
        attestation = {
            "provider": "github",
            "pull_request": "https://github.com/acme/skills/pull/7",
            "commit": "a" * 40,
        }
        responses = [
            {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}},
            [{"user": {"login": "reviewer"}, "state": "APPROVED"}],
            {"check_runs": [{"status": "completed", "conclusion": "failure"}]},
        ]
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            with patch("domain_registry.attestations.github_json", side_effect=responses):
                self.assertEqual(
                    ["SCM attested commit has incomplete or unsuccessful checks"],
                    verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN"),
                )

    def test_external_scm_verification_can_make_ci_optional(self) -> None:
        attestation = {"provider": "github", "pull_request": "https://github.com/acme/skills/pull/7", "commit": "a" * 40}
        responses = [
            {"state": "closed", "merged_at": "2026-01-01T00:00:00Z", "head": {"sha": "a" * 40}},
            [{"user": {"login": "reviewer"}, "state": "APPROVED"}],
        ]
        with patch.dict("os.environ", {"DOMAIN_MEMORY_SCM_TOKEN": "token"}):
            with patch("domain_registry.attestations.github_json", side_effect=responses):
                self.assertEqual([], verify_external_scm(attestation, "DOMAIN_MEMORY_SCM_TOKEN", False))

    def sign_a_domain_memory_commit(self, message: str) -> str:
        target = self.repo / "docs" / "domain-memory"
        target.mkdir(parents=True, exist_ok=True)
        (target / "note.txt").write_text(message, encoding="utf-8")
        subprocess.run(["git", "add", "docs"], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "-c", "user.name=Probe", "-c", "user.email=probe@example.com",
             "commit", "-qS", "-m", message],
            cwd=self.repo, check=True,
        )
        return subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo,
            capture_output=True, text=True, check=True,
        ).stdout.strip()

    def test_a_generated_signing_key_signs_a_commit_git_can_verify(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            result = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        self.assertEqual("generated", result["source"])
        self.assertTrue(result["fingerprint"].startswith("SHA256:"))
        self.assertEqual(
            ["probe@example.com", result["fingerprint"]], result["authorized_signers"]
        )
        commit = self.sign_a_domain_memory_commit("signed by the generated key")
        for authorized in (["probe@example.com"], [result["fingerprint"]]):
            with self.subTest(authorized=authorized[0]):
                self.assertEqual(
                    [],
                    verify_git_signed_commit(
                        {"commit": commit}, self.repo, self.repo / "docs" / "domain-memory",
                        authorized,
                    ),
                )

    def test_the_same_key_rebuilds_from_the_environment(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            first = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        material = (self.repo / "keys" / "signing-key").read_text(encoding="utf-8")
        elsewhere = self.repo / "elsewhere"
        subprocess.run(["git", "init", "-q", str(elsewhere)], check=True)
        with patch.dict("os.environ", {SIGNING_KEY_ENV: material}):
            second = init_signing_key(
                elsewhere, "probe@example.com", elsewhere / "keys" / "signing-key"
            )
        self.assertEqual("environment", second["source"])
        self.assertEqual(first["fingerprint"], second["fingerprint"])

    def test_unreadable_key_material_leaves_no_key_behind(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: "-----BEGIN OPENSSH PRIVATE KEY-----\nnot a key\n"}):
            with self.assertRaisesRegex(ValueError, "readable OpenSSH private key"):
                init_signing_key(self.repo, "probe@example.com", key_file)
        self.assertFalse(key_file.exists())

    def test_the_reported_result_never_carries_private_key_material(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            result = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        self.assertNotIn("PRIVATE KEY", json.dumps(result))

    def test_an_ssh_signed_commit_is_authorized_by_principal_or_fingerprint(self) -> None:
        attestation = {"commit": "a" * 40}
        signed = subprocess.CompletedProcess(
            [],
            0,
            'Good "git" signature for arthur@example.com with ED25519 key '
            "SHA256:HMt6j4Woy7Beu896pq7/ApyBUR9qPWi/JJXlPBOVvnk\n",
            "",
        )
        changed = subprocess.CompletedProcess(
            [], 0, "docs/domain-memory/registry/rules.json\n", ""
        )
        registry_root = self.repo / "docs" / "domain-memory"
        for authorized in (
            ["arthur@example.com"],
            ["SHA256:HMt6j4Woy7Beu896pq7/ApyBUR9qPWi/JJXlPBOVvnk"],
        ):
            with self.subTest(authorized=authorized[0]):
                with patch(
                    "domain_registry.attestations.subprocess.run",
                    side_effect=[signed, changed],
                ):
                    self.assertEqual(
                        [],
                        verify_git_signed_commit(
                            attestation, self.repo, registry_root, authorized
                        ),
                    )
        with patch(
            "domain_registry.attestations.subprocess.run", side_effect=[signed, changed]
        ):
            self.assertEqual(
                ["Git commit signer is not authorized by Domain Memory policy"],
                verify_git_signed_commit(
                    attestation, self.repo, registry_root, ["someone.else@example.com"]
                ),
            )

    def test_git_signed_commit_requires_authorized_signer_and_domain_memory_change(self) -> None:
        attestation = {"commit": "a" * 40}
        verified = subprocess.CompletedProcess(
            [], 0, "[GNUPG:] VALIDSIG ABCD", ""
        )
        changed = subprocess.CompletedProcess([], 0, "docs/domain-memory/registry/rules.json\n", "")
        with patch("domain_registry.attestations.subprocess.run", side_effect=[verified, changed]):
            self.assertEqual(
                [],
                verify_git_signed_commit(
                    attestation, self.repo, self.repo / "docs" / "domain-memory", ["ABCD"]
                ),
            )
        with patch("domain_registry.attestations.subprocess.run", return_value=verified):
            self.assertEqual(
                ["Git commit signer is not authorized by Domain Memory policy"],
                verify_git_signed_commit(
                    attestation, self.repo, self.repo / "docs" / "domain-memory", ["OTHER"]
                ),
            )

    def test_pre_push_hook_checks_each_domain_memory_commit(self) -> None:
        (self.repo / ".git" / "hooks").mkdir(parents=True, exist_ok=True)
        existing = self.repo / ".git" / "hooks" / "pre-push"
        existing.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        hook = install_pre_push_hook(
            self.repo / "docs" / "domain-memory", self.repo, self.repo / "registry_tools.py"
        )
        content = hook.read_text(encoding="utf-8")
        self.assertIn("git rev-list $range -- \"$memory\"", content)
        self.assertIn("verify-git-governance", content)
        self.assertEqual(
            "#!/bin/sh\nexit 0\n",
            (self.repo / ".git" / "hooks" / "pre-push.domain-memory-existing").read_text(encoding="utf-8"),
        )

    def test_the_hook_installs_where_a_configured_hooks_path_points(self) -> None:
        amend_policy(
            self.repo / "memory", "authorized_signers", "SHA256:AAA", "Register a key."
        )
        amend_policy(
            self.repo / "memory", "review_trigger", "git-push", "Enforce on push."
        )
        amend_policy(
            self.repo / "memory",
            "review_verifier",
            "git-signed-commit",
            "The maintainer signs.",
        )
        (self.repo / ".githooks").mkdir()
        subprocess.run(
            ["git", "config", "core.hooksPath", ".githooks"], cwd=self.repo, check=True
        )
        self.assertEqual(
            ["Git HITL pre-push hook is not installed on this machine"],
            governance_readiness(self.repo / "memory", self.repo)["blocks"],
        )
        hook = install_pre_push_hook(
            self.repo / "memory", self.repo, self.repo / "registry_tools.py"
        )
        self.assertEqual(self.repo / ".githooks" / "pre-push", hook)
        self.assertFalse((self.repo / ".git" / "hooks" / "pre-push").exists())
        self.assertEqual(
            [], governance_readiness(self.repo / "memory", self.repo)["blocks"]
        )

    def test_the_generated_hook_carries_no_machine_specific_path(self) -> None:
        (self.repo / ".githooks").mkdir()
        subprocess.run(
            ["git", "config", "core.hooksPath", ".githooks"], cwd=self.repo, check=True
        )
        (self.repo / ".githooks" / "pre-push").write_text(
            "#!/bin/sh\nexit 0\n", encoding="utf-8"
        )
        hook = install_pre_push_hook(
            self.repo / "memory", self.repo, self.repo / "registry_tools.py"
        )
        content = hook.read_bytes().decode("utf-8")
        self.assertNotIn("\r", content)
        self.assertNotIn(self.repo.resolve().as_posix(), content)
        self.assertIn("tool='registry_tools.py'", content)
        self.assertIn(
            "existing='.githooks/pre-push.domain-memory-existing'", content
        )

    def test_an_existing_hook_reading_stdin_does_not_disarm_the_check(self) -> None:
        amend_policy(
            self.repo / "memory", "authorized_signers", "SHA256:NOBODY", "A key."
        )
        amend_policy(self.repo / "memory", "review_trigger", "git-push", "On push.")
        amend_policy(
            self.repo / "memory",
            "review_verifier",
            "git-signed-commit",
            "The maintainer signs.",
        )
        hooks = self.repo / ".git" / "hooks"
        hooks.mkdir(parents=True, exist_ok=True)
        with open(hooks / "pre-push", "w", encoding="utf-8", newline="\n") as handle:
            handle.write("#!/bin/sh\ncat >/dev/null\nexit 0\n")
        (hooks / "pre-push").chmod(0o755)
        hook = install_pre_push_hook(
            self.repo / "memory",
            self.repo,
            Path(__file__).resolve().parent / "registry_tools.py",
        )
        (self.repo / "memory" / "registry" / "rules.json").write_text(
            json.dumps({"format": "domain-rules/v1", "status": "candidate", "rules": []}),
            encoding="utf-8",
        )
        subprocess.run(["git", "add", "memory"], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "-c", "user.name=T", "-c", "user.email=t@example.com",
             "commit", "-qm", "unsigned domain memory change"],
            cwd=self.repo, check=True,
        )
        head = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo,
            capture_output=True, text=True, check=True,
        ).stdout.strip()
        parent = subprocess.run(
            ["git", "rev-parse", "HEAD~1"], cwd=self.repo,
            capture_output=True, text=True, check=True,
        ).stdout.strip()
        result = subprocess.run(
            ["sh", str(hook), "origin", "https://example.com/repo.git"],
            cwd=self.repo,
            input=f"refs/heads/main {head} refs/heads/main {parent}\n",
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(0, result.returncode, result.stdout + result.stderr)

    def test_local_working_memory_needs_no_governance_setup(self) -> None:
        amend_policy(self.repo / "memory", "review_mode", "local-draft-only", "Drafting only.")
        self.assertEqual(
            {"status": "working-memory", "blocks": []},
            governance_readiness(self.repo / "memory", self.repo),
        )

    def test_git_attestation_does_not_require_a_pull_request(self) -> None:
        proposal = {"proposal_revision": 1, "base_registry_revision": {"registry_digest": "sha256:" + "0" * 64, "observed_commit": None}}
        path = self.write_record("attestation.json", {"provider": "git-signed-commit", "commit": "a" * 40, "status": "approved", "proposal_revision": 1, "base_registry_revision": proposal["base_registry_revision"]})
        self.assertEqual([], verify_scm(path, proposal))

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

    def test_audit_rejects_reserved_fields_before_writing(self) -> None:
        root = self.repo / "memory"
        append_audit(root, {"operation": "first"})
        with self.assertRaisesRegex(ValueError, "reserved fields"):
            append_audit(root, {"operation": "forged", "sequence": 99})
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

    def test_audit_refuses_to_extend_a_corrupt_log(self) -> None:
        root = self.repo / "memory"
        append_audit(root, {"operation": "first"})
        events = root / "audit" / "events.jsonl"
        events.write_text(
            events.read_text(encoding="utf-8").replace("first", "tampered"),
            encoding="utf-8",
        )
        with self.assertRaisesRegex(ValueError, "audit log is invalid"):
            append_audit(root, {"operation": "second"})
        self.assertEqual(1, len(events.read_text(encoding="utf-8").splitlines()))

    def test_json_loader_rejects_duplicate_keys(self) -> None:
        path = self.repo / "duplicate.json"
        path.write_text('{"status":"approved","status":"rejected"}\n', encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "invalid JSON"):
            load_json(path)

    def test_evidence_classification_rejects_excessive_nesting(self) -> None:
        value: dict[str, Any] = {}
        nested: dict[str, Any] = value
        for _ in range(101):
            child: dict[str, Any] = {}
            nested["child"] = child
            nested = child
        with self.assertRaisesRegex(ValueError, "nesting depth"):
            classify_all(value, {})

    def test_recovery_rolls_back_an_installed_transaction_without_its_audit_event(
        self,
    ) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-new"
        staging = root / ".domain-registry-stage-new"
        shutil.copytree(root / "registry", backup)
        staging.mkdir()
        (root / "registry" / "manifest.json").write_text("{}\n", encoding="utf-8")
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
        self.assertEqual(
            "domain-registry/v1",
            json.loads(
                (root / "registry" / "manifest.json").read_text(encoding="utf-8")
            )["format"],
        )

    def test_recovery_keeps_an_installed_transaction_whose_audit_event_is_present(
        self,
    ) -> None:
        root = self.repo / "memory"
        backup = root / ".domain-registry-backup-audited"
        staging = root / ".domain-registry-stage-audited"
        shutil.copytree(root / "registry", backup)
        staging.mkdir()
        (root / "registry" / "manifest.json").write_text("{}\n", encoding="utf-8")
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
            "{}\n", (root / "registry" / "manifest.json").read_text(encoding="utf-8")
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

    def test_structured_evidence_detects_source_drift(self) -> None:
        source = self.repo / "evidence.md"
        source.write_text("one\ntwo\n", encoding="utf-8")
        content = source.read_bytes()
        excerpt = "".join(
            content.decode("utf-8").splitlines(keepends=True)[1:2]
        ).encode("utf-8")
        reference = {
            "path": "evidence.md",
            "lines": {"start": 2, "end": 2},
            "content_sha256": digest(content),
            "excerpt_sha256": digest(excerpt),
        }
        self.assertEqual("current", verify(reference, self.repo)["status"])
        source.write_text("one\nchanged\n", encoding="utf-8")
        self.assertEqual("stale", verify(reference, self.repo)["status"])

    def test_structured_evidence_allows_changes_outside_the_cited_excerpt(self) -> None:
        source = self.repo / "evidence.md"
        source.write_text("before\nfact\nafter\n", encoding="utf-8")
        reference = citation(self.repo, "evidence.md", 2, 2)

        source.write_text("changed\nfact\nafter\n", encoding="utf-8")

        self.assertEqual("current", verify(reference, self.repo)["status"])

    def draft_package(self) -> Path:
        self.add_context()
        package = self.repo / "proposal"
        init_change_package(package)
        requirement = {
            "format": "domain-requirement/v1",
            "requirement_id": "REQ-1",
            "acceptance_criteria": [
                {"id": "AC-1", "statement": "Orders have a total."}
            ],
            "risk_flags": [],
            "required_approval_roles": ["domain-owner"],
        }
        proposal = {
            "format": "domain-change-proposal/v1",
            "proposal_id": "PRO-1",
            "proposer": "author",
            "requirement_id": "REQ-1",
            "status": "draft",
            "change_classification": "material",
            "submitted_at": None,
            "finalized_at": None,
            "proposal_revision": 1,
            "base_registry_revision": None,
            "affected_contexts": ["orders"],
            "rule_ids": [],
            "contract_ids": [],
            "ownership_decision": {
                "owner_context": "orders",
                "invariant": "Order total is non-negative.",
                "evidence": [],
            },
            "boundary_decision": {
                "interaction": "none",
                "consistency": "synchronous",
                "prohibited_dependencies": [],
                "evidence": [],
            },
            "event_change": None,
            "contract_change": None,
            "registry_updates": [
                {
                    "operation": "upsert",
                    "asset": "rules",
                    "record": {
                        "id": "order-total",
                        "contexts": ["orders"],
                        "statement": "An order total is non-negative.",
                        "evidence": ["README.md:1"],
                    },
                }
            ],
            "risk_flags": [],
            "open_questions": [],
            "approvals": [],
            "implementation_design": {
                "domain_forces": ["The total must stay non-negative."],
                "decision": "Use the order boundary invariant; no tactical pattern is required.",
                "invariants_preserved": ["Order total is non-negative."],
                "rejected_alternatives": ["Let callers validate totals independently."],
                "proof_obligations": ["OB-1"],
                "counterfactual_check": "Remove the invariant check and expect OB-1 to fail.",
            },
        }
        obligations = {
            "format": "domain-test-obligations/v1",
            "requirement_id": "REQ-1",
            "proposal_id": "PRO-1",
            "obligations": [
                {
                    "id": "OB-1",
                    "source_type": "acceptance-criterion",
                    "source_id": "AC-1",
                    "expected_outcome": "A negative total is rejected.",
                    "check": "test command",
                    "status": "executed",
                }
            ],
        }
        evidence = {
            "format": "domain-evidence-bundle/v1",
            "requirement_id": "REQ-1",
            "proposal_id": "PRO-1",
            "registry_revision": None,
            "approvals": [],
            "scm_attestation": None,
            "counterfactual_check": {
                "status": "passed",
                "obligation_id": "OB-1",
                "mutated_protection": "Remove the non-negative total validation.",
                "failure_evidence": "OB-1 failed with a negative total accepted.",
                "restored": True,
            },
            "sources": [],
            "test_results": [
                {
                    "obligation_id": "OB-1",
                    "status": "passed",
                    "evidence": "test output",
                    "command_profile": "unit",
                    "exit_code": 0,
                    "output_sha256": "sha256:" + "0" * 64,
                    "finished_at": "2026-01-01T00:00:00Z",
                }
            ],
            "contract_checks": [],
            "architecture_checks": [],
            "security_checks": [],
            "residual_risks": [],
        }
        for name, value in (
            ("requirement-normalization.json", requirement),
            ("domain-change-proposal.json", proposal),
            ("test-obligations.json", obligations),
            ("evidence-bundle.json", evidence),
        ):
            (package / name).write_text(json.dumps(value), encoding="utf-8")
        return package

    def written_package(self, **replacements) -> Path:
        package = self.draft_package()
        for name, value in replacements.items():
            path = package / (name.replace("_", "-") + ".json")
            path.write_text(json.dumps(value), encoding="utf-8")
        return package

    def test_approved_package_applies_against_its_exact_revision(self) -> None:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        proposal_path = package / "domain-change-proposal.json"
        submitted = json.loads(proposal_path.read_text(encoding="utf-8"))
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["scm_attestation"] = {
            "provider": "github",
            "pull_request": "https://github.example/repo/pull/1",
            "checks_url": "https://github.example/repo/actions/runs/1",
            "commit": "a" * 40,
            "status": "approved",
            "proposal_revision": 1,
            "base_registry_revision": submitted["base_registry_revision"],
        }
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        verify_proposal(package, self.repo / "memory", self.repo)
        with patch("domain_registry.hitl.verify_external_scm", return_value=[]) as verify_scm:
            finalize_proposal(package, self.repo / "memory", self.repo)
        verify_scm.assert_called_once_with(evidence["scm_attestation"], "GITHUB_TOKEN", True)
        apply_approved_updates(package, self.repo / "memory", self.repo)
        rules = json.loads(
            (self.repo / "memory" / "registry" / "rules.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual("reviewed", rules["rules"][0]["status"])
        applied = json.loads(
            (package / "domain-change-proposal.json").read_text(encoding="utf-8")
        )
        self.assertEqual("applied", applied["status"])
        self.assertIn("registry_digest", applied["applied_registry_revision"])
        self.assertFalse(
            (self.repo / "memory" / ".domain-registry-reconciliation.json").exists()
        )
        self.assertEqual([], validate_change_package(package, self.repo / "memory"))

    def test_oversized_source_names_the_file_and_the_limit(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "big.md").write_text("x" * 64, encoding="utf-8")
        policy = json.loads(
            (self.repo / "memory" / "domain-memory-policy.json").read_text(
                encoding="utf-8"
            )
        )
        policy["limits"] = {
            "max_file_count": 100,
            "max_file_bytes": 16,
            "max_total_bytes": 10000,
            "max_query_results": 1,
        }
        report = source_policy_report(self.repo, [self.repo / "docs"], policy)
        self.assertEqual(report["oversized_files"], ["docs/big.md"])
        self.assertEqual(len(report["errors"]), 1)
        self.assertIn("docs/big.md", report["errors"][0])
        self.assertIn("16", report["errors"][0])

    def test_material_proposal_requires_implementation_design(self) -> None:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal.pop("implementation_design")
        path.write_text(json.dumps(proposal), encoding="utf-8")
        errors = validate_change_package(package, self.repo / "memory")
        self.assertTrue(
            any("requires implementation_design" in error for error in errors)
        )

    def test_proof_obligation_must_exist_in_test_obligations(self) -> None:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["implementation_design"]["proof_obligations"] = ["OB-404"]
        path.write_text(json.dumps(proposal), encoding="utf-8")
        errors = validate_change_package(package, self.repo / "memory")
        self.assertIn(
            "implementation_design references an unknown proof obligation", errors
        )

    def test_verified_material_proposal_requires_counterfactual_evidence(self) -> None:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["status"] = "verified"
        proposal["verified_at"] = "2026-01-01T00:00:00Z"
        proposal["base_registry_revision"] = {
            "observed_commit": None,
            "registry_digest": "sha256:" + "0" * 64,
        }
        path.write_text(json.dumps(proposal), encoding="utf-8")
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["registry_revision"] = proposal["base_registry_revision"]
        evidence["counterfactual_check"] = None
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        errors = validate_change_package(package, self.repo / "memory")
        self.assertIn(
            "a material verified proposal requires counterfactual_check evidence",
            errors,
        )

    def test_a_gitignored_file_is_not_a_source(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        (self.repo / "docs" / "build.out").write_text("generated", encoding="utf-8")
        (self.repo / ".gitignore").write_text("*.out" + chr(10), encoding="utf-8")
        found = source_files(self.repo, [self.repo / "docs"])
        self.assertEqual([path.name for path in found], ["kept.md"])

    def test_a_freshly_selected_source_map_is_only_ever_agent_asserted(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        asserted = selected_source_map(self.repo, [self.repo / "docs"])
        self.assertEqual(asserted["selection_status"], "agent-asserted")
        self.assertNotIn("confirmed_by", asserted)

    def test_confirming_records_who_chose_the_sources_and_when(self) -> None:
        path = self.committed_source_map()
        confirmed = confirm_sources(self.repo / "memory", self.repo, "Arthur")
        self.assertEqual(confirmed["selection_status"], "developer-confirmed")
        self.assertEqual(confirmed["confirmed_by"], "Arthur")
        self.assertTrue(confirmed["confirmed_at"].endswith("Z"))
        self.assertEqual(
            json.loads(path.read_text(encoding="utf-8"))["selection_status"],
            "developer-confirmed",
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

    def test_confirming_without_a_named_developer_leaves_the_map_agent_asserted(
        self,
    ) -> None:
        path = self.committed_source_map()
        for empty in ("", "   ", "<identity>"):
            with self.assertRaisesRegex(ValueError, "developer"):
                confirm_sources(self.repo / "memory", self.repo, empty)
        self.assertEqual(
            json.loads(path.read_text(encoding="utf-8"))["selection_status"],
            "agent-asserted",
        )

    def test_sources_that_moved_cannot_be_confirmed(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "kept.md").write_text(
            "changed after the developer looked", encoding="utf-8"
        )
        with self.assertRaisesRegex(ValueError, "no longer there"):
            confirm_sources(self.repo / "memory", self.repo, "Arthur")
        self.assertEqual(
            json.loads(path.read_text(encoding="utf-8"))["selection_status"],
            "agent-asserted",
        )

    def test_initialization_does_not_take_a_developer_identity(self) -> None:
        result = self.run_cli(
            "init-domain-memory",
            "--repo-root",
            str(self.repo),
            "--output",
            str(self.repo / "elsewhere"),
            "--source",
            str(self.repo / "docs"),
            "--storage-mode",
            "ignored",
            "--data-classification",
            "internal",
            "--review-mode",
            "local-draft-only",
            "--source-authority",
            "test",
            "--confirmed-by",
            "Arthur",
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("--confirmed-by", result.stderr)
        self.assertFalse((self.repo / "elsewhere").exists())

    def permit_sources(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        value = json.loads(path.read_text(encoding="utf-8"))
        value["limits"] |= {
            "max_file_count": 50,
            "max_file_bytes": 100000,
            "max_total_bytes": 500000,
        }
        path.write_text(json.dumps(value), encoding="utf-8")

    def test_a_source_map_the_policy_rejects_is_not_reported_as_moved(self) -> None:
        self.committed_source_map()
        with self.assertRaisesRegex(ValueError, "does not verify"):
            confirm_sources(
                self.repo / "memory", self.repo, "Arthur", stored_policy_of(self.repo)
            )

    def test_the_confirm_command_moves_the_map_through_the_command_line(self) -> None:
        self.committed_source_map()
        self.permit_sources()
        result = self.run_cli(
            "confirm-sources",
            "--registry-root",
            str(self.repo / "memory"),
            "--repo-root",
            str(self.repo),
            "--confirmed-by",
            "Arthur",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Arthur", result.stdout)
        stored = json.loads(
            (self.repo / "memory" / "source-map.json").read_text(encoding="utf-8")
        )
        self.assertEqual(stored["selection_status"], "developer-confirmed")

    def test_the_confirm_command_reports_a_refusal_instead_of_a_traceback(self) -> None:
        self.committed_source_map()
        self.permit_sources()
        result = self.run_cli(
            "confirm-sources",
            "--registry-root",
            str(self.repo / "memory"),
            "--repo-root",
            str(self.repo),
            "--confirmed-by",
            "<identity>",
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("ERROR", result.stdout)
        self.assertNotIn("Traceback", result.stderr)

    def test_a_malformed_json_argument_reports_an_error_instead_of_a_traceback(
        self,
    ) -> None:
        malformed = self.repo / "malformed.json"
        malformed.write_text("{", encoding="utf-8")
        result = self.run_cli("validate-policy", "--policy", str(malformed))
        self.assertEqual(result.returncode, 1)
        self.assertIn("ERROR", result.stdout)
        self.assertNotIn("Traceback", result.stderr)

    def test_the_instruction_files_group_says_it_is_not_a_statement_of_the_domain(
        self,
    ) -> None:
        (self.repo / "AGENTS.md").write_text("How agents work here.", encoding="utf-8")
        group = next(
            g
            for g in discover_sources(self.repo)["source_groups"]
            if g["kind"] == "repository_instructions"
        )
        self.assertEqual(group["paths"], ["AGENTS.md"])
        self.assertIn("not a statement of the business domain", group["authority"])
        self.assertIn("loses its evidence", group["caution"])

    def defined_term(self) -> None:
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "run",
                    "name": "Run",
                    "definition": "One execution of a Skill against a Test Case.",
                    "contexts": ["trial"],
                },
            ],
        )
        self.allow_results(10)

    def test_a_requirement_sentence_resolves_the_terms_it_names(self) -> None:
        self.defined_term()
        matches = resolve_terms(
            self.repo / "memory", "Who starts the Evaluation once a Run succeeds?", None
        )
        self.assertEqual([term["id"] for term in matches], ["run"])

    def test_a_sentence_naming_no_registered_term_still_resolves_to_nothing(
        self,
    ) -> None:
        self.defined_term()
        self.assertEqual(
            resolve_terms(
                self.repo / "memory", "Who signs off on the quarterly budget?", None
            ),
            [],
        )

    def test_a_word_from_the_definition_still_resolves_the_term(self) -> None:
        self.defined_term()
        matches = resolve_terms(self.repo / "memory", "test case", None)
        self.assertEqual([term["id"] for term in matches], ["run"])

    def test_a_file_holding_several_records_says_how_many_it_should_hold(self) -> None:
        record = self.write_record("many.json", [{"id": "orders"}, {"id": "billing"}])
        with self.assertRaisesRegex(ValueError, "One record per call"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def test_a_change_package_offered_as_a_record_is_refused(self) -> None:
        record = self.write_record(
            "package.json", {"registry_updates": [{"id": "orders"}]}
        )
        with self.assertRaisesRegex(ValueError, "not a Change Package"):
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def test_an_agent_asserted_map_still_reports_whether_its_sources_moved(
        self,
    ) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        kept = self.repo / "docs" / "kept.md"
        kept.write_text("kept", encoding="utf-8")
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, selected_source_map(self.repo, [self.repo / "docs"]))
        fresh = verify_source_map(self.repo, path)
        self.assertEqual(fresh["status"], "current")
        self.assertEqual(fresh["selection_status"], "agent-asserted")
        kept.write_text("edited", encoding="utf-8")
        self.assertEqual(verify_source_map(self.repo, path)["status"], "stale")

    def test_a_map_without_snapshots_cannot_answer_the_question(self) -> None:
        path = self.write_record(
            "source-map.json",
            {"selected_paths": [], "selection_status": "developer-confirmed"},
        )
        self.assertEqual(verify_source_map(self.repo, path)["status"], "unverified")

    def test_a_clean_repo_answers_the_probe_without_hashing_anything(self) -> None:
        result = probe_sources(self.repo, self.committed_source_map())
        self.assertEqual(result["status"], "current")
        self.assertEqual(result["checked"], "git")

    def test_probe_rechecks_policy_limits_before_using_the_git_fast_path(self) -> None:
        path = self.committed_source_map()
        policy = stored_policy_of(self.repo)
        policy["limits"]["max_file_bytes"] = 1
        result = probe_sources(self.repo, path, policy)
        self.assertEqual(result["status"], "invalid")

    def test_probe_rejects_policy_source_paths_that_differ_from_the_map(self) -> None:
        path = self.committed_source_map()
        policy = stored_policy_of(self.repo)
        policy["source_policy"]["selected_paths"] = ["other"]
        result = probe_sources(self.repo, path, policy)
        self.assertEqual(result["status"], "invalid")
        self.assertEqual(
            "invalid", verify_source_map(self.repo, path, policy)["status"]
        )

    def test_an_uncommitted_source_sends_the_probe_back_to_hashing(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "kept.md").write_text("edited", encoding="utf-8")
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "stale")

    def test_a_committed_edit_to_a_source_is_reported_as_stale(self) -> None:
        path = self.committed_source_map()
        (self.repo / "docs" / "kept.md").write_text("edited", encoding="utf-8")
        self.commit_all("edit a source")
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "stale")

    def test_a_map_without_a_recorded_git_state_is_probed_by_hashing(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        source_map = selected_source_map(self.repo, [self.repo / "docs"])
        del source_map["git_state"]
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, source_map)
        result = probe_sources(self.repo, path)
        self.assertEqual(result["checked"], "hash")
        self.assertEqual(result["status"], "current")

    def nested_sources(self) -> dict:
        (self.repo / "apps" / "svc").mkdir(parents=True, exist_ok=True)
        (self.repo / "apps" / "other.go").write_text("other", encoding="utf-8")
        (self.repo / "apps" / "svc" / "doc.go").write_text(
            "package svc", encoding="utf-8"
        )
        return selected_source_map(
            self.repo, [self.repo / "apps", self.repo / "apps" / "svc" / "doc.go"]
        )

    def test_a_nested_source_owns_its_files_instead_of_its_parent(self) -> None:
        source_map = self.nested_sources()
        counts = {
            snapshot["path"]: snapshot["file_count"]
            for snapshot in source_map["source_snapshots"]
        }
        self.assertEqual(counts["apps/svc/doc.go"], 1)
        self.assertEqual(counts["apps"], 1)

    def test_editing_a_nested_source_leaves_its_parent_current(self) -> None:
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, self.nested_sources())
        (self.repo / "apps" / "svc" / "doc.go").write_text(
            "package svc // revised", encoding="utf-8"
        )
        result = verify_source_map(self.repo, path)
        self.assertEqual(result["status"], "stale")
        self.assertEqual(
            [source["path"] for source in result["changed_sources"]],
            ["apps/svc/doc.go"],
        )

    def test_the_most_specific_source_decides_what_a_citation_establishes(self) -> None:
        source_map = self.nested_sources()
        self.assertEqual(
            source_kind_for(source_map, "apps/svc/doc.go"), "bounded_contexts"
        )
        self.assertEqual(source_kind_for(source_map, "apps/other.go"), "implementation")

    def test_a_citation_outside_every_selected_source_is_unclassified(self) -> None:
        self.assertEqual(
            source_kind_for(self.nested_sources(), "infra/node.txt"), "unclassified"
        )

    def test_a_selected_file_inherits_its_discovered_parent_kind(self) -> None:
        source_map = {
            "source_kinds": {"apps/svc/preflight.go": "unclassified"},
            "source_groups": [{"kind": "implementation", "paths": ["apps"]}],
        }
        self.assertEqual(
            source_kind_for(source_map, "apps/svc/worker.go"), "implementation"
        )

    def test_nested_test_directories_are_reported_without_reclassifying_implementation(
        self,
    ) -> None:
        (self.repo / "apps" / "svc").mkdir(parents=True, exist_ok=True)
        (self.repo / "apps" / "svc" / "thing_test.go").write_text("x", encoding="utf-8")
        (self.repo / "apps" / "svc" / "thing.go").write_text("x", encoding="utf-8")
        self.assertEqual(
            discovered_test_locations(self.repo), ["apps/svc/thing_test.go"]
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
        stored = json.loads(
            (self.repo / "memory" / "registry" / "contexts.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual(
            stored["contexts"][0]["evidence"][0]["source_kind"], "decisions"
        )
        summary = verify_evidence(self.repo / "memory", self.repo)["summary"]
        self.assertEqual(summary["by_source_kind"], {"decisions": 1})

    def test_a_probe_of_an_uninitialized_repository_says_so(self) -> None:
        result = probe_sources(self.repo, self.repo / "nowhere" / "source-map.json")
        self.assertEqual(result["status"], "absent")

    def test_ci_discovery_offers_candidates_without_selecting_governance(self) -> None:
        (self.repo / ".github" / "workflows").mkdir(parents=True)
        self.assertEqual(["github-actions"], discover_ci_tools(self.repo))
        discovered = discover_sources(self.repo)
        self.assertEqual("github-pr", discovered["governance_candidates"]["recommended_verifier"])
        self.assertEqual("discovered", discovered["selection_status"])

    def test_discovery_only_recommends_governance_a_policy_accepts(self) -> None:
        def policy_from(candidates: dict) -> dict:
            value = json.loads(
                (self.repo / "memory" / "domain-memory-policy.json").read_text(
                    encoding="utf-8"
                )
            )
            value["review_mode"] = "scm-verified"
            value["review_governance"] = {
                "verifier": candidates["recommended_verifier"],
                "trigger": candidates["recommended_trigger"],
                "ci_requirement": "optional",
                "authorized_signers": ["ABCD"],
            }
            return value

        with self.subTest("no CI discovered"):
            candidates = discover_sources(self.repo)["governance_candidates"]
            self.assertEqual([], validate_policy(policy_from(candidates)))

        (self.repo / ".github" / "workflows").mkdir(parents=True)
        with self.subTest("CI discovered"):
            candidates = discover_sources(self.repo)["governance_candidates"]
            self.assertEqual([], validate_policy(policy_from(candidates)))

    def test_resolve_terms_reads_the_definition_and_not_the_rest_of_the_record(
        self,
    ) -> None:
        self.two_contexts()
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A confirmed purchase.",
                    "contexts": ["orders"],
                },
                {
                    "id": "invoice",
                    "name": "Invoice",
                    "definition": "A demand for payment.",
                    "contexts": ["billing"],
                    "note": "A confirmed purchase becomes this.",
                },
            ],
        )
        self.allow_results(10)
        found = [
            term["id"]
            for term in resolve_terms(self.repo / "memory", "confirmed purchase", None)
        ]
        self.assertEqual(found, ["order"])

    def test_resolve_terms_matches_regardless_of_letter_case(self) -> None:
        self.two_contexts()
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A purchase.",
                    "contexts": ["orders"],
                }
            ],
        )
        self.assertEqual(len(resolve_terms(self.repo / "memory", "ORDER", None)), 1)

    def test_resolve_terms_excludes_a_term_that_does_not_belong_to_the_named_context(
        self,
    ) -> None:
        self.two_contexts()
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A purchase.",
                    "contexts": ["orders"],
                }
            ],
        )
        self.assertEqual(
            resolve_terms(self.repo / "memory", "order", "orders")[0]["id"], "order"
        )
        self.assertEqual(resolve_terms(self.repo / "memory", "order", "billing"), [])

    def test_resolve_terms_stops_at_the_policy_result_limit(self) -> None:
        self.two_contexts()
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A purchase.",
                    "contexts": ["orders"],
                },
                {
                    "id": "order-line",
                    "name": "Order line",
                    "definition": "A purchase line.",
                    "contexts": ["orders"],
                },
            ],
        )
        self.allow_results(1)
        self.assertEqual(len(resolve_terms(self.repo / "memory", "purchase", None)), 1)

    def test_an_absent_record_is_reported_as_absent_rather_than_raised(self) -> None:
        self.two_contexts()
        record = record_by_id(self.repo / "memory", "contexts", "orders")
        self.assertIsNotNone(record)
        assert record is not None
        self.assertEqual(record["name"], "Orders")
        self.assertIsNone(record_by_id(self.repo / "memory", "contexts", "shipping"))

    def test_an_unknown_context_has_no_model(self) -> None:
        self.two_contexts()
        self.assertIsNone(context_model(self.repo / "memory", "shipping"))

    def test_a_context_model_carries_the_contracts_it_consumes_as_well_as_those_it_produces(
        self,
    ) -> None:
        self.two_contexts()
        self.seed(
            "contracts.json",
            [
                {
                    "id": "invoice-api",
                    "producer_context": "billing",
                    "consumer_contexts": ["orders"],
                },
                {
                    "id": "order-api",
                    "producer_context": "orders",
                    "consumer_contexts": [],
                },
                {
                    "id": "shipping-api",
                    "producer_context": "billing",
                    "consumer_contexts": [],
                },
            ],
        )
        model = context_model(self.repo / "memory", "orders")
        self.assertIsNotNone(model)
        assert model is not None
        self.assertEqual(model["usage"], "constraint")
        self.assertEqual(
            sorted(entry["id"] for entry in model["contracts"]),
            ["invoice-api", "order-api"],
        )

    def test_a_local_draft_context_model_is_working_memory(self) -> None:
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        self.two_contexts()
        model = context_model(self.repo / "memory", "orders")
        self.assertIsNotNone(model)
        assert model is not None
        self.assertEqual(model["usage"], "working-memory")

    def test_local_draft_memory_cannot_satisfy_reviewed_validation(self) -> None:
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        errors = validate(self.repo / "memory", self.repo, True)
        self.assertTrue(any("cannot satisfy" in error for error in errors), errors)

    def retracted_contexts(self) -> list[dict]:
        document = self.repo / "memory" / "registry" / "contexts.json"
        return json.loads(document.read_text(encoding="utf-8"))["contexts"]

    def test_retracting_a_candidate_removes_it(self) -> None:
        self.two_contexts()
        retract_candidate(
            self.repo / "memory",
            self.repo,
            "contexts",
            "billing",
            "Examination found no Billing boundary in this repository.",
        )
        self.assertEqual([entry["id"] for entry in self.retracted_contexts()], ["orders"])

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
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["contexts"][0]["status"] = "reviewed"
        document.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "superseding proposal"):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "orders", "Changed my mind."
            )
        self.assertEqual(
            [entry["id"] for entry in self.retracted_contexts()], ["orders", "billing"]
        )

    def test_retracting_an_absent_record_changes_nothing(self) -> None:
        self.two_contexts()
        with self.assertRaisesRegex(ValueError, "no such record"):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "shipping", "Never existed."
            )
        self.assertEqual(
            [entry["id"] for entry in self.retracted_contexts()], ["orders", "billing"]
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
        with self.assertRaises(ValueError):
            retract_candidate(
                self.repo / "memory", self.repo, "contexts", "billing", "No longer wanted."
            )
        self.assertEqual(
            [entry["id"] for entry in self.retracted_contexts()], ["orders", "billing"]
        )

    def test_a_candidate_record_is_named_by_reviewed_validation(self) -> None:
        self.two_contexts()
        errors = validate(self.repo / "memory", self.repo, True)
        self.assertIn("contexts.json:orders is not reviewed", errors)

    def test_a_registry_of_reviewed_records_satisfies_reviewed_validation(self) -> None:
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "status": "reviewed",
                    "review": {"proposal_id": "P-1"},
                    "evidence": ["docs/kept.md:1"],
                }
            ],
        )
        errors = validate(self.repo / "memory", self.repo, True)
        self.assertEqual([error for error in errors if "not reviewed" in error], [])

    def test_demoting_local_reviews_removes_reviewed_status(self) -> None:
        amend_policy(
            self.repo / "memory", "review_mode", "local-draft-only", "No external verifier is configured."
        )
        self.two_contexts()
        document = self.repo / "memory" / "registry" / "contexts.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["status"] = "reviewed"
        value["contexts"][0]["status"] = "reviewed"
        value["contexts"][0]["review"] = {"proposal_id": "P-1"}
        document.write_text(json.dumps(value), encoding="utf-8")
        demote_local_reviews(self.repo / "memory", self.repo)
        demoted = json.loads(document.read_text(encoding="utf-8"))
        self.assertEqual(demoted["contexts"][0]["status"], "candidate")
        self.assertNotIn("review", demoted["contexts"][0])

    def test_two_contexts_without_registered_collaboration_say_so(self) -> None:
        self.two_contexts()
        self.assertEqual(
            boundary_analysis(self.repo / "memory", "orders", "billing")["status"],
            "no_registered_collaboration",
        )

    def test_a_registered_interaction_makes_a_boundary_known(self) -> None:
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
        self.assertEqual(
            boundary_analysis(self.repo / "memory", "orders", "billing")["status"],
            "known",
        )

    def test_a_reviewed_internal_interaction_does_not_require_a_contract(self) -> None:
        self.two_contexts()
        source = self.repo / "fact.md"
        source.write_text("Orders notify Billing.", encoding="utf-8")
        document = self.repo / "memory" / "registry" / "interactions.json"
        value = json.loads(document.read_text(encoding="utf-8"))
        value["status"] = "reviewed"
        value["interactions"] = [{"id": "order-billed", "producer_context": "orders", "consumer_context": "billing", "consistency": "eventual", "delivery": "outbox event", "evidence": [citation(self.repo, "fact.md", 1, 1)], "review": {"proposal_id": "P-1"}}]
        document.write_text(json.dumps(value), encoding="utf-8")
        self.assertEqual(validate(self.repo / "memory", self.repo, False), [])

    def test_a_registered_interaction_does_not_make_the_reverse_direction_known(
        self,
    ) -> None:
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
        self.assertEqual(
            boundary_analysis(self.repo / "memory", "billing", "orders")["status"],
            "no_registered_collaboration",
        )

    def test_a_dependency_policy_alone_does_not_register_a_collaboration(self) -> None:
        self.two_contexts()
        self.seed(
            "dependency-policies.json",
            [
                {
                    "id": "orders-billing",
                    "from_context": "orders",
                    "to_context": "billing",
                }
            ],
        )
        analysis = boundary_analysis(self.repo / "memory", "orders", "billing")
        self.assertEqual(len(analysis["dependencies"]), 1)
        self.assertEqual(analysis["status"], "no_registered_collaboration")

    def test_coverage_reports_a_context_only_under_the_asset_it_lacks(self) -> None:
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "order",
                    "name": "Order",
                    "definition": "A purchase.",
                    "contexts": ["orders"],
                }
            ],
        )
        gaps = coverage(self.repo / "memory")["gaps"]
        self.assertEqual(gaps["without_vocabulary"], [])
        self.assertEqual(gaps["without_aggregate"], ["orders"])
        self.assertEqual(gaps["without_rule"], ["orders"])
        self.assertEqual(gaps["without_contract"], ["orders"])

    def test_requiring_reviewed_records_rejects_a_candidate_registry(self) -> None:
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        self.assertEqual(validate(self.repo / "memory", None, False), [])
        errors = validate(self.repo / "memory", None, True)
        self.assertTrue(
            any("contexts.json:orders is not reviewed" in error for error in errors)
        )

    def test_a_reviewed_record_without_evidence_is_rejected(self) -> None:
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "status": "reviewed",
                    "review": {"proposal_id": "p-1"},
                }
            ],
        )
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(
            any(
                "contexts.json:orders is reviewed but lacks evidence" in error
                for error in errors
            )
        )

    def test_a_non_json_contract_is_reported_unverified_instead_of_valid(self) -> None:
        path = self.repo / "openapi.yaml"
        path.write_text("openapi: 3.0.0", encoding="utf-8")
        self.assertEqual(validate_schema(path)["status"], "unverified")

    def test_a_malformed_json_contract_is_invalid(self) -> None:
        path = self.repo / "broken.json"
        path.write_text("{not json", encoding="utf-8")
        self.assertEqual(validate_schema(path)["status"], "invalid")

    def test_an_openapi_document_is_recognized_as_a_contract(self) -> None:
        path = self.repo / "public.json"
        path.write_text(json.dumps({"openapi": "3.0.0", "paths": {}}), encoding="utf-8")
        self.assertEqual(
            validate_schema(path),
            {"status": "valid", "kind": "openapi", "path": str(path)},
        )

    def test_the_secret_scanner_finds_each_kind_it_claims_to_find(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        label = "PRIVATE KEY"
        (secrets / "key.pem").write_text(
            f"-----BEGIN RSA {label}-----", encoding="utf-8"
        )
        (secrets / "ci.env").write_text("ghp_" + "a" * 30, encoding="utf-8")
        (secrets / "app.conf").write_text("api_key = " + "b" * 20, encoding="utf-8")
        (secrets / "clean.md").write_text("A token is a word.", encoding="utf-8")
        found = {finding["kind"] for finding in scan_secrets(secrets)}
        self.assertEqual(found, {"private-key", "github-token", "generic-secret"})

    def test_a_secret_finding_never_carries_the_secret(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        (secrets / "ci.env").write_text("ghp_" + "c" * 30, encoding="utf-8")
        finding = scan_secrets(secrets)[0]
        self.assertEqual(sorted(finding), ["kind", "line", "path"])
        self.assertNotIn("c" * 30, json.dumps(finding))

    def test_secret_scanner_stops_at_its_resource_budget(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        (secrets / "ci.env").write_text("token = " + "d" * 20, encoding="utf-8")
        with patch("domain_registry.security.MAX_SCAN_FILES", 0):
            report = scan_report(secrets)
        self.assertEqual("incomplete", report["status"])
        self.assertEqual("resource-limit", report["skipped"][0]["reason"])

    def test_secret_scan_reports_oversized_files_as_incomplete(self) -> None:
        secrets = self.repo / "leaky"
        secrets.mkdir()
        with (secrets / "large.log").open("wb") as output:
            output.truncate(1_048_577)
        report = scan_report(secrets)
        self.assertEqual("incomplete", report["status"])
        self.assertEqual("file-too-large", report["skipped"][0]["reason"])

    def test_citation_refuses_an_oversized_source(self) -> None:
        source = self.repo / "large.md"
        with source.open("wb") as output:
            output.truncate(16 * 1024 * 1024 + 1)
        with self.assertRaisesRegex(ValueError, "exceeds"):
            citation(self.repo, "large.md", 1, 1)

    def run_cli(self, *arguments: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", *arguments],
            cwd=Path(__file__).parent,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_the_module_entry_point_runs_the_command_instead_of_exiting_quietly(
        self,
    ) -> None:
        result = self.run_cli("coverage", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["contexts"], 0)

    def test_readiness_calls_a_repository_without_product_sources_empty(self) -> None:
        result = assess_readiness(self.repo)
        self.assertEqual(result["state"], "empty")
        self.assertEqual(result["next_capability"], "discover")

    def test_readiness_calls_documented_work_without_implementation_greenfield(
        self,
    ) -> None:
        (self.repo / "docs" / "requirements").mkdir(parents=True)
        result = assess_readiness(self.repo)
        self.assertEqual(result["state"], "greenfield")
        self.assertEqual(result["next_capability"], "design")

    def test_readiness_calls_existing_implementation_brownfield(self) -> None:
        implementation = self.repo / "apps" / "orders"
        implementation.mkdir(parents=True)
        (implementation / "main.py").write_text("pass\n", encoding="utf-8")
        result = assess_readiness(self.repo)
        self.assertEqual(result["state"], "brownfield")
        self.assertEqual(result["next_capability"], "read-and-maintain")

    def test_readiness_calls_an_unverifiable_memory_without_sources_dead(self) -> None:
        memory = self.repo / "orphaned-memory"
        memory.mkdir()
        result = assess_readiness(self.repo, memory)
        self.assertEqual(result["state"], "dead")
        self.assertEqual(result["confidence"], "medium")
        self.assertTrue(result["blocks"])

    def test_readiness_routes_a_valid_registry_with_domain_records_to_greenfield(
        self,
    ) -> None:
        docs = self.repo / "docs"
        docs.mkdir()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [docs]),
        )
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        result = assess_readiness(self.repo, self.repo / "memory")
        self.assertEqual(result["state"], "greenfield")
        self.assertEqual(
            result["signals"][-1], {"kind": "registry_records", "count": 1}
        )

    def test_readiness_reports_an_empty_working_memory_as_current(self) -> None:
        (self.repo / "docs").mkdir()
        implementation = self.repo / "apps" / "orders"
        implementation.mkdir(parents=True)
        (implementation / "main.py").write_text("pass\n", encoding="utf-8")
        self.commit_all("implementation")
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(
                self.repo,
                [self.repo / "docs"],
                stored_policy_of(self.repo),
            ),
        )
        self.commit_all("source map")
        result = assess_readiness(self.repo, self.repo / "memory")

        self.assertEqual("brownfield", result["state"])
        self.assertEqual("read-and-maintain", result["next_capability"])
        self.assertEqual(
            {"kind": "registry", "status": "current"}, result["signals"][-2]
        )
        self.assertEqual([], result["blocks"])

    def test_readiness_does_not_treat_a_corrupt_registry_as_empty(self) -> None:
        docs = self.repo / "domain-notes"
        docs.mkdir()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [docs]),
        )
        (self.repo / "memory" / "registry" / "contexts.json").write_text(
            '{"contexts": "broken"}', encoding="utf-8"
        )
        result = assess_readiness(self.repo, self.repo / "memory")
        self.assertEqual(result["state"], "dead")
        self.assertEqual(result["signals"][-2]["status"], "invalid")

    def test_readiness_command_returns_json_without_writing(self) -> None:
        before = sorted(
            path.relative_to(self.repo).as_posix() for path in self.repo.rglob("*")
        )
        result = self.run_cli("readiness", "--repo-root", str(self.repo))
        after = sorted(
            path.relative_to(self.repo).as_posix() for path in self.repo.rglob("*")
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["state"], "empty")
        self.assertEqual(before, after)

    def test_initializing_into_an_occupied_destination_is_refused(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "rules.md").write_text("rules", encoding="utf-8")
        result = self.run_cli(
            "init-domain-memory",
            "--repo-root",
            str(self.repo),
            "--output",
            str(self.repo / "memory"),
            "--source",
            str(self.repo / "docs"),
            "--storage-mode",
            "ignored",
            "--data-classification",
            "internal",
            "--review-mode",
            "local-draft-only",
            "--source-authority",
            "test",
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("not empty", result.stdout)

    def test_initializing_a_fresh_destination_writes_the_registry_and_its_policy(
        self,
    ) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "rules.md").write_text("rules", encoding="utf-8")
        destination = self.repo / "domain-memory"
        result = self.run_cli(
            "init-domain-memory",
            "--repo-root",
            str(self.repo),
            "--output",
            str(destination),
            "--source",
            str(self.repo / "docs"),
            "--storage-mode",
            "ignored",
            "--data-classification",
            "internal",
            "--review-mode",
            "local-draft-only",
            "--source-authority",
            "test",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((destination / "source-map.json").is_file())
        self.assertTrue((destination / "domain-memory-policy.json").is_file())
        self.assertTrue((destination / "registry" / "contexts.json").is_file())

    def test_a_policy_is_rejected_one_step_past_every_limit(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        valid = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual(validate_policy(valid), [])
        for key in (
            "max_file_count",
            "max_file_bytes",
            "max_total_bytes",
            "max_query_results",
        ):
            broken = json.loads(json.dumps(valid))
            broken["limits"][key] = 0
            self.assertEqual(
                validate_policy(broken), ["policy has invalid limits"], key
            )

    def test_a_policy_without_a_named_source_authority_is_rejected(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        broken = json.loads(path.read_text(encoding="utf-8"))
        broken["source_policy"]["authority"] = "   "
        self.assertIn(
            "policy requires selected source paths and authority",
            validate_policy(broken),
        )

    def test_an_unknown_storage_mode_is_rejected(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        broken = json.loads(path.read_text(encoding="utf-8"))
        broken["storage_mode"] = "somewhere-else"
        self.assertIn("policy has an invalid storage_mode", validate_policy(broken))

    def test_migrating_restores_a_missing_asset_without_touching_the_others(
        self,
    ) -> None:
        self.seed(
            "contexts.json",
            [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}],
        )
        (self.repo / "memory" / "registry" / "events.json").unlink()
        created = migrate_registry(self.repo / "memory")
        self.assertEqual(created, ["events.json"])
        record = record_by_id(self.repo / "memory", "contexts", "orders")
        self.assertIsNotNone(record)
        assert record is not None
        self.assertEqual(record["name"], "Orders")

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
        self.assertTrue((root / "registry" / "contexts.json").is_file())
        self.assertFalse(transaction_path(root).exists())

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
        self.assertTrue((root / "registry" / "contexts.json").is_file())

    def test_a_recovery_journal_pointing_outside_the_registry_is_refused(self) -> None:
        root = self.repo / "memory"
        transaction_path(root).write_text(
            json.dumps(
                {
                    "backup": "../.domain-registry-backup-abc",
                    "staging": ".domain-registry-stage-abc",
                }
            ),
            encoding="utf-8",
        )
        with self.assertRaisesRegex(ValueError, "unsafe path"):
            recover_interrupted_update(root, False)

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

    def test_an_obligation_source_type_error_names_the_types_that_are_allowed(
        self,
    ) -> None:
        package = self.written_package(
            test_obligations={
                "requirement_id": "REQ-1",
                "proposal_id": "PRO-1",
                "obligations": [
                    {
                        "id": "OB-1",
                        "source_type": "architecture",
                        "source_id": "AC-1",
                        "expected_outcome": "A negative total is rejected.",
                        "check": "test command",
                        "status": "executed",
                    }
                ],
            }
        )
        errors = validate_change_package(package, self.repo / "memory")
        named = next(
            error for error in errors if error.startswith("test obligation OB-1")
        )
        self.assertIn("'architecture'", named)
        for allowed in ("acceptance-criterion", "rule", "contract", "invariant"):
            self.assertIn(allowed, named)

    def test_an_incomplete_source_id_is_reported_separately_from_an_unknown_source_type(
        self,
    ) -> None:
        package = self.written_package(
            test_obligations={
                "requirement_id": "REQ-1",
                "proposal_id": "PRO-1",
                "obligations": [
                    {
                        "id": "OB-1",
                        "source_type": "acceptance-criterion",
                        "source_id": "<stable-id>",
                        "expected_outcome": "A negative total is rejected.",
                        "check": "test command",
                        "status": "executed",
                    }
                ],
            }
        )
        errors = validate_change_package(package, self.repo / "memory")
        named = next(
            error for error in errors if error.startswith("test obligation OB-1")
        )
        self.assertIn("incomplete source_id", named)
        self.assertNotIn("source_type", named)

    def test_an_uncovered_acceptance_criterion_is_told_what_to_add(self) -> None:
        package = self.written_package(
            test_obligations={
                "requirement_id": "REQ-1",
                "proposal_id": "PRO-1",
                "obligations": [
                    {
                        "id": "OB-1",
                        "source_type": "invariant",
                        "source_id": "PRO-1",
                        "expected_outcome": "A negative total is rejected.",
                        "check": "test command",
                        "status": "executed",
                    }
                ],
            }
        )
        errors = validate_change_package(package, self.repo / "memory")
        named = next(
            error for error in errors if error.startswith("acceptance criterion AC-1")
        )
        self.assertIn("source_type is acceptance-criterion", named)
        self.assertIn("source_id is AC-1", named)

    def context_declaring(self, asset: str, reason: str) -> None:
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "confirmed_absences": [{"asset": asset, "reason": reason}],
                },
                {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
            ],
        )

    def test_an_examined_context_leaves_the_gap_list_but_an_unexamined_one_stays(
        self,
    ) -> None:
        self.context_declaring(
            "aggregates",
            "Invariants are thin here; a transaction script is the declared pattern.",
        )
        report = coverage(self.repo / "memory")
        self.assertEqual(report["gaps"]["without_aggregate"], ["billing"])
        self.assertEqual(report["confirmed_absent"]["without_aggregate"], ["orders"])

    def test_an_absence_confirmed_for_one_asset_does_not_answer_another(self) -> None:
        self.context_declaring("aggregates", "Invariants are thin here.")
        report = coverage(self.repo / "memory")
        self.assertIn("orders", report["gaps"]["without_rule"])
        self.assertEqual(report["confirmed_absent"]["without_rule"], [])

    def test_a_confirmed_absence_without_a_reason_is_rejected(self) -> None:
        self.context_declaring("aggregates", "   ")
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(any("without a reason" in error for error in errors), errors)

    def test_a_confirmed_absence_the_records_contradict_is_rejected(self) -> None:
        self.context_declaring(
            "rules", "No business rule is written down for this context."
        )
        self.seed(
            "rules.json",
            [
                {
                    "id": "order-total",
                    "contexts": ["orders"],
                    "statement": "An order total is non-negative.",
                }
            ],
        )
        errors = validate(self.repo / "memory", None, False)
        self.assertTrue(
            any("rules records name it" in error for error in errors), errors
        )

    def test_a_confirmed_absence_of_an_unknown_asset_names_the_ones_allowed(
        self,
    ) -> None:
        self.context_declaring("events", "Events are elsewhere.")
        errors = validate(self.repo / "memory", None, False)
        named = next(error for error in errors if "unknown asset" in error)
        for allowed in ("aggregates", "contracts", "rules", "vocabulary"):
            self.assertIn(allowed, named)

    def submitted_package(self) -> Path:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        return package

    def proposal_in(self, package: Path) -> dict:
        return json.loads(
            (package / "domain-change-proposal.json").read_text(encoding="utf-8")
        )

    def test_superseding_keeps_the_status_the_proposal_died_in(self) -> None:
        package = self.submitted_package()
        supersede_proposal(
            package,
            "The event catalogue shows the collaboration runs the other way.",
            None,
        )
        proposal = self.proposal_in(package)
        self.assertEqual(proposal["status"], "superseded")
        self.assertEqual(proposal["superseded_from_status"], "submitted")
        self.assertIn("other way", proposal["superseded_reason"])

    def test_a_superseded_proposal_can_name_what_replaced_it(self) -> None:
        package = self.submitted_package()
        supersede_proposal(
            package, "Replaced by a proposal with the corrected direction.", "PRO-2"
        )
        self.assertEqual(self.proposal_in(package)["superseded_by"], "PRO-2")

    def test_superseding_without_a_reason_is_refused(self) -> None:
        package = self.submitted_package()
        with self.assertRaisesRegex(ValueError, "requires a reason"):
            supersede_proposal(package, "   ", None)
        self.assertEqual(self.proposal_in(package)["status"], "submitted")

    def test_a_superseded_proposal_cannot_be_superseded_again(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "Overtaken by the event catalogue.", None)
        with self.assertRaisesRegex(ValueError, "create a new draft instead"):
            supersede_proposal(package, "Overtaken twice.", None)

    def test_a_superseded_proposal_no_longer_accepts_approval(self) -> None:
        package = self.submitted_package()
        supersede_proposal(package, "Overtaken by the event catalogue.", None)
        with self.assertRaisesRegex(ValueError, "only for a submitted proposal"):
            record_approval(
                package, "domain-owner", "reviewer", "entire proposal", None
            )

    def stored_policy(self) -> dict:
        return json.loads(
            (self.repo / "memory" / "domain-memory-policy.json").read_text(
                encoding="utf-8"
            )
        )

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

    def test_a_policy_can_leave_local_draft_only_for_external_review(self) -> None:
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone until a reviewer exists.",
        )
        with self.assertRaisesRegex(ValueError, "requires the verifier"):
            amend_policy(
                self.repo / "memory",
                "review_mode",
                "scm-verified",
                "A reviewer exists now.",
            )
        self.assertEqual(self.stored_policy()["review_mode"], "local-draft-only")
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "scm-verified",
            "A reviewer exists now.",
            verifier="github-pr",
        )
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")
        self.assertEqual(
            self.stored_policy()["review_governance"]["verifier"], "github-pr"
        )

    def test_a_repository_without_ci_can_adopt_and_rotate_a_signer(self) -> None:
        root = self.repo / "memory"
        amend_policy(
            root, "authorized_signers", "SHA256:first", "Register the maintainer key."
        )
        amend_policy(root, "review_trigger", "git-push", "Signing is enforced on push.")
        amend_policy(
            root,
            "review_verifier",
            "git-signed-commit",
            "No CI reviewer exists, so the maintainer signs instead.",
        )
        governance = self.stored_policy()["review_governance"]
        self.assertEqual("git-signed-commit", governance["verifier"])
        self.assertEqual(["SHA256:first"], governance["authorized_signers"])
        amend_policy(
            root, "authorized_signers", "SHA256:second", "Rotate a compromised key."
        )
        self.assertEqual(
            ["SHA256:second"],
            self.stored_policy()["review_governance"]["authorized_signers"],
        )
        with self.assertRaisesRegex(ValueError, "amended policy is invalid"):
            amend_policy(root, "authorized_signers", "   ", "Leave nobody able to sign.")
        self.assertEqual(
            ["SHA256:second"],
            self.stored_policy()["review_governance"]["authorized_signers"],
        )

    def test_amending_without_a_reason_leaves_the_policy_alone(self) -> None:
        with self.assertRaisesRegex(ValueError, "requires a reason"):
            amend_policy(self.repo / "memory", "review_mode", "local-draft-only", "   ")
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")

    def test_an_unamendable_field_names_the_ones_that_are(self) -> None:
        with self.assertRaises(ValueError) as raised:
            amend_policy(self.repo / "memory", "limits", "0", "Loosen the limits.")
        for amendable in (
            "review_mode",
            "storage_mode",
            "data_classification",
            "source_authority",
        ):
            self.assertIn(amendable, str(raised.exception))

    def test_an_invalid_value_is_refused_before_anything_is_written(self) -> None:
        with self.assertRaisesRegex(ValueError, "amended policy is invalid"):
            amend_policy(
                self.repo / "memory", "review_mode", "whatever-i-like", "Try it."
            )
        self.assertEqual(self.stored_policy()["review_mode"], "scm-verified")

    def test_the_registry_revision_moves_when_the_policy_moves(self) -> None:
        before = registry_digest(self.repo / "memory")
        amend_policy(
            self.repo / "memory",
            "review_mode",
            "local-draft-only",
            "Drafting alone for now.",
        )
        self.assertNotEqual(before, registry_digest(self.repo / "memory"))

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

    def cited_file(self) -> Path:
        path = self.repo / "docs" / "adr" / "boundaries.md"
        path.parent.mkdir(parents=True, exist_ok=True)
        rows = [
            "# Boundaries",
            "Orders owns pricing.",
            "Billing owns invoices.",
            "No trailing newline",
        ]
        path.write_text(chr(10).join(rows), encoding="utf-8")
        return path

    def test_a_built_citation_verifies_as_current(self) -> None:
        self.cited_file()
        reference = citation(self.repo, "docs/adr/boundaries.md", 2, 3)
        self.assertEqual(verify(reference, self.repo)["status"], "current")

    def test_a_citation_covers_only_the_lines_it_names(self) -> None:
        path = self.cited_file()
        reference = citation(self.repo, "docs/adr/boundaries.md", 2, 3)
        whole_file = digest(path.read_bytes())
        self.assertEqual(reference["content_sha256"], whole_file)
        self.assertNotEqual(reference["excerpt_sha256"], whole_file)
        hand_written = reference | {"excerpt_sha256": whole_file}
        self.assertEqual(verify(hand_written, self.repo)["status"], "stale")

    def test_the_last_line_can_be_cited_but_the_one_after_it_cannot(self) -> None:
        self.cited_file()
        self.assertEqual(
            citation(self.repo, "docs/adr/boundaries.md", 4, 4)["lines"]["end"], 4
        )
        with self.assertRaisesRegex(ValueError, "has 4 lines"):
            citation(self.repo, "docs/adr/boundaries.md", 4, 5)

    def test_a_citation_starting_before_the_first_line_is_refused(self) -> None:
        self.cited_file()
        self.assertEqual(
            citation(self.repo, "docs/adr/boundaries.md", 1, 1)["lines"]["start"], 1
        )
        with self.assertRaisesRegex(ValueError, "1 <= start <= end"):
            citation(self.repo, "docs/adr/boundaries.md", 0, 1)

    def test_a_citation_ending_before_it_starts_is_refused(self) -> None:
        self.cited_file()
        with self.assertRaisesRegex(ValueError, "1 <= start <= end"):
            citation(self.repo, "docs/adr/boundaries.md", 3, 2)

    def test_a_citation_reaching_outside_the_repository_is_refused(self) -> None:
        (self.repo.parent / "outside.md").write_text("elsewhere", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "escapes the repository"):
            citation(self.repo, "../outside.md", 1, 1)

    def test_citing_a_file_that_is_not_there_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "no such file to cite"):
            citation(self.repo, "docs/adr/missing.md", 1, 1)

    def test_the_cite_command_reports_the_kind_of_source_the_lines_came_from(
        self,
    ) -> None:
        self.cited_file()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr"]),
        )
        result = self.run_cli(
            "cite",
            "--repo-root",
            str(self.repo),
            "--path",
            "docs/adr/boundaries.md",
            "--start",
            "2",
            "--end",
            "2",
            "--registry-root",
            str(self.repo / "memory"),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["source_kind"], "decisions")

    def confirmed_corpus(self) -> None:
        self.cited_file()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr"]),
        )

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
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record),
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
            upsert_candidate(self.repo / "memory", self.repo, "contexts", record), []
        )


if __name__ == "__main__":
    unittest.main()
