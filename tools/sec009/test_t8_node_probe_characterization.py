#!/usr/bin/env python3
"""Characterization of the node probe's C-01 and P-05 grading, with docker
and /proc replaced by fakes: no command runs and no real process is read."""

import builtins
import contextlib
import importlib.util
import io
import json
import os
import sys
import tempfile
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest import mock

_spec = importlib.util.spec_from_file_location("t8_node_probe", Path(__file__).with_name("t8-node-probe.py"))
probe = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(probe)

RUNTIMES = ("docker", "info", "--format", "{{range $k, $v := .Runtimes}}{{$k}} {{end}}")
PS = ("docker", "ps", "-q", "--no-trunc")


def c01_rows(answers):
    def fake_sh(*cmd):
        if cmd[:2] == ("docker", "inspect"):
            return answers["inspect"](cmd)
        return answers[cmd]
    rep = probe.Report()
    with mock.patch.object(probe, "sh", fake_sh):
        probe.check_c01(rep)
    return [(r["id"], r["status"], r["detail"]) for r in rep.rows]


ELSEWHERE_ROW = ("C-01c", probe.ELSEWHERE, None)
HOW_2 = ("read `docker inspect .Mounts` on 2 container(s); a Type=bind mount with RW=true is a "
         "writable host path (dockerdrv keeps Binds and Mounts empty on purpose -- C-05/C-07)")


def without_c01c_detail(rows):
    assert rows[-1][:2] == ELSEWHERE_ROW[:2], rows[-1]
    assert rows[-1][2].startswith("NOT MEASURABLE FROM A SNAPSHOT OF ONE NODE"), rows[-1]
    return rows[:-1]


def test_docker_unreadable_leaves_both_declarative_rows_unknown():
    rows = c01_rows({RUNTIMES: (-1, "docker is not installed on this node"), PS: (1, "")})
    assert without_c01c_detail(rows) == [
        ("C-01a", probe.UNKNOWN, "cannot read `docker info`, so we cannot see which runtimes dockerd offers"),
        ("C-01b", probe.UNKNOWN, "cannot list containers, so their mounts were not inspected"),
    ], rows


def test_no_runsc_fails_and_no_containers_passes():
    rows = c01_rows({RUNTIMES: (0, "io.containerd.runc.v2 runc"), PS: (0, "  \n")})
    assert without_c01c_detail(rows) == [
        ("C-01a", probe.FAIL, "`docker info` lists io.containerd.runc.v2 runc -- no runsc, so sandboxd's "
                              "Runtime:\"runsc\" would fail to start rather than silently fall back, but "
                              "this node cannot host Runs"),
        ("C-01b", probe.PASS, "no containers are running, so no host path is shared by any"),
    ], rows


def test_an_empty_runtime_list_is_named_as_none():
    rows = c01_rows({RUNTIMES: (0, ""), PS: (0, "")})
    assert rows[0][2].startswith("`docker info` lists <none> -- no runsc"), rows[0]


def test_writable_binds_fail_and_only_they_are_named():
    mounts = {
        "/run-a": [{"Type": "bind", "RW": True, "Source": "/srv/b"}, {"Type": "volume", "RW": True},
                   {"Type": "bind", "RW": True, "Source": "/srv/a"}],
        "/run-b": [{"Type": "bind", "RW": False, "Source": "/ro"}],
    }
    inspect_out = "\n".join(f"{name}|{json.dumps(m)}" for name, m in mounts.items()) + "\n/broken|{not json"
    seen = []

    def inspect(cmd):
        seen.append(cmd)
        return 0, inspect_out
    rows = c01_rows({RUNTIMES: (0, "runc runsc"), PS: (0, "id1\nid2\n"), "inspect": inspect})
    assert seen == [("docker", "inspect", "--format", "{{.Name}}|{{json .Mounts}}", "id1", "id2")], seen
    assert without_c01c_detail(rows) == [
        ("C-01a", probe.PASS, "`docker info` lists: runc runsc"),
        ("C-01b", probe.FAIL, HOW_2 + " -- run-a:/srv/a(rw), run-a:/srv/b(rw)"),
    ], rows


def test_read_only_mounts_pass_and_a_failed_inspect_is_unknown():
    ok = c01_rows({RUNTIMES: (0, "runsc"), PS: (0, "id1 id2"),
                   "inspect": lambda cmd: (0, '/a|[{"Type": "bind", "RW": false}]\n/b|null')})
    assert without_c01c_detail(ok)[1] == ("C-01b", probe.PASS, HOW_2), ok
    failed = c01_rows({RUNTIMES: (0, "runsc"), PS: (0, "id1"), "inspect": lambda cmd: (1, "boom")})
    assert without_c01c_detail(failed)[1] == (
        "C-01b", probe.UNKNOWN, "`docker inspect` failed, so mounts were not read"), failed


def p05_row(cred_paths, environs=None):
    real_isdir, real_listdir, real_open = os.path.isdir, os.listdir, builtins.open
    environs = environs or {}

    def isdir(path):
        return environs != {} if path == "/proc" else real_isdir(path)

    def listdir(path):
        return list(environs) + ["self"] if path == "/proc" else real_listdir(path)

    def fake_open(path, *args, **kwargs):
        if isinstance(path, str) and path.startswith("/proc/"):
            pid = path.split("/")[2]
            if environs[pid] is None:
                raise PermissionError(path)
            return io.BytesIO(environs[pid])
        return real_open(path, *args, **kwargs)
    rep = probe.Report()
    with mock.patch.object(probe, "CRED_PATHS", cred_paths), \
            mock.patch.object(probe.os.path, "isdir", isdir), \
            mock.patch.object(probe.os, "listdir", listdir), \
            mock.patch.object(builtins, "open", fake_open):
        probe.check_p05(rep)
    assert len(rep.rows) == 1
    return rep.rows[0]["status"], rep.rows[0]["detail"]


NAMES = "SKILLHUB_DATABASE_URL, DATABASE_URL, PGPASSWORD, SKILLHUB_SECRETS_TOKEN"


def test_a_clean_node_without_proc_passes_and_says_what_it_scanned():
    with tempfile.TemporaryDirectory() as root:
        (Path(root) / "clean.env").write_text("LOG_LEVEL=info\nPGPASSWORD=\n", encoding="utf-8")
        missing = os.path.join(root, "absent")
        status, detail = p05_row((root, missing))
    assert status == probe.PASS
    assert detail == (f"scanned no /proc, so running processes were not scanned; {root}; {missing} "
                      f"for the names {NAMES} and for postgres:// URLs (values deliberately not printed)")


def test_every_credential_shape_is_found_by_location_and_never_by_value():
    with tempfile.TemporaryDirectory() as root:
        conf = Path(root) / "conf"
        conf.mkdir()
        (conf / "a.env").write_text("DATABASE_URL = x\nPGPASSWORD=y\n", encoding="utf-8")
        (conf / "b.yaml").write_text("dsn: postgresql://u:hunter2@db/core\n", encoding="utf-8")
        single = Path(root) / "environment"
        single.write_text("SKILLHUB_SECRETS_TOKEN: tok\n", encoding="utf-8")
        environs = {
            "12": b"HOME=/root\0PGPASSWORD=hunter2\0EMPTY_OK=\0",
            "30": b"X_URL=postgres://u:hunter2@db/x\0DATABASE_URL=\0",
            "44": None,
        }
        status, detail = p05_row((str(conf), str(single)), environs)
    assert status == probe.FAIL
    hits = sorted({
        "pid 12 env PGPASSWORD", "pid 30 env X_URL holds a postgres:// URL",
        f"{conf / 'a.env'} sets DATABASE_URL", f"{conf / 'b.yaml'} contains a postgres:// URL",
        f"{single} sets SKILLHUB_SECRETS_TOKEN",
    })
    assert detail == (f"scanned every readable /proc/<pid>/environ; {conf}; {single} for the names {NAMES} "
                      f"and for postgres:// URLs (values deliberately not printed) -- 5 hit(s): "
                      + "; ".join(hits)), detail
    assert "hunter2" not in detail


def test_the_offline_self_check_passes_every_grading_case():
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = probe.self_check()
    assert code == 0
    assert "BAD" not in out.getvalue(), out.getvalue()
    assert out.getvalue().splitlines()[-1] == "self-check: all grading cases behave"


def test_grade_gvisor_reports_the_early_verdict_when_the_baseline_is_unusable():
    assert probe.grade_gvisor("runsc 1.2.3 abcdef", None)[0] == probe.UNKNOWN
    assert probe.grade_gvisor("runsc 1.2.3 abcdef", "unset")[0] == probe.FAIL
    assert probe.grade_gvisor("runsc 1.2.3 abcdef", "")[0] == probe.UNKNOWN


def test_grade_gvisor_passes_when_the_node_is_at_or_above_baseline():
    status, detail = probe.grade_gvisor(
        "runsc version release-20260201.0 (go1.22)", "release-20260101.0"
    )
    assert status == probe.PASS, detail


def test_grade_gvisor_fails_when_the_node_is_below_baseline():
    status, detail = probe.grade_gvisor(
        "runsc version release-20260101.0 (go1.22)", "release-20260201.0"
    )
    assert status == probe.FAIL, detail
    assert "BELOW BASELINE" in detail


def test_grade_node_age_reports_the_precondition_before_parsing_a_timestamp():
    now = datetime(2026, 8, 27, tzinfo=timezone.utc)
    assert probe.grade_node_age(None, now)[0] == probe.UNKNOWN
    assert probe.grade_node_age("2026-08-20T00:00:00Z", now, "provision")[0] == probe.UNKNOWN


def test_grade_node_age_passes_for_a_fresh_serving_node():
    now = datetime(2026, 8, 27, tzinfo=timezone.utc)
    created = (now - timedelta(days=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
    status, _ = probe.grade_node_age(created, now, probe.BUILD_PHASE_SERVING)
    assert status == probe.PASS


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
