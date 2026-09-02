// How a clean-mode launch resolves the repository's deployment variables.
//
// Split from start.mjs because start.mjs starts three processes on import and
// cannot be required by a test, and because this is the part with branches: a
// parser and a precedence rule, both of which fail silently when wrong. The
// first version of the overlay below shadowed a per-launch secret with a blank
// line from .env, and the only symptom would have been every failed Run saying
// "workload exited with code 1" — the exact failure start.mjs already records
// as the reason it passes that secret at all.
//
// # Why .env is read here at all
//
// `task env:init` creates it, the README points at it, it documents four dozen
// deployment variables — and no Go process reads a line of it. Someone who set
// GENERATE_SKILL_EXPOSED=on there and started this mode got silence. The other
// launcher on this repository already closed the same gap: `task dev:model`
// passes `--env-file .env` to compose, and the Taskfile comment beside it
// records what omitting it cost (an empty gateway master key, 401 on every
// call, 04 丙-56).
//
// # Why the result is not put into process.env
//
// start.mjs hands `...process.env` to BOTH children, and it deliberately keeps
// the control plane's DSN away from sandboxd ("the driver's workload env is an
// allowlist precisely so a DSN never reaches a skill"). Pouring four dozen
// deployment variables into process.env would hand sandboxd that DSN on every
// launch. The overlay goes to the API child and nowhere else.
import { existsSync, readFileSync } from "node:fs";

/**
 * Minimal `KEY=value` reader. Not a dotenv library: this file's dependency
 * budget is Node's standard library — 02:PORT-005 is about a machine that
 * installs nothing — and the format .env.example writes is this small.
 *
 * Values may contain `=` (a DSN's query string does), so only the first one
 * separates. Surrounding quotes are stripped; nothing else is interpreted,
 * because nothing else appears in .env.example.
 */
export function parseDotEnv(text) {
  const out = {};
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const eq = trimmed.indexOf("=");
    if (eq < 1) continue;
    const key = trimmed
      .slice(0, eq)
      .replace(/^export\s+/, "")
      .trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) continue;
    let value = trimmed.slice(eq + 1).trim();
    if (value.length > 1 && /^(".*"|'.*')$/.test(value)) value = value.slice(1, -1);
    out[key] = value;
  }
  return out;
}

/** The file is optional: a checkout that never ran `task env:init` still boots. */
export function readDotEnv(path) {
  return existsSync(path) ? parseDotEnv(readFileSync(path, "utf8")) : {};
}

/**
 * One deployment variable as this launch sees it.
 *
 * The shell wins over the file for the reason every dotenv implementation
 * agrees on: an explicit export is the more immediate instruction.
 */
export function resolve(dotEnv, shellEnv, name) {
  return shellEnv[name] ?? dotEnv[name] ?? "";
}

/**
 * What .env contributes to the API child: keys the shell has not set, and
 * nothing blank.
 *
 * Blank is dropped for a reason that is not tidiness. .env.example ships four
 * dozen keys with empty values, and an empty value carries no information —
 * envx's contract is literally "unset or empty" and compose writes `${X:-}` for
 * the same reason. What a blank entry DOES carry is the power to shadow:
 * start.mjs mints SKILLHUB_TRACE_INGEST_SECRET per launch, and the child env is
 * built by spreading this overlay over process.env, so a blank line in .env
 * would land on top of the minted value and switch tracing off silently.
 */
export function childOverlay(dotEnv, shellEnv) {
  return Object.fromEntries(
    Object.entries(dotEnv).filter(([k, v]) => shellEnv[k] === undefined && v !== ""),
  );
}
