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
  const budgetUsd = Number(process.env.SKILLHUB_SERVICE_KEY_BUDGET_USD || "1.0");
  if (!Number.isFinite(budgetUsd) || budgetUsd <= 0 || budgetUsd > 20) {
    console.error("SKILLHUB_SERVICE_KEY_BUDGET_USD 必須是 0 到 20 之間的數字");
    process.exit(1);
  }
  try {
    key = await mintServiceKey({
      fetchImpl: fetch,
      adminUrl: deployment.SKILLHUB_MODEL_GATEWAY_ADMIN_URL || deployment.SKILLHUB_MODEL_GATEWAY_URL,
      adminKey: deployment.SKILLHUB_MODEL_GATEWAY_KEY,
      models,
      budgetUsd,
      alias: process.env.SKILLHUB_SERVICE_KEY_ALIAS || "skillhub-llm-service",
    });
  } catch (err) {
    console.error(`${err.message}；apps/llm 不會拿 master key 啟動（${plan.reason}）`);
    process.exit(1);
  }
  console.error(`已為 apps/llm 簽發 Virtual Key（${plan.reason}；預算 ${budgetUsd} USD、24 小時、${models.length} 個模型）`);
} else if (plan.action === "skip") {
  console.error("沒有設定模型閘道，apps/llm 不帶 LITELLM_API_KEY 啟動；需要模型的端點會回 503");
  key = "";
}

const env = llmChildEnv({ ...process.env, ...childOverlay(dotEnv, process.env) }, key);
const child = spawn(cmd, args, { stdio: "inherit", env, shell: process.platform === "win32" });
child.on("exit", (code, signal) => process.exit(code ?? (signal ? 1 : 0)));
