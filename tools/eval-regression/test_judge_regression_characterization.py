"""Characterization of judge_regression.py's request building, citation
verification and replay loop, with every database, archive and Judge call
replaced by an in-memory fake."""

from __future__ import annotations

import contextlib
import io
import json
import re
import sys
import tempfile
from pathlib import Path
from unittest import mock

import judge_regression as jr

ACTIVATION = "Run 的 trace 中出現對指定 Skill 的 skill_activation"
ARTIFACT = "/out/artifacts/ 中至少產出一個檔案"
EV_A = "aaaaaaaa-0000-4000-8000-000000000001"
EV_B = "bbbbbbbb-0000-4000-8000-000000000002"


def event(event_id, event_type, payload, seq, source="sandbox", attempt=1):
    return {"event_id": event_id, "occurred_at": f"2026-08-01T00:00:{seq:02d}Z",
            "event_type": event_type, "source": source, "attempt": attempt, "seq": seq,
            "payload": payload}


def row(name, run_id, criteria=None):
    return {"skill_name": name, "rubric_skill_name": name, "skill_summary": name + " summary",
            "runtime_image": "img", "run_id": run_id, "run_status": "succeeded",
            "failure_class": "", "user_prompt": "do it",
            "acceptance_criteria": criteria or [{"id": "c1", "text": ACTIVATION},
                                                {"id": "c2", "text": ARTIFACT}]}


def test_a_small_run_builds_an_uncut_request_where_the_last_final_output_wins():
    events = [
        event(EV_A, "agent_output", {"kind": "final", "text": "first"}, 1),
        event("n1", "not_citable", {"x": 1}, 2),
        event(EV_B, "agent_output", {"kind": "final", "text": None}, 3),
        event("n2", "agent_output", None, 4),
        event("n3", "agent_output", {"kind": "final", "text": "last"}, 5),
    ]
    request, digest, final = jr.build_request(row("s", "r1"), events, [{"path": "a"}], "ev-1")
    assert final == "last"
    assert request == {
        "run_id": "r1", "evaluation_id": "ev-1",
        "skill": {"name": "s", "summary": "s summary"}, "user_prompt": "do it",
        "criteria": [{"id": "c1", "text": ACTIVATION, "evidence_excerpt": None},
                     {"id": "c2", "text": ARTIFACT, "evidence_excerpt": None}],
        "final_output": "last", "artifacts": [{"path": "a"}],
        "trace_digest": {"complete": True, "entries": [
            {"trace_event_id": e["event_id"], "occurred_at": e["occurred_at"], "type": "agent_output",
             "excerpt": json.dumps(e["payload"], ensure_ascii=False)}
            for e in events if e["event_type"] == "agent_output"]},
        "truncation": [],
    }, request
    assert list(digest) == [EV_A, EV_B, "n2", "n3"]


def test_every_budget_cut_is_named_in_order_and_the_digest_keeps_the_tail():
    events = [event(f"e{i}", "tool_call", {"i": i}, i) for i in range(jr.MAX_DIGEST_COUNT + 5)]
    events.append(event("big", "tool_call", {"t": "x" * jr.MAX_DIGEST_ENTRY}, 200))
    events.append(event("fin", "agent_output", {"kind": "final", "text": "y" * (jr.MAX_FINAL_OUTPUT + 1)}, 201))
    events.append(event("tail", "usage", {"tokens": 1}, 202))
    criteria = [{"id": f"c{i}", "text": f"t{i}"} for i in range(jr.MAX_CRITERIA)]
    rubric = {"criteria": [{"id": "r1", "text": "voice"}], "rubric": {"items": [{"id": "r1"}]}}
    artifacts = [{"path": f"f{i}"} for i in range(jr.MAX_ARTIFACT_ROWS + 1)]
    request, digest, final = jr.build_request(row("s", "r1", criteria), events, artifacts, "ev", rubric)
    assert request["truncation"] == ["final_output", "criteria", "artifacts", "trace_digest.entries",
                                     "trace_digest.entries[].excerpt"], request["truncation"]
    assert len(final) == jr.MAX_FINAL_OUTPUT and request["final_output"] == final
    assert [c["id"] for c in request["criteria"]] == [f"c{i}" for i in range(jr.MAX_CRITERIA)]
    assert len(request["artifacts"]) == jr.MAX_ARTIFACT_ROWS
    entries = request["trace_digest"]["entries"]
    assert len(entries) == jr.MAX_DIGEST_COUNT and entries[0]["trace_event_id"] == "e8"
    assert [e["trace_event_id"] for e in entries[-3:]] == ["big", "fin", "tail"]
    assert len(entries[-3]["excerpt"]) == jr.MAX_DIGEST_ENTRY
    assert request["rubric"] == {"items": [{"id": "r1"}]}
    assert request["trace_digest"]["complete"] is False
    assert len(digest) == jr.MAX_DIGEST_COUNT


def test_a_rubric_within_budget_adds_its_criteria_without_a_cut():
    rubric = {"criteria": [{"id": "r1", "text": "voice"}], "rubric": {"items": [{"id": "r1"}]}}
    request, _, _ = jr.build_request(row("s", "r1"), [], [], "ev", rubric)
    assert [c["id"] for c in request["criteria"]] == ["c1", "c2", "r1"]
    assert request["truncation"] == [] and request["final_output"] == ""


QUOTE = "the quarterly figures were restated"
DIGEST = {EV_A: {"payload": {"text": "unrelated tool output here"}},
          EV_B: {"payload": {"text": "log: " + QUOTE + " in full"}}}
FINAL = "summary: " + QUOTE


VERIFY_CASES = [
    ("a missing trace event with no quote",
     {"kind": "trace_event", "trace_event_id": "gone"}, DIGEST, "",
     (None, "cited trace event 'gone' was not in the digest")),
    ("a missing trace event whose quote is in the final output",
     {"kind": "trace_event", "trace_event_id": "gone", "quote": QUOTE}, DIGEST, FINAL,
     ({"kind": "agent_output", "quote": QUOTE, "match": "exact",
       "reattributed_from": "trace_event"}, "")),
    ("a missing trace event whose quote is nowhere",
     {"kind": "trace_event", "trace_event_id": "gone", "quote": "absent everywhere at all"}, DIGEST, "",
     (None, "cited trace event 'gone' was not in the digest, and it is in no other verifiable "
            "source of this run")),
    ("a present trace event cited without a quote",
     {"kind": "trace_event", "trace_event_id": EV_A}, DIGEST, "",
     ({"kind": "trace_event", "trace_event_id": EV_A, "match": "exact", "reattributed_from": None}, "")),
    ("a quote filed under the wrong trace event",
     {"kind": "trace_event", "trace_event_id": EV_A, "quote": QUOTE}, DIGEST, "",
     ({"kind": "trace_event", "trace_event_id": EV_B, "quote": QUOTE, "match": "exact",
       "reattributed_from": "trace_event"}, "")),
    ("a quote filed under the wrong trace event and found nowhere",
     {"kind": "trace_event", "trace_event_id": EV_A, "quote": "absent everywhere at all"}, DIGEST, "",
     (None, f"the quote cited from trace event {EV_A!r} is not in it, and it is in no other "
            "verifiable source of this run")),
    ("an agent output cited without a quote",
     {"kind": "agent_output"}, DIGEST, FINAL,
     (None, "an agent output reference was cited with no quote to locate")),
    ("an agent output quote found in it",
     {"kind": "agent_output", "quote": QUOTE}, {}, FINAL,
     ({"kind": "agent_output", "quote": QUOTE, "match": "exact", "reattributed_from": None}, "")),
    ("an agent output quote found nowhere",
     {"kind": "agent_output", "quote": "absent everywhere at all"}, DIGEST, FINAL,
     (None, "the quote cited from the agent's final output is not in it, and it is in no other "
            "verifiable source of this run")),
    ("an artifact without a quote in the manifest",
     {"kind": "artifact", "artifact_path": "a.md"}, {}, "",
     ({"kind": "artifact", "artifact_path": "a.md", "match": "not_checked", "reattributed_from": None}, "")),
    ("an artifact without a quote outside the manifest",
     {"kind": "artifact", "artifact_path": "b.md"}, {}, "",
     (None, "cited artifact 'b.md' is not in this run's manifest")),
    ("an artifact whose quote is in the trace",
     {"kind": "artifact", "artifact_path": "b.md", "quote": QUOTE}, DIGEST, "",
     ({"kind": "trace_event", "artifact_path": "b.md", "quote": QUOTE, "match": "exact",
       "reattributed_from": "artifact", "trace_event_id": EV_B}, "")),
    ("a reference with no kind",
     {"quote": QUOTE}, DIGEST, FINAL,
     (None, "reference kind None is not one this platform can resolve")),
]


def test_every_verify_branch_returns_the_same_reference_and_reason():
    for label, ref, digest, final, want in VERIFY_CASES:
        got = jr.verify(ref, digest, [{"path": "a.md"}], final)
        assert got == want, (label, got)


def test_a_reattributed_citation_keeps_its_exact_key_order():
    stored, _ = jr.verify(VERIFY_CASES[1][1], DIGEST, [], FINAL)
    assert list(stored) == ["kind", "quote", "match", "reattributed_from"], list(stored)


RUBRIC = {"rubric_version": "rv1", "skills": {
    "alpha": {"criteria": [{"id": "r1", "text": "keeps the voice"}],
              "rubric": {"items": [{"id": "r1", "evidence_required": False}]}},
    "zeta": {"criteria": [{"id": "r9", "text": "x"}], "rubric": {"items": [{"id": "r9"}]}},
}}


def events_for(name):
    return [event(EV_A, "skill_activation", {"skill_name": name, "decision": "activated"}, 1),
            event(EV_B, "agent_output", {"kind": "final", "text": "wrote report.md for " + name}, 2)]


def verdict_for(request, overall="met"):
    results = []
    for c in request["criteria"]:
        refs = [{"kind": "artifact", "artifact_path": "report.md"}] if c["id"] == "c2" else \
            [{"kind": "trace_event", "trace_event_id": EV_A}]
        results.append({"criterion_id": c["id"], "result": "passed", "reason": "seen " + c["id"],
                        "evidence_refs": refs})
    return {"criterion_results": results, "overall": overall, "summary": "ok"}


def replay(argv, rows, costs, rubric=None):
    calls, out, folder = [], io.StringIO(), tempfile.mkdtemp()
    results = Path(folder) / "results.jsonl"
    argv = list(argv)
    if rubric is not None:
        path = Path(folder) / "rubric.json"
        path.write_text(json.dumps(rubric), encoding="utf-8")
        argv += ["--rubric", str(path)]
    cost_iter = iter(costs)

    def fake_judge(request, url):
        calls.append((request["run_id"], url))
        return {"verdict": verdict_for(request), "model": "judge-m", "prompt_version": "p1",
                "temperature": 0, "seed": 7, "usage": {"cost_usd": next(cost_iter)}}

    def fake_explicit(run_ids):
        return [r for r in rows if r["run_id"] in {str(i) for i in run_ids}]

    with mock.patch.object(sys, "argv", ["judge_regression.py", *argv]), \
            mock.patch.object(jr, "regression_set", lambda: list(rows)), \
            mock.patch.object(jr, "explicit_run_set", fake_explicit), \
            mock.patch.object(jr, "trace_events", lambda run_id: events_for(run_id)), \
            mock.patch.object(jr, "artifact_manifest",
                              lambda run_id: [] if run_id == "beta" else [{"path": "report.md"}]), \
            mock.patch.object(jr, "judge", fake_judge), mock.patch.object(jr, "OUT", results), \
            contextlib.redirect_stdout(out):
        try:
            jr.main()
            stopped = None
        except SystemExit as exc:
            stopped = str(exc)
    text = re.sub(r"regression \d{4}-\d{2}-\d{2}T\d{6}Z", "regression <id>", out.getvalue())
    text = text.replace(str(results), "<out>")
    lines = [json.loads(ln) for ln in results.read_text(encoding="utf-8").splitlines()] \
        if results.exists() else []
    return text, stopped, calls, lines


ROWS = [row("alpha", "alpha"), row("beta", "beta"), row("gamma", "gamma")]


def test_a_dry_run_reads_every_selected_run_and_calls_no_judge():
    text, stopped, calls, lines = replay(["--dry-run"], ROWS, [])
    assert stopped is None and calls == [] and lines == []
    assert text == (
        "regression <id>: 3 runs, selection=m2_latest_compatibility, rubric_version=None\n"
        "[1/3] alpha: 2 events, 1 files, expect activation=passed artifact=passed\n"
        "[2/3] beta: 2 events, 0 files, expect activation=passed artifact=failed\n"
        "[3/3] gamma: 2 events, 1 files, expect activation=passed artifact=passed\n"
    ), text


def test_a_rubric_narrows_the_baseline_and_names_what_it_could_not_find():
    text, stopped, _, _ = replay(["--dry-run", "--limit", "5"], ROWS, [], rubric=RUBRIC)
    assert stopped is None
    assert text == (
        "! no baseline run for: zeta\n"
        "regression <id>: 1 runs, selection=m2_latest_compatibility, rubric_version=rv1\n"
        "[1/1] alpha: 2 events, 1 files, expect activation=passed artifact=passed\n"
    ), text


def test_explicit_runs_a_rubric_does_not_cover_are_refused():
    ids = ["--run-id", "11111111-1111-4111-8111-111111111111"]
    rows = [row("beta", "11111111-1111-4111-8111-111111111111")]
    _, stopped, calls, _ = replay(["--dry-run", *ids], rows, [], rubric=RUBRIC)
    assert stopped == ("--rubric does not cover explicitly selected Run ids: "
                       "11111111-1111-4111-8111-111111111111"), stopped
    assert calls == []


def test_an_empty_selection_is_refused():
    _, stopped, _, _ = replay(["--dry-run"], [], [])
    assert stopped == "selection produced zero Runs", stopped


def test_a_live_run_scores_records_and_stops_at_the_cost_alarm():
    text, stopped, calls, lines = replay(["--judge-url", "http://j", "--note", "n"], ROWS, [0.01, None, 0.5])
    assert stopped is None
    assert calls == [("alpha", "http://j"), ("beta", "http://j"), ("gamma", "http://j")]
    assert [(ln["run_id"], ln["note"], ln["run_selection"]) for ln in lines] == [
        ("alpha", "n", "m2_latest_compatibility"), ("beta", "n", "m2_latest_compatibility"),
        ("gamma", "n", "m2_latest_compatibility")]
    assert [[c["outcome"] for c in ln["criteria"]] for ln in lines] == [
        ["match", "match"], ["match", "downgraded"], ["match", "match"]]
    assert text == LIVE_REPORT, text


LIVE_REPORT = (
    "regression <id>: 3 runs, selection=m2_latest_compatibility, rubric_version=None\n"
    "[1/3] alpha: 2 events, 1 files, expect activation=passed artifact=passed\n"
    "    activation   want=passed       got=passed       ok\n"
    "    artifact     want=passed       got=passed       ok\n"
    "    $0.0100  running total $0.0100\n"
    "[2/3] beta: 2 events, 0 files, expect activation=passed artifact=failed\n"
    "    activation   want=passed       got=passed       ok\n"
    "    artifact     want=failed       got=undetermined undet\n"
    "    $nan  running total $0.0100\n"
    "[3/3] gamma: 2 events, 1 files, expect activation=passed artifact=passed\n"
    "    !! $0.5000 for one call, over the $0.1 alarm - stopping\n"
    "\n"
    "appended 3 rows to <out>\n"
    "\n"
    "scored 6 criteria over 3 runs\n"
    "  agreement   5/6 = 83.3%\n"
    "  mismatch    0\n"
    "  downgraded  1 (undetermined, counted apart from wrong)\n"
    "  model undetermined 0 (counted apart from wrong)\n"
    "  cost        $0.5100, 1 calls unreported\n"
)


if __name__ == "__main__":
    failed = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"ok   {name}")
            except AssertionError as exc:
                failed += 1
                print(f"FAIL {name}: {exc}")
    sys.exit(1 if failed else 0)
