export function serviceKeyPlan(deployment) {
  const adminUrl =
    deployment.SKILLHUB_MODEL_GATEWAY_ADMIN_URL || deployment.SKILLHUB_MODEL_GATEWAY_URL;
  const adminKey = deployment.SKILLHUB_MODEL_GATEWAY_KEY;
  if (!adminUrl || !adminKey) {
    return { action: "skip", reason: "no model gateway is configured" };
  }
  const current = deployment.LITELLM_API_KEY;
  if (!current) {
    return { action: "mint", reason: "LITELLM_API_KEY is not set" };
  }
  if (current === deployment.LITELLM_MASTER_KEY) {
    return { action: "mint", reason: "LITELLM_API_KEY is the master key" };
  }
  if (current === adminKey) {
    return { action: "mint", reason: "LITELLM_API_KEY is the gateway admin key" };
  }
  return { action: "keep", reason: "an operator supplied a distinct LITELLM_API_KEY" };
}

export async function mintServiceKey({ fetchImpl, adminUrl, adminKey, models, budgetUsd, alias }) {
  const response = await fetchImpl(`${adminUrl.replace(/\/$/, "")}/key/generate`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      Authorization: `Bearer ${adminKey}`,
    },
    body: JSON.stringify({
      key_alias: alias,
      duration: "24h",
      max_budget: budgetUsd,
      models,
    }),
  });
  if (!response.ok) {
    throw new Error(`向模型閘道簽發 apps/llm 的 Virtual Key 失敗（/key/generate -> ${response.status}）`);
  }
  const body = await response.json();
  if (!body.key) {
    throw new Error("模型閘道的 /key/generate 回應沒有 key 欄位");
  }
  return body.key;
}

export function llmChildEnv(base, key) {
  const env = { ...base };
  delete env.LITELLM_MASTER_KEY;
  delete env.SKILLHUB_MODEL_GATEWAY_KEY;
  if (key) env.LITELLM_API_KEY = key;
  else delete env.LITELLM_API_KEY;
  return env;
}

export function gatewayModels(configText) {
  return [...configText.matchAll(/^\s*-\s*model_name:\s*(\S+)/gm)].map((m) => m[1]);
}
