import os
import pathlib
import shutil
import subprocess
import tempfile
import textwrap
import unittest


SCRIPT = pathlib.Path(__file__).with_name("gvisor-incident-issues.sh")
RUNTIME_WORKFLOW = pathlib.Path(__file__).parents[2] / ".github/workflows/runtime-scan-expiry.yml"
MOCK_GH = """#!/usr/bin/env bash
set -euo pipefail
if [ "$1 $2" = 'label list' ]; then
  exit 0
fi
if [ "$1 $2" = 'label create' ]; then
  exit 0
fi
if [ "$1 $2" = 'issue list' ]; then
  if [ "${MOCK_LIST_FAIL:-}" = 1 ]; then
    exit 2
  fi
  if [[ " $* " == *' --state all '* ]]; then
    cat "$EXISTING_FILE"
  elif [[ " $* " == *' --state open '* ]]; then
    cat "$OPEN_FILE"
  else
    exit 3
  fi
  exit 0
fi
if [ "$1 $2" = 'issue create' ]; then
  shift 2
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --title) title="$2" ;;
      --label) label="$2" ;;
      --assignee) assignee="$2" ;;
      --body) body="$2" ;;
    esac
    shift 2
  done
  printf '%s\\t%s\\t%s\\n' "$title" "$label" "$assignee" >> "$CALLS_FILE"
  printf '%s\\n' "$body" >> "$BODIES_FILE"
  exit 0
fi
exit 4
"""


class IncidentIssueFixture(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.root = pathlib.Path(self.scratch.name)
        self.bash = (
            pathlib.Path(r"C:\Program Files\Git\bin\bash.exe")
            if os.name == "nt"
            else shutil.which("bash")
        )
        if not self.bash:
            self.skipTest("bash is unavailable")
        mock = self.root / "gh"
        mock.write_text(MOCK_GH, encoding="utf-8")
        mock.chmod(0o755)
        for name in ("advisories.tsv", "relnotes.tsv", "existing", "open", "calls", "bodies"):
            (self.root / name).write_text("", encoding="utf-8")
        self.env = dict(os.environ)
        self.env.update(
            PATH=str(self.root) + os.pathsep + self.env["PATH"],
            INCIDENT_ASSIGNEE="operator",
            GITHUB_REPOSITORY="owner/SkillHub",
            GITHUB_RUN_ID="123",
            EXISTING_FILE=str(self.root / "existing"),
            OPEN_FILE=str(self.root / "open"),
            CALLS_FILE=str(self.root / "calls"),
            BODIES_FILE=str(self.root / "bodies"),
        )

    def run_script(self, **changes):
        env = dict(self.env)
        env.update(changes)
        return subprocess.run(
            [str(self.bash), str(SCRIPT)],
            cwd=self.root,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )

    def created(self):
        return [line.split("\t") for line in (self.root / "calls").read_text(encoding="utf-8").splitlines()]


class GvisorIncidentIssuesTest(IncidentIssueFixture):
    def test_distinct_advisories_and_release_get_distinct_p1_issues(self):
        (self.root / "advisories.tsv").write_text(
            "GHSA-111\tcritical\tFirst advisory\thttps://example.test/1\n"
            "GHSA-222\thigh\tSecond escape\thttps://example.test/2\n",
            encoding="utf-8",
        )
        (self.root / "relnotes.tsv").write_text("2026-10-07\thttps://example.test/release\n", encoding="utf-8")

        result = self.run_script(ESCAPE="1")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.created(),
            [
                ["gVisor security advisory GHSA-111", "sev/P1", "operator"],
                ["gVisor security advisory GHSA-222", "sev/P1", "operator"],
                ["gVisor release 2026-10-07 security review", "sev/P1", "operator"],
            ],
        )
        bodies = (self.root / "bodies").read_text(encoding="utf-8")
        self.assertIn("https://example.test/1", bodies)
        self.assertIn("Automatic action: no dispatch halt", bodies)
        self.assertIn("https://github.com/owner/SkillHub/actions/runs/123", bodies)

    def test_closed_incident_is_not_duplicated_on_retry(self):
        (self.root / "advisories.tsv").write_text(
            "GHSA-111\tcritical\tFirst advisory\thttps://example.test/1\n"
            "GHSA-222\tcritical\tSecond advisory\thttps://example.test/2\n",
            encoding="utf-8",
        )
        (self.root / "existing").write_text("gVisor security advisory GHSA-111\n", encoding="utf-8")

        result = self.run_script(ESCAPE="1")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.created(), [["gVisor security advisory GHSA-222", "sev/P1", "operator"]])

    def test_failed_issue_lookup_does_not_create_a_duplicate(self):
        (self.root / "advisories.tsv").write_text(
            "GHSA-111\tcritical\tFirst advisory\thttps://example.test/1\n", encoding="utf-8"
        )

        result = self.run_script(ESCAPE="1", MOCK_LIST_FAIL="1")

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.created(), [])

    def test_baseline_drift_is_p3_and_does_not_claim_a_halt(self):
        result = self.run_script(FAIL="baseline is old")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.created(), [["gVisor baseline has drifted (SEC-002 P-04)", "sev/P3", "operator"]])
        bodies = (self.root / "bodies").read_text(encoding="utf-8")
        self.assertIn("Automatic action: no dispatch halt", bodies)


class RuntimeScanIssueTest(IncidentIssueFixture):
    def run_workflow_step(self, **changes):
        workflow = RUNTIME_WORKFLOW.read_text(encoding="utf-8")
        step = workflow.split("      - name: File the issue\n", 1)[1]
        script = textwrap.dedent(step.split("        run: |\n", 1)[1])
        env = dict(self.env)
        env.update(changes)
        return subprocess.run(
            [str(self.bash), "-c", script],
            cwd=self.root,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_scan_warning_is_p3_and_assigned(self):
        result = self.run_workflow_step(PROBLEM="scan expires in 3 days")

        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(
            self.created(),
            [["Runtime image scan is expiring or failing (SBX-002 I-04)", "sev/P3", "operator"]],
        )
        bodies = (self.root / "bodies").read_text(encoding="utf-8")
        self.assertIn("cannot establish whether an expired digest is still referenced", bodies)
        self.assertIn("https://github.com/owner/SkillHub/actions/runs/123", bodies)

    def test_failed_issue_lookup_does_not_create_a_scan_issue(self):
        result = self.run_workflow_step(PROBLEM="scan expires in 3 days", MOCK_LIST_FAIL="1")

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.created(), [])

    def test_existing_open_scan_issue_is_not_duplicated(self):
        (self.root / "open").write_text(
            "Runtime image scan is expiring or failing (SBX-002 I-04)\n", encoding="utf-8"
        )

        result = self.run_workflow_step(PROBLEM="scan expires in 3 days")

        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(self.created(), [])


if __name__ == "__main__":
    unittest.main()
