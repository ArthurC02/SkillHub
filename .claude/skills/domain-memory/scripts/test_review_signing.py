from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.attestations import verify_git_signed_commit, verify_scm
from domain_registry.hitl import (
    attest_signed_commit,
    finalize_proposal,
    record_approval,
    submit_proposal,
    verify_proposal,
)
from domain_registry.git_hooks import governance_readiness, install_pre_push_hook
from domain_registry.signing import SIGNING_KEY_ENV, init_signing_key
from test_support import RegistryTestCase, amend_policy


class ReviewSigningTest(RegistryTestCase):
    def signing_for_the_registry(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            key = init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["review_governance"].update(
            verifier="git-signed-commit", trigger="git-push", authorized_signers=[key["fingerprint"]]
        )
        policy_path.write_text(json.dumps(policy), encoding="utf-8")

    def test_one_governance_call_checks_every_commit_it_reads_and_names_the_one_that_fails(self) -> None:
        self.signing_for_the_registry()
        first = self.sign_a_commit_changing("memory/a.txt")
        second = self.sign_a_commit_changing("memory/b.txt")
        outside = self.sign_a_commit_changing("src/c.txt")
        verify = ("verify-git-governance", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo))
        for given, stdin, expected in (
            (("--stdin",), f"{first}\n{second}\n", (0, "Git governance is valid for 2 commits.", "")),
            (("--commit", first), None, (0, "Git governance is valid for 1 commit.", "")),
            (("--stdin",), f"{first}\n{outside}\n",
             (1, "", f"ERROR: {outside}: Git commit does not change Domain Memory files")),
            (("--stdin",), "", (1, "", "ERROR: no commit to verify")),
        ):
            with self.subTest(expected[1] or expected[2]):
                completed = self.run_cli(*verify, *given, stdin=stdin)
                self.assertEqual(expected, (completed.returncode, completed.stdout.strip(), completed.stderr.strip()))
        (self.repo / "memory" / "changes" / "a").mkdir(parents=True)
        (self.repo / "memory" / "changes" / "a" / "domain-change-proposal.json").write_text(
            json.dumps({"proposal_id": "PRO-a", "status": "approved"}), encoding="utf-8"
        )
        self.assertTrue(
            self.run_cli(*verify, "--commit", first).stderr.startswith("NOTE: 1 approved Change Package not applied: PRO-a.")
        )

    def test_the_pre_push_hook_reports_a_push_of_several_commits_once(self) -> None:
        self.signing_for_the_registry()
        base = self.commit_everything("selects signed commits")
        self.sign_a_commit_changing("memory/a.txt")
        tip = self.sign_a_commit_changing("memory/b.txt")
        outside = self.sign_a_commit_changing("src/c.txt")
        hook = install_pre_push_hook(self.repo / "memory", self.repo, Path(__file__).resolve().parent / "registry_tools.py")
        for remote, local, expected in ((base, tip, "Git governance is valid for 2 commits."), (tip, outside, "")):
            with self.subTest(expected):
                pushed = subprocess.run(
                    ["sh", str(hook), "origin", "unused"], cwd=self.repo,
                    input=f"refs/heads/main {local} refs/heads/main {remote}\n", capture_output=True, text=True, check=False,
                )
                self.assertEqual((0, expected), (pushed.returncode, pushed.stdout.strip()), pushed.stderr)

    def test_attesting_a_signed_commit_that_carries_the_package_lets_it_finalize(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            key = init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["review_governance"].update(
            verifier="git-signed-commit", trigger="git-push", authorized_signers=[key["fingerprint"]]
        )
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        unsigned = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo, capture_output=True, text=True, check=True
        ).stdout.strip()
        signed_before = self.commit_everything("signs the policy before the package", signed=True)
        package = self.draft_package()
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        with self.assertRaisesRegex(ValueError, "cannot attest this proposal: Git commit signature is invalid"):
            attest_signed_commit(package, self.repo / "memory", self.repo, unsigned)
        with self.assertRaisesRegex(ValueError, "does not carry this Change Package"):
            attest_signed_commit(package, self.repo / "memory", self.repo, signed_before)
        signed = self.commit_everything("adds the Change Package", signed=True)
        self.assertEqual(signed, attest_signed_commit(package, self.repo / "memory", self.repo, signed[:12]))
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual(
            {"provider": "git-signed-commit", "commit": signed, "status": "approved",
             "proposal_revision": 1, "base_registry_revision": proposal["base_registry_revision"]},
            evidence["scm_attestation"],
        )
        verify_proposal(package, self.repo / "memory", self.repo)
        finalize_proposal(package, self.repo / "memory", self.repo)
        finalized = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("approved", finalized["status"])

    def test_attesting_needs_the_signed_commit_verifier_and_a_submitted_proposal(self) -> None:
        package = self.draft_package()
        with self.assertRaisesRegex(ValueError, "this policy uses github-pr"):
            attest_signed_commit(package, self.repo / "memory", self.repo, "HEAD")
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["review_governance"].update(
            verifier="git-signed-commit", trigger="git-push", authorized_signers=["probe@example.com"]
        )
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "submit the proposal first"):
            attest_signed_commit(package, self.repo / "memory", self.repo, "HEAD")

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

    def test_a_rejected_commit_says_whether_it_lost_its_signature_or_fails_to_verify(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        registry_root = self.repo / "docs" / "domain-memory"
        signed = self.sign_a_domain_memory_commit("signed")
        (registry_root / "note.txt").write_text("unsigned", encoding="utf-8")
        unsigned = self.commit_everything("unsigned")
        [lost] = verify_git_signed_commit({"commit": unsigned}, self.repo, registry_root, ["probe@example.com"])
        self.assertIn(f"Git commit signature is invalid: {unsigned} carries no signature", lost)
        self.assertIn("git pull --no-rebase", lost)
        subprocess.run(["git", "config", "--unset", "gpg.ssh.allowedSignersFile"], cwd=self.repo, check=True)
        [failing] = verify_git_signed_commit({"commit": signed}, self.repo, registry_root, ["probe@example.com"])
        self.assertRegex(
            failing,
            rf"^Git commit signature is invalid: {signed} is signed, but the signature does not verify \(.*allowedSignersFile.*\)$",
        )

    def test_a_signature_program_that_cannot_run_is_reported_as_not_checked(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")
        signed = self.sign_a_domain_memory_commit("signed")
        subprocess.run(["git", "config", "gpg.ssh.program", "no-such-keygen"], cwd=self.repo, check=True)
        [unchecked] = verify_git_signed_commit(
            {"commit": signed}, self.repo, self.repo / "docs" / "domain-memory", ["probe@example.com"]
        )
        self.assertRegex(
            unchecked,
            r"^Git commit signature was not checked: cannot (spawn|run) no-such-keygen: No such file or directory\. "
            r"Install the signing program, or point gpg\.ssh\.program or gpg\.program at it, and verify again$",
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

    def test_key_material_with_crlf_line_endings_and_no_final_newline_rebuilds(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            first = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        material = (self.repo / "keys" / "signing-key").read_text(encoding="utf-8")
        pasted = material.rstrip("\n").replace("\n", "\r\n")
        elsewhere = self.repo / "elsewhere"
        subprocess.run(["git", "init", "-q", str(elsewhere)], check=True)
        with patch.dict("os.environ", {SIGNING_KEY_ENV: pasted}):
            second = init_signing_key(
                elsewhere, "probe@example.com", elsewhere / "keys" / "signing-key"
            )
        self.assertEqual("environment", second["source"])
        self.assertEqual(first["fingerprint"], second["fingerprint"])

    def test_unreadable_key_material_leaves_no_key_behind(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        label = "PRIVATE KEY"
        material = f"-----BEGIN OPENSSH {label}-----\nnot a key\n"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: material}):
            with self.assertRaisesRegex(ValueError, "readable OpenSSH private key"):
                init_signing_key(self.repo, "probe@example.com", key_file)
        self.assertFalse(key_file.exists())

    def test_the_reported_result_never_carries_private_key_material(self) -> None:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            result = init_signing_key(
                self.repo, "probe@example.com", self.repo / "keys" / "signing-key"
            )
        self.assertNotIn("PRIVATE KEY", json.dumps(result))

    def test_no_line_of_the_private_key_body_appears_in_the_reported_result(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            result = init_signing_key(self.repo, "probe@example.com", key_file)
        body = [
            line for line in key_file.read_text(encoding="utf-8").splitlines()
            if line and not line.startswith("-----")
        ]
        serialized = json.dumps(result)
        self.assertTrue(body)
        self.assertEqual([], [line for line in body if line in serialized])

    def test_a_second_call_reuses_the_existing_key(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            first = init_signing_key(self.repo, "probe@example.com", key_file)
            second = init_signing_key(self.repo, "probe@example.com", key_file)
        self.assertEqual(
            ("generated", "reused", first["fingerprint"]),
            (first["source"], second["source"], second["fingerprint"]),
        )

    def test_a_blank_principal_is_refused_and_creates_no_key(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        for principal in ("", "   ", None):
            with self.subTest(principal=principal), patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
                with self.assertRaises(ValueError) as refused:
                    init_signing_key(self.repo, principal, key_file)
                self.assertEqual(
                    "a signing key needs a principal: the identity an approval is attributed to",
                    str(refused.exception),
                )
                self.assertFalse(key_file.exists())

    def test_force_without_the_environment_key_is_refused_and_leaves_the_key_untouched(self) -> None:
        key_file = self.repo / "keys" / "signing-key"
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            init_signing_key(self.repo, "probe@example.com", key_file)
            before = key_file.read_bytes()
            with self.assertRaises(ValueError) as refused:
                init_signing_key(self.repo, "probe@example.com", key_file, force=True)
        self.assertEqual(
            "--force replaces a key from DOMAIN_MEMORY_SIGNING_KEY, and DOMAIN_MEMORY_SIGNING_KEY "
            f"is not set; delete {key_file} yourself if you mean to discard it",
            str(refused.exception),
        )
        self.assertEqual(before, key_file.read_bytes())

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

    def test_a_raw_gnupg_signer_is_accepted_when_listed_and_refused_when_not(self) -> None:
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

    def test_a_new_branch_skips_commits_the_remote_already_has(self) -> None:
        hook, _ = self.push_fixture()
        head = self.commit_file("elsewhere/readme.txt")
        result = self.run_pre_push(
            hook, f"refs/heads/feature {head} refs/heads/feature {'0' * 40}\n"
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

    def test_a_new_branch_still_verifies_its_own_unsigned_memory_commit(self) -> None:
        hook, _ = self.push_fixture()
        head = self.commit_file("memory/registry/extra.json")
        result = self.run_pre_push(
            hook, f"refs/heads/feature {head} refs/heads/feature {'0' * 40}\n"
        )
        self.assertNotEqual(0, result.returncode)
        self.assertIn("Git commit signature is invalid", result.stdout + result.stderr)

    def test_the_pre_push_hook_runs_python3_where_python_does_not_work(self) -> None:
        result = self.pushed_with(self.interpreters_on_path(python=False, python3=True))
        self.assertIn("Git commit signature is invalid", result.stdout + result.stderr)

    def test_the_pre_push_hook_runs_python_where_python3_does_not_work(self) -> None:
        result = self.pushed_with(self.interpreters_on_path(python=True, python3=False))
        self.assertIn("Git commit signature is invalid", result.stdout + result.stderr)

    def test_a_push_without_memory_commits_needs_no_interpreter(self) -> None:
        environment = self.interpreters_on_path(python=False, python3=False)
        hook, _ = self.push_fixture()
        head = self.commit_file("elsewhere/readme.txt")
        result = subprocess.run(
            ["sh", str(hook), "origin", "unused"], cwd=self.repo, env=environment,
            input=f"refs/heads/feature {head} refs/heads/feature {'0' * 40}\n",
            capture_output=True, text=True, check=False,
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

    def test_the_pre_push_hook_names_a_missing_interpreter(self) -> None:
        result = self.pushed_with(self.interpreters_on_path(python=False, python3=False))
        self.assertEqual(1, result.returncode)
        self.assertIn("no Python 3.10 or later on PATH as python3 or python", result.stderr)

    def test_an_existing_branch_checks_only_the_range_after_the_remote_tip(self) -> None:
        hook, remote_tip = self.push_fixture()
        head = self.commit_file("elsewhere/readme.txt")
        result = self.run_pre_push(
            hook, f"refs/heads/main {head} refs/heads/main {remote_tip}\n"
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

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


if __name__ == "__main__":
    unittest.main()
