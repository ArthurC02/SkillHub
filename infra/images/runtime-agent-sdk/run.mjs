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
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { query } from "@anthropic-ai/claude-agent-sdk";

const workDir = process.env.SKILLHUB_WORKDIR ?? "/work";
const outDir = process.env.SKILLHUB_OUTDIR ?? "/out";
const skillDir = process.env.SKILLHUB_SKILL_DIR ?? join(workDir, ".claude", "skills");
const prompt = process.env.SKILLHUB_USER_PROMPT;

function finish(status, extra = {}) {
  // The provider reads state from the container, not from this file; the file
  // is for the artifact manifest and the agent's own output.
  writeFileSync(join(outDir, "result.json"), JSON.stringify({ status, ...extra }, null, 2));
  process.exit(status === "succeeded" ? 0 : 1);
}

if (!prompt) {
  finish("failed", { error: "SKILLHUB_USER_PROMPT is required" });
}

// Fetch and unpack the skill package. The hash is verified by the control plane
// before the grant is minted; re-checking it here would need the archive on
// disk twice, and a sandbox vouching for its own inputs proves nothing anyway.
const pkg = process.env.SKILLHUB_SKILL_PACKAGE ? JSON.parse(process.env.SKILLHUB_SKILL_PACKAGE) : null;
if (pkg?.url) {
  const res = await fetch(pkg.url);
  if (!res.ok) {
    finish("failed", { error: `skill package fetch failed with ${res.status}` });
  }
  mkdirSync(skillDir, { recursive: true });
  const archive = join(workDir, "skill.tar.gz");
  writeFileSync(archive, Buffer.from(await res.arrayBuffer()));
  // Refuse absolute paths and ../ entries rather than trusting the archive.
  execFileSync("tar", ["-xzf", archive, "-C", skillDir, "--no-same-owner", "--no-same-permissions"], {
    stdio: "inherit",
  });
}

const messages = [];
let output = "";
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
    if (msg.type === "result") {
      output = msg.result ?? "";
    }
  }
} catch (err) {
  finish("failed", { error: String(err?.message ?? err) });
}

finish("succeeded", { agent_output: output, message_types: messages });
