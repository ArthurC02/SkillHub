#!/usr/bin/env python3
"""QA-002: generate the deliberately broken half of the spec-validation corpus.

The 45 pinned-commit seed packages are the *legal* samples and already have a
real run through `skillpkg` behind them (CONTENT-006). They only ever prove that
validation does not reject good input. This script produces the other half: one
mutated package per failure mode, each with an expected finding list in
`expected-findings.json` that `TestQA002BrokenPackageCorpus` compares against.

Verification tool, not production code: it is not imported by any service, it is
not in CI, and it needs network once per base package (the pinned repo zips).

Design notes
------------
* **Script, not 45 committed binaries.** Mutations are one function each, so a
  reader can see exactly what makes a variant broken. Committed zips would be
  opaque and would rot against `skillpkg` without anyone noticing.
* **Bases are real seed packages, not synthetic ones** (`03` QA-002: "不重新蒐集
  語料"). Repacking reuses `tools/content/import_seed.py` — the packer and the
  scanner must agree about package shape, and two copies of that logic would
  eventually disagree.
* **Fake credentials are assembled at runtime**, never written as one literal.
  A corpus that trips the repo's own secret scan is a corpus nobody can commit.

Usage:
    python generate.py --out <dir>        # write base-*.zip and every variant
    python generate.py --list             # variant ids and what each breaks
    python generate.py --selftest         # offline check of the mutators
"""

from __future__ import annotations

import argparse
import importlib.util
import io
import json
import os
import pathlib
import sys
import tempfile
import zipfile

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parents[2]
EXPECTED = HERE / "expected-findings.json"

# Fixed timestamp, same reason as import_seed.repack_skill: a wall-clock stamp
# would make every regeneration produce different bytes for identical content.
FIXED_TIME = (1980, 1, 1, 0, 0, 0)


def _load_import_seed():
    """Import tools/content/import_seed.py by path (it is not a package)."""
    path = REPO / "tools" / "content" / "import_seed.py"
    spec = importlib.util.spec_from_file_location("import_seed", path)
    if spec is None or spec.loader is None:  # pragma: no cover - environment
        raise SystemExit(f"cannot load {path}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


# --- bases ------------------------------------------------------------------

# Three seed packages, chosen for three different shapes so a mutation lands on
# a realistic neighbourhood rather than on an empty package:
#   humanizer    prompt-only, no scripts, no dependency manifest
#   csv-to-json  data skill, declares dependencies, no shipped script
#   excel-freeze documents skill, embedded Python and reference files
# All three are MIT (seed-skills.json sources), so the corpus carries no
# source-available content.
BASES = ["humanizer/humanizer", "wrangler/csv-to-json", "yuyy-excel/excel-freeze"]


def base_packages(cache: pathlib.Path) -> dict[str, bytes]:
    """Fetch and repack the base packages, keyed by their short skill name."""
    seed = json.loads(
        (REPO / "tools" / "content" / "seed-skills.json").read_text(encoding="utf-8")
    )
    sources = seed["sources"]
    by_id = {s["id"]: s for s in seed["skills"]}
    mod = _load_import_seed()

    out: dict[str, bytes] = {}
    for sid in BASES:
        skill = by_id[sid]
        src = sources[skill["source_id"]]
        repo_zip = mod.download_repo_zip(src, cache)
        out[skill["name"]] = mod.repack_skill(repo_zip, skill["skill_md_path"], src)
    return out


# --- zip helpers ------------------------------------------------------------


def read_zip(data: bytes) -> list[tuple[str, bytes]]:
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        return [(n, z.read(n)) for n in z.namelist()]


def write_zip(entries, *, symlinks=()) -> bytes:
    """Build a zip. `symlinks` names entries to mark with the symlink mode bit."""
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        for name, data in entries:
            info = zipfile.ZipInfo(name, date_time=FIXED_TIME)
            info.compress_type = zipfile.ZIP_DEFLATED
            if name in symlinks:
                # 0o120777 << 16: the Unix mode a zip carries for a symlink. The
                # entry body is the link target, which is how a zip stores one.
                info.external_attr = (0o120777 << 16) | 0o20
                info.create_system = 3
            z.writestr(info, data)
    return buf.getvalue()


def edit(entries, name, fn):
    """Return entries with `name`'s text content replaced by fn(text)."""
    out = []
    for n, data in entries:
        if n == name:
            data = fn(data.decode("utf-8")).encode("utf-8")
        out.append((n, data))
    return out


def add(entries, name, data: bytes):
    return list(entries) + [(name, data)]


def drop(entries, name):
    return [(n, d) for n, d in entries if n != name]


def split_frontmatter(md: str) -> tuple[str, str]:
    """Return (frontmatter_body, rest) for a `---\\n...\\n---\\n` header."""
    assert md.startswith("---\n"), "base SKILL.md must open with frontmatter"
    fm, _, rest = md[4:].partition("\n---\n")
    return fm, rest


def strip_hr_lines(body: str) -> str:
    """Drop every line that is exactly `---`.

    Needed by the unterminated-frontmatter variant: `cutClosingDelimiter` closes
    the header at the *first* bare `---` anywhere in the file, and a 29 KiB
    SKILL.md has several as horizontal rules. Without this the variant tested
    `frontmatter-invalid-yaml` while claiming to test the missing delimiter —
    a corpus entry whose name and behaviour disagree is worse than no entry.
    """
    return "\n".join(line for line in body.split("\n") if line.strip() != "---")


def set_field(md: str, key: str, value: str | None) -> str:
    """Replace (or delete, when value is None) one top-level frontmatter key."""
    fm, rest = split_frontmatter(md)
    kept, replaced = [], False
    for line in fm.split("\n"):
        if line.startswith(f"{key}:"):
            replaced = True
            if value is not None:
                kept.append(f"{key}: {value}")
            continue
        kept.append(line)
    if value is not None and not replaced:
        kept.append(f"{key}: {value}")
    return "---\n" + "\n".join(kept) + "\n---\n" + rest


# --- the fake credentials ---------------------------------------------------
#
# Assembled from parts so the source file itself contains no string that a
# credential scanner (this repo's own `git grep` pre-push check included) would
# flag. The value still matches skillpkg.secretPatterns once assembled, which is
# the whole point of the variant.
FAKE_AWS_KEY = "AKIA" + "EXAMPLE" + "0" * 5 + "NOTREAL"  # AKIA + 16 chars
FAKE_PEM = "-----BEGIN " + "RSA PRIVATE KEY" + "-----\nnot-a-real-key\n"

EMBEDDED_PYTHON = "```python\n" + "".join(
    f"value_{i} = compute(value_{i - 1})\n" for i in range(1, 61)
) + "```\n"

MEGABYTE_TEXT = ("the content scan stops above one mebibyte. " * 26_000).encode()


# --- mutations --------------------------------------------------------------
#
# Each entry: (variant id, base skill name, what it breaks, mutator).
# The mutator takes the base's [(name, bytes)] and returns either new entries or
# a (entries, symlink-names) pair.

MUTATIONS: list[tuple[str, str, str, object]] = [
    # -- SKILL.md missing or structurally unreadable --------------------------
    ("skill-md-missing", "humanizer", "SKILL.md deleted from the package root",
     lambda e: drop(e, "SKILL.md")),
    ("frontmatter-missing", "humanizer", "SKILL.md starts with prose, no `---` header",
     lambda e: edit(e, "SKILL.md", lambda md: "# Humanizer\n\n" + split_frontmatter(md)[1])),
    ("frontmatter-unterminated", "humanizer", "opening `---` with no closing delimiter",
     lambda e: edit(e, "SKILL.md",
                    lambda md: "---\n" + split_frontmatter(md)[0] + "\n"
                    + strip_hr_lines(split_frontmatter(md)[1]))),
    ("frontmatter-invalid-yaml", "humanizer", "frontmatter is not parseable YAML",
     lambda e: edit(e, "SKILL.md", lambda md: set_field(md, "name", "[unclosed, list"))),

    # -- required fields and the name rule ------------------------------------
    ("name-missing", "humanizer", "frontmatter has no `name`",
     lambda e: edit(e, "SKILL.md", lambda md: set_field(md, "name", None))),
    ("name-invalid", "humanizer", "`name` breaks the lowercase-hyphen rule",
     lambda e: edit(e, "SKILL.md", lambda md: set_field(md, "name", "Humanizer_Pro!"))),
    ("name-too-long", "humanizer", "`name` is 65 runes, one over the cap",
     lambda e: edit(e, "SKILL.md", lambda md: set_field(md, "name", "a" + "-b" * 32))),
    ("description-missing", "csv-to-json", "frontmatter has no `description`",
     lambda e: edit(e, "SKILL.md", lambda md: set_field(md, "description", None))),
    ("description-too-long", "csv-to-json", "`description` is 1025 runes, one over the cap",
     lambda e: edit(e, "SKILL.md", lambda md: set_field(md, "description", "d" * 1025))),
    ("frontmatter-unknown-field", "csv-to-json", "an unrecognised frontmatter key",
     lambda e: edit(e, "SKILL.md", lambda md: set_field(md, "auto_run", "true"))),

    # -- file references leaving the package -----------------------------------
    ("file-ref-escapes", "excel-freeze", "SKILL.md links to a path outside the package",
     lambda e: edit(e, "SKILL.md",
                    lambda md: md + "\n\nSee [the host config](../../../etc/passwd).\n")),
    ("file-ref-missing", "excel-freeze", "SKILL.md links to a file the package does not ship",
     lambda e: edit(e, "SKILL.md",
                    lambda md: md + "\n\nSee [the helper](reference/not-shipped.md).\n")),

    # -- executable content ----------------------------------------------------
    ("embedded-script", "humanizer", "60 lines of Python written into SKILL.md itself",
     lambda e: edit(e, "SKILL.md", lambda md: md + "\n\n" + EMBEDDED_PYTHON)),
    ("binary-file", "csv-to-json", "a compiled object nobody can review as text",
     lambda e: add(e, "bin/helper.so", b"\x7fELF\x02\x01\x01\x00" + b"\x00" * 512)),

    # -- credentials -----------------------------------------------------------
    ("secret-in-script", "csv-to-json", "an AWS access key id inside a shipped script",
     lambda e: add(e, "scripts/deploy.sh",
                   f"#!/bin/sh\nexport AWS_ACCESS_KEY_ID={FAKE_AWS_KEY}\n".encode())),
    ("secret-in-skill-md", "humanizer", "private key material pasted into SKILL.md",
     lambda e: edit(e, "SKILL.md", lambda md: md + "\n\n```\n" + FAKE_PEM + "```\n")),

    # -- archive-level attacks -------------------------------------------------
    ("zip-path-traversal", "humanizer", "a zip entry whose name walks out of the package",
     lambda e: add(e, "../../evil.sh", b"#!/bin/sh\nrm -rf /\n")),
    ("zip-absolute-path", "humanizer", "a zip entry with an absolute path",
     lambda e: add(e, "/etc/cron.d/evil", b"* * * * * root /evil.sh\n")),
    ("zip-symlink-escape", "humanizer", "a symlink entry pointing outside the package",
     lambda e: (add(e, "reference/host-passwd", b"/etc/passwd"), ("reference/host-passwd",))),
    ("zip-bomb", "humanizer", "declared uncompressed size over the unpack cap",
     "ZIP_BOMB"),
    ("oversize-file", "humanizer", "a file above the content-scan size cap",
     lambda e: add(e, "reference/corpus.txt", MEGABYTE_TEXT)),
]


def build_zip_bomb(entries) -> bytes:
    """A real bomb: 260 MiB of zeros, which deflates to a few hundred KiB.

    Not a doctored header. `ingest.PackageFS` reads the declared uncompressed
    size out of the central directory, and a corpus that lies in the header
    would stop testing the cap the day that reader changes how it measures.
    """
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        for name, data in entries:
            info = zipfile.ZipInfo(name, date_time=FIXED_TIME)
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, data)
        info = zipfile.ZipInfo("reference/filler.bin", date_time=FIXED_TIME)
        info.compress_type = zipfile.ZIP_DEFLATED
        chunk = b"\0" * (1 << 20)
        with z.open(info, "w") as fh:
            for _ in range(260):
                fh.write(chunk)
    return buf.getvalue()


def apply_mutation(mutator, entries) -> bytes:
    if mutator == "ZIP_BOMB":
        return build_zip_bomb(entries)
    result = mutator(entries)
    if isinstance(result, tuple):
        new_entries, symlinks = result
        return write_zip(new_entries, symlinks=symlinks)
    return write_zip(result)


# --- commands ---------------------------------------------------------------


def cmd_generate(args) -> int:
    out = pathlib.Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    bases = base_packages(pathlib.Path(args.cache))

    written = []
    for name, data in sorted(bases.items()):
        path = out / f"base-{name}.zip"
        path.write_bytes(data)
        written.append((path.name, len(data)))

    entries_by_base = {name: read_zip(data) for name, data in bases.items()}
    for vid, base, _what, mutator in MUTATIONS:
        data = apply_mutation(mutator, entries_by_base[base])
        path = out / f"{vid}.zip"
        path.write_bytes(data)
        written.append((path.name, len(data)))

    for name, size in written:
        print(f"{name:<34} {size:>10,} bytes")
    print(f"\n{len(bases)} base package(s) + {len(MUTATIONS)} variant(s) in {out}")
    print(f"expectations: {EXPECTED}")
    return 0


def cmd_list(_args) -> int:
    print(f"{'variant':<28} {'base':<14} breaks")
    for vid, base, what, _ in MUTATIONS:
        print(f"{vid:<28} {base:<14} {what}")
    return 0


def selftest() -> int:
    """Offline: every mutator must produce a readable zip that actually differs."""
    base = [
        ("SKILL.md", b"---\nname: demo\ndescription: A demo skill.\n---\n\nBody text.\n"),
        ("reference/guide.md", b"guide\n"),
    ]
    entries_by_base = {"humanizer": base, "csv-to-json": base, "excel-freeze": base}

    seen = set()
    for vid, b, _what, mutator in MUTATIONS:
        assert vid not in seen, f"duplicate variant id {vid}"
        seen.add(vid)
        data = apply_mutation(mutator, entries_by_base[b])
        names = zipfile.ZipFile(io.BytesIO(data)).namelist()
        assert names, vid
        assert data != write_zip(base), f"{vid} did not change anything"

    # The mutators that must produce a specific structural shape.
    assert "SKILL.md" not in zipfile.ZipFile(io.BytesIO(
        apply_mutation(dict((m[0], m[3]) for m in MUTATIONS)["skill-md-missing"], base)
    )).namelist()

    traversal = zipfile.ZipFile(io.BytesIO(apply_mutation(
        dict((m[0], m[3]) for m in MUTATIONS)["zip-path-traversal"], base)))
    assert "../../evil.sh" in traversal.namelist(), traversal.namelist()

    symlinked = zipfile.ZipFile(io.BytesIO(apply_mutation(
        dict((m[0], m[3]) for m in MUTATIONS)["zip-symlink-escape"], base)))
    info = symlinked.getinfo("reference/host-passwd")
    assert (info.external_attr >> 16) & 0o170000 == 0o120000, oct(info.external_attr)

    # Frontmatter editing must stay surgical.
    md = base[0][1].decode()
    assert "name:" not in set_field(md, "name", None)
    assert "name: renamed" in set_field(md, "name", "renamed")
    assert "auto_run: true" in set_field(md, "auto_run", "true")
    assert split_frontmatter(set_field(md, "name", "x"))[1] == "\nBody text.\n"

    # The credential literals must be assembled, never present as one string.
    src = pathlib.Path(__file__).read_text(encoding="utf-8")
    assert FAKE_AWS_KEY not in src, "assembled key leaked into the source"
    assert FAKE_PEM.split("\n")[0] not in src, "assembled PEM header leaked into the source"

    # Every variant must be described in the expectations file, and vice versa.
    if EXPECTED.exists():
        expected = json.loads(EXPECTED.read_text(encoding="utf-8"))["variants"]
        assert set(expected) == seen | {f"base-{b}" for b in BASES_SHORT}, (
            sorted(set(expected) ^ (seen | {f'base-{b}' for b in BASES_SHORT}))
        )

    print(f"selftest ok ({len(MUTATIONS)} variants)")
    return 0


BASES_SHORT = ["humanizer", "csv-to-json", "excel-freeze"]


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--out", default=os.path.join(tempfile.gettempdir(), "skillpkg-corpus"))
    p.add_argument("--cache", default=os.path.join(tempfile.gettempdir(), "skillhub-seed-cache"),
                   help="shared with import_seed.py; pinned repo zips land here")
    p.add_argument("--list", action="store_true", help="print the variant table and exit")
    p.add_argument("--selftest", action="store_true", help="offline check of the mutators")
    args = p.parse_args()
    if args.selftest:
        return selftest()
    if args.list:
        return cmd_list(args)
    return cmd_generate(args)


if __name__ == "__main__":
    sys.exit(main())
