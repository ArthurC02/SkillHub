from __future__ import annotations

import bisect
import io
import re
from collections.abc import Callable
from functools import cache
from pathlib import Path
from typing import Any

from .common import MAX_EVIDENCE_BYTES, MAX_NESTING_DEPTH, inside, load_json, review_field, sha256_of, whole_number
from .git import checked_git_bytes
from .sources import repository_files, source_kind_index


DERIVED_EVIDENCE_KEYS = ("content_sha256", "source_kind")
WIDEN_TO_UNIQUE ="Widen the citation to the enclosing block, so its lines occur once in the file."


def source_map_for(registry_root: Path) -> dict[str, Any]:
    path = registry_root / "source-map.json"
    if not path.is_file():
        return {}
    return load_json(path)


def _valid_line_range(start: Any, end: Any) -> bool:
    return (
        whole_number(start)
        and whole_number(end)
        and start >= 1
        and end >= start
    )


def _read_lines(path: Path) -> tuple[bytes, list[str]]:
    content = path.read_bytes()
    return content, _rows(content)


def _rows(content: bytes) -> list[str]:
    return io.StringIO(content.decode("utf-8", errors="replace")).readlines()


def _excerpt(rows: list[str], start: int, end: int) -> bytes:
    return "".join(rows[start - 1 : end]).encode("utf-8")


def classifier(source_map: dict[str, Any]) -> Callable[[Any], Any]:
    if not source_map:
        return lambda reference: reference
    kind_of = cache(lambda: source_kind_index(source_map))

    def classify(reference: Any) -> Any:
        if not isinstance(reference, dict) or not isinstance(reference.get("path"), str):
            return reference
        return reference | {"source_kind": kind_of()(reference["path"])}

    return classify


def classified(reference: Any, source_map: dict[str, Any]) -> Any:
    return classifier(source_map)(reference)


def with_references(value: Any, change: Callable[[Any], Any], depth: int = 0) -> Any:
    if depth > MAX_NESTING_DEPTH:
        raise ValueError(f"evidence document exceeds nesting depth {MAX_NESTING_DEPTH}")
    if isinstance(value, dict):
        return {
            key: [change(item) for item in child]
            if key == "evidence" and isinstance(child, list)
            else with_references(child, change, depth + 1)
            for key, child in value.items()
        }
    if isinstance(value, list):
        return [with_references(child, change, depth + 1) for child in value]
    return value


def without_derived_keys(value: Any) -> Any:
    return with_references(
        value,
        lambda reference: {key: item for key, item in reference.items() if key not in DERIVED_EVIDENCE_KEYS}
        if isinstance(reference, dict)
        else reference,
    )


def fields_beyond_evidence(asset: str, before: dict[str, Any], after: dict[str, Any]) -> list[str]:
    settled_on_apply = ("verified", review_field(asset))
    stripped = [
        with_references({key: value for key, value in record.items() if key not in settled_on_apply}, lambda _: None)
        for record in (before, after)
    ]
    return sorted(
        key for key in stripped[0].keys() | stripped[1].keys()
        if key not in stripped[0] or key not in stripped[1] or stripped[0][key] != stripped[1][key]
    )


def classify_all(value: Any, source_map: dict[str, Any]) -> Any:
    return with_references(value, classifier(source_map))


def _cited_file(root: Path, relative: str) -> Path:
    path = inside(root, relative)
    if path is None:
        raise ValueError(f"evidence path escapes the repository: {relative}")
    if not path.is_file():
        raise ValueError(f"no such file to cite: {relative}")
    return path


INDENT = r"^(?P<indent>[ \t]*)"
GO_START = r"func(?:\s*\([^)]*\))?\s+{name}\s*(?P<open>[(\[])"
PYTHON_START = r"(?:async\s+)?def\s+{name}\s*[(\[]"
SCRIPT_START = (
    r"(?:export\s+)?(?:default\s+)?(?:"
    r"(?:async\s+)?function\s*\*?\s*{name}\s*(?P<open>[(<])"
    r"|(?:const|let|var)\s+{name}\s*(?::[^=]*?)?(?P<arrow>=)(?!=)"
    r"|(?:(?:public|private|protected|static|async|readonly|override)\s+)*{name}\s*(?P<method>[(<])"
    r")"
)
JAVA_START = (
    r"(?!return\b|new\b|throw\b|else\b)(?:(?:public|private|protected|static|final|abstract|synchronized|native|default)\s+)*"
    r"(?:<[^>]+>\s+)?(?:[\w.$<>\[\],?]+\s+)?{name}\s*(?P<open>\()"
)
KOTLIN_START = (
    r"(?:(?:public|private|protected|internal|override|open|abstract|final|suspend|inline|infix|operator|tailrec|external|actual|expect)\s+)*"
    r"fun\s+(?:<[^>]+>\s+)?(?:[\w.<>?]+\.)?{name}\s*(?P<open>[(<])"
)
RUST_START = r"(?:pub(?:\([^)]*\))?\s+)?(?:(?:const|async|unsafe|extern\s+\"[^\"]*\")\s+)*fn\s+{name}\s*(?P<open>[(<])"
BRACES, INDENTATION = "braces", "indentation"
LANGUAGES: dict[str, tuple[str, str, str]] = {
    ".go": ("Go", GO_START, BRACES),
    ".py": ("Python", PYTHON_START, INDENTATION),
    ".ts": ("TypeScript", SCRIPT_START, BRACES),
    ".tsx": ("TypeScript", SCRIPT_START, BRACES),
    ".js": ("JavaScript", SCRIPT_START, BRACES),
    ".jsx": ("JavaScript", SCRIPT_START, BRACES),
    ".mjs": ("JavaScript", SCRIPT_START, BRACES),
    ".java": ("Java", JAVA_START, BRACES),
    ".kt": ("Kotlin", KOTLIN_START, BRACES),
    ".rs": ("Rust", RUST_START, BRACES),
}
LANGUAGE_NAMES = ", ".join(dict.fromkeys(language for language, _, _ in LANGUAGES.values()))
SCRIPT_LANGUAGES = {"TypeScript", "JavaScript"}
QUOTES = "'\"`"
OPENERS = "([{"
CLOSERS = ")]}"
BLOCK, EXPRESSION, LINE = "block", "expression", "line"
REGEX_AFTER_CHARACTERS = "(,=:[!&|?{};+-<>\n"
REGEX_AFTER_WORDS = {"return", "typeof", "case", "in", "of", "void", "delete", "throw", "await", "yield", "instanceof"}


def _language(relative: str) -> tuple[str, str, str]:
    suffix = Path(relative).suffix
    if suffix not in LANGUAGES:
        shown = f"a {suffix} file" if suffix else "a file without a suffix"
        raise ValueError(f"{relative}: --function reads {LANGUAGE_NAMES} files; cite {shown} with --lines")
    return LANGUAGES[suffix]


def _is_name_char(character: str) -> bool:
    return character.isalnum() or character in "_$"


class _Source:
    def __init__(self, rows: list[str], language: str) -> None:
        self.text = "".join(rows)
        self.lifetimes = language == "Rust"
        self.regex_literals = language in SCRIPT_LANGUAGES
        self.offsets = [0]
        for row in rows:
            self.offsets.append(self.offsets[-1] + len(row))

    def line_of(self, index: int) -> int:
        return bisect.bisect_right(self.offsets, index)

    def word_at(self, index: int) -> str:
        end = index
        while end < len(self.text) and _is_name_char(self.text[end]):
            end += 1
        return self.text[index:end]

    def skip_string(self, index: int) -> int:
        quote = self.text[index]
        if quote == "'" and self.lifetimes and not self.text.startswith("'", index + 2):
            return index + 1
        position = index + 1
        while position < len(self.text):
            character = self.text[position]
            if character == "\\":
                position += 2
                continue
            if character == quote:
                return position + 1
            if quote == "`" and self.text.startswith("${", position):
                closed = self.skip_balanced(position + 2)
                if closed is None:
                    return len(self.text)
                position = closed
                continue
            if quote != "`" and character == "\n":
                return position
            position += 1
        return len(self.text)

    def significant(self, index: int) -> int:
        text = self.text
        while index < len(text):
            character = text[index]
            if character.isspace():
                index += 1
            elif text.startswith("//", index):
                newline = text.find("\n", index)
                index = len(text) if newline < 0 else newline
            elif text.startswith("/*", index):
                closed = text.find("*/", index + 2)
                index = len(text) if closed < 0 else closed + 2
            elif character in QUOTES:
                index = self.skip_string(index)
            elif character == "/" and self.regex_starts(index):
                index = self.skip_regex(index)
            else:
                return index
        return index

    def regex_starts(self, index: int) -> bool:
        if not self.regex_literals:
            return False
        back = index - 1
        while back >= 0 and self.text[back] in " \t":
            back -= 1
        if back < 0 or self.text[back] in REGEX_AFTER_CHARACTERS:
            return True
        start = back
        while start >= 0 and _is_name_char(self.text[start]):
            start -= 1
        return self.text[start + 1:back + 1] in REGEX_AFTER_WORDS

    def skip_regex(self, index: int) -> int:
        position = index + 1
        in_class = False
        while position < len(self.text):
            character = self.text[position]
            if character == "\\":
                position += 2
                continue
            if character == "\n":
                return position
            if in_class:
                in_class = character != "]"
            elif character == "[":
                in_class = True
            elif character == "/":
                position += 1
                while position < len(self.text) and self.text[position].isalpha():
                    position += 1
                return position
            position += 1
        return position

    def skip_balanced(self, index: int) -> int | None:
        depth = 1
        while True:
            index = self.significant(index)
            if index >= len(self.text):
                return None
            character = self.text[index]
            if character in OPENERS:
                depth += 1
            elif character in CLOSERS:
                depth -= 1
                if depth == 0:
                    return index + 1
            index += 1

    def skip_type_parameters(self, index: int) -> int | None:
        if self.text[index] == "[":
            closed = self.skip_balanced(index + 1)
        else:
            depth, closed = 0, None
            for position in range(index, len(self.text)):
                depth += {"<": 1, ">": -1}.get(self.text[position], 0)
                if depth == 0:
                    closed = position + 1
                    break
        if closed is None:
            return None
        opener = self.significant(closed)
        return opener if opener < len(self.text) and self.text[opener] == "(" else None


def _definition(source: _Source, language: str, number: int, found: re.Match[str]) -> tuple[str, int] | None:
    start = source.offsets[number - 1]
    if found.groupdict().get("arrow"):
        return _arrow_definition(source, start + found.end())
    index = start + found.end() - 1
    if source.text[index] in "[<":
        index = source.skip_type_parameters(index)
        if index is None:
            return None
    closed = source.skip_balanced(index + 1)
    return None if closed is None else _body_after_parameters(source, language, closed)


def _arrow_definition(source: _Source, index: int) -> tuple[str, int] | None:
    text = source.text
    index = source.significant(index)
    if source.word_at(index) == "async":
        index = source.significant(index + len("async"))
    if source.word_at(index) == "function":
        opener = text.find("(", index)
        closed = None if opener < 0 else source.skip_balanced(opener + 1)
        return None if closed is None else _body_after_parameters(source, "TypeScript", closed)
    if index < len(text) and text[index] == "(":
        closed = source.skip_balanced(index + 1)
        return None if closed is None else _arrow_after_parameters(source, closed)
    parameter = source.word_at(index)
    if not parameter:
        return None
    index = source.significant(index + len(parameter))
    return _arrow_body(source, index + 2) if text.startswith("=>", index) else None


def _arrow_after_parameters(source: _Source, index: int) -> tuple[str, int] | None:
    text = source.text
    annotated = False
    while True:
        index = source.significant(index)
        if index >= len(text):
            return None
        if text.startswith("=>", index):
            return _arrow_body(source, index + 2)
        character = text[index]
        if not annotated:
            if character != ":":
                return None
            annotated = True
        elif character in ";=":
            return None
        elif character in OPENERS:
            closed = source.skip_balanced(index + 1)
            if closed is None:
                return None
            index = closed
            continue
        index += 1


def _arrow_body(source: _Source, index: int) -> tuple[str, int]:
    index = source.significant(index)
    return (BLOCK if index < len(source.text) and source.text[index] == "{" else EXPRESSION, index)


def _body_after_parameters(source: _Source, language: str, index: int) -> tuple[str, int] | None:
    text = source.text
    annotated = False
    while True:
        index = source.significant(index)
        if index >= len(text):
            return None
        character = text[index]
        if character == "{":
            return BLOCK, index
        if language in SCRIPT_LANGUAGES and not annotated:
            if character != ":":
                return None
            annotated = True
        elif language == "Kotlin" and character == "=" and not text.startswith("==", index):
            return LINE, index + 1
        elif character == ";" or (language in SCRIPT_LANGUAGES and character == "="):
            return None
        elif character in OPENERS:
            closed = source.skip_balanced(index + 1)
            if closed is None:
                return None
            index = closed
            continue
        index += 1


def _end(source: _Source, body: tuple[str, int]) -> int | None:
    kind, index = body
    text = source.text
    if kind == BLOCK:
        closed = source.skip_balanced(index + 1)
        return None if closed is None else source.line_of(closed - 1)
    while index < len(text):
        if kind == EXPRESSION:
            index = source.significant(index)
            if index >= len(text):
                break
        character = text[index]
        if kind == EXPRESSION and character == ";":
            return source.line_of(index)
        if kind == LINE and character == "\n":
            return source.line_of(index)
        if kind == LINE and character in QUOTES:
            index = source.skip_string(index)
            continue
        if character in OPENERS:
            closed = source.skip_balanced(index + 1)
            if closed is None:
                return None
            index = closed
            continue
        index += 1
    return None if kind == EXPRESSION else source.line_of(len(text) - 1)


def _definitions(rows: list[str], language: str, pattern: re.Pattern[str]) -> list[tuple[int, re.Match[str], tuple[str, int] | None]]:
    source = _Source(rows, language)
    found_all = []
    for number, row in enumerate(rows, 1):
        found = pattern.match(row)
        if not found:
            continue
        body = None if language == "Python" else _definition(source, language, number, found)
        if language == "Python" or body is not None:
            found_all.append((number, found, body))
    return found_all


def function_span(repo_root: Path, relative: str, name: str) -> tuple[int, int]:
    language, start_form, end_form = _language(relative)
    rows = _rows(_cited_file(repo_root.resolve(), relative).read_bytes())
    pattern = re.compile(INDENT + start_form.format(name=re.escape(name)))
    starts = _definitions(rows, language, pattern)
    if len(starts) != 1:
        raise ValueError(
            f"{relative} defines no function named {name} in a form --function reads"
            f"{_defined_elsewhere(repo_root.resolve(), relative, language, pattern)}; a definition of another form is cited with --lines"
            if not starts
            else f"{relative} defines {name} {len(starts)} times; cite the one you mean with --lines"
        )
    start, found, body = starts[0]
    end = _python_end(rows, start, len(found["indent"])) if end_form == INDENTATION else _end(_Source(rows, language), body)
    if end is None:
        raise ValueError(f"{relative}: the body of {name} starting on line {start} does not end; cite it with --lines")
    return start, end


def _defined_elsewhere(repo_root: Path, relative: str, language: str, pattern: re.Pattern[str]) -> str:
    suffix = Path(relative).suffix
    found = []
    for path in repository_files(repo_root):
        if path == relative or not path.endswith(suffix):
            continue
        rows = _rows((repo_root / path).read_bytes())
        found.extend(f"{path}:{number}" for number, _, _ in _definitions(rows, language, pattern))
    found.sort()
    if not found:
        return ""
    return f"; it is defined in {found[0]}" if len(found) == 1 else "; it is defined in several files: " + ", ".join(found)


def _python_end(rows: list[str], start: int, indent: int) -> int | None:
    signature_end = next((number for number in range(start, len(rows) + 1) if rows[number - 1].rstrip().endswith(":")), None)
    if signature_end is None:
        return None
    end = signature_end
    for number in range(signature_end + 1, len(rows) + 1):
        row = rows[number - 1]
        if not row.strip():
            continue
        if len(row) - len(row.lstrip()) <= indent:
            break
        end = number
    return end


def citation(repo_root: Path, relative: str, start: int, end: int) -> dict[str, Any]:
    root = repo_root.resolve()
    path = _cited_file(root, relative)
    if path.stat().st_size > MAX_EVIDENCE_BYTES:
        raise ValueError(
            f"evidence source exceeds {MAX_EVIDENCE_BYTES} bytes: {relative}"
        )
    if not _valid_line_range(start, end):
        raise ValueError(
            f"a citation needs 1 <= start <= end, got start={start} end={end}"
        )
    content, rows = _read_lines(path)
    if end > len(rows):
        raise ValueError(
            f"{relative} has {len(rows)} lines; the citation asks for line {end}"
        )
    return {
        "path": path.relative_to(root).as_posix(),
        "lines": {"start": start, "end": end},
        "content_sha256": sha256_of(content),
        "excerpt_sha256": sha256_of(_excerpt(rows, start, end)),
    }


def excerpt_starts(repo_root: Path, relative: str, length: int, excerpt_sha256: str) -> list[int]:
    path = (repo_root / relative).resolve()
    if _source_verdict(path, relative, repo_root.resolve()):
        return []
    _, rows = _read_lines(path)
    return _starts_of(rows, length, excerpt_sha256)


def repeated_note(repo_root: Path, cited: dict[str, Any]) -> str | None:
    start, end = cited["lines"]["start"], cited["lines"]["end"]
    others = [
        line
        for line in excerpt_starts(repo_root, cited["path"], end - start + 1, cited["excerpt_sha256"])
        if line != start
    ]
    if not others:
        return None
    return (
        f"NOTE: the text of {cited['path']}:{start}-{end} also starts at line "
        f"{', '.join(map(str, others))}, so a later edit cannot tell the copies apart. "
        f"{WIDEN_TO_UNIQUE}"
    )


def hash_all(value: Any, repo_root: Path) -> Any:
    return with_references(value, lambda reference: _hashed(reference, repo_root))


def _hashed(reference: Any, repo_root: Path) -> Any:
    if _shape_error(reference) or "excerpt_sha256" in reference:
        return reference
    lines = reference["lines"]
    return {**reference, **citation(repo_root, reference["path"], lines["start"], lines["end"])}


def unclassified_paths(value: Any) -> list[str]:
    return sorted(
        {
            reference["path"]
            for reference in all_references(value)
            if isinstance(reference, dict)
            and reference.get("source_kind") == "unclassified"
        }
    )


def _shape_error(reference: Any) -> str | None:
    if not isinstance(reference, dict):
        return "evidence must be an object"
    lines = reference.get("lines")
    if not isinstance(reference.get("path"), str) or not isinstance(lines, dict):
        return "evidence requires path and lines"
    if not _valid_line_range(lines.get("start"), lines.get("end")):
        return "evidence line range is invalid"
    return None


def _source_verdict(path: Path, path_text: str, root: Path) -> dict[str, Any] | None:
    if not path.is_relative_to(root):
        return {"status": "invalid", "reason": "evidence path escapes repository"}
    if not path.is_file():
        return {"status": "missing", "path": path_text}
    if path.stat().st_size > MAX_EVIDENCE_BYTES:
        return {
            "status": "invalid",
            "reason": f"evidence source exceeds {MAX_EVIDENCE_BYTES} bytes",
            "path": path_text,
        }
    return None


def verify(reference: Any, repo_root: Path) -> dict[str, Any]:
    return verifier(repo_root)(reference)


ContentReader = Callable[[str], bytes | None]


def staged_content(repo_root: Path) -> ContentReader:
    root = repo_root.resolve()

    def git(*arguments: str) -> bytes:
        return checked_git_bytes(root, *arguments, failure=f"could not read the Git index of {root}").stdout

    @cache
    def read(path_text: str) -> bytes | None:
        wanted = path_text.encode("utf-8")
        for entry in git("ls-files", "--stage", "-z", "--", f":(literal){path_text}").split(b"\0"):
            meta, _, listed = entry.partition(b"\t")
            mode, _, rest = meta.partition(b" ")
            blob, _, stage = rest.partition(b" ")
            if listed == wanted and stage == b"0" and mode.startswith(b"100"):
                return git("cat-file", "blob", blob.decode("ascii"))
        return None

    return read


def _rows_from(root: Path, path_text: str, content_of: ContentReader) -> dict[str, Any] | list[str]:
    if not (root / path_text).resolve().is_relative_to(root):
        return {"status": "invalid", "reason": "evidence path escapes repository"}
    content = content_of(path_text)
    if content is None:
        return {"status": "missing", "path": path_text}
    if len(content) > MAX_EVIDENCE_BYTES:
        return {"status": "invalid", "reason": f"evidence source exceeds {MAX_EVIDENCE_BYTES} bytes", "path": path_text}
    return _rows(content)


def _disk_rows(root: Path, path_text: str) -> dict[str, Any] | list[str]:
    path = (root / path_text).resolve()
    return _source_verdict(path, path_text, root) or _read_lines(path)[1]


def verifier(repo_root: Path, content_of: ContentReader | None = None) -> Callable[[Any], dict[str, Any]]:
    root = repo_root.resolve()
    sources: dict[str, dict[str, Any] | list[str]] = {}
    windows: dict[tuple[str, int], dict[str, list[int]]] = {}

    def source(path_text: str) -> dict[str, Any] | list[str]:
        if path_text not in sources:
            sources[path_text] = (
                _disk_rows(root, path_text) if content_of is None else _rows_from(root, path_text, content_of)
            )
        return sources[path_text]

    def check(reference: Any) -> dict[str, Any]:
        shape = _shape_error(reference)
        if shape:
            return {"status": "invalid", "reason": shape}
        rows = source(reference["path"])
        if isinstance(rows, dict):
            return dict(rows)
        return {"cited_lines": dict(reference["lines"]), **_cited_lines_verdict(reference, reference["path"], rows, windows)}

    return check


def _cited_lines_verdict(reference: dict[str, Any], path_text: str, rows: list[str],
                         windows: dict[tuple[str, int], dict[str, list[int]]] | None = None) -> dict[str, Any]:
    start, end = reference["lines"]["start"], reference["lines"]["end"]
    if end > len(rows) and "excerpt_sha256" in reference:
        return _moved_or_stale(reference["excerpt_sha256"], path_text, rows, end - start + 1, windows)
    if end > len(rows):
        return {
            "status": "invalid",
            "reason": "evidence line range exceeds source",
            "path": path_text,
        }
    if "excerpt_sha256" not in reference:
        return {
            "status": "unhashed",
            "path": path_text,
            "reason": f"{path_text} was cited without an excerpt hash; regenerate the citation with cite",
        }
    if reference.get("excerpt_sha256") != sha256_of(_excerpt(rows, start, end)):
        return _moved_or_stale(reference["excerpt_sha256"], path_text, rows, end - start + 1, windows)
    return {
        "status": "current",
        "path": path_text,
        "source_kind": reference.get("source_kind", "unclassified"),
    }


def _starts_by_digest(rows: list[str], length: int) -> dict[str, list[int]]:
    starts: dict[str, list[int]] = {}
    for start in range(1, len(rows) - length + 2):
        starts.setdefault(sha256_of(_excerpt(rows, start, start + length - 1)), []).append(start)
    return starts


def _starts_of(rows: list[str], length: int, excerpt_sha256: str,
               windows: dict[tuple[str, int], dict[str, list[int]]] | None = None, path_text: str = "") -> list[int]:
    if windows is None:
        return _starts_by_digest(rows, length).get(excerpt_sha256, [])
    if (path_text, length) not in windows:
        windows[(path_text, length)] = _starts_by_digest(rows, length)
    return windows[(path_text, length)].get(excerpt_sha256, [])


def _moved_or_stale(excerpt_sha256: str, path_text: str, rows: list[str], length: int,
                    windows: dict[tuple[str, int], dict[str, list[int]]] | None = None) -> dict[str, Any]:
    starts = _starts_of(rows, length, excerpt_sha256, windows, path_text)
    if len(starts) != 1:
        return {
            "status": "stale",
            "path": path_text,
            "reason": f"the cited excerpt is {'repeated' if starts else 'no longer'} in {path_text}; "
            "check that the fact still holds, then recite it" + (f". {WIDEN_TO_UNIQUE}" if starts else ""),
        }
    return {
        "status": "shifted",
        "path": path_text,
        "lines": {"start": starts[0], "end": starts[0] + length - 1},
        "reason": f"the cited lines are unchanged but now sit elsewhere in {path_text}; this passes "
        "every check and needs no Change Package of its own; when one next touches the record, "
        f"relocate-evidence --from {path_text} --to {path_text} re-cites them and prints the supersedes",
    }


def cited_excerpts(record: dict[str, Any]) -> set[tuple[str, str]]:
    return {
        (reference["path"], reference["excerpt_sha256"])
        for reference in all_references(record)
        if isinstance(reference, dict) and isinstance(reference.get("path"), str)
        and isinstance(reference.get("excerpt_sha256"), str)
    }


def one_cites_within_the_other(one: dict[str, Any], other: dict[str, Any]) -> bool:
    mine, theirs = cited_excerpts(one), cited_excerpts(other)
    return bool(mine and theirs) and (mine <= theirs or theirs <= mine)


def all_references(value: Any, depth: int = 0) -> list[Any]:
    if depth > MAX_NESTING_DEPTH:
        raise ValueError(f"evidence document exceeds nesting depth {MAX_NESTING_DEPTH}")
    if isinstance(value, dict):
        result = []
        for key, child in value.items():
            if key == "evidence" and isinstance(child, list):
                result.extend(child)
            result.extend(all_references(child, depth + 1))
        return result
    if isinstance(value, list):
        return [entry for child in value for entry in all_references(child, depth + 1)]
    return []
