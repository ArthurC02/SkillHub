#!/usr/bin/env python3
import contextlib
import io
import sys
import urllib.error
from types import SimpleNamespace

import seed_testcases as st


class FakeResponse:
    def __init__(self, status, body):
        self.status = status
        self._body = body

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def read(self):
        return self._body


class FakeOpener:
    def __init__(self, response=None, error=None):
        self.response = response
        self.error = error
        self.sent = None

    def open(self, req, timeout=None):
        self.sent = req
        if self.error is not None:
            raise self.error
        return self.response


def client_with(opener):
    mod = SimpleNamespace(make_opener=lambda: opener)
    return st.Client(mod, "http://api")


def test_a_json_body_call_sends_json_content_type_and_parses_the_reply():
    opener = FakeOpener(FakeResponse(200, b'{"ok": true}'))
    client = client_with(opener)
    result = client.call("POST", "/x", json_body={"a": 1})
    assert result == {"ok": True}
    assert opener.sent.get_header("Content-type") == "application/json"
    assert opener.sent.data == b'{"a": 1}'


def test_a_raw_body_call_sends_its_own_content_type():
    opener = FakeOpener(FakeResponse(201, b""))
    client = client_with(opener)
    result = client.call(
        "POST", "/x", raw=st.RawBody(b"bytes", "multipart/form-data; boundary=z")
    )
    assert result == {}
    assert opener.sent.get_header("Content-type") == "multipart/form-data; boundary=z"
    assert opener.sent.data == b"bytes"


def test_a_status_outside_want_raises_with_the_body_excerpt():
    opener = FakeOpener(FakeResponse(500, b"boom"))
    client = client_with(opener)
    try:
        client.call("GET", "/x")
    except SystemExit as exc:
        assert str(exc) == "GET /x -> 500: boom", exc
    else:
        raise AssertionError("an unwanted status must raise")


def test_an_http_error_status_is_read_like_a_normal_response():
    opener = FakeOpener(error=urllib.error.HTTPError(
        "http://api/x", 404, "not found", {}, io.BytesIO(b"missing")
    ))
    client = client_with(opener)
    try:
        client.call("GET", "/x")
    except SystemExit as exc:
        assert str(exc) == "GET /x -> 404: missing", exc
    else:
        raise AssertionError("a 404 outside want must raise")


def test_a_url_error_aborts_with_its_reason_and_keeps_the_cause():
    opener = FakeOpener(error=urllib.error.URLError("connection refused"))
    client = client_with(opener)
    try:
        client.call("GET", "/x")
    except SystemExit as exc:
        assert str(exc) == "GET /x: connection refused", exc
        assert isinstance(exc.__cause__, urllib.error.URLError)
    else:
        raise AssertionError("a URLError must raise SystemExit")


def test_the_offline_selftest_passes_and_reports_the_curated_counts():
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = st.selftest()
    assert code == 0
    assert out.getvalue() == "selftest ok (15 test cases, 67 criteria, 22 rubric items)\n", out.getvalue()


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
