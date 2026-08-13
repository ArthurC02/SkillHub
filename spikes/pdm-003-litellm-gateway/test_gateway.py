"""PDM-003 spike: verify LiteLLM Proxy's Anthropic-compatible /v1/messages endpoint
under tool use + streaming, and the per-Run Virtual Key flow.

Scope: protocol layer only. The client speaks Anthropic Messages format and
LiteLLM translates to an OpenAI backend, because this environment has no
Anthropic key. See the report for what that does and does not prove.

Run:  .venv/Scripts/python.exe test_gateway.py
Needs: OPENAI_API_KEY in ../../.env, and DATABASE_URL pointing at a scratch
Postgres (only for test E, the Virtual Key flow).

Secrets are never printed, logged, or written to results.txt.
"""

import json
import os
import secrets
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

HERE = Path(__file__).parent
REPO = HERE.parent.parent
PORT = 4000
BASE = f"http://127.0.0.1:{PORT}"
MODEL = "sonnet-test"
OTHER_MODEL = "haiku-test"

WEATHER_TOOL = {
    "name": "get_weather",
    "description": "Get the current weather in a given city.",
    "input_schema": {
        "type": "object",
        "properties": {
            "location": {"type": "string", "description": "City name, e.g. Taipei"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]},
        },
        "required": ["location"],
    },
}

results = []


def record(name, ok, detail=""):
    results.append(("PASS" if ok else "FAIL", name, detail))
    print(f"[{'PASS' if ok else 'FAIL'}] {name} {detail}", flush=True)


def check(name, fn):
    try:
        record(name, True, fn() or "")
    except Exception as exc:  # noqa: BLE001 - spike wants every failure recorded
        record(name, False, f"{type(exc).__name__}: {exc}")


def load_env_file():
    """Read .env, put OPENAI_API_KEY into os.environ. Value is never echoed."""
    path = REPO / ".env"
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        os.environ.setdefault(key.strip(), value.strip())
    assert os.environ.get("OPENAI_API_KEY"), "OPENAI_API_KEY missing from .env"


def api(path, payload=None, token=None, method=None):
    req = urllib.request.Request(
        f"{BASE}{path}",
        data=json.dumps(payload).encode() if payload is not None else None,
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {token}"},
        method=method or ("POST" if payload is not None else "GET"),
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp)


def _litellm_exe():
    """litellm ships a console script only - `python -m litellm` does not work."""
    scripts = Path(sys.executable).parent
    for name in ("litellm.exe", "litellm"):
        if (scripts / name).exists():
            return str(scripts / name)
    raise RuntimeError(f"litellm console script not found in {scripts}")


def start_proxy(master_key):
    env = dict(os.environ, LITELLM_MASTER_KEY=master_key, LITELLM_LOG="ERROR")
    log = open(HERE / "proxy.log", "w", encoding="utf-8", errors="replace")
    proc = subprocess.Popen(
        [_litellm_exe(), "--config", str(HERE / "config.yaml"),
         "--port", str(PORT), "--num_workers", "1"],
        cwd=str(HERE), env=env, stdout=log, stderr=subprocess.STDOUT,
    )
    deadline = time.time() + 240
    while time.time() < deadline:
        if proc.poll() is not None:
            raise RuntimeError(f"proxy exited early (rc={proc.returncode}), see proxy.log")
        try:
            api("/health/liveliness", token=master_key)
            return proc, log
        except Exception:
            time.sleep(2)
    proc.terminate()
    raise RuntimeError("proxy did not become healthy within 240s")


def anthropic_client(token):
    """Build the client exactly the way a Sandbox would: env-var injection only."""
    import anthropic

    os.environ.pop("ANTHROPIC_API_KEY", None)  # PDM-003: must be unset, not empty
    os.environ["ANTHROPIC_BASE_URL"] = BASE
    os.environ["ANTHROPIC_AUTH_TOKEN"] = token
    return anthropic.Anthropic(max_retries=0)


# --------------------------------------------------------------------------- #
# Test cases
# --------------------------------------------------------------------------- #

def test_a_basic(client):
    msg = client.messages.create(
        model=MODEL, max_tokens=128,
        messages=[{"role": "user", "content": "Reply with exactly: hello spike"}],
    )
    assert msg.type == "message", f"type={msg.type}"
    assert msg.role == "assistant", f"role={msg.role}"
    assert msg.content and msg.content[0].type == "text", f"content={msg.content}"
    assert msg.content[0].text.strip(), "empty text block"
    assert msg.stop_reason in ("end_turn", "max_tokens"), f"stop_reason={msg.stop_reason}"
    assert msg.usage.input_tokens > 0 and msg.usage.output_tokens > 0, "usage not populated"
    return (f"blocks={len(msg.content)} stop={msg.stop_reason} "
            f"usage={msg.usage.input_tokens}/{msg.usage.output_tokens}")


def test_b_streaming(client):
    seen = []
    with client.messages.stream(
        model=MODEL, max_tokens=128,
        messages=[{"role": "user", "content": "Count from 1 to 5, one number per line."}],
    ) as stream:
        for event in stream:
            seen.append(event.type)
        final = stream.get_final_message()

    assert seen[0] == "message_start", f"first event={seen[0]}"
    assert "content_block_start" in seen, "no content_block_start"
    assert any(t in seen for t in ("content_block_delta", "text")), "no delta events"
    assert "content_block_stop" in seen, "no content_block_stop"
    assert "message_stop" in seen, "no message_stop"
    assert seen.index("message_start") < seen.index("content_block_start") \
        < seen.index("content_block_stop") < seen.index("message_stop"), "event order wrong"
    assert final.content[0].text.strip(), "reassembled text empty"
    return f"events={len(seen)} distinct={sorted(set(seen))}"


def test_c_tool_use(client):
    messages = [{"role": "user", "content": "What is the weather in Taipei right now?"}]
    first = client.messages.create(
        model=MODEL, max_tokens=512, tools=[WEATHER_TOOL], messages=messages,
    )
    assert first.stop_reason == "tool_use", f"stop_reason={first.stop_reason}"
    blocks = [b for b in first.content if b.type == "tool_use"]
    assert blocks, f"no tool_use block: {[b.type for b in first.content]}"
    tu = blocks[0]
    assert tu.name == "get_weather", f"tool name={tu.name}"
    assert isinstance(tu.input, dict) and tu.input.get("location"), f"input={tu.input}"
    assert tu.id, "tool_use block has no id"

    messages.append({"role": "assistant", "content": first.content})
    messages.append({"role": "user", "content": [{
        "type": "tool_result", "tool_use_id": tu.id,
        "content": "18 degrees celsius, light rain",
    }]})
    second = client.messages.create(
        model=MODEL, max_tokens=512, tools=[WEATHER_TOOL], messages=messages,
    )
    assert second.stop_reason == "end_turn", f"final stop_reason={second.stop_reason}"
    text = "".join(b.text for b in second.content if b.type == "text")
    assert "18" in text, f"tool_result not reflected in final answer: {text[:120]!r}"
    return f"tool={tu.name} input={tu.input} round_trip_ok"


def test_d_tool_use_streaming(client):
    types, partials = [], []
    tool_name, tool_id = None, None
    with client.messages.stream(
        model=MODEL, max_tokens=512, tools=[WEATHER_TOOL],
        messages=[{"role": "user", "content": "What is the weather in Kyoto?"}],
    ) as stream:
        for event in stream:
            types.append(event.type)
            if event.type == "content_block_start" and event.content_block.type == "tool_use":
                tool_name = event.content_block.name
                tool_id = event.content_block.id
            if event.type == "content_block_delta" and event.delta.type == "input_json_delta":
                partials.append(event.delta.partial_json)
        final = stream.get_final_message()

    assert tool_name == "get_weather", f"streamed tool name={tool_name}"
    assert tool_id, "streamed tool_use block has no id"
    assert partials, "no input_json_delta events emitted"
    args = json.loads("".join(partials))
    assert args.get("location"), f"reassembled args missing location: {args}"

    final_tu = [b for b in final.content if b.type == "tool_use"]
    assert final_tu, "final message has no tool_use block"
    assert final_tu[0].input == args, f"SDK-assembled {final_tu[0].input} != manual {args}"
    assert final.stop_reason == "tool_use", f"stop_reason={final.stop_reason}"
    return f"deltas={len(partials)} args={args} matches_sdk_assembly"


def test_e_virtual_key(master_key):
    import anthropic

    # Per-Run key: model-scoped, budget-capped, short TTL (ADR-017 / PDM-003).
    created = api("/key/generate", {
        "models": [MODEL],
        "max_budget": 0.000002,
        "duration": "20m",
        "metadata": {"run_id": "spike-run-0001"},
    }, token=master_key)
    vkey = created["key"]
    assert vkey, "no key returned"
    notes = ["created"]

    client = anthropic_client(vkey)

    # 1. allowed model works via ANTHROPIC_AUTH_TOKEN injection
    msg = client.messages.create(
        model=MODEL, max_tokens=32,
        messages=[{"role": "user", "content": "say ok"}],
    )
    assert msg.content, "allowed model returned no content"
    notes.append("allowed_model_ok")

    # 2. model outside the key's allow-list is rejected
    try:
        client.messages.create(
            model=OTHER_MODEL, max_tokens=32,
            messages=[{"role": "user", "content": "say ok"}],
        )
        raise AssertionError("disallowed model was NOT rejected")
    except anthropic.APIStatusError as exc:
        assert exc.status_code in (400, 401, 403), f"unexpected status {exc.status_code}"
        notes.append(f"disallowed_model_rejected({exc.status_code})")

    # 3. budget exhaustion blocks further calls (spend flush is async)
    blocked = None
    for _ in range(12):
        time.sleep(2)
        try:
            client.messages.create(
                model=MODEL, max_tokens=32,
                messages=[{"role": "user", "content": "say ok"}],
            )
        except anthropic.APIStatusError as exc:
            blocked = exc.status_code
            break
    assert blocked, "budget was never enforced after 12 attempts"
    notes.append(f"budget_exhausted_rejected({blocked})")

    return " ".join(notes)


def test_f_claude_agent_sdk(token):
    """Does the real Agent SDK harness talk to the gateway via env-var injection?

    Uses an in-process SDK MCP tool so nothing touches the filesystem and no
    permission prompt can appear.
    """
    import asyncio

    from claude_agent_sdk import (ClaudeAgentOptions, create_sdk_mcp_server,
                                  query, tool)

    @tool("get_weather", "Get current weather for a city", {"location": str})
    async def get_weather(args):
        return {"content": [{"type": "text",
                             "text": f"18 degrees celsius, light rain in {args['location']}"}]}

    server = create_sdk_mcp_server(name="spike", version="1.0.0", tools=[get_weather])

    os.environ.pop("ANTHROPIC_API_KEY", None)
    os.environ["ANTHROPIC_BASE_URL"] = BASE
    os.environ["ANTHROPIC_AUTH_TOKEN"] = token
    # Claude Code sends an Anthropic `thinking` block by default. LiteLLM 1.96.2
    # translates it to OpenAI `reasoning_effort` and does NOT honour
    # drop_params/additional_drop_params on the /v1/messages route, so the OpenAI
    # backend 400s. Disabling thinking client-side is the only workaround that
    # does not require patching the gateway. Not needed with an Anthropic backend.
    os.environ["MAX_THINKING_TOKENS"] = "0"

    async def run():
        seen, texts, usage = [], [], {}
        options = ClaudeAgentOptions(
            model=MODEL,
            mcp_servers={"spike": server},
            allowed_tools=["mcp__spike__get_weather"],
            permission_mode="bypassPermissions",
            setting_sources=[],
            max_turns=4,
        )
        async for message in query(
            prompt="What is the weather in Taipei? Use the get_weather tool, then answer.",
            options=options,
        ):
            for block in getattr(message, "content", []) or []:
                seen.append(type(block).__name__)
                if type(block).__name__ == "ToolUseBlock":
                    seen.append(f"tool:{getattr(block, 'name', '?')}")
                if type(block).__name__ == "TextBlock":
                    texts.append(getattr(block, "text", ""))
            if type(message).__name__ == "ResultMessage":
                usage.update(getattr(message, "usage", None) or {})
                usage["is_error"] = getattr(message, "is_error", None)
                usage["num_turns"] = getattr(message, "num_turns", None)
        return seen, texts, usage

    seen, texts, usage = asyncio.run(asyncio.wait_for(run(), timeout=180))
    assert usage.get("is_error") is False, f"agent run ended in error: {usage}"
    assert any(s.startswith("tool:") and "get_weather" in s for s in seen), \
        f"agent never called the tool; blocks seen: {seen}"
    joined = " ".join(texts)
    assert "18" in joined, f"tool result not reflected in answer: {joined[:160]!r}"
    return (f"turns={usage.get('num_turns')} "
            f"harness_input_tokens={usage.get('input_tokens')} "
            f"output_tokens={usage.get('output_tokens')} blocks={seen}")


# --------------------------------------------------------------------------- #

def main():
    load_env_file()
    # EXTERNAL_PROXY=1 means a proxy is already listening on PORT (e.g. the
    # official docker image); we then only need the matching master key.
    external = os.environ.get("EXTERNAL_PROXY")
    master_key = os.environ.get("LITELLM_MASTER_KEY") or ("sk-" + secrets.token_urlsafe(24))
    if not external and not os.environ.get("DATABASE_URL"):
        print("warning: DATABASE_URL unset, virtual key test will fail", flush=True)

    proc = log = None
    try:
        if external:
            api("/health/liveliness", token=master_key)
            record("proxy startup (external, /health/liveliness)", True)
        else:
            proc, log = start_proxy(master_key)
            record("proxy startup (/health/liveliness)", True)

        client = anthropic_client(master_key)
        check("A. non-streaming basic message", lambda: test_a_basic(client))
        check("B. streaming event sequence", lambda: test_b_streaming(client))
        check("C. tool use, non-streaming round trip", lambda: test_c_tool_use(client))
        check("D. tool use + streaming (input_json_delta)", lambda: test_d_tool_use_streaming(client))
        check("E. virtual key: scope + budget enforcement", lambda: test_e_virtual_key(master_key))
        check("F. claude-agent-sdk via ANTHROPIC_BASE_URL",
              lambda: test_f_claude_agent_sdk(master_key))
    except Exception as exc:  # noqa: BLE001
        record("proxy startup (/health/liveliness)", False, f"{type(exc).__name__}: {exc}")
    finally:
        if proc:
            proc.terminate()
            try:
                proc.wait(timeout=20)
            except subprocess.TimeoutExpired:
                proc.kill()
        if log:
            log.close()

    failed = sum(1 for status, *_ in results if status == "FAIL")
    lines = [
        "PDM-003 LiteLLM gateway spike - results",
        "Scope: Anthropic Messages protocol over LiteLLM -> OpenAI backend.",
        "Anthropic-model-behind-the-gateway is NOT covered (no Anthropic key here).",
        f"litellm={_ver('litellm')} anthropic={_ver('anthropic')} openai={_ver('openai')} python={sys.version.split()[0]}",
        "",
    ]
    lines += [f"[{s}] {n}" + (f"\n       {d}" if d else "") for s, n, d in results]
    lines += ["", f"{len(results) - failed}/{len(results)} passed"]
    (HERE / "results.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")
    print("\n".join(lines[-3:]))
    return 1 if failed else 0


def _ver(pkg):
    from importlib.metadata import version
    try:
        return version(pkg)
    except Exception:  # noqa: BLE001
        return "n/a"


if __name__ == "__main__":
    sys.exit(main())
