#!/usr/bin/env python3
"""Import the CONTENT-003 seed skill list into a local dev Skill Hub.

Verification tool, not production code. It drives the public HTTP API only:
dev login (ADR-020), then one package upload per skill (INGEST-002).

Why upload and not URL import (INGEST-001): a seed entry points at
``<repo>/<dir>/SKILL.md`` inside a monorepo, while the URL importer only accepts
a package whose SKILL.md sits at the archive root (ingest.packageFS). One repo
zip therefore yields at most one importable skill. This tool downloads the
pinned-commit repo zip once per source and re-packs each skill directory into
its own archive with SKILL.md at the root.

Usage:
    python import_seed.py                      # import all skills
    python import_seed.py --only excel-insert  # substring filter on skill id
    python import_seed.py --probe-url          # INGEST-001 allow-list probes
    python import_seed.py --selftest           # offline check of the repacker
"""

from __future__ import annotations

import argparse
import http.cookiejar
import io
import json
import os
import pathlib
import sys
import tempfile
import urllib.error
import urllib.request
import zipfile

SEED = pathlib.Path(__file__).with_name("seed-skills.json")
DEFAULT_API = os.environ.get("SKILLHUB_API", "http://localhost:8080")


# --- HTTP -------------------------------------------------------------------


def make_opener() -> urllib.request.OpenerDirector:
    """Opener with a cookie jar so the dev session cookie is carried along."""
    return urllib.request.build_opener(
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
    )


def request(opener, url: str, *, data=None, ctype=None, timeout=120):
    """Return (status, body_bytes). HTTP error responses come back, not raise."""
    req = urllib.request.Request(url, data=data, method="POST" if data is not None else "GET")
    if ctype:
        req.add_header("Content-Type", ctype)
    try:
        with opener.open(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except urllib.error.URLError as e:
        return 0, str(e.reason).encode()


def dev_login(opener, api: str, user: str = "seed-importer") -> None:
    status, body = request(
        opener, f"{api}/auth/dev/login", data=json.dumps({"user": user}).encode(), ctype="application/json"
    )
    if status != 204:
        raise SystemExit(f"dev login failed ({status}): {body[:200]!r} — is DEV_LOGIN=1 set?")


# --- Packaging --------------------------------------------------------------


def download_repo_zip(source: dict, cache: pathlib.Path) -> pathlib.Path:
    """Fetch the repo archive at the pinned commit (seed schema: sources.<id>.commit)."""
    owner, repo = source["url"].rstrip("/").split("/")[-2:]
    commit = source["commit"]
    dest = cache / f"{owner}--{repo}--{commit}.zip"
    if dest.exists():
        return dest
    url = f"https://codeload.github.com/{owner}/{repo}/zip/{commit}"
    cache.mkdir(parents=True, exist_ok=True)
    with urllib.request.urlopen(url, timeout=180) as resp:
        dest.write_bytes(resp.read())
    return dest


def repack_skill(repo_zip: pathlib.Path, skill_md_path: str) -> bytes:
    """Re-root one skill directory of a repo archive into its own package zip.

    The repo archive has a single ``<repo>-<sha>/`` top directory; entries under
    the skill's directory are rewritten so SKILL.md lands at the package root.

    Entry timestamps are pinned to a constant so the same commit always produces
    the same bytes. INGEST-005 dedupes on the archive's sha256, so a wall-clock
    timestamp would make every re-import a brand new version of every skill.
    """
    fixed_time = (1980, 1, 1, 0, 0, 0)
    with zipfile.ZipFile(repo_zip) as src:
        names = src.namelist()
        top = names[0].split("/", 1)[0] + "/"
        skill_dir = skill_md_path.rsplit("/", 1)[0] + "/" if "/" in skill_md_path else ""
        prefix = top + skill_dir
        members = [n for n in names if n.startswith(prefix) and not n.endswith("/")]
        if not members:
            raise FileNotFoundError(f"{skill_dir!r} not found in {repo_zip.name}")
        buf = io.BytesIO()
        with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as out:
            for name in sorted(members):
                info = zipfile.ZipInfo(name[len(prefix) :], date_time=fixed_time)
                info.compress_type = zipfile.ZIP_DEFLATED
                out.writestr(info, src.read(name))
    return buf.getvalue()


# --- Import -----------------------------------------------------------------


def classify(status: int, payload) -> str:
    """Map an import response to a report bucket."""
    if status == 201:
        return "duplicate" if payload.get("duplicate") else "imported"
    if status == 422:
        return "rejected_validation"  # INGEST-006: blocking findings
    return f"error_http_{status}"


def import_one(opener, api: str, zip_bytes: bytes) -> tuple[str, dict]:
    status, body = request(
        opener, f"{api}/skills/import/upload", data=zip_bytes, ctype="application/zip"
    )
    try:
        payload = json.loads(body)
    except (json.JSONDecodeError, UnicodeDecodeError):
        payload = {"raw": body[:400].decode("utf-8", "replace")}
    return classify(status, payload), payload


def findings_of(payload: dict) -> dict:
    """Findings live at the top level on 422 and under "findings" on 201."""
    f = payload.get("findings", payload)
    return {k: f.get(k) or [] for k in ("errors", "warnings", "infos")}


def run(args) -> int:
    seed = json.loads(SEED.read_text(encoding="utf-8"))
    sources, skills = seed["sources"], seed["skills"]
    if args.only:
        skills = [s for s in skills if args.only in s["id"]]

    cache = pathlib.Path(args.cache)
    opener = make_opener()
    dev_login(opener, args.api)

    results = []
    for i, skill in enumerate(skills, 1):
        row = {k: skill[k] for k in ("id", "name", "category", "tier", "source_id")}
        try:
            repo_zip = download_repo_zip(sources[skill["source_id"]], cache)
            zip_bytes = repack_skill(repo_zip, skill["skill_md_path"])
            row["package_bytes"] = len(zip_bytes)
            row["status"], payload = import_one(opener, args.api, zip_bytes)
            row["findings"] = findings_of(payload)
            row["skill_id"] = payload.get("skill_id")
        except Exception as e:  # fetch/repack failure is a result, not a crash
            row["status"] = "error_fetch"
            row["findings"] = {"errors": [{"code": "fetch", "message": str(e)[:300]}]}
        print(f"[{i:>2}/{len(skills)}] {row['id']:<40} {row['status']}", flush=True)
        results.append(row)

    pathlib.Path(args.out).write_text(
        json.dumps({"api": args.api, "results": results}, ensure_ascii=False, indent=1),
        encoding="utf-8",
    )
    print(f"\nwrote {args.out}")
    for bucket in sorted({r["status"] for r in results}):
        print(f"  {bucket}: {sum(1 for r in results if r['status'] == bucket)}")
    return 0


def probe_url(args) -> int:
    """INGEST-001 reality check against the URL-import allow list."""
    seed = json.loads(SEED.read_text(encoding="utf-8"))
    src = seed["sources"]["yuyy-excel"]
    owner, repo = src["url"].rstrip("/").split("/")[-2:]
    probes = [
        ("raw.githubusercontent.com (host allow-list)",
         f"https://raw.githubusercontent.com/{owner}/{repo}/{src['commit']}/claude/skills/excel-insert/SKILL.md"),
        ("github.com repo root (pin ignored, SKILL.md not at root)", src["url"]),
        ("codeload pinned-commit archive", f"https://codeload.github.com/{owner}/{repo}/zip/{src['commit']}"),
    ]
    opener = make_opener()
    dev_login(opener, args.api)
    for label, url in probes:
        status, body = request(
            opener, f"{args.api}/skills/import/url",
            data=json.dumps({"url": url}).encode(), ctype="application/json",
        )
        print(f"{label}\n  {url}\n  -> {status} {body[:300].decode('utf-8', 'replace')}\n")
    return 0


def selftest() -> int:
    """Offline check: the repacker must put SKILL.md at the package root."""
    tmp = pathlib.Path(tempfile.mkdtemp()) / "repo.zip"
    with zipfile.ZipFile(tmp, "w") as z:
        z.writestr("repo-abc123/claude/skills/demo/SKILL.md", "---\nname: demo\n---\nbody")
        z.writestr("repo-abc123/claude/skills/demo/ref/guide.md", "guide")
        z.writestr("repo-abc123/claude/skills/other/SKILL.md", "other")
        z.writestr("repo-abc123/README.md", "readme")
    names = set(zipfile.ZipFile(io.BytesIO(
        repack_skill(tmp, "claude/skills/demo/SKILL.md")
    )).namelist())
    assert names == {"SKILL.md", "ref/guide.md"}, names

    with zipfile.ZipFile(tmp, "w") as z:
        z.writestr("repo-abc123/SKILL.md", "root")
    assert set(zipfile.ZipFile(io.BytesIO(
        repack_skill(tmp, "SKILL.md")
    )).namelist()) == {"SKILL.md"}

    try:
        repack_skill(tmp, "nope/SKILL.md")
    except FileNotFoundError:
        pass
    else:
        raise AssertionError("missing skill directory must raise")

    # Determinism: the same input must repack to identical bytes, or INGEST-005
    # dedupe never fires and every re-run duplicates the whole catalog.
    with zipfile.ZipFile(tmp, "w") as z:
        z.writestr("repo-abc123/skills/demo/SKILL.md", "---\nname: demo\n---\nbody")
    stamps = {
        i.date_time
        for i in zipfile.ZipFile(io.BytesIO(repack_skill(tmp, "skills/demo/SKILL.md"))).infolist()
    }
    assert stamps == {(1980, 1, 1, 0, 0, 0)}, stamps

    assert classify(201, {"duplicate": True}) == "duplicate"
    assert classify(422, {}) == "rejected_validation"
    print("selftest ok")
    return 0


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--api", default=DEFAULT_API)
    p.add_argument("--only", help="substring filter on skill id")
    p.add_argument("--cache", default=os.path.join(tempfile.gettempdir(), "skillhub-seed-cache"))
    p.add_argument("--out", default=os.path.join(tempfile.gettempdir(), "import-results.json"))
    p.add_argument("--probe-url", action="store_true", help="probe the INGEST-001 URL importer")
    p.add_argument("--selftest", action="store_true", help="offline repacker check")
    args = p.parse_args()
    if args.selftest:
        return selftest()
    if args.probe_url:
        return probe_url(args)
    return run(args)


if __name__ == "__main__":
    sys.exit(main())
