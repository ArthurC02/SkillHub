#!/usr/bin/env python3
import sys
import urllib.error
from unittest import mock

import check_spec_drift as csd


def test_blob_sha_matches_gits_own_hash_object():
    assert csd.blob_sha(b"hello world\n") == "3b18e512dba79e4c8300dd08aeb37f8e728b8dad"


def test_blob_sha_of_empty_bytes_matches_gits_empty_blob():
    assert csd.blob_sha(b"") == "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"


def test_fetch_exits_with_code_1_and_keeps_the_network_error_as_its_cause():
    error = urllib.error.URLError("no route to host")
    with mock.patch.object(csd.urllib.request, "urlopen", side_effect=error):
        try:
            csd.fetch("https://example.invalid/x")
        except SystemExit as exc:
            assert exc.code == 1, exc.code
            assert exc.__cause__ is error
        else:
            raise AssertionError("an unreachable URL must exit(1)")


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
