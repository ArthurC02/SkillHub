#!/usr/bin/env python3
import sys
import urllib.error
from unittest import mock

import survey_plugin_shapes as sps


class FakeResponse:
    def __init__(self, body):
        self._body = body

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def read(self, n=None):
        return self._body if n is None else self._body[:n]


def test_a_successful_fetch_returns_200_and_the_raw_bytes():
    with mock.patch.object(sps.urllib.request, "urlopen", return_value=FakeResponse(b'{"a": 1}')):
        status, body = sps._get_bytes("http://x")
    assert (status, body) == (200, b'{"a": 1}')


def test_an_http_error_returns_its_status_and_no_body():
    error = urllib.error.HTTPError("http://x", 404, "not found", {}, None)
    with mock.patch.object(sps.urllib.request, "urlopen", side_effect=error):
        status, body = sps._get_bytes("http://x")
    assert (status, body) == (404, None)


def test_any_other_failure_reports_status_zero_and_no_body():
    with mock.patch.object(sps.urllib.request, "urlopen", side_effect=TimeoutError("slow")):
        status, body = sps._get_bytes("http://x")
    assert (status, body) == (0, None)


def test_get_json_parses_the_body_as_json_on_success():
    with mock.patch.object(sps.urllib.request, "urlopen", return_value=FakeResponse(b'{"a": 1}')):
        status, payload = sps._get_json("http://x")
    assert (status, payload) == (200, {"a": 1})


def test_get_json_reports_none_when_the_fetch_fails():
    error = urllib.error.HTTPError("http://x", 403, "forbidden", {}, None)
    with mock.patch.object(sps.urllib.request, "urlopen", side_effect=error):
        status, payload = sps._get_json("http://x")
    assert (status, payload) == (403, None)


def test_get_text_decodes_the_body_as_utf8_on_success():
    with mock.patch.object(sps.urllib.request, "urlopen", return_value=FakeResponse("héllo".encode())):
        status, text = sps._get_text("http://x")
    assert (status, text) == (200, "héllo")


def test_get_text_reports_none_when_the_fetch_fails():
    with mock.patch.object(sps.urllib.request, "urlopen", side_effect=TimeoutError("slow")):
        status, text = sps._get_text("http://x")
    assert (status, text) == (0, None)


def test_a_file_fetch_stops_at_the_read_cap():
    with mock.patch.object(sps.urllib.request, "urlopen", return_value=FakeResponse(b"y" * 300_000)):
        status, body = sps._get_bytes("http://x")
    assert (status, len(body)) == (200, 200_000)


def test_get_json_reads_a_response_larger_than_that_cap_whole():
    body = b'{"a": "' + b"x" * 300_000 + b'"}'
    with mock.patch.object(sps.urllib.request, "urlopen", return_value=FakeResponse(body)):
        status, payload = sps._get_json("http://x")
    assert (status, len(payload["a"])) == (200, 300_000)


def test_author_shape_names_what_makes_an_author_unusable():
    cases = {
        "conforming": [None, {}, {"name": "A"}, {"name": "A", "email": "a@b.invalid", "url": "https://b.invalid"}],
        "not an object": ["A", 1, [], ["A"]],
        "a value that is not a string": [{"name": 5}, {"url": ["https://b.invalid"]}],
    }
    for expected, authors in cases.items():
        for author in authors:
            shape = sps.author_shape(author)
            if expected == "conforming" and author is None:
                assert shape == "absent", shape
                continue
            assert shape == expected, f"{author!r} -> {shape}"
    assert sps.author_shape({"name": "A", "twitter": "@a"}) == (
        "fields the specification does not allow: twitter")
    assert sps.author_shape({"zebra": 1, "apple": 2}) == (
        "fields the specification does not allow: apple, zebra")


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
