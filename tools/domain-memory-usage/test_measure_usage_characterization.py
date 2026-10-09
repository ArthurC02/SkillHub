#!/usr/bin/env python3
import json
import sys
import tempfile
from pathlib import Path

import measure_usage as mu

KNOWN = {"validate", "cite", "verify-audit"}
PLUGIN = Path(__file__).resolve().parents[2] / ".claude/skills/domain-memory/scripts/registry_tools.py"


def call(call_id, command, timestamp="2026-10-10T00:00:00Z"):
    block = {"type": "tool_use", "id": call_id, "name": "Bash", "input": {"command": command}}
    return {"timestamp": timestamp, "message": {"content": [block]}}


def result(call_id, content, is_error=False):
    block = {"type": "tool_result", "tool_use_id": call_id, "content": content, "is_error": is_error}
    return {"message": {"content": [block]}}


def scanned(entries, since=None):
    with tempfile.TemporaryDirectory() as root:
        path = Path(root) / "project" / "session.jsonl"
        path.parent.mkdir()
        path.write_text("\n".join(json.dumps(entry) for entry in entries) + "\nnot json\n", encoding="utf-8")
        return mu.report(mu.scan(Path(root), KNOWN, since), top=3)


def row(summary, command):
    return next(entry for entry in summary["commands"] if entry["command"] == command)


def test_each_failure_form_is_counted_with_its_last_failure_line():
    cases = [
        ("ERROR: registry is invalid", False, "ERROR: registry is invalid"),
        ("usage: x\nregistry_tools.py validate: error: the following arguments are required: --registry-root",
         False, "registry_tools.py validate: error: the following arguments are required: --registry-root"),
        ("Traceback (most recent call last):\n  File x\nKeyError: 'x'", False, "KeyError: <text>"),
        ("Traceback (most recent call last):\n  File x\njson.decoder.JSONDecodeError: Expecting value",
         False, "json.decoder.JSONDecodeError: Expecting value"),
        ("ERROR: a staged copy failed\nERROR: nothing was written", False, "ERROR: nothing was written"),
        ("Exit code 1\nERROR: broken\nsummary follows", True, "ERROR: broken"),
        ("Exit code 2\nsomething", True, "Exit code <n>"),
    ]
    for content, is_error, expected in cases:
        summary = scanned([call("a", "python registry_tools.py validate --registry-root r"),
                           result("a", content, is_error)])
        assert (row(summary, "validate")["failures"], row(summary, "validate")["top_failures"][0]["signature"]) \
            == (1, expected), content


def test_a_clean_run_is_a_call_and_not_a_failure():
    summary = scanned([call("a", "python registry_tools.py cite --file x"), result("a", "Registry is valid.")])
    assert (row(summary, "cite")["calls"], row(summary, "cite")["failures"]) == (1, 0)


def test_a_failure_echoed_inside_a_clean_run_is_not_a_failure():
    echoed = '{\n  "output": "Traceback (most recent call last):\nAssertionError: expected red"\n}'
    summary = scanned([call("a", "python registry_tools.py validate"), result("a", echoed)])
    assert row(summary, "validate")["failures"] == 0


def test_list_content_is_read_like_text():
    summary = scanned([call("a", "python registry_tools.py validate"),
                       result("a", [{"type": "text", "text": "ERROR: broken"}])])
    assert row(summary, "validate")["failures"] == 1


def test_a_call_refused_by_the_host_is_not_charged_to_the_command():
    summary = scanned([call("a", "python registry_tools.py validate"),
                       result("a", "Permission to use Bash with command x has been denied.", True)])
    assert (summary["host_refusals"], summary["commands"]) == (1, [])


def test_several_commands_in_one_call_are_counted_but_not_judged():
    summary = scanned([call("a", "python registry_tools.py validate && python registry_tools.py cite"),
                       result("a", "ERROR: broken", True)])
    assert (summary["compound_calls"], summary["failures"], row(summary, "cite")["calls"]) == (1, 0, 1)


def test_a_call_piped_into_another_python_program_is_counted_but_not_judged():
    command = '"C:/Python314/python.exe" registry_tools.py validate | python -c "import json; json.load(open(\'x\'))"'
    summary = scanned([call("a", command), result("a", "FileNotFoundError: [Errno 2] x", True)])
    assert (summary["compound_calls"], summary["failures"], row(summary, "validate")["calls"]) == (1, 0, 1)


def test_one_python_running_the_plugin_is_still_judged():
    summary = scanned([call("a", "python3.12 .claude/scripts/registry_tools.py validate | tail -3"),
                       result("a", "ERROR: broken", True)])
    assert (summary["compound_calls"], summary["failures"]) == (0, 1)


def test_a_call_without_a_result_is_counted_but_not_called_a_success():
    summary = scanned([call("a", "python registry_tools.py validate")])
    assert (summary["unanswered_calls"], row(summary, "validate")["calls"], summary["failures"]) == (1, 1, 0)


def test_words_after_the_script_name_that_are_not_commands_are_ignored():
    summary = scanned([call("a", "echo registry_tools.py in the plugin"), result("a", "ok")])
    assert summary["calls"] == 0


def test_calls_before_since_are_left_out_and_calls_at_since_are_kept():
    entries = [
        call("a", "python registry_tools.py validate", "2026-10-09T23:59:59Z"), result("a", "ok"),
        call("b", "python registry_tools.py cite", "2026-10-10T00:00:00Z"), result("b", "ok"),
    ]
    summary = scanned(entries, since="2026-10-10T00:00:00Z")
    assert [entry["command"] for entry in summary["commands"]] == ["cite"]


def test_a_signature_carries_no_path_quoted_text_digest_or_number():
    text = "ERROR: C:\\Users\\me\\repo\\file.json at docs/a/b.json said 'secret' for 0123abcd99 after 42 tries"
    assert mu.signature(text) == "ERROR: <path> at <path> said <text> for <hex> after <n> tries"


def test_the_plugin_help_names_its_commands():
    commands = mu.plugin_commands(PLUGIN)
    assert {"validate", "record-test-result", "audit-attestations"} <= commands and "in" not in commands


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
