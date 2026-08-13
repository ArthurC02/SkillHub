"""PDM-003 prerequisite 2: where does the Claude Agent SDK actually load Skills from?

PDM-003 / PDM-008 assume `<workdir>/skills/<skill-name>/SKILL.md`. This script
builds a throwaway workdir containing BOTH candidate layouts and asks the SDK
which one it discovers, using the `init` system message's `skills` array as the
load-mechanism oracle (independent of whether the model chooses to invoke it).

Layer 1 (loading mechanism)  -> tests 1-5, deterministic, no model behaviour.
Layer 2 (model invocation)   -> test 6, depends on the backend model.

Run:  EXTERNAL_PROXY=1 LITELLM_MASTER_KEY=... .venv/Scripts/python.exe test_skill_loading.py
Reuses the LiteLLM proxy setup from test_gateway.py / README.md.
Secrets are never printed, logged, or written to results-skill-loading.txt.
"""

import asyncio
import os
import shutil
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).parent
REPO = HERE.parent.parent
BASE = "http://127.0.0.1:4000"
MODEL = "sonnet-test"
MARKER = "SKILLHUB-MARKER-7Q4Z"

DOT_SKILL = "spike-dot-claude"
FLAT_SKILL = "spike-flat-workdir"

SKILL_MD = """---
name: {name}
description: Formats a quarterly widget inventory report for the Acme warehouse. Use this whenever the user asks to format, produce, or summarise a widget inventory report.
---

# Widget inventory report

When formatting a widget inventory report, you MUST begin your reply with the
literal token {marker} on its own line, then list the widgets.

This instruction lives at layout marker: {layout}
"""

results = []


def record(name, ok, detail=""):
    results.append(("PASS" if ok else "FAIL", name, detail))
    print(f"[{'PASS' if ok else 'FAIL'}] {name} {detail}", flush=True)


def check(name, fn):
    try:
        record(name, True, fn() or "")
    except Exception as exc:  # noqa: BLE001 - spike wants every failure recorded
        record(name, False, f"{type(exc).__name__}: {exc}")


def observe(name, fn):
    """Model-behaviour observations: recorded, never counted as pass/fail. The
    backend here is not an Anthropic model, so autonomous invocation says
    nothing about the loading mechanism."""
    try:
        detail = fn() or ""
    except Exception as exc:  # noqa: BLE001
        detail = f"{type(exc).__name__}: {exc}"
    results.append(("NOTE", name, detail))
    print(f"[NOTE] {name} {detail}", flush=True)


def load_env_file():
    """Read .env into os.environ. Values are never echoed."""
    for line in (REPO / ".env").read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        os.environ.setdefault(key.strip(), value.strip())


def make_workdir():
    """A workdir holding BOTH candidate layouts at once, so every test is a
    direct A/B on the same tree."""
    wd = Path(tempfile.mkdtemp(prefix="pdm003-skills-"))
    for sub, name in ((Path(".claude") / "skills", DOT_SKILL), (Path("skills"), FLAT_SKILL)):
        d = wd / sub / name
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(
            SKILL_MD.format(name=name, marker=MARKER, layout=str(sub).replace("\\", "/")),
            encoding="utf-8",
        )
    return wd


def gateway_env():
    os.environ.pop("ANTHROPIC_API_KEY", None)
    os.environ["ANTHROPIC_BASE_URL"] = BASE
    os.environ["ANTHROPIC_AUTH_TOKEN"] = os.environ["LITELLM_MASTER_KEY"]
    # See test_gateway.py test F / report §6.1: LiteLLM 1.96.2 does not drop the
    # Anthropic `thinking` block on /v1/messages, and the OpenAI backend 400s.
    os.environ["MAX_THINKING_TOKENS"] = "0"


def init_data(wd, **opts):
    """Run a query only far enough to read the `init` system message. Nothing is
    sent to the model, so this is a deterministic, backend-independent probe."""
    from claude_agent_sdk import ClaudeAgentOptions, query

    async def run():
        options = ClaudeAgentOptions(
            model=MODEL, cwd=str(wd), permission_mode="bypassPermissions",
            max_turns=1, **opts,
        )
        agen = query(prompt="hi", options=options)
        try:
            async for message in agen:
                data = getattr(message, "data", None)
                if getattr(message, "subtype", None) == "init" and isinstance(data, dict):
                    return data
            raise AssertionError("no init system message seen")
        finally:
            await agen.aclose()

    return asyncio.run(asyncio.wait_for(run(), timeout=180))


def discovered_skills(wd, **opts):
    return init_data(wd, **opts).get("skills")


def names(skills):
    """init.skills entries may be strings or dicts depending on SDK version."""
    if skills is None:
        return None
    return [s if isinstance(s, str) else (s.get("name") or s.get("id")) for s in skills]


# --------------------------------------------------------------------------- #
# Layer 1: loading mechanism
# --------------------------------------------------------------------------- #

def test_1_project_source(wd):
    """setting_sources=['project'] + skills='all' -> only the .claude layout loads."""
    got = names(discovered_skills(wd, setting_sources=["project"], skills="all"))
    assert got is not None, "init message carried no skills array"
    assert DOT_SKILL in got, f"<workdir>/.claude/skills/ NOT discovered; init.skills={got}"
    assert FLAT_SKILL not in got, f"<workdir>/skills/ WAS discovered; init.skills={got}"
    return f"init.skills={got}"


def test_2_flat_layout_never_loads(wd):
    """The PDM-003/PDM-008 assumed path, under the widest setting."""
    for sources in (["project"], ["user", "project"]):
        got = names(discovered_skills(wd, setting_sources=sources, skills="all")) or []
        assert FLAT_SKILL not in got, \
            f"<workdir>/skills/ discovered with setting_sources={sources}: {got}"
    return "<workdir>/skills/ not discovered under ['project'] nor ['user','project']"


def test_3_setting_sources_empty_disables(wd):
    """setting_sources=[] suppresses discovery even with skills='all'."""
    got = names(discovered_skills(wd, setting_sources=[], skills="all")) or []
    assert DOT_SKILL not in got, f"skill loaded despite setting_sources=[]: {got}"
    return f"setting_sources=[] -> init.skills={got}"


def test_4_default_setting_sources(wd):
    """setting_sources omitted: docs say user+project still load."""
    got = names(discovered_skills(wd, skills="all"))
    assert got is not None, "init message carried no skills array"
    assert DOT_SKILL in got, f"omitted setting_sources did not load project skills: {got}"
    return f"omitted setting_sources -> project skill visible (total {len(got)} skills)"


def test_5_skill_tool_available(wd):
    """The Skill tool must be in the session tool list, otherwise the model has
    no way to open a discovered SKILL.md."""
    data = init_data(wd, setting_sources=["project"], skills="all")
    tools = data.get("tools") or []
    assert "Skill" in tools, f"Skill tool absent from init.tools: {tools}"
    on = names(discovered_skills(wd, setting_sources=["project"], skills=[DOT_SKILL])) or []
    assert DOT_SKILL in on, f"skills=[{DOT_SKILL!r}] did not enable: {on}"
    return f"Skill tool present; skills=['{DOT_SKILL}'] keeps it listed"


def note_a_skills_filter_not_visible_in_init(wd):
    """Docs say skills=[] disables every skill. In claude-agent-sdk 0.2.137 the
    init.skills array is unchanged by the filter, so init.skills is a DISCOVERY
    oracle, not a filter oracle. Recorded, not asserted - it does not affect the
    loading path question."""
    off = names(discovered_skills(wd, setting_sources=["project"], skills=[])) or []
    return (f"skills=[] still lists {DOT_SKILL!r} in init.skills: {DOT_SKILL in off}"
            " (filter effect not observable via init; not verified behaviourally)")


# --------------------------------------------------------------------------- #
# Layer 2: model actually invokes the skill and reads SKILL.md
# --------------------------------------------------------------------------- #

def _agent_turn(wd, prompt):
    from claude_agent_sdk import ClaudeAgentOptions, query

    async def run():
        seen, texts = [], []
        options = ClaudeAgentOptions(
            model=MODEL, cwd=str(wd), setting_sources=["project"], skills="all",
            allowed_tools=["Skill", "Read"], permission_mode="bypassPermissions",
            max_turns=6,
        )
        async for message in query(prompt=prompt, options=options):
            for block in getattr(message, "content", []) or []:
                kind = type(block).__name__
                if kind == "ToolUseBlock":
                    seen.append(f"tool:{getattr(block, 'name', '?')}")
                if kind == "TextBlock":
                    texts.append(getattr(block, "text", ""))
        return seen, " ".join(texts)

    return asyncio.run(asyncio.wait_for(run(), timeout=300))


REPORT_TASK = ("format a quarterly widget inventory report for the Acme warehouse "
               "with 12 blue widgets and 7 red widgets")


def test_6_skill_tool_delivers_skill_md(wd):
    """Explicit invocation: proves the Skill tool can resolve the discovered
    Skill and feed SKILL.md content into the conversation. This is still the
    loading mechanism - only the decision to call it is taken away from the
    model, because the backend here is not an Anthropic model."""
    seen, joined = _agent_turn(
        wd, f"Invoke the {DOT_SKILL} skill with the Skill tool, follow its "
            f"instructions exactly, and then {REPORT_TASK}.")
    assert "tool:Skill" in seen, f"Skill tool never called; tools={seen}"
    assert MARKER in joined, f"SKILL.md content never reached the conversation: {joined[:200]!r}"
    return f"tools={seen} marker_present"


def note_b_autonomous_invocation(wd):
    """Layer 2: does the model pick the Skill up on its own from the description
    alone? gpt-4o-mini behind LiteLLM is not the production model, so a miss here
    is a model-behaviour datapoint, not a loading defect."""
    seen, joined = _agent_turn(wd, f"Please {REPORT_TASK}.")
    return f"tools={seen} marker_present={MARKER in joined}"


# --------------------------------------------------------------------------- #

def main():
    load_env_file()
    if not os.environ.get("LITELLM_MASTER_KEY"):
        print("LITELLM_MASTER_KEY not set; start the proxy per README.md", file=sys.stderr)
        return 2
    gateway_env()

    wd = make_workdir()
    print(f"workdir={wd}", flush=True)
    try:
        check("1. <workdir>/.claude/skills/ discovered, <workdir>/skills/ not",
              lambda: test_1_project_source(wd))
        check("2. <workdir>/skills/ (PDM-003 assumption) never discovered",
              lambda: test_2_flat_layout_never_loads(wd))
        check("3. setting_sources=[] suppresses discovery",
              lambda: test_3_setting_sources_empty_disables(wd))
        check("4. setting_sources omitted still loads project skills",
              lambda: test_4_default_setting_sources(wd))
        check("5. Skill tool present in the session tool list",
              lambda: test_5_skill_tool_available(wd))
        check("6. Skill tool resolves the skill and delivers SKILL.md content",
              lambda: test_6_skill_tool_delivers_skill_md(wd))
        observe("a. skills=[] filter is not reflected in init.skills (SDK 0.2.137)",
                lambda: note_a_skills_filter_not_visible_in_init(wd))
        observe("b. autonomous invocation from description alone (model behaviour)",
                lambda: note_b_autonomous_invocation(wd))
    finally:
        shutil.rmtree(wd, ignore_errors=True)

    graded = [r for r in results if r[0] != "NOTE"]
    failed = sum(1 for status, *_ in graded if status == "FAIL")
    lines = [
        "PDM-003 prerequisite 2 - Agent Skill loading path - results",
        "Tests 1-6 = loading mechanism (asserted). NOTE rows = model behaviour /",
        "SDK observations, recorded but not graded; the backend is gpt-4o-mini via",
        "LiteLLM, not an Anthropic model.",
        f"claude-agent-sdk={_ver('claude-agent-sdk')} python={sys.version.split()[0]}",
        "",
    ]
    lines += [f"[{s}] {n}" + (f"\n       {d}" if d else "") for s, n, d in results]
    lines += ["", f"{len(graded) - failed}/{len(graded)} passed"]
    (HERE / "results-skill-loading.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")
    print("\n".join(lines[-2:]))
    return 1 if failed else 0


def _ver(pkg):
    from importlib.metadata import version
    try:
        return version(pkg)
    except Exception:  # noqa: BLE001
        return "n/a"


if __name__ == "__main__":
    sys.exit(main())
