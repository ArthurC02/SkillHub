"""PDM-003 supplemental tests (2026-08-14): the two items previously blocked on
"needs Anthropic credentials". The provider is now decided as OpenAI behind the
LiteLLM gateway (ADR-017 unchanged), so both can be answered on the real backend.

A. Autonomous Skill invocation + behavioural verification of the `skills`
   whitelist. Report SS10.5 left both open: gpt-4o-mini invoked the Skill 0/1
   times unprompted, and `skills=[]` was never verified behaviourally because
   init.skills only reflects discovery.

B. Prompt-caching correction for the PDM-005 300K input ceiling. Report SS6.2
   measured ~50K input per turn with no cache and estimated 5-6 turns.

Run:  EXTERNAL_PROXY=1 LITELLM_MASTER_KEY=... .venv/Scripts/python.exe test_supplemental.py
Secrets are read from .env only and are never printed, logged, or written to
results-supplemental.txt.
"""

import asyncio
import json
import os
import shutil
import sys
import tempfile
import time
import urllib.request
import uuid
from pathlib import Path

HERE = Path(__file__).parent
REPO = HERE.parent.parent
BASE = "http://127.0.0.1:4000"

FLAGSHIP = "flagship-test"   # openai/gpt-5.6-sol
MINI = "mini-test"           # openai/gpt-5.4-mini

REPORT_SKILL = "spike-report"
DECOY_SKILL = "spike-decoy"
BUILTIN_SKILL = "code-review"   # one of the 15 CLI built-ins
MARKER = "SKILLHUB-MARKER-7Q4Z"

TRIALS = 3

SKILL_MD = """---
name: {name}
description: {desc}
---

# {title}

{body}
"""

results = []


def record(kind, name, detail=""):
    results.append((kind, name, detail))
    print(f"[{kind}] {name} {detail}", flush=True)


def load_env_file():
    """Read .env into os.environ. Values are never echoed."""
    for line in (REPO / ".env").read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        os.environ.setdefault(key.strip(), value.strip())


def gateway_env():
    os.environ.pop("ANTHROPIC_API_KEY", None)
    os.environ["ANTHROPIC_BASE_URL"] = BASE
    os.environ["ANTHROPIC_AUTH_TOKEN"] = os.environ["LITELLM_MASTER_KEY"]
    os.environ["MAX_THINKING_TOKENS"] = "0"  # see report SS6.1


def make_workdir():
    wd = Path(tempfile.mkdtemp(prefix="pdm003-supp-"))
    skills = {
        REPORT_SKILL: dict(
            desc="Formats a quarterly widget inventory report for the Acme warehouse. "
                 "Use this whenever the user asks to format, produce, or summarise a "
                 "widget inventory report.",
            title="Widget inventory report",
            body=f"When formatting a widget inventory report, you MUST begin your reply "
                 f"with the literal token {MARKER} on its own line, then list the widgets.",
        ),
        DECOY_SKILL: dict(
            desc="Converts imperial cooking measurements to metric. Use this when the "
                 "user asks to convert cups, ounces or Fahrenheit to metric units.",
            title="Imperial to metric",
            body="Convert cups to millilitres, ounces to grams, Fahrenheit to Celsius.",
        ),
    }
    for name, kw in skills.items():
        d = wd / ".claude" / "skills" / name
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(SKILL_MD.format(name=name, **kw), encoding="utf-8")
    return wd


# --------------------------------------------------------------------------- #
# Agent turn helper: captures tool calls, tool-result errors, and per-call usage
# --------------------------------------------------------------------------- #

def agent_turn(wd, prompt, model, **opts):
    from claude_agent_sdk import ClaudeAgentOptions, query

    async def run():
        calls, errors, texts, usages = [], [], [], []
        options = ClaudeAgentOptions(
            model=model, cwd=str(wd), setting_sources=["project"],
            allowed_tools=["Skill", "Read"], permission_mode="bypassPermissions",
            max_turns=6, **opts,
        )
        async for message in query(prompt=prompt, options=options):
            u = getattr(message, "usage", None)
            if u and type(message).__name__ == "AssistantMessage":
                usages.append(u)
            for block in getattr(message, "content", []) or []:
                kind = type(block).__name__
                if kind == "ToolUseBlock":
                    inp = getattr(block, "input", {}) or {}
                    calls.append((getattr(block, "name", "?"),
                                  inp.get("skill") or inp.get("command") or ""))
                elif kind == "ToolResultBlock":
                    content = getattr(block, "content", None)
                    text = content if isinstance(content, str) else json.dumps(content)
                    if getattr(block, "is_error", False):
                        errors.append((text or "")[:300])
                elif kind == "TextBlock":
                    texts.append(getattr(block, "text", ""))
        return calls, errors, " ".join(texts), usages

    return asyncio.run(asyncio.wait_for(run(), timeout=600))


def skill_calls(calls):
    return [arg for name, arg in calls if name == "Skill"]


REPORT_TASK = ("format a quarterly widget inventory report for the Acme warehouse "
               "with 12 blue widgets and 7 red widgets")


# --------------------------------------------------------------------------- #
# A1. Autonomous invocation rate, per model
# --------------------------------------------------------------------------- #

def a1_autonomous(wd, model):
    hits, detail = 0, []
    for i in range(TRIALS):
        try:
            calls, _errors, joined, _u = agent_turn(
                wd, f"Please {REPORT_TASK}.", model, skills="all")
        except Exception as exc:  # noqa: BLE001 - a stalled turn is still a miss
            detail.append(f"#{i+1}:miss({type(exc).__name__}: {exc})")
            continue
        got = REPORT_SKILL in skill_calls(calls)
        hits += got
        detail.append(f"#{i+1}:{'HIT' if got else 'miss'}"
                      f"(tools={[c[0] for c in calls]},marker={MARKER in joined})")
    record("DATA", f"A1 autonomous invocation {model}",
           f"{hits}/{TRIALS} " + " ".join(detail))
    return hits


# --------------------------------------------------------------------------- #
# A2. `skills` whitelist -- behavioural filtering, not just discovery
# --------------------------------------------------------------------------- #

def a1c_explicit_control(wd, model):
    """Control for A1: with the SAME workdir and models, does explicit invocation
    still work? Separates 'the model did not choose to' from 'it could not'."""
    calls, errors, joined, _u = agent_turn(
        wd, f"Invoke the {REPORT_SKILL} skill with the Skill tool, follow its "
            f"instructions exactly, and then {REPORT_TASK}.", model, skills="all")
    ok = REPORT_SKILL in skill_calls(calls) and MARKER in joined
    record("PASS" if ok else "FAIL", f"A1c explicit invocation control {model}",
           f"tools={[c[0] for c in calls]} skill_args={skill_calls(calls)} "
           f"marker={MARKER in joined} errors={len(errors)}")
    return ok


def a2_whitelist_excludes_project_skill(wd, model):
    """Whitelist contains only the decoy; the prompt names the excluded skill."""
    calls, errors, joined, _u = agent_turn(
        wd, f"Invoke the {REPORT_SKILL} skill with the Skill tool, follow its "
            f"instructions exactly, and then {REPORT_TASK}.",
        model, skills=[DECOY_SKILL])
    invoked = REPORT_SKILL in skill_calls(calls)
    blocked = (not invoked) or bool(errors)
    record("PASS" if blocked and MARKER not in joined else "FAIL",
           "A2 skills whitelist blocks an excluded project skill",
           f"attempted={invoked} tool_errors={len(errors)} marker_leaked={MARKER in joined} "
           f"err0={errors[0][:160] if errors else '-'}")
    return blocked


def a3_whitelist_excludes_builtin(wd, model):
    """Same, for one of the 15 built-in CLI skills -- decides whether `skills`
    can suppress built-in Skills as a Run interference source."""
    calls, errors, joined, _u = agent_turn(
        wd, f"Invoke the {BUILTIN_SKILL} skill with the Skill tool right now. "
            f"Report exactly what the tool returned.",
        model, skills=[REPORT_SKILL])
    invoked = BUILTIN_SKILL in skill_calls(calls)
    blocked = (not invoked) or bool(errors)
    record("PASS" if blocked else "FAIL",
           f"A3 skills whitelist blocks built-in skill {BUILTIN_SKILL!r}",
           f"attempted={invoked} tool_errors={len(errors)} "
           f"err0={errors[0][:200] if errors else '-'} reply={joined[:160]!r}")
    return blocked


def a0_builtin_inventory(wd):
    """init.skills baseline: how many built-ins ship with the CLI, and does the
    whitelist change the discovery array at all (report SS10.5 note a)."""
    from claude_agent_sdk import ClaudeAgentOptions, query

    async def init_skills(**opts):
        agen = query(prompt="hi", options=ClaudeAgentOptions(
            model=MINI, cwd=str(wd), permission_mode="bypassPermissions",
            max_turns=1, setting_sources=["project"], **opts))
        try:
            async for m in agen:
                if getattr(m, "subtype", None) == "init":
                    return (m.data or {}).get("skills") or []
        finally:
            await agen.aclose()
        return []

    all_s = asyncio.run(asyncio.wait_for(init_skills(skills="all"), timeout=180))
    filtered = asyncio.run(asyncio.wait_for(init_skills(skills=[REPORT_SKILL]), timeout=180))
    builtins = [s for s in all_s if s not in (REPORT_SKILL, DECOY_SKILL)]
    record("DATA", "A0 init.skills inventory",
           f"total={len(all_s)} builtins={len(builtins)} {builtins} | "
           f"skills=[{REPORT_SKILL!r}] -> init.skills unchanged={sorted(all_s) == sorted(filtered)}")


# --------------------------------------------------------------------------- #
# B. Prompt caching
# --------------------------------------------------------------------------- #

def _post(path, payload, extra_headers=None):
    req = urllib.request.Request(
        BASE + path, data=json.dumps(payload).encode(),
        headers={"content-type": "application/json",
                 "authorization": "Bearer " + os.environ["LITELLM_MASTER_KEY"],
                 **(extra_headers or {})})
    with urllib.request.urlopen(req, timeout=300) as r:
        return json.load(r)


def long_prefix(nonce):
    """~27K tokens. The nonce makes each route's first call a guaranteed cache
    MISS: OpenAI's cache now persists 24h, so a fixed prefix would still be warm
    from an earlier run and the cold/warm contrast would be lost."""
    return (f"You are warehouse inventory assistant {nonce}. Reference table follows.\n"
            + "\n".join(f"SKU-{i:05d} widget colour={'blue' if i % 2 else 'red'} "
                        f"bin={i % 37} qty={i % 91} reorder_point={i % 13} "
                        f"supplier=Acme-{i % 7} lead_time_days={i % 21}"
                        for i in range(900)))


def b1_openai_route_cache(nonce):
    """Control: does the OpenAI-native route surface cached_tokens through LiteLLM?"""
    prefix = long_prefix(nonce + "-oai")
    body = lambda: {"model": MINI, "max_tokens": 16, "messages": [
        {"role": "system", "content": prefix},
        {"role": "user", "content": "Reply with the single word OK."}]}
    first = _post("/v1/chat/completions", body())
    second = _post("/v1/chat/completions", body())
    def cached(r):
        return ((r.get("usage") or {}).get("prompt_tokens_details") or {}).get("cached_tokens")
    record("DATA", "B1 /v1/chat/completions cached_tokens (control)",
           f"cold prompt={first['usage']['prompt_tokens']} cached={cached(first)} | "
           f"warm prompt={second['usage']['prompt_tokens']} cached={cached(second)}")
    return cached(second)


def b2_messages_route_cache(nonce):
    """The route the Agent SDK actually uses. Does it carry cache_read_input_tokens?"""
    prefix = long_prefix(nonce + "-msg")
    body = lambda: {"model": MINI, "max_tokens": 16, "system": prefix,
                    "messages": [{"role": "user", "content": "Reply with the single word OK."}]}
    hdr = {"anthropic-version": "2023-06-01"}
    first = _post("/v1/messages", body(), hdr)
    second = _post("/v1/messages", body(), hdr)
    record("DATA", "B2 /v1/messages usage fields (Agent SDK route)",
           f"cold usage={first.get('usage')} | warm usage={second.get('usage')}")
    keys = set(second.get("usage") or {})
    passthrough = bool(keys & {"cache_read_input_tokens", "cache_creation_input_tokens"})
    # GAP, not FAIL: a measured LiteLLM limitation we are documenting, not a
    # broken test. Only genuine errors should set the exit code.
    record("PASS" if passthrough else "GAP",
           "B2 /v1/messages exposes cache_read_input_tokens",
           f"usage keys={sorted(keys)}")
    return passthrough


def b2b_spend_accounting():
    """Does the dropped cache field also corrupt LiteLLM's spend record? That is
    what per-Run Virtual Key budgets (PDM-003) are enforced against."""
    def fetch():
        req = urllib.request.Request(
            BASE + "/spend/logs",
            headers={"authorization": "Bearer " + os.environ["LITELLM_MASTER_KEY"]})
        with urllib.request.urlopen(req, timeout=60) as r:
            raw = json.load(r)
        raw = raw if isinstance(raw, list) else raw.get("data", [])
        # only the four B1/B2 probe calls are this large
        return [row for row in raw if (row.get("prompt_tokens") or 0) >= 20_000]

    rows = []
    for _ in range(15):  # spend rows are flushed asynchronously (report SS6.3)
        rows = fetch()
        if len(rows) >= 4:
            break
        time.sleep(2)

    by_route = {}
    for row in rows:
        by_route.setdefault(row.get("call_type"), []).append(
            (row.get("spend"), row.get("cache_read_input_tokens")))
    detail = " | ".join(
        f"{route}: spends={[round(s, 6) for s, _ in v]} cache_read={[c for _, c in v]}"
        for route, v in sorted(by_route.items()))
    record("DATA", "B2b LiteLLM spend per route for an identical cached prompt", detail)

    msg = by_route.get("anthropic_messages", [])
    discounted = len({round(s, 5) for s, _ in msg}) > 1 if len(msg) > 1 else None
    record("PASS" if discounted else "FAIL",
           "B2b /v1/messages spend reflects the cache discount",
           f"anthropic_messages spends={[round(s, 6) for s, _ in msg]} "
           f"(identical prompts; the second is a ~99% cache hit upstream)")
    return discounted


def b3_multiturn_curve(wd, model, turns=7):
    """Real Agent SDK conversation: per-turn input tokens over N user turns.

    Note: AssistantMessage.usage is all-zero on this path; ResultMessage.usage
    carries the per-turn figure and ResultMessage.model_usage the session total.
    """
    from claude_agent_sdk import ClaudeAgentOptions, ClaudeSDKClient

    prompts = [
        "How many blue widgets does the Acme warehouse hold this quarter? Answer in one sentence, no tools.",
        "And how many red ones? One sentence, no tools.",
        "What is the total? One sentence, no tools.",
        "Which colour is the majority? One sentence, no tools.",
        "State the ratio blue:red. One sentence, no tools.",
        "Summarise the answers above in one sentence, no tools.",
        "Restate that summary in fewer words, no tools.",
        "Now list the four figures as a bullet list, no tools.",
    ][:turns]

    async def run():
        rows = []
        options = ClaudeAgentOptions(
            model=model, cwd=str(wd), setting_sources=["project"], skills="all",
            permission_mode="bypassPermissions", max_turns=2)
        async with ClaudeSDKClient(options=options) as client:
            for n, p in enumerate(prompts, 1):
                await client.query(p)
                async for message in client.receive_response():
                    if type(message).__name__ == "ResultMessage":
                        rows.append((n, message.usage or {}, message.model_usage or {}))
        return rows

    rows = asyncio.run(asyncio.wait_for(run(), timeout=1800))

    total_in = total_cached = 0
    lines = []
    session_total = None
    for n, u, mu in rows:
        i = u.get("input_tokens", 0) + u.get("cache_read_input_tokens", 0) \
            + u.get("cache_creation_input_tokens", 0)
        c = u.get("cache_read_input_tokens", 0)
        total_in += i
        total_cached += c
        lines.append(f"t{n}:in={i},cached={c},out={u.get('output_tokens', 0)}")
        for m in mu.values():
            session_total = m.get("inputTokens")
    record("DATA", f"B3 per-turn input tokens ({model}, {len(rows)} turns)",
           " | ".join(lines))

    avg = total_in / max(len(rows), 1)
    record("DATA", "B3 totals",
           f"cumulative_input_{len(rows)}turns={total_in} cumulative_cached={total_cached} "
           f"avg_per_turn={avg:.0f} sdk_session_total={session_total} "
           f"turns_until_300K={300_000 / avg:.1f}")
    return total_in, total_cached, avg


# --------------------------------------------------------------------------- #

def _spend_rows():
    req = urllib.request.Request(
        BASE + "/spend/logs",
        headers={"authorization": "Bearer " + os.environ["LITELLM_MASTER_KEY"]})
    with urllib.request.urlopen(req, timeout=60) as r:
        raw = json.load(r)
    return raw if isinstance(raw, list) else raw.get("data", [])


def _settle(seen=None, stable_for=3, interval=3, limit=30):
    """Spend rows flush asynchronously (report SS6.3), so a row-count threshold
    races: late rows from the previous measurement land in the next window.
    Poll until the set of rows stops growing, then return it."""
    prev, stable = -1, 0
    rows = []
    for _ in range(limit):
        time.sleep(interval)
        rows = [r for r in _spend_rows() if seen is None or r.get("request_id") not in seen]
        stable = stable + 1 if len(rows) == prev else 0
        prev = len(rows)
        if stable >= stable_for:
            break
    return rows


def _billed_for(fn):
    """Run fn(), then return the gateway's own billing rows for the calls it made.

    Ground truth beats the SDK here: ResultMessage.usage was observed to report
    the last API call on some turns and a sum on others, whereas the gateway log
    is exactly what a per-Run Virtual Key budget is charged against."""
    _settle()  # let any in-flight rows land before snapshotting
    before = {r.get("request_id") for r in _spend_rows()}
    fn()
    return sorted((r.get("prompt_tokens") or 0) for r in _settle(before))


def b4_tool_round_trip_cost(wd, model):
    """B3 measures tool-FREE turns. A turn that calls a tool re-sends the whole
    harness prefix once per tool result, so it bills a multiple of that. This is
    what report SS6.2's 50,139-token figure measured, and it decides how many
    turns a 300K input ceiling really buys."""
    from claude_agent_sdk import ClaudeAgentOptions, ClaudeSDKClient

    def turn(prompt):
        async def run():
            options = ClaudeAgentOptions(
                model=model, cwd=str(wd), setting_sources=["project"], skills="all",
                allowed_tools=["Skill", "Read"], permission_mode="bypassPermissions",
                max_turns=6)
            async with ClaudeSDKClient(options=options) as client:
                await client.query(prompt)
                async for _ in client.receive_response():
                    pass
        return lambda: asyncio.run(asyncio.wait_for(run(), timeout=600))

    plain = _billed_for(turn("Reply with the single word OK. Do not use any tool."))
    tooled = _billed_for(turn(
        f"Invoke the {REPORT_SKILL} skill with the Skill tool, follow its "
        f"instructions exactly, and then {REPORT_TASK}."))

    a, b = sum(plain), sum(tooled)
    record("DATA", f"B4 gateway-billed input tokens per turn ({model})",
           f"no_tool_turn: calls={len(plain)} per_call={plain} total={a} | "
           f"skill_tool_turn: calls={len(tooled)} per_call={tooled} total={b} | "
           f"ratio={b / a if a else 0:.2f}x | "
           f"turns_until_300K: no_tool={300_000 / a if a else 0:.1f} "
           f"with_1_tool_call={300_000 / b if b else 0:.1f}")
    return a, b


def b5_multiturn_billing(wd, model, turns=5):
    """The decisive number for PDM-005: across a real multi-turn Agent SDK
    conversation, does the harness prefix actually hit OpenAI's cache, and does
    the discount reach the LiteLLM spend record the Virtual Key budget uses?"""
    from claude_agent_sdk import ClaudeAgentOptions, ClaudeSDKClient

    prompts = [
        "How many blue widgets does the Acme warehouse hold? One sentence, no tools.",
        "And how many red ones? One sentence, no tools.",
        "What is the total? One sentence, no tools.",
        "Which colour is the majority? One sentence, no tools.",
        "State the ratio blue:red. One sentence, no tools.",
    ][:turns]

    def run():
        async def go():
            options = ClaudeAgentOptions(
                model=model, cwd=str(wd), setting_sources=["project"], skills="all",
                permission_mode="bypassPermissions", max_turns=2)
            async with ClaudeSDKClient(options=options) as client:
                for p in prompts:
                    await client.query(p)
                    async for _ in client.receive_response():
                        pass
        asyncio.run(asyncio.wait_for(go(), timeout=1800))

    _settle()
    before = {r.get("request_id") for r in _spend_rows()}
    run()
    rows = sorted(_settle(before), key=lambda r: r.get("startTime") or "")
    big = [r for r in rows if (r.get("prompt_tokens") or 0) > 5_000]

    detail = " | ".join(f"in={r.get('prompt_tokens')},${r.get('spend'):.6f}" for r in big)
    tok = sum(r.get("prompt_tokens") or 0 for r in rows)
    usd = sum(r.get("spend") or 0 for r in rows)
    nominal = sum((r.get("prompt_tokens") or 0) for r in rows) * 0.75e-6
    record("DATA", f"B5 multi-turn gateway billing ({model}, {turns} turns, "
                   f"{len(rows)} calls)", detail)
    record("DATA", "B5 totals",
           f"input_tokens={tok} billed=${usd:.4f} "
           f"uncached_input_would_be=${nominal:.4f} "
           f"effective_discount={1 - usd / nominal if nominal else 0:.0%} "
           f"turns_per_300K_tokens={300_000 / (tok / turns):.1f}")
    return tok, usd


def main():
    load_env_file()
    if not os.environ.get("LITELLM_MASTER_KEY"):
        print("LITELLM_MASTER_KEY not set; start the proxy per README.md", file=sys.stderr)
        return 2
    gateway_env()

    wd = make_workdir()
    nonce = uuid.uuid4().hex
    print(f"workdir={wd}", flush=True)
    try:
        for fn, args in (
            (a0_builtin_inventory, (wd,)),
            (a1_autonomous, (wd, FLAGSHIP)),
            (a1_autonomous, (wd, MINI)),
            (a1c_explicit_control, (wd, FLAGSHIP)),
            (a1c_explicit_control, (wd, MINI)),
            (a2_whitelist_excludes_project_skill, (wd, FLAGSHIP)),
            (a3_whitelist_excludes_builtin, (wd, FLAGSHIP)),
            (b1_openai_route_cache, (nonce,)),
            (b2_messages_route_cache, (nonce,)),
            (b2b_spend_accounting, ()),
            (b3_multiturn_curve, (wd, MINI)),
            (b4_tool_round_trip_cost, (wd, MINI)),
            (b5_multiturn_billing, (wd, MINI)),
        ):
            try:
                fn(*args)
            except Exception as exc:  # noqa: BLE001 - spike records every failure
                record("FAIL", fn.__name__, f"{type(exc).__name__}: {exc}")
    finally:
        shutil.rmtree(wd, ignore_errors=True)

    lines = [
        "PDM-003 supplemental (OpenAI backend) - results",
        "A = autonomous Skill invocation + `skills` whitelist behavioural filtering",
        "B = prompt caching / PDM-005 300K input ceiling",
        f"models: {FLAGSHIP}=openai/gpt-5.6-sol  {MINI}=openai/gpt-5.4-mini",
        f"claude-agent-sdk={_ver('claude-agent-sdk')} python={sys.version.split()[0]}",
        "",
    ]
    lines += [f"[{s}] {n}" + (f"\n       {d}" if d else "") for s, n, d in results]
    (HERE / "results-supplemental.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")
    return 1 if any(s == "FAIL" for s, *_ in results) else 0


def _ver(pkg):
    from importlib.metadata import version
    try:
        return version(pkg)
    except Exception:  # noqa: BLE001
        return "n/a"


if __name__ == "__main__":
    sys.exit(main())
