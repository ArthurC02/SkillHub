// Sandbox entrypoint: unpack the skill, run one agent turn, write the result.
//
// Everything it needs arrives in the environment (SBX-008). The provider passes
// paths and short-lived URLs and never opens what is behind them, so this file
// is the first thing that touches untrusted bytes — and it does so inside the
// sandbox, which is the whole point (iron rule 1).
//
// The four Skill-load conditions are all here, because missing any one of them
// silently loads no skills at all (PDM-003 spike §10, measured):
//   1. cwd points at the directory holding .claude/skills/
//   2. settingSources includes "project" — and excludes "user", so a stray
//      ~/.claude in the image cannot leak into a run
//   3. skills are left enabled (the default for what was discovered)
//   4. the tool list, if given, contains "Skill"
//
// It is also the Run Trace producer (TRACE-002~004). Events are appended, one
// JSON object per line, to $SKILLHUB_OUTDIR/trace/events.jsonl. They are not
// posted anywhere from in here: this container has no network (--network none),
// which is the point — sandboxd reads the file out and pushes it (TRACE-002).
import { execFileSync } from "node:child_process";
import { appendFileSync, mkdirSync, writeFileSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { join } from "node:path";
import { query } from "@anthropic-ai/claude-agent-sdk";

const workDir = process.env.SKILLHUB_WORKDIR ?? "/work";
const outDir = process.env.SKILLHUB_OUTDIR ?? "/out";
const skillDir = process.env.SKILLHUB_SKILL_DIR ?? join(workDir, ".claude", "skills");
const prompt = process.env.SKILLHUB_USER_PROMPT;

// --- trace emission ---------------------------------------------------------

const traceDir = join(outDir, "trace");
const tracePath = join(traceDir, "events.jsonl");
const runId = process.env.SKILLHUB_RUN_ID ?? "";
const attempt = Number(process.env.SKILLHUB_ATTEMPT ?? "1") || 1;

// seq is gapless from 1 and scoped to (run_id, attempt, emitted_by) — this
// process is the only `sandbox` producer for this attempt, so a hole in the
// stored sequence means an event was lost in transit and nothing else
// (contracts/events/README.md §7).
let seq = 0;
let traceReady = false;

// The schema's maxLength values, so a runaway tool result is cut here rather
// than rejected whole by the platform. Cutting sets the payload's `truncated`
// flag where the type has one, so the UI never shows a clipped string as
// complete.
const LIMITS = { message: 16000, text: 64000, result: 8000, reason: 2000, error: 4000 };

function clip(value, limit) {
  const s = typeof value === "string" ? value : JSON.stringify(value ?? null);
  if (s.length <= limit) return { value: s, truncated: false };
  return { value: s.slice(0, limit), truncated: true };
}

function emit(type, payload, status = "ok") {
  if (!runId) return; // nothing to attribute the event to
  if (!traceReady) {
    mkdirSync(traceDir, { recursive: true });
    traceReady = true;
  }
  seq += 1;
  const event = {
    schema_version: "1.0",
    event_id: randomUUID(),
    run_id: runId,
    attempt,
    seq,
    occurred_at: new Date().toISOString(),
    emitted_by: "sandbox",
    type,
    status,
    // Masking is the control plane's job and happens before storage
    // (TRACE-005, iron rule 11). Claiming `true` here would be this container
    // vouching for itself, which is exactly what the trust boundary forbids.
    masked: false,
    masked_fields: [],
    payload,
  };
  try {
    // One append per event, not one write at the end: sandboxd copies this file
    // while the run is still going, and a buffered writer would mean the trace
    // only ever appears after the workload has finished.
    appendFileSync(tracePath, JSON.stringify(event) + "\n");
  } catch {
    // A full /out must not take the run down with it. The gap shows up as a
    // hole in seq, which is precisely how a lost event is meant to surface.
  }
}

function finish(status, extra = {}) {
  // The provider reads state from the container, not from this file; the file
  // is for the artifact manifest and the agent's own output.
  writeFileSync(join(outDir, "result.json"), JSON.stringify({ status, ...extra }, null, 2));
  process.exit(status === "succeeded" ? 0 : 1);
}

// fail records the error in the trace before ending the run, so a failure the
// user can see in the timeline and the exit code always agree (TRACE-004).
function fail(category, code, message) {
  const clipped = clip(message, LIMITS.error);
  emit("error", { category, code, message: clipped.value, retryable: false }, "error");
  finish("failed", { error: clipped.value });
}

if (!prompt) {
  fail("execution", "missing_prompt", "SKILLHUB_USER_PROMPT is required");
}

// Fetch and unpack the skill package. The hash is verified by the control plane
// before the grant is minted; re-checking it here would need the archive on
// disk twice, and a sandbox vouching for its own inputs proves nothing anyway.
const pkg = process.env.SKILLHUB_SKILL_PACKAGE ? JSON.parse(process.env.SKILLHUB_SKILL_PACKAGE) : null;
if (pkg?.url) {
  const res = await fetch(pkg.url);
  if (!res.ok) {
    fail("provision", "skill_package_fetch_failed", `skill package fetch failed with ${res.status}`);
  }
  mkdirSync(skillDir, { recursive: true });
  const archive = join(workDir, "skill.tar.gz");
  writeFileSync(archive, Buffer.from(await res.arrayBuffer()));
  // Refuse absolute paths and ../ entries rather than trusting the archive.
  execFileSync("tar", ["-xzf", archive, "-C", skillDir, "--no-same-owner", "--no-same-permissions"], {
    stdio: "inherit",
  });
}

// --- agent turn -------------------------------------------------------------

// Tool calls are traced as one completed event each, carrying their own
// duration (TRACE-003, contracts/events/README.md §3): the tool_use block opens
// an entry here and the matching tool_result closes it.
const pending = new Map();

// A path the agent read that lives under the skill package is a resource read
// (TRACE-002); anything else is an ordinary file tool call and stays one.
function skillResourcePath(input) {
  const path = input?.file_path ?? input?.path ?? input?.notebook_path;
  if (typeof path !== "string" || !path.startsWith(skillDir)) return null;
  // Relative to the package root: absolute sandbox paths are not exported.
  return path.slice(skillDir.length).replace(/^[/\\]+/, "");
}

function openToolUse(block) {
  pending.set(block.id, { name: block.name, input: block.input, at: Date.now() });

  if (block.name === "Skill") {
    // TRACE-002. The SDK surfaces a skill being used as a `Skill` tool call;
    // a skill that was available and never invoked produces no block at all, so
    // `skipped` is not something this producer can observe and is left to
    // whoever can (the evaluator, EVAL-002). Recording a guess would be worse
    // than recording nothing.
    const name = block.input?.command ?? block.input?.skill ?? block.input?.name ?? "";
    emit("skill_activation", {
      skill_name: String(name).slice(0, 200),
      // The platform knows which version it mounted; the sandbox only ever sees
      // a directory name, so skill_version_id is filled in by nobody here and
      // the field is left out rather than invented.
      skill_version_id: process.env.SKILLHUB_SKILL_VERSION_ID ?? "",
      decision: "activated",
      reason: clip(block.input?.description ?? "", LIMITS.reason).value || null,
    });
  }
}

function closeToolUse(block) {
  const open = pending.get(block.tool_use_id);
  if (!open) return;
  pending.delete(block.tool_use_id);

  const durationMs = Date.now() - open.at;
  const failed = block.is_error === true;
  const rendered = clip(block.content, LIMITS.result);

  const resource = skillResourcePath(open.input);
  if (resource) {
    emit("resource_read", {
      resource_path: resource.slice(0, 1024),
      outcome: failed ? "not_found" : "read",
      bytes_read: failed ? null : rendered.value.length,
      truncated: rendered.truncated,
    }, failed ? "error" : "ok");
  }

  // TRACE-003: script output. Bash is the only tool in the allow list that runs
  // a script, and its result is that script's combined output.
  if (open.name === "Bash") {
    const log = clip(block.content, LIMITS.message);
    emit("script_log", {
      script_path: null,
      stream: failed ? "stderr" : "stdout",
      message: log.value,
      truncated: log.truncated,
      dropped_bytes: null, // unknown, and the UI must show unknown rather than 0
    }, failed ? "error" : "ok");
  }

  emit("tool_call", {
    tool_name: String(open.name).slice(0, 200),
    invocation_id: String(block.tool_use_id).slice(0, 200),
    arguments: open.input ?? null,
    result_summary: rendered.value,
    outcome: failed ? "failed" : "succeeded",
    duration_ms: durationMs,
    truncated: rendered.truncated,
  }, failed ? "error" : "ok");
}

const messages = [];
let output = "";
const startedAt = Date.now();
try {
  for await (const msg of query({
    prompt,
    options: {
      cwd: workDir,
      settingSources: ["project"],
      allowedTools: ["Skill", "Read", "Write", "Edit", "Glob", "Grep", "Bash"],
      model: process.env.SKILLHUB_MODEL,
      permissionMode: "bypassPermissions",
    },
  })) {
    messages.push(msg.type);
    const blocks = Array.isArray(msg.message?.content) ? msg.message.content : [];
    for (const block of blocks) {
      if (block.type === "tool_use") openToolUse(block);
      else if (block.type === "tool_result") closeToolUse(block);
      else if (block.type === "text" && msg.type === "assistant") {
        const text = clip(block.text ?? "", LIMITS.text);
        if (text.value.trim() !== "") {
          emit("agent_output", { kind: "intermediate", text: text.value, truncated: text.truncated });
        }
      }
    }

    if (msg.type === "result") {
      output = msg.result ?? "";
      const text = clip(output, LIMITS.text);
      emit("agent_output", { kind: "final", text: text.value, truncated: text.truncated },
        msg.is_error ? "error" : "ok");

      // TRACE-004: tokens and cost. Both come from whatever the gateway
      // reported back through the SDK (iron rule 8), never from a local
      // estimate. `cost_usd: null` means the gateway did not report one and the
      // UI must render that as unreported, not as zero (contract README §5) -
      // which is the current state, because the model gateway grant is not
      // minted yet (SBX-008) and there is nothing to report.
      const usage = msg.usage ?? {};
      const cost = typeof msg.total_cost_usd === "number" ? msg.total_cost_usd : null;
      emit("usage", {
        scope: "run_total",
        model: process.env.SKILLHUB_MODEL ?? "",
        input_tokens: usage.input_tokens ?? 0,
        output_tokens: usage.output_tokens ?? 0,
        cache_read_input_tokens: usage.cache_read_input_tokens ?? null,
        cache_write_input_tokens: usage.cache_creation_input_tokens ?? null,
        cost_usd: cost,
        cost_source: cost === null ? null : "gateway",
        duration_ms: Date.now() - startedAt,
      });
    }
  }
} catch (err) {
  fail("execution", "agent_turn_failed", String(err?.message ?? err));
}

finish("succeeded", { agent_output: output, message_types: messages });
