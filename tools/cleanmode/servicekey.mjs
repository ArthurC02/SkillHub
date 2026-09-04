// Mints the Virtual Key apps/llm runs with, instead of handing it the
// gateway's master key (ADR-017, iron rule 8: apps/llm must not hold it).
//
// Mirrors the shape apps/platform/internal/trial/execution/gateway.go already
// uses to mint a per-attempt key — same endpoint, same auth header, same body
// fields — so this is a second caller of the same contract, not a second
// contract.
//
// Split from start.mjs for the same reason env.mjs is: this has a branch
// (serviceKeyPlan) and a network call, both of which fail silently when
// wrong, and only a file a test can import catches that.

/**
 * Decides what this launch should do about apps/llm's LiteLLM key, without
 * doing it. Pure so the three outcomes are each one assertion in a test.
 *
 * - "mint": a gateway is configured and LITELLM_API_KEY is either unset or is
 *   a key this launcher must not hand to apps/llm anyway (the master key, or
 *   the admin key it read from SKILLHUB_MODEL_GATEWAY_KEY).
 * - "keep": an operator supplied a distinct key of their own — supplied wins,
 *   same rule ownedSettings() in start.mjs already follows.
 * - "skip": no gateway configured, so there is nothing to scope a key to.
 */
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

/**
 * POSTs /key/generate on the gateway's admin API and returns the minted key.
 *
 * `fetchImpl` is injected so the test never makes a network call — same
 * pattern as this module's only caller, which passes the real `fetch`.
 */
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

/**
 * The environment apps/llm is started with: the caller's, minus the two
 * credentials it must never hold (the gateway master key and the admin key
 * the platform mints with), plus the key this launch decided on. Pure, so the
 * "master key never reaches the child" rule is one assertion.
 */
export function llmChildEnv(base, key) {
  const env = { ...base };
  delete env.LITELLM_MASTER_KEY;
  delete env.SKILLHUB_MODEL_GATEWAY_KEY;
  if (key) env.LITELLM_API_KEY = key;
  else delete env.LITELLM_API_KEY;
  return env;
}

/**
 * The model names infra/compose/litellm-config.yaml serves — the allowlist a
 * minted key is scoped to. Read from the one file that defines them (the same
 * rule start.mjs follows for agentSdkVersion), never a second copy.
 */
export function gatewayModels(configText) {
  return [...configText.matchAll(/^\s*-\s*model_name:\s*(\S+)/gm)].map((m) => m[1]);
}
