#!/usr/bin/env python3
import contextlib
import io
import sys

import generate_summaries


def test_the_offline_selftest_passes_and_reports_the_seed_count():
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = generate_summaries.selftest()
    assert code == 0
    assert out.getvalue().startswith("selftest ok — 45 seed skills, "), out.getvalue()
    assert out.getvalue().endswith(" reusable\n"), out.getvalue()


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
