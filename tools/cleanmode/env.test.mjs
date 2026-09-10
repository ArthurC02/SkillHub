import assert from "node:assert/strict";
import { test } from "node:test";

import { childOverlay, parseDotEnv, releasePath, resolve } from "./env.mjs";

test("parses the shapes .env.example actually writes", () => {
  const env = parseDotEnv(
    [
      "# a comment",
      "",
      "DEV_LOGIN=1",
      "  APP_URL=http://localhost:5173  ",
      "EMPTY=",
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

test("a blank line in .env cannot shadow a value the launcher mints", () => {
  const overlay = childOverlay({ SKILLHUB_TRACE_INGEST_SECRET: "" }, {});
  assert.deepEqual(overlay, {});

  const minted = { SKILLHUB_TRACE_INGEST_SECRET: "a-real-secret" };
  const childEnv = { ...minted, ...overlay };
  assert.equal(childEnv.SKILLHUB_TRACE_INGEST_SECRET, "a-real-secret");
});

test("a value supplied in .env counts as supplied", () => {
  assert.equal(resolve({ SKILLHUB_TRACE_INGEST_SECRET: "mine" }, {}, "SKILLHUB_TRACE_INGEST_SECRET"), "mine");
  assert.equal(resolve({ SKILLHUB_TRACE_INGEST_SECRET: "" }, {}, "SKILLHUB_TRACE_INGEST_SECRET"), "");
});

test("a checkout with no .env still resolves, to nothing", () => {
  assert.deepEqual(childOverlay({}, { PATH: "/usr/bin" }), {});
  assert.equal(resolve({}, {}, "ANYTHING"), "");
});

test("a trailing comment is not part of the value", () => {
  assert.equal(parseDotEnv("GENERATE_SKILL_EXPOSED=on # 只在 demo 開").GENERATE_SKILL_EXPOSED, "on");
  assert.equal(parseDotEnv("SKILLHUB_RUN_MODEL=gpt-5.4-mini # mini 層").SKILLHUB_RUN_MODEL, "gpt-5.4-mini");
  assert.equal(parseDotEnv("A=p#ss").A, "p#ss");
  assert.equal(parseDotEnv('A="on # 真的要這個字串"').A, "on # 真的要這個字串");
});

test("an exported-but-empty shell variable is not an instruction", () => {
  const dotEnv = { OPERATOR_USER_IDS: "abc" };
  assert.equal(resolve(dotEnv, { OPERATOR_USER_IDS: "" }, "OPERATOR_USER_IDS"), "abc");
  assert.deepEqual(childOverlay(dotEnv, { OPERATOR_USER_IDS: "" }), { OPERATOR_USER_IDS: "abc" });
  assert.equal(resolve(dotEnv, { OPERATOR_USER_IDS: "xyz" }, "OPERATOR_USER_IDS"), "xyz");
  assert.deepEqual(childOverlay(dotEnv, { OPERATOR_USER_IDS: "xyz" }), {});
});

test("the release list the launcher announces is the one the API will read", () => {
  const fallback = "/tmp/launcher-owned.txt";
  assert.equal(releasePath({}, {}, fallback), fallback);
  assert.equal(releasePath({ SKILLHUB_CLEAN_MODE_RELEASES: "D:/demo.txt" }, {}, fallback), "D:/demo.txt");
  assert.equal(
    releasePath({ SKILLHUB_CLEAN_MODE_RELEASES: "D:/demo.txt" }, { SKILLHUB_CLEAN_MODE_RELEASES: "E:/x.txt" }, fallback),
    "E:/x.txt",
  );
});
