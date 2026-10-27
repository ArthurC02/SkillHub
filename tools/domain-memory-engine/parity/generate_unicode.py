import sys
import unicodedata
from pathlib import Path

TARGET = Path(__file__).resolve().parent.parent / "src" / "unicode_data.rs"
HANGUL = range(0xAC00, 0xD7A4)
SURROGATES = range(0xD800, 0xE000)
CODEPOINTS = [code for code in range(0x110000) if code not in SURROGATES]
PAIR = 2


def escaped(text: str) -> str:
    return '"' + "".join(f"\\u{{{ord(character):X}}}" for character in text) + '"'


def ranges(codes: list[int]) -> list[tuple[int, int]]:
    spans: list[tuple[int, int]] = []
    for code in codes:
        if spans and spans[-1][1] + 1 == code:
            spans[-1] = (spans[-1][0], code)
        else:
            spans.append((code, code))
    return spans


def composition_pairs() -> list[tuple[int, int, int]]:
    pairs = []
    for code in CODEPOINTS:
        if code in HANGUL:
            continue
        decomposition = unicodedata.decomposition(chr(code))
        if not decomposition or decomposition.startswith("<"):
            continue
        parts = [int(part, 16) for part in decomposition.split()]
        if len(parts) == PAIR and unicodedata.normalize("NFC", chr(parts[0]) + chr(parts[1])) == chr(code):
            pairs.append((parts[0], parts[1], code))
    return sorted(pairs)


def table(name: str, kind: str, rows: list[str]) -> str:
    body = "".join(f"    {row},\n" for row in rows)
    return f"pub const {name}: &[{kind}] = &[\n{body}];\n"


def main() -> None:
    if sys.version_info[:2] != (3, 13):
        sys.exit("generate the tables with Python 3.13, the version the parity harness runs")
    casefold = [(code, chr(code).casefold()) for code in CODEPOINTS if chr(code).casefold() != chr(code)]
    decompositions = [
        (code, unicodedata.normalize("NFKD", chr(code)))
        for code in CODEPOINTS
        if code not in HANGUL and unicodedata.normalize("NFKD", chr(code)) != chr(code)
    ]
    combining = [(code, unicodedata.combining(chr(code))) for code in CODEPOINTS if unicodedata.combining(chr(code))]
    alphanumeric = ranges([code for code in CODEPOINTS if chr(code).isalnum()])
    text = (
        table("CASEFOLD", "(u32, &str)", [f"(0x{code:X}, {escaped(folded)})" for code, folded in casefold])
        + "\n"
        + table("COMPAT_DECOMPOSITION", "(u32, &str)", [f"(0x{code:X}, {escaped(parts)})" for code, parts in decompositions])
        + "\n"
        + table("COMBINING_CLASS", "(u32, u8)", [f"(0x{code:X}, {value})" for code, value in combining])
        + "\n"
        + table("COMPOSITION", "(u32, u32, u32)", [f"(0x{a:X}, 0x{b:X}, 0x{c:X})" for a, b, c in composition_pairs()])
        + "\n"
        + table("ALPHANUMERIC", "(u32, u32)", [f"(0x{low:X}, 0x{high:X})" for low, high in alphanumeric])
    )
    TARGET.write_text(text, encoding="utf-8", newline="\n")


if __name__ == "__main__":
    main()
