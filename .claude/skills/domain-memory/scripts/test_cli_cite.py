from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.evidence import citation, digest
from domain_registry.sources import selected_source_map, write_source_map
from test_support import RegistryTestCase


class CliCiteTest(RegistryTestCase):
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
        self.assertEqual("", result.stderr)

    def test_citing_outside_the_confirmed_sources_says_review_will_refuse_it(self) -> None:
        self.cited_file()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr"]),
        )
        (self.repo / "db").mkdir()
        (self.repo / "db" / "queries.sql").write_text("select 1;\n", encoding="utf-8")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "db/queries.sql",
            "--start", "1", "--end", "1", "--registry-root", str(self.repo / "memory"),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        reference = json.loads(result.stdout)
        self.assertEqual("unclassified", reference["source_kind"])
        self.assertEqual({"path", "lines", "content_sha256", "excerpt_sha256", "source_kind"}, set(reference))
        self.assertIn("db/queries.sql is outside the confirmed sources", result.stderr)
        self.assertIn("ship-package refuses to review", result.stderr)

    def test_citing_a_selected_path_no_discovery_group_classifies_is_selected_and_silent(self) -> None:
        self.cited_file()
        (self.repo / "db").mkdir()
        (self.repo / "db" / "queries.sql").write_text("select 1;\n", encoding="utf-8")
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr", self.repo / "db"]),
        )
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "db/queries.sql",
            "--start", "1", "--end", "1", "--registry-root", str(self.repo / "memory"),
        )
        self.assertEqual(("selected", ""), (json.loads(result.stdout)["source_kind"], result.stderr))

    def test_cite_shows_the_first_and_last_cited_lines_when_asked(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\nthree\n")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "notes.md", "--start", "1", "--end", "3", "--show",
        )
        self.assertEqual(["notes.md:1: one", "notes.md: ... 1 lines not shown", "notes.md:3: three"], result.stderr.splitlines())

    def test_cite_shows_two_adjacent_lines_without_an_elision(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\n")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "notes.md", "--lines", "1-2", "--show",
        )
        self.assertEqual(["notes.md:1: one", "notes.md:2: two"], result.stderr.splitlines())

    def cite(self, *range_options: str) -> subprocess.CompletedProcess:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\nthree\n")
        return self.run_cli("cite", "--repo-root", str(self.repo), "--path", "notes.md", *range_options)

    def test_cite_takes_lines_as_recite_does(self) -> None:
        self.assertEqual(json.loads(self.cite("--start", "2", "--end", "3").stdout), json.loads(self.cite("--lines", "2-3").stdout))

    def test_cite_refuses_both_line_forms_or_neither(self) -> None:
        for options in (("--lines", "1-2", "--start", "1", "--end", "2"), ("--lines", "1-2", "--start", "1"), ("--start", "1"),
                        ("--function", "f", "--lines", "1-2"), ("--function", "f", "--start", "1", "--end", "2"), ()):
            with self.subTest(options):
                self.assertEqual("ERROR: cite takes --lines START-END, --start and --end, or --function NAME",
                                 self.cite(*options).stderr.strip())

    def test_cite_finds_the_span_of_a_go_func_or_python_def_by_name(self) -> None:
        (self.repo / "total_test.go").write_text(
            "package orders\n\nfunc TestTotalPos(t *testing.T) {}\n\nfunc TestTotal(t *testing.T) {\n"
            "\tif total() < 0 {\n\t\tt.Fatal(\"negative\")\n\t}\n}\n\nfunc (s *suite) TestMethod() {\n}\n",
            encoding="utf-8",
        )
        (self.repo / "test_total.py").write_text(
            "class TotalTest:\n    def test_total(\n        self,\n    ) -> None:\n        x = 1\n\n"
            "        assert x\n\n    def test_other(self) -> None:\n        pass\n",
            encoding="utf-8",
        )
        for path, name, lines in (
            ("total_test.go", "TestTotal", (5, 9)), ("total_test.go", "TestTotalPos", (3, 3)),
            ("total_test.go", "TestMethod", (11, 12)), ("test_total.py", "test_total", (2, 7)),
            ("test_total.py", "test_other", (9, 10)),
        ):
            with self.subTest(name):
                cited = json.loads(self.cite("--path", path, "--function", name).stdout)
                self.assertEqual(lines, (cited["lines"]["start"], cited["lines"]["end"]))

    def test_cite_refuses_a_function_it_cannot_place(self) -> None:
        (self.repo / "twice.py").write_text("def f():\n    pass\n\ndef f():\n    pass\n", encoding="utf-8")
        (self.repo / "open.go").write_text("package p\n\nfunc Open() {\n\treturn\n", encoding="utf-8")
        for path, name, refusal in (
            ("twice.py", "g", "ERROR: twice.py defines no Go func or Python def named g"),
            ("twice.py", "f", "ERROR: twice.py defines f 2 times; cite the one you mean with --lines"),
            ("open.go", "Open", "ERROR: open.go: the body of Open starting on line 3 does not end; cite it with --lines"),
        ):
            with self.subTest(refusal):
                self.assertEqual(refusal, self.cite("--path", path, "--function", name).stderr.strip())

    def test_output_lines_end_in_a_line_feed_on_every_platform(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\nthree\n")
        result = subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", "cite", "--repo-root", str(self.repo), "--path", "notes.md",
             "--lines", "1-3", "--show"],
            cwd=Path(__file__).parent, capture_output=True, check=False,
        )
        self.assertEqual((False, False, True), (b"\r\n" in result.stdout, b"\r\n" in result.stderr, b"\n" in result.stderr))

    def test_cite_shows_a_one_line_citation_once(self) -> None:
        (self.repo / "notes.md").write_bytes(b"one\ntwo\n")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "notes.md", "--start", "2", "--end", "2", "--show",
        )
        self.assertEqual(["notes.md:2: two"], result.stderr.splitlines())

    def test_the_upsert_command_says_review_will_refuse_an_unconfirmed_file(self) -> None:
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
        result = self.run_cli(
            "upsert-candidate", "--registry-root", str(self.repo / "memory"),
            "--repo-root", str(self.repo), "--asset", "contexts", "--record-file", str(record),
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("files the confirmed source map does not cover: notes.md", result.stderr)
        self.assertIn("ship-package refuses to review", result.stderr)
        self.assertEqual("Candidate record created.", result.stdout.strip())

    def upsert_with(self, **fields: object) -> subprocess.CompletedProcess:
        record = self.write_record("context.json", {
            "id": "orders", "name": "Orders", "responsibility": "Own orders.",
            "evidence": [citation(self.repo, "notes.md", 1, 1)], **fields,
        })
        return self.run_cli("upsert-candidate", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                            "--asset", "contexts", "--record-file", str(record))

    def test_upsert_says_when_it_drops_a_review_status_the_record_carried(self) -> None:
        note = ("NOTE: contexts/orders carried a review status, stored as candidate: a record becomes reviewed only "
                "through an approved Change Package.")
        self.confirmed_corpus()
        (self.repo / "notes.md").write_text("Orders owns pricing.", encoding="utf-8")
        for label, fields, expected in (
            ("reviewed", {"status": "reviewed"}, True),
            ("verified", {"verified": True}, True),
            ("candidate", {"status": "candidate"}, False),
            ("no status", {}, False),
        ):
            with self.subTest(label):
                result = self.upsert_with(**fields)
                self.assertEqual((0, expected), (result.returncode, note in result.stderr))

    def test_recovery_reports_that_nothing_was_interrupted_or_pending(self) -> None:
        result = self.run_cli(
            "recover-registry-update", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo)
        )
        self.assertEqual(
            (0, ["No interrupted Registry update.", "No pending apply to reconcile."]),
            (result.returncode, result.stdout.splitlines()),
        )

    def test_recovery_without_a_repo_root_says_a_pending_apply_remains(self) -> None:
        self.pending_marker("superseded", applied=False)
        result = self.run_cli("recover-registry-update", "--registry-root", str(self.repo / "memory"))
        self.assertEqual(1, result.returncode)
        self.assertIn("ERROR: a pending apply remains; rerun with --repo-root", result.stderr)
        self.assertTrue(self.marker().exists())

    def test_recovery_with_a_repo_root_reconciles_a_pending_apply(self) -> None:
        self.pending_marker("superseded", applied=False)
        result = self.run_cli(
            "recover-registry-update", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo)
        )
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertIn("Pending apply reconciled: PRO-1 was never applied", result.stdout)
        self.assertFalse(self.marker().exists())

    def test_reciting_from_the_command_line_prints_the_update(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--lines", "3",
        )
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertEqual(["PRO-1"], json.loads(result.stdout)["supersedes"])
        self.assertEqual("", result.stderr)

    def test_reciting_onto_an_earlier_output_file_merges_into_it(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        earlier = self.repo / "earlier.json"
        earlier.write_text(json.dumps({"registry_updates": [], "supersedes": ["PRO-9"], "candidates": []}), encoding="utf-8")
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--lines", "3", "--onto", str(earlier),
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        self.assertEqual(["PRO-1", "PRO-9"], json.loads(result.stdout)["supersedes"])

    def test_reciting_without_lines_re_cites_the_replaced_lines_of_the_same_file(self) -> None:
        self.cited_rule("reviewed", (1, 1), (2, 2))
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--replace", "src/a.py:2-2",
        )
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        evidence = json.loads(result.stdout)["registry_updates"][0]["record"]["evidence"]
        one = digest((self.repo / "src" / "a.py").read_bytes().splitlines(keepends=True)[1])
        self.assertEqual(
            [({"start": 1, "end": 1}, one), ({"start": 2, "end": 2}, one)],
            [(reference["lines"], reference["excerpt_sha256"]) for reference in evidence],
        )

    def test_reciting_without_lines_is_refused_unless_replace_names_lines_of_the_same_file(self) -> None:
        self.cited_rule("reviewed", (1, 1), (2, 2))
        (self.repo / "src" / "b.py").write_text("one\n", encoding="utf-8")
        for name, replace in (
            ("no replace", ()),
            ("replace without lines", ("--replace", "src/a.py")),
            ("replace in another file", ("--replace", "src/b.py:1-1")),
        ):
            with self.subTest(name):
                result = self.run_cli(
                    "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
                    "--asset", "rules", "--id", "rule", "--path", "src/a.py", *replace,
                )
                self.assertEqual(1, result.returncode, result.stdout)
                self.assertIn("recite needs --lines, unless --replace names lines of the same file", result.stderr)

    def test_reciting_lines_that_repeat_in_the_file_says_to_widen_them(self) -> None:
        self.cited_rule("reviewed", (2, 2))
        (self.repo / "src" / "a.py").write_text("zero\none\ntwo\ntwo\n", encoding="utf-8")
        result = self.run_cli(
            "recite", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo),
            "--asset", "rules", "--id", "rule", "--path", "src/a.py", "--lines", "3",
        )
        self.assertEqual(0, result.returncode, result.stdout)
        self.assertIn("the text of src/a.py:3-3 also starts at line 4", result.stderr)
        self.assertIn("Widen the citation to the enclosing block", result.stderr)

    def test_citing_lines_that_repeat_in_the_file_says_where_else_they_start(self) -> None:
        (self.repo / "fact.md").write_text("same\nother\nsame\nsame\n", encoding="utf-8")
        result = self.run_cli(
            "cite", "--repo-root", str(self.repo), "--path", "fact.md", "--start", "3", "--end", "3",
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual({"start": 3, "end": 3}, json.loads(result.stdout)["lines"])
        self.assertIn("the text of fact.md:3-3 also starts at line 1, 4", result.stderr)


if __name__ == "__main__":
    unittest.main()
