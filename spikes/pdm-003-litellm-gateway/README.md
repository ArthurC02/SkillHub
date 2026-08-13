# PDM-003 spike: LiteLLM Anthropic-compatible endpoint

Verifies that LiteLLM Proxy's Anthropic-compatible `/v1/messages` endpoint behaves
correctly under tool use and streaming, and that the per-Run Virtual Key flow
(model scoping + budget enforcement, injected via `ANTHROPIC_BASE_URL` /
`ANTHROPIC_AUTH_TOKEN`) works end to end.

**Scope note (updated 2026-08-14).** The model provider is now decided: **OpenAI
API behind the LiteLLM gateway** (ADR-017 unchanged). The client still speaks the
Anthropic Messages protocol and LiteLLM translates it to the OpenAI backend, so
`test_gateway.py` and `test_skill_loading.py` keep their original
`sonnet-test`/`haiku-test` → `gpt-4o-mini` aliases for reproducibility, while
`test_supplemental.py` runs against current-generation models
(`flagship-test` → `gpt-5.6-sol`, `mini-test` → `gpt-5.4-mini`). The earlier
caveat "does not validate an Anthropic model behind the gateway" no longer
applies — there is no Anthropic model in the design. See
`plans/mvp/m0/pdm-003-litellm-spike-report.md`.

## Results

`results.txt` (regenerated on each run). Contains no credentials.

## Prerequisites

- Python 3.14, Docker, and `OPENAI_API_KEY` in the repo-root `.env` (gitignored).
- Test E (Virtual Key) needs a Postgres for LiteLLM's key store.

## Setup

```bash
python -m venv .venv
.venv/Scripts/python.exe -m pip install "litellm[proxy]" openai anthropic claude-agent-sdk
# litellm 1.96.2 is incompatible with fastapi >= 0.140 (ImportError: get_flat_dependant)
.venv/Scripts/python.exe -m pip install "fastapi<0.140"
```

## Run

### Tests A-D only (no database needed)

The pip-installed proxy is started and stopped by the test script itself:

```bash
.venv/Scripts/python.exe test_gateway.py
```

### Full run including E (Virtual Key) and F (Claude Agent SDK)

`/key/generate` requires a database, and the pip-installed proxy cannot reach it
on this platform (the bundled Prisma CLI rejects LiteLLM's schema — see the
report). Use the official image instead:

```bash
# 1. scratch Postgres for the LiteLLM key store
docker run -d --name pdm003-litellm-db \
  -e POSTGRES_PASSWORD=spikepw -e POSTGRES_DB=litellm \
  -p 5544:5432 postgres:17-alpine

# 2. LiteLLM proxy (MSYS_NO_PATHCONV=1 stops Git Bash mangling container paths)
MK="sk-$(python -c 'import secrets;print(secrets.token_urlsafe(24))')"
OAI=$(grep '^OPENAI_API_KEY=' ../../.env | cut -d= -f2-)
MSYS_NO_PATHCONV=1 docker run -d --name pdm003-litellm -p 4000:4000 \
  -e LITELLM_MASTER_KEY="$MK" -e OPENAI_API_KEY="$OAI" \
  -e DATABASE_URL="postgresql://postgres:spikepw@host.docker.internal:5544/litellm" \
  -e STORE_MODEL_IN_DB="False" \
  -v "$(pwd -W)/config.yaml:/app/config.yaml:ro" \
  ghcr.io/berriai/litellm:main-stable --config /app/config.yaml --port 4000

# 3. run the suite against it
EXTERNAL_PROXY=1 LITELLM_MASTER_KEY="$MK" .venv/Scripts/python.exe test_gateway.py
```

### Skill loading path (PDM-003 prerequisite 2)

Needs the same proxy running (steps 1-2 above):

```bash
EXTERNAL_PROXY=1 LITELLM_MASTER_KEY="$MK" .venv/Scripts/python.exe test_skill_loading.py
```

Tests 1-5 read only the SDK's `init` system message and never call the model, so
they are backend-independent. Test 6 and the two `[NOTE]` rows do call it.

### Supplemental: autonomous Skill invocation, `skills` whitelist, prompt caching

Report §11. Needs the same proxy running (steps 1-2 above). Costs about **$1** in
real model calls (189 calls on the recorded run):

```bash
EXTERNAL_PROXY=1 LITELLM_MASTER_KEY="$MK" .venv/Scripts/python.exe test_supplemental.py
```

Rows are `PASS`/`FAIL` for assertions, `DATA` for measurements, and `GAP` for a
measured LiteLLM limitation being documented rather than a broken test (only
`FAIL` sets the exit code). B4/B5 read LiteLLM's own `/spend/logs` rather than the
SDK's `usage`, because `ResultMessage.usage.input_tokens` was observed to report
the last API call on some turns and a sum on others; the gateway log is what a
per-Run Virtual Key budget is actually charged against.

## Teardown

```bash
docker rm -f pdm003-litellm pdm003-litellm-db
```

## Files

| File | Purpose |
| --- | --- |
| `config.yaml` | LiteLLM model aliases (`sonnet-test`/`haiku-test` → `gpt-4o-mini`; `flagship-test` → `gpt-5.6-sol`; `mini-test` → `gpt-5.4-mini`) |
| `test_gateway.py` | The whole suite; also starts/stops the pip proxy unless `EXTERNAL_PROXY=1` |
| `test_skill_loading.py` | Where the Agent SDK actually loads Skills from (`.claude/skills/`, not `skills/`) |
| `test_supplemental.py` | Autonomous Skill invocation rate, `skills` whitelist filtering, prompt caching / 300K ceiling |
| `results.txt` | PASS/FAIL summary, generated |
| `results-skill-loading.txt` | PASS/FAIL summary for the Skill loading tests, generated |
| `results-supplemental.txt` | PASS/FAIL/DATA summary for the supplemental tests, generated |
| `proxy.log` | Proxy stdout when the script starts it, generated |

## Notes

- Credentials are read from `.env` only, and are never printed, logged, or written
  to `results.txt`.
- `MAX_THINKING_TOKENS=0` is set inside test F on purpose: LiteLLM 1.96.2 forwards
  Anthropic's `thinking` block to the OpenAI backend as `reasoning_effort` and does
  not honour `drop_params` on the `/v1/messages` route. Not needed with an
  Anthropic backend.
