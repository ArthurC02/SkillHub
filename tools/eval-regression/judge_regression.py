"""Replay the M2 baseline Runs through the Judge and score it.

    python judge_regression.py            # the whole set, one gateway call per run
    python judge_regression.py --limit 3  # a cheap smoke pass
    python judge_regression.py --dry-run  # read facts and build requests; call nothing
    python judge_regression.py --run-id <UUID> --run-id <UUID> --dry-run
    python judge_regression.py --rubric rubric-content-007-writing-v1.json

Not product code and does not run in CI; a non-dry run spends real money.
Reads runs, trace events, test case snapshots and artifacts already in the
platform and writes nothing back to any of them.

Scores the STORED verdict, not the model's raw answer: every evidence
reference is re-resolved, a criterion whose reference does not resolve is
downgraded to `undetermined`, and the truncation rule is applied — mirroring
what a user would actually have seen.

--rubric adds a Skill-scoped rubric's extra acceptance criteria to the run
alongside the snapshot's own; a different rubric, judge model, or prompt
version is a different regression. Output is one JSON object per Run
appended (never overwritten) to results.jsonl, stamped with judge_model /
judge_prompt_version / rubric_version / the truncation budget so different
regressions stay readable side by side."""

from __future__ import annotations

import argparse
import datetime
import hashlib
import hmac
import io
import json
import os
import subprocess
import tarfile
import unicodedata
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

PG_CONTAINER = "skillhub-postgres-1"
PG_USER = "skillhub"
PG_DB = "skillhub"
JUDGE_URL = "http://127.0.0.1:8000/judge-run"

S3_HOST = "127.0.0.1:8333"
S3_BUCKET = "skillhub"
S3_ACCESS_KEY = "skillhubdev"
S3_SECRET_KEY = "skillhubdevsecret"

OUT = Path(__file__).with_name("results.jsonl")

MAX_FINAL_OUTPUT = 40000  # one-number: maxFinalOutput
MAX_CRITERIA = 20  # one-number: maxCriteria
MAX_DIGEST_ENTRY = 8000  # one-number: maxDigestEntry - raised 2026-08-23, evaluation-design 6.3 / 04 丙-47
MAX_DIGEST_COUNT = 100  # one-number: maxDigestCount
MAX_ARTIFACT_ROWS = 500  # one-number: maxArtifactRows
EXCERPT_LIMIT = 1000  # one-number: excerptLimit

CITABLE = (
    "skill_activation",
    "resource_read",
    "tool_call",
    "script_log",
    "agent_output",
    "error",
    "usage",
)

CRITERION_KINDS = {
    "Run 的 trace 中出現對指定 Skill 的 skill_activation": "activation",
    "/out/artifacts/ 中至少產出一個檔案": "artifact",
    "最終回覆說明了產出的檔案": "final_reply",
}

COST_ALARM_USD = 0.10



def psql(sql: str):
    """One query, JSON back. No writes: every statement here is a SELECT."""
    out = subprocess.run(
        ["docker", "exec", PG_CONTAINER, "psql", "-U", PG_USER, "-d", PG_DB, "-tAc",
         f"select coalesce(json_agg(t), '[]') from ({sql}) t"],
        capture_output=True, text=True, encoding="utf-8", check=True,
    )
    return json.loads(out.stdout)


def s3_get(key: str) -> bytes:
    """SigV4 GET. The bucket refuses anonymous reads and that is deliberate."""
    path = f"/{S3_BUCKET}/{urllib.parse.quote(key)}"
    now = datetime.datetime.now(datetime.UTC)
    amzdate, datestamp = now.strftime("%Y%m%dT%H%M%SZ"), now.strftime("%Y%m%d")
    empty = hashlib.sha256(b"").hexdigest()
    signed = "host;x-amz-content-sha256;x-amz-date"
    canonical = "\n".join([
        "GET", path, "",
        f"host:{S3_HOST}\nx-amz-content-sha256:{empty}\nx-amz-date:{amzdate}\n",
        signed, empty,
    ])
    scope = f"{datestamp}/us-east-1/s3/aws4_request"
    to_sign = "\n".join(
        ["AWS4-HMAC-SHA256", amzdate, scope, hashlib.sha256(canonical.encode()).hexdigest()]
    )
    k = hmac.new(f"AWS4{S3_SECRET_KEY}".encode(), datestamp.encode(), hashlib.sha256).digest()
    for part in ("us-east-1", "s3", "aws4_request"):
        k = hmac.new(k, part.encode(), hashlib.sha256).digest()
    sig = hmac.new(k, to_sign.encode(), hashlib.sha256).hexdigest()
    req = urllib.request.Request(
        f"http://{S3_HOST}{path}",
        headers={
            "Authorization": f"AWS4-HMAC-SHA256 Credential={S3_ACCESS_KEY}/{scope}, "
                             f"SignedHeaders={signed}, Signature={sig}",
            "x-amz-date": amzdate,
            "x-amz-content-sha256": empty,
        },
    )
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read()


def regression_set():
    """The 45 baseline Runs: the latest compatibility measurement per Skill Version.

    Not a hand-copied list of ids. `skill_runtime_compatibility` records which
    Run each measurement came from, so "the baseline" is a query rather than a
    transcription - which is what makes the set reproducible and its membership
    auditable (02:EVAL-013 clause 1). 41 land on 2026.08-3 and 4 stay on
    2026.08-2 because their Skills are licence-restricted and could not be re-run.
    """
    return psql("""
        with latest as (
          select distinct on (skill_version_id)
                 skill_version_id, runtime_image, source_run_id
            from skill_runtime_compatibility
           order by skill_version_id, measured_at desc
        )
        select s.name              as skill_name,
               s.name              as rubric_skill_name,
               coalesce(s.summary, '') as skill_summary,
               l.runtime_image,
               r.id::text          as run_id,
               r.status::text      as run_status,
               coalesce(r.failure_class, '') as failure_class,
               snap.user_prompt,
               snap.acceptance_criteria
          from latest l
          join runs r  on r.id = l.source_run_id
          join skill_versions sv on sv.id = l.skill_version_id
          join skills s on s.id = sv.skill_id
          join test_case_snapshots snap on snap.id = r.test_case_snapshot_id
         order by s.name
    """)


def explicit_run_set(run_ids: list[uuid.UUID]):
    """Rows selected by caller-owned Run ids, independent of compatibility.
    Preserves a fork's actual name for the request while using its parent
    name only to select the parent-owned content rubric."""
    requested = ", ".join(f"'{run_id}'::uuid" for run_id in run_ids)
    rows = psql(f"""
        select s.name              as skill_name,
               coalesce(parent.name, s.name) as rubric_skill_name,
               coalesce(s.summary, '') as skill_summary,
               null::text          as runtime_image,
               r.id::text          as run_id,
               r.status::text      as run_status,
               coalesce(r.failure_class, '') as failure_class,
               snap.user_prompt,
               snap.acceptance_criteria
          from runs r
          join skill_versions sv on sv.id = r.skill_version_id
          join skills s on s.id = sv.skill_id
     left join skills parent on parent.id = s.forked_from_skill_id
          join test_case_snapshots snap on snap.id = r.test_case_snapshot_id
         where r.id in ({requested})
         order by array_position(array[{requested}], r.id)
    """)
    found = {row["run_id"] for row in rows}
    missing = [str(run_id) for run_id in run_ids if str(run_id) not in found]
    if missing:
        raise SystemExit(
            "--run-id must name an existing Run with a Skill Version and Test Case Snapshot; "
            f"not selectable: {', '.join(missing)}"
        )
    if len(rows) != len(run_ids):
        raise SystemExit("--run-id selection returned duplicate or incomplete Run rows")
    return rows


def trace_events(run_id: str):
    return psql(f"""
        select event_id::text, occurred_at, event_type, source, attempt, seq, payload
          from trace_events where run_id = '{run_id}' order by occurred_at, seq
    """)


def artifact_manifest(run_id: str):
    """Filenames and sizes read out of the run's archive index — a manifest,
    not content: nothing is unpacked and no bytes reach the model."""
    keys = psql(f"""
        select id::text from run_attempts
         where run_id = '{run_id}' order by attempt_number desc limit 1
    """)
    if not keys:
        return []
    key = f"run-artifacts/{run_id}/{keys[0]['id']}/artifacts.tar"
    try:
        blob = s3_get(key)
    except Exception as e:  # noqa: BLE001 - a missing archive is data, not a crash
        print(f"    ! no archive at {key}: {e}")
        return []
    with tarfile.open(fileobj=io.BytesIO(blob)) as tar:
        return [
            {"path": m.name, "size_bytes": m.size, "content_type": ""}
            for m in tar.getmembers()
            if m.isfile()
        ]



def clip(text: str, limit: int):
    return (text, False) if len(text) <= limit else (text[:limit], True)


def load_rubrics(path: Path) -> dict:
    """Read a --rubric file: {rubric_version, skills: {name: {criteria, rubric}}}.
    Rejects a rubric item whose id was never sent alongside it as a
    criterion, since such an item would otherwise measure nothing."""
    doc = json.loads(path.read_text(encoding="utf-8"))
    version, skills = doc["rubric_version"], doc["skills"]
    for name, entry in skills.items():
        ids = {c["id"] for c in entry["criteria"]}
        orphans = [i["id"] for i in entry["rubric"]["items"] if i["id"] not in ids]
        if orphans:
            raise SystemExit(f"{path}: {name} has rubric items with no criterion: {orphans}")
    return {"version": version, "skills": skills}


def build_request(row, events, artifacts, evaluation_id: str, rubric_entry=None):
    """The JudgeRunRequest, plus the digest map that verification checks against."""
    truncation = []

    final = ""
    for e in events:
        if e["event_type"] == "agent_output" and (e["payload"] or {}).get("kind") == "final":
            final = (e["payload"] or {}).get("text") or ""
    final, cut_output = clip(final, MAX_FINAL_OUTPUT)
    if cut_output:
        truncation.append("final_output")

    wanted = list(row["acceptance_criteria"])
    if rubric_entry:
        wanted += rubric_entry["criteria"]
    criteria = [
        {"id": c["id"], "text": c["text"], "evidence_excerpt": None}
        for c in wanted[:MAX_CRITERIA]
    ]
    if len(wanted) > MAX_CRITERIA:
        truncation.append("criteria")

    if len(artifacts) > MAX_ARTIFACT_ROWS:
        artifacts, truncation = artifacts[:MAX_ARTIFACT_ROWS], [*truncation, "artifacts"]

    citable = [e for e in events if e["event_type"] in CITABLE]
    if len(citable) > MAX_DIGEST_COUNT:
        # Keeps the tail, not the head, so the final output, errors and usage
        # roll-up (all near the end) survive the cut.
        citable = citable[-MAX_DIGEST_COUNT:]
        truncation.append("trace_digest.entries")

    entries, digest = [], {}
    trimmed = False
    for e in citable:
        excerpt, cut_excerpt = clip(json.dumps(e["payload"], ensure_ascii=False), MAX_DIGEST_ENTRY)
        trimmed = trimmed or cut_excerpt
        entries.append({
            "trace_event_id": e["event_id"],
            "occurred_at": e["occurred_at"],
            "type": e["event_type"],
            "excerpt": excerpt,
        })
        digest[e["event_id"]] = e
    if trimmed:
        truncation.append("trace_digest.entries[].excerpt")

    request = {
        "run_id": row["run_id"],
        "evaluation_id": evaluation_id,
        "skill": {"name": row["skill_name"], "summary": row["skill_summary"]},
        "user_prompt": row["user_prompt"],
        "criteria": criteria,
        "final_output": final,
        "artifacts": artifacts,
        "trace_digest": {"complete": trace_complete(events), "entries": entries},
        "truncation": truncation,
    }
    if rubric_entry:
        request["rubric"] = {"items": rubric_entry["rubric"]["items"]}
    return request, digest, final


def trace_complete(events) -> bool:
    """False if any producer stream has a gap in its seq numbers (TRACE-008)."""
    streams: dict[tuple, list[int]] = {}
    for e in events:
        streams.setdefault((e["source"], e["attempt"]), []).append(e["seq"])
    return all(
        sorted(seqs) == list(range(min(seqs), min(seqs) + len(seqs)))
        for seqs in streams.values() if seqs
    )



def trace_search_text(payload) -> str:
    """What a trace citation is checked against: the payload as stored, plus
    every string leaf decoded, joined with NUL so no quote can match by
    spanning two fields."""
    raw = json.dumps(payload, ensure_ascii=False)
    leaves: list[str] = []

    def walk(node) -> None:
        if isinstance(node, str):
            leaves.append(node)
        elif isinstance(node, list):
            for item in node:
                walk(item)
        elif isinstance(node, dict):
            for item in node.values():
                walk(item)

    walk(payload)
    return raw + "\0" + "\0".join(leaves) if leaves else raw


STRUCTURAL_PUNCTUATION = "}]),;\"'`「」『』 "

MIN_NORMALIZED_QUOTE = 12

MATCH_EXACT = "exact"
MATCH_NORMALIZED = "normalized"
MATCH_NOT_CHECKED = "not_checked"


def normalize_quote(s: str) -> str:
    out: list[str] = []
    pending = False
    for ch in unicodedata.normalize("NFC", s):
        if ch.isspace():
            pending = True
            continue
        if pending and out:
            out.append(" ")
        pending = False
        out.append(ch)
    return "".join(out).strip(STRUCTURAL_PUNCTUATION)


def verified_quote(match) -> bool:
    """Whether this citation rests on a quote the platform actually found."""
    return match in (MATCH_EXACT, MATCH_NORMALIZED)


def locate(text: str, quote: str):
    """(match state, index) for one text: the exact attempt before the normalized one."""
    i = text.find(quote)
    if i >= 0:
        return MATCH_EXACT, i
    nq = normalize_quote(quote)
    if len(nq) < MIN_NORMALIZED_QUOTE:
        return None, -1
    if nq in normalize_quote(text):
        return MATCH_NORMALIZED, -1
    return None, -1


def verifiable_sources(digest, final_output):
    """(kind, event_id, searchable text) for every place a quote may honestly
    be: the final output first, then the trace entries in digest (trace)
    order."""
    out = []
    if final_output:
        out.append(("agent_output", None, final_output))
    for event_id, event in digest.items():
        out.append(("trace_event", event_id, trace_search_text(event["payload"])))
    return out


def find_quote(quote: str, sources):
    """Where is this quote, whatever it was filed as: exact across every
    source before normalized across any, since an exact hit anywhere beats
    a normalized one."""
    for kind, event_id, text in sources:
        if text.find(quote) >= 0:
            return kind, event_id, MATCH_EXACT
    nq = normalize_quote(quote)
    if len(nq) < MIN_NORMALIZED_QUOTE:
        return None
    for kind, event_id, text in sources:
        if nq in normalize_quote(text):
            return kind, event_id, MATCH_NORMALIZED
    return None


def verify(ref, digest, artifacts, final_output):
    """(stored reference, why it did not resolve): a citation holds only if
    its quote is findable in a verifiable source of this run. `ref["kind"]`
    is tried first as a hint, then every other source before giving up."""
    kind = ref.get("kind")
    named_failure = ""

    if kind == "trace_event":
        event_id = ref.get("trace_event_id") or ""
        event = digest.get(event_id)
        if event is None:
            if not ref.get("quote"):
                return None, f"cited trace event {event_id!r} was not in the digest"
            named_failure = f"cited trace event {event_id!r} was not in the digest"
        elif not ref.get("quote"):
            return {**ref, "match": MATCH_EXACT, "reattributed_from": None}, ""
        else:
            match, _ = locate(trace_search_text(event["payload"]), ref["quote"])
            if match:
                return {**ref, "match": match, "reattributed_from": None}, ""
            named_failure = f"the quote cited from trace event {event_id!r} is not in it"

    elif kind == "agent_output":
        if not ref.get("quote"):
            return None, "an agent output reference was cited with no quote to locate"
        match, _ = locate(final_output, ref["quote"])
        if match:
            return {**ref, "match": match, "reattributed_from": None}, ""
        named_failure = "the quote cited from the agent's final output is not in it"

    elif kind == "artifact":
        named_failure = "an artifact citation's quote is verified against nothing"

    else:
        return None, f"reference kind {kind!r} is not one this platform can resolve"

    if ref.get("quote"):
        hit = find_quote(ref["quote"], verifiable_sources(digest, final_output))
        if hit:
            src_kind, event_id, match = hit
            out = {**ref, "kind": src_kind, "match": match, "reattributed_from": kind}
            if src_kind == "trace_event":
                out["trace_event_id"] = event_id
            else:
                out.pop("trace_event_id", None)
            return out, ""

    if kind == "artifact":
        path = ref.get("artifact_path") or ""
        if any(a["path"] == path for a in artifacts):
            return {**ref, "match": MATCH_NOT_CHECKED, "reattributed_from": None}, ""
        return None, f"cited artifact {path!r} is not in this run's manifest"
    return None, named_failure + ", and it is in no other verifiable source of this run"


def store(verdict, request, digest, artifacts, final_output):
    """The stored criterion results: what the user would have been shown,
    after downgrading unresolved evidence, quoteless artifact citations, and
    passes reached on cut or gapped material, each to its own reason."""
    answers = {c["criterion_id"]: c for c in verdict["criterion_results"]}
    incomplete = not request["trace_digest"]["complete"] or bool(request["truncation"])
    evidence_required = {
        item["id"]: bool(item.get("evidence_required"))
        for item in (request.get("rubric") or {}).get("items", [])
    }

    results = []
    for c in request["criteria"]:
        answer = answers.get(c["id"])
        if answer is None:
            results.append({
                "criterion_id": c["id"], "text": c["text"], "result": "undetermined",
                "model_result": None, "downgrade": "unanswered", "reason": "no verdict returned",
                "evidence": [],
            })
            continue

        result = answer["result"] if answer["result"] in {"passed", "failed", "undetermined"} \
            else "undetermined"
        evidence = []
        unverifiable = []
        for ref in answer["evidence_refs"]:
            stored, why = verify(ref, digest, artifacts, final_output)
            if why:
                unverifiable.append(why)
                evidence.append({**ref, "match": None, "reattributed_from": None,
                                 "rejected": why})
            else:
                evidence.append({**stored, "rejected": None})
        downgrade = None
        if unverifiable and result != "undetermined":
            result, downgrade = "undetermined", "evidence_unverifiable"
        elif (evidence_required.get(c["id"]) and result != "undetermined"
              and not any(verified_quote(e["match"]) for e in evidence)):
            result, downgrade = "undetermined", "evidence_unverifiable"
        elif result == "passed" and incomplete:
            result, downgrade = "undetermined", "incomplete_evidence"

        results.append({
            "criterion_id": c["id"], "text": c["text"], "result": result,
            "model_result": answer["result"], "downgrade": downgrade,
            "reason": answer["reason"][:600],
            "evidence": evidence,
        })
    return results



def expected(row, events, artifacts) -> dict:
    """Per-criterion expected answers, derived from the platform's own facts:
    activation from the trace, artifact presence from the archive. The third
    criterion has no derivable ground truth and comes back as None,
    reported unscored."""
    activated = {
        (e["payload"] or {}).get("skill_name")
        for e in events
        if e["event_type"] == "skill_activation"
        and (e["payload"] or {}).get("decision") == "activated"
    }
    return {
        "activation": "passed" if row["rubric_skill_name"] in activated else "failed",
        "artifact": "passed" if artifacts else "failed",
        "final_reply": None,
    }



def judge(request: dict, url: str) -> dict:
    """POST one judgement request, bearing the same LLM_SERVICE_TOKEN apps/llm
    checks, read from the environment and never defaulted."""
    body = json.dumps(request, ensure_ascii=False).encode()
    headers = {"Content-Type": "application/json"}
    token = os.getenv("LLM_SERVICE_TOKEN", "")
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=body, headers=headers)
    with urllib.request.urlopen(req, timeout=300) as r:
        return json.loads(r.read())


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--limit", type=int, help="only the first N Runs")
    ap.add_argument(
        "--dry-run", action="store_true",
        help="live-read PostgreSQL/S3 and build requests; no Judge/model calls, no writes",
    )
    ap.add_argument("--note", default="", help="free text stored with every row")
    ap.add_argument("--judge-url", default=JUDGE_URL, help=f"default {JUDGE_URL}")
    ap.add_argument("--rubric", type=Path,
                    help="CONTENT-007 rubric file; narrows the set to the Skills it covers")
    ap.add_argument(
        "--run-id", action="append", type=uuid.UUID, metavar="UUID",
        help="select this Run directly (repeatable); bypasses compatibility baseline selection",
    )
    args = ap.parse_args()

    rubrics = load_rubrics(args.rubric) if args.rubric else None
    started = datetime.datetime.now(datetime.UTC).isoformat()
    regression_id = f"{started[:19].replace(':', '')}Z"
    selection = "explicit_run_ids" if args.run_id else "m2_latest_compatibility"
    rows = explicit_run_set(args.run_id) if args.run_id else regression_set()
    if rubrics:
        if args.run_id:
            not_covered = [r["run_id"] for r in rows if r["rubric_skill_name"] not in rubrics["skills"]]
            if not_covered:
                raise SystemExit(
                    "--rubric does not cover explicitly selected Run ids: "
                    + ", ".join(not_covered)
                )
        else:
            rows = [r for r in rows if r["rubric_skill_name"] in rubrics["skills"]]
        missing = set(rubrics["skills"]) - {r["rubric_skill_name"] for r in rows}
        if missing:
            print(f"! no baseline run for: {', '.join(sorted(missing))}")
    if args.limit:
        rows = rows[: args.limit]
    version = rubrics["version"] if rubrics else None
    if not rows:
        raise SystemExit("selection produced zero Runs")
    print(f"regression {regression_id}: {len(rows)} runs, selection={selection}, rubric_version={version}")

    total_cost, unreported, lines = 0.0, 0, []
    for i, row in enumerate(rows, 1):
        events = trace_events(row["run_id"])
        artifacts = artifact_manifest(row["run_id"])
        evaluation_id = str(uuid.uuid4())
        entry = rubrics["skills"][row["rubric_skill_name"]] if rubrics else None
        request, digest, final_output = build_request(
            row, events, artifacts, evaluation_id, entry)
        want = expected(row, events, artifacts)

        print(f"[{i}/{len(rows)}] {row['skill_name']}: "
              f"{len(request['trace_digest']['entries'])} events, {len(artifacts)} files, "
              f"expect activation={want['activation']} artifact={want['artifact']}")
        if args.dry_run:
            continue

        response = judge(request, args.judge_url)
        results = store(response["verdict"], request, digest, artifacts, final_output)
        usage = response.get("usage") or {}
        cost = usage.get("cost_usd")
        if cost is None:
            unreported += 1
        else:
            total_cost += cost
            if cost > COST_ALARM_USD:
                print(f"    !! ${cost:.4f} for one call, over the ${COST_ALARM_USD} alarm - stopping")
                lines.append(record(regression_id, started, args.note, selection, row, request, want,
                                    results, response, usage, version))
                break

        line = record(regression_id, started, args.note, selection, row, request, want, results, response,
                      usage, version)
        lines.append(line)
        for r in line["criteria"]:
            mark = {"match": "ok", "mismatch": "MISMATCH", "unscored": "--",
                    "downgraded": "undet"}[r["outcome"]]
            print(f"    {r['kind']:<12} want={r['expected'] or '-':<12} got={r['result']:<12} {mark}")
        print(f"    ${cost if cost is not None else float('nan'):.4f}  "
              f"running total ${total_cost:.4f}")

    if lines:
        # newline="\n" keeps this file LF-only; text mode would append CRLF
        # rows on Windows.
        with OUT.open("a", encoding="utf-8", newline="\n") as f:
            for line in lines:
                f.write(json.dumps(line, ensure_ascii=False) + "\n")
        print(f"\nappended {len(lines)} rows to {OUT}")
    summarise(lines, total_cost, unreported)


def record(regression_id, started, note, run_selection, row, request, want, results, response, usage,
           rubric_version=None):
    by_id = {r["criterion_id"]: r for r in results}
    rubric_ids = {i["id"] for i in (request.get("rubric") or {}).get("items", [])}
    criteria = []
    for c in request["criteria"]:
        kind = "rubric" if c["id"] in rubric_ids else CRITERION_KINDS.get(c["text"], "unknown")
        got = by_id[c["id"]]
        exp = want.get(kind)
        if exp is None:
            outcome = "unscored"
        elif got["result"] == exp:
            outcome = "match"
        elif got["downgrade"]:
            outcome = "downgraded"
        else:
            outcome = "mismatch"
        criteria.append({**got, "kind": kind, "expected": exp, "outcome": outcome})

    return {
        "regression_id": regression_id,
        "started_at": started,
        "note": note,
        "judge_model": response["model"],
        "judge_prompt_version": response["prompt_version"],
        "rubric_version": rubric_version,
        "truncation_budget": {
            "final_output": MAX_FINAL_OUTPUT, "criteria": MAX_CRITERIA,
            "digest_entry": MAX_DIGEST_ENTRY, "digest_count": MAX_DIGEST_COUNT,
            "artifact_rows": MAX_ARTIFACT_ROWS,
        },
        "skill_name": row["skill_name"],
        "run_selection": run_selection,
        "runtime_image": row["runtime_image"],
        "run_id": row["run_id"],
        "run_status": row["run_status"],
        "evaluation_id": request["evaluation_id"],
        "trace_complete": request["trace_digest"]["complete"],
        "truncation": request["truncation"],
        "artifact_count": len(request["artifacts"]),
        "overall_model": response["verdict"]["overall"],
        "summary": response["verdict"]["summary"],
        "usage": usage,
        "criteria": criteria,
    }


def summarise(lines, total_cost, unreported) -> None:
    rubric = [c for line in lines for c in line["criteria"] if c["kind"] == "rubric"]
    if rubric:
        by_result: dict[str, int] = {}
        for c in rubric:
            by_result[c["result"]] = by_result.get(c["result"], 0) + 1
        downgraded = sum(1 for c in rubric if c["downgrade"])
        print(f"\nrubric criteria {len(rubric)} (unscored by design)")
        for result, n in sorted(by_result.items()):
            print(f"  {result:<14} {n} = {n / len(rubric):.1%}")
        print(f"  of which downgraded by the platform: {downgraded}")

    scored = [c for line in lines for c in line["criteria"] if c["outcome"] != "unscored"]
    if not scored:
        return
    match = sum(c["outcome"] == "match" for c in scored)
    mismatch = [c for c in scored if c["outcome"] == "mismatch"]
    downgraded = [c for c in scored if c["outcome"] == "downgraded"]
    print(f"\nscored {len(scored)} criteria over {len(lines)} runs")
    print(f"  agreement   {match}/{len(scored)} = {match / len(scored):.1%}")
    print(f"  mismatch    {len(mismatch)}")
    print(f"  downgraded  {len(downgraded)} (undetermined, counted apart from wrong)")
    print(f"  cost        ${total_cost:.4f}" + (f", {unreported} calls unreported" if unreported else ""))
    for c in mismatch:
        print(f"    ! {c['kind']}: want {c['expected']}, got {c['result']} - {c['reason'][:120]}")


if __name__ == "__main__":
    main()
