#!/usr/bin/env python3
import contextlib
import io
import sys

import import_seed


def test_the_offline_selftest_passes_and_says_so_once():
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = import_seed.selftest()
    assert code == 0
    assert out.getvalue() == "selftest ok\n", out.getvalue()


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
