#!/usr/bin/env python3
import contextlib
import io
import sys

import generate


def test_the_offline_selftest_passes_and_reports_every_variant():
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = generate.selftest()
    assert code == 0
    assert out.getvalue() == f"selftest ok ({len(generate.MUTATIONS)} variants)\n", out.getvalue()


def test_edit_replaces_only_the_named_entry_and_leaves_others_untouched():
    entries = [("SKILL.md", b"before"), ("reference/guide.md", b"guide")]
    edited = generate.edit(entries, "SKILL.md", lambda text: text.upper())
    assert edited == [("SKILL.md", b"BEFORE"), ("reference/guide.md", b"guide")]


def test_edit_on_a_missing_name_changes_nothing():
    entries = [("SKILL.md", b"before")]
    assert generate.edit(entries, "nope.md", lambda text: text.upper()) == entries


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
