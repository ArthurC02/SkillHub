#!/usr/bin/env python3
"""Backfill the `artifacts` manifest rows M2's pipeline never wrote (04 丙-13).

M2 executed 159 runs and wrote **zero** rows into `artifacts`; the archives
themselves survived in object storage at
`run-artifacts/<run_id>/<attempt_id>/artifacts.tar`. M3's `recordArtifacts`
writes the rows from then on, so this is a one-off data repair, not a code fix.

Why it matters, in the order the damage happens:

  1. `buildRequest` sends `artifacts[]` to the judge. With no rows it sends an
     empty array, so the judge decides every criterion under "this run produced
     no output files" - and evaluations are append-only, so a wrong verdict
     stays in the revision list forever. That already happened once: the
     2026-08-23 suggest baseline evaluated 53 of these runs by hand and all 84
     evaluations carry the empty-manifest finding
     (docs/plans/mvp/m3/report-suggest-baseline.md §9).
  2. `artifactFindings()` then reports "the run reported success and no output
     files were recorded for it", which is literally true and reads as an
     accusation.

**The order is hard: backfill first, evaluate second.**

Reading a tar's index is not executing it (evaluation-design §2.2): nothing is
unpacked to disk, nothing is parsed by extension, and no bytes reach a model.
The member bodies are read for one purpose only - sha256 - which is how the
sandbox produced `content_hash` in the first place
(apps/sandbox/internal/sandbox/artifacts.go), so this recovers the original
value rather than inventing a plausible one.

Two values are deliberately NOT taken from the live write path:

  * `expires_at`. `InsertRunArtifact` hardcodes `now() + interval '30 days'`.
    Applying that to a run from last month invents a retention window that
    never existed - those bytes have been sitting there since the run, and
    whatever should have expired should have expired on its own clock. Derived
    from `runs.created_at` here.
  * `content_type`. The live path defaults to `application/octet-stream`
    because the provider never populates the field at all (`Artifact` in
    apps/sandbox/internal/sandbox/artifacts.go sets FileName, SizeBytes and
    ContentHash and nothing else). So this is not a compromise: it is exactly
    what the row would have said. Sniffing the bytes here would make the
    backfilled rows *better* than the real ones, which is its own kind of lie.

Fetch the archives first. Kept out of this script so it also runs against a
dump, and because the credentials belong in the caller's shell, not here:

    docker run --rm --network host -v "C:/tmp/b13tars:/out" -e AWS_ACCESS_KEY_ID=... -e AWS_SECRET_ACCESS_KEY=... -e AWS_DEFAULT_REGION=us-east-1 amazon/aws-cli --endpoint-url http://127.0.0.1:8333 s3 sync s3://skillhub/run-artifacts/ /out/

(Git Bash rewrites the container-side `/out/`; prefix the command with
`MSYS_NO_PATHCONV=1` there or the files land inside the container and the sync
reports success having downloaded nothing you can see.)

Then:

    python tools/content/backfill_artifacts.py --tar-dir C:/tmp/b13tars
    python tools/content/backfill_artifacts.py --tar-dir C:/tmp/b13tars --apply
"""

import argparse
import collections
import hashlib
import json
import os
import subprocess
import sys
import tarfile

PG_CONTAINER = os.getenv("PG_CONTAINER", "skillhub-postgres-1")
PG_USER = os.getenv("PG_USER", "skillhub")
PG_DB = os.getenv("PG_DB", "skillhub")


def psql(sql):
    """One read-only query, JSON back."""
    out = subprocess.run(
        ["docker", "exec", PG_CONTAINER, "psql", "-U", PG_USER, "-d", PG_DB, "-tAc",
         "select coalesce(json_agg(t), '[]') from (" + sql + ") t"],
        capture_output=True, text=True, encoding="utf-8", check=True,
    )
    return json.loads(out.stdout)


def quote(s):
    """Single-quoted SQL literal. Only file names reach this and they come out of
    a tar header rather than a user form - but they are still untrusted strings,
    and a manifest is not the place to learn that the hard way."""
    return "'" + s.replace("'", "''") + "'"


def manifest(path):
    """(file_name, size_bytes, content_hash) for every regular file in one tar.

    Anything that is not a regular file is skipped, matching what the provider
    put in: it writes only `tar.TypeReg` headers.
    """
    rows = []
    with tarfile.open(path) as t:
        for m in t.getmembers():
            if not m.isfile():
                continue
            body = t.extractfile(m).read()
            rows.append((m.name, len(body), hashlib.sha256(body).hexdigest()))
    return rows


def statement(run_id, workspace_id, created_at, key, name, size, digest):
    """One idempotent INSERT, the same shape as InsertRunArtifact's.

    `WHERE NOT EXISTS` rather than `ON CONFLICT` for the same reason the live
    query gives: there is no unique key to conflict on, and running this twice
    must not double the manifest.
    """
    return (
        "INSERT INTO artifacts (workspace_id, run_id, kind, file_name, "
        "content_type, size_bytes, content_hash, object_key, expires_at)\n"
        "SELECT " + quote(workspace_id) + "::uuid, " + quote(run_id) + "::uuid, "
        "'run_output', " + quote(name) + ", 'application/octet-stream', "
        + str(size) + ", " + quote(digest) + ", " + quote(key) + ", "
        + quote(created_at) + "::timestamptz + interval '30 days'\n"
        "WHERE NOT EXISTS (SELECT 1 FROM artifacts WHERE run_id = "
        + quote(run_id) + "::uuid AND kind = 'run_output' AND file_name = "
        + quote(name) + ");"
    )


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--tar-dir", required=True,
                    help="local mirror of the run-artifacts/ prefix")
    ap.add_argument("--apply", action="store_true",
                    help="execute the SQL instead of only printing it")
    args = ap.parse_args()

    # Every run, with its own artifact count. Counted rather than filtered out in
    # SQL so "already backfilled" is a number on the summary: a backfill that
    # quietly does nothing looks identical to one that quietly did everything.
    runs = {r["id"]: r for r in psql("""
        select r.id::text, r.workspace_id::text as workspace_id,
               r.created_at::text as created_at,
               (select count(*) from artifacts a where a.run_id = r.id) as existing
          from runs r
    """)}

    stmts, orphan_archives, already, total_rows = [], [], 0, 0
    per_run = collections.Counter()
    for root, _dirs, files in os.walk(args.tar_dir):
        if "artifacts.tar" not in files:
            continue
        attempt_id = os.path.basename(root)
        run_id = os.path.basename(os.path.dirname(root))
        run = runs.get(run_id)
        if run is None:
            # An archive whose run is gone. Reported, never guessed at: the
            # workspace_id is not recoverable from the object key, and a row in
            # the wrong workspace is worse than a missing one (iron rule 3).
            orphan_archives.append(run_id)
            continue
        if int(run["existing"]) > 0:
            already += 1
            continue
        key = "run-artifacts/" + run_id + "/" + attempt_id + "/artifacts.tar"
        for name, size, digest in manifest(os.path.join(root, "artifacts.tar")):
            total_rows += 1
            per_run[run_id] += 1
            stmts.append(statement(run_id, run["workspace_id"], run["created_at"],
                                   key, name, size, digest))

    say = lambda line: print(line, file=sys.stderr)  # noqa: E731
    say("archives read:       " + str(len(per_run) + already + len(orphan_archives)))
    say("runs to backfill:    " + str(len(per_run)))
    say("manifest rows:       " + str(total_rows))
    say("already backfilled:  " + str(already))
    say("archive with no run: " + str(len(orphan_archives)))
    for run_id in orphan_archives:
        say("  ! no run row for " + run_id + "; archive left alone")
    if per_run:
        counts = sorted(per_run.values())
        say("files per run:       min " + str(counts[0]) + " max " + str(counts[-1])
            + " median " + str(counts[len(counts) // 2]))

    sql = "\n".join(stmts)
    if not args.apply:
        print(sql)
        return 0
    if not stmts:
        say("nothing to apply")
        return 0
    # One transaction: a half-written manifest is worse than none, because the
    # judge would then be told about some of the files and reason from a list it
    # has no way to know is partial.
    proc = subprocess.run(
        ["docker", "exec", "-i", PG_CONTAINER, "psql", "-U", PG_USER, "-d", PG_DB,
         "-q", "-v", "ON_ERROR_STOP=1", "-1"],
        input=sql, capture_output=True, text=True, encoding="utf-8",
    )
    sys.stderr.write(proc.stdout + proc.stderr)
    if proc.returncode != 0:
        return proc.returncode
    say("applied " + str(total_rows) + " rows")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
