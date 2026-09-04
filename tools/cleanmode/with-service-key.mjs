// Runs a command — in practice apps/llm's uvicorn — with LITELLM_API_KEY set
// to a Virtual Key this launch minted, and with the gateway's master key and
// the platform's admin key removed from the child's environment.
//
//   node tools/cleanmode/with-service-key.mjs -- uv run uvicorn skillhub_llm.app:app
//
// Why a wrapper and not a line in start.mjs: apps/llm is not a child of the
// clean-mode launcher (it is the external service LLM_SERVICE_URL points at),
// so the place a key can be handed to it is the command that starts it. Every
// recorded real run before 2026-09-05 started apps/llm with the master key in
// LITELLM_API_KEY; apps/llm now refuses that (gateway.py, 503), so this is the
// only way `task dev:llm` still works. The key never touches stdout, a file or
// a log line — it goes from the gateway's reply into one child's env.
import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { childOverlay, readDotEnv, resolve } from "./env.mjs";
import { gatewayModels, llmChildEnv, mintServiceKey, serviceKeyPlan } from "./servicekey.mjs";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

const sep = process.argv.indexOf("--");
if (sep < 0 || sep === process.argv.length - 1) {
  console.error("用法：node tools/cleanmode/with-service-key.mjs -- <要啟動的命令> [參數…]");
  process.exit(2);
}
const [cmd, ...args] = process.argv.slice(sep + 1);

const dotEnv = readDotEnv(join(repoRoot, ".env"));
const names = [
  "SKILLHUB_MODEL_GATEWAY_URL",
  "SKILLHUB_MODEL_GATEWAY_ADMIN_URL",
  "SKILLHUB_MODEL_GATEWAY_KEY",
  "LITELLM_API_KEY",
  "LITELLM_MASTER_KEY",
];
const deployment = Object.fromEntries(names.map((n) => [n, resolve(dotEnv, process.env, n)]));
const plan = serviceKeyPlan(deployment);

let key = deployment.LITELLM_API_KEY;
if (plan.action === "mint") {
  let models = [];
  try {
    models = gatewayModels(readFileSync(join(repoRoot, "infra", "compose", "litellm-config.yaml"), "utf8"));
  } catch {
    models = [];
  }
  try {
    key = await mintServiceKey({
      fetchImpl: fetch,
      adminUrl: deployment.SKILLHUB_MODEL_GATEWAY_ADMIN_URL || deployment.SKILLHUB_MODEL_GATEWAY_URL,
      adminKey: deployment.SKILLHUB_MODEL_GATEWAY_KEY,
      models,
      budgetUsd: 1.0,
      alias: "skillhub-llm-service",
    });
  } catch (err) {
    // Never fall back to the master key: that is the deployment defect this
    // wrapper exists to end. No key at all is a 503 the operator can read.
    console.error(`${err.message}；apps/llm 不會拿 master key 啟動（${plan.reason}）`);
    process.exit(1);
  }
  console.error(`已為 apps/llm 簽發 Virtual Key（${plan.reason}；預算 1 USD、24 小時、${models.length} 個模型）`);
} else if (plan.action === "skip") {
  console.error("沒有設定模型閘道，apps/llm 不帶 LITELLM_API_KEY 啟動；需要模型的端點會回 503");
  key = "";
}

const env = llmChildEnv({ ...process.env, ...childOverlay(dotEnv, process.env) }, key);
const child = spawn(cmd, args, { stdio: "inherit", env, shell: process.platform === "win32" });
child.on("exit", (code, signal) => process.exit(code ?? (signal ? 1 : 0)));
