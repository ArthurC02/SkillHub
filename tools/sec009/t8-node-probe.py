#!/usr/bin/env python3
"""ADR-022 part three, T8 -- the node half: gate A's node admission probe.

============================ WHAT THIS IS ==================================
T8 is split across two suites. The *image* half (I-01..I-04, I-06) is a query
against GHCR and lives in t8-image-audit.py. This is the other half: the five
items that can only be answered by standing on the node -- C-01, P-01, P-03,
P-04, P-05.

ADR-022 calls this "閘門 A 節點准入探針" and gives it one job: run at node boot,
emit `pass`/`fail` for every item it covers, and report. `unknown` counts as
fail, so every message below says what could NOT be seen rather than implying
nothing was wrong.

--------------------------- THE INPUT CONTRACT -----------------------------
This probe does not infer what the node is. It reads facts that IaC/cloud-init
wrote at build time, from a JSON file:

    /etc/skillhub/node.json      (override: SKILLHUB_NODE_FACTS=<path>)

    {
      "node_id":         "sbx-01",              # who this is, for the report
      "role":            "sandbox-exec",        # P-01: must be the dedicated
                                                #       execution-pool role
      "node_created_at": "2026-08-25T04:11:07Z", # P-03: ISO8601, written ONCE
                                                #       by cloud-init at build
      "iac_commit":      "c860c64",             # which IaC built it
      "build_phase":     "serving"              # P-03: `provision` written by
                                                #       cloud-init at build,
                                                #       rewritten `serving` by
                                                #       the boot script once
                                                #       the node is in service
    }

All five fields are required (05 R-17c, 2026-09-10); `role` must be the
literal `sandbox-exec`.

Two properties of `node_created_at` are the whole point of it, and the
deployment batch has to preserve both:

  1. cloud-init writes it, at build. ADR-022 §1 is explicit that P-03 measures
     "來自 cloud-init 寫入的建置時戳，非節點自報的當下時間" -- a node that
     writes its own timestamp on every boot reports an age of zero forever and
     the 7-day rebuild rule silently stops existing.
  2. It survives reboots. It is the node's *build* time, not its boot time.

`build_phase` is what proves property 1, and it replaced a 2-second clock
window that could only guess at it (05 R-17c). A node whose facts say
`serving` finished being built and is not stamping itself now; one that says
`provision`, says something else, or says nothing is `unknown` -- and the
last of those means the node's IaC predates this contract.

File missing => every fact-dependent item is `unknown` => fail. That is the
designed behaviour, not a gap: a node that cannot say what it is does not join
the pool.

============================ WHAT THIS IS NOT ==============================
Not the SEC-009 acceptance, and not even all of C-01. Three honest limits:

  - C-01's runtime half ("each Run gets its own scratch, Runs share no writable
    path") is a statement about two concurrent Runs. A declarative snapshot of
    an idle node cannot see it; SBX-005's integration tests can. Until
    2026-09-10 this probe reported it as `unknown`, which under ADR-022's
    fail-closed rule meant a perfectly configured node still exited 2. 05 R-17a
    ruled the split: gate A judges C-01's declarative face, SBX-005 judges the
    runtime one, and the row here is `ELSEWHERE` -- printed, named, and not
    counted as this gate's answer.
  - P-01's "not co-scheduled with Web/API/DB workloads" is graded from the
    declared role plus what is actually running. A node can lie in its facts
    file; only the IaC review catches that.
  - P-05 greps for the *shape* of a core-database credential. It proves the
    obvious ones are absent, never that none exists.

Usage: python tools/sec009/t8-node-probe.py [--json] [--self-check]
       SKILLHUB_NODE_FACTS=<path>        where cloud-init wrote the facts
       SKILLHUB_GVISOR_BASELINE=<path>   where IaC rendered the baseline
Exit:  0 every check passed
       1 setup failed -- nothing was measured, which is not a pass
       2 a check failed, or could not be made (`unknown` is fail)
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from datetime import datetime, timedelta, timezone

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))

NODE_FACTS = os.environ.get("SKILLHUB_NODE_FACTS", "/etc/skillhub/node.json")
BASELINE_FILE = os.environ.get(
    "SKILLHUB_GVISOR_BASELINE", os.path.join(REPO_ROOT, "infra", "nodes", "gvisor-baseline.txt")
)

# P-01: the only role allowed to run untrusted Skills. Anything else is a node
# that was pointed at the execution pool by accident.
EXPECTED_ROLE = "sandbox-exec"

# ADR-022 §1: 7-day rolling rebuild; >14 days is the on-call drain trigger.
REBUILD_DAYS = 7
DRAIN_DAYS = 14

# 05 R-17c (2026-09-10) replaced the 2-second window with a field. The window
# was a heuristic: it separated "cloud-init wrote this at build" from
# "something wrote datetime.now() as the probe started", and it could not
# separate either from a probe that genuinely ran 2s after cloud-init -- so on
# a slow node it misjudged, and nobody would have known. The comment that used
# to sit here said the answer was for cloud-init to record something rather
# than for this window to widen; that is now the input contract.
#
# `build_phase` is written `provision` by cloud-init at build and rewritten
# `serving` by the boot script once the node is in service. A node still in
# `provision` has not finished being built, and one with no phase at all is
# running an IaC older than this contract -- both are `unknown`, which is what
# "we could not tell" is supposed to look like.
BUILD_PHASE_PROVISION = "provision"
BUILD_PHASE_SERVING = "serving"

# apps/sandbox/internal/dockerdrv/docker.go: every Run container carries this.
SANDBOX_LABEL = "skillhub.sandbox.managed"
# ADR-022 §Q1: the node runs one compose file and one service.
PLATFORM_CONTAINER_RE = re.compile(r"sandboxd", re.I)

# P-05. Names first, then value shapes -- a variable called anything at all
# holding a `postgres://` URL is the same finding.
CRED_NAMES = ("SKILLHUB_DATABASE_URL", "DATABASE_URL", "PGPASSWORD", "SKILLHUB_SECRETS_TOKEN")
CRED_VALUE_RE = re.compile(rb"postgres(?:ql)?://[^\s\"']+")
CRED_PATHS = ("/etc/skillhub", "/etc/environment", "/etc/default", "/run/secrets", "/opt/skillhub")

PASS, FAIL, UNKNOWN = "PASS", "FAIL", "UNKNOWN"
# 05 R-17a: a baseline item this gate is not the one that judges. It is not a
# pass -- nothing here measured it -- and it must not be an `unknown`, because
# ADR-022 §3 reads `unknown` as fail and a gate that is red on a correct node
# gets switched off. The row still prints, and it names who does judge it, so
# omitting the check is not what "covered" means here.
ELSEWHERE = "ELSEWHERE"


class SetupError(Exception):
    """Something the probe depends on is missing, so nothing was measured."""


class Report:
    def __init__(self) -> None:
        self.rows: list[dict] = []

    def add(self, ident: str, name: str, status: str, detail: str) -> None:
        self.rows.append({"id": ident, "check": name, "status": status, "detail": detail})

    @property
    def ok(self) -> bool:
        return all(r["status"] in (PASS, ELSEWHERE) for r in self.rows)

    def render(self) -> None:
        width = max(len(r["id"]) for r in self.rows)
        for r in self.rows:
            print("  %-*s  %-7s %s -- %s" % (width, r["id"], r["status"], r["check"], r["detail"]))


# --------------------------------------------------------------------------
# helpers


def sh(*cmd: str) -> tuple[int, str]:
    """Run a command. Returns (-1, reason) when it is not installed at all.

    Not installed and exited non-zero are different facts here: the first is
    "we could not look" (unknown), the second is an answer.
    """
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
    except FileNotFoundError:
        return -1, "%s is not installed on this node" % cmd[0]
    except (subprocess.SubprocessError, OSError) as exc:
        return -1, "%s could not be run: %s" % (cmd[0], exc)
    return p.returncode, (p.stdout + p.stderr).strip()


def first_value(path: str) -> str | None:
    """First non-comment, non-blank line of a file; None if unreadable."""
    try:
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                line = line.strip()
                if line and not line.startswith("#"):
                    return line
    except OSError:
        return None
    return ""


def load_facts(path: str) -> tuple[dict | None, str]:
    try:
        with open(path, encoding="utf-8") as fh:
            return json.load(fh), path
    except FileNotFoundError:
        return None, "%s does not exist -- IaC/cloud-init wrote no node facts here" % path
    except (OSError, ValueError) as exc:
        return None, "%s is unreadable or not JSON: %s" % (path, exc)


# --------------------------------------------------------------------------
# grading, pulled out so --self-check can drive it with no node


GVISOR_RE = re.compile(r"(?:release-)?(\d{8})(?:\.(\d+))?")


def parse_gvisor(text: str | None) -> tuple[int, int] | None:
    """`runsc version release-20260817.0` -> (20260817, 0).

    gVisor versions its releases by date, so ordering is a plain tuple compare.
    Anything that does not carry an 8-digit date is not a version we can rank.
    """
    if not text:
        return None
    m = GVISOR_RE.search(text)
    if not m:
        return None
    return int(m.group(1)), int(m.group(2) or 0)


def grade_gvisor(node_version: str | None, baseline: str | None) -> tuple[str, str]:
    """P-04. Node's runsc vs the IaC-rendered baseline. Blocking, per ADR-022."""
    if baseline is None:
        return UNKNOWN, ("cannot read %s -- IaC rendered no baseline onto this node, so "
                         "there is nothing to compare `runsc --version` against" % BASELINE_FILE)
    if baseline == "unset":
        return FAIL, (
            "baseline file says `unset`: the baseline itself has no value, so there is "
            "nothing to compare against. ADR-022 §2 makes P-04 blocking -- an unset "
            "baseline admits no node, and that is fail-closed, not a probe defect")
    if not baseline:
        return UNKNOWN, "baseline file has no value line"

    want = parse_gvisor(baseline)
    if want is None:
        return UNKNOWN, "baseline %r carries no release date, so it cannot be ranked" % baseline
    if node_version is None:
        return UNKNOWN, "`runsc --version` produced nothing -- gVisor is not installed here"
    got = parse_gvisor(node_version)
    if got is None:
        return UNKNOWN, "`runsc --version` said %r, which carries no release date" % (
            node_version.splitlines()[0] if node_version else node_version)

    shown = "node runsc %s vs baseline %s (compared as (date, patch))" % (
        "%d.%d" % got, "%d.%d" % want)
    if got >= want:
        return PASS, shown
    return FAIL, shown + " -- BELOW BASELINE, node must not join the pool"


def grade_node_age(created_at: str | None, now: datetime,
                   build_phase: str | None = None) -> tuple[str, str]:
    """P-03. Age of the cloud-init build timestamp against the 7-day cycle."""
    if not created_at:
        return UNKNOWN, "node facts carry no `node_created_at`, so the node's age is unknown"
    if not build_phase:
        return UNKNOWN, (
            "node facts carry no `build_phase`, so nothing says this timestamp came from the "
            "build rather than from this boot. ADR-022 §1 measures the cloud-init BUILD "
            "timestamp specifically to exclude that -- a node that stamps itself on every "
            "boot reports age zero forever and the 7-day rule stops existing. The node's IaC "
            "predates 05 R-17c; re-render it")
    if build_phase == BUILD_PHASE_PROVISION:
        return UNKNOWN, (
            "`build_phase` is still %r, so this node has not finished being built and its "
            "age is not yet a fact about a serving node" % build_phase)
    if build_phase != BUILD_PHASE_SERVING:
        return UNKNOWN, (
            "`build_phase` is %r, which is neither %r nor %r -- the facts file does not "
            "follow the contract this gate reads" % (
                build_phase, BUILD_PHASE_PROVISION, BUILD_PHASE_SERVING))
    try:
        built = datetime.fromisoformat(str(created_at).replace("Z", "+00:00"))
    except (ValueError, TypeError):
        return UNKNOWN, "`node_created_at` is not a timestamp: %r" % created_at
    if built.tzinfo is None:
        built = built.replace(tzinfo=timezone.utc)

    age = now - built
    if age < timedelta(seconds=0):
        return UNKNOWN, "`node_created_at` (%s) is in the future -- not a build timestamp" % created_at

    detail = "built %s, %d day(s) old (limit %d)" % (created_at, age.days, REBUILD_DAYS)
    if age > timedelta(days=DRAIN_DAYS):
        return FAIL, detail + " -- OVER %d DAYS: on-call must drain this node by hand per " \
                              "SEC-010 runbook, automatic drain is not the rule here" % DRAIN_DAYS
    if age > timedelta(days=REBUILD_DAYS):
        return FAIL, detail + " -- past the rebuild cycle; ADR-022 grades P-03 as alert-level " \
                              "and queues a rebuild, but T8 reports pass/fail only"
    return PASS, detail


# --------------------------------------------------------------------------
# checks against the actual node


def check_p01(rep: Report, facts: dict | None, facts_why: str) -> None:
    if facts is None:
        rep.add("P-01a", "declared role is the execution pool", UNKNOWN, facts_why)
    else:
        role = facts.get("role")
        if role == EXPECTED_ROLE:
            rep.add("P-01a", "declared role is the execution pool", PASS,
                    "node facts declare role=%r (read from %s, written by IaC)" % (role, facts_why))
        elif role is None:
            rep.add("P-01a", "declared role is the execution pool", UNKNOWN,
                    "node facts carry no `role` key, so we cannot tell what this node is for")
        else:
            rep.add("P-01a", "declared role is the execution pool", FAIL,
                    "node facts declare role=%r, expected %r" % (role, EXPECTED_ROLE))

    rc, out = sh("docker", "ps", "--no-trunc", "--format", "{{.ID}}|{{.Names}}|{{.Image}}|{{.Labels}}")
    if rc != 0:
        why = out.splitlines()[0] if out else "docker produced no output"
        rep.add("P-01b", "only sandbox + platform containers run here", UNKNOWN,
                "cannot read the container list, so co-scheduled workloads would be "
                "invisible: %s" % why)
        return
    strangers, counted = [], 0
    for line in out.splitlines():
        parts = line.split("|", 3)
        if len(parts) < 4:
            continue
        counted += 1
        _, name, image, labels = parts
        if SANDBOX_LABEL in labels:
            continue
        if PLATFORM_CONTAINER_RE.search(name) or PLATFORM_CONTAINER_RE.search(image):
            continue
        strangers.append("%s (%s)" % (name, image))
    how = "read `docker ps`; a container counts as expected only if it carries the %s label " \
          "(dockerdrv puts it on every Run) or is sandboxd itself" % SANDBOX_LABEL
    if strangers:
        rep.add("P-01b", "only sandbox + platform containers run here", FAIL,
                "%s -- %d of %d containers are neither: %s" % (how, len(strangers), counted,
                                                               ", ".join(sorted(strangers))))
    else:
        rep.add("P-01b", "only sandbox + platform containers run here", PASS,
                "%s -- %d container(s), all expected" % (how, counted))


def check_p03(rep: Report, facts: dict | None, facts_why: str) -> None:
    if facts is None:
        rep.add("P-03", "rebuilt within the 7-day cycle", UNKNOWN, facts_why)
        return
    status, detail = grade_node_age(facts.get("node_created_at"), datetime.now(timezone.utc),
                                    facts.get("build_phase"))
    rep.add("P-03", "rebuilt within the 7-day cycle", status, detail)


def check_p04(rep: Report) -> None:
    rc, out = sh("runsc", "--version")
    rep.add("P-04", "gVisor at or above the baseline", *grade_gvisor(out if rc == 0 else None, first_value(BASELINE_FILE)))


def check_p05(rep: Report) -> None:
    """P-05. Long-lived core-DB / secrets credentials must not exist on the node.

    Never prints a matched value: rule 11 puts secrets out of logs and outputs,
    and a probe that quotes what it found is itself the leak. Location and the
    name of the pattern that hit are enough to go fix it.
    """
    hits: list[str] = []
    looked: list[str] = []

    if os.path.isdir("/proc"):
        looked.append("every readable /proc/<pid>/environ")
        for pid in os.listdir("/proc"):
            if not pid.isdigit():
                continue
            try:
                with open("/proc/%s/environ" % pid, "rb") as fh:
                    blob = fh.read()
            except OSError:
                continue  # kernel threads and other users' processes; expected
            for entry in blob.split(b"\0"):
                name, _, value = entry.partition(b"=")
                key = name.decode("utf-8", "replace")
                if key in CRED_NAMES and value:
                    hits.append("pid %s env %s" % (pid, key))
                elif value and CRED_VALUE_RE.search(value):
                    hits.append("pid %s env %s holds a postgres:// URL" % (pid, key))
    else:
        looked.append("no /proc, so running processes were not scanned")

    for root in CRED_PATHS:
        looked.append(root)
        if os.path.isfile(root):
            files = [root]
        elif os.path.isdir(root):
            files = [os.path.join(dp, f) for dp, _, fs in os.walk(root) for f in fs]
        else:
            continue
        for path in files:
            try:
                with open(path, "rb") as fh:
                    blob = fh.read(1 << 20)
            except OSError:
                continue
            if CRED_VALUE_RE.search(blob):
                hits.append("%s contains a postgres:// URL" % path)
                continue
            for name in CRED_NAMES:
                if re.search(rb"\b%s\s*[=:]\s*\S" % re.escape(name.encode()), blob):
                    hits.append("%s sets %s" % (path, name))
                    break

    how = "scanned " + "; ".join(looked) + " for the names %s and for postgres:// URLs " \
          "(values deliberately not printed)" % ", ".join(CRED_NAMES)
    if hits:
        rep.add("P-05", "no long-lived core-DB / secrets credential", FAIL,
                "%s -- %d hit(s): %s" % (how, len(hits), "; ".join(sorted(set(hits)))))
    else:
        rep.add("P-05", "no long-lived core-DB / secrets credential", PASS, how)


def check_c01(rep: Report) -> None:
    rc, out = sh("docker", "info", "--format", "{{range $k, $v := .Runtimes}}{{$k}} {{end}}")
    if rc != 0:
        rep.add("C-01a", "runsc registered as a docker runtime", UNKNOWN,
                "cannot read `docker info`, so we cannot see which runtimes dockerd offers")
    else:
        runtimes = out.split()
        if "runsc" in runtimes:
            rep.add("C-01a", "runsc registered as a docker runtime", PASS,
                    "`docker info` lists: %s" % " ".join(runtimes))
        else:
            rep.add("C-01a", "runsc registered as a docker runtime", FAIL,
                    "`docker info` lists %s -- no runsc, so sandboxd's Runtime:\"runsc\" would "
                    "fail to start rather than silently fall back, but this node cannot host Runs"
                    % (" ".join(runtimes) or "<none>"))

    rc, out = sh("docker", "ps", "-q", "--no-trunc")
    if rc != 0:
        rep.add("C-01b", "no host path shared writable into a container", UNKNOWN,
                "cannot list containers, so their mounts were not inspected")
    else:
        ids = out.split()
        if not ids:
            rep.add("C-01b", "no host path shared writable into a container", PASS,
                    "no containers are running, so no host path is shared by any")
        else:
            rc2, out2 = sh("docker", "inspect", "--format",
                           "{{.Name}}|{{json .Mounts}}", *ids)
            if rc2 != 0:
                rep.add("C-01b", "no host path shared writable into a container", UNKNOWN,
                        "`docker inspect` failed, so mounts were not read")
            else:
                bad = []
                for line in out2.splitlines():
                    name, _, raw = line.partition("|")
                    try:
                        mounts = json.loads(raw) or []
                    except ValueError:
                        continue
                    for m in mounts:
                        if m.get("Type") == "bind" and m.get("RW"):
                            bad.append("%s:%s(rw)" % (name.lstrip("/"), m.get("Source")))
                how = "read `docker inspect .Mounts` on %d container(s); a Type=bind mount with " \
                      "RW=true is a writable host path (dockerdrv keeps Binds and Mounts empty " \
                      "on purpose -- C-05/C-07)" % len(ids)
                if bad:
                    rep.add("C-01b", "no host path shared writable into a container", FAIL,
                            "%s -- %s" % (how, ", ".join(sorted(bad))))
                else:
                    rep.add("C-01b", "no host path shared writable into a container", PASS, how)

    rep.add("C-01c", "each Run gets its own scratch, Runs share none", ELSEWHERE,
            "NOT MEASURABLE FROM A SNAPSHOT OF ONE NODE, AND NOT THIS GATE'S TO JUDGE. "
            "C-01's runtime half is a statement about two concurrent Runs; this probe "
            "photographs an idle node. 05 R-17a (2026-09-10) split C-01 accordingly -- gate A "
            "judges the declarative face (C-01a, C-01b) and SBX-005's integration tests judge "
            "this one, where two Runs actually exist. Until that ruling this row was `unknown`, "
            "which ADR-022 §3 reads as fail: a correctly built node exited 2, and a gate that "
            "is red on a correct node gets switched off in week one. The row still prints and "
            "still names its judge, so nothing here is omitted -- see ADR-022 §2 row 1")


# --------------------------------------------------------------------------


def self_check() -> int:
    """Offline proof that the two graders can be red. No node, no docker.

    Same reason the image audit carries one: both rules here are the kind that
    only get exercised on the day they matter. P-04's comparison is invisible
    until a node shows up below baseline; P-03's arithmetic is invisible until
    a node is eight days old. Every run before then looks identical whether or
    not the comparison is the right way round.
    """
    bad = 0

    def want(label: str, got: tuple[str, str], expect: str, must_say: str = "") -> None:
        nonlocal bad
        status, detail = got
        wrong = status != expect or (must_say and must_say not in detail)
        bad += 1 if wrong else 0
        print("  %s %-46s want %-7s got %-7s" % ("BAD " if wrong else "ok  ", label, expect, status))

    print("P-04 -- version comparison:")
    want("node newer than baseline", grade_gvisor("runsc version release-20260817.0", "release-20260701.0"), PASS)
    want("node equal to baseline", grade_gvisor("runsc version release-20260817.0", "release-20260817.0"), PASS)
    want("node older than baseline", grade_gvisor("runsc version release-20260601.0", "release-20260817.0"), FAIL)
    want("same date, higher patch", grade_gvisor("runsc version release-20260817.2", "release-20260817.1"), PASS)
    want("same date, lower patch", grade_gvisor("runsc version release-20260817.0", "release-20260817.1"), FAIL)
    want("baseline is `unset`", grade_gvisor("runsc version release-20260817.0", "unset"), FAIL,
         "nothing to compare against")
    want("baseline file unreadable", grade_gvisor("runsc version release-20260817.0", None), UNKNOWN)
    want("runsc not installed", grade_gvisor(None, "release-20260817.0"), UNKNOWN)
    want("runsc printed something else", grade_gvisor("command not found", "release-20260817.0"), UNKNOWN)

    print("Report.ok -- which statuses let a gate pass:")
    for label, status, expect in [
        ("a pass alone passes", PASS, True),
        ("an unknown still fails (ADR-022 §3)", UNKNOWN, False),
        ("a fail still fails", FAIL, False),
        # 05 R-17a. Before the ruling this was UNKNOWN and a correct node
        ("judged elsewhere does not fail this gate", ELSEWHERE, True),
    ]:
        rep = Report()
        rep.add("X", "case", status, "self-check")
        wrong = rep.ok is not expect
        bad += 1 if wrong else 0
        print("  %s %-46s want %-7s got %-7s" % (
            "BAD " if wrong else "ok  ", label, "ok" if expect else "not ok",
            "ok" if rep.ok else "not ok"))

    print("P-03 -- node age:")
    now = datetime(2026, 8, 27, 12, 0, 0, tzinfo=timezone.utc)
    SERVING = BUILD_PHASE_SERVING

    def at(days: float) -> str:
        return (now - timedelta(days=days)).strftime("%Y-%m-%dT%H:%M:%SZ")

    # Literal 6.9 / 7.1 / 15 on purpose, not REBUILD_DAYS +/- epsilon: ADR-022 §1
    # is where 7 and 14 come from, so the self-check has to pin those numbers
    # rather than re-derive them from the constant it is supposed to be guarding.
    want("one day old", grade_node_age(at(1), now, SERVING), PASS)
    want("6.9 days -- inside the cycle", grade_node_age(at(6.9), now, SERVING), PASS)
    want("7.1 days -- past the cycle", grade_node_age(at(7.1), now, SERVING), FAIL)
    want("15 days names the drain", grade_node_age(at(15), now, SERVING), FAIL, "drain")
    want("no timestamp", grade_node_age(None, now), UNKNOWN)
    want("unparseable timestamp", grade_node_age("last tuesday", now, SERVING), UNKNOWN)
    want("timestamp in the future", grade_node_age(at(-1), now, SERVING), UNKNOWN)
    # 05 R-17c: the phase, not a clock window. A node that stamps itself on
    # every boot now fails to say `serving` from a build, and a probe that
    # happens to run seconds after cloud-init is no longer misread as one.
    # Same trap as the case below: every phase branch says "build_phase", so the
    # assertion has to name the sentence only this branch writes.
    want("no build_phase at all", grade_node_age(at(1), now, None), UNKNOWN,
         "predates 05 R-17c")
    # The phrase, not the word: "provision" also appears in the off-contract
    # branch's message, so asserting on it let a mutation that deleted this
    # branch entirely stay green. Caught on 2026-09-10 by making it red.
    want("still provisioning", grade_node_age(at(1), now, "provision"), UNKNOWN,
         "has not finished being built")
    want("phase off contract", grade_node_age(at(1), now, "ready"), UNKNOWN, "contract")
    want("serving, fresh off the build", grade_node_age(at(0), now, SERVING), PASS)

    print("self-check: %s" % ("all grading cases behave" if not bad else "%d case(s) wrong" % bad))
    return 0 if not bad else 2


def probe() -> Report:
    if not sys.platform.startswith("linux"):
        raise SetupError("this probe reads /proc and a local dockerd; on %s it measures nothing"
                         % sys.platform)
    facts, facts_why = load_facts(NODE_FACTS)
    rep = Report()
    check_c01(rep)
    check_p01(rep, facts, facts_why)
    check_p03(rep, facts, facts_why)
    check_p04(rep)
    check_p05(rep)
    return rep


def main() -> int:
    if "--self-check" in sys.argv:
        return self_check()
    try:
        rep = probe()
    except SetupError as exc:
        print("t8 node probe: setup failed, nothing was measured -- %s" % exc, file=sys.stderr)
        return 1

    facts, facts_why = load_facts(NODE_FACTS)
    header = {
        "probe": "t8-node-probe",
        "adr": "ADR-022 part three, T8 node half (gate A)",
        "ran_at": datetime.now(timezone.utc).isoformat(),
        "node_facts_path": NODE_FACTS,
        "node_facts": facts if facts is not None else None,
        "node_facts_error": None if facts is not None else facts_why,
        "gvisor_baseline_path": BASELINE_FILE,
        "checks": rep.rows,
    }
    if "--json" in sys.argv:
        print(json.dumps(header, indent=2))
    else:
        print("node:     %s" % ((facts or {}).get("node_id") or "<no node facts>"))
        print("iac:      %s" % ((facts or {}).get("iac_commit") or "<unknown>"))
        print("facts:    %s" % (NODE_FACTS if facts is not None else facts_why))
        print()
        print("T8 node half (Suite 2 -- gate A admission probe):")
        rep.render()
        print()

    if rep.ok:
        print("t8 node probe: PASS -- NOT the SEC-009 acceptance, five of 45 items only")
        return 0
    print("t8 node probe: at least one item failed or could not be measured; "
          "`unknown` counts as fail (ADR-022 T8), so this node is not admitted", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
