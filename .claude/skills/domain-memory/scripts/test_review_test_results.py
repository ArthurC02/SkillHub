from __future__ import annotations

import hashlib
import json
import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.hitl import ObligationRun, no_test_passed, record_test_result, submit_proposal
from test_support import RegistryTestCase


class ReviewTestResultsTest(RegistryTestCase):
    def test_a_passing_run_replaces_the_obligation_result_with_its_output_digest(self) -> None:
        package = self.draft_package()
        self.declare_check(package, self.run_writing("ok"))
        record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit"), 60)
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual(
            ("passed", 0, "unit", "sha256:" + hashlib.sha256(b"ok").hexdigest()),
            (result["status"], result["exit_code"], result["command_profile"], result["output_sha256"]),
        )

    def test_a_recorded_command_passes_shell_characters_through_unchanged(self) -> None:
        package = self.draft_package()
        echo = "import sys; sys.stdout.write(sys.argv[1])"
        self.declare_check(package, f'"{sys.executable}" -c "{echo}" ^(A|B)$')
        record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit"), 60)
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual("sha256:" + hashlib.sha256(b"^(A|B)$").hexdigest(), result["output_sha256"])

    def test_a_check_that_gives_no_result_in_time_records_nothing(self) -> None:
        package = self.draft_package()
        before = (package / "evidence-bundle.json").read_bytes()
        sleeper = f'"{sys.executable}" -c "import time; time.sleep(5)"'
        with self.assertRaisesRegex(ValueError, "^" + re.escape(sleeper) + " gave no result within 1 seconds; nothing recorded$"):
            self.declare_check(package, sleeper)
            record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit"), 1)
        self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())

    def test_a_command_that_cannot_start_records_nothing(self) -> None:
        package = self.draft_package()
        before = (package / "evidence-bundle.json").read_bytes()
        with self.assertRaisesRegex(ValueError, "could not start: .*runs as written, with no shell and no aliases"):
            self.declare_check(package, "no-such-program-for-domain-memory --version")
            record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit"), 60)
        self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())

    def test_a_policy_without_approved_profiles_accepts_any_profile(self) -> None:
        package = self.draft_package()
        policy_path = self.repo / "memory" / "domain-memory-policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        policy["approved_command_profiles"] = []
        policy_path.write_text(json.dumps(policy), encoding="utf-8")
        self.declare_check(package, self.run_writing("ok"))
        record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "release"), 60)
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual("release", result["command_profile"])

    def test_a_failing_run_and_an_unknown_obligation_or_profile_record_nothing(self) -> None:
        package = self.draft_package()
        before = (package / "evidence-bundle.json").read_bytes()
        cases = [
            ("OB-1", self.run_writing("broken", 3), "unit", "exited 3; only a passing run is recorded"),
            ("OB-9", self.run_writing("ok"), "unit", "no obligation OB-9"),
            ("OB-1", self.run_writing("ok"), "release", "release is not approved"),
        ]
        for obligation, command, profile, refusal in cases:
            with self.subTest(refusal=refusal):
                with self.assertRaisesRegex(ValueError, refusal):
                    self.declare_check(package, command)
                    record_test_result(package, self.repo / "memory", self.repo, ObligationRun(obligation, profile), 60)
                self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())

    def test_a_test_run_in_a_subdirectory_records_where_it_ran(self) -> None:
        package = self.draft_package()
        (self.repo / "apps" / "web").mkdir(parents=True)
        command = self.run_writing("ok").replace(
            "sys.stdout.write('ok')", "import os; sys.stdout.write(os.path.basename(os.getcwd()))"
        )
        self.declare_check(package, command)
        record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit", cwd="apps/web"), 60)
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        self.assertEqual(
            ("apps/web", "sha256:" + hashlib.sha256(b"web").hexdigest()),
            (result["working_directory"], result["output_sha256"]),
        )

    def test_a_test_run_refuses_a_directory_outside_the_repository_or_missing(self) -> None:
        package = self.draft_package()
        before = (package / "evidence-bundle.json").read_bytes()
        for cwd in ("..", "missing"):
            with self.subTest(cwd=cwd), self.assertRaisesRegex(ValueError, "--cwd must name a directory inside"):
                self.declare_check(package, self.run_writing("ok"))
                record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit", cwd=cwd), 60)
        self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())

    def recorded(self, package: Path) -> dict:
        evidence = json.loads((package / "evidence-bundle.json").read_text(encoding="utf-8"))
        [result] = [entry for entry in evidence["test_results"] if entry["obligation_id"] == "OB-1"]
        return result

    def test_a_run_records_the_obligation_check_and_its_output_digest(self) -> None:
        package = self.draft_package()
        check = self.declare_check(package, self.run_writing("ok"))
        record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit"), 60)
        self.assertEqual(
            (check, "sha256:" + hashlib.sha256(b"ok").hexdigest()),
            (self.recorded(package)["evidence"], self.recorded(package)["output_sha256"]),
        )

    def test_test_results_are_recorded_only_before_verification(self) -> None:
        package = self.draft_package()
        check = self.declare_check(package, self.run_writing("ok"))
        proposal_path = package / "domain-change-proposal.json"
        for status, records in (
            ("draft", True), ("submitted", True), ("verified", False), ("approved", False), ("applied", False),
            ("superseded", False),
        ):
            with self.subTest(status):
                proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
                proposal["status"] = status
                proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
                evidence_path = package / "evidence-bundle.json"
                evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
                evidence["test_results"] = []
                evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
                run = ObligationRun("OB-1", "unit")
                if records:
                    record_test_result(package, self.repo / "memory", self.repo, run, 60)
                else:
                    with self.assertRaisesRegex(
                        ValueError,
                        f"^the proposal is {status}: its test results are what verify-proposal checked, "
                        "so they stay as they are; redraft it to record new ones$",
                    ):
                        record_test_result(package, self.repo / "memory", self.repo, run, 60)
                recorded = json.loads(evidence_path.read_text(encoding="utf-8"))["test_results"]
                self.assertEqual([check] if records else [], [entry["evidence"] for entry in recorded])

    def test_a_passing_run_in_which_no_test_passed_records_nothing(self) -> None:
        package = self.draft_package()
        self.declare_check(package, self.run_writing("Ran 0 tests in 0.000s\n\nNO TESTS RAN\n"))
        before = (package / "evidence-bundle.json").read_bytes()
        with self.assertRaisesRegex(ValueError, "exited 0, but unittest ran no test that was not skipped; nothing recorded"):
            record_test_result(package, self.repo / "memory", self.repo, ObligationRun("OB-1", "unit"), 60)
        self.assertEqual(before, (package / "evidence-bundle.json").read_bytes())

    def with_recorded_evidence(self, evidence: str) -> Path:
        package = self.draft_package()
        self.declare_check(package, "go test -v ./orders/...")
        path = package / "evidence-bundle.json"
        bundle = json.loads(path.read_text(encoding="utf-8"))
        bundle["test_results"][0]["evidence"] = evidence
        path.write_text(json.dumps(bundle), encoding="utf-8")
        return package

    def test_verification_refuses_a_test_result_that_ran_a_command_other_than_the_check(self) -> None:
        package = self.with_recorded_evidence("go test -v ./billing/...")
        with self.assertRaisesRegex(
            ValueError,
            "the test result of OB-1 ran 'go test -v ./billing/...', not the obligation's check 'go test -v ./orders/...'",
        ):
            self.approve_and_finalize(package)

    def assert_verified_with(self, evidence: str) -> None:
        package = self.with_recorded_evidence(evidence)
        self.approve_and_finalize(package)
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        self.assertEqual("approved", proposal["status"])

    def test_verification_accepts_the_check_written_with_other_spacing(self) -> None:
        self.assert_verified_with("go test  -v ./orders/...")

    def test_verification_accepts_a_link_to_the_ci_run(self) -> None:
        self.assert_verified_with("https://ci.example/runs/1")

    def test_submitting_refuses_a_go_test_check_that_cannot_report_its_tests(self) -> None:
        package = self.draft_package()
        self.declare_check(package, "go test -count=1 -run ^TestTotal$ ./orders/")
        with self.assertRaisesRegex(ValueError, "^obligations OB-1 run go test without -v or -json"):
            submit_proposal(package, self.repo / "memory", self.repo)
        proposal_path = package / "domain-change-proposal.json"
        self.assertEqual("draft", json.loads(proposal_path.read_text(encoding="utf-8"))["status"])
        self.declare_check(package, "go test -v -count=1 -run ^TestTotal$ ./orders/")
        submit_proposal(package, self.repo / "memory", self.repo)
        self.assertEqual("submitted", json.loads(proposal_path.read_text(encoding="utf-8"))["status"])

    def test_each_runner_says_when_no_test_passed(self) -> None:
        go_requires_v = (
            "go test without -v or -json does not say which tests passed, so a skipped test reads as a pass; add -v"
        )
        skipped_one = "no test passed: every test was skipped (1)"
        none_matched = "no test passed: no test matched"
        unittest_none = "unittest ran no test that was not skipped"
        for command, output, expected in (
            ("go test -v ./...", "=== RUN   TestA\n--- PASS: TestA (0.00s)\nok  p 0.1s\n", None),
            ("go test -v ./...", "=== RUN   TestA\n--- SKIP: TestA (0.00s)\nok  p 0.1s\n", skipped_one),
            ("go test -v ./...",
             "=== RUN   TestA\n    rule_test.go:12: DATABASE_URL is not set\n--- SKIP: TestA (0.00s)\n"
             "=== RUN   TestB\n    rule_test.go:30: DATABASE_URL is not set\n--- SKIP: TestB (0.00s)\nok  p 0.1s\n",
             "no test passed: every test was skipped (2), the first because DATABASE_URL is not set"),
            ("go test -v -run ^TestNothing$ ./...", "ok  p 0.1s\n", none_matched),
            ("go test -v=true ./...", "    --- PASS: TestA/sub (0.00s)\n", None),
            ("go test -test.v ./...", "--- PASS: TestA (0.00s)\n", None),
            ("go test ./...", "ok  p 0.1s\n", go_requires_v),
            ("go -C tools test ./...", "ok  p 0.1s\n", go_requires_v),
            ("C:/Go/bin/go.exe test ./...", "ok  p 0.1s\n", go_requires_v),
            ("go vet ./...", "", None),
            ("go test -json ./...", '{"Action":"pass","Package":"p","Test":"TestA","Elapsed":0}\n', None),
            ("go test -json ./...", '{"Action":"pass","Package":"p","Elapsed":0}\n', none_matched),
            ("go test -json ./...", '{"Action":"skip","Package":"p","Test":"TestA","Elapsed":0}\n', skipped_one),
            ("task test", "ok  p 0.1s [no tests to run]\n", "go test found no test to run"),
            ("task test", "?   p [no test files]\n", "go test found no test to run"),
            ("task test", "--- PASS: TestA (0.00s)\nok  a 0.1s\nok  b 0.1s [no tests to run]\n", None),
            ("python -m unittest", "Ran 0 tests in 0.000s\n\nNO TESTS RAN\n", unittest_none),
            ("python -m unittest", "Ran 2 tests in 0.1s\n\nOK (skipped=2)\n", unittest_none),
            ("python -m unittest", "Ran 2 tests in 0.1s\n\nOK (skipped=1)\n", None),
            ("python -m unittest", "Ran 1 test in 0.1s\n\nOK\n", None),
            ("pytest", "===== 3 skipped in 0.12s =====\n", "pytest passed no test: 3 skipped"),
            ("pytest -q", "3 deselected in 0.01s\n", "pytest passed no test: 3 deselected"),
            ("pytest", "===== 1 passed, 3 skipped in 0.12s =====\n", None),
            ("pytest", "== 2 passed in 0.1s ==\n== 1 skipped in 0.1s ==\n", "pytest passed no test: 1 skipped"),
            ("npm test", "anything at all\n", None),
        ):
            with self.subTest(f"{command}: {output!r}"):
                self.assertEqual(expected, no_test_passed(command.split(), output))


if __name__ == "__main__":
    unittest.main()
