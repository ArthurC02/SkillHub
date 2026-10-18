from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.evidence import citation
from test_support import RegistryTestCase

STALE_LINE = (
    "rules/cited-rule: src/a.py:1-1 changed; check that its statement still holds; recite only if it does, "
    "otherwise amend it"
)


class VerifyStagedEvidenceTest(RegistryTestCase):
    def setUp(self) -> None:
        super().setUp()
        (self.repo / "src").mkdir()
        (self.repo / "src" / "a.py").write_text("one\ntwo\n", encoding="utf-8")
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.seed("rules.json", [{
            "id": "cited-rule", "contexts": ["orders"], "statement": "S.",
            "evidence": [citation(self.repo, "src/a.py", 1, 1)],
        }])
        self.commit_all("cite")

    def write(self, text: str) -> None:
        (self.repo / "src" / "a.py").write_text(text, encoding="utf-8")

    def verify_staged(self, *paths: str, repo_root: Path | None = None) -> tuple[int, dict | str, str]:
        result = self.run_cli(
            "verify-evidence", "--registry-root", str(self.repo / "memory"),
            "--repo-root", str(repo_root or self.repo), "--staged", *(f"--path={path}" for path in paths),
        )
        report = json.loads(result.stdout) if result.stdout.startswith("{") else result.stdout
        return result.returncode, report, result.stderr

    def test_a_weakened_line_in_the_index_is_stale_while_the_disk_holds_the_original(self) -> None:
        self.write("changed\ntwo\n")
        self.git("add", "src/a.py")
        self.write("one\ntwo\n")
        code, report, stderr = self.verify_staged("src/a.py")
        self.assertEqual(
            (1, "stale", {"citations": 1, "exists": True}, f"{STALE_LINE}\nverify-evidence failed: 1 stale citation\n"),
            (code, report["results"][0]["status"], report["paths"]["src/a.py"], stderr),
        )

    def test_the_original_line_in_the_index_is_current_while_the_disk_is_weakened(self) -> None:
        self.write("changed\ntwo\n")
        code, report, stderr = self.verify_staged("src/a.py")
        self.assertEqual((0, "current", ""), (code, report["results"][0]["status"], stderr))

    def test_a_cited_file_on_disk_but_not_in_the_index_is_missing(self) -> None:
        self.git("rm", "-q", "--cached", "src/a.py")
        code, report, stderr = self.verify_staged("src/a.py")
        self.assertEqual(
            (1, {"status": "missing", "path": "src/a.py"}, {"citations": 1, "exists": False},
             "rules/cited-rule: src/a.py is missing\nverify-evidence failed: 1 missing citation\n"),
            (code, {key: report["results"][0][key] for key in ("status", "path")}, report["paths"]["src/a.py"], stderr),
        )

    def status_when_the_index_holds_instead(self, entry: str) -> str:
        self.git("rm", "-q", "--cached", "src/a.py")
        subprocess.run(
            ["git", "update-index", "--index-info"], cwd=self.repo, input=f"{entry}\n".encode(), check=True,
        )
        self.assertEqual(entry, self.git("ls-files", "--stage", "--", "src"))
        _, report, _ = self.verify_staged("src/a.py")
        return report["results"][0]["status"]

    def test_a_submodule_at_the_cited_path_in_the_index_is_missing(self) -> None:
        commit = self.git("rev-parse", "HEAD")
        self.assertEqual("missing", self.status_when_the_index_holds_instead(f"160000 {commit} 0\tsrc/a.py"))

    def test_a_cited_file_held_only_as_a_conflict_stage_is_missing(self) -> None:
        blob = self.git("hash-object", "-w", "src/a.py")
        self.assertEqual("missing", self.status_when_the_index_holds_instead(f"100644 {blob} 2\tsrc/a.py"))

    def test_a_directory_at_the_cited_path_in_the_index_is_missing(self) -> None:
        blob = self.git("hash-object", "-w", "src/a.py")
        self.assertEqual("missing", self.status_when_the_index_holds_instead(f"100644 {blob} 0\tsrc/a.py/inner.py"))

    def test_an_index_git_cannot_read_is_an_error_not_a_missing_file(self) -> None:
        with tempfile.TemporaryDirectory() as elsewhere:
            outside = Path(elsewhere).resolve()
            (outside / "src").mkdir()
            (outside / "src" / "a.py").write_text("one\ntwo\n", encoding="utf-8")
            code, report, _ = self.verify_staged("src/a.py", repo_root=outside)
        self.assertEqual(1, code)
        self.assertTrue(report.startswith(f"ERROR: could not read the Git index of {outside}: "), report)

    def test_without_staged_the_disk_is_verified_and_the_message_is_unchanged(self) -> None:
        self.write("changed\ntwo\n")
        result = self.run_cli(
            "verify-evidence", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--path", "src/a.py",
        )
        self.assertEqual(
            (1, "stale", "verify-evidence failed: 1 stale citations; run recite on each to cite its current lines\n"),
            (result.returncode, json.loads(result.stdout)["results"][0]["status"], result.stderr),
        )


if __name__ == "__main__":
    unittest.main()
