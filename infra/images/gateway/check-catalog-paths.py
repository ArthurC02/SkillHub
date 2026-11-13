import ctypes
import json
import locale
import os
from pathlib import Path
import struct
import sys
import subprocess
import tempfile

EXPECTED_CASES = 6

libc = ctypes.CDLL("libc.so.6")
libc.bindtextdomain.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
libc.bindtextdomain.restype = ctypes.c_char_p
libc.gettext.argtypes = [ctypes.c_char_p]
libc.gettext.restype = ctypes.c_char_p
libc.textdomain.argtypes = [ctypes.c_char_p]
libc.textdomain.restype = ctypes.c_char_p


def catalog():
    originals = [b"", b"message"]
    translated = [b"Content-Type: text/plain; charset=UTF-8\n", b"translated"]
    table_offset = 28
    data_offset = table_offset + 16 * len(originals)
    tables = []
    body = bytearray()
    for strings in [originals, translated]:
        for value in strings:
            tables.append(struct.pack("<2I", len(value), data_offset + len(body)))
            body.extend(value + b"\0")
    return (
        struct.pack(
            "<7I",
            0x950412DE,
            0,
            len(originals),
            table_offset,
            table_offset + 8 * len(originals),
            0,
            0,
        )
        + b"".join(tables)
        + body
    )


cases = {
    "normal": ("root", "de_DE", b"translated"),
    "parent": ("messages", "..", b"message"),
    "relative": ("locale", "../de_DE", b"message"),
    "nested": ("messages", "../../de_DE", b"message"),
    "fallback": ("root", "..:../de_DE:de_DE", b"translated"),
    "embedded": ("locale", "LC_MESSAGES/..", b"message"),
}


def run_case(name):
    binding, language, expected = cases[name]
    locale.setlocale(locale.LC_ALL, "de_DE.UTF-8")
    os.environ.pop("OUTPUT_CHARSET", None)
    os.environ["LANGUAGE"] = language
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        localedir = root / "de_DE"
        messages = localedir / "LC_MESSAGES"
        messages.mkdir(parents=True)
        (messages / "catalog-probe.mo").write_bytes(catalog())
        directory = {"root": root, "locale": localedir, "messages": messages}[binding]
        assert libc.bindtextdomain(b"catalog-probe", os.fsencode(directory))
        assert libc.textdomain(b"catalog-probe") == b"catalog-probe"
        actual = libc.gettext(b"message")
        print(
            json.dumps(
                {
                    "case": name,
                    "locale": locale.setlocale(locale.LC_ALL),
                    "language": language,
                    "expected": expected.decode(),
                    "actual": actual.decode(),
                }
            ),
            flush=True,
        )
        if actual != expected:
            raise AssertionError((name, actual, expected))


def run_all():
    outcomes = []
    for name in cases:
        result = subprocess.run(
            [sys.executable, "-B", "-I", __file__, name],
            capture_output=True,
            text=True,
            check=False,
        )
        print(result.stdout + result.stderr, flush=True)
        if result.returncode:
            raise SystemExit(result.returncode)
        outcomes.append(json.loads(result.stdout))
    if len(outcomes) != EXPECTED_CASES or any(
        item["expected"] != item["actual"] for item in outcomes
    ):
        raise ValueError("catalog probe incomplete")
    Path("/tmp/catalog-proof.json").write_text(json.dumps({"passed": len(outcomes)}))


if __name__ == "__main__":
    if sys.argv[1:]:
        run_case(*sys.argv[1:])
    else:
        run_all()
