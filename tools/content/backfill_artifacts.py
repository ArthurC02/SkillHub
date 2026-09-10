#!/usr/bin/env python3
"""Backfill `artifacts` manifest rows for runs whose pipeline never wrote them.

Reads each run's archive from a local tar directory (fetch it first, e.g. via
`aws s3 sync s3://skillhub/run-artifacts/ <dir>`) and hashes member bodies
with sha256 to recover the same content_hash the sandbox originally produced.

    python tools/content/backfill_artifacts.py --tar-dir <dir> [--apply]
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
    """Single-quoted SQL literal; tar-header file names are still untrusted."""
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

    `WHERE NOT EXISTS` rather than `ON CONFLICT`: there is no unique key to
    conflict on.
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
    # `-1` runs every statement in one transaction: a half-written manifest is
    # worse than none, since a partial one looks complete to anything reading it.
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
