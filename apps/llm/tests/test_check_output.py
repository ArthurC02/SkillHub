import io
import subprocess
import sys
from pathlib import Path

import pytest

from skillhub_llm import check_output

SCRIPT = Path(__file__).parent.parent / "src" / "skillhub_llm" / "check_output.py"


def run_main(argv, monkeypatch=None, stdin_text=None, capsys=None):
    if stdin_text is not None:
        monkeypatch.setattr(sys, "stdin", io.StringIO(stdin_text))
    code = check_output.main(argv)
    out = capsys.readouterr().out if capsys is not None else None
    return code, out


def test_sentence_count_at_the_limit_passes_and_one_over_fails(tmp_path, capsys):
    target = tmp_path / "draft.txt"
    target.write_text("one。two。three。", encoding="utf-8")
    code, out = run_main(["--max-sentences", "3", str(target)], capsys=capsys)
    assert code == 0
    assert out == "OK\n"
    code, out = run_main(["--max-sentences", "2", str(target)], capsys=capsys)
    assert code == 1
    assert out == "sentences 3 > 2\n"


def test_sentences_split_by_newline_alone_with_no_terminal_punctuation(capsys, tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text("one\ntwo\nthree\n", encoding="utf-8")
    code, out = run_main(["--max-sentences", "3", str(target)], capsys=capsys)
    assert code == 0
    code, out = run_main(["--max-sentences", "2", str(target)], capsys=capsys)
    assert out == "sentences 3 > 2\n"


def test_sentences_split_by_punctuation_alone_on_a_single_line(capsys, tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text("one!two?three!", encoding="utf-8")
    code, out = run_main(["--max-sentences", "3", str(target)], capsys=capsys)
    assert code == 0
    code, out = run_main(["--max-sentences", "2", str(target)], capsys=capsys)
    assert out == "sentences 3 > 2\n"


def test_char_count_ignores_whitespace_and_the_limit_is_a_boundary(capsys, tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text("ab cd\tef\ngh", encoding="utf-8")
    code, out = run_main(["--max-chars", "8", str(target)], capsys=capsys)
    assert code == 0
    code, out = run_main(["--max-chars", "7", str(target)], capsys=capsys)
    assert out == "chars 8 > 7\n"


def test_item_count_recognizes_markers_and_the_limit_is_a_boundary(capsys, tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text(
        "intro line, not an item\n- one\n* two\n• three\n1. four\n2) five\n3、 six\n",
        encoding="utf-8",
    )
    code, out = run_main(["--max-items", "6", str(target)], capsys=capsys)
    assert code == 0
    code, out = run_main(["--max-items", "5", str(target)], capsys=capsys)
    assert out == "items 6 > 5\n"


def test_require_passes_when_present_and_fails_when_missing(capsys, tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text("the code is AB1234 and nothing else", encoding="utf-8")
    code, out = run_main(["--require", "AB1234", str(target)], capsys=capsys)
    assert code == 0
    code, out = run_main(["--require", "ZZ9999", str(target)], capsys=capsys)
    assert code == 1
    assert out == "missing: ZZ9999\n"


def test_every_failing_check_is_reported_on_its_own_line(capsys, tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text("one。two。three。", encoding="utf-8")
    code, out = run_main(
        ["--max-sentences", "1", "--max-chars", "2", "--require", "ZZ9999", str(target)],
        capsys=capsys,
    )
    assert code == 1
    assert out.splitlines() == [
        "sentences 3 > 1",
        "chars 14 > 2",
        "missing: ZZ9999",
    ]


def test_file_dash_reads_stdin(capsys, monkeypatch):
    code, out = run_main(
        ["--max-sentences", "1", "-"], monkeypatch=monkeypatch, stdin_text="one。", capsys=capsys
    )
    assert code == 0
    assert out == "OK\n"


def test_a_missing_file_exits_2_and_names_the_path(capsys, tmp_path):
    missing = tmp_path / "does-not-exist.txt"
    code, out = run_main(["--max-chars", "1", str(missing)], capsys=capsys)
    assert code == 2
    assert "does-not-exist.txt" in out


def test_a_file_that_is_not_utf8_exits_2_and_says_so(capsys, tmp_path):
    target = tmp_path / "latin1.txt"
    target.write_bytes(b"\xff\xfe\x00")
    code, out = run_main(["--max-chars", "5", str(target)], capsys=capsys)
    assert code == 2
    assert out.startswith(f"cannot decode {target} as UTF-8")


def test_no_check_flags_at_all_exits_2(capsys, tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text("anything", encoding="utf-8")
    code, out = run_main([str(target)], capsys=capsys)
    assert code == 2
    assert "no checks given" in out


def test_running_the_script_as_a_real_subprocess_prints_ok(tmp_path):
    target = tmp_path / "draft.txt"
    target.write_text("one。two。", encoding="utf-8")
    result = subprocess.run(
        [sys.executable, str(SCRIPT), "--max-sentences", "2", str(target)],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0
    assert result.stdout == "OK\n"


@pytest.mark.parametrize(
    ("max_sentences", "max_chars", "max_items", "require", "expected"),
    [
        (None, None, None, [], []),
        (1, None, None, [], ["sentences 2 > 1"]),
    ],
)
def test_check_function_directly(max_sentences, max_chars, max_items, require, expected):
    assert check_output.check("a。b。", max_sentences, max_chars, max_items, require) == expected
