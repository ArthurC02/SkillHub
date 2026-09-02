// Tests for how a clean-mode launch resolves deployment variables.
//
// `node --test`, no framework, same shape as
// infra/images/runtime-agent-sdk/run.test.mjs — the machine 02:PORT-005 is
// about installs nothing, and neither does this.
import assert from "node:assert/strict";
import { test } from "node:test";

import { childOverlay, parseDotEnv, resolve } from "./env.mjs";

test("parses the shapes .env.example actually writes", () => {
  const env = parseDotEnv(
    [
      "# a comment",
      "",
      "DEV_LOGIN=1",
      "  APP_URL=http://localhost:5173  ",
      "EMPTY=",
      // A DSN's query string puts `=` in the value; only the first one separates.
      "DATABASE_URL=postgres://u:p@localhost:5432/db?sslmode=disable",
      'QUOTED="a value"',
      "export EXPORTED=yes",
      "not a variable line",
      "=novalue",
    ].join("\n"),
  );
  assert.deepEqual(env, {
    DEV_LOGIN: "1",
    APP_URL: "http://localhost:5173",
    EMPTY: "",
    DATABASE_URL: "postgres://u:p@localhost:5432/db?sslmode=disable",
    QUOTED: "a value",
    EXPORTED: "yes",
  });
});

test("the shell wins over the file, and the file over nothing", () => {
  const dotEnv = { A: "from-file", B: "from-file" };
  const shell = { A: "from-shell" };
  assert.equal(resolve(dotEnv, shell, "A"), "from-shell");
  assert.equal(resolve(dotEnv, shell, "B"), "from-file");
  assert.equal(resolve(dotEnv, shell, "C"), "");
});

test("the overlay carries only what the shell has not set", () => {
  const overlay = childOverlay({ A: "file", B: "file" }, { A: "shell" });
  assert.deepEqual(overlay, { B: "file" });
});

// The bug this file exists for. .env.example ships four dozen keys with empty
// values; SKILLHUB_TRACE_INGEST_SECRET is one of them, and start.mjs mints it
// per launch. The child env is process.env with this overlay spread ON TOP, so
// a blank entry here lands on the minted value and switches tracing off — with
// no error, and every failed Run then says only "workload exited with code 1".
test("a blank line in .env cannot shadow a value the launcher mints", () => {
  const overlay = childOverlay({ SKILLHUB_TRACE_INGEST_SECRET: "" }, {});
  assert.deepEqual(overlay, {});

  const minted = { SKILLHUB_TRACE_INGEST_SECRET: "a-real-secret" };
  const childEnv = { ...minted, ...overlay };
  assert.equal(childEnv.SKILLHUB_TRACE_INGEST_SECRET, "a-real-secret");
});

// ownedSettings() reads through resolve() for the same reason: its contract is
// "fills gaps, it does not override", and a value written in .env was supplied
// by somebody. Minting over it would run the launch on a secret nobody chose.
test("a value supplied in .env counts as supplied", () => {
  assert.equal(resolve({ SKILLHUB_TRACE_INGEST_SECRET: "mine" }, {}, "SKILLHUB_TRACE_INGEST_SECRET"), "mine");
  assert.equal(resolve({ SKILLHUB_TRACE_INGEST_SECRET: "" }, {}, "SKILLHUB_TRACE_INGEST_SECRET"), "");
});

test("a checkout with no .env still resolves, to nothing", () => {
  assert.deepEqual(childOverlay({}, { PATH: "/usr/bin" }), {});
  assert.equal(resolve({}, {}, "ANYTHING"), "");
});
