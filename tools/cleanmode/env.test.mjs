// Tests for how a clean-mode launch resolves deployment variables.
//
// `node --test`, no framework, same shape as
// infra/images/runtime-agent-sdk/run.test.mjs — the machine 02:PORT-005 is
// about installs nothing, and neither does this.
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

// 探索性測試（2026-09-02）找到的三件事。三件都是「設定被安靜地忽略」，
// 也就是這個檔案的檔頭自己寫下的、它存在的理由。

test("a trailing comment is not part of the value", () => {
  // 檔頭的例子逐字重現：有人在 .env 寫下這一行，然後這個模式什麼都沒發生。
  assert.equal(parseDotEnv("GENERATE_SKILL_EXPOSED=on # 只在 demo 開").GENERATE_SKILL_EXPOSED, "on");
  // 模型名字上更糟：帶著註解的名字閘道不認得，而那正是 preflight 唯一直接拒絕的組合。
  assert.equal(parseDotEnv("SKILLHUB_RUN_MODEL=gpt-5.4-mini # mini 層").SKILLHUB_RUN_MODEL, "gpt-5.4-mini");
  // 但 `#` 前面沒有空白時它是值的一部分：`p#ss` 是密碼，不是註解。
  assert.equal(parseDotEnv("A=p#ss").A, "p#ss");
  // 引號內的一律照留，連 `#` 也是。
  assert.equal(parseDotEnv('A="on # 真的要這個字串"').A, "on # 真的要這個字串");
});

test("an exported-but-empty shell variable is not an instruction", () => {
  const dotEnv = { OPERATOR_USER_IDS: "abc" };
  // 這一條是 start.mjs 那句「a roster set in .env is one somebody wrote down」的
  // 前提：`??` 會讓空字串贏，於是 launcher 認定沒有人指定過，覆蓋成 seed importer。
  assert.equal(resolve(dotEnv, { OPERATOR_USER_IDS: "" }, "OPERATOR_USER_IDS"), "abc");
  // 而且兩半必須一致，否則 child 拿到的與 launcher 以為的是兩個部署。
  assert.deepEqual(childOverlay(dotEnv, { OPERATOR_USER_IDS: "" }), { OPERATOR_USER_IDS: "abc" });
  // 真的設了值的 shell 仍然贏。
  assert.equal(resolve(dotEnv, { OPERATOR_USER_IDS: "xyz" }, "OPERATOR_USER_IDS"), "xyz");
  assert.deepEqual(childOverlay(dotEnv, { OPERATOR_USER_IDS: "xyz" }), {});
});

test("the release list the launcher announces is the one the API will read", () => {
  const fallback = "/tmp/launcher-owned.txt";
  // 沒有人指定 → launcher 自己的路徑，也就是它會種、會印的那一個。
  assert.equal(releasePath({}, {}, fallback), fallback);
  // .env 指定了 → API 讀那一個，所以種與印都必須跟著換；否則操作者會去改一個
  // 沒有人在讀的檔案，而拒絕訊息一次都不會變。
  assert.equal(releasePath({ SKILLHUB_CLEAN_MODE_RELEASES: "D:/demo.txt" }, {}, fallback), "D:/demo.txt");
  // shell 仍然贏過檔案。
  assert.equal(
    releasePath({ SKILLHUB_CLEAN_MODE_RELEASES: "D:/demo.txt" }, { SKILLHUB_CLEAN_MODE_RELEASES: "E:/x.txt" }, fallback),
    "E:/x.txt",
  );
});
